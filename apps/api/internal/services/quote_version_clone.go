package services

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type quoteVersionCloneRepository interface {
	CreateVersion(ctx context.Context, q repository.Querier, accountID uuid.UUID,
		in domain.NewQuoteVersion) (*domain.QuoteVersion, error)
	UpdateCurrentVersion(ctx context.Context, q repository.Querier, accountID, quoteID,
		versionID uuid.UUID) (*domain.Quote, error)
	ListItems(ctx context.Context, q repository.Querier, accountID,
		versionID uuid.UUID) ([]domain.QuoteItem, error)
	CreateItems(ctx context.Context, q repository.Querier, accountID, versionID uuid.UUID,
		items []domain.NewQuoteItem) ([]domain.QuoteItem, error)
	ListAlternativesByItemIDs(ctx context.Context, q repository.Querier, accountID uuid.UUID,
		itemIDs []uuid.UUID) (map[uuid.UUID][]domain.QuoteItemAlternative, error)
	CreateAlternatives(ctx context.Context, q repository.Querier, accountID uuid.UUID,
		alternatives []domain.NewQuoteItemAlternative) error
}

type quoteVersionFreezer interface {
	FreezeVersion(ctx context.Context, q repository.Querier, accountID, branchID, quoteID,
		versionID uuid.UUID) (*domain.QuoteVersion, error)
}

// draftDiscountCopier copies the seller's discount decisions from a version into a draft cloned
// from it.
type draftDiscountCopier interface {
	CopySellerDecisions(ctx context.Context, q repository.Querier, accountID, sourceVersionID,
		draftVersionID uuid.UUID) error
}

type draftTotalWriter interface {
	UpdateVersionTotal(ctx context.Context, q repository.Querier, accountID, versionID uuid.UUID,
		total decimal.Decimal) (*domain.QuoteVersion, error)
}

// draftCarry settles a fresh draft as the customer saw its source: the seller's typed and
// suppressed discounts come along, and the sweep re-earns the promotions and sets the total.
type draftCarry struct {
	discounts draftDiscountCopier
	sweep     *discountSweep
	totals    draftTotalWriter
}

// apply copies the decisions from sourceID into draftID and settles the draft.
func (c *draftCarry) apply(ctx context.Context, q repository.Querier,
	quotes quoteVersionCloneRepository, accountID, branchID, sourceID, draftID uuid.UUID,
) (*domain.QuoteVersion, error) {
	if err := c.discounts.CopySellerDecisions(ctx, q, accountID, sourceID, draftID); err != nil {
		return nil, err
	}
	items, err := quotes.ListItems(ctx, q, accountID, draftID)
	if err != nil {
		return nil, err
	}
	total, err := c.sweep.settle(ctx, q, accountID, branchID, draftID, items)
	if err != nil {
		return nil, err
	}
	return c.totals.UpdateVersionTotal(ctx, q, accountID, draftID, total)
}

func (c *draftCarry) applySelectionSnapshot(ctx context.Context, q repository.Querier,
	quotes quoteVersionCloneRepository, accountID, branchID, sourceID, draftID uuid.UUID,
	snapshot domain.QuoteSelectionPricingSnapshot,
) (*domain.QuoteVersion, error) {
	if err := c.discounts.CopySellerDecisions(ctx, q, accountID, sourceID, draftID); err != nil {
		return nil, err
	}
	items, err := quotes.ListItems(ctx, q, accountID, draftID)
	if err != nil {
		return nil, err
	}
	total, err := c.sweep.settleWithSnapshot(ctx, q, accountID, branchID, draftID, items, snapshot)
	if err != nil {
		return nil, err
	}
	return c.totals.UpdateVersionTotal(ctx, q, accountID, draftID, total)
}

type quoteVersionClone struct {
	Draft         *domain.QuoteVersion
	SourceItemIDs map[uuid.UUID]uuid.UUID
}

