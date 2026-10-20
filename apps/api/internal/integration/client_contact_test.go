//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
	"github.com/santialemarino/coti/apps/api/internal/services"
)

func TestCustomerProfile_SavesOwnContactsAndKeepsRecoverySeparate(t *testing.T) {
	e := newEnv(t)
	account, _ := e.seedAccount(t, "Customer contacts")
	other, _ := e.seedAccount(t, "Other customer contacts")
	host, token := e.registerCustomer(t, account, "20123456786", "recovery@example.test")
	otherHost, otherToken := e.registerCustomer(t, other, "20123456786", "")
	base := "/v1/customer/sites/" + host + "/profile"
	input := map[string]any{"name": "Customer", "client_id": uuid.New(), "tax_id": "30712345671", "contacts": []map[string]any{
		{"kind": "EMAIL", "value": " COMMERCIAL@EXAMPLE.TEST ", "primary": true},
		{"kind": "EMAIL", "value": "other@example.test", "primary": false},
		{"kind": "WHATSAPP", "value": "+54 9 (11) 5555-0101", "primary": true},
	}}
	for range 2 {
		response := e.do(t, request{method: http.MethodPut, path: base, token: token, body: input})
		if response.Code != 204 {
			t.Fatalf("save: %d %s", response.Code, response.Body)
		}
	}
	var count int
	var recovery string
	if err := e.db.CrossAccount().QueryRow(context.Background(), `SELECT count(*) FROM client_contact WHERE account_id=$1`, account).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("retry contacts=%d", count)
	}
	if err := e.db.CrossAccount().QueryRow(context.Background(), `SELECT recovery_email FROM customer_identity WHERE account_id=$1`, account).Scan(&recovery); err != nil {
		t.Fatal(err)
	}
	if recovery != "recovery@example.test" {
		t.Fatal("commercial update changed recovery")
	}
	response := e.do(t, request{method: http.MethodGet, path: base, token: token})
	var result struct {
		TaxID    string `json:"tax_id"`
		Contacts []struct {
			Kind    string `json:"kind"`
			Value   string `json:"value"`
			Primary bool   `json:"primary"`
		} `json:"contacts"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if response.Code != 200 || result.TaxID != "20123456786" || len(result.Contacts) != 3 {
		t.Fatalf("own profile: %d %s", response.Code, response.Body)
	}
	response = e.do(t, request{method: http.MethodGet, path: base, token: otherToken})
	if response.Code != 401 {
		t.Fatalf("foreign supplier token: %d", response.Code)
	}
	response = e.do(t, request{method: http.MethodGet, path: "/v1/customer/sites/" + otherHost + "/profile", token: otherToken})
	if response.Code != 200 {
		t.Fatal(response.Body)
	}
	var foreign struct {
		Contacts []any `json:"contacts"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &foreign); err != nil {
		t.Fatal(err)
	}
	if len(foreign.Contacts) != 0 {
		t.Fatal("contacts crossed suppliers with the same CUIT")
	}
	invalid := map[string]any{"contacts": []map[string]any{{"kind": "EMAIL", "value": "one@example.test", "primary": false}}}
	response = e.do(t, request{method: http.MethodPut, path: base, token: token, body: invalid})
	if response.Code != 422 {
		t.Fatalf("missing primary: %d %s", response.Code, response.Body)
	}
	response = e.do(t, request{method: http.MethodPut, path: base, token: token, body: map[string]any{"name": "Customer", "contacts": []any{}}})
	if response.Code != 204 {
		t.Fatal(response.Body)
	}
	if err := e.db.CrossAccount().QueryRow(context.Background(), `SELECT count(*) FROM client_contact WHERE account_id=$1`, account).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("optional contacts could not be cleared")
	}
}

