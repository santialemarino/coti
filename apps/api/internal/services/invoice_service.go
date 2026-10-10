package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type invoicingStore interface {
	SaveProfile(context.Context, repository.Querier, uuid.UUID, domain.InvoiceIssuerProfile) error
	GetSnapshot(ctx context.Context, q repository.Querier, accountID, branchID, quoteID, versionID uuid.UUID) (domain.QuoteRepresentationPayload, error)
	ClaimRecovery(ctx context.Context, q repository.Querier, accountID, branchID, id uuid.UUID, before time.Time) (bool, error)
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
	GetLatestByQuote(ctx context.Context, q repository.Querier, accountID, branchID, quoteID uuid.UUID) (*domain.Invoice, error)
	CreatePending(ctx context.Context, q repository.Querier, in domain.Invoice) (*domain.Invoice, error)
	MarkIssued(ctx context.Context, q repository.Querier, accountID, branchID, id uuid.UUID, auth domain.InvoiceAuthorization) (*domain.Invoice, error)
	MarkRejected(ctx context.Context, q repository.Querier, accountID, branchID, id uuid.UUID, rejection domain.InvoiceRejectedError) error
	ReleasePending(ctx context.Context, q repository.Querier, accountID, branchID, id uuid.UUID, reason string) error
	ClaimNumber(ctx context.Context, q repository.Querier, accountID, branchID, id uuid.UUID, number int64) error
	GetSealedTicket(ctx context.Context, q repository.Querier, accountID uuid.UUID) (*domain.SealedARCATicket, error)
	SaveSealedTicket(ctx context.Context, q repository.Querier, accountID uuid.UUID, ticket *domain.SealedARCATicket) error
}

type invoiceQuoteReader interface {
	GetByIDForUpdate(ctx context.Context, q repository.Querier, accountID, branchID, id uuid.UUID) (*domain.Quote, error)
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

// InvoiceSettings is what the invoice service is configured with.
type InvoiceSettings struct {
	Enabled         bool
	Environment     string
	IssueTimeout    time.Duration // bounds one issue end to end, detached from the caller's request.
	ReconcileAfter  time.Duration // a pending invoice this old is checked against ARCA.
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
	settings  InvoiceSettings
	now       func() time.Time
	renderer  domain.InvoicePDFRenderer
}

// NewInvoiceService builds the invoicing use cases.
func NewInvoiceService(
	db tenantTxRunner, store invoicingStore, quotes invoiceQuoteReader, discounts invoiceDiscountReader,
	issuer domain.InvoiceIssuer, sealer keySealer, settings InvoiceSettings,
	now func() time.Time, renderers ...domain.InvoicePDFRenderer,
) *InvoiceService {
	if now == nil {
		now = time.Now
	}
	var renderer domain.InvoicePDFRenderer
	if len(renderers) != 0 {
		renderer = renderers[0]
	}
	return &InvoiceService{renderer: renderer, db: db, store: store, quotes: quotes, discounts: discounts, issuer: issuer,
		sealer: sealer, settings: settings, now: now}
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

// UpdateSettings replaces the issuer profile, IVA condition and catalog price policy.
func (s *InvoiceService) UpdateSettings(
	ctx context.Context, tenant domain.Tenant, in domain.InvoicingSettingsUpdate,
) (*domain.InvoicingSettings, error) {
	if in.IVACondition != nil && (!in.IVACondition.Valid() || *in.IVACondition == domain.IVAConditionFinalConsumer) {
		return nil, fmt.Errorf("%w: unknown IVA condition %q", domain.ErrInvalidInput, *in.IVACondition)
	}
	if in.Profile != nil {
		in.Profile.Address = strings.TrimSpace(in.Profile.Address)
		in.Profile.GrossIncomeRegistration = strings.TrimSpace(in.Profile.GrossIncomeRegistration)
		day, err := time.Parse("2006-01-02", in.Profile.ActivityStartedOn)
		if err != nil || day.After(domain.InvoiceDay(s.now())) || in.Profile.Address == "" || in.Profile.GrossIncomeRegistration == "" {
			return nil, domain.ErrInvalidInput
		}
	}
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		if in.Profile != nil {
			if err := s.store.SaveProfile(ctx, q, tenant.AccountID, *in.Profile); err != nil {
				return err
			}
		}
		if err := s.store.UpdateAccountFiscal(ctx, q, tenant.AccountID, in.IVACondition, in.PricesIncludeVAT); err != nil {
			return err
		}
		err := s.store.UpdateBranchPointsOfSale(ctx, q, tenant.AccountID, in.PointsOfSale)
		if errors.Is(err, domain.ErrConflict) {
			return domain.WithCode(domain.CodePointOfSaleTaken, err)
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return s.GetSettings(ctx, tenant)
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
	in.Address = trimmedOrNil(in.Address)
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

// Issue asks ARCA to authorize the invoice the seller confirmed for an accepted quote. A pending
// invoice holds the quote while ARCA answers, so a second press can never authorize a second one.
func (s *InvoiceService) Issue(
	ctx context.Context, tenant domain.Tenant, quoteID uuid.UUID, expected domain.InvoiceExpectation,
) (*domain.Invoice, error) {
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
		if draft.preview.Fingerprint != expected.Fingerprint || draft.version.ID != expected.VersionID || draft.preview.Type != expected.Type ||
			!draft.preview.Amounts.Total.Equal(expected.Total) {
			return domain.WithCode(domain.CodeInvoiceStale, domain.ErrConflict)
		}
		creds, err = s.openCredentials(ctx, q, tenant)
		return err
	})
	if err != nil {
		return nil, err
	}

	// Once ARCA is being asked, the seller closing the tab must not cut the exchange short: an
	// invoice authorized but never recorded is the one outcome nothing can repair.
	detached := context.WithoutCancel(ctx)
	work, cancel := context.WithTimeout(detached, s.settings.IssueTimeout)
	defer cancel()

	if stale != nil {
		claimed := false
		err := s.db.InTenantTx(work, tenant, func(q repository.Querier) error {
			var err error
			claimed, err = s.store.ClaimRecovery(work, q, tenant.AccountID, tenant.BranchID, stale.ID, s.now().Add(-s.settings.ReconcileAfter))
			return err
		})
		if err != nil {
			return nil, err
		}
		if !claimed {
			return nil, domain.WithCode(domain.CodeInvoiceInProgress, domain.ErrConflict)
		}
		adopted, err := s.reconcile(work, tenant, creds, *stale)
		if err != nil || adopted != nil {
			return adopted, err
		}
	}

	pending, err := s.hold(work, tenant, *draft)
	if err != nil {
		return nil, err
	}

	auth, issueErr := s.issuer.Issue(work, creds, domain.InvoiceRequest{
		IssuerCUIT:  pending.IssuerCUIT,
		Type:        pending.Type,
		PointOfSale: pending.PointOfSale,
		Date:        pending.IssuedOn,
		Receiver:    pending.Receiver,
		Amounts:     pending.Amounts,
		Claim: func(ctx context.Context, number int64) error {
			return s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
				return s.store.ClaimNumber(ctx, q, tenant.AccountID, tenant.BranchID, pending.ID, number)
			})
		},
	})
	return s.settle(detached, tenant, *pending, auth, issueErr)
}

