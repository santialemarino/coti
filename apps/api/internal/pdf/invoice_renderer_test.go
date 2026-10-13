package pdf

import (
	"bytes"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/shopspring/decimal"
	"image"
	"image/png"
	"strings"
	"testing"
	"time"
)

func TestInvoiceRenderer_RefusesUnauthorizedDocument(t *testing.T) {
	if _, err := NewInvoiceRenderer().Render(domain.Invoice{Status: domain.InvoiceStatusPending}, nil); err == nil {
		t.Fatal("pending invoice rendered")
	}
}
func authorizedInvoiceFixture() domain.Invoice {
	number := int64(1)
	cae := "74123456789012"
	date := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	expiry := date.AddDate(0, 0, 10)
	invoice := domain.Invoice{Status: domain.InvoiceStatusIssued, Type: domain.InvoiceTypeA, Number: &number, PointOfSale: 1, CAE: &cae, CAEExpiresOn: &expiry, IssuedOn: date, CreatedAt: date, IssuerCUIT: "30712345678", IssuerCondition: domain.IVAConditionRegistered, Currency: "ARS", Receiver: domain.InvoiceReceiver{Name: "Obra Norte", DocType: domain.ReceiverDocCUIT, DocNumber: "20123456786", IVACondition: domain.IVAConditionRegistered}, Amounts: domain.InvoiceAmounts{Net: decimal.NewFromInt(1000), VAT: decimal.NewFromInt(210), Total: decimal.NewFromInt(1210)}, Snapshot: domain.QuoteRepresentationPayload{Supplier: domain.QuoteRepresentationSupplier{Name: "Corralón Ñandú"}, Currency: "ARS", Items: []domain.QuoteRepresentationItem{{ProductName: "Cemento", Quantity: "1.00", UnitPrice: "1000.00", Subtotal: "1000.00"}}}}
	return invoice
}

func TestInvoiceRenderer_RendersAuthorizedSnapshotWithQR(t *testing.T) {
	invoice := authorizedInvoiceFixture()
	content, err := NewInvoiceRenderer().Render(invoice, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(content, []byte("%PDF-")) || !bytes.Contains(content, []byte(domain.InvoiceQRURL(invoice))) {
		t.Fatal("missing fiscal PDF or QR verification link")
	}
}

func TestInvoiceRenderer_LongRowsRepeatFiscalHeadersAndPreserveLogo(t *testing.T) {
	for _, size := range []image.Point{{400, 80}, {100, 100}, {80, 300}} {
		inv := authorizedInvoiceFixture()
		inv.Snapshot.Supplier.Name = strings.Repeat("Corralón Ñandú ", 12)
		inv.Receiver.Address = strings.Repeat("Av. San Martín 1234, Buenos Aires. ", 7)
		inv.Snapshot.Items[0].ProductName = strings.Repeat("Cemento de albañilería de calidad superior. ", 100)
		var logoBytes bytes.Buffer
		if err := png.Encode(&logoBytes, image.NewRGBA(image.Rect(0, 0, size.X, size.Y))); err != nil {
			t.Fatal(err)
		}
		content, err := NewInvoiceRenderer().Render(inv, &domain.BrandLogo{Bytes: logoBytes.Bytes(), ContentType: "image/png"})
		if err != nil {
			t.Fatal(err)
		}
		pages := bytes.Count(content, []byte("/Type /Page\n"))
		if pages < 3 {
			t.Fatalf("long row truncated: only %d pages", pages)
		}
		if !bytes.Contains(content, []byte("/Subtype /Image")) {
			t.Fatal("logo image missing")
		}
		if bytes.Count(content, []byte(domain.InvoiceQRURL(inv))) != pages {
			t.Fatal("fiscal QR missing on a continuation page")
		}
	}
}

func TestInvoiceGrossIncome_PrintsExemptAsAWord(t *testing.T) {
	for raw, want := range map[string]string{domain.GrossIncomeExempt: "Exento", "exento": "Exento", "901-123456-7": "901-123456-7"} {
		if got := invoiceGrossIncome(raw); got != want {
			t.Errorf("invoiceGrossIncome(%q) = %q, want %q", raw, got, want)
		}
	}
}
