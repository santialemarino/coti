//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

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
			service := e.quoteDeliveryService(t, &captureWhatsAppSender{}, &stagedQuoteEmailSender{}, nil)
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
			if !e.customerOrderIDs(t, host, session)[seed.draft.RFQ.ID] {
				t.Fatal("a successful send to the proved recipient was missing from history")
			}
			assertLinks(current, session, http.StatusOK)
			assertLinks(beforeProof, session, http.StatusNotFound)
			otherRecipient := send(other)
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
