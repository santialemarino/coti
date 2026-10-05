package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// pendingStaleCalls is how many ARCA calls an issue may chain (login, last number, request, one
// retry, recovery). A pending invoice older than that many timeouts is no longer in flight.
const pendingStaleCalls = 10

// argentina is the zone an invoice's date is read in.
var argentina = mustLoadLocation("America/Argentina/Buenos_Aires")

type invoicingStore interface {
	GetAccountFiscal(ctx context.Context, q repository.Querier, accountID uuid.UUID) (*domain.AccountFiscal, error)
	UpdateAccountFiscal(ctx context.Context, q repository.Querier, accountID uuid.UUID, condition *domain.IVACondition, pricesIncludeVAT bool) error
	ListBranchPointsOfSale(ctx context.Context, q repository.Querier, accountID uuid.UUID) ([]domain.BranchPointOfSale, error)
	GetBranchPointOfSale(ctx context.Context, q repository.Querier, accountID, branchID uuid.UUID) (*int, error)
	UpdateBranchPointsOfSale(ctx context.Context, q repository.Querier, accountID uuid.UUID, points map[uuid.UUID]*int) error
	GetClientFiscal(ctx context.Context, q repository.Querier, accountID, clientID uuid.UUID) (*domain.ClientFiscal, error)
	UpdateClientFiscal(ctx context.Context, q repository.Querier, accountID, clientID uuid.UUID, in domain.ClientFiscalUpdate) (*domain.ClientFiscal, error)
	VATRatesByProductIDs(ctx context.Context, q repository.Querier, accountID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]domain.VATRate, error)
	GetCredentialStatus(ctx context.Context, q repository.Querier, accountID uuid.UUID) (*domain.ARCACredentialStatus, error)
	GetSealedCredential(ctx context.Context, q repository.Querier, accountID uuid.UUID) (string, string, error)
	UpsertCredential(ctx context.Context, q repository.Querier, accountID uuid.UUID, certificatePEM, sealedKey string, status domain.ARCACredentialStatus) (*domain.ARCACredentialStatus, error)
	DeleteCredential(ctx context.Context, q repository.Querier, accountID uuid.UUID) error
	GetLatestByQuote(ctx context.Context, q repository.Querier, accountID, quoteID uuid.UUID) (*domain.Invoice, error)
	CreatePending(ctx context.Context, q repository.Querier, in domain.Invoice) (*domain.Invoice, error)
	MarkIssued(ctx context.Context, q repository.Querier, accountID, id uuid.UUID, auth domain.InvoiceAuthorization) (*domain.Invoice, error)
	MarkRejected(ctx context.Context, q repository.Querier, accountID, id uuid.UUID, rejection domain.InvoiceRejectedError) error
	ReleasePending(ctx context.Context, q repository.Querier, accountID, id uuid.UUID, reason string) error
}

type invoiceQuoteReader interface {
	GetByID(ctx context.Context, q repository.Querier, accountID, branchID, id uuid.UUID) (*domain.Quote, error)
	GetCurrentVersion(ctx context.Context, q repository.Querier, accountID, branchID, quoteID uuid.UUID) (*domain.QuoteVersion, error)
	ListItems(ctx context.Context, q repository.Querier, accountID, versionID uuid.UUID) ([]domain.QuoteItem, error)
}

type invoiceDiscountReader interface {
	ListByVersionID(ctx context.Context, q repository.Querier, accountID, versionID uuid.UUID) ([]domain.QuoteDiscount, error)
	ListItemIDsByDiscountIDs(ctx context.Context, q repository.Querier, accountID, versionID uuid.UUID, discountIDs []uuid.UUID) (map[uuid.UUID][]uuid.UUID, error)
}

type keySealer interface {
	Enabled() bool
	Seal(plaintext string) (string, error)
	Open(sealed string) (string, error)
}

// CredentialParser reads an uploaded certificate and key pair, refusing one that does not match.
type CredentialParser func(certificatePEM, keyPEM []byte) (*domain.ARCACredentialStatus, error)

