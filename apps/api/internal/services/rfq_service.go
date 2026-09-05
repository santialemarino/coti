package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// rfqRepository is the RFQ persistence surface the service needs.
type rfqRepository interface {
	Create(ctx context.Context, q repository.Querier, accountID uuid.UUID, in domain.NewRFQ) (*domain.RFQ, error)
	UpdateStatus(ctx context.Context, q repository.Querier, accountID, id uuid.UUID, status domain.RFQStatus) (*domain.RFQ, error)
	AppendStatusChange(ctx context.Context, q repository.Querier, accountID, rfqID uuid.UUID, previousStatus *domain.RFQStatus, newStatus domain.RFQStatus, userID *uuid.UUID) (*domain.RFQStatusChange, error)
	ListByTenant(ctx context.Context, q repository.Querier, tenant domain.Tenant) ([]domain.RfqListItem, error)
	GetByRFQID(ctx context.Context, q repository.Querier, accountID, rfqID uuid.UUID) (*domain.RfqListItem, error)
	GetManualEntryChannelID(ctx context.Context, q repository.Querier, accountID, branchID uuid.UUID) (uuid.UUID, error)
	CountProductsInAccount(ctx context.Context, q repository.Querier, accountID uuid.UUID, productIDs []uuid.UUID) (int, error)
	CreateManualEntry(ctx context.Context, q repository.Querier, tenant domain.Tenant, channelID uuid.UUID, in domain.NewRfq, now time.Time) (*domain.RfqCreation, error)
}

// quoteDraftRepository is the quote persistence surface for creating draft versions.
type quoteDraftRepository interface {
	Create(ctx context.Context, q repository.Querier, accountID uuid.UUID, in domain.NewQuote) (*domain.Quote, error)
	UpdateCurrentVersion(ctx context.Context, q repository.Querier, accountID, quoteID, versionID uuid.UUID) (*domain.Quote, error)
	CreateVersion(ctx context.Context, q repository.Querier, accountID uuid.UUID, in domain.NewQuoteVersion) (*domain.QuoteVersion, error)
	UpdateVersionTotal(ctx context.Context, q repository.Querier, accountID, versionID uuid.UUID, total decimal.Decimal) (*domain.QuoteVersion, error)
	CreateItems(ctx context.Context, q repository.Querier, accountID, versionID uuid.UUID, items []domain.NewQuoteItem) ([]domain.QuoteItem, error)
	CreateAlternatives(ctx context.Context, q repository.Querier, accountID uuid.UUID, alternatives []domain.NewQuoteItemAlternative) error
	ListAlternativesByItemIDs(ctx context.Context, q repository.Querier, accountID uuid.UUID, itemIDs []uuid.UUID) (map[uuid.UUID][]domain.QuoteItemAlternative, error)
	AppendStatusChange(ctx context.Context, q repository.Querier, accountID, quoteID uuid.UUID, previousStatus *domain.QuoteStatus, newStatus domain.QuoteStatus, userID *uuid.UUID) (*domain.QuoteStatusChange, error)
	GetByRFQID(ctx context.Context, q repository.Querier, accountID, rfqID uuid.UUID) (*domain.Quote, error)
	GetByID(ctx context.Context, q repository.Querier, accountID, branchID, quoteID uuid.UUID) (*domain.Quote, error)
	GetCurrentVersion(ctx context.Context, q repository.Querier, accountID, branchID, quoteID uuid.UUID) (*domain.QuoteVersion, error)
	ListItems(ctx context.Context, q repository.Querier, accountID, versionID uuid.UUID) ([]domain.QuoteItem, error)
	ListItemsWithProduct(ctx context.Context, q repository.Querier, accountID, versionID uuid.UUID) ([]domain.QuoteItem, error)
	GetItem(ctx context.Context, q repository.Querier, accountID, versionID, itemID uuid.UUID) (*domain.QuoteItem, error)
	UpdateItem(ctx context.Context, q repository.Querier, accountID, versionID, itemID uuid.UUID, in domain.QuoteItemUpdate) (*domain.QuoteItem, error)
	DeleteItem(ctx context.Context, q repository.Querier, accountID, versionID, itemID uuid.UUID) error
	CreateSingleItem(ctx context.Context, q repository.Querier, accountID, versionID uuid.UUID, in domain.QuoteItemCreate) (*domain.QuoteItem, error)
}

// quoteAIGenerationRepository stores the original proposal independently of its editable version.
type quoteAIGenerationRepository interface {
	Create(ctx context.Context, q repository.Querier, accountID uuid.UUID,
		in domain.NewQuoteAIGeneration, items []domain.NewQuoteAIGenerationItem,
	) (*domain.QuoteAIGeneration, error)
}

