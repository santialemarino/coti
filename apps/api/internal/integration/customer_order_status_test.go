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

// The customer's last status follows the backoffice's sale-complete rule: an accepted sale reads
// COMPLETED once paid and invoiced, waived, or on an account that does not invoice.
func TestCustomerOrders_AcceptedSaleReadsCompletedOnceClosed(t *testing.T) {
	e := newEnv(t)
	seed := e.seedSendableQuote(t, "Order status", false)
	account, quoteID := seed.tenant.AccountID, seed.draft.Quote.ID
	ctx := context.Background()
	if _, err := e.db.CrossAccount().Exec(ctx, `INSERT INTO channel(account_id,branch_id,type,identifier) VALUES($1,$2,'EMAIL','branch@example.test')`, account, seed.tenant.BranchID); err != nil {
		t.Fatal(err)
	}
	host, session := e.registerCustomer(t, account, "20123456786")
	value := "customer20123456786@example.test"
	e.verifyCustomerContact(t, account, "20123456786", domain.ChannelTypeEmail, value)
	service := e.quoteDeliveryService(t, &captureWhatsAppSender{}, &stagedQuoteEmailSender{}, nil)
	if _, err := service.Send(ctx, seed.tenant, quoteID, domain.QuoteDeliveryInput{IdempotencyKey: uuid.New(), Email: &value}); err != nil {
		t.Fatal(err)
	}
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := e.db.CrossAccount().Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	status := func() string {
		t.Helper()
		rec := e.do(t, request{method: http.MethodGet, path: "/v1/customer/sites/" + host + "/orders/" + seed.draft.RFQ.ID.String(), token: session})
		var order struct {
			Status string `json:"status"`
		}
		if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &order) != nil {
			t.Fatalf("order detail: %d %s", rec.Code, rec.Body)
		}
		list := e.do(t, request{method: http.MethodGet, path: "/v1/customer/sites/" + host + "/orders", token: session})
		var page []struct {
			ID     uuid.UUID `json:"id"`
			Status string    `json:"status"`
		}
		if list.Code != http.StatusOK || json.Unmarshal(list.Body.Bytes(), &page) != nil {
			t.Fatalf("order list: %d %s", list.Code, list.Body)
		}
		for _, item := range page {
			if item.ID == seed.draft.RFQ.ID && item.Status != order.Status {
				t.Fatalf("list says %q, detail says %q", item.Status, order.Status)
			}
		}
		return order.Status
	}

	exec(`UPDATE account SET invoicing_enabled = TRUE WHERE id = $1`, account)
	exec(`UPDATE quote SET current_status = 'ACCEPTED' WHERE account_id = $1 AND id = $2`, account, quoteID)
	if got := status(); got != "ACCEPTED" {
		t.Fatalf("accepted and unpaid = %q, want ACCEPTED", got)
	}
	exec(`UPDATE quote SET paid_at = now(), paid_by_user_id = $3 WHERE account_id = $1 AND id = $2`, account, quoteID, seed.tenant.UserID)
	if got := status(); got != "ACCEPTED" {
		t.Fatalf("paid but not invoiced on an invoicing account = %q, want ACCEPTED", got)
	}
	exec(`UPDATE account SET invoicing_enabled = FALSE WHERE id = $1`, account)
	if got := status(); got != "COMPLETED" {
		t.Fatalf("paid on an account that does not invoice = %q, want COMPLETED", got)
	}
	exec(`UPDATE account SET invoicing_enabled = TRUE WHERE id = $1`, account)
	exec(`UPDATE quote SET invoice_waived_at = now(), invoice_waived_by_user_id = $3 WHERE account_id = $1 AND id = $2`, account, quoteID, seed.tenant.UserID)
	if got := status(); got != "COMPLETED" {
		t.Fatalf("paid with the invoice waived = %q, want COMPLETED", got)
	}
}
