package domain_test

import (
	"testing"
	"time"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// Complete is accepted + paid + (invoiced, waived, or an account that does not invoice).
func TestSaleComplete(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name    string
		status  domain.QuoteStatus
		closure domain.SaleClosure
		want    bool
	}{
		{"accepted, unpaid", domain.QuoteStatusAccepted, domain.SaleClosure{InvoicingEnabled: true, Invoiced: true}, false},
		{"paid, waiting for the invoice", domain.QuoteStatusAccepted, domain.SaleClosure{PaidAt: &now, InvoicingEnabled: true}, false},
		{"paid and invoiced", domain.QuoteStatusAccepted, domain.SaleClosure{PaidAt: &now, InvoicingEnabled: true, Invoiced: true}, true},
		{"paid and waived", domain.QuoteStatusAccepted, domain.SaleClosure{PaidAt: &now, InvoicingEnabled: true, InvoiceWaivedAt: &now}, true},
		{"paid, invoicing off", domain.QuoteStatusAccepted, domain.SaleClosure{PaidAt: &now}, true},
		{"unpaid, invoicing off", domain.QuoteStatusAccepted, domain.SaleClosure{}, false},
		{"not accepted", domain.QuoteStatusSent, domain.SaleClosure{PaidAt: &now, Invoiced: true}, false},
	}
	for _, tc := range cases {
		if got := domain.SaleComplete(tc.status, tc.closure); got != tc.want {
			t.Errorf("%s: SaleComplete = %v, want %v", tc.name, got, tc.want)
		}
	}
}
