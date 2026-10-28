package services

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

const (
	quotePDFContentType    = "application/pdf"
	quoteDefaultBrandColor = "#1F2937"
	quoteValidityNote      = "Consultá la vigencia actual en el enlace de la cotización."
)

type quoteRepresentationRepository interface {
	GetByVersion(ctx context.Context, q repository.Querier, accountID, branchID, quoteID,
		versionID uuid.UUID) (*domain.QuoteRepresentation, error)
	GetByVersionID(ctx context.Context, q repository.Querier, accountID,
		versionID uuid.UUID) (*domain.QuoteRepresentation, error)
	Create(ctx context.Context, q repository.Querier, accountID uuid.UUID,
		in domain.NewQuoteRepresentation) (*domain.QuoteRepresentation, error)
	LoadSource(ctx context.Context, q repository.Querier, accountID, branchID, quoteID,
		versionID uuid.UUID) (*domain.QuoteRepresentationSource, error)
}

type quoteRepresentationQuoteRepository interface {
	GetByIDForUpdate(ctx context.Context, q repository.Querier, accountID, branchID,
		id uuid.UUID) (*domain.Quote, error)
	GetCurrentVersion(ctx context.Context, q repository.Querier, accountID, branchID,
		quoteID uuid.UUID) (*domain.QuoteVersion, error)
	FreezeVersion(ctx context.Context, q repository.Querier, accountID, branchID, quoteID,
		versionID uuid.UUID) (*domain.QuoteVersion, error)
}

// QuoteRepresentationService freezes, validates, renders and persists one bundle per version.
type QuoteRepresentationService struct {
	db              quotePublicDB
	representations quoteRepresentationRepository
	quotes          quoteRepresentationQuoteRepository
	storage         domain.ObjectStorage
	logos           domain.BrandLogoLoader
	renderer        domain.QuotePDFRenderer
	signedURLExpiry time.Duration
	now             func() time.Time
	log             *slog.Logger
	// settler records the seller's decision on the handler runs a frozen draft carries.
	settler handlerDecisionSettler
	pricing quoteSelectionPricingReader
}

type handlerDecisionSettler interface {
	Settle(ctx context.Context, q repository.Querier, tenant domain.Tenant, versionID uuid.UUID) error
}

type quoteSelectionPricingReader interface {
	ListInForce(ctx context.Context, q repository.Querier, accountID, branchID uuid.UUID,
		at time.Time) ([]domain.Promotion, error)
	ListProductGroups(ctx context.Context, q repository.Querier, accountID uuid.UUID,
		productIDs []uuid.UUID) (map[uuid.UUID]domain.ProductGroup, error)
}

// NewQuoteRepresentationService builds the representation orchestrator.
func NewQuoteRepresentationService(
	db quotePublicDB, representations quoteRepresentationRepository,
	quotes quoteRepresentationQuoteRepository, storage domain.ObjectStorage,
	logos domain.BrandLogoLoader, renderer domain.QuotePDFRenderer, signedURLExpiry time.Duration,
	now func() time.Time, log *slog.Logger,
) *QuoteRepresentationService {
	if now == nil {
		now = time.Now
	}
	if log == nil {
		log = slog.Default()
	}
	return &QuoteRepresentationService{db: db, representations: representations, quotes: quotes,
		storage: storage, logos: logos, renderer: renderer, signedURLExpiry: signedURLExpiry,
		now: now, log: log}
}

// WithHandlerDecisions settles the conversation handler's runs on a draft when it is frozen.
func (s *QuoteRepresentationService) WithHandlerDecisions(settler handlerDecisionSettler,
) *QuoteRepresentationService {
	s.settler = settler
	return s
}

// WithSelectionPricing freezes the promotion rules and taxonomy client-selectable offers need.
func (s *QuoteRepresentationService) WithSelectionPricing(
	pricing quoteSelectionPricingReader,
) *QuoteRepresentationService {
	s.pricing = pricing
	return s
}

// Ensure returns the existing representation or creates it after freezing the current version.
func (s *QuoteRepresentationService) Ensure(
	ctx context.Context, tenant domain.Tenant, quoteID uuid.UUID,
) (*domain.QuoteRepresentationResult, error) {
	if err := requireBranch(tenant, "a quote representation"); err != nil {
		return nil, err
	}
	if s.storage == nil || s.renderer == nil {
		return nil, domain.ErrNotConfigured
	}
	var result *domain.QuoteRepresentationResult
	err := s.db.WithAdvisoryLock(ctx, fmt.Sprintf("quote-representation:%s:%s",
		tenant.AccountID, quoteID), func() error {
		var ensureErr error
		result, ensureErr = s.ensureLocked(ctx, tenant, quoteID)
		return ensureErr
	})
	return result, err
}