func TestClientContacts_IngressSuggestionsAndIsolation(t *testing.T) {
	e := newEnv(t)
	account, branch := e.seedAccount(t, "Ingress contacts")
	other, otherBranch := e.seedAccount(t, "Other ingress")
	store := repository.NewClientContactRepository()
	profiles := repository.NewClientRepository()
	service := services.NewClientContactService(e.db, profiles, store)
	route := domain.InboundChannelRoute{AccountID: account, BranchID: branch}
	for _, kind := range []domain.ChannelType{domain.ChannelTypeWhatsApp, domain.ChannelTypeEmail} {
		value := "one@example.test"
		newValue := "two@example.test"
		if kind == domain.ChannelTypeWhatsApp {
			value = "+5491155550101"
			newValue = "+5491155550102"
		}
		id := uuid.New()
		taxID := "20123456786"
		if kind == domain.ChannelTypeEmail {
			taxID = "30712345671"
		}
		name := "Pepe"
		record := func(message uuid.UUID, contact string, hints domain.ClientContactHints) {
			t.Helper()
			err := e.db.InTenantTx(context.Background(), route.Tenant(), func(q repository.Querier) error {
				return service.RecordInbound(context.Background(), q, route, message, kind, contact, "Retained original", hints)
			})
			if err != nil {
				t.Fatal(err)
			}
		}
		record(id, value, domain.ClientContactHints{Name: &name, TaxID: &taxID})
		record(id, value, domain.ClientContactHints{Name: &name, TaxID: &taxID})
		record(uuid.New(), newValue, domain.ClientContactHints{TaxID: &taxID})
	}
	tenant := domain.Tenant{AccountID: account, Role: domain.UserRoleAdmin, BranchID: branch}
	suggestions, err := service.ListSuggestions(context.Background(), tenant)
	if err != nil {
		t.Fatal(err)
	}
	if len(suggestions) != 2 {
		t.Fatalf("pending=%d", len(suggestions))
	}
	foreign, err := service.ListSuggestions(context.Background(), domain.Tenant{AccountID: other, BranchID: otherBranch})
	if err != nil || len(foreign) != 0 {
		t.Fatalf("foreign suggestions: %v %v", foreign, err)
	}
	err = e.db.InTenantTx(context.Background(), domain.Tenant{AccountID: other}, func(q repository.Querier) error {
		_, err := store.GetObservation(context.Background(), q, domain.Tenant{AccountID: other, Role: domain.UserRoleAdmin}, suggestions[0].ID)
		return err
	})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("foreign lookup: %v", err)
	}
	owner, err := e.db.AdminTx(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Error(err)
		}
	})
	_, err = store.GetObservation(context.Background(), owner, domain.Tenant{AccountID: other, Role: domain.UserRoleAdmin}, suggestions[0].ID)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("application account predicate with RLS bypassed: %v", err)
	}
	var count int
	if err := e.db.CrossAccount().QueryRow(context.Background(), `SELECT count(*) FROM client_contact WHERE account_id=$1`, account).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatal("new contact was automatically associated")
	}
	err = e.db.InTenantTx(context.Background(), domain.Tenant{AccountID: other}, func(q repository.Querier) error {
		var visible int
		if err := q.QueryRow(context.Background(), `SELECT count(*) FROM client_contact WHERE account_id=$1`, account).Scan(&visible); err != nil {
			return err
		}
		if visible != 0 {
			t.Fatal("RLS exposed foreign contacts")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestClientContacts_ConfirmedMergePreservesProfilesContactsAndOrders(t *testing.T) {
	e := newEnv(t)
	seed := e.seedSendableQuote(t, "Merge contacts", false)
	ctx := context.Background()
	var source uuid.UUID
	if err := e.db.CrossAccount().QueryRow(ctx, `SELECT client_id FROM quote WHERE id=$1`, seed.draft.Quote.ID).Scan(&source); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.CrossAccount().Exec(ctx, `UPDATE client SET tax_id=NULL WHERE id=$1`, source); err != nil {
		t.Fatal(err)
	}
	profiles := repository.NewClientRepository()
	contacts := repository.NewClientContactRepository()
	service := services.NewClientContactService(e.db, profiles, contacts)
	taxID := "20123456786"
	name := "Canonical customer"
	var target *domain.Client
	err := e.db.InTenantTx(ctx, seed.tenant, func(q repository.Querier) error {
		var err error
		target, err = profiles.Create(ctx, q, seed.tenant.AccountID, domain.NewClient{TaxID: &taxID, Name: &name})
		if err != nil {
			return err
		}
		if err := contacts.Save(ctx, q, seed.tenant.AccountID, source, []domain.ClientContact{{Kind: domain.ChannelTypeEmail, Value: "source@example.test", Primary: true}}, "SELLER"); err != nil {
			return err
		}
		if err := contacts.Save(ctx, q, seed.tenant.AccountID, target.ID, []domain.ClientContact{{Kind: domain.ChannelTypeEmail, Value: "target@example.test", Primary: true}}, "SELLER"); err != nil {
			return err
		}
		return service.RecordInbound(ctx, q, domain.InboundChannelRoute{AccountID: seed.tenant.AccountID, BranchID: seed.tenant.BranchID}, uuid.New(), domain.ChannelTypeEmail, "source@example.test", "CUIT explicitly declared", domain.ClientContactHints{TaxID: &taxID})
	})
	if err != nil {
		t.Fatal(err)
	}
	suggestions, err := service.ListSuggestions(ctx, seed.tenant)
	if err != nil || len(suggestions) != 1 {
		t.Fatalf("suggestions: %v %v", suggestions, err)
	}
	for range 2 {
		if err := service.ResolveSuggestion(ctx, seed.tenant, suggestions[0].ID, "MERGE"); err != nil {
			t.Fatal(err)
		}
	}
	var quoteClient, rfqClient, mergedInto uuid.UUID
	if err := e.db.CrossAccount().QueryRow(ctx, `SELECT q.client_id,r.client_id,c.merged_into FROM quote q JOIN rfq r ON r.id=q.rfq_id JOIN client c ON c.id=$2 WHERE q.id=$1`, seed.draft.Quote.ID, source).Scan(&quoteClient, &rfqClient, &mergedInto); err != nil {
		t.Fatal(err)
	}
	if quoteClient != target.ID || rfqClient != target.ID || mergedInto != target.ID {
		t.Fatal("merge did not preserve and transfer associations")
	}
	var count, primaries int
	if err := e.db.CrossAccount().QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE is_primary) FROM client_contact WHERE account_id=$1 AND client_id=$2`, seed.tenant.AccountID, target.ID).Scan(&count, &primaries); err != nil {
		t.Fatal(err)
	}
	if count != 2 || primaries != 1 {
		t.Fatalf("merged contacts=%d primaries=%d", count, primaries)
	}
}
