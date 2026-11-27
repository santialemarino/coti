//go:build integration

package integration

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
	"github.com/santialemarino/coti/apps/api/internal/services"
)

// A customer invited by a quote signs up and verifies the address it went to. The verification keeps
// the quote as its destination server-side, never in the link. Whether the quote then opens follows
// the visibility rule: a delivery made before the proof is only reachable through history, which a
// contact the customer wrote in from brings with it and one a seller typed does not, until the seller
// confirms the match.
func TestCustomerInvitation_VerificationReturnsToTheQuoteItWasFor(t *testing.T) {
	for _, tc := range []struct {
		label    string
		wroteIn  bool
		wantSeen int
	}{
		{"contact the customer wrote in from", true, http.StatusOK},
		{"contact a seller typed", false, http.StatusNotFound},
	} {
		t.Run(tc.label, func(t *testing.T) {
			e := newEnv(t)
			seed := e.seedSendableQuote(t, "Invitation "+tc.label, false)
			ctx := context.Background()
			account := seed.tenant.AccountID
			if _, err := e.db.CrossAccount().Exec(ctx, `INSERT INTO channel(account_id,branch_id,type,identifier) VALUES($1,$2,'EMAIL','branch@example.test')`, account, seed.tenant.BranchID); err != nil {
				t.Fatal(err)
			}
			profiles := repository.NewClientRepository()
			store := repository.NewClientContactRepository()
			contacts := services.NewClientContactService(e.db, profiles, store)
			name, email := "Invited buyer", "invited@example.test"
			var source uuid.UUID
			if err := e.db.InTenantTx(ctx, seed.tenant, func(q repository.Querier) error {
				c, err := profiles.Create(ctx, q, account, domain.NewClient{Name: &name, Email: &email})
				if err == nil {
					source = c.ID
				}
				return err
			}); err != nil {
				t.Fatal(err)
			}
			if tc.wroteIn {
				e.channelVerify(t, account, source)
			}
			for _, table := range []string{"quote", "rfq"} {
				id := seed.draft.Quote.ID
				if table == "rfq" {
					id = seed.draft.RFQ.ID
				}
				if _, err := e.db.CrossAccount().Exec(ctx, `UPDATE `+table+` SET client_id=$2 WHERE account_id=$3 AND id=$1`, id, source, account); err != nil {
					t.Fatal(err)
				}
			}
			sent, err := e.quoteDeliveryService(t, &captureWhatsAppSender{}, &stagedQuoteEmailSender{}, nil).Send(ctx, seed.tenant,
				seed.draft.Quote.ID, domain.QuoteDeliveryInput{IdempotencyKey: uuid.New(), Email: &email})
			if err != nil {
				t.Fatal(err)
			}
			var token string
			for _, delivery := range sent.Deliveries {
				if delivery.ChannelType == domain.ChannelTypeEmail {
					token = delivery.PublicToken
				}
			}
			next := "/quotes/" + token

			host, session := e.registerCustomer(t, account, "20123456786")
			p := domain.CustomerPrincipal{AccountID: account, TaxID: "20123456786"}
			if err := e.db.CrossAccount().QueryRow(ctx, `SELECT id FROM customer_identity WHERE account_id=$1 AND tax_id=$2`, account, p.TaxID).Scan(&p.CustomerID); err != nil {
				t.Fatal(err)
			}
			if err := contacts.UpdateProfile(ctx, p, domain.ClientContactUpdate{Name: "Registered Name", Contacts: []domain.ClientContact{{Kind: domain.ChannelTypeEmail, Value: email, Primary: true}}}); err != nil {
				t.Fatal(err)
			}
			messenger := &captureContactMessenger{}
			verifier := services.NewClientContactVerificationService(contacts, store, messenger, sharedDestinationCaps(config.CustomerAuthConfig{ContactVerificationTTL: time.Minute, ContactVerificationCooldown: time.Minute, ContactVerificationAttempts: 3}))
			proof, err := verifier.Request(ctx, p, domain.ChannelTypeEmail, email, next)
			if err != nil {
				t.Fatal(err)
			}
			link, err := url.Parse(messenger.messages[0].Link)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(link.String(), token) || len(link.Query()) != 2 {
				t.Fatalf("verification link %q carries more than the challenge and its secret", link)
			}
			view, err := verifier.Describe(ctx, p, proof.ID)
			if err != nil || view.Status != domain.ContactChallengePending || view.Value != email || view.NextPath != next {
				t.Fatalf("before confirming: %+v, %v", view, err)
			}
			returned, err := verifier.Confirm(ctx, p, proof.ID, link.Query().Get("token"))
			if err != nil || returned != next {
				t.Fatalf("Confirm() = %q, %v, want %q", returned, err, next)
			}
			if again, err := verifier.Confirm(ctx, p, proof.ID, link.Query().Get("token")); err != nil || again != next {
				t.Fatalf("a repeated confirmation = %q, %v, want the same destination", again, err)
			}
			if view, err := verifier.Describe(ctx, p, proof.ID); err != nil || view.Status != domain.ContactChallengeVerified {
				t.Fatalf("after confirming: %+v, %v", view, err)
			}
			path := "/v1/customer/sites/" + host + "/quote-sends/" + token + "/order"
			if rec := e.do(t, request{method: http.MethodGet, path: path, token: session}); rec.Code != tc.wantSeen {
				t.Fatalf("quote sent before the proof: %d %s, want %d", rec.Code, rec.Body, tc.wantSeen)
			}
		})
	}
}