// cloneQuoteVersionForClientSelection materializes an answer to a sent offer without rewriting
// that offer. The new version keeps every sent line's snapshots except the approved products the
// customer explicitly selected.
func cloneQuoteVersionForClientSelection(
	ctx context.Context, q repository.Querier, quotes quoteVersionCloneRepository,
	freezer quoteVersionFreezer,
	accountID, branchID, quoteID uuid.UUID, source domain.QuoteVersion,
	selections []domain.ClientAlternativeSelection, carry *draftCarry,
	snapshot domain.QuoteSelectionPricingSnapshot,
) (*domain.QuoteVersion, error) {
	if carry == nil || snapshot.CapturedAt.IsZero() {
		return nil, domain.ErrNotConfigured
	}
	items, err := quotes.ListItems(ctx, q, accountID, source.ID)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, domain.ErrInvalidInput
	}
	alternatives, err := quotes.ListAlternativesByItemIDs(ctx, q, accountID, quoteItemIDs(items))
	if err != nil {
		return nil, err
	}
	selected, err := selectedAlternatives(items, alternatives, selections)
	if err != nil {
		return nil, err
	}
	comment := "Customer-selected alternatives"
	draft, err := quotes.CreateVersion(ctx, q, accountID, domain.NewQuoteVersion{
		QuoteID: quoteID, VersionNumber: source.VersionNumber + 1, Currency: source.Currency,
		Total: decimal.Zero, Comment: &comment,
	})
	if err != nil {
		return nil, err
	}
	selectedItems := withSelectedAlternatives(items, selected)
	cloned := make([]domain.NewQuoteItem, 0, len(selectedItems))
	for _, item := range selectedItems {
		sourceItemID := item.ID
		cloned = append(cloned, domain.NewQuoteItem{ID: uuid.New(), ProductID: item.ProductID,
			RequestedDescription: item.RequestedDescription, Quantity: item.Quantity, Unit: item.Unit,
			UnitPriceSnapshot: item.UnitPriceSnapshot, MinPriceSnapshot: item.MinPriceSnapshot,
			Subtotal: item.Subtotal, ConfidenceScore: item.ConfidenceScore, MatchStatus: item.MatchStatus,
			QuantityRationale: item.QuantityRationale, SourceItemID: &sourceItemID})
	}
	if _, err := quotes.CreateItems(ctx, q, accountID, draft.ID, cloned); err != nil {
		return nil, err
	}
	draft, err = carry.applySelectionSnapshot(ctx, q, quotes, accountID, branchID, source.ID, draft.ID, snapshot)
	if err != nil {
		return nil, err
	}
	if _, err := quotes.UpdateCurrentVersion(ctx, q, accountID, quoteID, draft.ID); err != nil {
		return nil, err
	}
	frozen, err := freezer.FreezeVersion(ctx, q, accountID, branchID, quoteID, draft.ID)
	if err != nil {
		return nil, err
	}
	return frozen, nil
}

// withSelectedAlternatives applies approved customer choices without changing the sent item ids.
func withSelectedAlternatives(
	items []domain.QuoteItem, selected map[uuid.UUID]domain.QuoteItemAlternative,
) []domain.QuoteItem {
	result := make([]domain.QuoteItem, len(items))
	copy(result, items)
	for index := range result {
		alternative, chosen := selected[result[index].ID]
		if !chosen {
			continue
		}
		result[index].ProductID = alternative.ProductID
		result[index].UnitPriceSnapshot = alternative.PriceSnapshot
		result[index].MinPriceSnapshot = alternative.MinPriceSnapshot
		result[index].Subtotal = decimal.NewNullDecimal(
			roundMoney(result[index].Quantity.Mul(alternative.PriceSnapshot.Decimal)),
		)
		result[index].ConfidenceScore = decimal.NullDecimal{}
		result[index].MatchStatus = domain.ItemMatchStatusMatched
	}
	return result
}

func selectedAlternatives(
	items []domain.QuoteItem, alternatives map[uuid.UUID][]domain.QuoteItemAlternative,
	selections []domain.ClientAlternativeSelection,
) (map[uuid.UUID]domain.QuoteItemAlternative, error) {
	knownItems := make(map[uuid.UUID]struct{}, len(items))
	for _, item := range items {
		knownItems[item.ID] = struct{}{}
	}
	selected := make(map[uuid.UUID]domain.QuoteItemAlternative, len(selections))
	for _, selection := range selections {
		if selection.ItemID == uuid.Nil || selection.AlternativeID == uuid.Nil {
			return nil, fmt.Errorf("%w: an alternative selection is incomplete", domain.ErrInvalidInput)
		}
		if _, exists := knownItems[selection.ItemID]; !exists {
			return nil, domain.ErrNotFound
		}
		if _, exists := selected[selection.ItemID]; exists {
			return nil, fmt.Errorf("%w: a line can only select one alternative", domain.ErrInvalidInput)
		}
		var candidate *domain.QuoteItemAlternative
		for i := range alternatives[selection.ItemID] {
			alternative := &alternatives[selection.ItemID][i]
			if alternative.ID == selection.AlternativeID {
				candidate = alternative
				break
			}
		}
		if candidate == nil || !candidate.ApprovedBySeller ||
			candidate.Type != domain.QuoteItemAlternativeTypeProduct || candidate.ProductID == nil ||
			!candidate.PriceSnapshot.Valid {
			return nil, domain.ErrNotFound
		}
		if err := validateAmount(candidate.PriceSnapshot.Decimal, "alternative price"); err != nil {
			return nil, err
		}
		selected[selection.ItemID] = *candidate
	}
	return selected, nil
}

