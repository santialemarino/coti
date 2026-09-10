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
	ListStatusChanges(ctx context.Context, q repository.Querier, accountID, branchID, rfqID uuid.UUID) ([]domain.RFQStatusChange, error)
	ListByTenant(ctx context.Context, q repository.Querier, tenant domain.Tenant) ([]domain.RfqListItem, error)
	GetByRFQID(ctx context.Context, q repository.Querier, tenant domain.Tenant, rfqID uuid.UUID) (*domain.RfqListItem, error)
	AssignSeller(ctx context.Context, q repository.Querier, tenant domain.Tenant, rfqID uuid.UUID) (*domain.Quote, error)
	SetSeller(ctx context.Context, q repository.Querier, tenant domain.Tenant, rfqID uuid.UUID, sellerID *uuid.UUID) (*domain.Quote, error)
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
	GetPreviousVersion(ctx context.Context, q repository.Querier, accountID, branchID, quoteID uuid.UUID, versionNumber int) (*domain.QuoteVersion, error)
	ListItems(ctx context.Context, q repository.Querier, accountID, versionID uuid.UUID) ([]domain.QuoteItem, error)
	ListItemsWithProduct(ctx context.Context, q repository.Querier, accountID, versionID uuid.UUID) ([]domain.QuoteItem, error)
	GetItem(ctx context.Context, q repository.Querier, accountID, versionID, itemID uuid.UUID) (*domain.QuoteItem, error)
	UpdateItem(ctx context.Context, q repository.Querier, accountID, versionID, itemID uuid.UUID, in domain.QuoteItemUpdate) (*domain.QuoteItem, error)
	DeleteItem(ctx context.Context, q repository.Querier, accountID, versionID, itemID uuid.UUID) error
	CreateSingleItem(ctx context.Context, q repository.Querier, accountID, versionID uuid.UUID, in domain.QuoteItemCreate) (*domain.QuoteItem, error)
	ListStatusChanges(ctx context.Context, q repository.Querier, accountID, branchID, quoteID uuid.UUID) ([]domain.QuoteStatusChange, error)
}

// quoteSendTracker is the delivery tracking surface the RFQ detail needs.
type quoteSendTracker interface {
	ListByQuote(ctx context.Context, q repository.Querier, accountID, branchID, quoteID uuid.UUID) ([]domain.QuoteSend, error)
}

