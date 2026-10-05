package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// invoiceTestDB runs every transaction inline; the fakes hold the state.
type invoiceTestDB struct{}

func (invoiceTestDB) InTenantTx(_ context.Context, _ domain.Tenant, fn func(repository.Querier) error) error {
	return fn(nil)
}

type fakeInvoicingStore struct {
	account     domain.AccountFiscal
	pointOfSale *int
	client      *domain.ClientFiscal
	rates       map[uuid.UUID]domain.VATRate
	credential  *domain.ARCACredentialStatus
	sealedKey   string
	branches    []domain.BranchPointOfSale
	invoice     *domain.Invoice
	released    []string
	rejected    *domain.InvoiceRejectedError
	stored      string
}

func (f *fakeInvoicingStore) GetAccountFiscal(context.Context, repository.Querier, uuid.UUID) (*domain.AccountFiscal, error) {
	account := f.account
	return &account, nil
}

func (f *fakeInvoicingStore) UpdateAccountFiscal(context.Context, repository.Querier, uuid.UUID, *domain.IVACondition, bool) error {
	return nil
}

func (f *fakeInvoicingStore) ListBranchPointsOfSale(context.Context, repository.Querier, uuid.UUID) ([]domain.BranchPointOfSale, error) {
	return f.branches, nil
}

func (f *fakeInvoicingStore) GetBranchPointOfSale(context.Context, repository.Querier, uuid.UUID, uuid.UUID) (*int, error) {
	return f.pointOfSale, nil
}

func (f *fakeInvoicingStore) UpdateBranchPointsOfSale(_ context.Context, _ repository.Querier, _ uuid.UUID, points map[uuid.UUID]*int) error {
	for i := range f.branches {
		if pos, ok := points[f.branches[i].BranchID]; ok {
			f.branches[i].PointOfSale = pos
		}
	}
	return nil
}

func (f *fakeInvoicingStore) GetClientFiscal(context.Context, repository.Querier, uuid.UUID, uuid.UUID) (*domain.ClientFiscal, error) {
	return f.client, nil
}

func (f *fakeInvoicingStore) UpdateClientFiscal(_ context.Context, _ repository.Querier, _ uuid.UUID, id uuid.UUID, in domain.ClientFiscalUpdate) (*domain.ClientFiscal, error) {
	return &domain.ClientFiscal{ID: id, LegalName: in.LegalName, TaxID: in.TaxID, IVACondition: in.IVACondition}, nil
}

func (f *fakeInvoicingStore) VATRatesByProductIDs(context.Context, repository.Querier, uuid.UUID, []uuid.UUID) (map[uuid.UUID]domain.VATRate, error) {
	return f.rates, nil
}

func (f *fakeInvoicingStore) GetCredentialStatus(context.Context, repository.Querier, uuid.UUID) (*domain.ARCACredentialStatus, error) {
	return f.credential, nil
}

func (f *fakeInvoicingStore) GetSealedCredential(context.Context, repository.Querier, uuid.UUID) (string, string, error) {
	return "CERT", f.sealedKey, nil
}

func (f *fakeInvoicingStore) UpsertCredential(_ context.Context, _ repository.Querier, _ uuid.UUID, _ string, sealedKey string, status domain.ARCACredentialStatus) (*domain.ARCACredentialStatus, error) {
	f.stored = sealedKey
	return &status, nil
}

func (f *fakeInvoicingStore) DeleteCredential(context.Context, repository.Querier, uuid.UUID) error {
	return nil
}

func (f *fakeInvoicingStore) GetLatestByQuote(context.Context, repository.Querier, uuid.UUID, uuid.UUID) (*domain.Invoice, error) {
	return f.invoice, nil
}

func (f *fakeInvoicingStore) CreatePending(_ context.Context, _ repository.Querier, in domain.Invoice) (*domain.Invoice, error) {
	if f.invoice != nil && f.invoice.Status != domain.InvoiceStatusRejected && f.invoice.QuoteVersionID == in.QuoteVersionID {
		return nil, domain.ErrConflict
	}
	in.ID = uuid.New()
	in.Status = domain.InvoiceStatusPending
	f.invoice = &in
	return &in, nil
}

