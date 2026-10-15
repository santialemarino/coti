package services

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// The project's core pricing suite. Every expected amount is worked out by hand in the comment
// beside it; none is produced by the code under test.

var (
	engineAt     = time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	engineBranch = uuid.MustParse("b0000000-0000-4000-8000-000000000001")
	cement       = uuid.MustParse("d0000000-0000-4000-8000-000000000001")
	sand         = uuid.MustParse("d0000000-0000-4000-8000-000000000002")
	brick        = uuid.MustParse("d0000000-0000-4000-8000-000000000003")
	masonry      = uuid.MustParse("f0000000-0000-4000-8000-000000000001")
)

func decPtr(s string) *decimal.Decimal {
	d := dec(s)
	return &d
}

// testLine is a priced line; the subtotal is given, never derived.
func testLine(id string, product uuid.UUID, quantity, unitPrice, subtotal string, floor *string) engineLine {
	line := engineLine{
		ID: uuid.MustParse(id), ProductID: &product, Quantity: dec(quantity),
		UnitPrice: decimal.NewNullDecimal(dec(unitPrice)),
		Subtotal:  decimal.NewNullDecimal(dec(subtotal)),
	}
	if floor != nil {
		line.MinPrice = decimal.NewNullDecimal(dec(*floor))
	}
	return line
}

const (
	lineA = "10000000-0000-4000-8000-00000000000a"
	lineB = "10000000-0000-4000-8000-00000000000b"
	lineC = "10000000-0000-4000-8000-00000000000c"
)

type promo struct {
	id        string
	condition domain.PromotionConditionType
	action    domain.PromotionActionType
	value     string
	priority  int
	exclusive bool
	targets   []uuid.UUID
	tiers     []domain.PromotionTier
	minTotal  *decimal.Decimal
}

func (p promo) build() domain.Promotion {
	out := domain.Promotion{
		ID: uuid.MustParse(p.id), ConditionType: p.condition, ActionType: p.action,
		ActionValue: dec(p.value), Priority: p.priority, IsExclusive: p.exclusive,
		IsActive: true, Tiers: p.tiers, MinTotal: p.minTotal,
	}
	for _, target := range p.targets {
		out.Conditions = append(out.Conditions, domain.PromotionCondition{ProductID: &target})
	}
	return out
}

func evaluate(lines []engineLine, promos ...promo) []domain.DiscountApplication {
	return evaluateSuppressed(lines, nil, promos...)
}

func evaluateSuppressed(lines []engineLine, suppressed []engineSuppression, promos ...promo) []domain.DiscountApplication {
	built := make([]domain.Promotion, 0, len(promos))
	for _, p := range promos {
		built = append(built, p.build())
	}
	return evaluateDiscounts(engineInput{Lines: lines, Promotions: built, BranchID: engineBranch,
		At: engineAt, Suppressed: suppressed})
}

// applied summarises the applications as promotion → amount, so a test states what it expects.
func applied(apps []domain.DiscountApplication) map[string]string {
	out := map[string]string{}
	for _, app := range apps {
		key := app.PromotionID.String()[:8]
		if app.Scope == domain.DiscountScopeItem {
			key += "/" + app.ItemIDs[0].String()[34:]
		}
		out[key] = app.Amount.StringFixed(2)
	}
	return out
}

func assertApplied(t *testing.T, got []domain.DiscountApplication, want map[string]string) {
	t.Helper()
	if summary := applied(got); !reflect.DeepEqual(summary, want) {
		t.Fatalf("applied = %v, want %v", summary, want)
	}
}

const (
	promoP = "a0000000-0000-4000-8000-000000000001"
	promoQ = "a1000000-0000-4000-8000-000000000001"
	promoR = "a2000000-0000-4000-8000-000000000001"
)