// invoiceDraft is a preview with what issuing it needs on top.
type invoiceDraft struct {
	preview          domain.InvoicePreview
	quote            domain.Quote
	version          domain.QuoteVersion
	issuerCUIT       string
	issuerCondition  domain.IVACondition
	profile          domain.InvoiceIssuerProfile
	issuerName       string
	pricesIncludeVAT bool
	items            []domain.QuoteItem
	rates            map[uuid.UUID]domain.VATRate
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
	existing, err := s.store.GetLatestByQuote(ctx, q, tenant.AccountID, tenant.BranchID, quoteID)
	if err != nil {
		return nil, err
	}

	issuerCondition := domain.IVAConditionRegistered
	if account.IVACondition != nil {
		issuerCondition = *account.IVACondition
	}
	receiver := invoiceReceiver(client, "")
	invoiceType := invoiceTypeFor(issuerCondition, receiver.IVACondition)
	byRate := grossByRate(items, rates, discounts)
	amounts := invoiceAmounts(byRate, invoiceType, account.PricesIncludeVAT)
	issues := invoiceIssues(invoiceInputs{
		enabled: s.settings.Enabled, quote: *quote, version: *version, gross: sumGross(byRate), account: *account,
		pointOfSale: pointOfSale, credential: credential, receiver: receiver, amounts: amounts,
		invoiceType: invoiceType, unidentifiedMax: s.settings.UnidentifiedMax, now: s.now(),
	})

	draft := &invoiceDraft{
		quote: *quote, version: *version, issuerCUIT: account.CUIT(), issuerCondition: issuerCondition, items: items, rates: rates, pricesIncludeVAT: account.PricesIncludeVAT, profile: account.Profile, issuerName: firstNonEmpty(account.LegalName, &account.Name),
		preview: domain.InvoicePreview{VersionID: version.ID, Type: invoiceType, PointOfSale: pointOfSale, Receiver: receiver,
			Amounts: amounts, Currency: arcaCurrency, Issues: nonNilIssues(issues)},
	}
	// A live invoice is the sale's whatever the version; a refusal only speaks for the version it was for.
	if existing != nil && (existing.Status != domain.InvoiceStatusRejected ||
		(existing.QuoteVersionID == version.ID && !released(*existing))) {
		draft.preview.Invoice = existing
	}
	fingerprintPreview := draft.preview
	fingerprintPreview.Invoice = nil
	fingerprint, err := json.Marshal(struct {
		Account domain.AccountFiscal
		Preview domain.InvoicePreview
	}{*account, fingerprintPreview})
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(fingerprint)
	draft.preview.Fingerprint = hex.EncodeToString(digest[:])
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
		// The quote may have been reopened while ARCA was being reconciled.
		quote, err := s.quotes.GetByIDForUpdate(ctx, q, tenant.AccountID, tenant.BranchID, draft.quote.ID)
		if err != nil {
			return err
		}
		version, err := s.quotes.GetCurrentVersion(ctx, q, tenant.AccountID, tenant.BranchID, draft.quote.ID)
		if err != nil {
			return err
		}
		if quote.CurrentStatus != domain.QuoteStatusAccepted || version.ID != draft.version.ID {
			return domain.WithCode(domain.CodeInvoiceStale, domain.ErrConflict)
		}
		current, err := s.draft(ctx, q, tenant, draft.quote.ID)
		if err != nil {
			return err
		}
		if current.preview.Fingerprint != draft.preview.Fingerprint {
			return domain.WithCode(domain.CodeInvoiceStale, domain.ErrConflict)
		}
		snapshot, err := s.store.GetSnapshot(ctx, q, tenant.AccountID, tenant.BranchID, draft.quote.ID, draft.version.ID)
		if err != nil {
			return err
		}
		snapshot, err = fiscalSnapshot(snapshot, draft)
		if err != nil {
			return err
		}
		snapshot.Customer.Address = &draft.preview.Receiver.Address
		snapshot.Supplier.Name = draft.issuerName
		snapshot.Supplier.LegalName = &draft.issuerName
		snapshot.Supplier.TaxID = &draft.issuerCUIT
		pending, err = s.store.CreatePending(ctx, q, domain.Invoice{
			Snapshot: snapshot, IssuerCondition: draft.issuerCondition, IssuerProfile: draft.profile, AccountID: tenant.AccountID, BranchID: draft.quote.BranchID, QuoteID: draft.quote.ID,
			QuoteVersionID: draft.version.ID, Type: draft.preview.Type, PointOfSale: *draft.preview.PointOfSale,
			IssuedOn: domain.InvoiceDay(s.now()), IssuerCUIT: draft.issuerCUIT, Receiver: draft.preview.Receiver,
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
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.settings.IssueTimeout)
	defer cancel()
	if issueErr == nil && auth == nil {
		issueErr = domain.ErrInvoiceOutcomeUnknown
	}
	var rejected *domain.InvoiceRejectedError
	switch {
	case issueErr == nil:
		var issued *domain.Invoice
		err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
			var err error
			issued, err = s.store.MarkIssued(ctx, q, tenant.AccountID, tenant.BranchID, pending.ID, *auth)
			return err
		})
		return issued, err
	case errors.As(issueErr, &rejected):
		if err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
			return s.store.MarkRejected(ctx, q, tenant.AccountID, tenant.BranchID, pending.ID, *rejected)
		}); err != nil {
			return nil, errors.Join(issueErr, err)
		}
		return nil, issueErr
	case errors.Is(issueErr, domain.ErrInvoiceOutcomeUnknown):
		return nil, domain.WithCode(domain.CodeInvoiceInProgress, fmt.Errorf("%w: %w", domain.ErrConflict, issueErr))
	default:
		if err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
			return s.store.ReleasePending(ctx, q, tenant.AccountID, tenant.BranchID, pending.ID, releasedIssue)
		}); err != nil {
			return nil, errors.Join(issueErr, err)
		}
		return nil, issueErr
	}
}