// quoteDiscountRepository is the discount application persistence surface the RFQ flow needs.
type quoteDiscountRepository interface {
	ListByVersionID(ctx context.Context, q repository.Querier, accountID, versionID uuid.UUID) ([]domain.QuoteDiscount, error)
	Create(ctx context.Context, q repository.Querier, accountID, versionID uuid.UUID, in domain.QuoteDiscountCreate) (*domain.QuoteDiscount, error)
	GetByID(ctx context.Context, q repository.Querier, accountID, versionID, discountID uuid.UUID) (*domain.QuoteDiscount, error)
	UpdateByID(ctx context.Context, q repository.Querier, accountID, versionID, discountID uuid.UUID, in domain.QuoteDiscountUpdate) (*domain.QuoteDiscount, error)
	DeleteByID(ctx context.Context, q repository.Querier, accountID, versionID, discountID uuid.UUID) error
	UpdateAmount(ctx context.Context, q repository.Querier, accountID, versionID, discountID uuid.UUID, amount decimal.Decimal) error
	CreateItemLinks(ctx context.Context, q repository.Querier, accountID, versionID, discountID uuid.UUID, itemIDs []uuid.UUID) error
	ReplaceItemLinks(ctx context.Context, q repository.Querier, accountID, versionID, discountID uuid.UUID, itemIDs []uuid.UUID) error
	ListItemIDs(ctx context.Context, q repository.Querier, accountID, discountID uuid.UUID) ([]uuid.UUID, error)
	ListItemIDsByDiscountIDs(ctx context.Context, q repository.Querier, accountID, versionID uuid.UUID, discountIDs []uuid.UUID) (map[uuid.UUID][]uuid.UUID, error)
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

// rfqAttachmentStorer keeps the file an order arrived as, against the RFQ it produced.
type rfqAttachmentStorer interface {
	StoreForRFQ(ctx context.Context, tenant domain.Tenant, rfqID uuid.UUID,
		file domain.AttachmentUpload, data []byte, extractedText string) error
}

// sellerReach checks that a named seller can serve the caller's branch before a manual entry
// names them, so an order cannot point at a seller another branch owns.
type sellerReach interface {
	SellerServesBranches(ctx context.Context, q repository.Querier, accountID uuid.UUID,
		branchIDs []uuid.UUID, userID uuid.UUID) (bool, error)
}

// RFQService owns the text RFQ pipeline up to a seller-reviewable quote draft, and the manual
// entry flow.
type RFQService struct {
	db          tenantTxRunner
	rfqs        rfqRepository
	quotes      quoteDraftRepository
	discounts   quoteDiscountRepository
	sends       quoteSendTracker
	generations quoteAIGenerationRepository
	channels    rfqChannelReader
	sellers     sellerReach
	extractor   domain.RFQExtractor
	matcher     catalogMatcher
	memories    interpretationMemoryFinder
	// transcriber and attachments are only needed by the file intake, which refuses when
	// either is unbound rather than making the text pipeline depend on them.
	transcriber domain.Transcriber
	attachments rfqAttachmentStorer
	maxFileSize int64
	log         *slog.Logger
	cfg         config.RFQConfig
	now         func() time.Time
}

// WithFileIntake wires what an order that arrived as a file needs: somewhere to keep the file
// and a transcriber for a voice note.
func (s *RFQService) WithFileIntake(attachments rfqAttachmentStorer,
	transcriber domain.Transcriber, maxFileSize int64) *RFQService {
	s.attachments = attachments
	s.transcriber = transcriber
	s.maxFileSize = maxFileSize
	return s
}

// WithCorrectionMemory enables account-local interpretation examples.
func (s *RFQService) WithCorrectionMemory(memories interpretationMemoryFinder) *RFQService {
	s.memories = memories
	return s
}

// WithDiscounts wires quote discount persistence, which the manual entry detail and the
// version total depend on.
func (s *RFQService) WithDiscounts(discounts quoteDiscountRepository) *RFQService {
	s.discounts = discounts
	return s
}

// NewRFQService builds an RFQService.
func NewRFQService(
	db tenantTxRunner, rfqs rfqRepository, quotes quoteDraftRepository,
	sends quoteSendTracker, generations quoteAIGenerationRepository,
	channels rfqChannelReader, sellers sellerReach,
	extractor domain.RFQExtractor, matcher catalogMatcher,
	log *slog.Logger, cfg config.RFQConfig,
) *RFQService {
	if log == nil {
		log = slog.Default()
	}
	return &RFQService{
		db: db, rfqs: rfqs, quotes: quotes, sends: sends, generations: generations, channels: channels,
		sellers: sellers, extractor: extractor, matcher: matcher, log: log, cfg: cfg,
		now: time.Now,
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
		rfq, rfqErr := s.rfqs.GetByRFQID(ctx, q, tenant, rfqID)
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

		rfqChanges, rfqChangesErr := s.rfqs.ListStatusChanges(ctx, q, tenant.AccountID,
			rfq.BranchID, rfqID)
		if rfqChangesErr != nil {
			return rfqChangesErr
		}
		detail.RFQStatusChanges = rfqChanges

		if quote == nil {
			return nil
		}

		detail.Quote = quote

		quoteChanges, quoteChangesErr := s.quotes.ListStatusChanges(ctx, q, tenant.AccountID,
			quote.BranchID, quote.ID)
		if quoteChangesErr != nil {
			return quoteChangesErr
		}
		detail.QuoteStatusChanges = quoteChanges

		if s.sends != nil {
			deliveries, deliveryErr := s.sends.ListByQuote(ctx, q, tenant.AccountID,
				quote.BranchID, quote.ID)
			if deliveryErr != nil {
				return deliveryErr
			}
			detail.Deliveries = deliveries
		}

		if quote.CurrentVersionID != nil {
			version, versionErr := s.quotes.GetCurrentVersion(ctx, q, tenant.AccountID,
				quote.BranchID, quote.ID)
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

				detail.Discounts = []domain.QuoteDiscount{}
				if s.discounts != nil {
					discounts, discountErr := s.discounts.ListByVersionID(ctx, q,
						tenant.AccountID, version.ID)
					if discountErr != nil {
						return discountErr
					}
					detail.Discounts = discounts
					if linkErr := s.attachDiscountItemIDs(ctx, q, tenant.AccountID,
						version.ID, detail.Discounts); linkErr != nil {
						return linkErr
					}
					if quote.CurrentStatus == domain.QuoteStatusChangeRequested && !version.IsImmutable {
						if buildErr := s.buildChangeRequestDiff(ctx, q, tenant,
							quote, version, items, discounts, detail); buildErr != nil {
							return buildErr
						}
					}
				}
			}
		}

		return nil
	}); err != nil {
		return nil, err
	}
	return detail, nil
}

// AssignSeller claims an unclaimed order for the seller. Self-assignment is seller-only: an
// admin already reaches the whole account, so they have no unclaimed pool to draw from. The
// claim is atomic at the repository, and the loser of a race answers ErrConflict.
func (s *RFQService) AssignSeller(
	ctx context.Context, tenant domain.Tenant, rfqID uuid.UUID,
) (*domain.Quote, error) {
	if tenant.IsAdmin() {
		return nil, domain.ErrForbidden
	}
	var quote *domain.Quote
	if err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var err error
		quote, err = s.rfqs.AssignSeller(ctx, q, tenant, rfqID)
		return err
	}); err != nil {
		return nil, err
	}
	return quote, nil
}

