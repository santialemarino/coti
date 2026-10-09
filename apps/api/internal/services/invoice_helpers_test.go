package services

import (
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func dec(value string) decimal.Decimal { return decimal.RequireFromString(value) }

func invoiceLine(product *uuid.UUID, subtotal string) domain.QuoteItem {
	return domain.QuoteItem{ID: uuid.New(), ProductID: product,
		Subtotal: decimal.NewNullDecimal(dec(subtotal))}
}

func assertMoney(t *testing.T, label string, got decimal.Decimal, want string) {
	t.Helper()
	if !got.Equal(dec(want)) {
		t.Fatalf("%s = %s, want %s", label, got, want)
	}
}

func TestInvoiceAmounts_PricesWithoutVATAddItOnTopPerRate(t *testing.T) {
	t.Parallel()
	cement, sand := uuid.New(), uuid.New()
	items := []domain.QuoteItem{invoiceLine(&cement, "1000.00"), invoiceLine(&sand, "2000.00")}
	rates := map[uuid.UUID]domain.VATRate{cement: domain.VATRateTwentyOne, sand: domain.VATRateTenFive}

	got := invoiceAmounts(grossByRate(items, rates, nil), domain.InvoiceTypeA, false)

	assertMoney(t, "net", got.Net, "3000.00")
	assertMoney(t, "vat", got.VAT, "420.00")
	assertMoney(t, "total", got.Total, "3420.00")
	if len(got.ByRate) != 2 || got.ByRate[0].Rate != domain.VATRateTwentyOne ||
		got.ByRate[1].Rate != domain.VATRateTenFive {
		t.Fatalf("breakdown = %+v, want 21%% then 10.5%%", got.ByRate)
	}
	assertMoney(t, "21% vat", got.ByRate[0].Amount, "210.00")
	assertMoney(t, "10.5% vat", got.ByRate[1].Amount, "210.00")
}

func TestInvoiceAmounts_PricesWithVATAreDividedBackOut(t *testing.T) {
	t.Parallel()
	cement := uuid.New()
	items := []domain.QuoteItem{invoiceLine(&cement, "1210.00")}

	got := invoiceAmounts(grossByRate(items, map[uuid.UUID]domain.VATRate{cement: domain.VATRateTwentyOne}, nil),
		domain.InvoiceTypeB, true)

	assertMoney(t, "net", got.Net, "1000.00")
	assertMoney(t, "vat", got.VAT, "210.00")
	assertMoney(t, "total", got.Total, "1210.00")
}

// A total discount lowers every line's base in proportion, so each rate pays IVA on what it was
// actually sold for.
func TestInvoiceAmounts_TotalDiscountIsSpreadAcrossRates(t *testing.T) {
	t.Parallel()
	cement, sand := uuid.New(), uuid.New()
	items := []domain.QuoteItem{invoiceLine(&cement, "1000.00"), invoiceLine(&sand, "2000.00")}
	rates := map[uuid.UUID]domain.VATRate{cement: domain.VATRateTwentyOne, sand: domain.VATRateTenFive}
	discounts := []domain.QuoteDiscount{{Scope: domain.DiscountScopeTotal, Amount: dec("300.00")}}

	got := invoiceAmounts(grossByRate(items, rates, discounts), domain.InvoiceTypeA, false)

	assertMoney(t, "21% base", got.ByRate[0].Base, "900.00")
	assertMoney(t, "10.5% base", got.ByRate[1].Base, "1800.00")
	assertMoney(t, "vat", got.VAT, "378.00")
	assertMoney(t, "total", got.Total, "3078.00")
}

func TestInvoiceAmounts_ItemDiscountOnlyTouchesItsLines(t *testing.T) {
	t.Parallel()
	cement, sand := uuid.New(), uuid.New()
	first, second := invoiceLine(&cement, "1000.00"), invoiceLine(&sand, "2000.00")
	rates := map[uuid.UUID]domain.VATRate{cement: domain.VATRateTwentyOne, sand: domain.VATRateTenFive}
	discounts := []domain.QuoteDiscount{
		{Scope: domain.DiscountScopeItem, Amount: dec("100.00"), ItemIDs: []uuid.UUID{first.ID}},
		{Scope: domain.DiscountScopeTotal, Amount: dec("999.00"), SuppressedBySeller: true},
	}

	got := invoiceAmounts(grossByRate([]domain.QuoteItem{first, second}, rates, discounts),
		domain.InvoiceTypeA, false)

	assertMoney(t, "21% base", got.ByRate[0].Base, "900.00")
	assertMoney(t, "10.5% base", got.ByRate[1].Base, "2000.00")
}

// The shares are rounded to cents, and the last line takes the leftover cent.
func TestInvoiceAmounts_DiscountSharesAddUpToTheDiscount(t *testing.T) {
	t.Parallel()
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	items := []domain.QuoteItem{invoiceLine(&a, "100.00"), invoiceLine(&b, "100.00"), invoiceLine(&c, "100.00")}
	discounts := []domain.QuoteDiscount{{Scope: domain.DiscountScopeTotal, Amount: dec("100.00")}}

	got := grossByRate(items, map[uuid.UUID]domain.VATRate{}, discounts)

	assertMoney(t, "21% gross", got[domain.VATRateTwentyOne], "200.00")
}

func TestInvoiceAmounts_LinesWithoutPriceOrProduct(t *testing.T) {
	t.Parallel()
	unmatched := domain.QuoteItem{ID: uuid.New()}
	loose := invoiceLine(nil, "500.00")

	got := grossByRate([]domain.QuoteItem{unmatched, loose}, nil, nil)

	if len(got) != 1 {
		t.Fatalf("rates = %v, want only the default rate", got)
	}
	assertMoney(t, "21% gross", got[domain.DefaultVATRate], "500.00")
}

func TestInvoiceAmounts_ExemptGoesApartFromTheTaxedBase(t *testing.T) {
	t.Parallel()
	taxed, exempt := uuid.New(), uuid.New()
	items := []domain.QuoteItem{invoiceLine(&taxed, "1000.00"), invoiceLine(&exempt, "300.00")}
	rates := map[uuid.UUID]domain.VATRate{taxed: domain.VATRateTwentyOne, exempt: domain.VATRateExempt}

	got := invoiceAmounts(grossByRate(items, rates, nil), domain.InvoiceTypeB, false)

	assertMoney(t, "net", got.Net, "1000.00")
	assertMoney(t, "exempt", got.Exempt, "300.00")
	assertMoney(t, "total", got.Total, "1510.00")
	if len(got.ByRate) != 1 {
		t.Fatalf("breakdown = %+v, want the exempt amount left out", got.ByRate)
	}
}

func TestInvoiceAmounts_CInvoiceBreaksNothingOut(t *testing.T) {
	t.Parallel()
	cement, sand := uuid.New(), uuid.New()
	items := []domain.QuoteItem{invoiceLine(&cement, "1000.00"), invoiceLine(&sand, "2000.00")}
	rates := map[uuid.UUID]domain.VATRate{cement: domain.VATRateTwentyOne, sand: domain.VATRateTenFive}

	got := invoiceAmounts(grossByRate(items, rates, nil), domain.InvoiceTypeC, false)

	assertMoney(t, "net", got.Net, "3000.00")
	assertMoney(t, "vat", got.VAT, "0")
	assertMoney(t, "total", got.Total, "3000.00")
	if len(got.ByRate) != 0 {
		t.Fatalf("breakdown = %+v, want none on a C invoice", got.ByRate)
	}
}

func TestInvoiceTypeFor(t *testing.T) {
	t.Parallel()
	cases := []struct {
		issuer, receiver domain.IVACondition
		want             domain.InvoiceType
	}{
		{domain.IVAConditionRegistered, domain.IVAConditionRegistered, domain.InvoiceTypeA},
		{domain.IVAConditionRegistered, domain.IVAConditionMonotributo, domain.InvoiceTypeA},
		{domain.IVAConditionRegistered, domain.IVAConditionFinalConsumer, domain.InvoiceTypeB},
		{domain.IVAConditionRegistered, domain.IVAConditionExempt, domain.InvoiceTypeB},
		{domain.IVAConditionMonotributo, domain.IVAConditionRegistered, domain.InvoiceTypeC},
		{domain.IVAConditionExempt, domain.IVAConditionFinalConsumer, domain.InvoiceTypeC},
	}
	for _, tc := range cases {
		if got := invoiceTypeFor(tc.issuer, tc.receiver); got != tc.want {
			t.Errorf("invoiceTypeFor(%s, %s) = %s, want %s", tc.issuer, tc.receiver, got, tc.want)
		}
	}
}

func TestInvoiceReceiver_DocumentFollowsTheTaxIDLength(t *testing.T) {
	t.Parallel()
	registered := domain.IVAConditionRegistered
	cuit, dni := "30712345678", "28123456"
	name, legal := "Obra Norte", "Constructora Norte SA"

	company := invoiceReceiver(&domain.ClientFiscal{Name: &name, LegalName: &legal, TaxID: &cuit,
		IVACondition: &registered}, "")
	if company.DocType != domain.ReceiverDocCUIT || company.DocNumber != cuit ||
		company.Name != legal || company.IVACondition != registered {
		t.Fatalf("company receiver = %+v", company)
	}

	person := invoiceReceiver(&domain.ClientFiscal{Name: &name, TaxID: &dni}, "")
	if person.DocType != domain.ReceiverDocDNI || person.IVACondition != domain.IVAConditionFinalConsumer {
		t.Fatalf("person receiver = %+v", person)
	}

	walkIn := invoiceReceiver(nil, "Mostrador")
	if walkIn.DocType != domain.ReceiverDocNone || walkIn.Name != "Mostrador" {
		t.Fatalf("walk-in receiver = %+v", walkIn)
	}
}

func readyInputs() invoiceInputs {
	registered := domain.IVAConditionRegistered
	taxID, pos := "30-71234567-8", 3
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	return invoiceInputs{
		enabled:     true,
		quote:       domain.Quote{CurrentStatus: domain.QuoteStatusAccepted},
		version:     domain.QuoteVersion{Currency: "ARS"},
		account:     domain.AccountFiscal{TaxID: &taxID, IVACondition: &registered},
		pointOfSale: &pos,
		credential: &domain.ARCACredentialStatus{CUIT: "30712345678",
			ExpiresAt: now.AddDate(1, 0, 0)},
		receiver:        domain.InvoiceReceiver{DocType: domain.ReceiverDocNone},
		amounts:         domain.InvoiceAmounts{Total: dec("1000.00")},
		invoiceType:     domain.InvoiceTypeB,
		unidentifiedMax: dec("10000000"),
		now:             now,
	}
}

func TestInvoiceIssues_ReadyQuoteHasNone(t *testing.T) {
	t.Parallel()
	if issues := invoiceIssues(readyInputs()); len(issues) != 0 {
		t.Fatalf("issues = %v, want none", issues)
	}
}

func TestInvoiceIssues_ReportsEveryGapAtOnce(t *testing.T) {
	t.Parallel()
	in := readyInputs()
	in.enabled = false
	in.quote.CurrentStatus = domain.QuoteStatusSent
	in.account = domain.AccountFiscal{}
	in.pointOfSale = nil
	in.credential = nil

	got := invoiceIssues(in)
	for _, want := range []string{invoiceIssueDisabled, invoiceIssueNotAccepted, invoiceIssueIVACondition,
		invoiceIssueTaxID, invoiceIssuePointOfSale, invoiceIssueCredentials} {
		if !slices.Contains(got, want) {
			t.Errorf("issues = %v, missing %s", got, want)
		}
	}
}

func TestInvoiceIssues_ReceiverRules(t *testing.T) {
	t.Parallel()
	aWithoutCUIT := readyInputs()
	aWithoutCUIT.invoiceType = domain.InvoiceTypeA
	aWithoutCUIT.receiver = domain.InvoiceReceiver{DocType: domain.ReceiverDocDNI, DocNumber: "28123456"}
	if got := invoiceIssues(aWithoutCUIT); !slices.Contains(got, invoiceIssueReceiverCUIT) {
		t.Errorf("A to a DNI: issues = %v, want %s", got, invoiceIssueReceiverCUIT)
	}

	large := readyInputs()
	large.amounts.Total = dec("10000000.00")
	if got := invoiceIssues(large); !slices.Contains(got, invoiceIssueReceiverID) {
		t.Errorf("unidentified at the threshold: issues = %v, want %s", got, invoiceIssueReceiverID)
	}
}

func TestInvoiceIssues_CredentialChecks(t *testing.T) {
	t.Parallel()
	expired := readyInputs()
	expired.credential.ExpiresAt = expired.now
	if got := invoiceIssues(expired); !slices.Contains(got, invoiceIssueCredentialsExpired) {
		t.Errorf("expired certificate: issues = %v", got)
	}

	other := readyInputs()
	other.credential.CUIT = "20111111112"
	if got := invoiceIssues(other); !slices.Contains(got, invoiceIssueCredentialsCUIT) {
		t.Errorf("certificate for another CUIT: issues = %v", got)
	}
}