// InvoiceSettings is what the invoice service is configured with.
type InvoiceSettings struct {
	Enabled         bool
	Environment     string
	RequestTimeout  time.Duration
	UnidentifiedMax decimal.Decimal
}

// InvoiceService issues ARCA electronic invoices for accepted quotes and keeps the fiscal data
// they need.
type InvoiceService struct {
	db        tenantTxRunner
	store     invoicingStore
	quotes    invoiceQuoteReader
	discounts invoiceDiscountReader
	issuer    domain.InvoiceIssuer
	sealer    keySealer
	parse     CredentialParser
	settings  InvoiceSettings
	now       func() time.Time
}

// NewInvoiceService builds the invoicing use cases.
func NewInvoiceService(
	db tenantTxRunner, store invoicingStore, quotes invoiceQuoteReader, discounts invoiceDiscountReader,
	issuer domain.InvoiceIssuer, sealer keySealer, parse CredentialParser, settings InvoiceSettings,
	now func() time.Time,
) *InvoiceService {
	if now == nil {
		now = time.Now
	}
	return &InvoiceService{db: db, store: store, quotes: quotes, discounts: discounts, issuer: issuer,
		sealer: sealer, parse: parse, settings: settings, now: now}
}

// --- Settings ---

// GetSettings returns the account's invoicing setup.
func (s *InvoiceService) GetSettings(ctx context.Context, tenant domain.Tenant) (*domain.InvoicingSettings, error) {
	settings := &domain.InvoicingSettings{Enabled: s.settings.Enabled, Environment: s.settings.Environment}
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		account, err := s.store.GetAccountFiscal(ctx, q, tenant.AccountID)
		if err != nil {
			return err
		}
		settings.Account = *account
		if settings.Branches, err = s.store.ListBranchPointsOfSale(ctx, q, tenant.AccountID); err != nil {
			return err
		}
		settings.Credential, err = s.store.GetCredentialStatus(ctx, q, tenant.AccountID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return settings, nil
}

// UpdateSettings replaces the account's IVA condition, how its prices treat IVA and the given
// branches' points of sale. Two branches cannot share a point of sale.
func (s *InvoiceService) UpdateSettings(
	ctx context.Context, tenant domain.Tenant, in domain.InvoicingSettingsUpdate,
) (*domain.InvoicingSettings, error) {
	if in.IVACondition != nil && !in.IVACondition.Valid() {
		return nil, fmt.Errorf("%w: unknown IVA condition %q", domain.ErrInvalidInput, *in.IVACondition)
	}
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		if err := s.store.UpdateAccountFiscal(ctx, q, tenant.AccountID, in.IVACondition, in.PricesIncludeVAT); err != nil {
			return err
		}
		if err := s.store.UpdateBranchPointsOfSale(ctx, q, tenant.AccountID, in.PointsOfSale); err != nil {
			return err
		}
		branches, err := s.store.ListBranchPointsOfSale(ctx, q, tenant.AccountID)
		if err != nil {
			return err
		}
		seen := make(map[int]bool, len(branches))
		for _, b := range branches {
			if b.PointOfSale == nil {
				continue
			}
			if seen[*b.PointOfSale] {
				return fmt.Errorf("%w: point of sale %d is set on two branches", domain.ErrInvalidInput, *b.PointOfSale)
			}
			seen[*b.PointOfSale] = true
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.GetSettings(ctx, tenant)
}

// UploadCredentials stores the account's ARCA certificate and seals its key, replacing any pair
// uploaded before.
func (s *InvoiceService) UploadCredentials(
	ctx context.Context, tenant domain.Tenant, certificatePEM, keyPEM []byte,
) (*domain.ARCACredentialStatus, error) {
	if !s.sealer.Enabled() {
		return nil, fmt.Errorf("%w: ARCA_CREDENTIALS_ENCRYPTION_KEY is unset", domain.ErrNotConfigured)
	}
	status, err := s.parse(certificatePEM, keyPEM)
	if err != nil {
		return nil, err
	}
	sealed, err := s.sealer.Seal(string(keyPEM))
	if err != nil {
		return nil, err
	}
	var stored *domain.ARCACredentialStatus
	err = s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var upsertErr error
		stored, upsertErr = s.store.UpsertCredential(ctx, q, tenant.AccountID, string(certificatePEM), sealed, *status)
		return upsertErr
	})
	return stored, err
}