func TestEvaluateDiscounts_LineActions(t *testing.T) {
	t.Parallel()
	// 10 × 1000.00 = 10000.00 on line A.
	lines := []engineLine{testLine(lineA, cement, "10", "1000.00", "10000.00", nil)}
	cases := []struct {
		name   string
		action domain.PromotionActionType
		value  string
		want   string
	}{
		{"percentage of the line", domain.PromotionActionPercentage, "10", "1000.00"},       // 10000 × 10%
		{"fixed amount off each unit", domain.PromotionActionFixedAmount, "150", "1500.00"}, // 150 × 10
		{"special unit price", domain.PromotionActionSpecialPrice, "850", "1500.00"},        // 10000 − 850 × 10
		{"special price above the price earns nothing", domain.PromotionActionSpecialPrice, "1100", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := evaluate(lines, promo{id: promoP, condition: domain.PromotionConditionPerItem,
				action: tc.action, value: tc.value, targets: []uuid.UUID{cement}})
			want := map[string]string{}
			if tc.want != "" {
				want["a0000000/0a"] = tc.want
			}
			assertApplied(t, got, want)
		})
	}
}

func TestEvaluateDiscounts_PerItemCoversOnlyItsTargetsAndMinimum(t *testing.T) {
	t.Parallel()
	lines := []engineLine{
		testLine(lineA, cement, "10", "1000.00", "10000.00", nil),
		testLine(lineB, sand, "4", "1000.00", "4000.00", nil),
	}
	p := promo{id: promoP, condition: domain.PromotionConditionPerItem,
		action: domain.PromotionActionPercentage, value: "10", targets: []uuid.UUID{cement}}
	assertApplied(t, evaluate(lines, p), map[string]string{"a0000000/0a": "1000.00"})

	// A minimum of 12 bags on the condition: the 10 on line A do not reach it.
	built := p.build()
	built.Conditions[0].MinQuantity = decPtr("12")
	got := evaluateDiscounts(engineInput{Lines: lines, Promotions: []domain.Promotion{built},
		BranchID: engineBranch, At: engineAt})
	assertApplied(t, got, map[string]string{})
}

func TestEvaluateDiscounts_FamilyConditionMatchesThroughTheTaxonomy(t *testing.T) {
	t.Parallel()
	line := testLine(lineA, cement, "10", "1000.00", "10000.00", nil)
	line.Group = domain.ProductGroup{FamilyID: &masonry}
	other := testLine(lineB, sand, "4", "1000.00", "4000.00", nil)
	p := domain.Promotion{ID: uuid.MustParse(promoP), ConditionType: domain.PromotionConditionPerItem,
		ActionType: domain.PromotionActionPercentage, ActionValue: dec("10"), IsActive: true,
		Conditions: []domain.PromotionCondition{{FamilyID: &masonry}}}
	got := evaluateDiscounts(engineInput{Lines: []engineLine{line, other},
		Promotions: []domain.Promotion{p}, BranchID: engineBranch, At: engineAt})
	assertApplied(t, got, map[string]string{"a0000000/0a": "1000.00"}) // only the masonry line
}

func TestEvaluateDiscounts_QuantityTiers(t *testing.T) {
	t.Parallel()
	tiers := []domain.PromotionTier{
		{FromQuantity: dec("10"), ToQuantity: decPtr("49"), Value: dec("10")},
		{FromQuantity: dec("50"), Value: dec("15")},
	}
	cases := []struct {
		name     string
		quantity string
		subtotal string
		want     string
	}{
		{"below the first tier", "5", "2500.00", ""},
		{"first tier, lower bound inclusive", "10", "5000.00", "500.00"}, // 5000 × 10%
		{"first tier, upper bound inclusive", "49", "24500.00", "2450.00"},
		{"open-ended tier", "60", "30000.00", "4500.00"}, // 30000 × 15%
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			lines := []engineLine{testLine(lineA, cement, tc.quantity, "500.00", tc.subtotal, nil)}
			got := evaluate(lines, promo{id: promoP, condition: domain.PromotionConditionQuantityTier,
				action: domain.PromotionActionPercentage, value: "0", tiers: tiers,
				targets: []uuid.UUID{cement}})
			want := map[string]string{}
			if tc.want != "" {
				want["a0000000/0a"] = tc.want
			}
			assertApplied(t, got, want)
		})
	}
}

