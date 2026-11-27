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
// the quote as its destination server-side, never in the link. A delivery made before the proof opens
// through the history a contact the customer wrote in from brings with it, or (decision 95) because
// the proof was born from that quote's own link; a seller-typed contact proved any other way waits
// for the seller to confirm the match.
func TestCustomerInvitation_VerificationReturnsToTheQuoteItWasFor(t *testing.T) {
	for _, tc := range []struct {
		label    string
		wroteIn  bool
		requests []string // the destinations asked for, in order; the last proof is confirmed
		wantSeen int
	}{
		{"contact the customer wrote in from", true, []string{"quote"}, http.StatusOK},
		{"contact a seller typed, proved from the quote's link", false, []string{"quote"}, http.StatusOK},
		{"contact a seller typed, proved from the profile", false, []string{"/profile"}, http.StatusNotFound},
		{"a quote-link proof replaced by one from the profile", false, []string{"quote", "/profile"}, http.StatusNotFound},
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
			destination := func(request string) string {
				if request == "quote" {
					return "/quotes/" + token
				}
				return request
			}

			host, session := e.registerCustomer(t, account, "20123456786")
			p := domain.CustomerPrincipal{AccountID: account, TaxID: "20123456786"}
			if err := e.db.CrossAccount().QueryRow(ctx, `SELECT id FROM customer_identity WHERE account_id=$1 AND tax_id=$2`, account, p.TaxID).Scan(&p.CustomerID); err != nil {
				t.Fatal(err)
			}
			if err := contacts.UpdateProfile(ctx, p, domain.ClientContactUpdate{Name: "Registered Name", Contacts: []domain.ClientContact{{Kind: domain.ChannelTypeEmail, Value: email, Primary: true}}}); err != nil {
				t.Fatal(err)
			}
			messenger := &captureContactMessenger{}
			verifier := services.NewClientContactVerificationService(contacts, store, messenger, sharedDestinationCaps(config.CustomerAuthConfig{ContactVerificationTTL: time.Minute, ContactVerificationCooldown: time.Nanosecond, ContactVerificationAttempts: 3}))
			var proof *domain.ContactChallenge
			var next string
			for _, request := range tc.requests {
				time.Sleep(time.Millisecond)
				next = destination(request)
				var err error
				if proof, err = verifier.Request(ctx, p, domain.ChannelTypeEmail, email, next); err != nil {
					t.Fatal(err)
				}
			}
			link, err := url.Parse(messenger.messages[len(messenger.messages)-1].Link)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(link.String(), token) || strings.Contains(link.String(), "profile") || len(link.Query()) != 2 {
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

// TC-10 (decision 95): a WhatsApp-only customer opens the quote they were sent by proving that very
// number from its link. Only that quote's live send to that number opens: another quote sent earlier
// to the same number, the same quote's send to another number, and an expired send stay hidden.
func TestCustomerInvitation_ProofFromAQuoteLinkOpensThatQuoteOnly(t *testing.T) {
	e := newEnv(t)
	invited := e.seedSendableQuote(t, "Quote link proof", false)
	earlier := e.seedSendableQuoteIn(t, invited, "Quote link proof earlier")
	ctx := context.Background()
	account := invited.tenant.AccountID
	profiles := repository.NewClientRepository()
	store := repository.NewClientContactRepository()
	contacts := services.NewClientContactService(e.db, profiles, store)
	name, phone, other := "Typed buyer", "+5491155550191", "+5491155550192"
	var source uuid.UUID
	if err := e.db.InTenantTx(ctx, invited.tenant, func(q repository.Querier) error {
		c, err := profiles.Create(ctx, q, account, domain.NewClient{Name: &name, Phone: &phone})
		if err == nil {
			source = c.ID
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	for _, seed := range []sendableQuote{invited, earlier} {
		if _, err := e.db.CrossAccount().Exec(ctx, `UPDATE quote SET client_id=$2 WHERE account_id=$3 AND id=$1`, seed.draft.Quote.ID, source, account); err != nil {
			t.Fatal(err)
		}
		if _, err := e.db.CrossAccount().Exec(ctx, `UPDATE rfq SET client_id=$2 WHERE account_id=$3 AND id=$1`, seed.draft.RFQ.ID, source, account); err != nil {
			t.Fatal(err)
		}
	}
	delivery := e.quoteDeliveryService(t, &captureWhatsAppSender{}, &stagedQuoteEmailSender{}, nil)
	tokenFor := func(seed sendableQuote, to string) string {
		t.Helper()
		sent, err := delivery.Send(ctx, seed.tenant, seed.draft.Quote.ID, domain.QuoteDeliveryInput{IdempotencyKey: uuid.New(), Phone: to})
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range sent.Deliveries {
			if d.ChannelType == domain.ChannelTypeWhatsApp {
				return d.PublicToken
			}
		}
		t.Fatal("no WhatsApp delivery")
		return ""
	}
	earlierToken := tokenFor(earlier, phone)
	invitedToken := tokenFor(invited, phone)
	otherNumberToken := tokenFor(invited, other)

	host, session := e.registerCustomer(t, account, "20123456786")
	p := domain.CustomerPrincipal{AccountID: account, TaxID: "20123456786"}
	if err := e.db.CrossAccount().QueryRow(ctx, `SELECT id FROM customer_identity WHERE account_id=$1 AND tax_id=$2`, account, p.TaxID).Scan(&p.CustomerID); err != nil {
		t.Fatal(err)
	}
	if err := contacts.UpdateProfile(ctx, p, domain.ClientContactUpdate{Name: "Registered Name", Contacts: []domain.ClientContact{{Kind: domain.ChannelTypeWhatsApp, Value: phone, Primary: true}}}); err != nil {
		t.Fatal(err)
	}
	opens := func(token string) int {
		t.Helper()
		return e.do(t, request{method: http.MethodGet, path: "/v1/customer/sites/" + host + "/quote-sends/" + token + "/order", token: session}).Code
	}
	if code := opens(invitedToken); code != http.StatusNotFound {
		t.Fatalf("the quote opened before any proof: %d", code)
	}

	messenger := &captureContactMessenger{}
	verifier := services.NewClientContactVerificationService(contacts, store, messenger, sharedDestinationCaps(config.CustomerAuthConfig{ContactVerificationTTL: time.Minute, ContactVerificationCooldown: time.Minute, ContactVerificationAttempts: 3}))
	proof, err := verifier.Request(ctx, p, domain.ChannelTypeWhatsApp, phone, "/quotes/"+invitedToken)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := verifier.Confirm(ctx, p, proof.ID, messenger.messages[0].Code); err != nil {
		t.Fatal(err)
	}

	for label, tc := range map[string]struct {
		token string
		want  int
	}{
		"the quote whose link the proof came from":      {invitedToken, http.StatusOK},
		"another quote sent earlier to the same number": {earlierToken, http.StatusNotFound},
		"the same quote sent to another number":         {otherNumberToken, http.StatusNotFound},
	} {
		if code := opens(tc.token); code != tc.want {
			t.Errorf("%s: %d, want %d", label, code, tc.want)
		}
	}
	orders := e.customerOrderIDs(t, host, session)
	if !orders[invited.draft.RFQ.ID] || orders[earlier.draft.RFQ.ID] {
		t.Errorf("orders = %v, want the invited request and not the earlier one", orders)
	}
	if _, err := e.db.CrossAccount().Exec(ctx, `UPDATE quote_send SET expires_at=now()-interval '1 hour' WHERE account_id=$1 AND public_token=$2`, account, invitedToken); err != nil {
		t.Fatal(err)
	}
	if code := opens(invitedToken); code != http.StatusNotFound {
		t.Errorf("an expired send still opens through the proof: %d", code)
	}
}