// DeleteCredentials removes the account's certificate and key.
func (s *InvoiceService) DeleteCredentials(ctx context.Context, tenant domain.Tenant) error {
	return s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		return s.store.DeleteCredential(ctx, q, tenant.AccountID)
	})
}

// GetClientFiscal returns a client's fiscal identity.
func (s *InvoiceService) GetClientFiscal(
	ctx context.Context, tenant domain.Tenant, clientID uuid.UUID,
) (*domain.ClientFiscal, error) {
	var client *domain.ClientFiscal
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var err error
		client, err = s.store.GetClientFiscal(ctx, q, tenant.AccountID, clientID)
		return err
	})
	return client, err
}

// UpdateClientFiscal replaces a client's legal name, tax id and IVA condition. The tax id is a CUIT
// (11 digits, check digit verified) or a DNI (7 or 8 digits); separators are dropped.
func (s *InvoiceService) UpdateClientFiscal(
	ctx context.Context, tenant domain.Tenant, clientID uuid.UUID, in domain.ClientFiscalUpdate,
) (*domain.ClientFiscal, error) {
	if in.IVACondition != nil && !in.IVACondition.Valid() {
		return nil, fmt.Errorf("%w: unknown IVA condition %q", domain.ErrInvalidInput, *in.IVACondition)
	}
	in.LegalName = trimmedOrNil(in.LegalName)
	if in.TaxID != nil {
		digits := domain.DigitsOnly(*in.TaxID)
		switch {
		case digits == "":
			in.TaxID = nil
		case len(digits) == 11 && validCUIT(digits), len(digits) == 7, len(digits) == 8:
			in.TaxID = &digits
		default:
			return nil, domain.WithCode(domain.CodeInvalidTaxID,
				fmt.Errorf("%w: %q is neither a valid CUIT nor a DNI", domain.ErrInvalidInput, *in.TaxID))
		}
	}
	var client *domain.ClientFiscal
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var err error
		client, err = s.store.UpdateClientFiscal(ctx, q, tenant.AccountID, clientID, in)
		return err
	})
	return client, err
}

// --- Invoices ---

// Preview returns the invoice an accepted quote would produce now, or the one it already has,
// with every gap that stops it.
func (s *InvoiceService) Preview(
	ctx context.Context, tenant domain.Tenant, quoteID uuid.UUID,
) (*domain.InvoicePreview, error) {
	if err := requireBranch(tenant, "an invoice"); err != nil {
		return nil, err
	}
	var preview *domain.InvoicePreview
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		draft, err := s.draft(ctx, q, tenant, quoteID)
		if err != nil {
			return err
		}
		preview = &draft.preview
		return nil
	})
	return preview, err
}