func TestEvaluateDiscounts_ItemSetNeedsEveryTarget(t *testing.T) {
	t.Parallel()
	set := promo{id: promoP, condition: domain.PromotionConditionItemSet,
		action: domain.PromotionActionPercentage, value: "5", targets: []uuid.UUID{cement, sand}}
	both := []engineLine{
		testLine(lineA, cement, "10", "1000.00", "10000.00", nil),
		testLine(lineB, sand, "4", "1000.00", "4000.00", nil),
		testLine(lineC, brick, "100", "50.00", "5000.00", nil),
	}
	got := evaluate(both, set)
	// (10000 + 4000) × 5%; the brick line is not part of the set.
	assertApplied(t, got, map[string]string{"a0000000": "700.00"})
	if want := []uuid.UUID{uuid.MustParse(lineA), uuid.MustParse(lineB)}; !slices.Equal(got[0].ItemIDs, want) {
		t.Errorf("covered = %v, want %v", got[0].ItemIDs, want)
	}
	if got[0].Scope != domain.DiscountScopeItemSet {
		t.Errorf("scope = %s, want ITEM_SET", got[0].Scope)
	}

	assertApplied(t, evaluate(both[:1], set), map[string]string{}) // no sand, no set

	fixed := set
	fixed.action, fixed.value = domain.PromotionActionFixedAmount, "1200"
	assertApplied(t, evaluate(both, fixed), map[string]string{"a0000000": "1200.00"}) // once per set
}

func TestEvaluateDiscounts_OnTotalIsComputedOnTheNet(t *testing.T) {
	t.Parallel()
	lines := []engineLine{
		testLine(lineA, cement, "10", "1000.00", "10000.00", nil),
		testLine(lineB, sand, "5", "1000.00", "5000.00", nil),
	}
	line := promo{id: promoP, condition: domain.PromotionConditionPerItem,
		action: domain.PromotionActionPercentage, value: "10", targets: []uuid.UUID{cement}}
	total := promo{id: promoQ, condition: domain.PromotionConditionOnTotal,
		action: domain.PromotionActionPercentage, value: "5"}
	// Line A: 10000 × 10% = 1000. Net: 15000 − 1000 = 14000; × 5% = 700 (not 750 on the gross).
	got := evaluate(lines, line, total)
	assertApplied(t, got, map[string]string{"a0000000/0a": "1000.00", "a1000000": "700.00"})
	for _, app := range got {
		if app.PromotionID == uuid.MustParse(promoQ) &&
			(app.Scope != domain.DiscountScopeTotal || len(app.ItemIDs) != 0) {
			t.Errorf("total application = %+v, want TOTAL scope with no lines", app)
		}
	}

	// A threshold of 14500 on the net: the gross (15000) reaches it, the net (14000) does not.
	total.minTotal = decPtr("14500")
	assertApplied(t, evaluate(lines, line, total), map[string]string{"a0000000/0a": "1000.00"})

	// Seller discounts on the lines come off the net too: 14000 − 2000 = 12000; × 5% = 600.
	total.minTotal = nil
	got = evaluateDiscounts(engineInput{Lines: lines,
		Promotions: []domain.Promotion{line.build(), total.build()}, BranchID: engineBranch,
		At: engineAt, Held: []engineHeldDiscount{{Amount: dec("2000.00"),
			ItemIDs: []uuid.UUID{uuid.MustParse(lineB)}}}})
	assertApplied(t, got, map[string]string{"a0000000/0a": "1000.00", "a1000000": "600.00"})
}

func TestEvaluateDiscounts_OneTotalPromotionAtMost(t *testing.T) {
	t.Parallel()
	lines := []engineLine{testLine(lineA, cement, "10", "1000.00", "10000.00", nil)}
	high := promo{id: promoP, condition: domain.PromotionConditionOnTotal,
		action: domain.PromotionActionPercentage, value: "3", priority: 5, minTotal: decPtr("50000")}
	low := promo{id: promoQ, condition: domain.PromotionConditionOnTotal,
		action: domain.PromotionActionPercentage, value: "2", priority: 1}
	// The higher one misses its threshold, so the next one applies: 10000 × 2% = 200.
	assertApplied(t, evaluate(lines, high, low), map[string]string{"a1000000": "200.00"})
	high.minTotal = nil
	assertApplied(t, evaluate(lines, high, low), map[string]string{"a0000000": "300.00"})
}