// ResolvePublic returns snapshot data and a PDF URL capped by the delivery deadline.
func (s *QuoteRepresentationService) ResolvePublic(
	ctx context.Context, accountID, versionID uuid.UUID, expiresAt time.Time, publicURL string,
) (*domain.PublicQuoteRepresentation, error) {
	result := &domain.PublicQuoteRepresentation{Status: "EXPIRED", ExpiresAt: expiresAt}
	if !s.now().Before(expiresAt) {
		return result, nil
	}
	var representation *domain.QuoteRepresentation
	err := s.db.InTenantTx(ctx, domain.Tenant{AccountID: accountID},
		func(q repository.Querier) error {
			var getErr error
			representation, getErr = s.representations.GetByVersionID(ctx, q, accountID, versionID)
			return getErr
		})
	if err != nil {
		return nil, err
	}
	lifetime := s.signedURLExpiry
	if remaining := expiresAt.Sub(s.now()); remaining < lifetime {
		lifetime = remaining
	}
	lifetime = lifetime.Truncate(time.Second)
	if lifetime <= 0 {
		return result, nil
	}
	pdfURL, err := s.storage.GenerateSignedURL(ctx, representation.PDFStorageKey, lifetime)
	if err != nil {
		return nil, fmt.Errorf("%w: sign public PDF URL: %v", domain.ErrRepresentationUnavailable, err)
	}
	message := strings.ReplaceAll(representation.Message, domain.QuotePublicURLPlaceholder, publicURL)
	result.Status = "ACTIVE"
	result.Payload = &representation.Payload
	result.Message = &message
	result.PDFURL = &pdfURL
	return result, nil
}

func (s *QuoteRepresentationService) ensureLocked(
	ctx context.Context, tenant domain.Tenant, quoteID uuid.UUID,
) (*domain.QuoteRepresentationResult, error) {
	var source *domain.QuoteRepresentationSource
	var existing *domain.QuoteRepresentation
	var selectionPricing *domain.QuoteSelectionPricingSnapshot
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		quote, err := s.quotes.GetByIDForUpdate(ctx, q, tenant.AccountID, tenant.BranchID, quoteID)
		if err != nil {
			return err
		}
		if quote.CurrentVersionID == nil {
			return invalidRepresentation("quote has no current version")
		}
		if err := requireUnarchived(*quote); err != nil {
			return err
		}
		if quote.CurrentStatus != domain.QuoteStatusQuoted &&
			quote.CurrentStatus != domain.QuoteStatusSent {
			return domain.WithCode(domain.CodeQuoteNotSendable, domain.ErrConflict)
		}
		version, err := s.quotes.GetCurrentVersion(ctx, q, tenant.AccountID, tenant.BranchID, quoteID)
		if err != nil {
			return err
		}
		existing, err = s.representations.GetByVersion(ctx, q, tenant.AccountID, tenant.BranchID,
			quoteID, version.ID)
		if err == nil {
			return nil
		}
		if !errors.Is(err, domain.ErrNotFound) {
			return err
		}
		if quote.CurrentStatus != domain.QuoteStatusQuoted {
			return domain.WithCode(domain.CodeQuoteNotSendable, domain.ErrConflict)
		}
		source, err = s.representations.LoadSource(ctx, q, tenant.AccountID, tenant.BranchID,
			quoteID, version.ID)
		if err != nil {
			return err
		}
		if err := validateRepresentationSource(*source); err != nil {
			return err
		}
		if strings.Count(buildQuoteMessage(buildRepresentationPayload(*source, s.now())),
			domain.QuotePublicURLPlaceholder) != 1 {
			return invalidRepresentation("commercial text contains a reserved URL marker")
		}
		selectionPricing, err = s.captureSelectionPricing(ctx, q, tenant, *source)
		if err != nil {
			return err
		}
		frozen, err := s.quotes.FreezeVersion(ctx, q, tenant.AccountID, tenant.BranchID,
			quoteID, version.ID)
		if err != nil {
			return err
		}
		source.Version = *frozen
		if s.settler == nil {
			return nil
		}
		return s.settler.Settle(ctx, q, tenant, frozen.ID)
	})
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return s.decorate(ctx, *existing, true)
	}

	if source.Version.FrozenAt == nil {
		return nil, invalidRepresentation("frozen version has no approval timestamp")
	}
	payload := buildRepresentationPayload(*source, *source.Version.FrozenAt)
	message := buildQuoteMessage(payload)
	logo, fallback := s.loadLogo(ctx, source.Account.BrandLogoURL, quoteID)
	pdfBytes, err := s.renderer.Render(payload, logo)
	if err != nil {
		return nil, fmt.Errorf("%w: render PDF: %v", domain.ErrRepresentationUnavailable, err)
	}
	if len(pdfBytes) == 0 {
		return nil, fmt.Errorf("%w: renderer returned an empty PDF", domain.ErrRepresentationUnavailable)
	}
	key := fmt.Sprintf("accounts/%s/quotes/%s/versions/%s/quote.pdf", tenant.AccountID,
		quoteID, source.Version.ID)
	if err := s.storage.Upload(ctx, key, quotePDFContentType, bytes.NewReader(pdfBytes)); err != nil {
		return nil, fmt.Errorf("%w: upload PDF: %v", domain.ErrRepresentationUnavailable, err)
	}
	checksum := sha256.Sum256(pdfBytes)
	var created *domain.QuoteRepresentation
	err = s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var createErr error
		created, createErr = s.representations.Create(ctx, q, tenant.AccountID,
			domain.NewQuoteRepresentation{ID: uuid.New(), BranchID: tenant.BranchID,
				QuoteID: quoteID, VersionID: source.Version.ID,
				SchemaVersion: domain.QuoteRepresentationSchemaVersion, Payload: payload,
				Message: message, PDFStorageKey: key, PDFContentType: quotePDFContentType,
				PDFSizeBytes: int64(len(pdfBytes)), PDFSHA256: hex.EncodeToString(checksum[:]),
				LogoFallbackUsed: fallback, SelectionPricing: selectionPricing})
		return createErr
	})
	if err != nil {
		return nil, err
	}
	return s.decorate(ctx, *created, false)
}

