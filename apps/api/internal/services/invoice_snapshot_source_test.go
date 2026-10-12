package services

import (
	"context"
	"errors"
	"testing"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/shopspring/decimal"
)

func invoiceFixtureWithoutDocument(t *testing.T) invoiceFixture {
	t.Helper()
	f := newInvoiceFixture(t)
	name, unit, color, logo := "Portland cement", "bag", "#BC0CDF", "/v1/public/account-logos/"+f.tenant.AccountID.String()+"/logo"
	item := f.quotes.items[0]
	item.ProductName = &name
	item.Unit = &unit
	item.MatchStatus = domain.ItemMatchStatusMatched
	item.Quantity = dec("1")
	item.UnitPriceSnapshot = decimal.NewNullDecimal(dec("1000"))
	f.quotes.items = []domain.QuoteItem{item}
	f.store.snapshotErr = domain.ErrNotFound
	f.store.snapshotSource = &domain.QuoteRepresentationSource{Account: domain.Account{Name: "Supplier", BrandColor: &color, BrandLogoURL: &logo}, Branch: domain.Branch{Name: "Main branch"}, Quote: f.quotes.quote, Version: f.quotes.version, Items: f.quotes.items}
	return f
}

func TestInvoiceService_IssueWithoutCommercialDocumentBuildsFiscalSnapshot(t *testing.T) {
	f := invoiceFixtureWithoutDocument(t)
	inv, err := f.issue(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if inv.Status != domain.InvoiceStatusIssued || len(f.issuer.requests) != 1 {
		t.Fatal("invoice was not authorized exactly once")
	}
	snapshot := inv.Snapshot
	if len(snapshot.Items) != 1 || snapshot.Items[0].ProductName != "Portland cement" || snapshot.Items[0].Quantity != "1.00" || snapshot.Items[0].UnitPrice != "1000.00" || snapshot.Total != "1210.00" {
		t.Fatalf("incomplete fiscal snapshot: %+v", snapshot)
	}
	if snapshot.Supplier.BrandColor != "#BC0CDF" || snapshot.Supplier.LogoURL == nil || snapshot.Branch.Name != "Main branch" || snapshot.Customer.Address == nil {
		t.Fatal("branding, branch or fiscal address missing")
	}
	if snapshot.Supplier.TaxID == nil || *snapshot.Supplier.TaxID != f.store.credential.CUIT {
		t.Fatal("issuer did not use ARCA credentials")
	}
}

func TestInvoiceService_SnapshotFailuresStopBeforeAuthorization(t *testing.T) {
	for _, test := range []struct {
		name      string
		configure func(invoiceFixture)
	}{
		{"document read error", func(f invoiceFixture) { f.store.snapshotErr = errors.New("storage unavailable") }},
		{"source read error", func(f invoiceFixture) { f.store.sourceErr = errors.New("source unavailable") }},
		{"invalid line", func(f invoiceFixture) { f.store.snapshotSource.Items[0].ProductName = nil }},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := invoiceFixtureWithoutDocument(t)
			test.configure(f)
			if _, err := f.issue(context.Background()); err == nil {
				t.Fatal("invalid snapshot accepted")
			}
			if len(f.issuer.requests) != 0 || f.store.invoice != nil {
				t.Fatal("failure reserved or authorized an invoice")
			}
		})
	}
}
