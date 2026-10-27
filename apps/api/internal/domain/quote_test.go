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
		{"paid, waived, a voucher pending", domain.QuoteStatusAccepted, domain.SaleClosure{PaidAt: &now, InvoicingEnabled: true, InvoiceWaivedAt: &now, InvoicePending: true}, false},
		{"paid, invoicing off, a voucher pending", domain.QuoteStatusAccepted, domain.SaleClosure{PaidAt: &now, InvoicePending: true}, false},
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

// A voucher waiting for ARCA wins over every other fact, then an invoice, then the waiver.
func TestInvoiceStatusOf(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name    string
		closure domain.SaleClosure
		want    domain.SaleInvoiceStatus
	}{
		{"nothing yet", domain.SaleClosure{PaidAt: &now}, domain.SaleInvoiceStatusUninvoiced},
		{"invoiced", domain.SaleClosure{Invoiced: true}, domain.SaleInvoiceStatusInvoiced},
		{"waived", domain.SaleClosure{InvoiceWaivedAt: &now}, domain.SaleInvoiceStatusWaived},
		{"pending", domain.SaleClosure{InvoicePending: true}, domain.SaleInvoiceStatusPending},
		{"invoiced, a credit note pending", domain.SaleClosure{Invoiced: true, InvoicePending: true}, domain.SaleInvoiceStatusPending},
		{"waived, then invoiced", domain.SaleClosure{Invoiced: true, InvoiceWaivedAt: &now}, domain.SaleInvoiceStatusInvoiced},
	}
	for _, tc := range cases {
		if got := domain.InvoiceStatusOf(tc.closure); got != tc.want {
			t.Errorf("%s: InvoiceStatusOf = %q, want %q", tc.name, got, tc.want)
		}
	}
}
