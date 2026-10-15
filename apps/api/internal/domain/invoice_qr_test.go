package domain

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func TestInvoiceQRURL_EncodesTheIssuedInvoice(t *testing.T) {
	t.Parallel()
	number, cae := int64(23), "74123456789012"
	inv := Invoice{
		Status: InvoiceStatusIssued, Type: InvoiceTypeA, PointOfSale: 3, Number: &number, CAE: &cae,
		IssuedOn: time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC), IssuerCUIT: "30712345678",
		Receiver: InvoiceReceiver{DocType: ReceiverDocCUIT, DocNumber: "20123456786"},
		Amounts:  InvoiceAmounts{Total: decimal.RequireFromString("3420")},
	}

	url := InvoiceQRURL(inv)
	encoded, found := strings.CutPrefix(url, "https://www.arca.gob.ar/fe/qr/?p=")
	if !found {
		t.Fatalf("url = %q, want ARCA's QR base", url)
	}
	body, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"ver":1,"fecha":"2026-10-05","cuit":30712345678,"ptoVta":3,"tipoCmp":1,"nroCmp":23,` +
		`"importe":3420.00,"moneda":"PES","ctz":1,"tipoDocRec":80,"nroDocRec":20123456786,` +
		`"tipoCodAut":"E","codAut":74123456789012}`
	if string(body) != want {
		t.Fatalf("payload =\n%s\nwant\n%s", body, want)
	}
}

func TestInvoiceQRURL_NoneWithoutAuthorization(t *testing.T) {
	t.Parallel()
	if url := InvoiceQRURL(Invoice{Status: InvoiceStatusPending}); url != "" {
		t.Fatalf("url = %q, want none for a pending invoice", url)
	}
}

func TestInvoiceQRURL_OmitsAnUnidentifiedReceiver(t *testing.T) {
	t.Parallel()
	number, cae := int64(1), "74123456789012"
	inv := Invoice{Status: InvoiceStatusIssued, Type: InvoiceTypeB, Number: &number, CAE: &cae,
		IssuerCUIT: "30712345678", Receiver: InvoiceReceiver{DocType: ReceiverDocNone},
		Amounts: InvoiceAmounts{Total: decimal.RequireFromString("100")}}

	encoded, _ := strings.CutPrefix(InvoiceQRURL(inv), "https://www.arca.gob.ar/fe/qr/?p=")
	body, _ := base64.StdEncoding.DecodeString(encoded)
	if strings.Contains(string(body), "tipoDocRec") {
		t.Fatalf("payload = %s, want no receiver document", body)
	}
}

// A credit note's QR names its own voucher type, which ARCA verifies it under.
func TestInvoiceQRURL_NamesTheCreditNoteType(t *testing.T) {
	t.Parallel()
	number, cae := int64(4), "74123456789012"
	inv := Invoice{Status: InvoiceStatusIssued, Kind: VoucherKindCreditNote, Type: InvoiceTypeB, Number: &number, CAE: &cae,
		IssuerCUIT: "30712345678", Receiver: InvoiceReceiver{DocType: ReceiverDocNone},
		Amounts: InvoiceAmounts{Total: decimal.RequireFromString("100")}}

	encoded, _ := strings.CutPrefix(InvoiceQRURL(inv), "https://www.arca.gob.ar/fe/qr/?p=")
	body, _ := base64.StdEncoding.DecodeString(encoded)
	if !strings.Contains(string(body), `"tipoCmp":8,`) {
		t.Fatalf("payload = %s, want tipoCmp 8 for a credit note B", body)
	}
}

func TestVoucherType_ARCACode(t *testing.T) {
	t.Parallel()
	for letter, want := range map[InvoiceType][2]int{InvoiceTypeA: {1, 3}, InvoiceTypeB: {6, 8}, InvoiceTypeC: {11, 13}} {
		invoice := VoucherType{Kind: VoucherKindInvoice, Letter: letter}.ARCACode()
		note := VoucherType{Kind: VoucherKindCreditNote, Letter: letter}.ARCACode()
		if invoice != want[0] || note != want[1] {
			t.Errorf("%s: invoice %d, credit note %d, want %v", letter, invoice, note, want)
		}
	}
}
