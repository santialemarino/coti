//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/santialemarino/coti/apps/api/internal/config"
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
	selection := &domain.AssociateQuoteClientInput{New: &domain.NewClient{Name: &name, Phone: &phone}}
	input := domain.QuoteDeliveryInput{IdempotencyKey: uuid.New(), Phone: phone, ClientAssociation: selection}
	result, err := delivery.Send(context.Background(), seed.tenant, seed.draft.Quote.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	var clientID, requestClientID uuid.UUID
	var taxID *string
	var status string
	if err := e.db.CrossAccount().QueryRow(context.Background(), `SELECT qt.client_id,r.client_id,c.tax_id,qt.current_status FROM quote qt JOIN rfq r ON r.account_id=qt.account_id AND r.id=qt.rfq_id JOIN client c ON c.account_id=qt.account_id AND c.id=qt.client_id WHERE qt.id=$1`, seed.draft.Quote.ID).Scan(&clientID, &requestClientID, &taxID, &status); err != nil {
		t.Fatal(err)
	}
	if clientID != requestClientID || taxID != nil || status != "SENT" || len(result.Deliveries) != 2 {
		t.Fatalf("association at publication: %s %s %v %s", clientID, requestClientID, taxID, status)
	}
	// The phone the seller typed proves nothing on its own: the customer's proof raises a case and
	// opens nothing until the seller confirms the join, so a typo alone hands over no history.
	host, token := e.registerCustomer(t, seed.tenant.AccountID, "30712345671")
	e.verifyCustomerContact(t, seed.tenant.AccountID, "30712345671", domain.ChannelTypeWhatsApp, phone)
	order := "/v1/customer/sites/" + host + "/orders/" + seed.draft.RFQ.ID.String()
	response := e.do(t, request{method: http.MethodGet, path: order, token: token})
	if response.Code != http.StatusNotFound {
		t.Fatalf("a proof of the seller-typed phone opened the order: %d %s", response.Code, response.Body)
	}
	var merged *uuid.UUID
	if err := e.db.CrossAccount().QueryRow(context.Background(), `SELECT merged_into FROM client WHERE id=$1`, clientID).Scan(&merged); err != nil || merged != nil {
		t.Fatalf("the seller's profile merged into %v (%v), want it left alone", merged, err)
	}
	var registered uuid.UUID
	if err := e.db.CrossAccount().QueryRow(context.Background(), `SELECT id FROM client WHERE account_id=$1 AND tax_id='30712345671'`, seed.tenant.AccountID).Scan(&registered); err != nil {
		t.Fatal(err)
	}
	cases := e.pendingMatches(t, seed.tenant.AccountID, phone)
	if len(cases) != 1 || cases[0] != [2]uuid.UUID{clientID, registered} {
		t.Fatalf("pending cases after the proof = %v, want one from the seller's profile %s to the account %s", cases, clientID, registered)
	}
	if _, err = delivery.Send(context.Background(), seed.tenant, seed.draft.Quote.ID, input); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := e.db.CrossAccount().QueryRow(context.Background(), `SELECT count(*) FROM client WHERE account_id=$1 AND tax_id=$2`, seed.tenant.AccountID, "30712345671").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("retry created %d profiles", count)
	}
	changedName := "Different profile"
	selection.New.Name = &changedName
	if _, err = delivery.Send(context.Background(), seed.tenant, seed.draft.Quote.ID, input); domain.CodeOf(err) != domain.CodeIdempotencyMismatch {
		t.Fatalf("changed selection retry: %v", err)
	}
	seller := e.tokenFor(t, domain.AppUser{ID: seed.tenant.UserID, AccountID: seed.tenant.AccountID, Role: domain.UserRoleAdmin, SessionEpoch: 1})
	response = e.do(t, request{method: http.MethodPut, path: "/v1/quotes/" + seed.draft.Quote.ID.String() + "/client-association", token: seller, branch: seed.tenant.BranchID.String(), body: map[string]any{"client_id": clientID}})
	if response.Code != http.StatusConflict {
		t.Fatalf("retired standalone association: %d %s", response.Code, response.Body)
	}

	// The seller confirms: the profile joins the customer's account, which survives and shows the order.
	caseID := e.pendingCaseID(t, seed.tenant.AccountID, clientID, registered)
	response = e.do(t, request{method: http.MethodPut, path: "/v1/client-contact-suggestions/" + caseID.String(), token: seller, branch: seed.tenant.BranchID.String(), body: map[string]any{"action": "MERGE"}})
	if response.Code != http.StatusNoContent {
		t.Fatalf("join into the account: %d %s", response.Code, response.Body)
	}
	if err := e.db.CrossAccount().QueryRow(context.Background(), `SELECT merged_into FROM client WHERE id=$1`, clientID).Scan(&merged); err != nil || merged == nil || *merged != registered {
		t.Fatalf("the seller's profile merged into %v (%v), want the account %s", merged, err, registered)
	}
	var audits int
	if err := e.db.CrossAccount().QueryRow(context.Background(), `SELECT count(*) FROM client_merge_audit WHERE source_client_id=$1 AND target_client_id=$2 AND merged_by=$3`, clientID, registered, seed.tenant.UserID).Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("audit rows by the seller = %d (%v), want 1", audits, err)
	}
	response = e.do(t, request{method: http.MethodGet, path: order, token: token})
	var visible struct {
		Token *string `json:"delivery_token"`
	}
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &visible) != nil || visible.Token == nil {
		t.Fatalf("the joined order: %d %s, want it in the customer's history", response.Code, response.Body)
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
	service.WithClientAssociations(services.NewClientService(e.db, repository.NewClientRepository(), repository.NewTagRepository(config.DefaultClientTagRules()), repository.NewQuoteRepository(), failedSendClientLink{}, repository.NewQuoteSendRepository()))
	name := "Rollback customer"
	phone := "+5491155550188"
	input := domain.QuoteDeliveryInput{IdempotencyKey: uuid.New(), Phone: "+5491155550188", ClientAssociation: &domain.AssociateQuoteClientInput{New: &domain.NewClient{Name: &name, Phone: &phone}}}
	if _, err := service.Send(context.Background(), seed.tenant, seed.draft.Quote.ID, input); err == nil {
		t.Fatal("failed association committed")
	}
	var profiles, published int
	var clientID *uuid.UUID
	var status string
	if err := e.db.CrossAccount().QueryRow(context.Background(), `SELECT client_id,current_status,(SELECT count(*) FROM client WHERE account_id=$1 AND name='Rollback customer'),(SELECT count(*) FROM quote_send WHERE account_id=$1 AND tracking_status='SENT') FROM quote WHERE account_id=$1 AND id=$2`, seed.tenant.AccountID, seed.draft.Quote.ID).Scan(&clientID, &status, &profiles, &published); err != nil {
		t.Fatal(err)
	}
	if clientID != nil || status != "QUOTED" || profiles != 0 || published != 0 {
		t.Fatalf("partial association: %v %s %d %d", clientID, status, profiles, published)
	}
	service.WithClientAssociations(services.NewClientService(e.db, repository.NewClientRepository(), repository.NewTagRepository(config.DefaultClientTagRules()), repository.NewQuoteRepository(), repository.NewRFQRepository(), repository.NewQuoteSendRepository()))
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

func TestClientProfile_SendPreservesImmutableCUITAndExistingContacts(t *testing.T) {
	e := newEnv(t)
	seed := e.seedSendableQuote(t, "Preserve client identity", false)
	var clientID uuid.UUID
	if err := e.db.CrossAccount().QueryRow(context.Background(), `SELECT client_id FROM quote WHERE account_id=$1 AND id=$2`, seed.tenant.AccountID, seed.draft.Quote.ID).Scan(&clientID); err != nil {
		t.Fatal(err)
	}
	var before string
	if err := e.db.CrossAccount().QueryRow(context.Background(), `SELECT tax_id FROM client WHERE account_id=$1 AND id=$2`, seed.tenant.AccountID, clientID).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.CrossAccount().Exec(context.Background(), `UPDATE client SET tax_id=NULL WHERE account_id=$1 AND id=$2`, seed.tenant.AccountID, clientID); err == nil {
		t.Fatal("database allowed clearing a registered CUIT")
	}
	sender := &captureWhatsAppSender{}
	service := e.quoteDeliveryService(t, sender, &stagedQuoteEmailSender{}, nil)
	input := domain.QuoteDeliveryInput{IdempotencyKey: uuid.New(), Phone: "+5491155550199", ClientAssociation: &domain.AssociateQuoteClientInput{ClientID: &clientID}}
	if _, err := service.Send(context.Background(), seed.tenant, seed.draft.Quote.ID, input); err != nil {
		t.Fatal(err)
	}
	var after string
	if err := e.db.CrossAccount().QueryRow(context.Background(), `SELECT tax_id FROM client WHERE account_id=$1 AND id=$2`, seed.tenant.AccountID, clientID).Scan(&after); err != nil || before != after {
		t.Fatalf("CUIT changed: %q %q %v", before, after, err)
	}
	name, phone := "Another client", "+5491155550888"
	input.IdempotencyKey = uuid.New()
	input.ClientAssociation = &domain.AssociateQuoteClientInput{New: &domain.NewClient{Name: &name, Phone: &phone}}
	if _, err := service.Send(context.Background(), seed.tenant, seed.draft.Quote.ID, input); !errors.Is(err, domain.ErrConflict) || sender.count() != 1 {
		t.Fatalf("published reassignment: %v, sends %d", err, sender.count())
	}
}

// A quote that already has a client can go out to another one on its first send, an existing
// profile or a new one: the choice is validated before anything goes out, and the send then lands
// whole (the quote SENT, its sends recorded, the client changed). A choice that fails validation
// sends nothing.
func TestClientProfile_FirstSendCanChangeTheClient(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	existing := e.seedSendableQuote(t, "First send other client", false)
	created := e.seedSendableQuoteIn(t, existing, "First send new client")
	account := existing.tenant.AccountID
	other := uuid.New()
	if _, err := e.db.CrossAccount().Exec(ctx, `INSERT INTO client(id,account_id,name,phone) VALUES($1,$2,'Otro cliente','+5491155550777')`, other, account); err != nil {
		t.Fatal(err)
	}
	sender := &captureWhatsAppSender{}
	delivery := e.quoteDeliveryService(t, sender, &stagedQuoteEmailSender{}, nil)
	state := func(seed sendableQuote) (status string, client uuid.UUID, rfqClient uuid.UUID, sent int) {
		t.Helper()
		if err := e.db.CrossAccount().QueryRow(ctx, `SELECT q.current_status, q.client_id, r.client_id,
 (SELECT count(*) FROM quote_send s JOIN quote_version v ON v.id=s.version_id WHERE v.quote_id=q.id AND s.tracking_status='SENT')
 FROM quote q JOIN rfq r ON r.id=q.rfq_id WHERE q.id=$1`, seed.draft.Quote.ID).Scan(&status, &client, &rfqClient, &sent); err != nil {
			t.Fatal(err)
		}
		return
	}

	if _, err := delivery.Send(ctx, existing.tenant, existing.draft.Quote.ID, domain.QuoteDeliveryInput{IdempotencyKey: uuid.New(),
		Phone: "+5491155550777", ClientAssociation: &domain.AssociateQuoteClientInput{ClientID: &other}}); err != nil {
		t.Fatalf("first send to another existing client = %v", err)
	}
	if status, client, rfqClient, sent := state(existing); status != "SENT" || client != other || rfqClient != other || sent != 2 {
		t.Errorf("existing client: status %s, client %v, request %v, %d sends; want SENT to the other client with both sends", status, client, rfqClient, sent)
	}

	name, phone := "Cliente nuevo", "+5491155550778"
	if _, err := delivery.Send(ctx, created.tenant, created.draft.Quote.ID, domain.QuoteDeliveryInput{IdempotencyKey: uuid.New(),
		Phone: phone, ClientAssociation: &domain.AssociateQuoteClientInput{New: &domain.NewClient{Name: &name, Phone: &phone}}}); err != nil {
		t.Fatalf("first send to a new client = %v", err)
	}
	status, client, _, sent := state(created)
	var clientName string
	if err := e.db.CrossAccount().QueryRow(ctx, `SELECT name FROM client WHERE id=$1`, client).Scan(&clientName); err != nil {
		t.Fatal(err)
	}
	if status != "SENT" || clientName != name || sent != 2 {
		t.Errorf("new client: status %s, client %q, %d sends; want SENT to the new profile with both sends", status, clientName, sent)
	}

	refused := e.seedSendableQuote(t, "First send refused client", false)
	calls := sender.count()
	missing := uuid.New()
	_, err := delivery.Send(ctx, refused.tenant, refused.draft.Quote.ID, domain.QuoteDeliveryInput{IdempotencyKey: uuid.New(),
		Phone: "+5491155550779", ClientAssociation: &domain.AssociateQuoteClientInput{ClientID: &missing}})
	if err == nil || sender.count() != calls {
		t.Fatalf("a refused client choice = %v after %d calls, want an error and nothing sent", err, sender.count()-calls)
	}
	if status, _, _, sent := state(refused); status != "QUOTED" || sent != 0 {
		t.Errorf("refused choice left status %s and %d sends, want the quote untouched", status, sent)
	}
	var pending int
	if err := e.db.CrossAccount().QueryRow(ctx, `SELECT count(*) FROM quote_send s JOIN quote_version v ON v.id=s.version_id WHERE v.quote_id=$1`, refused.draft.Quote.ID).Scan(&pending); err != nil || pending != 0 {
		t.Errorf("refused choice created %d sends (%v)", pending, err)
	}
}