// SetSeller writes an order's owner for an admin: the caller themself, a seller who can serve
// the order's own branch, or — with a nil id — no one. The order's branch anchors the seller
// check rather than the admin's active branch, so an admin shown every branch cannot hand an
// order to a seller who would never list it. Self always passes, because the caller is already
// inside the account. Unlike AssignSeller there is no unclaimed-pool to draw from: the write
// replaces whoever owns the order.
func (s *RFQService) SetSeller(
	ctx context.Context, tenant domain.Tenant, rfqID uuid.UUID, sellerID *uuid.UUID,
) (*domain.Quote, error) {
	if !tenant.IsAdmin() {
		return nil, domain.ErrForbidden
	}
	var quote *domain.Quote
	if err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		item, err := s.rfqs.GetByRFQID(ctx, q, tenant, rfqID)
		if err != nil {
			return err
		}
		if sellerID != nil && *sellerID != tenant.UserID {
			serves, checkErr := s.sellers.SellerServesBranches(ctx, q, tenant.AccountID,
				[]uuid.UUID{item.BranchID}, *sellerID)
			if checkErr != nil {
				return checkErr
			}
			if !serves {
				return fmt.Errorf("%w: seller_id names a seller who cannot serve the order's branch",
					domain.ErrInvalidInput)
			}
		}
		quote, err = s.rfqs.SetSeller(ctx, q, tenant, rfqID, sellerID)
		return err
	}); err != nil {
		return nil, err
	}
	return quote, nil
}

// attachDiscountItemIDs loads every discount's covered lines in one batch and folds them back
// into the detail so the seller sees what an ITEM/ITEM_SET discount applies to.
func (s *RFQService) attachDiscountItemIDs(
	ctx context.Context, q repository.Querier, accountID, versionID uuid.UUID,
	discounts []domain.QuoteDiscount,
) error {
	if len(discounts) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, 0, len(discounts))
	for _, discount := range discounts {
		ids = append(ids, discount.ID)
	}
	links, err := s.discounts.ListItemIDsByDiscountIDs(ctx, q, accountID, versionID, ids)
	if err != nil {
		return err
	}
	for i := range discounts {
		discounts[i].ItemIDs = links[discounts[i].ID]
	}
	return nil
}

// buildChangeRequestDiff fills the change-request comparison for a CHANGE_REQUESTED
// quote whose current version is a mutable draft: the frozen predecessor the client
// saw versus the lines and discounts the seller is rebuilding. The client's reason
// rides on the draft version comment, so it stays visible while the rework is open.
func (s *RFQService) buildChangeRequestDiff(
	ctx context.Context, q repository.Querier, tenant domain.Tenant,
	quote *domain.Quote, version *domain.QuoteVersion, items []domain.QuoteItem,
	discounts []domain.QuoteDiscount, detail *domain.RfqDetail,
) error {
	previous, prevErr := s.quotes.GetPreviousVersion(ctx, q, tenant.AccountID,
		tenant.BranchID, quote.ID, version.VersionNumber)
	if prevErr != nil {
		if errors.Is(prevErr, domain.ErrNotFound) {
			return nil
		}
		return prevErr
	}
	frozenItems, itemsErr := s.quotes.ListItemsWithProduct(ctx, q, tenant.AccountID, previous.ID)
	if itemsErr != nil {
		return itemsErr
	}
	frozenDiscounts, discountsErr := s.discounts.ListByVersionID(ctx, q,
		tenant.AccountID, previous.ID)
	if discountsErr != nil {
		return discountsErr
	}
	diff := domain.BuildChangeRequestDiff(version.Comment, domain.ChangeRequestSnapshot{
		Items: frozenItems, Discounts: frozenDiscounts, Total: previous.Total,
	}, domain.ChangeRequestSnapshot{
		Items: items, Discounts: discounts, Total: version.Total,
	})
	detail.ChangesRequested = &diff
	return nil
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
		if version.IsImmutable {
			return domain.ErrImmutable
		}
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
		if version.IsImmutable {
			return domain.ErrImmutable
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
		if version.IsImmutable {
			return domain.ErrImmutable
		}
		item, createErr = s.quotes.CreateSingleItem(ctx, q, tenant.AccountID, version.ID, in)
		return createErr
	}); err != nil {
		return nil, err
	}
	return item, nil
}

// AddDiscount applies a seller-typed discount to the current version. A FIXED_AMOUNT value
// must not exceed the subtotal it is scoped to; a PERCENTAGE one is a rate the recalculator
// turns into money, so it moves in step with the items underneath it. The total never goes
// negative: it bottoms out at zero in the recalculator.
func (s *RFQService) AddDiscount(
	ctx context.Context, tenant domain.Tenant, quoteID uuid.UUID, in domain.QuoteDiscountCreate,
) (*domain.QuoteDiscount, error) {
	if err := requireBranch(tenant, "adding a discount"); err != nil {
		return nil, err
	}
	if s.discounts == nil {
		return nil, fmt.Errorf("%w: discount persistence is not wired", domain.ErrInvalidInput)
	}
	normalized, err := normalizeDiscountRule(in)
	if err != nil {
		return nil, err
	}

	var discount *domain.QuoteDiscount
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
		items, itemsErr := s.quotes.ListItems(ctx, q, tenant.AccountID, version.ID)
		if itemsErr != nil {
			return itemsErr
		}
		amount, baseErr := s.amountForRule(items, normalized)
		if baseErr != nil {
			return baseErr
		}
		normalized.Amount = amount
		var createErr error
		discount, createErr = s.discounts.Create(ctx, q, tenant.AccountID, version.ID, normalized)
		if createErr != nil {
			return createErr
		}
		if normalized.Scope != domain.DiscountScopeTotal {
			if linkErr := s.discounts.CreateItemLinks(ctx, q, tenant.AccountID, version.ID,
				discount.ID, normalized.ItemIDs); linkErr != nil {
				return linkErr
			}
		}
		if recalcErr := s.recalculateVersionTotal(ctx, q, tenant, version.ID); recalcErr != nil {
			return recalcErr
		}
		fresh, freshErr := s.discounts.GetByID(ctx, q, tenant.AccountID, version.ID, discount.ID)
		if freshErr != nil {
			return freshErr
		}
		discount = fresh
		return nil
	}); err != nil {
		return nil, err
	}
	return discount, nil
}

