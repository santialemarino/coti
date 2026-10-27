package services

import (
	"context"
	"slices"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// adaptRaise is one draft line raised to the threshold of a promotion it does not yet reach.
type adaptRaise struct {
	item      domain.QuoteItem
	to        decimal.Decimal
	promotion domain.Promotion
}

// adaptToLowerTotal runs ADAPT_TO_REACH_DISCOUNT for a lower-price goal: it raises the targeted
// lines to a promotion threshold only where the engine's total for the draft ends strictly lower.
func (h *BatchChangeHandler) adaptToLowerTotal(ctx context.Context, q repository.Querier,
	quote domain.Quote, draft domain.QuoteVersion, current []domain.QuoteItem,
	proposal domain.ChangeRequestProposal, tried []domain.HandlerStrategy,
) (handlerWrite, error) {
	exhausted := &recipeNoMatch{reason: domain.HandlerNoMatchRecipesExhausted, strategies: tried}
	baseline, err := h.sweep.settle(ctx, q, quote.AccountID, quote.BranchID, draft.ID, current)
	if err != nil {
		return handlerWrite{}, err
	}
	state, err := h.sweep.load(ctx, q, quote.AccountID, quote.BranchID, draft.ID,
		quoteItemProductIDs(current))
	if err != nil {
		return handlerWrite{}, err
	}
	named := make(map[uuid.UUID]bool, len(proposal.IntentItemIDs))
	for _, id := range proposal.IntentItemIDs {
		named[id] = true
	}
	raises, planned, err := planLowerTotalAdaptation(state, quote.BranchID, current,
		func(item domain.QuoteItem) bool { return len(named) == 0 || named[item.ID] })
	if err != nil {
		return handlerWrite{}, err
	}
	if len(raises) == 0 || !planned.LessThan(baseline) {
		return handlerWrite{}, exhausted
	}
	adaptation, total, err := h.applyRaises(ctx, q, quote, draft.ID, state, raises, baseline)
	if err != nil {
		return handlerWrite{}, err
	}
	if !total.LessThan(baseline) {
		return handlerWrite{}, exhausted
	}
	if _, err := h.quotes.UpdateVersionTotal(ctx, q, quote.AccountID, draft.ID, total); err != nil {
		return handlerWrite{}, err
	}
	return handlerWrite{operations: raiseOperations(raises), strategies: tried,
		adaptation: adaptation}, nil
}

// reachPromotions makes promotions the customer asked for live by raising the lines each one
// targets below its threshold; the total may go up, since the customer chose the promotion. Lines
// the proposal itself requantified keep the customer's quantity. A promotion no raise earns is a
// no-match.
func (h *BatchChangeHandler) reachPromotions(ctx context.Context, q repository.Querier,
	quote domain.Quote, draftID uuid.UUID, items []domain.QuoteItem, missing []uuid.UUID,
	requantified map[uuid.UUID]bool, total decimal.Decimal,
) (decimal.Decimal, []adaptRaise, *domain.HandlerAdaptation, error) {
	notEarned := &recipeNoMatch{reason: domain.HandlerNoMatchPromotionNotEarned}
	state, err := h.sweep.load(ctx, q, quote.AccountID, quote.BranchID, draftID,
		quoteItemProductIDs(items))
	if err != nil {
		return decimal.Zero, nil, nil, err
	}
	planned := slices.Clone(items)
	raises := make([]adaptRaise, 0)
	for _, promotionID := range missing {
		index := slices.IndexFunc(state.promotions, func(p domain.Promotion) bool {
			return p.ID == promotionID
		})
		if index < 0 {
			return decimal.Zero, nil, nil, notEarned
		}
		reached, err := planPromotionReach(state, quote.BranchID, planned,
			state.promotions[index], func(item domain.QuoteItem) bool { return !requantified[item.ID] })
		if err != nil {
			return decimal.Zero, nil, nil, err
		}
		if len(reached) == 0 {
			return decimal.Zero, nil, nil, notEarned
		}
		for _, raise := range reached {
			planned = withQuantity(planned, raise.item.ID, raise.to)
			// A line two promotions raise is written once, from its quoted quantity to the last.
			if at := slices.IndexFunc(raises, func(r adaptRaise) bool {
				return r.item.ID == raise.item.ID
			}); at >= 0 {
				raises[at].to, raises[at].promotion = raise.to, raise.promotion
				continue
			}
			raises = append(raises, raise)
		}
	}
	adaptation, after, err := h.applyRaises(ctx, q, quote, draftID, state, raises, total)
	if err != nil {
		return decimal.Zero, nil, nil, err
	}
	discounts, err := h.discounts.ListByVersionID(ctx, q, quote.AccountID, draftID)
	if err != nil {
		return decimal.Zero, nil, nil, err
	}
	for _, promotionID := range missing {
		if !promotionLive(discounts, promotionID) {
			return decimal.Zero, nil, nil, notEarned
		}
	}
	adaptation.TotalRaised = after.GreaterThan(total)
	return after, raises, adaptation, nil
}

// applyRaises writes the raised quantities, re-sweeps the draft and marks what the raises earned
// as AI_ADAPTATION. before is the live state the raises were planned on.
func (h *BatchChangeHandler) applyRaises(ctx context.Context, q repository.Querier,
	quote domain.Quote, draftID uuid.UUID, before sweepState, raises []adaptRaise,
	totalBefore decimal.Decimal,
) (*domain.HandlerAdaptation, decimal.Decimal, error) {
	updates := make([]domain.QuoteItemQuantityUpdate, 0, len(raises))
	raised := make(map[uuid.UUID]bool, len(raises))
	for _, raise := range raises {
		updates = append(updates, domain.QuoteItemQuantityUpdate{ItemID: raise.item.ID,
			Quantity: raise.to})
		raised[raise.item.ID] = true
	}
	if err := h.quotes.UpdateItemQuantities(ctx, q, quote.AccountID, draftID, updates); err != nil {
		return nil, decimal.Zero, err
	}
	items, err := h.quotes.ListItems(ctx, q, quote.AccountID, draftID)
	if err != nil {
		return nil, decimal.Zero, err
	}
	total, err := h.sweep.settle(ctx, q, quote.AccountID, quote.BranchID, draftID, items)
	if err != nil {
		return nil, decimal.Zero, err
	}

	earnedBefore := make(map[string]bool)
	for _, discount := range before.discounts {
		if discount.Origin.EngineOwned() && !discount.SuppressedBySeller && discount.PromotionID != nil {
			earnedBefore[applicationKey(discount, before.links[discount.ID])] = true
		}
	}
	discounts, err := h.discounts.ListByVersionID(ctx, q, quote.AccountID, draftID)
	if err != nil {
		return nil, decimal.Zero, err
	}
	links, err := h.discounts.ListItemIDsByDiscountIDs(ctx, q, quote.AccountID, draftID,
		discountIDs(discounts))
	if err != nil {
		return nil, decimal.Zero, err
	}
	marked := make([]uuid.UUID, 0)
	earnedOn := make(map[uuid.UUID]domain.QuoteDiscount)
	for _, discount := range discounts {
		covered := links[discount.ID]
		if discount.Origin != domain.DiscountOriginAutomatic || discount.SuppressedBySeller ||
			discount.PromotionID == nil || earnedBefore[applicationKey(discount, covered)] ||
			!slices.ContainsFunc(covered, func(id uuid.UUID) bool { return raised[id] }) {
			continue
		}
		marked = append(marked, discount.ID)
		for _, id := range covered {
			if _, ok := earnedOn[id]; !ok && raised[id] {
				earnedOn[id] = discount
			}
		}
	}
	if err := h.discounts.MarkAdaptations(ctx, q, quote.AccountID, draftID, marked); err != nil {
		return nil, decimal.Zero, err
	}

	adaptation := &domain.HandlerAdaptation{Lines: make([]domain.HandlerAdaptedLine, 0, len(raises)),
		TotalBefore: totalBefore.StringFixed(domain.MoneyScale),
		TotalAfter:  total.StringFixed(domain.MoneyScale)}
	for _, raise := range raises {
		line := domain.HandlerAdaptedLine{ItemID: raise.item.ID,
			Description:  raise.item.RequestedDescription,
			FromQuantity: raise.item.Quantity.StringFixed(domain.MoneyScale),
			ToQuantity:   raise.to.StringFixed(domain.MoneyScale),
			PromotionID:  raise.promotion.ID, PromotionName: raise.promotion.Name}
		if raise.item.ProductName != nil {
			line.Description = *raise.item.ProductName
		}
		// The engine may settle the line on another promotion the raise reached; name that one.
		if earned, ok := earnedOn[raise.item.ID]; ok && earned.PromotionName != nil {
			line.PromotionID, line.PromotionName = *earned.PromotionID, *earned.PromotionName
		}
		adaptation.Lines = append(adaptation.Lines, line)
	}
	return adaptation, total, nil
}

// planLowerTotalAdaptation walks the draft's lines in order and, for each one in scope, keeps the
// raise to a promotion threshold that leaves the engine's total lowest, only when it is lower than
// the total the walk had reached. Returns the raises and the total they plan for.
func planLowerTotalAdaptation(state sweepState, branchID uuid.UUID, items []domain.QuoteItem,
	inScope func(domain.QuoteItem) bool,
) ([]adaptRaise, decimal.Decimal, error) {
	suppressed := suppressedPromotions(state)
	current := slices.Clone(items)
	result, err := state.evaluate(branchID, current)
	if err != nil {
		return nil, decimal.Zero, err
	}
	reached := result.total
	raises := make([]adaptRaise, 0)
	for _, item := range items {
		if !inScope(item) {
			continue
		}
		var best *adaptRaise
		bestTotal := reached
		for _, promotion := range state.promotions {
			if suppressed[promotion.ID] || !promotionInForce(promotion, branchID, state.at) {
				continue
			}
			to, ok := nextThreshold(promotion, item, state.groups)
			if !ok {
				continue
			}
			trial, err := state.evaluate(branchID, withQuantity(current, item.ID, to))
			if err != nil {
				return nil, decimal.Zero, err
			}
			if trial.total.LessThan(bestTotal) {
				best = &adaptRaise{item: item, to: to, promotion: promotion}
				bestTotal = trial.total
			}
		}
		if best != nil {
			current = withQuantity(current, item.ID, best.to)
			reached = bestTotal
			raises = append(raises, *best)
		}
	}
	return raises, reached, nil
}

// planPromotionReach raises each line in scope that the promotion targets below its threshold to
// the least quantity that earns it, keeping a raise only when the engine then applies the promotion
// on that line.
func planPromotionReach(state sweepState, branchID uuid.UUID, items []domain.QuoteItem,
	promotion domain.Promotion, inScope func(domain.QuoteItem) bool,
) ([]adaptRaise, error) {
	if !promotionInForce(promotion, branchID, state.at) {
		return nil, nil
	}
	current := slices.Clone(items)
	raises := make([]adaptRaise, 0)
	for _, item := range items {
		if !inScope(item) {
			continue
		}
		to, ok := nextThreshold(promotion, item, state.groups)
		if !ok {
			continue
		}
		trial := withQuantity(current, item.ID, to)
		result, err := state.evaluate(branchID, trial)
		if err != nil {
			return nil, err
		}
		if slices.ContainsFunc(result.earned, func(app domain.DiscountApplication) bool {
			return app.PromotionID == promotion.ID && slices.Contains(app.ItemIDs, item.ID)
		}) {
			current = trial
			raises = append(raises, adaptRaise{item: item, to: to, promotion: promotion})
		}
	}
	return raises, nil
}

// nextThreshold is the least quantity above the line's at which a PER_ITEM or QUANTITY_TIERED
// promotion targets it and earns a figure it does not earn there already. Other rules, unpriced
// lines and lines without a product have none.
func nextThreshold(promotion domain.Promotion, item domain.QuoteItem,
	groups map[uuid.UUID]domain.ProductGroup,
) (decimal.Decimal, bool) {
	tiered := promotion.ConditionType == domain.PromotionConditionQuantityTier
	if (!tiered && promotion.ConditionType != domain.PromotionConditionPerItem) ||
		item.ProductID == nil || !item.UnitPriceSnapshot.Valid || !item.Quantity.IsPositive() {
		return decimal.Zero, false
	}
	line := engineLine{ID: item.ID, ProductID: item.ProductID, Group: groups[*item.ProductID],
		Quantity: item.Quantity}
	current, earning := lineFigure(promotion, item.Quantity)
	earning = earning && matchesAnyCondition(promotion.Conditions, line)

	var best decimal.Decimal
	found := false
	for _, condition := range promotion.Conditions {
		if !conditionTargets(condition, line) {
			continue
		}
		floor := decimal.Zero
		if condition.MinQuantity != nil {
			floor = *condition.MinQuantity
		}
		candidates := make([]decimal.Decimal, 0, len(promotion.Tiers)+1)
		if floor.GreaterThan(item.Quantity) {
			candidates = append(candidates, floor)
		}
		if tiered {
			for _, tier := range promotion.Tiers {
				if tier.FromQuantity.GreaterThan(item.Quantity) {
					candidates = append(candidates, decimal.Max(tier.FromQuantity, floor))
				}
			}
		}
		for _, candidate := range candidates {
			figure, ok := lineFigure(promotion, candidate)
			if !ok || (earning && figure.Equal(current)) {
				continue
			}
			if !found || candidate.LessThan(best) {
				best, found = candidate, true
			}
		}
	}
	return best, found
}

// suppressedPromotions is every promotion the seller took off the draft somewhere.
func suppressedPromotions(state sweepState) map[uuid.UUID]bool {
	suppressed := make(map[uuid.UUID]bool)
	for _, discount := range state.discounts {
		if discount.Origin.EngineOwned() && discount.SuppressedBySeller && discount.PromotionID != nil {
			suppressed[*discount.PromotionID] = true
		}
	}
	return suppressed
}

// withQuantity returns a copy of the lines with one line requantified, its subtotal following.
func withQuantity(items []domain.QuoteItem, itemID uuid.UUID, quantity decimal.Decimal,
) []domain.QuoteItem {
	result := slices.Clone(items)
	for i := range result {
		if result[i].ID != itemID {
			continue
		}
		result[i].Quantity = quantity
		if result[i].UnitPriceSnapshot.Valid {
			result[i].Subtotal = decimal.NewNullDecimal(
				roundMoney(quantity.Mul(result[i].UnitPriceSnapshot.Decimal)))
		}
	}
	return result
}

// applicationKey identifies an engine application and the figure it applied, so a raise that only
// moves a line to a better tier still counts as reached by the adaptation.
func applicationKey(discount domain.QuoteDiscount, covered []uuid.UUID) string {
	figure := ""
	if discount.ActionValue != nil {
		figure = discount.ActionValue.String()
	}
	return suppressionKey(*discount.PromotionID, discount.Scope, covered) + "@" + figure
}

func raiseOperations(raises []adaptRaise) []domain.ChangeRequestOperation {
	operations := make([]domain.ChangeRequestOperation, 0, len(raises))
	for _, raise := range raises {
		target, quantity := raise.item.ID, raise.to
		operations = append(operations, domain.ChangeRequestOperation{
			Type: domain.ChangeRequestOperationUpdateQuantity, TargetItemID: &target,
			Quantity: &quantity})
	}
	return operations
}
