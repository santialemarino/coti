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
	ticket      *domain.SealedARCATicket
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
	// Mirrors uq_branch_point_of_sale.
	seen := map[int]bool{}
	for _, b := range f.branches {
		if b.PointOfSale != nil && seen[*b.PointOfSale] {
			return domain.ErrConflict
		}
		if b.PointOfSale != nil {
			seen[*b.PointOfSale] = true
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
	f.invoice.Issues = rejection.Issues
	f.rejected = &rejection
	return nil
}

func (f *fakeInvoicingStore) ReleasePending(_ context.Context, _ repository.Querier, _, _ uuid.UUID, reason string) error {
	f.invoice.Status = domain.InvoiceStatusRejected
	f.invoice.Issues = []string{reason}
	f.released = append(f.released, reason)
	return nil
}

func (f *fakeInvoicingStore) ClaimNumber(_ context.Context, _ repository.Querier, _, _ uuid.UUID, number int64) error {
	f.invoice.ClaimedNumber = &number
	return nil
}

func (f *fakeInvoicingStore) GetSealedTicket(context.Context, repository.Querier, uuid.UUID) (*domain.SealedARCATicket, error) {
	return f.ticket, nil
}

func (f *fakeInvoicingStore) SaveSealedTicket(_ context.Context, _ repository.Querier, _ uuid.UUID, ticket *domain.SealedARCATicket) error {
	f.ticket = ticket
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
	auth      *domain.InvoiceAuthorization
	err       error
	found     *domain.AuthorizedInvoice
	requests  []domain.InvoiceRequest
	consulted []int64
	ctxErr    error // the context's state when Issue was called.
	creds     domain.ARCACredentials
}

func (f *fakeInvoiceIssuer) Issue(ctx context.Context, creds domain.ARCACredentials, req domain.InvoiceRequest) (*domain.InvoiceAuthorization, error) {
	f.requests = append(f.requests, req)
	f.ctxErr, f.creds = ctx.Err(), creds
	if err := req.Claim(ctx, 42); err != nil {
		return nil, err
	}
	return f.auth, f.err
}

func (f *fakeInvoiceIssuer) Authorized(_ context.Context, _ domain.ARCACredentials, _ string, _ domain.InvoiceType, _ int, number int64) (*domain.AuthorizedInvoice, error) {
	f.consulted = append(f.consulted, number)
	return f.found, nil
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
	quotes  *fakeInvoiceQuotes
}

// issue confirms the invoice the fixture previews: an A for 1210.00 on the current version.
func (f invoiceFixture) issue(ctx context.Context) (*domain.Invoice, error) {
	return f.service.Issue(ctx, f.tenant, f.quoteID, domain.InvoiceExpectation{
		VersionID: f.quotes.version.ID, Type: domain.InvoiceTypeA, Total: dec("1210.00")})
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
		version: domain.QuoteVersion{ID: versionID, Currency: "ARS", Total: dec("1000.00")},
		items:   []domain.QuoteItem{invoiceLine(&product, "1000.00")},
	}
	issuer := &fakeInvoiceIssuer{auth: &domain.InvoiceAuthorization{Number: 23, CAE: "74123456789012",
		CAEExpiresOn: invoiceNow.AddDate(0, 0, 10)}}
	service := NewInvoiceService(invoiceTestDB{}, store, quotes, fakeInvoiceDiscounts{}, issuer,
		fakeKeySealer{enabled: true}, nil, InvoiceSettings{Enabled: true, IssueTimeout: time.Second,
			ReconcileAfter: time.Minute, UnidentifiedMax: decimal.NewFromInt(10_000_000)},
		func() time.Time { return invoiceNow })
	return invoiceFixture{service: service, store: store, issuer: issuer, quoteID: quoteID, quotes: quotes,
		tenant: domain.Tenant{AccountID: uuid.New(), BranchID: branchID}}
}

func TestInvoiceService_IssueAuthorizesAndRecordsTheInvoice(t *testing.T) {
	t.Parallel()
	f := newInvoiceFixture(t)

	invoice, err := f.issue(context.Background())
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

	_, err := f.issue(context.Background())

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
	if _, err := f.issue(context.Background()); err != nil {
		t.Fatal(err)
	}

	_, err := f.issue(context.Background())

	if domain.CodeOf(err) != domain.CodeQuoteAlreadyInvoiced || len(f.issuer.requests) != 1 {
		t.Fatalf("err = %v after %d requests, want QUOTE_ALREADY_INVOICED and one request", err, len(f.issuer.requests))
	}
}