// UpdateDiscount patches a seller-typed discount: its rule (value, action type, scope, the
// lines it covers) or the suppression flag. Editing the rule revalidates the covered base
// and recomputes the money amount, so the total stays consistent after the write.
func (s *RFQService) UpdateDiscount(
	ctx context.Context, tenant domain.Tenant, quoteID, discountID uuid.UUID,
	in domain.QuoteDiscountUpdate,
) (*domain.QuoteDiscount, error) {
	if err := requireBranch(tenant, "updating a discount"); err != nil {
		return nil, err
	}
	if s.discounts == nil {
		return nil, fmt.Errorf("%w: discount persistence is not wired", domain.ErrInvalidInput)
	}
	if err := validateDiscountUpdate(&in); err != nil {
		return nil, err
	}

	var discount *domain.QuoteDiscount
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
		existing, existingErr := s.discounts.GetByID(ctx, q, tenant.AccountID, version.ID,
			discountID)
		if existingErr != nil {
			return existingErr
		}
		if existing.Origin != domain.DiscountOriginManualSeller {
			return fmt.Errorf("%w: only a seller-typed discount can be edited",
				domain.ErrInvalidInput)
		}

		ruleEdited := in.Value != nil || in.ActionType != nil || in.Scope != nil || len(in.ItemIDs) > 0
		if ruleEdited {
			rule, ruleErr := s.rebuiltRule(ctx, q, tenant.AccountID, version.ID, existing, in)
			if ruleErr != nil {
				return ruleErr
			}
			suppression := existing.SuppressedBySeller
			update := domain.QuoteDiscountUpdate{
				ActionType:         &rule.actionType,
				Value:              &rule.value,
				Scope:              &rule.scope,
				ConditionType:      conditionTypeFor(rule.scope),
				SuppressedBySeller: &suppression,
			}
			if in.Description != nil {
				update.Description = in.Description
			}
			if _, updateErr := s.discounts.UpdateByID(ctx, q, tenant.AccountID, version.ID,
				discountID, update); updateErr != nil {
				return updateErr
			}
			if rule.items != nil {
				if linkErr := s.discounts.ReplaceItemLinks(ctx, q, tenant.AccountID, version.ID,
					discountID, rule.items); linkErr != nil {
					return linkErr
				}
			}
			if amountErr := s.discounts.UpdateAmount(ctx, q, tenant.AccountID, version.ID,
				discountID, rule.amount); amountErr != nil {
				return amountErr
			}
		} else if in.Description != nil {
			if _, updateErr := s.discounts.UpdateByID(ctx, q, tenant.AccountID, version.ID,
				discountID, domain.QuoteDiscountUpdate{Description: in.Description}); updateErr != nil {
				return updateErr
			}
		} else if in.SuppressedBySeller != nil {
			if _, updateErr := s.discounts.UpdateByID(ctx, q, tenant.AccountID, version.ID,
				discountID, domain.QuoteDiscountUpdate{SuppressedBySeller: in.SuppressedBySeller}); updateErr != nil {
				return updateErr
			}
		}

		if recalcErr := s.recalculateVersionTotal(ctx, q, tenant, version.ID); recalcErr != nil {
			return recalcErr
		}
		fresh, freshErr := s.discounts.GetByID(ctx, q, tenant.AccountID, version.ID, discountID)
		if freshErr != nil {
			return freshErr
		}
		discount = fresh
		return nil
	}); err != nil {
		return nil, err
	}
	return discount, nil
}