func TestEvaluateDiscounts_TwoPromotionsOnOneLine(t *testing.T) {
	t.Parallel()
	lines := []engineLine{testLine(lineA, cement, "10", "1000.00", "10000.00", nil)}
	small := promo{id: promoP, condition: domain.PromotionConditionPerItem,
		action: domain.PromotionActionPercentage, value: "3", targets: []uuid.UUID{cement}}
	large := promo{id: promoQ, condition: domain.PromotionConditionPerItem,
		action: domain.PromotionActionPercentage, value: "5", targets: []uuid.UUID{cement}}

	// Same priority: the larger discount wins, and they never stack (500, not 800).
	assertApplied(t, evaluate(lines, small, large), map[string]string{"a1000000/0a": "500.00"})

	// Higher priority beats the larger discount.
	small.priority = 10
	assertApplied(t, evaluate(lines, small, large), map[string]string{"a0000000/0a": "300.00"})

	// A line rule and a set rule over the same line do not stack either.
	set := promo{id: promoR, condition: domain.PromotionConditionItemSet,
		action: domain.PromotionActionPercentage, value: "20", priority: 20,
		targets: []uuid.UUID{cement, sand}}
	withSand := append(lines, testLine(lineB, sand, "4", "1000.00", "4000.00", nil))
	// Set: (10000 + 4000) × 20% = 2800, and it claims line A before the 3%.
	assertApplied(t, evaluate(withSand, small, set), map[string]string{"a2000000": "2800.00"})
}

func TestEvaluateDiscounts_ExclusiveBlocksTheRest(t *testing.T) {
	t.Parallel()
	lines := []engineLine{
		testLine(lineA, cement, "10", "1000.00", "10000.00", nil),
		testLine(lineB, sand, "5", "1000.00", "5000.00", nil),
	}
	exclusiveLine := promo{id: promoP, condition: domain.PromotionConditionPerItem,
		action: domain.PromotionActionPercentage, value: "10", exclusive: true,
		targets: []uuid.UUID{cement}}
	total := promo{id: promoQ, condition: domain.PromotionConditionOnTotal,
		action: domain.PromotionActionPercentage, value: "10"}
	// Line A is held by the exclusive rule, so the total acts on line B alone: 5000 × 10% = 500,
	// not (15000 − 1000) × 10% = 1400.
	assertApplied(t, evaluate(lines, exclusiveLine, total),
		map[string]string{"a0000000/0a": "1000.00", "a1000000": "500.00"})

	// An exclusive total that outranks everything applies alone, on the gross: 15000 × 10% = 1500.
	exclusiveTotal := total
	exclusiveTotal.exclusive, exclusiveTotal.priority = true, 20
	lineRule := exclusiveLine
	lineRule.exclusive = false
	assertApplied(t, evaluate(lines, lineRule, exclusiveTotal), map[string]string{"a1000000": "1500.00"})

	// Ranked below a line rule, it yields: the line rule applies and the exclusive total does not.
	exclusiveTotal.priority, lineRule.priority = 1, 5
	assertApplied(t, evaluate(lines, lineRule, exclusiveTotal),
		map[string]string{"a0000000/0a": "1000.00"})
}

