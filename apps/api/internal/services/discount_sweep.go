package services

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// versionDiscountStore is the discount persistence a sweep needs.
type versionDiscountStore interface {
	ListByVersionID(ctx context.Context, q repository.Querier, accountID, versionID uuid.UUID) ([]domain.QuoteDiscount, error)
	ListItemIDsByDiscountIDs(ctx context.Context, q repository.Querier, accountID, versionID uuid.UUID, discountIDs []uuid.UUID) (map[uuid.UUID][]uuid.UUID, error)
	CreateApplications(ctx context.Context, q repository.Querier, accountID, versionID uuid.UUID, apps []domain.DiscountApplication) error
	UpdateAmount(ctx context.Context, q repository.Querier, accountID, versionID, discountID uuid.UUID, amount decimal.Decimal) error
	UpdateApplication(ctx context.Context, q repository.Querier, accountID, versionID, discountID uuid.UUID, actionValue, amount decimal.Decimal) error
	ReplaceItemLinks(ctx context.Context, q repository.Querier, accountID, versionID, discountID uuid.UUID, itemIDs []uuid.UUID) error
	DeleteApplications(ctx context.Context, q repository.Querier, accountID, versionID uuid.UUID, discountIDs []uuid.UUID) error
}

// promotionReader is what a sweep reads the account's rules and the catalog taxonomy through.
type promotionReader interface {
	ListInForce(ctx context.Context, q repository.Querier, accountID, branchID uuid.UUID, at time.Time) ([]domain.Promotion, error)
	ListProductGroups(ctx context.Context, q repository.Querier, accountID uuid.UUID, productIDs []uuid.UUID) (map[uuid.UUID]domain.ProductGroup, error)
}

// discountSweep settles a mutable version's discounts: seller percentages follow their lines, the
// promotion engine re-earns the AUTOMATIC applications, and the total follows. Without promotions
// wired it only does the first and the last.
type discountSweep struct {
	discounts  versionDiscountStore
	promotions promotionReader
	now        func() time.Time
}

// settle re-evaluates a mutable version's discounts against its lines as stored and returns the
// total the version now carries. It runs inside the caller's transaction.
func (s *discountSweep) settle(
	ctx context.Context, q repository.Querier, accountID, branchID, versionID uuid.UUID,
	items []domain.QuoteItem,
) (decimal.Decimal, error) {
	discounts, err := s.discounts.ListByVersionID(ctx, q, accountID, versionID)
	if err != nil {
		return decimal.Zero, err
	}
	links, err := s.discounts.ListItemIDsByDiscountIDs(ctx, q, accountID, versionID,
		discountIDs(discounts))
	if err != nil {
		return decimal.Zero, err
	}

	live := decimal.Zero
	held := make([]engineHeldDiscount, 0)
	suppressed := map[string]struct{}{}
	automatic := make([]domain.QuoteDiscount, 0)
	for _, discount := range discounts {
		covered := links[discount.ID]
		if discount.Origin == domain.DiscountOriginAutomatic {
			if discount.SuppressedBySeller {
				if discount.PromotionID != nil {
					suppressed[suppressionKey(*discount.PromotionID, discount.Scope, covered)] = struct{}{}
				}
			} else {
				automatic = append(automatic, discount)
			}
			continue
		}
		if discount.SuppressedBySeller {
			continue
		}
		amount, err := s.followLines(ctx, q, accountID, versionID, discount, items, covered)
		if err != nil {
			return decimal.Zero, err
		}
		live = live.Add(amount)
		if discount.Scope != domain.DiscountScopeTotal {
			held = append(held, engineHeldDiscount{Amount: amount, ItemIDs: covered})
		}
	}

	earned, err := s.earn(ctx, q, accountID, branchID, items, suppressed, held)
	if err != nil {
		return decimal.Zero, err
	}
	if err := s.persist(ctx, q, accountID, versionID, automatic, links, earned); err != nil {
		return decimal.Zero, err
	}
	for _, app := range earned {
		live = live.Add(app.Amount)
	}

	// quote_version.total = Σ subtotals − Σ live discounts. A seller may discount past the net,
	// so the total bottoms out at zero instead of carrying a sign.
	total := sumItemSubtotals(items).Sub(live)
	if total.IsNegative() {
		total = decimal.Zero
	}
	return roundMoney(total), nil
}

