package services

import (
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func adaptLine(product uuid.UUID, quantity, price string) domain.QuoteItem {
	q, p := decimal.RequireFromString(quantity), decimal.RequireFromString(price)
	return domain.QuoteItem{ID: uuid.New(), ProductID: &product, RequestedDescription: "cemento",
		Quantity: q, UnitPriceSnapshot: decimal.NewNullDecimal(p),
		Subtotal: decimal.NewNullDecimal(q.Mul(p))}
}

func perItemFrom(product uuid.UUID, minimum, percent string) domain.Promotion {
	floor := decimal.RequireFromString(minimum)
	return domain.Promotion{ID: uuid.New(), Name: "Cemento " + percent + "%", IsActive: true,
		ConditionType: domain.PromotionConditionPerItem, ActionType: domain.PromotionActionPercentage,
		ActionValue: decimal.RequireFromString(percent),
		Conditions:  []domain.PromotionCondition{{ProductID: &product, MinQuantity: &floor}}}
}

func tieredOn(product uuid.UUID, tiers ...[2]string) domain.Promotion {
	promotion := domain.Promotion{ID: uuid.New(), Name: "Escalonada", IsActive: true,
		ConditionType: domain.PromotionConditionQuantityTier,
		ActionType:    domain.PromotionActionPercentage,
		Conditions:    []domain.PromotionCondition{{ProductID: &product}}}
	for _, tier := range tiers {
		promotion.Tiers = append(promotion.Tiers, domain.PromotionTier{
			FromQuantity: decimal.RequireFromString(tier[0]), Value: decimal.RequireFromString(tier[1])})
	}
	return promotion
}

func adaptState(promotions ...domain.Promotion) sweepState {
	return sweepState{links: map[uuid.UUID][]uuid.UUID{}, promotions: promotions,
		groups: map[uuid.UUID]domain.ProductGroup{}, at: time.Now()}
}

func TestNextThreshold(t *testing.T) {
	cement, family := uuid.New(), uuid.New()
	byFamily := perItemFrom(cement, "20", "10")
	byFamily.Conditions[0].ProductID, byFamily.Conditions[0].FamilyID = nil, &family
	set := perItemFrom(cement, "20", "10")
	set.ConditionType = domain.PromotionConditionItemSet
	tiered := tieredOn(cement, [2]string{"10", "5"}, [2]string{"20", "10"})
	unpriced := adaptLine(cement, "19", "100")
	unpriced.UnitPriceSnapshot = decimal.NullDecimal{}
	cases := []struct {
		name      string
		promotion domain.Promotion
		line      domain.QuoteItem
		groups    map[uuid.UUID]domain.ProductGroup
		want      string // empty = no threshold.
	}{
		{"per item below its minimum", perItemFrom(cement, "20", "10"), adaptLine(cement, "19", "100"), nil, "20"},
		{"per item already reached", perItemFrom(cement, "20", "10"), adaptLine(cement, "20", "100"), nil, ""},
		{"per item through the line's family", byFamily, adaptLine(cement, "12", "100"),
			map[uuid.UUID]domain.ProductGroup{cement: {FamilyID: &family}}, "20"},
		{"another product", perItemFrom(uuid.New(), "20", "10"), adaptLine(cement, "19", "100"), nil, ""},
		{"tiered below the first tier", tiered, adaptLine(cement, "4", "100"), nil, "10"},
		{"tiered inside a tier takes the next", tiered, adaptLine(cement, "15", "100"), nil, "20"},
		{"tiered past the last tier", tiered, adaptLine(cement, "25", "100"), nil, ""},
		{"a next tier that earns the same figure", tieredOn(cement, [2]string{"10", "5"}, [2]string{"20", "5"}),
			adaptLine(cement, "15", "100"), nil, ""},
		{"an item set is never adapted to", set, adaptLine(cement, "19", "100"), nil, ""},
		{"an unpriced line", perItemFrom(cement, "20", "10"), unpriced, nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := nextThreshold(tc.promotion, tc.line, tc.groups)
			if tc.want == "" {
				if ok {
					t.Fatalf("nextThreshold() = %s, want none", got)
				}
				return
			}
			if !ok || !got.Equal(decimal.RequireFromString(tc.want)) {
				t.Fatalf("nextThreshold() = %s, %v, want %s", got, ok, tc.want)
			}
		})
	}
}

/*
 * 19 bags at 100 stand at 1900; at 20 the 10% earns: 2000 − 200 = 1800, lower, so the raise is
 * kept. 15 bags would end at 1800 against 1500, dearer, so that line is left alone.
 */
func TestPlanLowerTotalAdaptation_KeepsOnlyRaisesThatLowerTheTotal(t *testing.T) {
	cement, sand := uuid.New(), uuid.New()
	near, far := adaptLine(cement, "19", "100"), adaptLine(sand, "15", "100")
	state := adaptState(perItemFrom(cement, "20", "10"), perItemFrom(sand, "20", "10"))

	raises, total, err := planLowerTotalAdaptation(state, uuid.New(), []domain.QuoteItem{near, far},
		func(domain.QuoteItem) bool { return true })

	if err != nil {
		t.Fatal(err)
	}
	if len(raises) != 1 || raises[0].item.ID != near.ID || !raises[0].to.Equal(decimal.NewFromInt(20)) {
		t.Fatalf("raises = %+v, want only the cement raised to 20", raises)
	}
	// 1800 for the cement plus the sand's untouched 1500.
	if !total.Equal(decimal.NewFromInt(3300)) {
		t.Errorf("planned total = %s, want 3300", total)
	}
}