func TestEvaluateDiscounts_TheFloorBinds(t *testing.T) {
	t.Parallel()
	floor := "950.00"
	p := promo{id: promoP, condition: domain.PromotionConditionPerItem,
		action: domain.PromotionActionPercentage, value: "10", targets: []uuid.UUID{cement}}
	// Floor 950 × 10 = 9500, so the line can come down 500, not the 1000 the rule asks.
	floored := []engineLine{testLine(lineA, cement, "10", "1000.00", "10000.00", &floor)}
	assertApplied(t, evaluate(floored, p), map[string]string{"a0000000/0a": "500.00"})

	// An absent floor is no floor, never a floor of zero that would let the line go to nothing.
	open := []engineLine{testLine(lineA, cement, "10", "1000.00", "10000.00", nil)}
	assertApplied(t, evaluate(open, p), map[string]string{"a0000000/0a": "1000.00"})

	// A line already at its floor earns nothing at all.
	atFloor := "1000.00"
	assertApplied(t, evaluate([]engineLine{testLine(lineA, cement, "10", "1000.00", "10000.00",
		&atFloor)}, p), map[string]string{})

	// The total rule respects the floors too: headroom 500 on A + 5000 on B (no floor) = 5500, and
	// 50% of the net 15000 would be 7500.
	total := promo{id: promoQ, condition: domain.PromotionConditionOnTotal,
		action: domain.PromotionActionPercentage, value: "50"}
	twoLines := append(floored, testLine(lineB, sand, "5", "1000.00", "5000.00", nil))
	assertApplied(t, evaluate(twoLines, total), map[string]string{"a1000000": "5500.00"})
}

func TestEvaluateDiscounts_NeverNegative(t *testing.T) {
	t.Parallel()
	lines := []engineLine{testLine(lineA, cement, "10", "1000.00", "10000.00", nil)}
	// 1500 off each of 10 units would be 15000 on a 10000 line: it stops at the line.
	perUnit := promo{id: promoP, condition: domain.PromotionConditionPerItem,
		action: domain.PromotionActionFixedAmount, value: "1500", targets: []uuid.UUID{cement}}
	assertApplied(t, evaluate(lines, perUnit), map[string]string{"a0000000/0a": "10000.00"})

	// A fixed total larger than the net stops at the net.
	total := promo{id: promoQ, condition: domain.PromotionConditionOnTotal,
		action: domain.PromotionActionFixedAmount, value: "50000"}
	assertApplied(t, evaluate(lines, total), map[string]string{"a1000000": "10000.00"})

	// A seller already discounted past the net: nothing is left for the engine to take.
	got := evaluateDiscounts(engineInput{Lines: lines, Promotions: []domain.Promotion{total.build()},
		BranchID: engineBranch, At: engineAt,
		Held: []engineHeldDiscount{{Amount: dec("12000.00"), ItemIDs: []uuid.UUID{uuid.MustParse(lineA)}}}})
	assertApplied(t, got, map[string]string{})
}

func TestEvaluateDiscounts_RoundsHalfUpOncePerApplication(t *testing.T) {
	t.Parallel()
	// 333.35 × 10% = 33.335 → 33.34.
	lines := []engineLine{testLine(lineA, cement, "1", "333.35", "333.35", nil)}
	assertApplied(t, evaluate(lines, promo{id: promoP, condition: domain.PromotionConditionPerItem,
		action: domain.PromotionActionPercentage, value: "10", targets: []uuid.UUID{cement}}),
		map[string]string{"a0000000/0a": "33.34"})
}

func TestEvaluateDiscounts_IsDeterministic(t *testing.T) {
	t.Parallel()
	lines := []engineLine{
		testLine(lineA, cement, "10", "1000.00", "10000.00", nil),
		testLine(lineB, sand, "5", "1000.00", "5000.00", nil),
	}
	promos := []domain.Promotion{
		promo{id: promoP, condition: domain.PromotionConditionPerItem,
			action: domain.PromotionActionPercentage, value: "5", targets: []uuid.UUID{cement, sand}}.build(),
		promo{id: promoQ, condition: domain.PromotionConditionPerItem,
			action: domain.PromotionActionPercentage, value: "5", targets: []uuid.UUID{cement}}.build(),
		promo{id: promoR, condition: domain.PromotionConditionOnTotal,
			action: domain.PromotionActionPercentage, value: "2"}.build(),
	}
	in := engineInput{Lines: lines, Promotions: promos, BranchID: engineBranch, At: engineAt}
	first := evaluateDiscounts(in)
	// P and Q tie on priority and amount on line A; the id decides, and the same way every time.
	assertApplied(t, first, map[string]string{"a0000000/0a": "500.00", "a0000000/0b": "250.00",
		"a2000000": "285.00"}) // (15000 − 750) × 2%
	for range 20 {
		if again := evaluateDiscounts(in); !reflect.DeepEqual(again, first) {
			t.Fatalf("second evaluation = %+v, want %+v", again, first)
		}
	}
	reversed := slices.Clone(promos)
	slices.Reverse(reversed)
	in.Promotions = reversed
	in.Lines = []engineLine{lines[1], lines[0]}
	assertApplied(t, evaluateDiscounts(in), applied(first))
}