func (s *QuoteRepresentationService) captureSelectionPricing(
	ctx context.Context, q repository.Querier, tenant domain.Tenant, source domain.QuoteRepresentationSource,
) (*domain.QuoteSelectionPricingSnapshot, error) {
	if s.pricing == nil {
		return nil, nil
	}
	productIDs := make([]uuid.UUID, 0, len(source.Items))
	seen := make(map[uuid.UUID]struct{})
	addProduct := func(productID *uuid.UUID) {
		if productID == nil {
			return
		}
		if _, exists := seen[*productID]; exists {
			return
		}
		seen[*productID] = struct{}{}
		productIDs = append(productIDs, *productID)
	}
	for _, item := range source.Items {
		addProduct(item.ProductID)
		for _, alternative := range source.Alternatives[item.ID] {
			if alternative.ApprovedBySeller && alternative.Type == domain.QuoteItemAlternativeTypeProduct &&
				alternative.PriceSnapshot.Valid {
				addProduct(alternative.ProductID)
			}
		}
	}
	capturedAt := s.now().UTC()
	promotions, err := s.pricing.ListInForce(ctx, q, tenant.AccountID, tenant.BranchID, capturedAt)
	if err != nil {
		return nil, err
	}
	groups, err := s.pricing.ListProductGroups(ctx, q, tenant.AccountID, productIDs)
	if err != nil {
		return nil, err
	}
	productGroups := make([]domain.QuoteSelectionProductGroup, 0, len(groups))
	for productID, group := range groups {
		productGroups = append(productGroups, domain.QuoteSelectionProductGroup{
			ProductID: productID, Group: group,
		})
	}
	sort.Slice(productGroups, func(i, j int) bool {
		return strings.Compare(productGroups[i].ProductID.String(), productGroups[j].ProductID.String()) < 0
	})
	return &domain.QuoteSelectionPricingSnapshot{CapturedAt: capturedAt, Promotions: promotions,
		ProductGroups: productGroups}, nil
}