// rfqChannelReader is the channel validation surface the RFQ flow needs.
type rfqChannelReader interface {
	ListActiveByType(ctx context.Context, q repository.Querier, accountID, branchID uuid.UUID, channelType domain.ChannelType) ([]domain.Channel, error)
	GetActiveByID(ctx context.Context, q repository.Querier, accountID, branchID, channelID uuid.UUID) (*domain.Channel, error)
}

// catalogMatcher is the matching surface the RFQ flow needs.
type catalogMatcher interface {
	Match(ctx context.Context, tenant domain.Tenant, descriptions []string) ([]domain.LineMatch, error)
}

// interpretationMemoryFinder retrieves account-local examples of how this supplier has read
// previous orders, which the extractor can lean on to disambiguate its own readings.
type interpretationMemoryFinder interface {
	FindInterpretationExamples(ctx context.Context, tenant domain.Tenant,
		raw string) ([]domain.RFQInterpretationExample, error)
}

// memoryAwareRFQExtractor is an extractor that can use interpretation examples when they exist.
type memoryAwareRFQExtractor interface {
	ExtractWithExamples(ctx context.Context, raw string,
		examples []domain.RFQInterpretationExample) (*domain.RFQExtraction, error)
}

// RFQService owns the text RFQ pipeline up to a seller-reviewable quote draft, and the manual
// entry flow.
type RFQService struct {
	db          tenantTxRunner
	rfqs        rfqRepository
	quotes      quoteDraftRepository
	generations quoteAIGenerationRepository
	channels    rfqChannelReader
	extractor   domain.RFQExtractor
	matcher     catalogMatcher
	memories    interpretationMemoryFinder
	log         *slog.Logger
	cfg         config.RFQConfig
	now         func() time.Time
}

// WithCorrectionMemory enables account-local interpretation examples.
func (s *RFQService) WithCorrectionMemory(memories interpretationMemoryFinder) *RFQService {
	s.memories = memories
	return s
}

// NewRFQService builds an RFQService.
func NewRFQService(
	db tenantTxRunner, rfqs rfqRepository, quotes quoteDraftRepository,
	generations quoteAIGenerationRepository,
	channels rfqChannelReader, extractor domain.RFQExtractor, matcher catalogMatcher,
	log *slog.Logger, cfg config.RFQConfig,
) *RFQService {
	if log == nil {
		log = slog.Default()
	}
	return &RFQService{
		db: db, rfqs: rfqs, quotes: quotes, generations: generations, channels: channels,
		extractor: extractor, matcher: matcher, log: log, cfg: cfg, now: time.Now,
	}
}

// ---------- Manual entry ----------

// List returns the RFQ list for the caller's tenant scope.
func (s *RFQService) List(ctx context.Context, tenant domain.Tenant) ([]domain.RfqListItem, error) {
	var items []domain.RfqListItem
	if err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var err error
		items, err = s.rfqs.ListByTenant(ctx, q, tenant)
		return err
	}); err != nil {
		return nil, err
	}
	return items, nil
}

// GetDetail returns the full detail of one RFQ including its quote, items, and alternatives.
func (s *RFQService) GetDetail(ctx context.Context, tenant domain.Tenant, rfqID uuid.UUID) (*domain.RfqDetail, error) {
	var detail *domain.RfqDetail
	if err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		rfq, rfqErr := s.rfqs.GetByRFQID(ctx, q, tenant.AccountID, rfqID)
		if rfqErr != nil {
			return rfqErr
		}

		quote, quoteErr := s.quotes.GetByRFQID(ctx, q, tenant.AccountID, rfqID)
		if quoteErr != nil && !errors.Is(quoteErr, domain.ErrNotFound) {
			return quoteErr
		}

		detail = &domain.RfqDetail{
			Rfq: *rfq,
		}

		if quote == nil {
			return nil
		}

		detail.Quote = quote

		if quote.CurrentVersionID != nil {
			version, versionErr := s.quotes.GetCurrentVersion(ctx, q, tenant.AccountID,
				tenant.BranchID, quote.ID)
			if versionErr != nil && !errors.Is(versionErr, domain.ErrNotFound) {
				return versionErr
			}
			if version != nil {
				detail.Version = version

				items, itemsErr := s.quotes.ListItemsWithProduct(ctx, q, tenant.AccountID, version.ID)
				if itemsErr != nil {
					return itemsErr
				}
				detail.Items = items

				itemIDs := make([]uuid.UUID, 0, len(items))
				for _, item := range items {
					itemIDs = append(itemIDs, item.ID)
				}
				alternatives, altErr := s.quotes.ListAlternativesByItemIDs(ctx, q,
					tenant.AccountID, itemIDs)
				if altErr != nil {
					return altErr
				}
				detail.Alternatives = alternatives
			}
		}

		return nil
	}); err != nil {
		return nil, err
	}
	return detail, nil
}