// Of two thresholds on one line the one that leaves the total lowest wins.
func TestPlanLowerTotalAdaptation_PicksTheLowestTotalPerLine(t *testing.T) {
	cement := uuid.New()
	line := adaptLine(cement, "19", "100")
	// From 1900: 20 at 8% ends at 1840, 21 at 15% at 1785.
	state := adaptState(perItemFrom(cement, "20", "8"), perItemFrom(cement, "21", "15"))

	raises, total, err := planLowerTotalAdaptation(state, uuid.New(), []domain.QuoteItem{line},
		func(domain.QuoteItem) bool { return true })

	if err != nil || len(raises) != 1 || raises[0].promotion.Name != "Cemento 15%" ||
		!raises[0].to.Equal(decimal.NewFromInt(21)) || !total.Equal(decimal.NewFromInt(1785)) {
		t.Fatalf("raises = %+v, total = %s, err = %v; want 21 at 15%% ending at 1785", raises, total, err)
	}
}

func TestPlanLowerTotalAdaptation_SkipsWhatIsOutOfReach(t *testing.T) {
	cement := uuid.New()
	promotion := perItemFrom(cement, "20", "10")
	cases := []struct {
		name    string
		state   sweepState
		inScope func(domain.QuoteItem) bool
	}{
		{"a line the customer did not name", adaptState(promotion),
			func(domain.QuoteItem) bool { return false }},
		{"a promotion the seller suppressed", sweepState{
			discounts: []domain.QuoteDiscount{{ID: uuid.New(), PromotionID: &promotion.ID,
				Origin: domain.DiscountOriginAIAdaptation, SuppressedBySeller: true,
				Scope: domain.DiscountScopeItem}},
			links: map[uuid.UUID][]uuid.UUID{}, promotions: []domain.Promotion{promotion}, at: time.Now()},
			func(domain.QuoteItem) bool { return true }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raises, _, err := planLowerTotalAdaptation(tc.state, uuid.New(),
				[]domain.QuoteItem{adaptLine(cement, "19", "100")}, tc.inScope)
			if err != nil || len(raises) != 0 {
				t.Fatalf("raises = %+v, err = %v, want none", raises, err)
			}
		})
	}
}

// Asked for by name, the promotion is reached even though the total goes up: 15 → 20 ends at 1800.
func TestPlanPromotionReach_RaisesToTheThresholdWhateverTheTotal(t *testing.T) {
	cement, sand := uuid.New(), uuid.New()
	promotion := perItemFrom(cement, "20", "10")
	line, other := adaptLine(cement, "15", "100"), adaptLine(sand, "15", "100")

	raises, err := planPromotionReach(adaptState(promotion), uuid.New(),
		[]domain.QuoteItem{line, other}, promotion, func(domain.QuoteItem) bool { return true })

	if err != nil || len(raises) != 1 || raises[0].item.ID != line.ID ||
		!raises[0].to.Equal(decimal.NewFromInt(20)) {
		t.Fatalf("raises = %+v, err = %v, want the cement at 20", raises, err)
	}
	if raises, _ := planPromotionReach(adaptState(promotion), uuid.New(),
		[]domain.QuoteItem{line}, promotion, func(domain.QuoteItem) bool { return false }); len(raises) != 0 {
		t.Errorf("raises = %+v, want none on a line the customer set the quantity of", raises)
	}
}

// A raise the engine would not apply the promotion on is not kept: here an exclusive rule of higher
// priority takes the line at 20.
func TestPlanPromotionReach_KeepsOnlyRaisesTheEngineHonours(t *testing.T) {
	cement := uuid.New()
	promotion := perItemFrom(cement, "20", "10")
	blocker := perItemFrom(cement, "20", "15")
	blocker.Priority, blocker.IsExclusive = 9, true

	raises, err := planPromotionReach(adaptState(promotion, blocker), uuid.New(),
		[]domain.QuoteItem{adaptLine(cement, "15", "100")}, promotion,
		func(domain.QuoteItem) bool { return true })

	if err != nil || len(raises) != 0 {
		t.Fatalf("raises = %+v, err = %v, want none", raises, err)
	}
}

func TestStrategyOrder(t *testing.T) {
	lower, improve := domain.ChangeRequestIntentLowerTotalPrice, domain.ChangeRequestIntentImproveQuality
	adapt, cheaper := domain.HandlerStrategyAdaptToReachDiscount, domain.HandlerStrategyFindCheaperAlternatives
	cases := []struct {
		name     string
		proposal domain.ChangeRequestProposal
		account  domain.HandlerStrategy
		want     []domain.HandlerStrategy
	}{
		{"default", domain.ChangeRequestProposal{Level: domain.ChangeRequestLevelComplex, Intent: &lower},
			"", []domain.HandlerStrategy{cheaper, adapt}},
		{"the account's choice", domain.ChangeRequestProposal{Level: domain.ChangeRequestLevelComplex,
			Intent: &lower}, adapt, []domain.HandlerStrategy{adapt, cheaper}},
		{"the customer's wording over the account", domain.ChangeRequestProposal{
			Level: domain.ChangeRequestLevelComplex, Intent: &lower, LowerPriceStrategy: &cheaper},
			adapt, []domain.HandlerStrategy{cheaper, adapt}},
		{"better quality", domain.ChangeRequestProposal{Level: domain.ChangeRequestLevelComplex,
			Intent: &improve}, adapt, []domain.HandlerStrategy{domain.HandlerStrategyImproveQuality}},
		{"atomic", domain.ChangeRequestProposal{Level: domain.ChangeRequestLevelAtomic}, adapt, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := strategyOrder(tc.proposal, tc.account); !slices.Equal(got, tc.want) {
				t.Errorf("strategyOrder() = %v, want %v", got, tc.want)
			}
		})
	}
}