func (f *fakeInvoicingStore) MarkIssued(_ context.Context, _ repository.Querier, _, _ uuid.UUID, auth domain.InvoiceAuthorization) (*domain.Invoice, error) {
	f.invoice.Status = domain.InvoiceStatusIssued
	f.invoice.Number = &auth.Number
	f.invoice.CAE = &auth.CAE
	return f.invoice, nil
}

func (f *fakeInvoicingStore) MarkRejected(_ context.Context, _ repository.Querier, _, _ uuid.UUID, rejection domain.InvoiceRejectedError) error {
	f.invoice.Status = domain.InvoiceStatusRejected
	f.rejected = &rejection
	return nil
}

func (f *fakeInvoicingStore) ReleasePending(_ context.Context, _ repository.Querier, _, _ uuid.UUID, reason string) error {
	f.invoice.Status = domain.InvoiceStatusRejected
	f.released = append(f.released, reason)
	return nil
}

type fakeInvoiceQuotes struct {
	quote   domain.Quote
	version domain.QuoteVersion
	items   []domain.QuoteItem
}

func (f *fakeInvoiceQuotes) GetByID(context.Context, repository.Querier, uuid.UUID, uuid.UUID, uuid.UUID) (*domain.Quote, error) {
	quote := f.quote
	return &quote, nil
}

func (f *fakeInvoiceQuotes) GetCurrentVersion(context.Context, repository.Querier, uuid.UUID, uuid.UUID, uuid.UUID) (*domain.QuoteVersion, error) {
	version := f.version
	return &version, nil
}

func (f *fakeInvoiceQuotes) ListItems(context.Context, repository.Querier, uuid.UUID, uuid.UUID) ([]domain.QuoteItem, error) {
	return f.items, nil
}

type fakeInvoiceDiscounts struct{}

func (fakeInvoiceDiscounts) ListByVersionID(context.Context, repository.Querier, uuid.UUID, uuid.UUID) ([]domain.QuoteDiscount, error) {
	return nil, nil
}

func (fakeInvoiceDiscounts) ListItemIDsByDiscountIDs(context.Context, repository.Querier, uuid.UUID, uuid.UUID, []uuid.UUID) (map[uuid.UUID][]uuid.UUID, error) {
	return nil, nil
}

type fakeInvoiceIssuer struct {
	auth     *domain.InvoiceAuthorization
	err      error
	latest   *domain.AuthorizedInvoice
	requests []domain.InvoiceRequest
}

func (f *fakeInvoiceIssuer) Issue(_ context.Context, _ domain.ARCACredentials, req domain.InvoiceRequest) (*domain.InvoiceAuthorization, error) {
	f.requests = append(f.requests, req)
	return f.auth, f.err
}

func (f *fakeInvoiceIssuer) LatestAuthorized(context.Context, domain.ARCACredentials, string, domain.InvoiceType, int) (*domain.AuthorizedInvoice, error) {
	return f.latest, nil
}

type fakeKeySealer struct{ enabled bool }

func (s fakeKeySealer) Enabled() bool                       { return s.enabled }
func (fakeKeySealer) Seal(plaintext string) (string, error) { return "sealed:" + plaintext, nil }
func (fakeKeySealer) Open(sealed string) (string, error)    { return sealed, nil }

var invoiceNow = time.Date(2026, 10, 6, 1, 0, 0, 0, time.UTC) // the 5th, late evening, in Argentina.

type invoiceFixture struct {
	service *InvoiceService
	store   *fakeInvoicingStore
	issuer  *fakeInvoiceIssuer
	tenant  domain.Tenant
	quoteID uuid.UUID
}