// reconcile reads back the number a pending invoice of unknown outcome claimed: ARCA's invoice there
// is adopted when it is this one, otherwise nothing was authorized and the hold is released.
func (s *InvoiceService) reconcile(
	ctx context.Context, tenant domain.Tenant, creds domain.ARCACredentials, pending domain.Invoice,
) (*domain.Invoice, error) {
	var found *domain.AuthorizedInvoice
	if pending.ClaimedNumber != nil {
		var err error
		found, err = s.issuer.Authorized(ctx, creds, pending.IssuerCUIT, pending.Type, pending.PointOfSale,
			*pending.ClaimedNumber)
		if err != nil {
			return nil, err
		}
	}
	if found != nil && matchesPending(*found, pending) {
		var issued *domain.Invoice
		err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
			var err error
			issued, err = s.store.MarkIssued(ctx, q, tenant.AccountID, tenant.BranchID, pending.ID, domain.InvoiceAuthorization{
				Number: found.Number, CAE: found.CAE, CAEExpiresOn: found.CAEExpiresOn,
			})
			return err
		})
		return issued, err
	}
	if found != nil {
		return nil, domain.WithCode(domain.CodeInvoiceInProgress, domain.ErrConflict)
	}
	return nil, s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		return s.store.ReleasePending(ctx, q, tenant.AccountID, tenant.BranchID, pending.ID, releasedIssue)
	})
}

