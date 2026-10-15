package services

import (
	"context"
	"errors"
	"slices"
	"strings"
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
	savedProfile *domain.InvoiceIssuerProfile
	// Off by default; a test turns it on to prove a refusal does not depend on the index.
	noUniqueIndex  bool
	pointWrites    int
	snapshotErr    error
	snapshotSource *domain.QuoteRepresentationSource
	sourceErr      error
	account        domain.AccountFiscal
	pointOfSale    *int
	client         *domain.ClientFiscal
	rates          map[uuid.UUID]domain.VATRate
	credential     *domain.ARCACredentialStatus
	sealedKey      string
	setupEnv       domain.ARCAEnvironment
	branches       []domain.BranchPointOfSale
	invoice        *domain.Invoice
	earlier        []domain.Invoice // the quote's older invoices, oldest first.
	released       []string
	rejected       *domain.InvoiceRejectedError
	stored         string
	ticket         *domain.SealedARCATicket
	accountLocked  bool
	heldUnlocked   bool
}

func (f *fakeInvoicingStore) LockAccountForShare(context.Context, repository.Querier, uuid.UUID) error {
	f.accountLocked = true
	return nil
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
	f.pointWrites++
	for i := range f.branches {
		if pos, ok := points[f.branches[i].BranchID]; ok {
			f.branches[i].PointOfSale = pos
		}
	}
	if f.noUniqueIndex {
		return nil
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

func (f *fakeInvoicingStore) GetSealedCredential(context.Context, repository.Querier, uuid.UUID) (string, string, domain.ARCAEnvironment, error) {
	return "CERT", f.sealedKey, f.setupEnv, nil
}

func (f *fakeInvoicingStore) GetByID(context.Context, repository.Querier, uuid.UUID, uuid.UUID, uuid.UUID) (*domain.Invoice, error) {
	if f.invoice == nil {
		return nil, domain.ErrNotFound
	}
	invoice := *f.invoice
	return &invoice, nil
}

func (f *fakeInvoicingStore) List(context.Context, repository.Querier, uuid.UUID, []uuid.UUID, int, int) ([]domain.Invoice, int, error) {
	return nil, 0, nil
}

func (f *fakeInvoicingStore) UpsertCredential(_ context.Context, _ repository.Querier, _ uuid.UUID, _ string, sealedKey string, status domain.ARCACredentialStatus) (*domain.ARCACredentialStatus, error) {
	f.stored = sealedKey
	return &status, nil
}

func (f *fakeInvoicingStore) DeleteCredential(context.Context, repository.Querier, uuid.UUID) error {
	return nil
}

// all lists the quote's invoices, oldest first.
func (f *fakeInvoicingStore) all() []*domain.Invoice {
	out := make([]*domain.Invoice, 0, len(f.earlier)+1)
	for i := range f.earlier {
		out = append(out, &f.earlier[i])
	}
	if f.invoice != nil {
		out = append(out, f.invoice)
	}
	return out
}

func (f *fakeInvoicingStore) GetLatestByQuote(_ context.Context, _ repository.Querier, _, _, _ uuid.UUID, env domain.ARCAEnvironment) (*domain.Invoice, error) {
	invoices := f.all()
	for i := len(invoices) - 1; i >= 0; i-- {
		if isCreditNote(*invoices[i]) || f.credited(invoices[i].ID) {
			continue
		}
		if invoices[i].Environment == env || invoices[i].Status == domain.InvoiceStatusPending {
			return invoices[i], nil
		}
	}
	return nil, nil
}

func (f *fakeInvoicingStore) GetLatestIssuedOutside(_ context.Context, _ repository.Querier, _, _, _ uuid.UUID, env domain.ARCAEnvironment) (*domain.Invoice, error) {
	invoices := f.all()
	for i := len(invoices) - 1; i >= 0; i-- {
		if !isCreditNote(*invoices[i]) && invoices[i].Environment != env && invoices[i].Status == domain.InvoiceStatusIssued {
			return invoices[i], nil
		}
	}
	return nil, nil
}

func (f *fakeInvoicingStore) GetLatestCreditedByQuote(_ context.Context, _ repository.Querier, _, _, _ uuid.UUID, env domain.ARCAEnvironment) (*domain.Invoice, error) {
	invoices := f.all()
	for i := len(invoices) - 1; i >= 0; i-- {
		if !isCreditNote(*invoices[i]) && invoices[i].Environment == env && f.credited(invoices[i].ID) {
			return invoices[i], nil
		}
	}
	return nil, nil
}

func (f *fakeInvoicingStore) GetByIDs(_ context.Context, _ repository.Querier, _ uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]domain.Invoice, error) {
	found := map[uuid.UUID]domain.Invoice{}
	for _, inv := range f.all() {
		if slices.Contains(ids, inv.ID) {
			found[inv.ID] = *inv
		}
	}
	return found, nil
}

func (f *fakeInvoicingStore) GetCreditNotesByInvoiceIDs(_ context.Context, _ repository.Querier, _ uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]domain.Invoice, error) {
	notes := map[uuid.UUID]domain.Invoice{}
	for _, inv := range f.all() {
		if isCreditNote(*inv) && slices.Contains(ids, *inv.CreditedInvoiceID) {
			notes[*inv.CreditedInvoiceID] = *inv
		}
	}
	return notes, nil
}