func TestInvoiceService_IssueWaitsForAnInvoiceInFlight(t *testing.T) {
	t.Parallel()
	f := newInvoiceFixture(t)
	f.store.invoice = &domain.Invoice{Status: domain.InvoiceStatusPending,
		QuoteVersionID: newVersionID(f), UpdatedAt: invoiceNow}

	_, err := f.issue(context.Background())

	if domain.CodeOf(err) != domain.CodeInvoiceInProgress || len(f.issuer.requests) != 0 {
		t.Fatalf("err = %v, want INVOICE_IN_PROGRESS without asking ARCA", err)
	}
}

func TestInvoiceService_RejectionFreesTheQuoteForAnotherTry(t *testing.T) {
	t.Parallel()
	f := newInvoiceFixture(t)
	f.issuer.err = &domain.InvoiceRejectedError{Issues: []string{"10013: DocTipo invalido"}}

	_, err := f.issue(context.Background())

	if domain.CodeOf(err) != domain.CodeInvoiceRejected {
		t.Fatalf("err = %v, want INVOICE_REJECTED", err)
	}
	if f.store.invoice.Status != domain.InvoiceStatusRejected || f.store.rejected == nil {
		t.Fatalf("invoice = %+v, want it recorded as rejected", f.store.invoice)
	}
	preview, err := f.service.Preview(context.Background(), f.tenant, f.quoteID)
	if err != nil || preview.Invoice == nil || preview.Invoice.Issues[0] != "10013: DocTipo invalido" {
		t.Fatalf("preview = %+v, %v, want the rejection shown", preview, err)
	}
}

func TestInvoiceService_UnavailableReleasesTheHold(t *testing.T) {
	t.Parallel()
	f := newInvoiceFixture(t)
	f.issuer.err = domain.ErrInvoicingUnavailable

	_, err := f.issue(context.Background())

	if !errors.Is(err, domain.ErrInvoicingUnavailable) || len(f.store.released) != 1 {
		t.Fatalf("err = %v, released = %v, want unavailable and the hold released", err, f.store.released)
	}
	// ARCA never answered, so there is no verdict to show beside the next try.
	preview, err := f.service.Preview(context.Background(), f.tenant, f.quoteID)
	if err != nil || preview.Invoice != nil {
		t.Fatalf("preview = %+v, %v, want no invoice shown", preview, err)
	}
}

// Releasing an invoice ARCA may have authorized would let the next press authorize a second one.
func TestInvoiceService_UnknownOutcomeKeepsTheHold(t *testing.T) {
	t.Parallel()
	f := newInvoiceFixture(t)
	f.issuer.err = domain.ErrInvoiceOutcomeUnknown

	_, err := f.issue(context.Background())

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
	claimed := int64(7)
	f.store.invoice = &domain.Invoice{ID: uuid.New(), Status: domain.InvoiceStatusPending,
		QuoteVersionID: newVersionID(f), UpdatedAt: invoiceNow.Add(-time.Hour), ClaimedNumber: &claimed,
		Receiver: domain.InvoiceReceiver{DocType: domain.ReceiverDocCUIT, DocNumber: "20123456786"},
		Amounts:  domain.InvoiceAmounts{Total: dec("1210.00")}}
	f.issuer.found = &domain.AuthorizedInvoice{Number: 7, CAE: "74000000000007",
		DocType: domain.ReceiverDocCUIT, DocNumber: "20123456786", Total: dec("1210.00")}

	invoice, err := f.issue(context.Background())

	if err != nil || invoice.Status != domain.InvoiceStatusIssued || *invoice.Number != 7 {
		t.Fatalf("invoice = %+v, err = %v, want ARCA's invoice 7 adopted", invoice, err)
	}
	if len(f.issuer.requests) != 0 || len(f.issuer.consulted) != 1 || f.issuer.consulted[0] != 7 {
		t.Fatalf("consulted %v, requested %d, want only the claimed number read back", f.issuer.consulted,
			len(f.issuer.requests))
	}
}