// matchesPending tells whether ARCA's invoice under the claimed number is the pending one.
func matchesPending(found domain.AuthorizedInvoice, pending domain.Invoice) bool {
	return found.DocType == pending.Receiver.DocType &&
		found.DocNumber == pending.Receiver.DocNumber &&
		found.Total.Equal(pending.Amounts.Total) && found.Date.Equal(pending.IssuedOn)
}

// openCredentials reads and unseals the account's certificate and key. A key that no longer
// opens, sealed under another encryption key, is a credential to upload again.
func (s *InvoiceService) openCredentials(
	ctx context.Context, q repository.Querier, tenant domain.Tenant,
) (domain.ARCACredentials, error) {
	certificate, sealed, err := s.store.GetSealedCredential(ctx, q, tenant.AccountID)
	if err != nil {
		return domain.ARCACredentials{}, err
	}
	key, err := s.sealer.Open(sealed)
	if err != nil {
		return domain.ARCACredentials{}, &domain.InvoiceNotReadyError{Issues: []string{invoiceIssueCredentials}}
	}
	key, bound := strings.CutPrefix(key, tenant.AccountID.String()+":")
	if !bound {
		return domain.ARCACredentials{}, domain.ErrNotConfigured
	}
	return domain.ARCACredentials{
		CertificatePEM: []byte(certificate),
		PrivateKeyPEM:  []byte(key),
		Tickets:        &invoiceTicketStore{service: s, tenant: tenant},
	}, nil
}

// invoiceTicketStore keeps an account's WSAA ticket on its credential row, sealed like the key.
type invoiceTicketStore struct {
	service *InvoiceService
	tenant  domain.Tenant
}

// Load returns the stored ticket, nil when there is none or it no longer opens.
func (t *invoiceTicketStore) Load(ctx context.Context) (*domain.ARCATicket, error) {
	var sealed *domain.SealedARCATicket
	err := t.service.db.InTenantTx(ctx, t.tenant, func(q repository.Querier) error {
		var err error
		sealed, err = t.service.store.GetSealedTicket(ctx, q, t.tenant.AccountID)
		return err
	})
	if err != nil || sealed == nil {
		return nil, err
	}
	raw, err := t.service.sealer.Open(sealed.Token)
	if err != nil {
		return nil, err
	}
	raw, bound := strings.CutPrefix(raw, t.tenant.AccountID.String()+":")
	if !bound {
		return nil, domain.ErrNotConfigured
	}
	var ticket domain.ARCATicket
	if err := json.Unmarshal([]byte(raw), &ticket); err != nil {
		return nil, err
	}
	return &ticket, nil
}

// Save stores the ticket; an empty one clears it.
func (t *invoiceTicketStore) Save(ctx context.Context, ticket domain.ARCATicket) error {
	var sealed *domain.SealedARCATicket
	if ticket.Token != "" {
		raw, err := json.Marshal(ticket)
		if err != nil {
			return err
		}
		value, err := t.service.sealer.Seal(t.tenant.AccountID.String() + ":" + string(raw))
		if err != nil {
			return err
		}
		sealed = &domain.SealedARCATicket{Token: value}
	}
	return t.service.db.InTenantTx(ctx, t.tenant, func(q repository.Querier) error {
		return t.service.store.SaveSealedTicket(ctx, q, t.tenant.AccountID, sealed)
	})
}

func (s *InvoiceService) isStale(pending domain.Invoice) bool {
	return s.now().Sub(pending.UpdatedAt) > s.settings.ReconcileAfter
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
