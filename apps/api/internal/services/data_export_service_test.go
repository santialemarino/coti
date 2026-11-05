package services

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type dataExportTestStore struct {
	filter   domain.DataExportFilter
	branches map[uuid.UUID]string
	quotes   []domain.DataExportQuote
	products []domain.DataExportProduct
	clients  []domain.DataExportClient
	invoices []domain.DataExportInvoice
	read     domain.DataExportEntity
}

func (f *dataExportTestStore) GetBranchName(
	_ context.Context, _ repository.Querier, _, branchID uuid.UUID,
) (string, error) {
	name, ok := f.branches[branchID]
	if !ok {
		return "", domain.ErrNotFound
	}
	return name, nil
}

func (f *dataExportTestStore) ListQuotes(
	_ context.Context, _ repository.Querier, filter domain.DataExportFilter,
) ([]domain.DataExportQuote, error) {
	f.filter, f.read = filter, domain.DataExportQuotes
	return f.quotes, nil
}

func (f *dataExportTestStore) ListCatalog(
	_ context.Context, _ repository.Querier, filter domain.DataExportFilter,
) ([]domain.DataExportProduct, error) {
	f.filter, f.read = filter, domain.DataExportCatalog
	return f.products, nil
}

func (f *dataExportTestStore) ListClients(
	_ context.Context, _ repository.Querier, filter domain.DataExportFilter,
) ([]domain.DataExportClient, error) {
	f.filter, f.read = filter, domain.DataExportClients
	return f.clients, nil
}

func (f *dataExportTestStore) ListInvoices(
	_ context.Context, _ repository.Querier, filter domain.DataExportFilter,
) ([]domain.DataExportInvoice, error) {
	f.filter, f.read = filter, domain.DataExportInvoices
	return f.invoices, nil
}

func newDataExportTestService(store *dataExportTestStore) *DataExportService {
	service := NewDataExportService(&sellerReportTestDB{}, store)
	service.now = func() time.Time { return time.Date(2026, 11, 6, 1, 30, 0, 0, time.UTC) }
	return service
}

func dataExportAdmin() domain.Tenant {
	return domain.Tenant{AccountID: uuid.New(), UserID: uuid.New(), Role: domain.UserRoleAdmin}
}

func TestDataExportRefusesASeller(t *testing.T) {
	service := newDataExportTestService(&dataExportTestStore{})
	tenant := dataExportAdmin()
	tenant.Role = domain.UserRoleSeller
	_, err := service.Export(context.Background(), tenant, domain.DataExportRequest{
		Entity: domain.DataExportClients, Format: domain.DataExportCSV,
	})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("err = %v, want ErrForbidden", err)
	}
}