func (s *QuoteRepresentationService) decorate(
	ctx context.Context, representation domain.QuoteRepresentation, replay bool,
) (*domain.QuoteRepresentationResult, error) {
	url, err := s.storage.GenerateSignedURL(ctx, representation.PDFStorageKey, s.signedURLExpiry)
	if err != nil {
		return nil, fmt.Errorf("%w: sign PDF URL: %v", domain.ErrRepresentationUnavailable, err)
	}
	return &domain.QuoteRepresentationResult{Representation: representation, PDFURL: url,
		Replay: replay}, nil
}

func (s *QuoteRepresentationService) loadLogo(
	ctx context.Context, rawURL *string, quoteID uuid.UUID,
) (*domain.BrandLogo, bool) {
	if rawURL == nil || strings.TrimSpace(*rawURL) == "" || s.logos == nil {
		return nil, true
	}
	logo, err := s.logos.Load(ctx, *rawURL)
	if err != nil {
		s.log.WarnContext(ctx, "quote logo unavailable; using supplier name",
			slog.String("quote_id", quoteID.String()), slog.Any("error", err))
		return nil, true
	}
	return logo, false
}

func validateRepresentationSource(source domain.QuoteRepresentationSource) error {
	unresolved := 0
	for _, item := range source.Items {
		if item.MatchStatus != domain.ItemMatchStatusMatched || item.ProductID == nil ||
			!item.UnitPriceSnapshot.Valid {
			unresolved++
		}
	}
	if unresolved > 0 {
		return domain.WithCode(domain.CodeQuoteItemsUnresolved, fmt.Errorf(
			"%w: %d lines need a confirmed, priced product", domain.ErrInvalidInput, unresolved))
	}
	var issues []string
	if len(source.Items) == 0 {
		issues = append(issues, "quote has no items")
	}
	subtotals := decimal.Zero
	for i, item := range source.Items {
		if item.MatchStatus != domain.ItemMatchStatusMatched {
			issues = append(issues, fmt.Sprintf("items[%d] is unresolved", i))
		}
		if item.ProductID == nil || item.ProductName == nil || strings.TrimSpace(*item.ProductName) == "" {
			issues = append(issues, fmt.Sprintf("items[%d] has no catalog product", i))
		}
		if !item.Quantity.IsPositive() || !item.UnitPriceSnapshot.Valid || !item.Subtotal.Valid {
			issues = append(issues, fmt.Sprintf("items[%d] is not fully priced", i))
		}
		for _, amount := range []decimal.Decimal{item.Quantity, item.UnitPriceSnapshot.Decimal, item.Subtotal.Decimal} {
			if amount.IsNegative() || validateAmount(amount, "item amount") != nil {
				issues = append(issues, fmt.Sprintf("items[%d] has an invalid amount", i))
			}
		}
		expected := item.Quantity.Mul(item.UnitPriceSnapshot.Decimal).Round(domain.MoneyScale)
		if !expected.Equal(item.Subtotal.Decimal) {
			issues = append(issues, fmt.Sprintf("items[%d] subtotal is inconsistent", i))
		}
		subtotals = subtotals.Add(item.Subtotal.Decimal)
		for _, alternative := range source.Alternatives[item.ID] {
			if !alternative.ApprovedBySeller || alternative.PriceSnapshot.Decimal.IsNegative() ||
				validateAmount(alternative.PriceSnapshot.Decimal, "alternative price") != nil {
				issues = append(issues, fmt.Sprintf("items[%d] has an invalid alternative", i))
			}
			if !alternative.PriceSnapshot.Valid || alternative.CanonicalName == nil ||
				strings.TrimSpace(*alternative.CanonicalName) == "" {
				issues = append(issues, fmt.Sprintf("items[%d] has an incomplete approved alternative", i))
			}
		}
	}
	discounts := decimal.Zero
	for i, discount := range source.Discounts {
		amount, err := decimal.NewFromString(discount.Amount)
		if err != nil || amount.IsNegative() {
			issues = append(issues, fmt.Sprintf("discounts[%d] has an invalid amount", i))
		}
		discounts = discounts.Add(amount)
	}
	expectedTotal := subtotals.Sub(discounts).Round(domain.MoneyScale)
	if expectedTotal.IsNegative() || !expectedTotal.Equal(source.Version.Total) {
		issues = append(issues, "quote total is inconsistent")
	}
	if !regexp.MustCompile(`^[A-Z]{3}$`).MatchString(source.Version.Currency) {
		issues = append(issues, "quote currency is invalid")
	}
	if len(issues) > 0 {
		return domain.WithCode(domain.CodeQuoteRepresentationInvalid,
			&domain.QuoteRepresentationValidationError{Issues: issues})
	}
	return nil
}