// UpdateItem patches a quote item. The version must be mutable and the item must belong to it.
func (s *RFQService) UpdateItem(
	ctx context.Context, tenant domain.Tenant, quoteID, itemID uuid.UUID, in domain.QuoteItemUpdate,
) (*domain.QuoteItem, error) {
	if err := requireBranch(tenant, "updating a quote item"); err != nil {
		return nil, err
	}
	if in.Quantity != nil {
		if in.Quantity.LessThanOrEqual(decimal.Zero) {
			return nil, fmt.Errorf("%w: quantity must be greater than zero", domain.ErrInvalidInput)
		}
		if err := validateAmount(*in.Quantity, "quantity"); err != nil {
			return nil, err
		}
	}
	if in.UnitPriceSnapshot != nil {
		if in.UnitPriceSnapshot.LessThan(decimal.Zero) {
			return nil, fmt.Errorf("%w: unit_price_snapshot must be greater than or equal to zero",
				domain.ErrInvalidInput)
		}
		if err := validateAmount(*in.UnitPriceSnapshot, "unit_price_snapshot"); err != nil {
			return nil, err
		}
	}

	var item *domain.QuoteItem
	if err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		quote, quoteErr := s.quotes.GetByID(ctx, q, tenant.AccountID, tenant.BranchID, quoteID)
		if quoteErr != nil {
			return quoteErr
		}
		if !domain.IsEditableStatus(quote.CurrentStatus) {
			return domain.WithCode(domain.CodeQuoteNotDraft, domain.ErrImmutable)
		}
		if quote.CurrentVersionID == nil {
			return domain.ErrNotFound
		}
		version, versionErr := s.quotes.GetCurrentVersion(ctx, q, tenant.AccountID,
			tenant.BranchID, quote.ID)
		if versionErr != nil {
			return versionErr
		}
		var updateErr error
		item, updateErr = s.quotes.UpdateItem(ctx, q, tenant.AccountID, version.ID, itemID, in)
		if updateErr != nil {
			return updateErr
		}

		// Recalculate subtotal and version total when price or quantity changed.
		if in.UnitPriceSnapshot != nil || in.Quantity != nil {
			if recalcErr := s.recalculateVersionTotal(ctx, q, tenant, version.ID); recalcErr != nil {
				return recalcErr
			}
			// Reload item with updated snapshots.
			item, updateErr = s.quotes.GetItem(ctx, q, tenant.AccountID, version.ID, itemID)
			if updateErr != nil {
				return updateErr
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return item, nil
}

// DeleteItem removes a quote item. The version must be mutable and the item must belong
// to it.
func (s *RFQService) DeleteItem(
	ctx context.Context, tenant domain.Tenant, quoteID, itemID uuid.UUID,
) error {
	if err := requireBranch(tenant, "deleting a quote item"); err != nil {
		return err
	}
	return s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		quote, quoteErr := s.quotes.GetByID(ctx, q, tenant.AccountID, tenant.BranchID, quoteID)
		if quoteErr != nil {
			return quoteErr
		}
		if !domain.IsEditableStatus(quote.CurrentStatus) {
			return domain.WithCode(domain.CodeQuoteNotDraft, domain.ErrImmutable)
		}
		if quote.CurrentVersionID == nil {
			return domain.ErrNotFound
		}
		version, versionErr := s.quotes.GetCurrentVersion(ctx, q, tenant.AccountID,
			tenant.BranchID, quote.ID)
		if versionErr != nil {
			return versionErr
		}
		if deleteErr := s.quotes.DeleteItem(ctx, q, tenant.AccountID, version.ID, itemID); deleteErr != nil {
			return deleteErr
		}
		// Recalculate version total after deletion.
		return s.recalculateVersionTotal(ctx, q, tenant, version.ID)
	})
}

// AddItem appends one material line to a quote version.
func (s *RFQService) AddItem(
	ctx context.Context, tenant domain.Tenant, quoteID uuid.UUID, in domain.QuoteItemCreate,
) (*domain.QuoteItem, error) {
	if err := requireBranch(tenant, "adding a quote item"); err != nil {
		return nil, err
	}
	in.RequestedDescription = strings.TrimSpace(in.RequestedDescription)
	if in.RequestedDescription == "" {
		return nil, fmt.Errorf("%w: requested_description is required", domain.ErrInvalidInput)
	}
	if in.Quantity.LessThanOrEqual(decimal.Zero) {
		return nil, fmt.Errorf("%w: quantity must be greater than zero", domain.ErrInvalidInput)
	}
	if err := validateAmount(in.Quantity, "quantity"); err != nil {
		return nil, err
	}

	var item *domain.QuoteItem
	if err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		quote, quoteErr := s.quotes.GetByID(ctx, q, tenant.AccountID, tenant.BranchID, quoteID)
		if quoteErr != nil {
			return quoteErr
		}
		if !domain.IsEditableStatus(quote.CurrentStatus) {
			return domain.WithCode(domain.CodeQuoteNotDraft, domain.ErrImmutable)
		}
		if quote.CurrentVersionID == nil {
			return domain.ErrNotFound
		}
		version, versionErr := s.quotes.GetCurrentVersion(ctx, q, tenant.AccountID,
			tenant.BranchID, quote.ID)
		if versionErr != nil {
			return versionErr
		}
		var createErr error
		item, createErr = s.quotes.CreateSingleItem(ctx, q, tenant.AccountID, version.ID, in)
		return createErr
	}); err != nil {
		return nil, err
	}
	return item, nil
}