// Issue asks ARCA to authorize the invoice for an accepted quote. The quote version is held by a
// pending invoice while ARCA answers, so a second press can never authorize a second invoice.
func (s *InvoiceService) Issue(ctx context.Context, tenant domain.Tenant, quoteID uuid.UUID) (*domain.Invoice, error) {
	if err := requireBranch(tenant, "an invoice"); err != nil {
		return nil, err
	}

	var draft *invoiceDraft
	var creds domain.ARCACredentials
	var stale *domain.Invoice
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var err error
		if draft, err = s.draft(ctx, q, tenant, quoteID); err != nil {
			return err
		}
		if current := draft.preview.Invoice; current != nil {
			switch {
			case current.Status == domain.InvoiceStatusIssued:
				return domain.WithCode(domain.CodeQuoteAlreadyInvoiced, domain.ErrConflict)
			case current.Status == domain.InvoiceStatusPending && !s.isStale(*current):
				return domain.WithCode(domain.CodeInvoiceInProgress, domain.ErrConflict)
			case current.Status == domain.InvoiceStatusPending:
				stale = current
			}
		}
		if len(draft.preview.Issues) > 0 {
			return &domain.InvoiceNotReadyError{Issues: draft.preview.Issues}
		}
		creds, err = s.openCredentials(ctx, q, tenant.AccountID)
		return err
	})
	if err != nil {
		return nil, err
	}

	if stale != nil {
		adopted, err := s.reconcile(ctx, tenant, creds, *stale)
		if err != nil || adopted != nil {
			return adopted, err
		}
	}

	pending, err := s.hold(ctx, tenant, *draft)
	if err != nil {
		return nil, err
	}

	auth, issueErr := s.issuer.Issue(ctx, creds, domain.InvoiceRequest{
		IssuerCUIT:  pending.IssuerCUIT,
		Type:        pending.Type,
		PointOfSale: pending.PointOfSale,
		Date:        pending.IssuedOn,
		Receiver:    pending.Receiver,
		Amounts:     pending.Amounts,
	})
	return s.settle(ctx, tenant, *pending, auth, issueErr)
}

// invoiceDraft is a preview with what issuing it needs on top.
type invoiceDraft struct {
	preview    domain.InvoicePreview
	quote      domain.Quote
	version    domain.QuoteVersion
	issuerCUIT string
}

// draft reads the quote and the fiscal data and computes the invoice it would produce.
func (s *InvoiceService) draft(
	ctx context.Context, q repository.Querier, tenant domain.Tenant, quoteID uuid.UUID,
) (*invoiceDraft, error) {
	quote, err := s.quotes.GetByID(ctx, q, tenant.AccountID, tenant.BranchID, quoteID)
	if err != nil {
		return nil, err
	}
	version, err := s.quotes.GetCurrentVersion(ctx, q, tenant.AccountID, tenant.BranchID, quoteID)
	if err != nil {
		return nil, err
	}
	items, err := s.quotes.ListItems(ctx, q, tenant.AccountID, version.ID)
	if err != nil {
		return nil, err
	}
	discounts, err := s.versionDiscounts(ctx, q, tenant.AccountID, version.ID)
	if err != nil {
		return nil, err
	}
	productIDs := make([]uuid.UUID, 0, len(items))
	for _, item := range items {
		if item.ProductID != nil {
			productIDs = append(productIDs, *item.ProductID)
		}
	}
	rates, err := s.store.VATRatesByProductIDs(ctx, q, tenant.AccountID, uniqueUUIDs(productIDs))
	if err != nil {
		return nil, err
	}
	account, err := s.store.GetAccountFiscal(ctx, q, tenant.AccountID)
	if err != nil {
		return nil, err
	}
	pointOfSale, err := s.store.GetBranchPointOfSale(ctx, q, tenant.AccountID, quote.BranchID)
	if err != nil {
		return nil, err
	}
	credential, err := s.store.GetCredentialStatus(ctx, q, tenant.AccountID)
	if err != nil {
		return nil, err
	}
	var client *domain.ClientFiscal
	if quote.ClientID != nil {
		if client, err = s.store.GetClientFiscal(ctx, q, tenant.AccountID, *quote.ClientID); err != nil {
			return nil, err
		}
	}
	existing, err := s.store.GetLatestByQuote(ctx, q, tenant.AccountID, quoteID)
	if err != nil {
		return nil, err
	}

	issuerCondition := domain.IVAConditionRegistered
	if account.IVACondition != nil {
		issuerCondition = *account.IVACondition
	}
	receiver := invoiceReceiver(client, "")
	invoiceType := invoiceTypeFor(issuerCondition, receiver.IVACondition)
	amounts := invoiceAmounts(grossByRate(items, rates, discounts), invoiceType, account.PricesIncludeVAT)
	issues := invoiceIssues(invoiceInputs{
		enabled: s.settings.Enabled, quote: *quote, version: *version, account: *account,
		pointOfSale: pointOfSale, credential: credential, receiver: receiver, amounts: amounts,
		invoiceType: invoiceType, unidentifiedMax: s.settings.UnidentifiedMax, now: s.now(),
	})

	draft := &invoiceDraft{
		quote: *quote, version: *version, issuerCUIT: account.CUIT(),
		preview: domain.InvoicePreview{Type: invoiceType, PointOfSale: pointOfSale, Receiver: receiver,
			Amounts: amounts, Currency: arcaCurrency, Issues: nonNilIssues(issues)},
	}
	// Only an invoice for the version on show is this sale's; one for an older version is history.
	if existing != nil && existing.QuoteVersionID == version.ID && !released(*existing) {
		draft.preview.Invoice = existing
	}
	return draft, nil
}