func buildRepresentationPayload(
	source domain.QuoteRepresentationSource, approvedAt time.Time,
) domain.QuoteRepresentationPayload {
	color := quoteDefaultBrandColor
	if source.Account.BrandColor != nil && regexp.MustCompile(`^#[0-9a-fA-F]{6}$`).MatchString(*source.Account.BrandColor) {
		color = strings.ToUpper(strings.TrimSpace(*source.Account.BrandColor))
	}
	var logoURL *string
	if source.Account.BrandLogoURL != nil && strings.TrimSpace(*source.Account.BrandLogoURL) != "" {
		trimmed := strings.TrimSpace(*source.Account.BrandLogoURL)
		logoURL = &trimmed
	}
	items := make([]domain.QuoteRepresentationItem, 0, len(source.Items))
	for _, item := range source.Items {
		alternatives := make([]domain.QuoteRepresentationAlternative, 0,
			len(source.Alternatives[item.ID]))
		for _, alternative := range source.Alternatives[item.ID] {
			alternatives = append(alternatives, domain.QuoteRepresentationAlternative{
				ID: alternative.ID, Code: alternative.Code, Name: *alternative.CanonicalName,
				Unit:               alternative.Unit,
				UnitPrice:          alternative.PriceSnapshot.Decimal.StringFixed(domain.MoneyScale),
				RecommendationType: alternative.RecommendationType,
				Selectable: alternative.Type == domain.QuoteItemAlternativeTypeProduct &&
					alternative.ProductID != nil && alternative.PriceSnapshot.Valid})
		}
		unit := item.Unit
		if unit == nil {
			unit = item.ProductUnit
		}
		items = append(items, domain.QuoteRepresentationItem{
			ID: item.ID, RequestedDescription: item.RequestedDescription, ProductCode: item.ProductCode,
			ProductName: *item.ProductName, Quantity: item.Quantity.StringFixed(domain.MoneyScale),
			Unit: unit, UnitPrice: item.UnitPriceSnapshot.Decimal.StringFixed(domain.MoneyScale),
			Subtotal: item.Subtotal.Decimal.StringFixed(domain.MoneyScale), Alternatives: alternatives})
	}
	return domain.QuoteRepresentationPayload{
		Reference:     domain.QuoteReference(source.Quote.Number),
		VersionNumber: source.Version.VersionNumber, ApprovedAt: approvedAt,
		Currency: source.Version.Currency,
		Supplier: domain.QuoteRepresentationSupplier{Name: source.Account.Name,
			LegalName: source.Account.LegalName, TaxID: source.Account.TaxID, BrandColor: color,
			LogoURL: logoURL},
		Branch: domain.QuoteRepresentationBranch{Name: source.Branch.Name,
			Address: source.Branch.Address},
		Customer: domain.QuoteRepresentationCustomer{Name: source.CustomerName},
		Items:    items, Discounts: source.Discounts,
		Total: source.Version.Total.StringFixed(domain.MoneyScale), ValidityNote: quoteValidityNote,
	}
}

func buildQuoteMessage(payload domain.QuoteRepresentationPayload) string {
	greeting := "Hola."
	if payload.Customer.Name != nil && strings.TrimSpace(*payload.Customer.Name) != "" {
		greeting = "Hola, " + strings.TrimSpace(*payload.Customer.Name) + "."
	}
	return fmt.Sprintf("%s\n\n%s preparó la cotización %s.\nTotal: %s\n\n"+
		"Podés consultar el detalle, las alternativas aprobadas y la vigencia acá:\n%s",
		greeting, payload.Supplier.Name, payload.Reference,
		formatCommercialMoney(payload.Currency, payload.Total), domain.QuotePublicURLPlaceholder)
}

func formatCommercialMoney(currency, raw string) string {
	parts := strings.SplitN(raw, ".", 2)
	integer := parts[0]
	for i := len(integer) - 3; i > 0; i -= 3 {
		integer = integer[:i] + "." + integer[i:]
	}
	fraction := "00"
	if len(parts) == 2 {
		fraction = parts[1]
	}
	return currency + " " + integer + "," + fraction
}

func invalidRepresentation(detail string) error {
	return domain.WithCode(domain.CodeQuoteRepresentationInvalid,
		fmt.Errorf("%w: %s", domain.ErrInvalidInput, detail))
}