// DeleteDiscount removes a seller-typed discount. An AUTOMATIC one is never deleted: it is
// suppressed instead, because suppressing stays reversible and deleting does not.
func (s *RFQService) DeleteDiscount(
	ctx context.Context, tenant domain.Tenant, quoteID, discountID uuid.UUID,
) error {
	if err := requireBranch(tenant, "deleting a discount"); err != nil {
		return err
	}
	if s.discounts == nil {
		return fmt.Errorf("%w: discount persistence is not wired", domain.ErrInvalidInput)
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
		existing, existingErr := s.discounts.GetByID(ctx, q, tenant.AccountID, version.ID,
			discountID)
		if existingErr != nil {
			return existingErr
		}
		if existing.Origin != domain.DiscountOriginManualSeller {
			return fmt.Errorf("%w: only a seller-typed discount can be deleted",
				domain.ErrInvalidInput)
		}
		if deleteErr := s.discounts.DeleteByID(ctx, q, tenant.AccountID, version.ID, discountID); deleteErr != nil {
			return deleteErr
		}
		return s.recalculateVersionTotal(ctx, q, tenant, version.ID)
	})
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
		if in.SellerID != nil {
			serves, checkErr := s.sellers.SellerServesBranches(ctx, q, tenant.AccountID,
				tenant.BranchFilter(), *in.SellerID)
			if checkErr != nil {
				return checkErr
			}
			if !serves {
				return fmt.Errorf("%w: seller_id names a seller who cannot serve the active branch",
					domain.ErrInvalidInput)
			}
		}
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

// CreateFileDraft turns an order that arrived as a file into a quote DRAFT for seller review.
// The file is read before anything is written, so an unreadable one leaves no half-made order
// behind; once it reads, the RFQ, the stored file and the draft are the same flow the text
// pipeline runs.
func (s *RFQService) CreateFileDraft(
	ctx context.Context, tenant domain.Tenant, in domain.FileRFQDraftInput,
) (*domain.TextRFQDraft, error) {
	if err := requireBranch(tenant, "an RFQ draft"); err != nil {
		return nil, err
	}
	if s.extractor == nil || s.channels == nil || s.attachments == nil {
		return nil, fmt.Errorf("%w: the RFQ file pipeline is not fully wired",
			domain.ErrInvalidInput)
	}
	contentExtractor, ok := s.extractor.(domain.RFQContentExtractor)
	if !ok {
		return nil, domain.ErrNotConfigured
	}

	normalized, format, data, err := s.normalizeFileRFQDraftInput(in)
	if err != nil {
		return nil, err
	}

	pipelineCtx, cancel := context.WithTimeout(ctx, s.cfg.PipelineTimeout)
	defer cancel()
	blocks, extractedText, err := s.readFileContent(pipelineCtx, normalized, format, data)
	if err != nil {
		return nil, err
	}
	if normalized.Note != nil {
		blocks = append(blocks,
			domain.TextContent("Nota del vendedor sobre este pedido:\n"+*normalized.Note))
	}

	// The RFQ keeps whatever text the file yielded. An image or a PDF yields none — the file
	// itself is the record, and the attachment row points at it.
	rawText := extractedText
	if rawText == "" {
		rawText = fileOrderPlaceholder(normalized.Filename)
	}
	rfq, err := s.persistReceivedRFQ(ctx, tenant, domain.TextRFQDraftInput{
		ChannelID:   normalized.ChannelID,
		ClientID:    normalized.ClientID,
		ClientLabel: normalized.ClientLabel,
		RawText:     rawText,
		WorkType:    normalized.WorkType,
	})
	if err != nil {
		return nil, err
	}

	// Storing the file before the model runs means an outage there still leaves the seller the
	// order the client actually sent, on an RFQ they can work by hand.
	if storeErr := s.attachments.StoreForRFQ(ctx, tenant, rfq.ID, normalized.File, data,
		extractedText); storeErr != nil {
		return nil, storeErr
	}

	extraction, items, alternatives, err := s.readMaterialsFromContent(pipelineCtx, tenant,
		contentExtractor, blocks, extractedText)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		s.log.InfoContext(ctx, "rfq file produced no materials",
			slog.String("rfq_id", rfq.ID.String()))
		return &domain.TextRFQDraft{RFQ: *rfq}, nil
	}
	sellerID := tenant.UserID
	return s.persistGeneratedDraft(ctx, tenant, rfq, &sellerID, extraction, items, alternatives)
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

// readMaterialsFromContent is readMaterials for an order that arrived as a file. Interpretation
// memory is keyed on text, so it only applies when the file yielded some: a photo has nothing
// to look up examples by.
func (s *RFQService) readMaterialsFromContent(
	ctx context.Context, tenant domain.Tenant, extractor domain.RFQContentExtractor,
	blocks []domain.Content, extractedText string,
) (*domain.RFQExtraction, []domain.NewQuoteItem, []domain.NewQuoteItemAlternative, error) {
	var examples []domain.RFQInterpretationExample
	if s.memories != nil && extractedText != "" {
		var memoryErr error
		examples, memoryErr = s.memories.FindInterpretationExamples(ctx, tenant, extractedText)
		if memoryErr != nil {
			s.log.WarnContext(ctx, "interpretation memory unavailable", slog.Any("error", memoryErr))
		}
	}
	extraction, err := extractor.ExtractFromContent(ctx, blocks, examples)
	if err != nil {
		return nil, nil, nil, err
	}
	if extraction == nil {
		return nil, nil, nil, fmt.Errorf("%w: RFQ extraction returned no result",
			domain.ErrInvalidInput)
	}
	return s.quoteItemsFromExtraction(ctx, tenant, extraction)
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
	return s.quoteItemsFromExtraction(pipelineCtx, tenant, extraction)
}