// CreateManual records a counter, phone or otherwise unintegrated order.
func (s *RFQService) CreateManual(
	ctx context.Context, tenant domain.Tenant, in domain.NewRfq,
) (*domain.RfqCreation, error) {
	if err := requireBranch(tenant, "a manual RFQ"); err != nil {
		return nil, err
	}
	in, err := normalizeManualEntry(in)
	if err != nil {
		return nil, err
	}

	var creation *domain.RfqCreation
	if err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		channelID, getErr := s.rfqs.GetManualEntryChannelID(ctx, q, tenant.AccountID, tenant.BranchID)
		if getErr != nil {
			return getErr
		}
		if assertErr := s.assertProductsInAccount(ctx, q, tenant.AccountID, in.Items); assertErr != nil {
			return assertErr
		}
		now := time.Now()
		if s.now != nil {
			now = s.now()
		}
		var createErr error
		creation, createErr = s.rfqs.CreateManualEntry(ctx, q, tenant, channelID, in, now)
		return createErr
	}); err != nil {
		return nil, err
	}
	return creation, nil
}

func normalizeManualEntry(in domain.NewRfq) (domain.NewRfq, error) {
	if in.RawText != nil {
		trimmed := strings.TrimSpace(*in.RawText)
		in.RawText = &trimmed
	}
	if in.WorkType != nil {
		trimmed := strings.TrimSpace(*in.WorkType)
		in.WorkType = &trimmed
	}
	if in.ClientLabel != nil {
		trimmed := strings.TrimSpace(*in.ClientLabel)
		in.ClientLabel = &trimmed
	}
	if len(in.Items) == 0 && (in.RawText == nil || *in.RawText == "") {
		return domain.NewRfq{}, fmt.Errorf("%w: a manual entry needs raw_text or at least one item",
			domain.ErrInvalidInput)
	}
	for i := range in.Items {
		if err := normalizeManualItem(&in.Items[i], i+1); err != nil {
			return domain.NewRfq{}, err
		}
	}
	return in, nil
}

func normalizeManualItem(it *domain.NewRfqItem, index int) error {
	it.RequestedDescription = strings.TrimSpace(it.RequestedDescription)
	if it.RequestedDescription == "" {
		return fmt.Errorf("%w: item %d needs a requested_description", domain.ErrInvalidInput, index)
	}
	if it.Unit != nil {
		trimmed := strings.TrimSpace(*it.Unit)
		if trimmed == "" {
			it.Unit = nil
		} else {
			it.Unit = &trimmed
		}
	}
	if it.Quantity.LessThanOrEqual(decimal.Zero) {
		return fmt.Errorf("%w: item %d quantity must be greater than zero", domain.ErrInvalidInput, index)
	}
	return validateAmount(it.Quantity, fmt.Sprintf("item %d quantity", index))
}