func TestEvaluateDiscounts_SuppressedLineStaysWithoutAPromotion(t *testing.T) {
	t.Parallel()
	lines := []engineLine{
		testLine(lineA, cement, "10", "1000.00", "10000.00", nil),
		testLine(lineB, sand, "4", "1000.00", "4000.00", nil),
	}
	high := promo{id: promoP, condition: domain.PromotionConditionPerItem,
		action: domain.PromotionActionPercentage, value: "10", priority: 5, targets: []uuid.UUID{cement}}
	low := promo{id: promoQ, condition: domain.PromotionConditionPerItem,
		action: domain.PromotionActionPercentage, value: "3", targets: []uuid.UUID{cement, sand}}
	got := evaluateSuppressed(lines, []engineSuppression{{PromotionID: uuid.MustParse(promoP),
		Scope: domain.DiscountScopeItem, ItemIDs: []uuid.UUID{uuid.MustParse(lineA)}}}, high, low)
	// The seller took P off line A, so line A carries nothing — not Q's 300. Line B keeps Q: 4000 × 3% = 120.
	assertApplied(t, got, map[string]string{"a1000000/0b": "120.00"})
}

func TestEvaluateDiscounts_SuppressedSetFreesNoLineForAnotherRule(t *testing.T) {
	t.Parallel()
	lines := []engineLine{
		testLine(lineA, cement, "10", "1000.00", "10000.00", nil),
		testLine(lineB, sand, "4", "1000.00", "4000.00", nil),
		testLine(lineC, brick, "100", "10.00", "1000.00", nil),
	}
	set := promo{id: promoP, condition: domain.PromotionConditionItemSet,
		action: domain.PromotionActionPercentage, value: "20", priority: 5,
		targets: []uuid.UUID{cement, sand}}
	line := promo{id: promoQ, condition: domain.PromotionConditionPerItem,
		action: domain.PromotionActionPercentage, value: "5", targets: []uuid.UUID{sand, brick}}
	got := evaluateSuppressed(lines, []engineSuppression{{PromotionID: uuid.MustParse(promoP),
		Scope:   domain.DiscountScopeItemSet,
		ItemIDs: []uuid.UUID{uuid.MustParse(lineA), uuid.MustParse(lineB)}}}, set, line)
	// Lines A and B were the suppressed set's; only line C earns Q: 1000 × 5% = 50.
	assertApplied(t, got, map[string]string{"a1000000/0c": "50.00"})
}

func TestEvaluateDiscounts_SuppressedTotalLeavesTheTotalWithoutAPromotion(t *testing.T) {
	t.Parallel()
	lines := []engineLine{testLine(lineA, cement, "10", "1000.00", "10000.00", nil)}
	high := promo{id: promoP, condition: domain.PromotionConditionOnTotal,
		action: domain.PromotionActionPercentage, value: "5", priority: 5}
	low := promo{id: promoQ, condition: domain.PromotionConditionOnTotal,
		action: domain.PromotionActionPercentage, value: "2"}
	onLine := promo{id: promoR, condition: domain.PromotionConditionPerItem,
		action: domain.PromotionActionPercentage, value: "1", targets: []uuid.UUID{cement}}
	got := evaluateSuppressed(lines, []engineSuppression{{PromotionID: uuid.MustParse(promoP),
		Scope: domain.DiscountScopeTotal}}, high, low, onLine)
	// No total rule takes P's place (Q's 2% would be 198); the line rule is untouched: 10000 × 1% = 100.
	assertApplied(t, got, map[string]string{"a2000000/0a": "100.00"})
}