func newInvoiceFixture(t *testing.T) invoiceFixture {
	t.Helper()
	registered := domain.IVAConditionRegistered
	taxID, pos := "30-71234567-8", 3
	cuit, product := "20123456786", uuid.New()
	versionID, quoteID, branchID := uuid.New(), uuid.New(), uuid.New()
	store := &fakeInvoicingStore{
		account:     domain.AccountFiscal{TaxID: &taxID, IVACondition: &registered},
		pointOfSale: &pos,
		client:      &domain.ClientFiscal{TaxID: &cuit, IVACondition: &registered},
		rates:       map[uuid.UUID]domain.VATRate{product: domain.VATRateTwentyOne},
		credential:  &domain.ARCACredentialStatus{CUIT: "30712345678", ExpiresAt: invoiceNow.AddDate(1, 0, 0)},
		sealedKey:   "KEY",
	}
	clientID := uuid.New()
	quotes := &fakeInvoiceQuotes{
		quote: domain.Quote{ID: quoteID, BranchID: branchID, ClientID: &clientID,
			CurrentStatus: domain.QuoteStatusAccepted},
		version: domain.QuoteVersion{ID: versionID, Currency: "ARS"},
		items:   []domain.QuoteItem{invoiceLine(&product, "1000.00")},
	}
	issuer := &fakeInvoiceIssuer{auth: &domain.InvoiceAuthorization{Number: 23, CAE: "74123456789012",
		CAEExpiresOn: invoiceNow.AddDate(0, 0, 10)}}
	service := NewInvoiceService(invoiceTestDB{}, store, quotes, fakeInvoiceDiscounts{}, issuer,
		fakeKeySealer{enabled: true}, nil, InvoiceSettings{Enabled: true, RequestTimeout: time.Second,
			UnidentifiedMax: decimal.NewFromInt(10_000_000)}, func() time.Time { return invoiceNow })
	return invoiceFixture{service: service, store: store, issuer: issuer, quoteID: quoteID,
		tenant: domain.Tenant{AccountID: uuid.New(), BranchID: branchID}}
}

func TestInvoiceService_IssueAuthorizesAndRecordsTheInvoice(t *testing.T) {
	t.Parallel()
	f := newInvoiceFixture(t)

	invoice, err := f.service.Issue(context.Background(), f.tenant, f.quoteID)
	if err != nil {
		t.Fatal(err)
	}
	if invoice.Status != domain.InvoiceStatusIssued || *invoice.CAE != "74123456789012" {
		t.Fatalf("invoice = %+v, want issued with ARCA's CAE", invoice)
	}
	req := f.issuer.requests[0]
	if req.Type != domain.InvoiceTypeA || req.IssuerCUIT != "30712345678" || req.PointOfSale != 3 ||
		req.Receiver.DocType != domain.ReceiverDocCUIT {
		t.Fatalf("request = %+v, want an A invoice from the account's CUIT at its point of sale", req)
	}
	assertMoney(t, "request total", req.Amounts.Total, "1210.00")
	if want := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC); !req.Date.Equal(want) {
		t.Fatalf("date = %s, want the day in Argentina", req.Date)
	}
}

func TestInvoiceService_IssueRefusesWhatIsNotReady(t *testing.T) {
	t.Parallel()
	f := newInvoiceFixture(t)
	f.store.pointOfSale = nil

	_, err := f.service.Issue(context.Background(), f.tenant, f.quoteID)

	var notReady *domain.InvoiceNotReadyError
	if !errors.As(err, &notReady) || domain.CodeOf(err) != domain.CodeInvoiceNotReady {
		t.Fatalf("err = %v, want INVOICE_NOT_READY", err)
	}
	if len(f.issuer.requests) != 0 {
		t.Fatal("ARCA was asked for an invoice that was not ready")
	}
}

func TestInvoiceService_IssueRefusesASecondInvoice(t *testing.T) {
	t.Parallel()
	f := newInvoiceFixture(t)
	if _, err := f.service.Issue(context.Background(), f.tenant, f.quoteID); err != nil {
		t.Fatal(err)
	}

	_, err := f.service.Issue(context.Background(), f.tenant, f.quoteID)

	if domain.CodeOf(err) != domain.CodeQuoteAlreadyInvoiced || len(f.issuer.requests) != 1 {
		t.Fatalf("err = %v after %d requests, want QUOTE_ALREADY_INVOICED and one request", err, len(f.issuer.requests))
	}
}