func (s *RFQService) assertProductsInAccount(
	ctx context.Context, q repository.Querier, accountID uuid.UUID, items []domain.NewRfqItem,
) error {
	ids := make([]uuid.UUID, 0, len(items))
	for _, it := range items {
		if it.ProductID != nil {
			ids = append(ids, *it.ProductID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	owned, err := s.rfqs.CountProductsInAccount(ctx, q, accountID, ids)
	if err != nil {
		return err
	}
	if owned != len(ids) {
		return domain.ErrNotFound
	}
	return nil
}

// ---------- AI pipeline ----------

// CreateTextDraft turns plain RFQ text into a quote DRAFT for seller review.
func (s *RFQService) CreateTextDraft(
	ctx context.Context, tenant domain.Tenant, in domain.TextRFQDraftInput,
) (*domain.TextRFQDraft, error) {
	if err := requireBranch(tenant, "an RFQ draft"); err != nil {
		return nil, err
	}
	normalized, err := s.normalizeTextRFQDraftInput(in)
	if err != nil {
		return nil, err
	}
	sellerID := tenant.UserID
	return s.createTextDraft(ctx, tenant, normalized, &sellerID)
}

// CreateWhatsAppMockDraft simulates one inbound WhatsApp text message in development.
func (s *RFQService) CreateWhatsAppMockDraft(
	ctx context.Context, tenant domain.Tenant, in domain.WhatsAppMockRFQInput,
) (*domain.TextRFQDraft, error) {
	if err := requireBranch(tenant, "a WhatsApp mock message"); err != nil {
		return nil, err
	}
	from, err := requiredText(in.From, "from")
	if err != nil {
		return nil, err
	}
	if err := requireMaxRunes(from, "from", 64); err != nil {
		return nil, err
	}
	profileName, err := optionalLimitedText(in.ProfileName, "profile_name", 160)
	if err != nil {
		return nil, err
	}
	raw, err := s.requiredRFQText(in.Text)
	if err != nil {
		return nil, err
	}

	channel, err := s.resolveWhatsAppChannel(ctx, tenant, in.ChannelID)
	if err != nil {
		return nil, err
	}
	clientLabel := from
	if profileName != nil {
		clientLabel = fmt.Sprintf("%s (%s)", *profileName, from)
	}
	return s.createTextDraft(ctx, tenant, domain.TextRFQDraftInput{
		ChannelID: channel.ID, ClientLabel: &clientLabel, RawText: raw,
	}, nil)
}

func (s *RFQService) createTextDraft(
	ctx context.Context, tenant domain.Tenant, in domain.TextRFQDraftInput, sellerID *uuid.UUID,
) (*domain.TextRFQDraft, error) {
	if s.extractor == nil || s.channels == nil {
		return nil, fmt.Errorf("%w: the RFQ pipeline is not fully wired", domain.ErrInvalidInput)
	}

	rfq, err := s.persistReceivedRFQ(ctx, tenant, in)
	if err != nil {
		return nil, err
	}

	extraction, items, alternatives, err := s.readMaterials(ctx, tenant, in.RawText)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		s.log.InfoContext(ctx, "rfq produced no materials", slog.String("rfq_id", rfq.ID.String()))
		return &domain.TextRFQDraft{RFQ: *rfq}, nil
	}
	return s.persistGeneratedDraft(ctx, tenant, rfq, sellerID, extraction, items, alternatives)
}

func (s *RFQService) readMaterials(
	ctx context.Context, tenant domain.Tenant, raw string,
) (*domain.RFQExtraction, []domain.NewQuoteItem, []domain.NewQuoteItemAlternative, error) {
	pipelineCtx, cancel := context.WithTimeout(ctx, s.cfg.PipelineTimeout)
	defer cancel()

	var examples []domain.RFQInterpretationExample
	if s.memories != nil {
		var memoryErr error
		examples, memoryErr = s.memories.FindInterpretationExamples(pipelineCtx, tenant, raw)
		if memoryErr != nil {
			s.log.WarnContext(ctx, "interpretation memory unavailable", slog.Any("error", memoryErr))
		}
	}
	var extraction *domain.RFQExtraction
	var err error
	if aware, ok := s.extractor.(memoryAwareRFQExtractor); ok && len(examples) > 0 {
		extraction, err = aware.ExtractWithExamples(pipelineCtx, raw, examples)
	} else {
		extraction, err = s.extractor.Extract(pipelineCtx, raw)
	}
	if err != nil {
		return nil, nil, nil, err
	}
	if extraction == nil {
		return nil, nil, nil, fmt.Errorf("%w: RFQ extraction returned no result",
			domain.ErrInvalidInput)
	}
	// Matching runs one query per line, so an order that came back as a catalog would turn one
	// request into hundreds of them. Stated in the prompt, enforced here.
	if len(extraction.Lines) > s.cfg.MaxItems {
		return nil, nil, nil, fmt.Errorf("%w: the order lists more than %d materials, which is a "+
			"catalog rather than a message", domain.ErrInvalidInput, s.cfg.MaxItems)
	}
	items, err := newQuoteItemsFromRFQLines(extraction.Lines)
	if err != nil {
		return nil, nil, nil, err
	}
	if len(items) == 0 {
		return extraction, nil, nil, nil
	}
	return extraction, items, s.applyMatches(pipelineCtx, tenant, items), nil
}

func (s *RFQService) applyMatches(
	ctx context.Context, tenant domain.Tenant, items []domain.NewQuoteItem,
) []domain.NewQuoteItemAlternative {
	if s.matcher == nil {
		return nil
	}
	descriptions := make([]string, len(items))
	for i, item := range items {
		descriptions[i] = item.RequestedDescription
	}
	matches, err := s.matcher.Match(ctx, tenant, descriptions)
	if err != nil {
		s.log.WarnContext(ctx, "catalog matching did not run; every line stays flagged",
			slog.Any("error", err), slog.Int("lines", len(items)))
		return nil
	}
	if len(matches) != len(items) {
		s.log.ErrorContext(ctx, "catalog matching returned a different number of decisions",
			slog.Int("decisions", len(matches)), slog.Int("lines", len(items)))
		return nil
	}
	var alternatives []domain.NewQuoteItemAlternative
	for i, match := range matches {
		items[i].ProductID = match.ProductID
		items[i].MatchStatus = match.MatchStatus
		items[i].ConfidenceScore = decimal.NewNullDecimal(match.Confidence)
		alternatives = append(alternatives, alternativesFromMatch(items[i].ID, match)...)
	}
	return alternatives
}

func (s *RFQService) persistReceivedRFQ(
	ctx context.Context, tenant domain.Tenant, in domain.TextRFQDraftInput,
) (*domain.RFQ, error) {
	var rfq *domain.RFQ
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		if _, channelErr := s.channels.GetActiveByID(ctx, q, tenant.AccountID, tenant.BranchID,
			in.ChannelID); channelErr != nil {
			return channelErr
		}
		var createErr error
		rfq, createErr = s.rfqs.Create(ctx, q, tenant.AccountID, domain.NewRFQ{
			BranchID:    tenant.BranchID,
			ClientID:    in.ClientID,
			ChannelID:   in.ChannelID,
			RawText:     &in.RawText,
			Status:      domain.RFQStatusReceived,
			WorkType:    in.WorkType,
			ClientLabel: in.ClientLabel,
		})
		return createErr
	})
	if err != nil {
		return nil, err
	}
	return rfq, nil
}

