package services

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type draftSnapshotQuotes interface {
	ListItems(ctx context.Context, q repository.Querier, accountID,
		versionID uuid.UUID) ([]domain.QuoteItem, error)
	ListAlternativesByItemIDs(ctx context.Context, q repository.Querier, accountID uuid.UUID,
		itemIDs []uuid.UUID) (map[uuid.UUID][]domain.QuoteItemAlternative, error)
}

type draftSnapshotDiscounts interface {
	ListByVersionID(ctx context.Context, q repository.Querier, accountID,
		versionID uuid.UUID) ([]domain.QuoteDiscount, error)
	ListItemIDsByDiscountIDs(ctx context.Context, q repository.Querier, accountID,
		versionID uuid.UUID, discountIDs []uuid.UUID) (map[uuid.UUID][]uuid.UUID, error)
}

type handlerDecisionSettleStore interface {
	ListTracesByVersion(ctx context.Context, q repository.Querier, accountID,
		versionID uuid.UUID) ([]domain.HandlerDecisionTrace, error)
	Decide(ctx context.Context, q repository.Querier, accountID, versionID, userID uuid.UUID,
		decisions map[uuid.UUID]domain.HandlerSellerDecision) (int, error)
}

// HandlerDecisionSettler writes the seller's side of every handler run on a draft when the draft is
// frozen for sending, inside the caller's transaction.
type HandlerDecisionSettler struct {
	decisions handlerDecisionSettleStore
	quotes    draftSnapshotQuotes
	discounts draftSnapshotDiscounts
}

// NewHandlerDecisionSettler builds a HandlerDecisionSettler.
func NewHandlerDecisionSettler(decisions handlerDecisionSettleStore, quotes draftSnapshotQuotes,
	discounts draftSnapshotDiscounts,
) *HandlerDecisionSettler {
	return &HandlerDecisionSettler{decisions: decisions, quotes: quotes, discounts: discounts}
}

// Settle decides every still-undecided run on a version the seller is freezing.
func (s *HandlerDecisionSettler) Settle(ctx context.Context, q repository.Querier,
	tenant domain.Tenant, versionID uuid.UUID,
) error {
	traces, err := s.decisions.ListTracesByVersion(ctx, q, tenant.AccountID, versionID)
	if err != nil || len(traces) == 0 {
		return err
	}
	final, err := draftSnapshot(ctx, q, s.quotes, s.discounts, tenant.AccountID, versionID)
	if err != nil {
		return err
	}
	_, err = s.decisions.Decide(ctx, q, tenant.AccountID, versionID, tenant.UserID,
		domain.SettleHandlerDecisions(traces, final))
	return err
}

// draftSnapshot reads what the seller controls on a version, in an order independent of row ids.
func draftSnapshot(ctx context.Context, q repository.Querier, quotes draftSnapshotQuotes,
	discounts draftSnapshotDiscounts, accountID, versionID uuid.UUID,
) (domain.DraftSnapshot, error) {
	items, err := quotes.ListItems(ctx, q, accountID, versionID)
	if err != nil {
		return domain.DraftSnapshot{}, err
	}
	alternatives, err := quotes.ListAlternativesByItemIDs(ctx, q, accountID, quoteItemIDs(items))
	if err != nil {
		return domain.DraftSnapshot{}, err
	}
	applied, err := discounts.ListByVersionID(ctx, q, accountID, versionID)
	if err != nil {
		return domain.DraftSnapshot{}, err
	}
	links, err := discounts.ListItemIDsByDiscountIDs(ctx, q, accountID, versionID,
		discountIDs(applied))
	if err != nil {
		return domain.DraftSnapshot{}, err
	}

	snapshotItems := make([]domain.DraftSnapshotItem, 0, len(items))
	itemIDs := make([]uuid.UUID, 0, len(items))
	for _, item := range items {
		approved := make([]uuid.UUID, 0)
		for _, alternative := range alternatives[item.ID] {
			if alternative.ApprovedBySeller && alternative.ProductID != nil {
				approved = append(approved, *alternative.ProductID)
			}
		}
		slices.SortFunc(approved, compareUUIDs)
		snapshotItems = append(snapshotItems, domain.DraftSnapshotItem{
			ProductID: item.ProductID, RequestedDescription: item.RequestedDescription,
			Quantity: item.Quantity.String(), Unit: item.Unit, ApprovedAlternatives: approved})
		itemIDs = append(itemIDs, item.ID)
	}
	order := make([]int, len(snapshotItems))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int {
		return compareSnapshotItems(snapshotItems[a], snapshotItems[b])
	})
	sorted := make([]domain.DraftSnapshotItem, len(order))
	position := make(map[uuid.UUID]int, len(order))
	for to, from := range order {
		sorted[to] = snapshotItems[from]
		position[itemIDs[from]] = to
	}

	snapshotDiscounts := make([]domain.DraftSnapshotDiscount, 0)
	for _, discount := range applied {
		if discount.Origin != domain.DiscountOriginManualSeller && !discount.SuppressedBySeller {
			continue
		}
		covered := make([]int, 0, len(links[discount.ID]))
		for _, id := range links[discount.ID] {
			covered = append(covered, position[id])
		}
		slices.Sort(covered)
		var value *string
		if discount.ActionValue != nil {
			text := discount.ActionValue.String()
			value = &text
		}
		snapshotDiscounts = append(snapshotDiscounts, domain.DraftSnapshotDiscount{
			Origin: discount.Origin, PromotionID: discount.PromotionID,
			ActionType: discount.ActionType, Value: value, Scope: discount.Scope,
			Suppressed: discount.SuppressedBySeller, Items: covered})
	}
	slices.SortFunc(snapshotDiscounts, compareSnapshotDiscounts)
	return domain.DraftSnapshot{Items: sorted, Discounts: snapshotDiscounts}, nil
}

func compareSnapshotItems(a, b domain.DraftSnapshotItem) int {
	return strings.Compare(snapshotItemKey(a), snapshotItemKey(b))
}

func snapshotItemKey(item domain.DraftSnapshotItem) string {
	product := ""
	if item.ProductID != nil {
		product = item.ProductID.String()
	}
	unit := ""
	if item.Unit != nil {
		unit = *item.Unit
	}
	return strings.Join([]string{product, item.RequestedDescription, item.Quantity, unit}, "\x00")
}

func compareSnapshotDiscounts(a, b domain.DraftSnapshotDiscount) int {
	return strings.Compare(snapshotDiscountKey(a), snapshotDiscountKey(b))
}

func snapshotDiscountKey(discount domain.DraftSnapshotDiscount) string {
	promotion, value := "", ""
	if discount.PromotionID != nil {
		promotion = discount.PromotionID.String()
	}
	if discount.Value != nil {
		value = *discount.Value
	}
	return strings.Join([]string{string(discount.Origin), promotion, string(discount.ActionType),
		value, string(discount.Scope), fmt.Sprint(discount.Suppressed), fmt.Sprint(discount.Items)},
		"\x00")
}

func compareUUIDs(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) }
