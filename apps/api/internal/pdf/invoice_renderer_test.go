package pdf

import (
	"bytes"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/shopspring/decimal"
	"testing"
	"time"
)

func TestInvoiceRenderer_RefusesUnauthorizedDocument(t *testing.T) {
	if _, err := NewInvoiceRenderer().Render(domain.Invoice{Status: domain.InvoiceStatusPending}); err == nil {
		t.Fatal("pending invoice rendered")
	}
}
func TestInvoiceRenderer_RendersAuthorizedSnapshotWithQR(t *testing.T) {
	number := int64(1)
	cae := "74123456789012"
	date := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	expiry := date.AddDate(0, 0, 10)
	invoice := domain.Invoice{Status: domain.InvoiceStatusIssued, Type: domain.InvoiceTypeA, Number: &number, PointOfSale: 1, CAE: &cae, CAEExpiresOn: &expiry, IssuedOn: date, CreatedAt: date, IssuerCUIT: "30712345678", IssuerCondition: domain.IVAConditionRegistered, Currency: "ARS", Receiver: domain.InvoiceReceiver{Name: "Obra Norte", DocType: domain.ReceiverDocCUIT, DocNumber: "20123456786", IVACondition: domain.IVAConditionRegistered}, Amounts: domain.InvoiceAmounts{Net: decimal.NewFromInt(1000), VAT: decimal.NewFromInt(210), Total: decimal.NewFromInt(1210)}, Snapshot: domain.QuoteRepresentationPayload{Supplier: domain.QuoteRepresentationSupplier{Name: "Corralón Ñandú"}, Currency: "ARS", Items: []domain.QuoteRepresentationItem{{ProductName: "Cemento", Quantity: "1.00", UnitPrice: "1000.00", Subtotal: "1000.00"}}}}
	content, err := NewInvoiceRenderer().Render(invoice)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(content, []byte("%PDF-")) || !bytes.Contains(content, []byte(domain.InvoiceQRURL(invoice))) {
		t.Fatal("missing fiscal PDF or QR verification link")
	}
}