// credited reports whether an issued credit note cancels the invoice id.
func (f *fakeInvoicingStore) credited(id uuid.UUID) bool {
	for _, inv := range f.all() {
		if isCreditNote(*inv) && *inv.CreditedInvoiceID == id && inv.Status == domain.InvoiceStatusIssued {
			return true
		}
	}
	return false
}

func isCreditNote(inv domain.Invoice) bool { return inv.Kind == domain.VoucherKindCreditNote }

func sameID(a, b *uuid.UUID) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

func (f *fakeInvoicingStore) CreatePending(_ context.Context, _ repository.Querier, in domain.Invoice) (*domain.Invoice, error) {
	if !f.accountLocked {
		f.heldUnlocked = true
	}
	// Mirrors uq_invoice_active_quote and uq_invoice_credit_note, unless a test turns the indexes off.
	for _, inv := range f.all() {
		if f.noUniqueIndex || inv.Status == domain.InvoiceStatusRejected || isCreditNote(*inv) != isCreditNote(in) {
			continue
		}
		if isCreditNote(in) && *inv.CreditedInvoiceID == *in.CreditedInvoiceID {
			return nil, domain.ErrConflict
		}
		if !isCreditNote(in) && inv.Environment == in.Environment && sameID(inv.ReplacesInvoiceID, in.ReplacesInvoiceID) {
			return nil, domain.ErrConflict
		}
	}
	if f.invoice != nil {
		f.earlier = append(f.earlier, *f.invoice)
	}
	in.ID = uuid.New()
	in.Status = domain.InvoiceStatusPending
	f.invoice = &in
	return &in, nil
}

func (f *fakeInvoicingStore) MarkIssued(_ context.Context, _ repository.Querier, _, _, _ uuid.UUID, auth domain.InvoiceAuthorization) (*domain.Invoice, error) {
	f.invoice.Status = domain.InvoiceStatusIssued
	f.invoice.Number = &auth.Number
	f.invoice.CAE = &auth.CAE
	return f.invoice, nil
}

func (f *fakeInvoicingStore) MarkRejected(_ context.Context, _ repository.Querier, _, _, _ uuid.UUID, rejection domain.InvoiceRejectedError) error {
	f.invoice.Status = domain.InvoiceStatusRejected
	f.invoice.Issues = rejection.Issues
	f.rejected = &rejection
	return nil
}

func (f *fakeInvoicingStore) ReleasePending(_ context.Context, _ repository.Querier, _, _, _ uuid.UUID, reason string) error {
	f.invoice.Status = domain.InvoiceStatusRejected
	f.invoice.Issues = []string{reason}
	f.released = append(f.released, reason)
	return nil
}

func (f *fakeInvoicingStore) ClaimNumber(_ context.Context, _ repository.Querier, _, _, _ uuid.UUID, number int64) error {
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
	readWith  []domain.ARCAEnvironment // the credentials' environment on each read-back.
	// consultedTypes is the series of each read-back.
	consultedTypes []domain.VoucherType
}