func (s *RFQService) persistGeneratedDraft(
	ctx context.Context, tenant domain.Tenant, rfq *domain.RFQ, sellerID *uuid.UUID,
	extraction *domain.RFQExtraction, items []domain.NewQuoteItem,
	alternatives []domain.NewQuoteItemAlternative,
) (*domain.TextRFQDraft, error) {
	var draft domain.TextRFQDraft
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		quote, createQuoteErr := s.quotes.Create(ctx, q, tenant.AccountID, domain.NewQuote{
			BranchID:      tenant.BranchID,
			ClientID:      rfq.ClientID,
			RFQID:         rfq.ID,
			SellerID:      sellerID,
			CurrentStatus: domain.QuoteStatusDraft,
		})
		if createQuoteErr != nil {
			return createQuoteErr
		}

		version, createVersionErr := s.quotes.CreateVersion(ctx, q, tenant.AccountID,
			domain.NewQuoteVersion{
				QuoteID:       quote.ID,
				AuthorID:      sellerID,
				VersionNumber: 1,
				Total:         decimal.Zero,
				IsImmutable:   false,
			})
		if createVersionErr != nil {
			return createVersionErr
		}

		createdItems, createItemsErr := s.quotes.CreateItems(ctx, q, tenant.AccountID,
			version.ID, items)
		if createItemsErr != nil {
			return createItemsErr
		}
		candidates, candidatesErr := s.persistAlternatives(ctx, q, tenant.AccountID, createdItems,
			alternatives)
		if candidatesErr != nil {
			return candidatesErr
		}
		if s.generations == nil {
			return fmt.Errorf("%w: AI generation persistence is not wired", domain.ErrInvalidInput)
		}
		generationItems, generationItemsErr := newQuoteAIGenerationItems(extraction.Lines, items)
		if generationItemsErr != nil {
			return generationItemsErr
		}
		if _, generationErr := s.generations.Create(ctx, q, tenant.AccountID,
			newQuoteAIGeneration(quote.ID, version.ID, extraction), generationItems); generationErr != nil {
			return generationErr
		}

		quote, updateQuoteErr := s.quotes.UpdateCurrentVersion(ctx, q, tenant.AccountID,
			quote.ID, version.ID)
		if updateQuoteErr != nil {
			return updateQuoteErr
		}

		previousRFQStatus := rfq.Status
		if _, appendRFQErr := s.rfqs.AppendStatusChange(ctx, q, tenant.AccountID, rfq.ID,
			&previousRFQStatus, domain.RFQStatusGenerated, sellerID); appendRFQErr != nil {
			return appendRFQErr
		}
		if _, appendQuoteErr := s.quotes.AppendStatusChange(ctx, q, tenant.AccountID, quote.ID,
			nil, domain.QuoteStatusDraft, sellerID); appendQuoteErr != nil {
			return appendQuoteErr
		}

		generated, updateRFQErr := s.rfqs.UpdateStatus(ctx, q, tenant.AccountID, rfq.ID,
			domain.RFQStatusGenerated)
		if updateRFQErr != nil {
			return updateRFQErr
		}

		draft = domain.TextRFQDraft{
			RFQ:          *generated,
			Quote:        quote,
			Version:      version,
			Items:        createdItems,
			Alternatives: candidates,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &draft, nil
}

func newQuoteAIGeneration(
	quoteID, versionID uuid.UUID, extraction *domain.RFQExtraction,
) domain.NewQuoteAIGeneration {
	return domain.NewQuoteAIGeneration{
		QuoteID: quoteID, QuoteVersionID: versionID,
		Provider: extraction.Usage.Provider, Model: extraction.Usage.Model,
		PromptVersion: extraction.PromptVersion, SchemaVersion: extraction.SchemaVersion,
		InputTokens: extraction.Usage.InputTokens, OutputTokens: extraction.Usage.OutputTokens,
		CacheReadTokens:  extraction.Usage.CacheReadTokens,
		CacheWriteTokens: extraction.Usage.CacheWriteTokens,
	}
}

func newQuoteAIGenerationItems(
	lines []domain.ExtractedRFQLine, items []domain.NewQuoteItem,
) ([]domain.NewQuoteAIGenerationItem, error) {
	if len(lines) != len(items) {
		return nil, fmt.Errorf("%w: extraction lines and quote items are not aligned",
			domain.ErrInvalidInput)
	}
	snapshots := make([]domain.NewQuoteAIGenerationItem, 0, len(items))
	for i, item := range items {
		snapshots = append(snapshots, domain.NewQuoteAIGenerationItem{
			Position: i, SourceQuoteItemID: item.ID, ProductID: item.ProductID,
			RequestedDescription: item.RequestedDescription, Quantity: item.Quantity, Unit: item.Unit,
			QuantitySource: lines[i].Source, QuantityRationale: *item.QuantityRationale,
			MatchStatus: item.MatchStatus, ConfidenceScore: item.ConfidenceScore,
		})
	}
	return snapshots, nil
}

// persistAlternatives writes the flagged lines' candidates and reads them back with the catalog
// identity a seller needs to tell them apart, which the insert cannot return: RETURNING sees only
// the row it wrote, and the product's name is a join away.
func (s *RFQService) persistAlternatives(
	ctx context.Context, q repository.Querier, accountID uuid.UUID, items []domain.QuoteItem,
	alternatives []domain.NewQuoteItemAlternative,
) (map[uuid.UUID][]domain.QuoteItemAlternative, error) {
	if len(alternatives) == 0 {
		return nil, nil
	}
	if err := s.quotes.CreateAlternatives(ctx, q, accountID, alternatives); err != nil {
		return nil, err
	}
	return s.quotes.ListAlternativesByItemIDs(ctx, q, accountID, quoteItemIDs(items))
}

func (s *RFQService) getActiveChannel(
	ctx context.Context, tenant domain.Tenant, channelID uuid.UUID,
) (*domain.Channel, error) {
	var channel *domain.Channel
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var getErr error
		channel, getErr = s.channels.GetActiveByID(ctx, q, tenant.AccountID, tenant.BranchID,
			channelID)
		return getErr
	})
	if err != nil {
		return nil, err
	}
	return channel, nil
}

