//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func TestCustomerHistory_OutgoingDeliveryRequiresItsRecipientsProof(t *testing.T) {
	for _, kind := range []domain.ChannelType{domain.ChannelTypeEmail, domain.ChannelTypeWhatsApp} {
		t.Run(string(kind), func(t *testing.T) {
			e := newEnv(t)
			seed := e.seedSendableQuote(t, "Outgoing history "+string(kind), false)
			account, branch := seed.tenant.AccountID, seed.tenant.BranchID
			ctx := context.Background()
			if _, err := e.db.CrossAccount().Exec(ctx, `INSERT INTO channel(account_id,branch_id,type,identifier) VALUES($1,$2,'EMAIL','branch@example.test')`, account, branch); err != nil {
				t.Fatal(err)
			}
			host, session := e.registerCustomer(t, account, "20123456786")
			base := "/v1/customer/sites/" + host
			value, other := "customer20123456786@example.test", "another@example.test"
			if kind == domain.ChannelTypeWhatsApp {
				value, other = "+5491155550147", "+5491155550148"
			}
			whatsappSender, emailSender := &captureWhatsAppSender{}, &stagedQuoteEmailSender{}
			service := e.quoteDeliveryService(t, whatsappSender, emailSender, nil)
			// Decision 92: a recipient not yet proved in a portal account is invited to sign up and
			// come back to the order; a proved one gets the order link itself.
			assertMessageLink := func(result *domain.QuoteDeliveryResult, invitation bool) {
				t.Helper()
				var token, link string
				for _, delivery := range result.Deliveries {
					if delivery.ChannelType == kind {
						token = delivery.PublicToken
					}
				}
				if kind == domain.ChannelTypeEmail {
					emailSender.mu.Lock()
					link = emailSender.links[len(emailSender.links)-1]
					files := emailSender.files[len(emailSender.files)-1]
					emailSender.mu.Unlock()
					if len(files) != 1 || !strings.HasPrefix(string(files[0].Content), "%PDF") {
						t.Fatalf("email attachments = %d, want the quote PDF", len(files))
					}
				} else {
					whatsappSender.mu.Lock()
					message := whatsappSender.messages[len(whatsappSender.messages)-1]
					whatsappSender.mu.Unlock()
					link = message.PublicURL
					if !strings.Contains(message.Body, link) || strings.Contains(message.Body, domain.QuotePublicURLPlaceholder) {
						t.Fatalf("WhatsApp body does not carry its link: %q", message.Body)
					}
				}
				want := "/quotes/" + token
				if invitation {
					want = "/signup?next=%2Fquotes%2F" + token
				}
				if !strings.HasSuffix(link, want) {
					t.Fatalf("message link = %q, want it to end in %q", link, want)
				}
			}
			send := func(destination string) *domain.QuoteDeliveryResult {
				t.Helper()
				input := domain.QuoteDeliveryInput{IdempotencyKey: uuid.New()}
				if kind == domain.ChannelTypeEmail {
					input.Email = &destination
				} else {
					input.Phone = destination
				}
				result, err := service.Send(ctx, seed.tenant, seed.draft.Quote.ID, input)
				if err != nil {
					t.Fatal(err)
				}
				if len(result.Deliveries) != 2 {
					t.Fatalf("deliveries=%d, want recipient and portal", len(result.Deliveries))
				}
				return result
			}
			assertLinks := func(result *domain.QuoteDeliveryResult, customerSession string, status int) {
				t.Helper()
				for _, delivery := range result.Deliveries {
					rec := e.do(t, request{method: http.MethodGet, path: base + "/quote-sends/" + delivery.PublicToken + "/order", token: customerSession})
					if rec.Code != status {
						t.Fatalf("%s link: %d %s, want %d", delivery.ChannelType, rec.Code, rec.Body, status)
					}
				}
			}

			beforeProof := send(value)
			assertMessageLink(beforeProof, true)
			assertLinks(beforeProof, session, http.StatusNotFound)
			e.verifyCustomerContact(t, account, "20123456786", kind, value)
			var includesHistory bool
			if err := e.db.CrossAccount().QueryRow(ctx, `SELECT o.includes_profile_history FROM customer_contact_ownership o
 JOIN client_contact c ON c.account_id=o.account_id AND c.id=o.contact_id WHERE o.account_id=$1 AND c.kind=$2 AND c.value=$3`, account, kind, value).Scan(&includesHistory); err != nil {
				t.Fatal(err)
			}
			if includesHistory || e.customerOrderIDs(t, host, session)[seed.draft.RFQ.ID] {
				t.Fatal("a new contact unlocked an earlier profile or delivery")
			}

			destination := value
			if kind == domain.ChannelTypeEmail {
				destination = strings.ToUpper(value)
			}
			current := send(destination)
			assertMessageLink(current, false)
			if !e.customerOrderIDs(t, host, session)[seed.draft.RFQ.ID] {
				t.Fatal("a successful send to the proved recipient was missing from history")
			}
			assertLinks(current, session, http.StatusOK)
			assertLinks(beforeProof, session, http.StatusNotFound)
			// A send to another recipient of a SENT quote states the expiry the quote already has.
			var expiresAt time.Time
			if err := e.db.CrossAccount().QueryRow(ctx, `UPDATE quote SET expires_at=now()+interval '2 days' WHERE account_id=$1 AND id=$2 RETURNING expires_at`, account, seed.draft.Quote.ID).Scan(&expiresAt); err != nil {
				t.Fatal(err)
			}
			otherRecipient := send(other)
			wantValidity := "Válida hasta el " + expiresAt.In(domain.Argentina).Format("2/1/2006") + "."
			var stated string
			if kind == domain.ChannelTypeEmail {
				emailSender.mu.Lock()
				stated = strings.Join(emailSender.closing[len(emailSender.closing)-1], " ")
				emailSender.mu.Unlock()
			} else {
				whatsappSender.mu.Lock()
				stated = whatsappSender.messages[len(whatsappSender.messages)-1].Body
				whatsappSender.mu.Unlock()
			}
			if !strings.Contains(stated, wantValidity) {
				t.Fatalf("re-send states %q, want %q", stated, wantValidity)
			}
			assertLinks(otherRecipient, session, http.StatusNotFound)
			assertLinks(current, session, http.StatusOK)
			rec := e.do(t, request{method: http.MethodGet, path: base + "/orders/" + seed.draft.RFQ.ID.String(), token: session})
			var response struct {
				DeliveryToken *string `json:"delivery_token"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if rec.Code != http.StatusOK || response.DeliveryToken == nil {
				t.Fatalf("order detail: %d %s", rec.Code, rec.Body)
			}
			owned := false
			for _, delivery := range current.Deliveries {
				owned = owned || *response.DeliveryToken == delivery.PublicToken
			}
			if !owned {
				t.Fatal("history selected a different recipient's publication")
			}
			_, otherSession := e.registerCustomer(t, account, "30712345671")
			assertLinks(current, otherSession, http.StatusNotFound)
			if e.customerOrderIDs(t, host, otherSession)[seed.draft.RFQ.ID] {
				t.Fatal("the send exposed history to another customer")
			}
			foreignAccount, _ := e.seedAccount(t, "Other outgoing supplier")
			foreignHost, foreignSession := e.registerCustomer(t, foreignAccount, "20123456786")
			e.verifyCustomerContact(t, foreignAccount, "20123456786", kind, value)
			if rec := e.do(t, request{method: http.MethodGet, path: "/v1/customer/sites/" + foreignHost + "/quote-sends/" + current.Deliveries[0].PublicToken + "/order", token: foreignSession}); rec.Code != http.StatusNotFound {
				t.Fatalf("same contact at another supplier: %d %s", rec.Code, rec.Body)
			}

			for _, delivery := range current.Deliveries {
				if delivery.ChannelType != domain.ChannelTypeWebApp {
					if _, err := e.db.CrossAccount().Exec(ctx, `UPDATE quote_send SET tracking_status='FAILED' WHERE account_id=$1 AND id=$2`, account, delivery.ID); err != nil {
						t.Fatal(err)
					}
				}
			}
			assertLinks(current, session, http.StatusNotFound)
			if e.customerOrderIDs(t, host, session)[seed.draft.RFQ.ID] {
				t.Fatal("a portal publication without a successful recipient delivery granted access")
			}
			otherBranch, otherChannel := uuid.New(), uuid.New()
			if _, err := e.db.CrossAccount().Exec(ctx, `INSERT INTO branch(id,account_id,name) VALUES($1,$2,'Other branch')`, otherBranch, account); err != nil {
				t.Fatal(err)
			}
			if _, err := e.db.CrossAccount().Exec(ctx, `INSERT INTO channel(id,account_id,branch_id,type) VALUES($1,$2,$3,$4)`, otherChannel, account, otherBranch, kind); err != nil {
				t.Fatal(err)
			}
			for _, delivery := range current.Deliveries {
				if delivery.ChannelType != domain.ChannelTypeWebApp {
					if _, err := e.db.CrossAccount().Exec(ctx, `INSERT INTO quote_send(account_id,version_id,channel_id,idempotency_key,destination,public_token,format,tracking_status,sent_at,expires_at)
 SELECT account_id,version_id,$3,idempotency_key,destination,$4,format,'SENT',sent_at,expires_at FROM quote_send WHERE account_id=$1 AND id=$2`, account, delivery.ID, otherChannel, uuid.NewString()); err != nil {
						t.Fatal(err)
					}
				}
			}
			assertLinks(current, session, http.StatusNotFound)
			if e.customerOrderIDs(t, host, session)[seed.draft.RFQ.ID] {
				t.Fatal("a delivery channel at another branch opened this publication")
			}
		})
	}
}

// Only an active customer's live contact earns the order link: a retired contact or a disabled
// account is invited again, since neither could open the order after logging in.
func TestQuoteDelivery_InvitesAContactNoActiveAccountHolds(t *testing.T) {
	for label, revoke := range map[string]string{
		"retired contact":  `UPDATE client_contact SET retired_at=now() WHERE account_id=$1 AND kind='WHATSAPP' AND value=$2`,
		"disabled account": `UPDATE customer_identity SET disabled_at=now() WHERE account_id=$1 AND id IN (SELECT o.customer_id FROM customer_contact_ownership o JOIN client_contact c ON c.account_id=o.account_id AND c.id=o.contact_id WHERE o.account_id=$1 AND c.value=$2)`,
	} {
		t.Run(label, func(t *testing.T) {
			e := newEnv(t)
			seed := e.seedSendableQuote(t, "Revoked portal "+label, false)
			account := seed.tenant.AccountID
			ctx := context.Background()
			e.registerCustomer(t, account, "20123456786")
			phone := "+5491155550149"
			e.verifyCustomerContact(t, account, "20123456786", domain.ChannelTypeWhatsApp, phone)
			if _, err := e.db.CrossAccount().Exec(ctx, revoke, account, phone); err != nil {
				t.Fatal(err)
			}
			sender := &captureWhatsAppSender{}
			result, err := e.quoteDeliveryService(t, sender, &stagedQuoteEmailSender{}, nil).Send(ctx, seed.tenant,
				seed.draft.Quote.ID, domain.QuoteDeliveryInput{IdempotencyKey: uuid.New(), Phone: phone})
			if err != nil {
				t.Fatal(err)
			}
			for _, delivery := range result.Deliveries {
				if delivery.ChannelType == domain.ChannelTypeWhatsApp &&
					!strings.HasSuffix(sender.messages[0].PublicURL, "/signup?next=%2Fquotes%2F"+delivery.PublicToken) {
					t.Fatalf("link = %q, want the sign-up invitation", sender.messages[0].PublicURL)
				}
			}
		})
	}
}