// quoteItemsFromExtraction turns one model answer into matched draft lines, whatever the order
// arrived as.
func (s *RFQService) quoteItemsFromExtraction(
	ctx context.Context, tenant domain.Tenant, extraction *domain.RFQExtraction,
) (*domain.RFQExtraction, []domain.NewQuoteItem, []domain.NewQuoteItemAlternative, error) {
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
	return extraction, items, s.applyMatches(ctx, tenant, items), nil
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
				Currency:      domain.DefaultCurrency,
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

/*
 * normalizeFileRFQDraftInput checks the envelope and buffers the file, refusing a type or a
 * size before any of it is read. It returns the resolved format and the bytes, so the caller
 * reads the upload once and both the model and storage are served from the same buffer.
 */
func (s *RFQService) normalizeFileRFQDraftInput(
	in domain.FileRFQDraftInput,
) (domain.FileRFQDraftInput, domain.AttachmentFormat, []byte, error) {
	var empty domain.AttachmentFormat
	if in.ChannelID == uuid.Nil {
		return in, empty, nil, fmt.Errorf("%w: channel_id is required", domain.ErrInvalidInput)
	}
	clientLabel, err := optionalLimitedText(in.ClientLabel, "client_label", 255)
	if err != nil {
		return in, empty, nil, err
	}
	workType, err := optionalLimitedText(in.WorkType, "work_type", 255)
	if err != nil {
		return in, empty, nil, err
	}
	note, err := optionalLimitedText(in.Note, "note", s.cfg.MaxTextCharacters)
	if err != nil {
		return in, empty, nil, err
	}

	in.File.ContentType = normalizeContentType(in.File.ContentType)
	format, ok := domain.AttachmentFormatFor(in.File.ContentType)
	if !ok {
		return in, empty, nil, domain.WithCode(domain.CodeUnsupportedFileType, fmt.Errorf(
			"%w: %q is not an accepted file type, which are: %s", domain.ErrInvalidInput,
			in.File.ContentType, strings.Join(domain.AcceptedAttachmentContentTypes(), ", ")))
	}
	data, err := readUpload(in.File, s.maxFileSize)
	if err != nil {
		return in, empty, nil, err
	}

	in.ClientLabel = clientLabel
	in.WorkType = workType
	in.Note = note
	// The extension decides how a spreadsheet is parsed and how a recording is decoded, and the
	// client's filename may carry neither, so the accepted format's own extension stands in.
	if in.Filename == "" || !strings.Contains(in.Filename, ".") {
		in.Filename = "pedido." + format.Extension
	}
	return in, format, data, nil
}

// fileOrderPlaceholder is the raw_text of an order whose file yields no text of its own — a
// photo or a PDF. The row cannot be blank and the file is the record, so it names the file.
func fileOrderPlaceholder(filename string) string {
	return "Pedido recibido como archivo: " + filename
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

// sumItemSubtotals totals a version's item subtotals, ignoring lines nothing has valued yet.
func sumItemSubtotals(items []domain.QuoteItem) decimal.Decimal {
	totals := decimal.Zero
	for _, item := range items {
		if item.Subtotal.Valid {
			totals = totals.Add(item.Subtotal.Decimal)
		}
	}
	return totals
}

// normalizeDiscountRule validates and trims the entries a manual discount is created from.
// The amount is set by the caller once the rule is validated against its scope base.
func normalizeDiscountRule(in domain.QuoteDiscountCreate) (domain.QuoteDiscountCreate, error) {
	in.Description = strings.TrimSpace(in.Description)
	if in.Description == "" {
		return domain.QuoteDiscountCreate{}, fmt.Errorf("%w: description is required",
			domain.ErrInvalidInput)
	}
	if err := requireMaxRunes(in.Description, "description", 512); err != nil {
		return domain.QuoteDiscountCreate{}, err
	}
	if in.ActionType != domain.PromotionActionFixedAmount &&
		in.ActionType != domain.PromotionActionPercentage {
		return domain.QuoteDiscountCreate{}, fmt.Errorf(
			"%w: action_type must be FIXED_AMOUNT or PERCENTAGE", domain.ErrInvalidInput)
	}
	if in.Value.LessThanOrEqual(decimal.Zero) {
		return domain.QuoteDiscountCreate{}, fmt.Errorf("%w: value must be greater than zero",
			domain.ErrInvalidInput)
	}
	if err := validateAmount(in.Value, "value"); err != nil {
		return domain.QuoteDiscountCreate{}, err
	}
	if in.ActionType == domain.PromotionActionPercentage &&
		in.Value.GreaterThan(decimal.NewFromInt(100)) {
		return domain.QuoteDiscountCreate{}, fmt.Errorf(
			"%w: a percentage discount cannot exceed 100", domain.ErrInvalidInput)
	}
	if err := validateDiscountScope(in.Scope, len(in.ItemIDs)); err != nil {
		return domain.QuoteDiscountCreate{}, err
	}
	return in, nil
}

// validateDiscountUpdate checks the patch entries a manual discount is edited with. Values
// the caller did not send stay nil, so the transaction can tell a rule edit from a plain
// suppression toggle.
func validateDiscountUpdate(in *domain.QuoteDiscountUpdate) error {
	if in.Description != nil {
		*in.Description = strings.TrimSpace(*in.Description)
		if *in.Description == "" {
			return fmt.Errorf("%w: description is required", domain.ErrInvalidInput)
		}
		if err := requireMaxRunes(*in.Description, "description", 512); err != nil {
			return err
		}
	}
	if in.Value != nil {
		if in.Value.LessThanOrEqual(decimal.Zero) {
			return fmt.Errorf("%w: value must be greater than zero", domain.ErrInvalidInput)
		}
		if err := validateAmount(*in.Value, "value"); err != nil {
			return err
		}
		if in.ActionType != nil && *in.ActionType == domain.PromotionActionPercentage &&
			in.Value.GreaterThan(decimal.NewFromInt(100)) {
			return fmt.Errorf("%w: a percentage discount cannot exceed 100",
				domain.ErrInvalidInput)
		}
	}
	if in.ActionType != nil &&
		*in.ActionType != domain.PromotionActionFixedAmount &&
		*in.ActionType != domain.PromotionActionPercentage {
		return fmt.Errorf("%w: action_type must be FIXED_AMOUNT or PERCENTAGE",
			domain.ErrInvalidInput)
	}
	if in.Scope != nil {
		return validateDiscountScope(*in.Scope, len(in.ItemIDs))
	}
	if len(in.ItemIDs) > 0 && in.Scope == nil {
		return fmt.Errorf("%w: item_ids needs a scope", domain.ErrInvalidInput)
	}
	return nil
}

// validateDiscountScope enforces the item count a discount scope allows.
func validateDiscountScope(scope domain.DiscountScope, itemCount int) error {
	switch scope {
	case domain.DiscountScopeTotal:
		if itemCount != 0 {
			return fmt.Errorf("%w: a TOTAL discount applies to no items",
				domain.ErrInvalidInput)
		}
	case domain.DiscountScopeItem, domain.DiscountScopeItemSet:
		if itemCount == 0 {
			return fmt.Errorf("%w: %s scope needs at least one item",
				domain.ErrInvalidInput, scope)
		}
	default:
		return fmt.Errorf("%w: scope must be TOTAL, ITEM or ITEM_SET",
			domain.ErrInvalidInput)
	}
	if scope == domain.DiscountScopeItem && itemCount != 1 {
		return fmt.Errorf("%w: ITEM scope takes exactly one item", domain.ErrInvalidInput)
	}
	return nil
}

// amountForRule validates the lines a manual discount covers and returns the money its rule
// applies: the fixed amount as typed, or the percentage of the covered base.
func (s *RFQService) amountForRule(items []domain.QuoteItem, in domain.QuoteDiscountCreate) (decimal.Decimal, error) {
	base, err := scopeBaseForItems(items, in.Scope, in.ItemIDs)
	if err != nil {
		return decimal.Zero, err
	}
	amount := ruleAmount(base, in.ActionType, in.Value)
	if in.ActionType == domain.PromotionActionFixedAmount && amount.GreaterThan(base) {
		return decimal.Zero, fmt.Errorf("%w: the discount cannot exceed the subtotal it covers",
			domain.ErrInvalidInput)
	}
	return amount, nil
}

// discountRule is the resolved shape of a manual discount before it is written.
type discountRule struct {
	actionType domain.PromotionActionType
	value      decimal.Decimal
	amount     decimal.Decimal
	scope      domain.DiscountScope
	items      []uuid.UUID
}

// rebuiltRule merges the current discount with a patch: absent patch fields keep the stored
// rule, and the covered lines come from ItemIDs or, when absent, from the existing links.
func (s *RFQService) rebuiltRule(
	ctx context.Context, q repository.Querier, accountID, versionID uuid.UUID,
	existing *domain.QuoteDiscount, in domain.QuoteDiscountUpdate,
) (discountRule, error) {
	rule := discountRule{actionType: existing.ActionType, value: decimal.Zero, scope: existing.Scope}
	if existing.ActionValue != nil {
		rule.value = *existing.ActionValue
	}
	if in.ActionType != nil {
		rule.actionType = *in.ActionType
	}
	if in.Value != nil {
		rule.value = *in.Value
	}
	if in.Scope != nil {
		rule.scope = *in.Scope
	}
	if rule.actionType == domain.PromotionActionPercentage &&
		rule.value.GreaterThan(decimal.NewFromInt(100)) {
		return rule, fmt.Errorf("%w: a percentage discount cannot exceed 100",
			domain.ErrInvalidInput)
	}
	rule.items = in.ItemIDs
	if rule.scope != domain.DiscountScopeTotal && len(rule.items) == 0 {
		ids, listErr := s.discounts.ListItemIDs(ctx, q, accountID, existing.ID)
		if listErr != nil {
			return rule, listErr
		}
		rule.items = ids
	}
	if err := validateDiscountScope(rule.scope, len(rule.items)); err != nil {
		return rule, err
	}
	items, listErr := s.quotes.ListItems(ctx, q, accountID, versionID)
	if listErr != nil {
		return rule, listErr
	}
	base, baseErr := scopeBaseForItems(items, rule.scope, rule.items)
	if baseErr != nil {
		return rule, baseErr
	}
	rule.amount = ruleAmount(base, rule.actionType, rule.value)
	if rule.actionType == domain.PromotionActionFixedAmount && rule.amount.GreaterThan(base) {
		return rule, fmt.Errorf("%w: the discount cannot exceed the subtotal it covers",
			domain.ErrInvalidInput)
	}
	return rule, nil
}

// scopeBaseForItems validates the covered lines and returns the subtotal the discount acts
// on: the whole version for TOTAL, the covered items otherwise.
func scopeBaseForItems(items []domain.QuoteItem, scope domain.DiscountScope, itemIDs []uuid.UUID) (decimal.Decimal, error) {
	if scope == domain.DiscountScopeTotal {
		return sumItemSubtotals(items), nil
	}
	ids := make(map[uuid.UUID]struct{}, len(items))
	subtotals := make(map[uuid.UUID]decimal.Decimal, len(items))
	for _, item := range items {
		ids[item.ID] = struct{}{}
		if item.Subtotal.Valid {
			subtotals[item.ID] = item.Subtotal.Decimal
		}
	}
	base := decimal.Zero
	for _, id := range itemIDs {
		if _, ok := ids[id]; !ok {
			return decimal.Zero, fmt.Errorf("%w: item %s does not belong to this quote",
				domain.ErrInvalidInput, id)
		}
		base = base.Add(subtotals[id])
	}
	return base, nil
}

// ruleAmount turns a typed rule over a base into money: the typed amount itself, or the
// base percentage rounded to the money scale.
func ruleAmount(base decimal.Decimal, actionType domain.PromotionActionType, value decimal.Decimal) decimal.Decimal {
	if actionType == domain.PromotionActionPercentage {
		return base.Mul(value).Div(decimal.NewFromInt(100)).Round(domain.MoneyScale)
	}
	return value.Round(domain.MoneyScale)
}

// conditionTypeFor derives the condition a manual discount's scope implies.
func conditionTypeFor(scope domain.DiscountScope) *domain.PromotionConditionType {
	switch scope {
	case domain.DiscountScopeTotal:
		condition := domain.PromotionConditionOnTotal
		return &condition
	case domain.DiscountScopeItem:
		condition := domain.PromotionConditionPerItem
		return &condition
	default:
		condition := domain.PromotionConditionItemSet
		return &condition
	}
}

// recalculateVersionTotal recomputes a version's total from its items and discounts, then
// persists it. A non-suppressed seller-typed PERCENTAGE is turned into money against its
// scope and written back, so a discount always tracks the lines that carry it. Called after
// any item, price, or discount mutation.
func (s *RFQService) recalculateVersionTotal(
	ctx context.Context, q repository.Querier, tenant domain.Tenant, versionID uuid.UUID,
) error {
	items, err := s.quotes.ListItems(ctx, q, tenant.AccountID, versionID)
	if err != nil {
		return err
	}
	subtotals := sumItemSubtotals(items)
	discountSum := decimal.Zero
	if s.discounts != nil {
		discounts, listErr := s.discounts.ListByVersionID(ctx, q, tenant.AccountID, versionID)
		if listErr != nil {
			return listErr
		}
		if len(discounts) > 0 {
			discountIDs := make([]uuid.UUID, 0, len(discounts))
			for _, discount := range discounts {
				discountIDs = append(discountIDs, discount.ID)
			}
			linked, listErr := s.discounts.ListItemIDsByDiscountIDs(ctx, q,
				tenant.AccountID, versionID, discountIDs)
			if listErr != nil {
				return listErr
			}
			for _, discount := range discounts {
				if discount.SuppressedBySeller {
					continue
				}
				amount := discount.Amount
				if discount.Origin == domain.DiscountOriginManualSeller &&
					discount.ActionType == domain.PromotionActionPercentage &&
					discount.ActionValue != nil {
					base, baseErr := scopeBaseForItems(items, discount.Scope, linked[discount.ID])
					if baseErr != nil {
						return baseErr
					}
					computed := ruleAmount(base, discount.ActionType, *discount.ActionValue)
					if !computed.Equal(amount) {
						if updateErr := s.discounts.UpdateAmount(ctx, q, tenant.AccountID,
							versionID, discount.ID, computed); updateErr != nil {
							return updateErr
						}
						amount = computed
					}
				}
				discountSum = discountSum.Add(amount)
			}
		}
	}
	// The result never goes negative: a discount scoped after the items it beat can exceed the
	// net, and the total bottoms out at zero instead of carrying a sign.
	net := subtotals.Sub(discountSum)
	if net.IsNegative() {
		net = decimal.Zero
	}
	total := net.Round(domain.MoneyScale)
	_, err = s.quotes.UpdateVersionTotal(ctx, q, tenant.AccountID, versionID, total)
	return err
}