func (f *fakeInvoiceIssuer) Issue(ctx context.Context, creds domain.ARCACredentials, req domain.InvoiceRequest) (*domain.InvoiceAuthorization, error) {
	f.requests = append(f.requests, req)
	f.ctxErr, f.creds = ctx.Err(), creds
	if err := req.Claim(ctx, 42); err != nil {
		return nil, err
	}
	return f.auth, f.err
}

func (f *fakeInvoiceIssuer) Authorized(_ context.Context, creds domain.ARCACredentials, _ string, voucher domain.VoucherType, _ int, number int64) (*domain.AuthorizedInvoice, error) {
	f.consulted = append(f.consulted, number)
	f.consultedTypes = append(f.consultedTypes, voucher)
	f.readWith = append(f.readWith, creds.Environment)
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
		Fingerprint: f.fingerprint(), VersionID: f.quotes.version.ID, Type: domain.InvoiceTypeA, Total: dec("1210.00")})
}

func newInvoiceFixture(t *testing.T) invoiceFixture {
	t.Helper()
	registered := domain.IVAConditionRegistered
	address := "Main 123"
	taxID, pos := "30-71234567-1", 3
	cuit, product := "20123456786", uuid.New()
	versionID, quoteID, branchID := uuid.New(), uuid.New(), uuid.New()
	accountID := uuid.New()
	store := &fakeInvoicingStore{
		account:     domain.AccountFiscal{Name: "Supplier", Profile: domain.InvoiceIssuerProfile{Address: "Main 123", GrossIncomeRegistration: "Exempt", ActivityStartedOn: "2020-01-01"}, TaxID: &taxID, IVACondition: &registered},
		pointOfSale: &pos,
		client:      &domain.ClientFiscal{Name: &address, Address: &address, TaxID: &cuit, IVACondition: &registered},
		rates:       map[uuid.UUID]domain.VATRate{product: domain.VATRateTwentyOne},
		credential: &domain.ARCACredentialStatus{Environment: domain.ARCAEnvironmentHomologation, CUIT: "30712345671",
			ExpiresAt: invoiceNow.AddDate(1, 0, 0)},
		sealedKey: accountID.String() + ":KEY",
		setupEnv:  domain.ARCAEnvironmentHomologation,
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
		fakeKeySealer{enabled: true}, InvoiceSettings{Enabled: true, Environment: domain.ARCAEnvironmentHomologation, IssueTimeout: time.Second,
			ReconcileAfter: time.Minute, UnidentifiedMax: decimal.NewFromInt(10_000_000)},
		func() time.Time { return invoiceNow }, nil)
	return invoiceFixture{service: service, store: store, issuer: issuer, quoteID: quoteID, quotes: quotes,
		tenant: domain.Tenant{AccountID: accountID, BranchID: branchID}}
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
	if req.Type != domain.InvoiceTypeA || req.IssuerCUIT != "30712345671" || req.PointOfSale != 3 ||
		req.Receiver.DocType != domain.ReceiverDocCUIT {
		t.Fatalf("request = %+v, want an A invoice from the account's CUIT at its point of sale", req)
	}
	assertMoney(t, "request total", req.Amounts.Total, "1210.00")
	if invoice.Environment != domain.ARCAEnvironmentHomologation || f.issuer.creds.Environment != domain.ARCAEnvironmentHomologation {
		t.Fatalf("invoice in %q, asked with %q credentials, want both in the server's environment",
			invoice.Environment, f.issuer.creds.Environment)
	}
	if want := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC); !req.Date.Equal(want) {
		t.Fatalf("date = %s, want the day in Argentina", req.Date)
	}
}