func TestEvaluateDiscounts_ASuppressionThatNoLongerQualifiesBlocksNothing(t *testing.T) {
	t.Parallel()
	lines := []engineLine{testLine(lineA, cement, "2", "1000.00", "2000.00", nil)}
	large := promo{id: promoP, condition: domain.PromotionConditionOnTotal,
		action: domain.PromotionActionPercentage, value: "5", minTotal: decPtr("10000")}
	small := promo{id: promoQ, condition: domain.PromotionConditionOnTotal,
		action: domain.PromotionActionPercentage, value: "2", minTotal: decPtr("1000")}
	tiered := promo{id: promoR, condition: domain.PromotionConditionQuantityTier,
		action: domain.PromotionActionPercentage, value: "0", priority: 5, targets: []uuid.UUID{cement},
		tiers: []domain.PromotionTier{{FromQuantity: dec("10"), Value: dec("10")}}}
	perItem := promo{id: "a3000000-0000-4000-8000-000000000001",
		condition: domain.PromotionConditionPerItem, action: domain.PromotionActionPercentage,
		value: "1", targets: []uuid.UUID{cement}}
	got := evaluateSuppressed(lines, []engineSuppression{
		{PromotionID: uuid.MustParse(promoP), Scope: domain.DiscountScopeTotal},
		{PromotionID: uuid.MustParse(promoR), Scope: domain.DiscountScopeItem,
			ItemIDs: []uuid.UUID{uuid.MustParse(lineA)}},
	}, large, small, tiered, perItem)
	// The net fell under P's 10000 and the quantity under R's first tier, so neither suppression
	// holds anything: the 1% line rule takes line A (2000 × 1% = 20) and Q the total
	// ((2000 − 20) × 2% = 39.60).
	assertApplied(t, got, map[string]string{"a1000000": "39.60", "a3000000/0a": "20.00"})
}

func TestEvaluateDiscounts_OnlyRulesInForce(t *testing.T) {
	t.Parallel()
	lines := []engineLine{testLine(lineA, cement, "10", "1000.00", "10000.00", nil)}
	base := promo{id: promoP, condition: domain.PromotionConditionPerItem,
		action: domain.PromotionActionPercentage, value: "10", targets: []uuid.UUID{cement}}
	past, future := engineAt.Add(-time.Hour), engineAt.Add(time.Hour)
	otherBranch := uuid.New()
	cases := []struct {
		name   string
		mutate func(*domain.Promotion)
		earns  bool
	}{
		{"permanent", func(*domain.Promotion) {}, true},
		{"inactive", func(p *domain.Promotion) { p.IsActive = false }, false},
		{"not started", func(p *domain.Promotion) { p.ValidFrom = &future }, false},
		{"ended", func(p *domain.Promotion) { p.ValidTo = &past }, false},
		{"within its window", func(p *domain.Promotion) { p.ValidFrom, p.ValidTo = &past, &future }, true},
		{"another branch's", func(p *domain.Promotion) { p.BranchID = &otherBranch }, false},
		{"this branch's", func(p *domain.Promotion) { b := engineBranch; p.BranchID = &b }, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := base.build()
			tc.mutate(&p)
			got := evaluateDiscounts(engineInput{Lines: lines, Promotions: []domain.Promotion{p},
				BranchID: engineBranch, At: engineAt})
			if earned := len(got) == 1; earned != tc.earns {
				t.Fatalf("earned = %v, want %v", earned, tc.earns)
			}
		})
	}
}

func TestEvaluateDiscounts_UnpricedLinesEarnNothing(t *testing.T) {
	t.Parallel()
	unpriced := engineLine{ID: uuid.MustParse(lineA), ProductID: &cement, Quantity: dec("10")}
	noProduct := testLine(lineB, cement, "10", "1000.00", "10000.00", nil)
	noProduct.ProductID = nil
	got := evaluate([]engineLine{unpriced, noProduct}, promo{id: promoP,
		condition: domain.PromotionConditionPerItem, action: domain.PromotionActionPercentage,
		value: "10", targets: []uuid.UUID{cement}})
	assertApplied(t, got, map[string]string{})
}