func (s *RFQService) resolveWhatsAppChannel(
	ctx context.Context, tenant domain.Tenant, channelID *uuid.UUID,
) (*domain.Channel, error) {
	if s.channels == nil {
		return nil, fmt.Errorf("%w: the RFQ pipeline is not fully wired", domain.ErrInvalidInput)
	}
	if channelID != nil {
		if *channelID == uuid.Nil {
			return nil, fmt.Errorf("%w: channel_id must be a valid UUID", domain.ErrInvalidInput)
		}
		channel, err := s.getActiveChannel(ctx, tenant, *channelID)
		if err != nil {
			return nil, err
		}
		if channel.Type != domain.ChannelTypeWhatsApp {
			return nil, fmt.Errorf("%w: channel_id must identify a WhatsApp channel",
				domain.ErrInvalidInput)
		}
		return channel, nil
	}

	var channels []domain.Channel
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var listErr error
		channels, listErr = s.channels.ListActiveByType(ctx, q, tenant.AccountID,
			tenant.BranchID, domain.ChannelTypeWhatsApp)
		return listErr
	})
	if err != nil {
		return nil, err
	}
	if len(channels) == 0 {
		return nil, fmt.Errorf("%w: no active WhatsApp channel for the selected branch",
			domain.ErrNotFound)
	}
	if len(channels) > 1 {
		return nil, fmt.Errorf(
			"%w: channel_id is required when the branch has multiple WhatsApp channels",
			domain.ErrInvalidInput)
	}
	return &channels[0], nil
}

func (s *RFQService) normalizeTextRFQDraftInput(
	in domain.TextRFQDraftInput,
) (domain.TextRFQDraftInput, error) {
	if in.ChannelID == uuid.Nil {
		return domain.TextRFQDraftInput{}, fmt.Errorf("%w: channel_id is required",
			domain.ErrInvalidInput)
	}
	raw, err := s.requiredRFQText(in.RawText)
	if err != nil {
		return domain.TextRFQDraftInput{}, err
	}
	clientLabel, err := optionalLimitedText(in.ClientLabel, "client_label", 255)
	if err != nil {
		return domain.TextRFQDraftInput{}, err
	}
	workType, err := optionalLimitedText(in.WorkType, "work_type", 255)
	if err != nil {
		return domain.TextRFQDraftInput{}, err
	}
	in.RawText = raw
	in.ClientLabel = clientLabel
	in.WorkType = workType
	return in, nil
}

