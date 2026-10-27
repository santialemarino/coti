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
// promotion engine re-earns the applications it owns, and the total follows. Without promotions
// wired it only does the first and the last.
type discountSweep struct {
	discounts  versionDiscountStore
	promotions promotionReader
	now        func() time.Time
}

// sweepState is everything a sweep of one version reads before it decides anything, so the same
// state can be evaluated against candidate lines without another round trip.
type sweepState struct {
	discounts  []domain.QuoteDiscount
	links      map[uuid.UUID][]uuid.UUID
	promotions []domain.Promotion
	groups     map[uuid.UUID]domain.ProductGroup
	at         time.Time
}

// sweepResult is what a sweep decides for a set of lines, before anything is written.
type sweepResult struct {
	total decimal.Decimal
	// engine are the stored engine-owned applications still live, which the earned set replaces.
	engine   []domain.QuoteDiscount
	earned   []domain.DiscountApplication
	followed []followedAmount
}

// followedAmount is a seller discount whose money moved with its lines.
type followedAmount struct {
	discountID uuid.UUID
	amount     decimal.Decimal
}

// settle re-evaluates a mutable version's discounts against its lines as stored and returns the
// total the version now carries. It runs inside the caller's transaction.
func (s *discountSweep) settle(
	ctx context.Context, q repository.Querier, accountID, branchID, versionID uuid.UUID,
	items []domain.QuoteItem,
) (decimal.Decimal, error) {
	state, err := s.load(ctx, q, accountID, branchID, versionID, quoteItemProductIDs(items))
	if err != nil {
		return decimal.Zero, err
	}
	result, err := state.evaluate(branchID, items)
	if err != nil {
		return decimal.Zero, err
	}
	for _, moved := range result.followed {
		if err := s.discounts.UpdateAmount(ctx, q, accountID, versionID, moved.discountID,
			moved.amount); err != nil {
			return decimal.Zero, err
		}
	}
	if err := s.persist(ctx, q, accountID, versionID, result.engine, state.links,
		result.earned); err != nil {
		return decimal.Zero, err
	}
	return result.total, nil
}

// load reads a version's discounts with their lines, and the rules in force with the taxonomy of
// the given products. No rules are read when none are wired.
func (s *discountSweep) load(
	ctx context.Context, q repository.Querier, accountID, branchID, versionID uuid.UUID,
	productIDs []uuid.UUID,
) (sweepState, error) {
	discounts, err := s.discounts.ListByVersionID(ctx, q, accountID, versionID)
	if err != nil {
		return sweepState{}, err
	}
	links, err := s.discounts.ListItemIDsByDiscountIDs(ctx, q, accountID, versionID,
		discountIDs(discounts))
	if err != nil {
		return sweepState{}, err
	}
	state := sweepState{discounts: discounts, links: links}
	if s.promotions == nil {
		return state, nil
	}
	state.at = s.now()
	if state.promotions, err = s.promotions.ListInForce(ctx, q, accountID, branchID,
		state.at); err != nil || len(state.promotions) == 0 {
		return state, err
	}
	state.groups, err = s.promotions.ListProductGroups(ctx, q, accountID, productIDs)
	return state, err
}

// evaluate decides the version's discounts and total for the given lines, writing nothing.
func (st sweepState) evaluate(branchID uuid.UUID, items []domain.QuoteItem) (sweepResult, error) {
	result := sweepResult{engine: make([]domain.QuoteDiscount, 0)}
	live := decimal.Zero
	held := make([]engineHeldDiscount, 0)
	suppressed := make([]engineSuppression, 0)
	for _, discount := range st.discounts {
		covered := st.links[discount.ID]
		if discount.Origin.EngineOwned() {
			if discount.SuppressedBySeller {
				if discount.PromotionID != nil {
					suppressed = append(suppressed, engineSuppression{PromotionID: *discount.PromotionID,
						Scope: discount.Scope, ItemIDs: covered})
				}
			} else {
				result.engine = append(result.engine, discount)
			}
			continue
		}
		if discount.SuppressedBySeller {
			continue
		}
		amount, err := followLines(discount, items, covered)
		if err != nil {
			return sweepResult{}, err
		}
		if !amount.Equal(discount.Amount) {
			result.followed = append(result.followed, followedAmount{discountID: discount.ID,
				amount: amount})
		}
		live = live.Add(amount)
		heldOn := covered
		if discount.Scope == domain.DiscountScopeTotal {
			heldOn = nil
		}
		held = append(held, engineHeldDiscount{Amount: amount, ItemIDs: heldOn})
	}

	result.earned = st.earn(branchID, items, suppressed, held)
	for _, app := range result.earned {
		live = live.Add(app.Amount)
	}

	// quote_version.total = Σ subtotals − Σ live discounts. A seller may discount past the net,
	// so the total bottoms out at zero instead of carrying a sign.
	total := sumItemSubtotals(items).Sub(live)
	if total.IsNegative() {
		total = decimal.Zero
	}
	result.total = roundMoney(total)
	return result, nil
}

// followLines returns a seller or AI discount's money for the lines as they stand: a seller's
// percentage follows them, a seller's fixed amount is capped at what they still carry.
func followLines(
	discount domain.QuoteDiscount, items []domain.QuoteItem, covered []uuid.UUID,
) (decimal.Decimal, error) {
	if discount.Origin != domain.DiscountOriginManualSeller || discount.ActionValue == nil {
		return discount.Amount, nil
	}
	base, err := scopeBaseForItems(items, discount.Scope, covered)
	if err != nil {
		return decimal.Zero, err
	}
	computed := ruleAmount(base, discount.ActionType, *discount.ActionValue)
	// A fixed amount never outgrows the lines it still covers, as when it was typed.
	if discount.ActionType == domain.PromotionActionFixedAmount &&
		discount.Scope != domain.DiscountScopeTotal {
		computed = decimal.Min(computed, base)
	}
	return computed, nil
}

// earn runs the engine over the version's lines, or earns nothing when no rules are in force.
func (st sweepState) earn(
	branchID uuid.UUID, items []domain.QuoteItem, suppressed []engineSuppression,
	held []engineHeldDiscount,
) []domain.DiscountApplication {
	if len(st.promotions) == 0 {
		return nil
	}
	lines := make([]engineLine, 0, len(items))
	for _, item := range items {
		line := engineLine{ID: item.ID, ProductID: item.ProductID, Quantity: item.Quantity,
			UnitPrice: item.UnitPriceSnapshot, MinPrice: item.MinPriceSnapshot,
			Subtotal: item.Subtotal}
		if item.ProductID != nil {
			line.Group = st.groups[*item.ProductID]
		}
		lines = append(lines, line)
	}
	return evaluateDiscounts(engineInput{Lines: lines, Promotions: st.promotions, BranchID: branchID,
		At: st.at, Suppressed: suppressed, Held: held})
}

// persist turns the stored engine-owned rows into the earned set: a row whose application is
// still earned keeps its id and origin and is updated in place, the rest are dropped, and new ones
// are written as AUTOMATIC.
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