func TestEvaluateDiscounts_TheFloorCountsTheSellersDiscounts(t *testing.T) {
	t.Parallel()
	floor := "950.00"
	lines := []engineLine{
		testLine(lineA, cement, "10", "1000.00", "10000.00", &floor),
		testLine(lineB, sand, "4", "1000.00", "4000.00", nil),
	}
	sellerOnA := []engineHeldDiscount{{Amount: dec("400.00"), ItemIDs: []uuid.UUID{uuid.MustParse(lineA)}}}
	run := func(p promo, held []engineHeldDiscount) []domain.DiscountApplication {
		return evaluateDiscounts(engineInput{Lines: lines, Promotions: []domain.Promotion{p.build()},
			BranchID: engineBranch, At: engineAt, Held: held})
	}

	// Line A: 10000 down to its floor 9500 leaves 500; the seller already took 400, so 100 is left
	// for the 20% rule (2000), and the line ends exactly at 9500.
	line := promo{id: promoP, condition: domain.PromotionConditionPerItem,
		action: domain.PromotionActionPercentage, value: "20", targets: []uuid.UUID{cement}}
	assertApplied(t, run(line, sellerOnA), map[string]string{"a0000000/0a": "100.00"})

	// The set over A and B: headroom 500 (A) + 4000 (B, no floor) − 400 = 4100, under the 20% (2800)
	// it asks; with a seller discount of 3000 on B as well, 4500 − 3400 = 1100 is all it gets.
	set := promo{id: promoQ, condition: domain.PromotionConditionItemSet,
		action: domain.PromotionActionPercentage, value: "20", targets: []uuid.UUID{cement, sand}}
	assertApplied(t, run(set, sellerOnA), map[string]string{"a1000000": "2800.00"})
	both := append(slices.Clone(sellerOnA),
		engineHeldDiscount{Amount: dec("3000.00"), ItemIDs: []uuid.UUID{uuid.MustParse(lineB)}})
	assertApplied(t, run(set, both), map[string]string{"a1000000": "1100.00"})

	// A seller discount at or past the floor leaves the line nothing.
	past := []engineHeldDiscount{{Amount: dec("600.00"), ItemIDs: []uuid.UUID{uuid.MustParse(lineA)}}}
	assertApplied(t, run(line, past), map[string]string{})

	// A quote-wide seller discount leaves the net alone but uses the headroom: 4500 − 4000 = 500
	// left for a 50% total rule that would take 7000 off the 14000 net.
	total := promo{id: promoR, condition: domain.PromotionConditionOnTotal,
		action: domain.PromotionActionPercentage, value: "50"}
	assertApplied(t, run(total, []engineHeldDiscount{{Amount: dec("4000.00")}}),
		map[string]string{"a2000000": "500.00"})
}

func TestEvaluateDiscounts_FractionalQuantitiesFindTheirTier(t *testing.T) {
	t.Parallel()
	tiers := []domain.PromotionTier{
		{FromQuantity: dec("10"), ToQuantity: decPtr("49"), Value: dec("10")},
		{FromQuantity: dec("50"), ToQuantity: decPtr("99"), Value: dec("15")},
	}
	cases := []struct {
		quantity, subtotal, want string
	}{
		{"10.5", "1050.00", "105.00"}, // first tier: 1050 × 10%
		{"49.5", "4950.00", "495.00"}, // between 49 and 50: stays in the lower tier
		{"99.5", "9950.00", ""},       // past the last tier's end
		{"9.99", "999.00", ""},        // below the first tier
		{"99", "9900.00", "1485.00"},  // last tier's end is inclusive: 9900 × 15%
	}
	for _, tc := range cases {
		t.Run(tc.quantity, func(t *testing.T) {
			t.Parallel()
			lines := []engineLine{testLine(lineA, cement, tc.quantity, "100.00", tc.subtotal, nil)}
			got := evaluate(lines, promo{id: promoP, condition: domain.PromotionConditionQuantityTier,
				action: domain.PromotionActionPercentage, value: "0", tiers: tiers,
				targets: []uuid.UUID{cement}})
			want := map[string]string{}
			if tc.want != "" {
				want["a0000000/0a"] = tc.want
			}
			assertApplied(t, got, want)
		})
	}
}