// versionDiscounts loads a version's discounts with the lines each one covers.
func (s *InvoiceService) versionDiscounts(
	ctx context.Context, q repository.Querier, accountID, versionID uuid.UUID,
) ([]domain.QuoteDiscount, error) {
	discounts, err := s.discounts.ListByVersionID(ctx, q, accountID, versionID)
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, 0, len(discounts))
	for _, d := range discounts {
		if d.Scope != domain.DiscountScopeTotal {
			ids = append(ids, d.ID)
		}
	}
	links, err := s.discounts.ListItemIDsByDiscountIDs(ctx, q, accountID, versionID, ids)
	if err != nil {
		return nil, err
	}
	for i := range discounts {
		discounts[i].ItemIDs = links[discounts[i].ID]
	}
	return discounts, nil
}

// hold writes the pending invoice that reserves the quote version while ARCA is asked.
func (s *InvoiceService) hold(ctx context.Context, tenant domain.Tenant, draft invoiceDraft) (*domain.Invoice, error) {
	var pending *domain.Invoice
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var err error
		pending, err = s.store.CreatePending(ctx, q, domain.Invoice{
			AccountID: tenant.AccountID, BranchID: draft.quote.BranchID, QuoteID: draft.quote.ID,
			QuoteVersionID: draft.version.ID, Type: draft.preview.Type, PointOfSale: *draft.preview.PointOfSale,
			IssuedOn: s.today(), IssuerCUIT: draft.issuerCUIT, Receiver: draft.preview.Receiver,
			Amounts: draft.preview.Amounts, Currency: draft.preview.Currency,
		})
		if errors.Is(err, domain.ErrConflict) {
			return domain.WithCode(domain.CodeInvoiceInProgress, domain.ErrConflict)
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	// The repository returns the stored amounts; the request has to carry the computed breakdown.
	pending.Amounts = draft.preview.Amounts
	return pending, nil
}

// releasedIssue marks an attempt ARCA never answered: nothing was authorized or refused.
const releasedIssue = "ARCA_UNAVAILABLE"

// released reports whether inv is an attempt ARCA never answered, which is no verdict to show.
func released(inv domain.Invoice) bool {
	return inv.Status == domain.InvoiceStatusRejected && len(inv.Issues) == 1 && inv.Issues[0] == releasedIssue
}

// settle records ARCA's verdict on a pending invoice. A refusal or a failure that authorized
// nothing frees the quote version; an unknown outcome keeps it held until it is reconciled.
func (s *InvoiceService) settle(
	ctx context.Context, tenant domain.Tenant, pending domain.Invoice, auth *domain.InvoiceAuthorization,
	issueErr error,
) (*domain.Invoice, error) {
	var rejected *domain.InvoiceRejectedError
	switch {
	case issueErr == nil:
		var issued *domain.Invoice
		err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
			var err error
			issued, err = s.store.MarkIssued(ctx, q, tenant.AccountID, pending.ID, *auth)
			return err
		})
		return issued, err
	case errors.As(issueErr, &rejected):
		if err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
			return s.store.MarkRejected(ctx, q, tenant.AccountID, pending.ID, *rejected)
		}); err != nil {
			return nil, errors.Join(issueErr, err)
		}
		return nil, issueErr
	case errors.Is(issueErr, domain.ErrInvoiceOutcomeUnknown):
		return nil, domain.WithCode(domain.CodeInvoiceInProgress, fmt.Errorf("%w: %w", domain.ErrConflict, issueErr))
	default:
		if err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
			return s.store.ReleasePending(ctx, q, tenant.AccountID, pending.ID, releasedIssue)
		}); err != nil {
			return nil, errors.Join(issueErr, err)
		}
		return nil, issueErr
	}
}

