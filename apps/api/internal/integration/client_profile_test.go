//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
	"github.com/santialemarino/coti/apps/api/internal/services"
)

func TestClientProfile_AssociatesOnSendBeforeCustomerAcceptance(t *testing.T) {
	e := newEnv(t)
	seed := e.seedSendableQuote(t, "Send client association", false)
	if _, err := e.db.CrossAccount().Exec(context.Background(), `UPDATE quote SET client_id=NULL WHERE id=$1`, seed.draft.Quote.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.CrossAccount().Exec(context.Background(), `UPDATE rfq SET client_id=NULL WHERE id=$1`, seed.draft.RFQ.ID); err != nil {
		t.Fatal(err)
	}
	delivery := e.quoteDeliveryService(t, &captureWhatsAppSender{}, &stagedQuoteEmailSender{}, nil)
	name := "Constructora Horizonte"
	phone := "+5491155550188"
	selection := &domain.AssociateQuoteClientInput{TaxID: "30712345671", New: &domain.NewClient{Name: &name, Phone: &phone}}
	input := domain.QuoteDeliveryInput{IdempotencyKey: uuid.New(), Phone: phone, ClientAssociation: selection}
	result, err := delivery.Send(context.Background(), seed.tenant, seed.draft.Quote.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	var clientID, requestClientID uuid.UUID
	var taxID, status string
	if err := e.db.CrossAccount().QueryRow(context.Background(), `SELECT qt.client_id,r.client_id,c.tax_id,qt.current_status FROM quote qt JOIN rfq r ON r.account_id=qt.account_id AND r.id=qt.rfq_id JOIN client c ON c.account_id=qt.account_id AND c.id=qt.client_id WHERE qt.id=$1`, seed.draft.Quote.ID).Scan(&clientID, &requestClientID, &taxID, &status); err != nil {
		t.Fatal(err)
	}
	if clientID != requestClientID || taxID != "30712345671" || status != "SENT" || len(result.Deliveries) != 2 {
		t.Fatalf("association at publication: %s %s %s %s", clientID, requestClientID, taxID, status)
	}
	host, token := e.registerCustomer(t, seed.tenant.AccountID, taxID, "")
	response := e.do(t, request{method: http.MethodGet, path: "/v1/customer/sites/" + host + "/orders/" + seed.draft.RFQ.ID.String(), token: token})
	var order struct {
		Token *string `json:"delivery_token"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &order); err != nil {
		t.Fatal(err)
	}
	if response.Code != 200 || order.Token == nil {
		t.Fatalf("pre-acceptance history: %d %s", response.Code, response.Body)
	}
	if _, err = delivery.Send(context.Background(), seed.tenant, seed.draft.Quote.ID, input); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := e.db.CrossAccount().QueryRow(context.Background(), `SELECT count(*) FROM client WHERE account_id=$1 AND tax_id=$2`, seed.tenant.AccountID, taxID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("retry created %d profiles", count)
	}
	selection.TaxID = "20123456786"
	if _, err = delivery.Send(context.Background(), seed.tenant, seed.draft.Quote.ID, input); domain.CodeOf(err) != domain.CodeIdempotencyMismatch {
		t.Fatalf("changed selection retry: %v", err)
	}
	seller := e.tokenFor(t, domain.AppUser{ID: seed.tenant.UserID, AccountID: seed.tenant.AccountID, Role: domain.UserRoleAdmin, SessionEpoch: 1})
	response = e.do(t, request{method: http.MethodPut, path: "/v1/quotes/" + seed.draft.Quote.ID.String() + "/client-association", token: seller, branch: seed.tenant.BranchID.String(), body: map[string]any{"client_id": clientID}})
	if response.Code != http.StatusConflict {
		t.Fatalf("retired standalone association: %d %s", response.Code, response.Body)
	}
}

type failedSendClientLink struct{}

func (failedSendClientLink) SetClient(context.Context, repository.Querier, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID) error {
	return errors.New("request association storage unavailable")
}

func TestClientProfile_PublicationFailureRollsBackProfileAndAssociations(t *testing.T) {
	e := newEnv(t)
	seed := e.seedSendableQuote(t, "Association rollback", false)
	if _, err := e.db.CrossAccount().Exec(context.Background(), `UPDATE quote SET client_id=NULL WHERE id=$1`, seed.draft.Quote.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.CrossAccount().Exec(context.Background(), `UPDATE rfq SET client_id=NULL WHERE id=$1`, seed.draft.RFQ.ID); err != nil {
		t.Fatal(err)
	}
	service := e.quoteDeliveryService(t, &captureWhatsAppSender{}, &stagedQuoteEmailSender{}, nil)
	service.WithClientAssociations(services.NewClientService(e.db, repository.NewClientRepository(), repository.NewTagRepository(), repository.NewQuoteRepository(), failedSendClientLink{}, repository.NewQuoteSendRepository()))
	name := "Rollback customer"
	input := domain.QuoteDeliveryInput{IdempotencyKey: uuid.New(), Phone: "+5491155550188", ClientAssociation: &domain.AssociateQuoteClientInput{TaxID: "30712345671", New: &domain.NewClient{Name: &name}}}
	if _, err := service.Send(context.Background(), seed.tenant, seed.draft.Quote.ID, input); err == nil {
		t.Fatal("failed association committed")
	}
	var profiles, published int
	var clientID *uuid.UUID
	var status string
	if err := e.db.CrossAccount().QueryRow(context.Background(), `SELECT client_id,current_status,(SELECT count(*) FROM client WHERE account_id=$1 AND tax_id='30712345671'),(SELECT count(*) FROM quote_send WHERE account_id=$1 AND tracking_status='SENT') FROM quote WHERE account_id=$1 AND id=$2`, seed.tenant.AccountID, seed.draft.Quote.ID).Scan(&clientID, &status, &profiles, &published); err != nil {
		t.Fatal(err)
	}
	if clientID != nil || status != "QUOTED" || profiles != 0 || published != 0 {
		t.Fatalf("partial association: %v %s %d %d", clientID, status, profiles, published)
	}
	service.WithClientAssociations(services.NewClientService(e.db, repository.NewClientRepository(), repository.NewTagRepository(), repository.NewQuoteRepository(), repository.NewRFQRepository(), repository.NewQuoteSendRepository()))
	if _, err := service.Send(context.Background(), seed.tenant, seed.draft.Quote.ID, input); err != nil {
		t.Fatalf("retry after rollback: %v", err)
	}
}

func TestClientProfile_InvalidSelectionNeverPublishesOrCreatesAClient(t *testing.T) {
	e := newEnv(t)
	seed := e.seedSendableQuote(t, "Invalid send client", false)
	sender := &captureWhatsAppSender{}
	delivery := e.quoteDeliveryService(t, sender, &stagedQuoteEmailSender{}, nil)
	foreignAccount, _ := e.seedAccount(t, "Foreign client owner")
	missing := uuid.New()
	if _, err := e.db.CrossAccount().Exec(context.Background(), `INSERT INTO client(id,account_id,name,tax_id) VALUES($1,$2,'Foreign client','20123456786')`, missing, foreignAccount); err != nil {
		t.Fatal(err)
	}
	_, err := delivery.Send(context.Background(), seed.tenant, seed.draft.Quote.ID, domain.QuoteDeliveryInput{IdempotencyKey: uuid.New(), Phone: "+5491155550188", ClientAssociation: &domain.AssociateQuoteClientInput{TaxID: "20123456786", ClientID: &missing}})
	if err == nil || sender.count() != 0 {
		t.Fatalf("invalid selection: %v, sends %d", err, sender.count())
	}
	var count int
	if err := e.db.CrossAccount().QueryRow(context.Background(), `SELECT count(*) FROM quote_send WHERE account_id=$1`, seed.tenant.AccountID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("invalid selection created %d publications", count)
	}
}

func TestClientProfile_SendRequiresCUITAndPreservesExistingContacts(t *testing.T) {
	e := newEnv(t)
	seed := e.seedSendableQuote(t, "Confirm existing client", false)
	var clientID uuid.UUID
	if err := e.db.CrossAccount().QueryRow(context.Background(), `SELECT client_id FROM quote WHERE id=$1`, seed.draft.Quote.ID).Scan(&clientID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.CrossAccount().Exec(context.Background(), `UPDATE client SET tax_id=NULL,phone='+5491155550100',email='original@example.test' WHERE id=$1`, clientID); err != nil {
		t.Fatal(err)
	}
	sender := &captureWhatsAppSender{}
	service := e.quoteDeliveryService(t, sender, &stagedQuoteEmailSender{}, nil)
	input := domain.QuoteDeliveryInput{IdempotencyKey: uuid.New(), Phone: "+5491155550199"}
	if _, err := service.Send(context.Background(), seed.tenant, seed.draft.Quote.ID, input); !errors.Is(err, domain.ErrInvalidInput) || sender.count() != 0 {
		t.Fatalf("missing CUIT: %v, provider calls %d", err, sender.count())
	}
	input.ClientAssociation = &domain.AssociateQuoteClientInput{TaxID: "20123456786", ClientID: &clientID}
	if _, err := service.Send(context.Background(), seed.tenant, seed.draft.Quote.ID, input); err != nil {
		t.Fatal(err)
	}
	var taxID, phone, email string
	if err := e.db.CrossAccount().QueryRow(context.Background(), `SELECT tax_id,phone,email FROM client WHERE id=$1`, clientID).Scan(&taxID, &phone, &email); err != nil {
		t.Fatal(err)
	}
	if taxID != "20123456786" || phone != "+5491155550100" || email != "original@example.test" {
		t.Fatalf("profile contents: %s %s %s", taxID, phone, email)
	}
	name := "Another client"
	input.IdempotencyKey = uuid.New()
	input.ClientAssociation = &domain.AssociateQuoteClientInput{TaxID: "30712345671", New: &domain.NewClient{Name: &name}}
	if _, err := service.Send(context.Background(), seed.tenant, seed.draft.Quote.ID, input); !errors.Is(err, domain.ErrConflict) || sender.count() != 1 {
		t.Fatalf("published client reassignment: %v, provider calls %d", err, sender.count())
	}
}