func TestInvoiceService_IssueWaitsForAnInvoiceInFlight(t *testing.T) {
	t.Parallel()
	f := newInvoiceFixture(t)
	f.store.invoice = &domain.Invoice{Status: domain.InvoiceStatusPending,
		QuoteVersionID: newVersionID(f), UpdatedAt: invoiceNow}

	_, err := f.service.Issue(context.Background(), f.tenant, f.quoteID)

	if domain.CodeOf(err) != domain.CodeInvoiceInProgress || len(f.issuer.requests) != 0 {
		t.Fatalf("err = %v, want INVOICE_IN_PROGRESS without asking ARCA", err)
	}
}

func TestInvoiceService_RejectionFreesTheQuoteForAnotherTry(t *testing.T) {
	t.Parallel()
	f := newInvoiceFixture(t)
	f.issuer.err = &domain.InvoiceRejectedError{Issues: []string{"10013: DocTipo invalido"}}

	_, err := f.service.Issue(context.Background(), f.tenant, f.quoteID)

	if domain.CodeOf(err) != domain.CodeInvoiceRejected {
		t.Fatalf("err = %v, want INVOICE_REJECTED", err)
	}
	if f.store.invoice.Status != domain.InvoiceStatusRejected || f.store.rejected == nil {
		t.Fatalf("invoice = %+v, want it recorded as rejected", f.store.invoice)
	}
}

func TestInvoiceService_UnavailableReleasesTheHold(t *testing.T) {
	t.Parallel()
	f := newInvoiceFixture(t)
	f.issuer.err = domain.ErrInvoicingUnavailable

	_, err := f.service.Issue(context.Background(), f.tenant, f.quoteID)

	if !errors.Is(err, domain.ErrInvoicingUnavailable) || len(f.store.released) != 1 {
		t.Fatalf("err = %v, released = %v, want unavailable and the hold released", err, f.store.released)
	}
}

// Releasing an invoice ARCA may have authorized would let the next press authorize a second one.
func TestInvoiceService_UnknownOutcomeKeepsTheHold(t *testing.T) {
	t.Parallel()
	f := newInvoiceFixture(t)
	f.issuer.err = domain.ErrInvoiceOutcomeUnknown

	_, err := f.service.Issue(context.Background(), f.tenant, f.quoteID)

	if domain.CodeOf(err) != domain.CodeInvoiceInProgress {
		t.Fatalf("err = %v, want INVOICE_IN_PROGRESS", err)
	}
	if f.store.invoice.Status != domain.InvoiceStatusPending || len(f.store.released) != 0 {
		t.Fatalf("invoice = %+v, want it still held", f.store.invoice)
	}
}

func TestInvoiceService_StalePendingIsAdoptedWhenARCAHasIt(t *testing.T) {
	t.Parallel()
	f := newInvoiceFixture(t)
	issuedOn := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	f.store.invoice = &domain.Invoice{ID: uuid.New(), Status: domain.InvoiceStatusPending,
		QuoteVersionID: newVersionID(f), UpdatedAt: invoiceNow.Add(-time.Hour), IssuedOn: issuedOn,
		Receiver: domain.InvoiceReceiver{DocType: domain.ReceiverDocCUIT, DocNumber: "20123456786"},
		Amounts:  domain.InvoiceAmounts{Total: dec("1210.00")}}
	f.issuer.latest = &domain.AuthorizedInvoice{Number: 7, CAE: "74000000000007", Date: issuedOn,
		DocType: domain.ReceiverDocCUIT, DocNumber: "20123456786", Total: dec("1210.00")}

	invoice, err := f.service.Issue(context.Background(), f.tenant, f.quoteID)

	if err != nil || invoice.Status != domain.InvoiceStatusIssued || *invoice.Number != 7 {
		t.Fatalf("invoice = %+v, err = %v, want ARCA's invoice 7 adopted", invoice, err)
	}
	if len(f.issuer.requests) != 0 {
		t.Fatal("a second invoice was requested for a sale ARCA had already invoiced")
	}
}