// reconcile settles a pending invoice whose outcome was never learned. ARCA's newest invoice at
// that point of sale is adopted when it is this one (same buyer, same total, issued since); if
// not, the hold is released and the caller issues afresh.
func (s *InvoiceService) reconcile(
	ctx context.Context, tenant domain.Tenant, creds domain.ARCACredentials, pending domain.Invoice,
) (*domain.Invoice, error) {
	latest, err := s.issuer.LatestAuthorized(ctx, creds, pending.IssuerCUIT, pending.Type, pending.PointOfSale)
	if err != nil {
		return nil, err
	}
	if latest != nil && matchesPending(*latest, pending) {
		var issued *domain.Invoice
		err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
			var err error
			issued, err = s.store.MarkIssued(ctx, q, tenant.AccountID, pending.ID, domain.InvoiceAuthorization{
				Number: latest.Number, CAE: latest.CAE, CAEExpiresOn: latest.CAEExpiresOn,
				Observations: []string{"RECONCILED"},
			})
			return err
		})
		return issued, err
	}
	return nil, s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		return s.store.ReleasePending(ctx, q, tenant.AccountID, pending.ID, "NOT_AUTHORIZED")
	})
}

// matchesPending tells whether ARCA's newest invoice is the pending one: same buyer, same total,
// dated no earlier than the pending one.
func matchesPending(latest domain.AuthorizedInvoice, pending domain.Invoice) bool {
	return latest.DocType == pending.Receiver.DocType &&
		latest.DocNumber == pending.Receiver.DocNumber &&
		latest.Total.Equal(pending.Amounts.Total) &&
		!latest.Date.Before(pending.IssuedOn)
}

// openCredentials reads and unseals the account's certificate and key.
func (s *InvoiceService) openCredentials(
	ctx context.Context, q repository.Querier, accountID uuid.UUID,
) (domain.ARCACredentials, error) {
	certificate, sealed, err := s.store.GetSealedCredential(ctx, q, accountID)
	if err != nil {
		return domain.ARCACredentials{}, err
	}
	key, err := s.sealer.Open(sealed)
	if err != nil {
		return domain.ARCACredentials{}, err
	}
	return domain.ARCACredentials{CertificatePEM: []byte(certificate), PrivateKeyPEM: []byte(key)}, nil
}

func (s *InvoiceService) isStale(pending domain.Invoice) bool {
	return s.now().Sub(pending.UpdatedAt) > pendingStaleCalls*s.settings.RequestTimeout
}

func (s *InvoiceService) today() time.Time {
	now := s.now().In(argentina)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}

// validCUIT checks a CUIT's check digit (modulo 11 over the first ten digits).
func validCUIT(digits string) bool {
	weights := []int{5, 4, 3, 2, 7, 6, 5, 4, 3, 2}
	sum := 0
	for i, w := range weights {
		sum += int(digits[i]-'0') * w
	}
	check := 11 - sum%11
	switch check {
	case 11:
		check = 0
	case 10:
		check = 9
	}
	return int(digits[10]-'0') == check
}

func trimmedOrNil(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func nonNilIssues(issues []string) []string {
	if issues == nil {
		return []string{}
	}
	return issues
}

func mustLoadLocation(name string) *time.Location {
	location, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return location
}