func TestInvoiceService_StalePendingIsReleasedWhenARCADoesNotHaveIt(t *testing.T) {
	t.Parallel()
	f := newInvoiceFixture(t)
	claimed := int64(6)
	f.store.invoice = &domain.Invoice{ID: uuid.New(), Status: domain.InvoiceStatusPending,
		QuoteVersionID: newVersionID(f), UpdatedAt: invoiceNow.Add(-time.Hour), ClaimedNumber: &claimed,
		Amounts: domain.InvoiceAmounts{Total: dec("1210.00")}}
	// Another system's invoice holds the number this one claimed, so ARCA refused ours.
	f.issuer.found = &domain.AuthorizedInvoice{Number: 6, Total: dec("99.00")}

	invoice, err := f.issue(context.Background())

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

	if domain.CodeOf(err) != domain.CodePointOfSaleTaken {
		t.Fatalf("err = %v, want POINT_OF_SALE_TAKEN", err)
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

// A hold that never reached ARCA claimed no number, so there is nothing to read back.
func TestInvoiceService_StalePendingWithoutAClaimIsReleasedUnasked(t *testing.T) {
	t.Parallel()
	f := newInvoiceFixture(t)
	f.store.invoice = &domain.Invoice{ID: uuid.New(), Status: domain.InvoiceStatusPending,
		QuoteVersionID: newVersionID(f), UpdatedAt: invoiceNow.Add(-time.Hour)}

	if _, err := f.issue(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(f.issuer.consulted) != 0 || len(f.store.released) != 1 {
		t.Fatalf("consulted %v, released %v, want the hold released without asking ARCA",
			f.issuer.consulted, f.store.released)
	}
}

func TestInvoiceService_IssueClaimsTheNumberBeforeARCAAnswers(t *testing.T) {
	t.Parallel()
	f := newInvoiceFixture(t)
	f.issuer.err = domain.ErrInvoiceOutcomeUnknown

	_, _ = f.issue(context.Background())

	if f.store.invoice.ClaimedNumber == nil || *f.store.invoice.ClaimedNumber != 42 {
		t.Fatalf("claimed = %v, want the number the issuer was about to request", f.store.invoice.ClaimedNumber)
	}
}

// The seller confirmed one invoice; if the sale moved since, another must not go out in its name.
func TestInvoiceService_IssueRefusesWhatTheSellerDidNotConfirm(t *testing.T) {
	t.Parallel()
	cases := map[string]domain.InvoiceExpectation{
		"another version": {VersionID: uuid.New(), Type: domain.InvoiceTypeA, Total: dec("1210.00")},
		"another type":    {Type: domain.InvoiceTypeB, Total: dec("1210.00")},
		"another total":   {Type: domain.InvoiceTypeA, Total: dec("1200.00")},
	}
	for name, expected := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newInvoiceFixture(t)
			if expected.VersionID == uuid.Nil {
				expected.VersionID = f.quotes.version.ID
			}

			_, err := f.service.Issue(context.Background(), f.tenant, f.quoteID, expected)

			if domain.CodeOf(err) != domain.CodeInvoiceStale || len(f.issuer.requests) != 0 {
				t.Fatalf("err = %v, want INVOICE_STALE without asking ARCA", err)
			}
		})
	}
}

// Closing the tab mid-request must not cut ARCA off: an authorization never recorded cannot be repaired.
func TestInvoiceService_IssueOutlivesTheCallersRequest(t *testing.T) {
	t.Parallel()
	f := newInvoiceFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	invoice, err := f.issue(ctx)

	if err != nil || f.issuer.ctxErr != nil || invoice.Status != domain.InvoiceStatusIssued {
		t.Fatalf("invoice = %+v, err = %v, issuer saw %v, want ARCA asked and the answer recorded",
			invoice, err, f.issuer.ctxErr)
	}
}

// Discounts larger than their lines leave a quote total no invoice can match.
func TestInvoiceService_PreviewFlagsATotalTheLinesCannotCarry(t *testing.T) {
	t.Parallel()
	f := newInvoiceFixture(t)
	f.quotes.version.Total = dec("900.00")

	preview, err := f.service.Preview(context.Background(), f.tenant, f.quoteID)

	if err != nil || !containsString(preview.Issues, "QUOTE_TOTAL_MISMATCH") {
		t.Fatalf("issues = %v, err = %v, want QUOTE_TOTAL_MISMATCH", preview.Issues, err)
	}
}

func TestInvoiceService_TicketStoreSealsWhatItKeeps(t *testing.T) {
	t.Parallel()
	f := newInvoiceFixture(t)
	if _, err := f.issue(context.Background()); err != nil {
		t.Fatal(err)
	}
	tickets := f.issuer.creds.Tickets
	expires := invoiceNow.Add(12 * time.Hour)

	if err := tickets.Save(context.Background(), domain.ARCATicket{Token: "T", Sign: "S", ExpiresAt: expires}); err != nil {
		t.Fatal(err)
	}
	if f.store.ticket == nil || f.store.ticket.Token != "sealed:T" || f.store.ticket.Sign != "sealed:S" {
		t.Fatalf("stored = %+v, want token and sign sealed", f.store.ticket)
	}
	if err := tickets.Save(context.Background(), domain.ARCATicket{}); err != nil || f.store.ticket != nil {
		t.Fatalf("stored = %+v, err = %v, want an empty ticket to clear it", f.store.ticket, err)
	}
}

func containsString(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}