func cloneQuoteVersionForEditing(
	ctx context.Context, q repository.Querier, quotes quoteVersionCloneRepository,
	prices branchPriceReader, accountID, branchID, quoteID uuid.UUID, source domain.QuoteVersion,
	authorID *uuid.UUID, comment *string, carry *draftCarry,
) (*quoteVersionClone, error) {
	content, err := prepareVersionCopy(ctx, q, quotes, prices, accountID, branchID, source.ID)
	if err != nil {
		return nil, err
	}
	draft, err := quotes.CreateVersion(ctx, q, accountID, domain.NewQuoteVersion{
		QuoteID: quoteID, AuthorID: authorID, VersionNumber: source.VersionNumber + 1,
		Currency: content.valuation.currency, Total: content.valuation.total, Comment: comment,
	})
	if err != nil {
		return nil, err
	}
	sourceItemIDs, err := insertVersionCopy(ctx, q, quotes, accountID, draft.ID, content)
	if err != nil {
		return nil, err
	}
	if _, err := quotes.UpdateCurrentVersion(ctx, q, accountID, quoteID, draft.ID); err != nil {
		return nil, err
	}
	if carry != nil {
		if draft, err = carry.apply(ctx, q, quotes, accountID, branchID, source.ID,
			draft.ID); err != nil {
			return nil, err
		}
	}
	return &quoteVersionClone{Draft: draft, SourceItemIDs: sourceItemIDs}, nil
}

// versionCopy is a version's lines and candidates revalued at today's prices, ready to write.
type versionCopy struct {
	valuation    quoteValuation
	alternatives map[uuid.UUID][]domain.QuoteItemAlternative
}

// prepareVersionCopy reads a version's lines and candidates and values them at the branch's
// current prices; candidates lose the seller's approval, which belongs to the version they were on.
func prepareVersionCopy(
	ctx context.Context, q repository.Querier, quotes quoteVersionCloneRepository,
	prices branchPriceReader, accountID, branchID, sourceID uuid.UUID,
) (versionCopy, error) {
	items, err := quotes.ListItems(ctx, q, accountID, sourceID)
	if err != nil {
		return versionCopy{}, err
	}
	alternatives, err := quotes.ListAlternativesByItemIDs(ctx, q, accountID,
		quoteItemIDs(items))
	if err != nil {
		return versionCopy{}, err
	}
	currentPrices, err := prices.GetCurrentByProductIDs(ctx, q, accountID, branchID,
		quoteProductIDs(items, alternatives))
	if err != nil {
		return versionCopy{}, err
	}
	valuation, err := valueQuoteItems(items, currentPrices)
	if err != nil {
		return versionCopy{}, err
	}

	for itemID, offered := range alternatives {
		for i := range offered {
			offered[i].ApprovedBySeller = false
		}
		alternatives[itemID] = offered
	}
	alternativePricings, err := valueQuoteAlternatives(alternatives, currentPrices,
		valuation.currency)
	if err != nil {
		return versionCopy{}, err
	}
	applyAlternativePricing(alternatives, alternativePricings)
	return versionCopy{valuation: valuation, alternatives: alternatives}, nil
}

// insertVersionCopy writes a prepared copy into a mutable version and returns which new line
// each source line became.
func insertVersionCopy(
	ctx context.Context, q repository.Querier, quotes quoteVersionCloneRepository,
	accountID, versionID uuid.UUID, content versionCopy,
) (map[uuid.UUID]uuid.UUID, error) {
	clonedItems := make([]domain.NewQuoteItem, 0, len(content.valuation.items))
	clonedAlternatives := make([]domain.NewQuoteItemAlternative, 0)
	sourceItemIDs := make(map[uuid.UUID]uuid.UUID, len(content.valuation.items))
	for _, item := range content.valuation.items {
		newID := uuid.New()
		sourceItemIDs[item.ID] = newID
		sourceID := item.ID
		clonedItems = append(clonedItems, domain.NewQuoteItem{
			ID: newID, SourceItemID: &sourceID, ProductID: item.ProductID, RequestedDescription: item.RequestedDescription,
			Quantity: item.Quantity, Unit: item.Unit, ConfidenceScore: item.ConfidenceScore,
			MatchStatus: item.MatchStatus, QuantityRationale: item.QuantityRationale,
			UnitPriceSnapshot: item.UnitPriceSnapshot, MinPriceSnapshot: item.MinPriceSnapshot,
			Subtotal: item.Subtotal,
		})
		for _, alternative := range content.alternatives[item.ID] {
			clonedAlternatives = append(clonedAlternatives, domain.NewQuoteItemAlternative{
				QuoteItemID: newID, ProductID: alternative.ProductID,
				ComboID: alternative.ComboID, Type: alternative.Type,
				RecommendationType: alternative.RecommendationType, Origin: alternative.Origin,
				Rank: alternative.Rank, ConfidenceScore: alternative.ConfidenceScore,
				PriceSnapshot: alternative.PriceSnapshot,
			})
		}
	}
	if _, err := quotes.CreateItems(ctx, q, accountID, versionID, clonedItems); err != nil {
		return nil, err
	}
	if err := quotes.CreateAlternatives(ctx, q, accountID, clonedAlternatives); err != nil {
		return nil, err
	}
	return sourceItemIDs, nil
}
