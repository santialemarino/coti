//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func TestClientProfile_ConfirmsAcceptedSaleWithoutAutoCreatingOnSend(t *testing.T) {
	e := newEnv(t)
	var clientID, tagID uuid.UUID
	t.Cleanup(func() {
		if clientID != uuid.Nil {
			e.mustCleanup(t, `DELETE FROM client_tag WHERE client_id = $1`, clientID)
			e.mustCleanup(t, `DELETE FROM client WHERE id = $1`, clientID)
		}
		if tagID != uuid.Nil {
			e.mustCleanup(t, `DELETE FROM tag WHERE id = $1`, tagID)
		}
	})

	seed := e.seedSendableQuote(t, "Client profile association", false)
	delivery := e.quoteDeliveryService(t, &captureWhatsAppSender{},
		&stagedQuoteEmailSender{}, nil)
	phone := "+5491155550188"
	result, err := delivery.Send(context.Background(), seed.tenant, seed.draft.Quote.ID,
		domain.QuoteDeliveryInput{IdempotencyKey: uuid.New(), Phone: phone})
	if err != nil {
		t.Fatalf("Send() = %v, want no error", err)
	}

	var quoteClientID, rfqClientID *uuid.UUID
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT quote.client_id, rfq.client_id FROM quote
		 JOIN rfq ON rfq.id = quote.rfq_id WHERE quote.id = $1`, seed.draft.Quote.ID,
	).Scan(&quoteClientID, &rfqClientID); err != nil {
		t.Fatalf("read client association after send: %v", err)
	}
	if quoteClientID != nil || rfqClientID != nil {
		t.Fatalf("delivery auto-associated quote/RFQ = %v/%v, want manual confirmation",
			quoteClientID, rfqClientID)
	}

	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`UPDATE quote SET current_status = 'ACCEPTED' WHERE id = $1`, seed.draft.Quote.ID); err != nil {
		t.Fatalf("accept quote: %v", err)
	}
	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`INSERT INTO quote_status_change
		 (account_id, quote_id, previous_status, new_status, user_id)
		 VALUES ($1, $2, 'SENT', 'ACCEPTED', $3)`, seed.tenant.AccountID,
		seed.draft.Quote.ID, seed.tenant.UserID); err != nil {
		t.Fatalf("record acceptance: %v", err)
	}

	token := e.tokenFor(t, domain.AppUser{ID: seed.tenant.UserID,
		AccountID: seed.tenant.AccountID, Role: domain.UserRoleAdmin, SessionEpoch: 1})
	tagResponse := e.do(t, request{method: http.MethodPost, path: "/v1/tags", token: token,
		body: map[string]any{"name": "  Recurrente  "}})
	if tagResponse.Code != http.StatusCreated {
		t.Fatalf("POST /v1/tags = %d %s, want 201", tagResponse.Code, tagResponse.Body)
	}
	var tag struct {
		ID   uuid.UUID `json:"id"`
		Name string    `json:"name"`
	}
	if err := json.Unmarshal(tagResponse.Body.Bytes(), &tag); err != nil {
		t.Fatalf("decode tag response: %v", err)
	}
	tagID = tag.ID
	if tag.Name != "Recurrente" {
		t.Fatalf("normalized tag = %q, want Recurrente", tag.Name)
	}

	associationResponse := e.do(t, request{method: http.MethodGet,
		path:  "/v1/quotes/" + seed.draft.Quote.ID.String() + "/client-association",
		token: token, branch: seed.tenant.BranchID.String()})
	if associationResponse.Code != http.StatusOK {
		t.Fatalf("GET client association = %d %s, want 200", associationResponse.Code,
			associationResponse.Body)
	}
	var association struct {
		CurrentClient any   `json:"current_client"`
		Suggestions   []any `json:"suggestions"`
		ContactHints  struct {
			Phone *string `json:"phone"`
		} `json:"contact_hints"`
	}
	if err := json.Unmarshal(associationResponse.Body.Bytes(), &association); err != nil {
		t.Fatalf("decode association response: %v", err)
	}
	if association.CurrentClient != nil || len(association.Suggestions) != 0 ||
		association.ContactHints.Phone == nil || *association.ContactHints.Phone != phone {
		t.Fatalf("association before confirmation = %#v, want only the delivery phone hint",
			association)
	}

	name := "Constructora Horizonte"
	confirmationResponse := e.do(t, request{method: http.MethodPut,
		path:  "/v1/quotes/" + seed.draft.Quote.ID.String() + "/client-association",
		token: token, branch: seed.tenant.BranchID.String(), body: map[string]any{
			"new_client": map[string]any{"name": name, "phone": phone},
			"tag_ids":    []uuid.UUID{tag.ID},
		}})
	if confirmationResponse.Code != http.StatusOK {
		t.Fatalf("PUT client association = %d %s, want 200", confirmationResponse.Code,
			confirmationResponse.Body)
	}
	var match struct {
		Client struct {
			ID uuid.UUID `json:"id"`
		} `json:"client"`
		Tags []struct {
			ID uuid.UUID `json:"id"`
		} `json:"tags"`
	}
	if err := json.Unmarshal(confirmationResponse.Body.Bytes(), &match); err != nil {
		t.Fatalf("decode confirmation response: %v", err)
	}
	clientID = match.Client.ID

	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT quote.client_id, rfq.client_id FROM quote
		 JOIN rfq ON rfq.id = quote.rfq_id WHERE quote.id = $1`, seed.draft.Quote.ID,
	).Scan(&quoteClientID, &rfqClientID); err != nil {
		t.Fatalf("read confirmed client association: %v", err)
	}
	if quoteClientID == nil || rfqClientID == nil || *quoteClientID != clientID ||
		*rfqClientID != clientID {
		t.Fatalf("confirmed quote/RFQ client = %v/%v, want %v", quoteClientID, rfqClientID,
			clientID)
	}

	profileResponse := e.do(t, request{method: http.MethodGet,
		path: "/v1/clients/" + clientID.String(), token: token,
		branch: seed.tenant.BranchID.String()})
	if profileResponse.Code != http.StatusOK {
		t.Fatalf("GET client profile = %d %s, want 200", profileResponse.Code,
			profileResponse.Body)
	}
	var profile struct {
		Tags []struct {
			ID uuid.UUID `json:"id"`
		} `json:"tags"`
		Sales []struct {
			QuoteID uuid.UUID `json:"quote_id"`
			Total   string    `json:"total"`
		} `json:"sales"`
	}
	if err := json.Unmarshal(profileResponse.Body.Bytes(), &profile); err != nil {
		t.Fatalf("decode profile response: %v", err)
	}
	if len(profile.Tags) != 1 || profile.Tags[0].ID != tag.ID || len(profile.Sales) != 1 ||
		profile.Sales[0].QuoteID != seed.draft.Quote.ID || profile.Sales[0].Total != "100.00" {
		t.Fatalf("profile = %#v, want one tag and the accepted sale", profile)
	}
	if len(result.Deliveries) != 1 {
		t.Fatalf("deliveries = %d, want one", len(result.Deliveries))
	}
}