func TestInvoiceService_StalePendingIsReleasedWhenARCADoesNotHaveIt(t *testing.T) {
	t.Parallel()
	f := newInvoiceFixture(t)
	f.store.invoice = &domain.Invoice{ID: uuid.New(), Status: domain.InvoiceStatusPending,
		QuoteVersionID: newVersionID(f), UpdatedAt: invoiceNow.Add(-time.Hour),
		Amounts: domain.InvoiceAmounts{Total: dec("1210.00")}}
	f.issuer.latest = &domain.AuthorizedInvoice{Number: 6, Total: dec("99.00")}

	invoice, err := f.service.Issue(context.Background(), f.tenant, f.quoteID)

	if err != nil || len(f.store.released) != 1 || len(f.issuer.requests) != 1 || *invoice.Number != 23 {
		t.Fatalf("invoice = %+v, err = %v, released = %v, want the hold released and a fresh invoice",
			invoice, err, f.store.released)
	}
}

func TestInvoiceService_UpdateClientFiscalChecksTheCUIT(t *testing.T) {
	t.Parallel()
	f := newInvoiceFixture(t)
	valid, invalid, dni := "20-12345678-6", "20-12345678-5", "28.123.456"

	client, err := f.service.UpdateClientFiscal(context.Background(), f.tenant, uuid.New(),
		domain.ClientFiscalUpdate{TaxID: &valid})
	if err != nil || *client.TaxID != "20123456786" {
		t.Fatalf("client = %+v, err = %v, want the CUIT stored as digits", client, err)
	}
	if _, err := f.service.UpdateClientFiscal(context.Background(), f.tenant, uuid.New(),
		domain.ClientFiscalUpdate{TaxID: &invalid}); domain.CodeOf(err) != domain.CodeInvalidTaxID {
		t.Fatalf("err = %v, want INVALID_TAX_ID for a wrong check digit", err)
	}
	if client, err := f.service.UpdateClientFiscal(context.Background(), f.tenant, uuid.New(),
		domain.ClientFiscalUpdate{TaxID: &dni}); err != nil || *client.TaxID != "28123456" {
		t.Fatalf("client = %+v, err = %v, want the DNI stored as digits", client, err)
	}
}

func TestInvoiceService_UpdateSettingsRefusesASharedPointOfSale(t *testing.T) {
	t.Parallel()
	f := newInvoiceFixture(t)
	first, second := uuid.New(), uuid.New()
	f.store.branches = []domain.BranchPointOfSale{{BranchID: first}, {BranchID: second}}
	pos := 4

	_, err := f.service.UpdateSettings(context.Background(), f.tenant, domain.InvoicingSettingsUpdate{
		PointsOfSale: map[uuid.UUID]*int{first: &pos, second: &pos}})

	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("err = %v, want two branches on one point of sale refused", err)
	}
}

func TestInvoiceService_UploadCredentialsSealsTheKey(t *testing.T) {
	t.Parallel()
	f := newInvoiceFixture(t)
	f.service.parse = func([]byte, []byte) (*domain.ARCACredentialStatus, error) {
		return &domain.ARCACredentialStatus{CUIT: "30712345678"}, nil
	}

	if _, err := f.service.UploadCredentials(context.Background(), f.tenant, []byte("CERT"), []byte("KEY")); err != nil {
		t.Fatal(err)
	}
	if f.store.stored != "sealed:KEY" {
		t.Fatalf("stored key = %q, want it sealed", f.store.stored)
	}

	f.service.sealer = fakeKeySealer{enabled: false}
	if _, err := f.service.UploadCredentials(context.Background(), f.tenant, []byte("CERT"),
		[]byte("KEY")); !errors.Is(err, domain.ErrNotConfigured) {
		t.Fatalf("err = %v, want NOT_CONFIGURED without an encryption key", err)
	}
}

func newVersionID(f invoiceFixture) uuid.UUID {
	quotes := f.service.quotes.(*fakeInvoiceQuotes)
	return quotes.version.ID
}