// Only a portal page is ever stored as a destination, and another customer cannot read the proof.
func TestCustomerInvitation_StoresOnlyAPortalDestination(t *testing.T) {
	e := newEnv(t)
	seed := e.seedSendableQuote(t, "Invitation destination", false)
	ctx := context.Background()
	account := seed.tenant.AccountID
	e.registerCustomer(t, account, "20123456786")
	p := domain.CustomerPrincipal{AccountID: account, TaxID: "20123456786"}
	if err := e.db.CrossAccount().QueryRow(ctx, `SELECT id FROM customer_identity WHERE account_id=$1 AND tax_id=$2`, account, p.TaxID).Scan(&p.CustomerID); err != nil {
		t.Fatal(err)
	}
	store := repository.NewClientContactRepository()
	contacts := services.NewClientContactService(e.db, repository.NewClientRepository(), store)
	verifier := services.NewClientContactVerificationService(contacts, store, &captureContactMessenger{}, sharedDestinationCaps(config.CustomerAuthConfig{ContactVerificationTTL: time.Minute, ContactVerificationCooldown: time.Nanosecond, ContactVerificationAttempts: 3}))
	email := "customer20123456786@example.test"
	var first uuid.UUID
	for _, next := range []string{"https://evil.test/quotes/x", "//evil.test", "/quotes/x?y=1", "/admin"} {
		proof, err := verifier.Request(ctx, p, domain.ChannelTypeEmail, email, next)
		if err != nil {
			t.Fatal(err)
		}
		if first == uuid.Nil {
			first = proof.ID
		}
		if view, err := verifier.Describe(ctx, p, proof.ID); err != nil || view.NextPath != "" {
			t.Fatalf("next %q stored as %+v, %v", next, view, err)
		}
		time.Sleep(time.Millisecond)
	}
	// A newer request cancels the older proof: its link is dead, not a verification that happened.
	if view, err := verifier.Describe(ctx, p, first); err != nil || view.Status != domain.ContactChallengeExpired {
		t.Fatalf("a cancelled proof reads %+v, %v, want EXPIRED", view, err)
	}
	e.registerCustomer(t, account, "30712345671")
	other := domain.CustomerPrincipal{AccountID: account, TaxID: "30712345671"}
	if err := e.db.CrossAccount().QueryRow(ctx, `SELECT id FROM customer_identity WHERE account_id=$1 AND tax_id=$2`, account, other.TaxID).Scan(&other.CustomerID); err != nil {
		t.Fatal(err)
	}
	proof, err := verifier.Request(ctx, p, domain.ChannelTypeEmail, email, "/orders")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := verifier.Describe(ctx, other, proof.ID); err == nil {
		t.Fatal("another customer read the verification")
	}
}