// A CUIT change or a disconnect must not slip between the readiness check and the pending invoice.
func TestInvoiceService_IssueHoldsTheAccountBeforeReservingTheInvoice(t *testing.T) {
	t.Parallel()
	f := newInvoiceFixture(t)

	if _, err := f.issue(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.store.heldUnlocked {
		t.Fatal("the pending invoice was written without holding the account row")
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
	f.store.invoice = &domain.Invoice{ID: uuid.New(), Environment: domain.ARCAEnvironmentHomologation, Status: domain.InvoiceStatusPending,
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

func TestInvoiceService_StalePendingMismatchKeepsItsHold(t *testing.T) {
	t.Parallel()
	f := newInvoiceFixture(t)
	claimed := int64(6)
	f.store.invoice = &domain.Invoice{ID: uuid.New(), Environment: domain.ARCAEnvironmentHomologation, Status: domain.InvoiceStatusPending,
		QuoteVersionID: newVersionID(f), UpdatedAt: invoiceNow.Add(-time.Hour), ClaimedNumber: &claimed,
		Amounts: domain.InvoiceAmounts{Total: dec("1210.00")}}
	// Another system's invoice holds the number this one claimed, so ARCA refused ours.
	f.issuer.found = &domain.AuthorizedInvoice{Number: 6, Total: dec("99.00")}

	invoice, err := f.issue(context.Background())

	if domain.CodeOf(err) != domain.CodeInvoiceInProgress || len(f.store.released) != 0 || len(f.issuer.requests) != 0 {
		t.Fatalf("invoice = %+v, err = %v, released = %v, want the mismatch held without another invoice",
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

// The duplicate is refused by the service before any write, so it holds even where no index would
// catch it.
func TestInvoiceService_UpdatePointsOfSaleRefusesADuplicateBeforeWriting(t *testing.T) {
	t.Parallel()
	f := newInvoiceFixture(t)
	f.store.noUniqueIndex = true
	first, second := uuid.New(), uuid.New()
	f.store.branches = []domain.BranchPointOfSale{{BranchID: first}, {BranchID: second}}
	pos := 4

	_, err := f.service.UpdatePointsOfSale(context.Background(), f.tenant, map[uuid.UUID]*int{first: &pos, second: &pos})

	if domain.CodeOf(err) != domain.CodePointOfSaleTaken || !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("err = %v, want 409 POINT_OF_SALE_TAKEN", err)
	}
	if f.store.pointWrites != 0 {
		t.Fatal("the duplicate reached the store")
	}
}

func TestInvoiceService_UpdatePointsOfSaleWordsTheIndexRefusal(t *testing.T) {
	t.Parallel()
	f := newInvoiceFixture(t)
	first, second := uuid.New(), uuid.New()
	kept := 4
	f.store.branches = []domain.BranchPointOfSale{{BranchID: first, PointOfSale: &kept}, {BranchID: second}}

	_, err := f.service.UpdatePointsOfSale(context.Background(), f.tenant, map[uuid.UUID]*int{second: &kept})

	if domain.CodeOf(err) != domain.CodePointOfSaleTaken {
		t.Fatalf("err = %v, want POINT_OF_SALE_TAKEN for a number another branch keeps", err)
	}
}

func TestInvoiceService_UpdateSettingsPersistsPointsOfSale(t *testing.T) {
	t.Parallel()
	f := newInvoiceFixture(t)
	first, second := uuid.New(), uuid.New()
	kept := 7
	f.store.branches = []domain.BranchPointOfSale{{BranchID: first, PointOfSale: &kept}, {BranchID: second}}
	moved := 3

	settings, err := f.service.UpdateSettings(context.Background(), f.tenant, domain.InvoicingSettingsUpdate{
		PointsOfSale: map[uuid.UUID]*int{first: nil, second: &moved}})

	if err != nil {
		t.Fatal(err)
	}
	if settings.Branches[0].PointOfSale != nil || *settings.Branches[1].PointOfSale != 3 {
		t.Fatalf("branches = %+v, want the first cleared and the second at 3", settings.Branches)
	}
}

func TestInvoiceService_UpdateSettingsRefusesAPointOfSaleOutOfRange(t *testing.T) {
	t.Parallel()
	for _, pos := range []int{0, domain.PointOfSaleMax + 1} {
		f := newInvoiceFixture(t)
		_, err := f.service.UpdateSettings(context.Background(), f.tenant, domain.InvoicingSettingsUpdate{
			PointsOfSale: map[uuid.UUID]*int{uuid.New(): &pos}})
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("point of sale %d: err = %v, want ErrInvalidInput", pos, err)
		}
	}
}

func TestInvoiceService_UpdateSettingsNormalizesTheIssuerProfile(t *testing.T) {
	t.Parallel()
	cases := []struct {
		registration string
		want         string
	}{
		{" exento ", domain.GrossIncomeExempt},
		{"901.123 456-7", "901123456-7"},
	}
	for _, tc := range cases {
		f := newInvoiceFixture(t)
		_, err := f.service.UpdateSettings(context.Background(), f.tenant, domain.InvoicingSettingsUpdate{
			Profile: &domain.InvoiceIssuerProfile{Address: "  Av.  Rivadavia   1234 ", GrossIncomeRegistration: tc.registration, ActivityStartedOn: "2020-01-02"}})
		if err != nil {
			t.Fatal(err)
		}
		if got := *f.store.savedProfile; got.Address != "Av. Rivadavia 1234" || got.GrossIncomeRegistration != tc.want {
			t.Fatalf("saved %+v, want the address collapsed and the registration %q", got, tc.want)
		}
	}
}

func TestInvoiceService_UpdateSettingsRefusesAMalformedGrossIncomeRegistration(t *testing.T) {
	t.Parallel()
	f := newInvoiceFixture(t)
	_, err := f.service.UpdateSettings(context.Background(), f.tenant, domain.InvoicingSettingsUpdate{
		Profile: &domain.InvoiceIssuerProfile{Address: "Av. Rivadavia 1234", GrossIncomeRegistration: "en trámite", ActivityStartedOn: "2020-01-02"}})
	if !errors.Is(err, domain.ErrInvalidInput) || f.store.savedProfile != nil {
		t.Fatalf("err = %v, saved = %+v, want the profile refused unsaved", err, f.store.savedProfile)
	}
}

func TestInvoiceService_StalePendingWithoutAClaimIsReleasedUnasked(t *testing.T) {
	t.Parallel()
	f := newInvoiceFixture(t)
	f.store.invoice = &domain.Invoice{ID: uuid.New(), Environment: domain.ARCAEnvironmentHomologation, Status: domain.InvoiceStatusPending,
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
	if f.store.ticket == nil || !strings.HasPrefix(f.store.ticket.Token, "sealed:"+f.tenant.AccountID.String()+":") {
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

func (f invoiceFixture) fingerprint() string {
	p, _ := f.service.Preview(context.Background(), f.tenant, f.quoteID)
	if p == nil {
		return ""
	}
	return p.Fingerprint
}
func (f *fakeInvoicingStore) GetSnapshot(context.Context, repository.Querier, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID) (domain.QuoteRepresentationPayload, error) {
	return domain.QuoteRepresentationPayload{}, f.snapshotErr
}
func (f *fakeInvoicingStore) GetSnapshotSource(context.Context, repository.Querier, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID) (*domain.QuoteRepresentationSource, error) {
	return f.snapshotSource, f.sourceErr
}
func (f *fakeInvoicingStore) ClaimRecovery(context.Context, repository.Querier, uuid.UUID, uuid.UUID, uuid.UUID, time.Time) (bool, error) {
	return true, nil
}
func (f *fakeInvoiceQuotes) GetByIDForUpdate(ctx context.Context, q repository.Querier, a, b, id uuid.UUID) (*domain.Quote, error) {
	return f.GetByID(ctx, q, a, b, id)
}

func (f *fakeInvoicingStore) SaveProfile(_ context.Context, _ repository.Querier, _ uuid.UUID, profile domain.InvoiceIssuerProfile) error {
	f.savedProfile = &profile
	return nil
}

func newVersionID(f invoiceFixture) uuid.UUID { return f.quotes.version.ID }

func TestInvoiceService_PreviewUsesARCACUITAndBranchPointOfSale(t *testing.T) {
	t.Parallel()
	f := newInvoiceFixture(t)
	f.store.account.TaxID = nil
	preview, err := f.service.Preview(context.Background(), f.tenant, f.quoteID)
	if err != nil {
		t.Fatal(err)
	}
	if preview.IssuerCUIT != f.store.credential.CUIT || preview.PointOfSale == nil || *preview.PointOfSale != *f.store.pointOfSale {
		t.Fatalf("preview = %+v, want ARCA CUIT and branch point of sale", preview)
	}
}

// Issue goes through the same checks as the preview, so neither gap reaches ARCA.
func TestInvoiceService_IssueRefusesACUITTheCertificateDoesNotCover(t *testing.T) {
	t.Parallel()
	other, blank := "20329642330", ""
	for name, taxID := range map[string]*string{"another CUIT": &other, "no CUIT": &blank} {
		f := newInvoiceFixture(t)
		f.store.account.TaxID = taxID
		_, err := f.issue(context.Background())
		var notReady *domain.InvoiceNotReadyError
		if !errors.As(err, &notReady) {
			t.Fatalf("%s: err = %v, want the invoice refused as not ready", name, err)
		}
		want := invoiceIssueCredentialsCUIT
		if taxID == &blank {
			want = invoiceIssueTaxID
		}
		if !slices.Contains(notReady.Issues, want) {
			t.Fatalf("%s: issues = %v, want %s", name, notReady.Issues, want)
		}
		if len(f.issuer.requests) != 0 {
			t.Fatalf("%s: the request reached ARCA", name)
		}
	}
}

// A connection made for the other environment stops the invoice where the seller sees it.
func TestInvoiceService_PreviewFlagsAConnectionForTheOtherEnvironment(t *testing.T) {
	t.Parallel()
	f := newInvoiceFixture(t)
	f.service.settings.Environment = domain.ARCAEnvironmentProduction

	preview, err := f.service.Preview(context.Background(), f.tenant, f.quoteID)
	if err != nil || !slices.Contains(preview.Issues, invoiceIssueCredentialsEnv) ||
		preview.Environment != domain.ARCAEnvironmentProduction {
		t.Fatalf("preview = %+v, err = %v, want ARCA_CREDENTIALS_ENVIRONMENT in production", preview, err)
	}
	_, err = f.issue(context.Background())
	var notReady *domain.InvoiceNotReadyError
	if !errors.As(err, &notReady) || len(f.issuer.requests) != 0 {
		t.Fatalf("err = %v after %d requests, want the invoice refused before ARCA", err, len(f.issuer.requests))
	}
}

// A pending invoice is read back where it was requested, whatever the server is set to now.
func TestInvoiceService_RecoveryAsksTheInvoicesOwnEnvironment(t *testing.T) {
	t.Parallel()
	f := newInvoiceFixture(t)
	f.service.settings.Environment = domain.ARCAEnvironmentProduction
	claimed := int64(7)
	f.store.invoice = &domain.Invoice{ID: uuid.New(), Environment: domain.ARCAEnvironmentHomologation,
		Status: domain.InvoiceStatusPending, QuoteVersionID: newVersionID(f), UpdatedAt: invoiceNow.Add(-time.Hour),
		ClaimedNumber: &claimed, Amounts: domain.InvoiceAmounts{Total: dec("1210.00")}}

	if _, err := f.service.Recover(context.Background(), f.tenant, f.store.invoice.ID); err != nil {
		t.Fatal(err)
	}
	if len(f.issuer.readWith) != 1 || f.issuer.readWith[0] != domain.ARCAEnvironmentHomologation {
		t.Fatalf("read back with %v, want the invoice's homologation credentials", f.issuer.readWith)
	}
}

// Asking the wrong deployment would find no invoice and release a hold ARCA may have authorized.
func TestInvoiceService_RecoveryRefusesAConnectionForAnotherEnvironment(t *testing.T) {
	t.Parallel()
	for name, recover := range map[string]func(f invoiceFixture) error{
		"recover": func(f invoiceFixture) error {
			_, err := f.service.Recover(context.Background(), f.tenant, f.store.invoice.ID)
			return err
		},
		"issue again": func(f invoiceFixture) error {
			_, err := f.issue(context.Background())
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newInvoiceFixture(t)
			claimed := int64(7)
			f.store.invoice = &domain.Invoice{ID: uuid.New(), Environment: domain.ARCAEnvironmentProduction,
				Status: domain.InvoiceStatusPending, QuoteVersionID: newVersionID(f), UpdatedAt: invoiceNow.Add(-time.Hour),
				ClaimedNumber: &claimed, Amounts: domain.InvoiceAmounts{Total: dec("1210.00")}}

			err := recover(f)

			if domain.CodeOf(err) != domain.CodeARCAEnvironmentMismatch {
				t.Fatalf("err = %v, want ARCA_ENVIRONMENT_MISMATCH", err)
			}
			if len(f.issuer.consulted) != 0 || len(f.issuer.requests) != 0 || len(f.store.released) != 0 ||
				f.store.invoice.Status != domain.InvoiceStatusPending {
				t.Fatalf("consulted %v, requested %d, released %v, want the hold kept and ARCA never asked",
					f.issuer.consulted, len(f.issuer.requests), f.store.released)
			}
		})
	}
}

// productionFixture is the fixture after go-live: the server and its connection in production.
func productionFixture(t *testing.T) invoiceFixture {
	t.Helper()
	f := newInvoiceFixture(t)
	f.service.settings.Environment = domain.ARCAEnvironmentProduction
	f.store.setupEnv = domain.ARCAEnvironmentProduction
	f.store.credential.Environment = domain.ARCAEnvironmentProduction
	return f
}

func issuedTestInvoice(f invoiceFixture) *domain.Invoice {
	number, cae := int64(5), "74000000000005"
	return &domain.Invoice{ID: uuid.New(), Environment: domain.ARCAEnvironmentHomologation,
		Status: domain.InvoiceStatusIssued, QuoteVersionID: newVersionID(f), Number: &number, CAE: &cae}
}

// A test invoice is history once the server invoices for real: it never holds the sale.
func TestInvoiceService_ATestInvoiceDoesNotHoldTheRealOne(t *testing.T) {
	t.Parallel()
	f := productionFixture(t)
	test := issuedTestInvoice(f)
	f.store.invoice = test

	preview, err := f.service.Preview(context.Background(), f.tenant, f.quoteID)
	if err != nil || preview.Invoice != nil || preview.OtherEnvironmentInvoice == nil ||
		preview.OtherEnvironmentInvoice.ID != test.ID {
		t.Fatalf("preview = %+v, err = %v, want no hold and the test invoice as history", preview, err)
	}
	invoice, err := f.issue(context.Background())
	if err != nil || invoice.Status != domain.InvoiceStatusIssued || invoice.Environment != domain.ARCAEnvironmentProduction {
		t.Fatalf("invoice = %+v, err = %v, want the real invoice issued beside the test one", invoice, err)
	}
}

// Within one environment the sale still takes one invoice, refused before the index is reached.
func TestInvoiceService_TwoInvoicesInOneEnvironmentAreRefused(t *testing.T) {
	t.Parallel()
	for name, f := range map[string]invoiceFixture{"homologation": newInvoiceFixture(t), "production": productionFixture(t)} {
		f.store.noUniqueIndex = true
		issued := issuedTestInvoice(f)
		issued.Environment = f.service.settings.Environment
		f.store.invoice = issued

		_, err := f.issue(context.Background())

		if domain.CodeOf(err) != domain.CodeQuoteAlreadyInvoiced || len(f.issuer.requests) != 0 {
			t.Fatalf("%s: err = %v after %d requests, want QUOTE_ALREADY_INVOICED", name, err, len(f.issuer.requests))
		}
	}
}

// A test invoice still waiting for ARCA keeps the sale until it is read back in its own environment.
func TestInvoiceService_APendingTestInvoiceHoldsTheSaleInProduction(t *testing.T) {
	t.Parallel()
	f := productionFixture(t)
	f.store.invoice = &domain.Invoice{ID: uuid.New(), Environment: domain.ARCAEnvironmentHomologation,
		Status: domain.InvoiceStatusPending, QuoteVersionID: newVersionID(f), UpdatedAt: invoiceNow}

	_, err := f.issue(context.Background())

	if domain.CodeOf(err) != domain.CodeInvoiceInProgress || len(f.issuer.requests) != 0 {
		t.Fatalf("err = %v, want INVOICE_IN_PROGRESS without asking ARCA", err)
	}
}
