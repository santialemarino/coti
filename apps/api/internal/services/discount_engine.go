package services

import (
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// The deterministic promotion evaluator. It reads only what a line froze when it was valued, so one
// version evaluated twice with the same lines earns the same discounts whatever the catalog did in
// between. Money rounds half-up to domain.MoneyScale once per application, as the invoice does.

// engineLine is one quote line as the evaluator reads it.
type engineLine struct {
	ID        uuid.UUID
	ProductID *uuid.UUID
	Group     domain.ProductGroup
	Quantity  decimal.Decimal
	UnitPrice decimal.NullDecimal
	MinPrice  decimal.NullDecimal // the floor; invalid = no floor, never a floor of zero.
	Subtotal  decimal.NullDecimal
}

// engineHeldDiscount is a live discount the engine does not own (a seller's or the AI's). It is
// never recomputed here: it uses up the headroom of the lines it names, and a line one also comes
// off the net an ON_TOTAL rule is measured on. No ItemIDs means it covers the whole quote.
type engineHeldDiscount struct {
	Amount  decimal.Decimal
	ItemIDs []uuid.UUID
}

// engineInput is everything one evaluation reads.
type engineInput struct {
	Lines      []engineLine
	Promotions []domain.Promotion
	BranchID   uuid.UUID
	At         time.Time
	Suppressed []engineSuppression
	Held       []engineHeldDiscount
}

// engineSuppression is an application a seller took off a version. What it covered stays without a
// promotion: neither it nor any other rule applies there until the seller restores it.
type engineSuppression struct {
	PromotionID uuid.UUID
	Scope       domain.DiscountScope
	ItemIDs     []uuid.UUID
}

// discountCandidate is one way a promotion could apply, before conflicts are resolved.
type discountCandidate struct {
	promotion   *domain.Promotion
	actionValue decimal.Decimal
	amount      decimal.Decimal
	lineIDs     []uuid.UUID // sorted; empty for ON_TOTAL.
}

// evaluateDiscounts returns the AUTOMATIC applications a version earns, conflicts resolved as
// docs/technical/discount-engine.md lays out.
func evaluateDiscounts(in engineInput) []domain.DiscountApplication {
	if len(in.Suppressed) == 0 {
		return evaluateWith(in, func(discountCandidate) bool { return false })
	}
	// A suppression blocks what it covered only while its promotion would still be earned there —
	// exactly when restoring it would succeed — so one that stopped qualifying never strands a line.
	unsuppressed := in
	unsuppressed.Suppressed = nil
	return evaluateWith(in, suppressedBy(in.Suppressed, evaluateDiscounts(unsuppressed)))
}

func evaluateWith(in engineInput, skip func(discountCandidate) bool) []domain.DiscountApplication {
	lines := pricedLines(in.Lines)
	candidates := make([]discountCandidate, 0)
	for i := range in.Promotions {
		promotion := &in.Promotions[i]
		if !promotionInForce(*promotion, in.BranchID, in.At) {
			continue
		}
		candidates = append(candidates, promotionCandidates(promotion, lines, in.Held)...)
	}
	candidates = slices.DeleteFunc(candidates, skip)
	slices.SortStableFunc(candidates, compareCandidates)
	return resolveCandidates(candidates, lines, in.Held)
}

// suppressedBy reports whether a candidate falls on something a seller suppressed: the suppressed
// application itself, or — while that application is still earned — a line it covered, or the
// total once an ON_TOTAL one is off.
func suppressedBy(
	suppressions []engineSuppression, earned []domain.DiscountApplication,
) func(discountCandidate) bool {
	stillEarned := map[string]bool{}
	for _, app := range earned {
		stillEarned[suppressionKey(app.PromotionID, app.Scope, app.ItemIDs)] = true
	}
	keys := map[string]bool{}
	lines := map[uuid.UUID]bool{}
	total := false
	for _, s := range suppressions {
		key := suppressionKey(s.PromotionID, s.Scope, s.ItemIDs)
		keys[key] = true
		if !stillEarned[key] {
			continue
		}
		if s.Scope == domain.DiscountScopeTotal {
			total = true
		}
		for _, id := range s.ItemIDs {
			lines[id] = true
		}
	}
	return func(c discountCandidate) bool {
		if keys[suppressionKey(c.promotion.ID, domain.ScopeOf(c.promotion.ConditionType), c.lineIDs)] {
			return true
		}
		if c.promotion.ConditionType == domain.PromotionConditionOnTotal {
			return total
		}
		return slices.ContainsFunc(c.lineIDs, func(id uuid.UUID) bool { return lines[id] })
	}
}

// suppressionKey identifies an application across sweeps: a line application by its promotion and
// line, a set or total one by its promotion alone, since the lines it covers can change.
func suppressionKey(promotionID uuid.UUID, scope domain.DiscountScope, itemIDs []uuid.UUID) string {
	if scope == domain.DiscountScopeItem && len(itemIDs) == 1 {
		return promotionID.String() + "/" + itemIDs[0].String()
	}
	return promotionID.String()
}

// promotionInForce reports whether a rule may apply to a branch's quote at the given instant.
func promotionInForce(p domain.Promotion, branchID uuid.UUID, at time.Time) bool {
	if !p.IsActive {
		return false
	}
	if p.BranchID != nil && *p.BranchID != branchID {
		return false
	}
	if p.ValidFrom != nil && at.Before(*p.ValidFrom) {
		return false
	}
	return p.ValidTo == nil || !at.After(*p.ValidTo)
}

func pricedLines(all []engineLine) []engineLine {
	priced := make([]engineLine, 0, len(all))
	for _, line := range all {
		if line.Subtotal.Valid && line.UnitPrice.Valid && line.Quantity.IsPositive() {
			priced = append(priced, line)
		}
	}
	return priced
}

func promotionCandidates(
	p *domain.Promotion, lines []engineLine, held []engineHeldDiscount,
) []discountCandidate {
	switch p.ConditionType {
	case domain.PromotionConditionPerItem, domain.PromotionConditionQuantityTier:
		return lineCandidates(p, lines, held)
	case domain.PromotionConditionItemSet:
		if c, ok := setCandidate(p, lines, held); ok {
			return []discountCandidate{c}
		}
	case domain.PromotionConditionOnTotal:
		if p.ActionType == domain.PromotionActionSpecialPrice {
			return nil
		}
		// Ranked against the others on the gross; the real amount is computed on the net later.
		gross := sumLines(lines, func(l engineLine) decimal.Decimal { return l.Subtotal.Decimal })
		return []discountCandidate{{promotion: p, actionValue: p.ActionValue,
			amount: baseAmount(p.ActionType, p.ActionValue, gross)}}
	}
	return nil
}

// lineCandidates is one candidate per line a PER_ITEM or QUANTITY_TIERED rule covers.
func lineCandidates(p *domain.Promotion, lines []engineLine, held []engineHeldDiscount) []discountCandidate {
	var out []discountCandidate
	for _, line := range lines {
		if !matchesAnyCondition(p.Conditions, line) {
			continue
		}
		value, ok := lineFigure(*p, line.Quantity)
		if !ok {
			continue
		}
		amount := decimal.Min(lineAmount(p.ActionType, value, line),
			remainingHeadroom([]engineLine{line}, held))
		if !amount.IsPositive() {
			continue
		}
		out = append(out, discountCandidate{promotion: p, actionValue: value, amount: amount,
			lineIDs: []uuid.UUID{line.ID}})
	}
	return out
}

// setCandidate applies an ITEM_SET rule once, when every condition is met by some line.
func setCandidate(p *domain.Promotion, lines []engineLine, held []engineHeldDiscount) (discountCandidate, bool) {
	if len(p.Conditions) == 0 || p.ActionType == domain.PromotionActionSpecialPrice {
		return discountCandidate{}, false
	}
	covered := map[uuid.UUID]engineLine{}
	for _, condition := range p.Conditions {
		found := false
		for _, line := range lines {
			if matchesCondition(condition, line) {
				covered[line.ID] = line
				found = true
			}
		}
		if !found {
			return discountCandidate{}, false
		}
	}
	setLines := make([]engineLine, 0, len(covered))
	for _, line := range covered {
		setLines = append(setLines, line)
	}
	base := sumLines(setLines, func(l engineLine) decimal.Decimal { return l.Subtotal.Decimal })
	amount := decimal.Min(baseAmount(p.ActionType, p.ActionValue, base),
		remainingHeadroom(setLines, held))
	if !amount.IsPositive() {
		return discountCandidate{}, false
	}
	return discountCandidate{promotion: p, actionValue: p.ActionValue, amount: amount,
		lineIDs: sortedLineIDs(setLines)}, true
}

// compareCandidates ranks by priority, then the larger discount, then ids so equal rules always
// resolve the same way.
func compareCandidates(a, b discountCandidate) int {
	if a.promotion.Priority != b.promotion.Priority {
		return b.promotion.Priority - a.promotion.Priority
	}
	if c := b.amount.Cmp(a.amount); c != 0 {
		return c
	}
	if c := strings.Compare(a.promotion.ID.String(), b.promotion.ID.String()); c != 0 {
		return c
	}
	return strings.Compare(firstLineID(a.lineIDs), firstLineID(b.lineIDs))
}

func resolveCandidates(
	candidates []discountCandidate, lines []engineLine, held []engineHeldDiscount,
) []domain.DiscountApplication {
	claimed := map[uuid.UUID]bool{}
	locked := map[uuid.UUID]bool{}
	var accepted []discountCandidate
	var pendingTotals []discountCandidate
	for _, c := range candidates {
		if c.promotion.ConditionType == domain.PromotionConditionOnTotal {
			if !c.promotion.IsExclusive {
				pendingTotals = append(pendingTotals, c)
				continue
			}
			// Its scope is the whole quote: anything that already ranked above it wins.
			if len(accepted) > 0 {
				continue
			}
			base, headroom := totalBase(lines, nil, nil, held)
			if viableTotal(pendingTotals, base, headroom) {
				continue
			}
			if amount, ok := totalAmount(c, base, headroom); ok {
				c.amount = amount
				return []domain.DiscountApplication{toApplication(c)}
			}
			continue
		}
		if slices.ContainsFunc(c.lineIDs, func(id uuid.UUID) bool { return claimed[id] }) {
			continue
		}
		for _, id := range c.lineIDs {
			claimed[id] = true
			if c.promotion.IsExclusive {
				locked[id] = true
			}
		}
		accepted = append(accepted, c)
	}

	applications := make([]domain.DiscountApplication, 0, len(accepted)+1)
	for _, c := range accepted {
		applications = append(applications, toApplication(c))
	}
	base, headroom := totalBase(lines, locked, accepted, held)
	for _, c := range pendingTotals {
		if amount, ok := totalAmount(c, base, headroom); ok {
			c.amount = amount
			applications = append(applications, toApplication(c))
			break
		}
	}
	return applications
}

// totalBase is the net an ON_TOTAL rule acts on, and how far it may go before a floor binds: the
// lines no exclusive rule holds, less every line discount already on them.
func totalBase(
	lines []engineLine, locked map[uuid.UUID]bool, accepted []discountCandidate,
	held []engineHeldDiscount,
) (decimal.Decimal, decimal.Decimal) {
	base, headroom := decimal.Zero, decimal.Zero
	for _, line := range lines {
		if locked[line.ID] {
			continue
		}
		base = base.Add(line.Subtotal.Decimal)
		headroom = headroom.Add(lineHeadroom(line))
	}
	for _, c := range accepted {
		if c.promotion.IsExclusive {
			continue
		}
		base = base.Sub(c.amount)
		headroom = headroom.Sub(c.amount)
	}
	for _, h := range held {
		if slices.ContainsFunc(h.ItemIDs, func(id uuid.UUID) bool { return locked[id] }) {
			continue
		}
		// A quote-wide one is not a line discount, so it leaves the net alone but still uses headroom.
		if len(h.ItemIDs) > 0 {
			base = base.Sub(h.Amount)
		}
		headroom = headroom.Sub(h.Amount)
	}
	return decimal.Max(base, decimal.Zero), decimal.Max(headroom, decimal.Zero)
}

// totalAmount is what an ON_TOTAL rule takes off a net, within the headroom, or false when its
// threshold is not met or nothing would come off.
func totalAmount(c discountCandidate, base, headroom decimal.Decimal) (decimal.Decimal, bool) {
	if c.promotion.MinTotal != nil && base.LessThan(*c.promotion.MinTotal) {
		return decimal.Zero, false
	}
	amount := decimal.Min(baseAmount(c.promotion.ActionType, c.promotion.ActionValue, base), headroom)
	return amount, amount.IsPositive()
}

func viableTotal(totals []discountCandidate, base, headroom decimal.Decimal) bool {
	return slices.ContainsFunc(totals, func(c discountCandidate) bool {
		_, ok := totalAmount(c, base, headroom)
		return ok
	})
}

// lineFigure is the figure a line rule applies at a quantity: the rule's own, or the tier the
// quantity falls in — the one with the highest start at or below it. A quantity between one tier's
// end and the next one's start stays in the lower tier; past the last tier's end it earns nothing.
func lineFigure(p domain.Promotion, quantity decimal.Decimal) (decimal.Decimal, bool) {
	if p.ConditionType != domain.PromotionConditionQuantityTier {
		return p.ActionValue, true
	}
	var figure *domain.PromotionTier
	higherStart := false
	for i := range p.Tiers {
		tier := &p.Tiers[i]
		if quantity.LessThan(tier.FromQuantity) {
			higherStart = true
			continue
		}
		if figure == nil || tier.FromQuantity.GreaterThan(figure.FromQuantity) {
			figure = tier
		}
	}
	if figure == nil ||
		(figure.ToQuantity != nil && quantity.GreaterThan(*figure.ToQuantity) && !higherStart) {
		return decimal.Zero, false
	}
	return figure.Value, true
}

// lineAmount turns a line rule into money. Every action transforms the unit price: a percentage
// off it, a fixed amount off each unit, or a special price replacing it.
func lineAmount(action domain.PromotionActionType, value decimal.Decimal, line engineLine) decimal.Decimal {
	switch action {
	case domain.PromotionActionPercentage:
		return roundMoney(line.Subtotal.Decimal.Mul(value).Div(decimal.NewFromInt(100)))
	case domain.PromotionActionFixedAmount:
		return roundMoney(value.Mul(line.Quantity))
	case domain.PromotionActionSpecialPrice:
		return decimal.Max(line.Subtotal.Decimal.Sub(roundMoney(value.Mul(line.Quantity))), decimal.Zero)
	}
	return decimal.Zero
}

// baseAmount turns a set or total rule into money over its base: a percentage of it, or the fixed
// amount once.
func baseAmount(action domain.PromotionActionType, value, base decimal.Decimal) decimal.Decimal {
	switch action {
	case domain.PromotionActionPercentage:
		return roundMoney(base.Mul(value).Div(decimal.NewFromInt(100)))
	case domain.PromotionActionFixedAmount:
		return roundMoney(value)
	}
	return decimal.Zero
}

// lineHeadroom is how much a line can come down before it reaches its floor, or zero.
func lineHeadroom(line engineLine) decimal.Decimal {
	floor := decimal.Zero
	if line.MinPrice.Valid {
		floor = roundMoney(line.MinPrice.Decimal.Mul(line.Quantity))
	}
	return decimal.Max(line.Subtotal.Decimal.Sub(floor), decimal.Zero)
}

// remainingHeadroom is how far some lines can still come down: their headroom less every held
// discount naming any of them. A held discount spanning more lines counts whole, so the floor
// still binds however it is spread.
func remainingHeadroom(lines []engineLine, held []engineHeldDiscount) decimal.Decimal {
	headroom := sumLines(lines, lineHeadroom)
	for _, h := range held {
		if slices.ContainsFunc(lines, func(l engineLine) bool { return slices.Contains(h.ItemIDs, l.ID) }) {
			headroom = headroom.Sub(h.Amount)
		}
	}
	return decimal.Max(headroom, decimal.Zero)
}

func matchesAnyCondition(conditions []domain.PromotionCondition, line engineLine) bool {
	return slices.ContainsFunc(conditions, func(c domain.PromotionCondition) bool {
		return matchesCondition(c, line)
	})
}

// matchesCondition reports whether a line is the condition's target in at least its minimum quantity.
func matchesCondition(c domain.PromotionCondition, line engineLine) bool {
	if line.ProductID == nil {
		return false
	}
	if c.MinQuantity != nil && line.Quantity.LessThan(*c.MinQuantity) {
		return false
	}
	switch {
	case c.ProductID != nil:
		return *c.ProductID == *line.ProductID
	case c.SubgroupID != nil:
		return line.Group.SubgroupID != nil && *c.SubgroupID == *line.Group.SubgroupID
	case c.FamilyID != nil:
		return line.Group.FamilyID != nil && *c.FamilyID == *line.Group.FamilyID
	}
	return false
}

func toApplication(c discountCandidate) domain.DiscountApplication {
	return domain.DiscountApplication{
		PromotionID:   c.promotion.ID,
		ConditionType: c.promotion.ConditionType,
		Scope:         domain.ScopeOf(c.promotion.ConditionType),
		ActionType:    c.promotion.ActionType,
		ActionValue:   c.actionValue,
		Amount:        c.amount,
		ItemIDs:       c.lineIDs,
	}
}

// roundMoney closes an amount at the money scale, half-up for the non-negative values it sees.
func roundMoney(d decimal.Decimal) decimal.Decimal {
	return d.Round(domain.MoneyScale)
}

func sumLines(lines []engineLine, value func(engineLine) decimal.Decimal) decimal.Decimal {
	total := decimal.Zero
	for _, line := range lines {
		total = total.Add(value(line))
	}
	return total
}

func sortedLineIDs(lines []engineLine) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(lines))
	for _, line := range lines {
		ids = append(ids, line.ID)
	}
	slices.SortFunc(ids, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
	return ids
}

func firstLineID(ids []uuid.UUID) string {
	if len(ids) == 0 {
		return ""
	}
	return ids[0].String()
}