func (s *RFQService) requiredRFQText(raw string) (string, error) {
	text, err := requiredText(raw, "raw_text")
	if err != nil {
		return "", err
	}
	if err := requireMaxRunes(text, "raw_text", s.cfg.MaxTextCharacters); err != nil {
		return "", err
	}
	return text, nil
}

func newQuoteItemsFromRFQLines(lines []domain.ExtractedRFQLine) ([]domain.NewQuoteItem, error) {
	items := make([]domain.NewQuoteItem, 0, len(lines))
	for i, line := range lines {
		field := fmt.Sprintf("items[%d]", i)
		description, err := requiredText(line.RequestedDescription, field+".requested_description")
		if err != nil {
			return nil, err
		}
		if err := requireMaxRunes(description, field+".requested_description", 512); err != nil {
			return nil, err
		}
		if err := validateQuantitySource(line, field); err != nil {
			return nil, err
		}
		if err := validateAmount(line.Quantity, field+".quantity"); err != nil {
			return nil, err
		}
		unit, err := optionalLimitedText(line.Unit, field+".unit", 64)
		if err != nil {
			return nil, err
		}
		rationale, err := requiredText(line.QuantityRationale, field+".quantity_rationale")
		if err != nil {
			return nil, err
		}
		if err := requireMaxRunes(rationale, field+".quantity_rationale", 512); err != nil {
			return nil, err
		}

		items = append(items, domain.NewQuoteItem{
			ID:                   uuid.New(),
			RequestedDescription: description,
			Quantity:             line.Quantity,
			Unit:                 unit,
			MatchStatus:          domain.ItemMatchStatusNoMatch,
			QuantityRationale:    &rationale,
		})
	}
	return items, nil
}

func alternativesFromMatch(
	itemID uuid.UUID, match domain.LineMatch,
) []domain.NewQuoteItemAlternative {
	if match.MatchStatus == domain.ItemMatchStatusMatched {
		return nil
	}
	alternatives := make([]domain.NewQuoteItemAlternative, 0, len(match.Candidates))
	for i, candidate := range match.Candidates {
		if match.ProductID != nil && *match.ProductID == candidate.ProductID {
			continue
		}
		if candidate.Confidence.IsZero() {
			continue
		}
		productID := candidate.ProductID
		alternatives = append(alternatives, domain.NewQuoteItemAlternative{
			QuoteItemID:     itemID,
			ProductID:       &productID,
			Type:            domain.QuoteItemAlternativeTypeProduct,
			Origin:          domain.QuoteItemAlternativeOriginAI,
			Rank:            i + 1,
			ConfidenceScore: decimal.NewNullDecimal(candidate.Confidence),
		})
	}
	return alternatives
}

func validateQuantitySource(line domain.ExtractedRFQLine, field string) error {
	switch line.Source {
	case domain.QuantitySourceExplicit, domain.QuantitySourceDerived:
		if !line.Quantity.IsPositive() {
			return fmt.Errorf("%w: %s.quantity must be positive when it comes from the message",
				domain.ErrInvalidInput, field)
		}
	case domain.QuantitySourceUnresolved:
		if !line.Quantity.IsZero() {
			return fmt.Errorf("%w: %s.quantity must be zero when no quantity could be read",
				domain.ErrInvalidInput, field)
		}
	default:
		return fmt.Errorf("%w: %s.quantity_source %q is not a known source",
			domain.ErrInvalidInput, field, line.Source)
	}
	return nil
}

func optionalLimitedText(raw *string, field string, max int) (*string, error) {
	trimmed := optionalText(raw)
	if trimmed == nil {
		return nil, nil
	}
	if err := requireMaxRunes(*trimmed, field, max); err != nil {
		return nil, err
	}
	return trimmed, nil
}

func requireMaxRunes(value, field string, max int) error {
	if utf8.RuneCountInString(value) > max {
		return fmt.Errorf("%w: %s cannot exceed %d characters", domain.ErrInvalidInput, field, max)
	}
	return nil
}

// recalculateVersionTotal recomputes a version's total from its items and discounts, then
// persists it. Called after any item or price mutation.
func (s *RFQService) recalculateVersionTotal(
	ctx context.Context, q repository.Querier, tenant domain.Tenant, versionID uuid.UUID,
) error {
	items, err := s.quotes.ListItems(ctx, q, tenant.AccountID, versionID)
	if err != nil {
		return err
	}
	subtotals := decimal.Zero
	for _, item := range items {
		if item.Subtotal.Valid {
			subtotals = subtotals.Add(item.Subtotal.Decimal)
		}
	}
	// The discount sweep is US-38. When implemented, load quote_discount rows here.
	discounts := decimal.Zero
	total := subtotals.Sub(discounts).Round(domain.MoneyScale)
	_, err = s.quotes.UpdateVersionTotal(ctx, q, tenant.AccountID, versionID, total)
	return err
}