// followLines returns a seller or AI discount's money, rewriting it when it is a percentage whose
// lines moved underneath it.
func (s *discountSweep) followLines(
	ctx context.Context, q repository.Querier, accountID, versionID uuid.UUID,
	discount domain.QuoteDiscount, items []domain.QuoteItem, covered []uuid.UUID,
) (decimal.Decimal, error) {
	if discount.Origin != domain.DiscountOriginManualSeller ||
		discount.ActionType != domain.PromotionActionPercentage || discount.ActionValue == nil {
		return discount.Amount, nil
	}
	base, err := scopeBaseForItems(items, discount.Scope, covered)
	if err != nil {
		return decimal.Zero, err
	}
	computed := ruleAmount(base, discount.ActionType, *discount.ActionValue)
	if computed.Equal(discount.Amount) {
		return computed, nil
	}
	if err := s.discounts.UpdateAmount(ctx, q, accountID, versionID, discount.ID,
		computed); err != nil {
		return decimal.Zero, err
	}
	return computed, nil
}

// earn runs the engine over the version's lines, or earns nothing when no rules are wired.
func (s *discountSweep) earn(
	ctx context.Context, q repository.Querier, accountID, branchID uuid.UUID,
	items []domain.QuoteItem, suppressed map[string]struct{}, held []engineHeldDiscount,
) ([]domain.DiscountApplication, error) {
	if s.promotions == nil {
		return nil, nil
	}
	at := s.now()
	promotions, err := s.promotions.ListInForce(ctx, q, accountID, branchID, at)
	if err != nil || len(promotions) == 0 {
		return nil, err
	}
	groups, err := s.promotions.ListProductGroups(ctx, q, accountID, quoteItemProductIDs(items))
	if err != nil {
		return nil, err
	}
	lines := make([]engineLine, 0, len(items))
	for _, item := range items {
		line := engineLine{ID: item.ID, ProductID: item.ProductID, Quantity: item.Quantity,
			UnitPrice: item.UnitPriceSnapshot, MinPrice: item.MinPriceSnapshot,
			Subtotal: item.Subtotal}
		if item.ProductID != nil {
			line.Group = groups[*item.ProductID]
		}
		lines = append(lines, line)
	}
	return evaluateDiscounts(engineInput{Lines: lines, Promotions: promotions, BranchID: branchID,
		At: at, Suppressed: suppressed, Held: held}), nil
}

// persist turns the stored AUTOMATIC rows into the earned set: a row whose application is still
// earned keeps its id and is updated in place, the rest are dropped, and new ones are written.
func (s *discountSweep) persist(
	ctx context.Context, q repository.Querier, accountID, versionID uuid.UUID,
	stored []domain.QuoteDiscount, links map[uuid.UUID][]uuid.UUID,
	earned []domain.DiscountApplication,
) error {
	byKey := make(map[string]domain.QuoteDiscount, len(stored))
	for _, discount := range stored {
		if discount.PromotionID == nil {
			continue
		}
		byKey[suppressionKey(*discount.PromotionID, discount.Scope, links[discount.ID])] = discount
	}
	fresh := make([]domain.DiscountApplication, 0)
	kept := map[uuid.UUID]bool{}
	for _, app := range earned {
		existing, ok := byKey[suppressionKey(app.PromotionID, app.Scope, app.ItemIDs)]
		if !ok || kept[existing.ID] {
			app.ID = uuid.New()
			fresh = append(fresh, app)
			continue
		}
		kept[existing.ID] = true
		if existing.ActionValue == nil || !existing.ActionValue.Equal(app.ActionValue) ||
			!existing.Amount.Equal(app.Amount) {
			if err := s.discounts.UpdateApplication(ctx, q, accountID, versionID, existing.ID,
				app.ActionValue, app.Amount); err != nil {
				return err
			}
		}
		if !sameIDs(links[existing.ID], app.ItemIDs) {
			if err := s.discounts.ReplaceItemLinks(ctx, q, accountID, versionID, existing.ID,
				app.ItemIDs); err != nil {
				return err
			}
		}
	}
	dropped := make([]uuid.UUID, 0)
	for _, discount := range stored {
		if !kept[discount.ID] {
			dropped = append(dropped, discount.ID)
		}
	}
	if err := s.discounts.DeleteApplications(ctx, q, accountID, versionID, dropped); err != nil {
		return err
	}
	return s.discounts.CreateApplications(ctx, q, accountID, versionID, fresh)
}

func discountIDs(discounts []domain.QuoteDiscount) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(discounts))
	for _, discount := range discounts {
		ids = append(ids, discount.ID)
	}
	return ids
}

func sameIDs(a, b []uuid.UUID) bool {
	byString := func(x, y uuid.UUID) int { return strings.Compare(x.String(), y.String()) }
	a, b = slices.Clone(a), slices.Clone(b)
	slices.SortFunc(a, byString)
	slices.SortFunc(b, byString)
	return slices.Equal(a, b)
}