func TestDataExportRejectsInvalidRequests(t *testing.T) {
	branchID := uuid.New()
	cases := map[string]domain.DataExportRequest{
		"entity":         {Entity: "orders", Format: "csv"},
		"format":         {Entity: "quotes", Format: "json"},
		"branch":         {Entity: "quotes", Format: "csv", BranchID: "centro"},
		"unknown branch": {Entity: "catalog", Format: "csv", BranchID: branchID.String()},
		"from":           {Entity: "quotes", Format: "csv", From: "05/11/2026"},
		"to":             {Entity: "invoices", Format: "xml", To: "2026-13-01"},
		"reversed":       {Entity: "quotes", Format: "csv", From: "2026-11-05", To: "2026-11-04"},
	}
	for name, request := range cases {
		t.Run(name, func(t *testing.T) {
			service := newDataExportTestService(&dataExportTestStore{})
			_, err := service.Export(context.Background(), dataExportAdmin(), request)
			if !errors.Is(err, domain.ErrInvalidInput) {
				t.Fatalf("err = %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestDataExportBoundsThePeriodInArgentineDays(t *testing.T) {
	branchID := uuid.New()
	store := &dataExportTestStore{branches: map[uuid.UUID]string{branchID: "Sucursal Centro"}}
	file, err := newDataExportTestService(store).Export(context.Background(), dataExportAdmin(),
		domain.DataExportRequest{
			Entity: domain.DataExportQuotes, Format: domain.DataExportCSV,
			BranchID: branchID.String(), From: "2026-11-01", To: "2026-11-05",
		})
	if err != nil {
		t.Fatal(err)
	}
	if store.filter.BranchID == nil || *store.filter.BranchID != branchID {
		t.Fatalf("branch filter = %v", store.filter.BranchID)
	}
	if got := store.filter.StartAt.Format(time.RFC3339); got != "2026-11-01T03:00:00Z" {
		t.Fatalf("start = %s", got)
	}
	if got := store.filter.EndAt.Format(time.RFC3339); got != "2026-11-06T03:00:00Z" {
		t.Fatalf("end = %s", got)
	}
	if file.Filename != "cotizaciones-sucursal-centro-2026-11-05.csv" {
		t.Fatalf("filename = %s", file.Filename)
	}
	if file.ContentType != "text/csv; charset=utf-8" {
		t.Fatalf("content type = %s", file.ContentType)
	}
}

func TestDataExportIgnoresFiltersAnEntityDoesNotTake(t *testing.T) {
	store := &dataExportTestStore{}
	_, err := newDataExportTestService(store).Export(context.Background(), dataExportAdmin(),
		domain.DataExportRequest{
			Entity: domain.DataExportClients, Format: domain.DataExportXML,
			BranchID: uuid.NewString(), From: "2026-11-01", To: "2026-11-05",
		})
	if err != nil {
		t.Fatal(err)
	}
	if store.read != domain.DataExportClients || store.filter.BranchID != nil ||
		store.filter.StartAt != nil || store.filter.EndAt != nil {
		t.Fatalf("clients read with filter %+v", store.filter)
	}

	store = &dataExportTestStore{}
	_, err = newDataExportTestService(store).Export(context.Background(), dataExportAdmin(),
		domain.DataExportRequest{Entity: domain.DataExportCatalog, Format: domain.DataExportCSV, From: "2026-11-01"})
	if err != nil {
		t.Fatal(err)
	}
	if store.filter.StartAt != nil || store.filter.FromDate != nil {
		t.Fatalf("catalog read with a period %+v", store.filter)
	}
}

func TestDataExportWritesQuotesWithLinesAndDiscounts(t *testing.T) {
	clientID := uuid.New()
	version := 2
	created := time.Date(2026, 11, 5, 13, 0, 0, 0, time.UTC)
	store := &dataExportTestStore{quotes: []domain.DataExportQuote{{
		ID: uuid.New(), Number: 42, Branch: "Centro", Status: domain.QuoteStatusAccepted,
		ClientID: &clientID, Client: strPtr("Obra Norte"), CreatedAt: created,
		VersionNumber: &version, Currency: strPtr("ARS"), ItemsSubtotal: strPtr("1500.00"),
		Total: strPtr("1350.00"),
		Items: []domain.DataExportQuoteItem{
			{RequestedDescription: "cemento", ProductCode: strPtr("CEM-1"), Quantity: "10.00",
				UnitPrice: strPtr("150.00"), Subtotal: strPtr("1500.00"), MatchStatus: domain.ItemMatchStatusMatched},
			{RequestedDescription: "ladrillo raro", Quantity: "100.00", MatchStatus: domain.ItemMatchStatusNoMatch},
		},
	}}}
	file, err := newDataExportTestService(store).Export(context.Background(), dataExportAdmin(),
		domain.DataExportRequest{Entity: domain.DataExportQuotes, Format: domain.DataExportCSV})
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimPrefix(string(file.Content), "\xEF\xBB\xBF"), "\r\n")
	if len(lines) != 4 {
		t.Fatalf("CSV has %d lines:\n%s", len(lines), file.Content)
	}
	for _, want := range []string{
		",42,Centro,Aceptada,NO,NO," + clientID.String() + ",Obra Norte,,2026-11-05T10:00:00-03:00,,,2,ARS,1500.00,150.00,1350.00,",
		"cemento,CEM-1,,10.00,,150.00,1500.00,Emparejado",
	} {
		if !strings.Contains(lines[1], want) {
			t.Fatalf("first line %q lacks %q", lines[1], want)
		}
	}
	if !strings.HasSuffix(lines[2], "ladrillo raro,,,100.00,,,,Sin emparejar") {
		t.Fatalf("unmatched line = %q", lines[2])
	}
}

func TestDataExportWritesClientsAsXML(t *testing.T) {
	iva := domain.IVAConditionRegistered
	store := &dataExportTestStore{clients: []domain.DataExportClient{{
		ID: uuid.New(), Name: strPtr("Ana Pérez"), TaxID: strPtr("20123456789"), IVACondition: &iva,
		Tags: []string{"Mayorista", "Obra"}, CreatedAt: time.Date(2026, 1, 2, 3, 0, 0, 0, time.UTC),
		Contacts: []domain.DataExportClientContact{
			{Kind: domain.ChannelTypeWhatsApp, Value: "+5491122334455", IsMain: true, Verified: true},
		},
	}}}
	file, err := newDataExportTestService(store).Export(context.Background(), dataExportAdmin(),
		domain.DataExportRequest{Entity: domain.DataExportClients, Format: domain.DataExportXML})
	if err != nil {
		t.Fatal(err)
	}
	if file.Filename != "clientes-2026-11-05.xml" || file.ContentType != "application/xml; charset=utf-8" {
		t.Fatalf("file = %s %s", file.Filename, file.ContentType)
	}
	for _, want := range []string{
		`<clientes exportado_el="2026-11-05T22:30:00-03:00">`,
		"<registrado>SI</registrado>", "<condicion_iva>Responsable inscripto</condicion_iva>",
		"<etiquetas>Mayorista; Obra</etiquetas>",
		"<contacto>\n        <tipo>WhatsApp</tipo>\n        <valor>+5491122334455</valor>\n        <principal>SI</principal>",
	} {
		if !strings.Contains(string(file.Content), want) {
			t.Fatalf("XML lacks %q:\n%s", want, file.Content)
		}
	}
}

func TestDataExportLabelsEveryEnumValue(t *testing.T) {
	for _, status := range []domain.QuoteStatus{domain.QuoteStatusDraft, domain.QuoteStatusQuoted,
		domain.QuoteStatusSent, domain.QuoteStatusChangeRequested, domain.QuoteStatusAccepted,
		domain.QuoteStatusRejected} {
		if exportQuoteStatusLabels[status] == "" {
			t.Errorf("quote status %s has no label", status)
		}
	}
	for _, rate := range []domain.VATRate{domain.VATRateZero, domain.VATRateTwoFive, domain.VATRateFive,
		domain.VATRateTenFive, domain.VATRateTwentyOne, domain.VATRateTwentySeven, domain.VATRateExempt} {
		if exportVATRateLabels[rate] == "" {
			t.Errorf("VAT rate %s has no label", rate)
		}
	}
	for _, origin := range []domain.ClientOrigin{domain.ClientOriginWhatsApp, domain.ClientOriginEmail,
		domain.ClientOriginWebApp, domain.ClientOriginPhone, domain.ClientOriginWalkIn} {
		if exportClientOriginLabels[origin] == "" {
			t.Errorf("client origin %s has no label", origin)
		}
	}
}

func TestDataExportWritesAnInvoiceNumberOnlyOnceAuthorized(t *testing.T) {
	number := int64(12)
	store := &dataExportTestStore{invoices: []domain.DataExportInvoice{
		{ID: uuid.New(), Kind: domain.VoucherKindInvoice, Type: "B", PointOfSale: 3, Number: &number,
			IssuedOn: "2026-11-05", Status: domain.InvoiceStatusIssued,
			Environment: domain.ARCAEnvironmentProduction, QuoteNumber: 42, ReceiverName: "Ana",
			ReceiverDocType: domain.ReceiverDocDNI, ReceiverIVACondition: domain.IVAConditionFinalConsumer,
			NetAmount: "100.00", ExemptAmount: "0.00", VATAmount: "21.00", Total: "121.00", Currency: "ARS"},
		{ID: uuid.New(), Kind: domain.VoucherKindInvoice, Type: "B", PointOfSale: 3,
			IssuedOn: "2026-11-05", Status: domain.InvoiceStatusRejected,
			Environment: domain.ARCAEnvironmentHomologation, QuoteNumber: 43, ReceiverName: "Ana",
			ReceiverDocType: domain.ReceiverDocNone, ReceiverIVACondition: domain.IVAConditionFinalConsumer,
			NetAmount: "100.00", ExemptAmount: "0.00", VATAmount: "21.00", Total: "121.00", Currency: "ARS"},
	}}
	file, err := newDataExportTestService(store).Export(context.Background(), dataExportAdmin(),
		domain.DataExportRequest{Entity: domain.DataExportInvoices, Format: domain.DataExportCSV})
	if err != nil {
		t.Fatal(err)
	}
	content := string(file.Content)
	if !strings.Contains(content, ",Factura,B,3,12,00003-00000012,2026-11-05,Autorizada,,,Producción,") {
		t.Fatalf("issued invoice missing:\n%s", content)
	}
	if !strings.Contains(content, ",Factura,B,3,,,2026-11-05,Rechazada,,,Prueba,") {
		t.Fatalf("rejected invoice missing:\n%s", content)
	}
}
