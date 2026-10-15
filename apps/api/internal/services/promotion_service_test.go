package services

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func validPromotion(condition domain.PromotionConditionType) domain.PromotionInput {
	product, other := uuid.New(), uuid.New()
	in := domain.PromotionInput{Name: " Cemento ", ConditionType: condition,
		ActionType: domain.PromotionActionPercentage, ActionValue: dec("10"), IsActive: true}
	switch condition {
	case domain.PromotionConditionPerItem:
		in.Conditions = []domain.PromotionCondition{{ProductID: &product}}
	case domain.PromotionConditionQuantityTier:
		in.Conditions = []domain.PromotionCondition{{ProductID: &product}}
		in.Tiers = []domain.PromotionTier{{FromQuantity: dec("10"), ToQuantity: decPtr("49"),
			Value: dec("10")}, {FromQuantity: dec("50"), Value: dec("15")}}
	case domain.PromotionConditionItemSet:
		in.Conditions = []domain.PromotionCondition{{ProductID: &product}, {ProductID: &other}}
	}
	return in
}

func TestNormalizePromotion_AcceptsEachShape(t *testing.T) {
	t.Parallel()
	for _, condition := range []domain.PromotionConditionType{domain.PromotionConditionPerItem,
		domain.PromotionConditionQuantityTier, domain.PromotionConditionItemSet,
		domain.PromotionConditionOnTotal} {
		got, err := normalizePromotion(validPromotion(condition))
		if err != nil {
			t.Fatalf("%s: %v", condition, err)
		}
		if got.Name != "Cemento" {
			t.Errorf("%s: name = %q, want it trimmed", condition, got.Name)
		}
		if condition == domain.PromotionConditionQuantityTier && !got.ActionValue.IsZero() {
			t.Errorf("tiered action_value = %s, want 0: the figures live on the tiers", got.ActionValue)
		}
	}
}

func TestNormalizePromotion_Refuses(t *testing.T) {
	t.Parallel()
	now := time.Now()
	earlier := now.Add(-time.Hour)
	family := uuid.New()
	cases := []struct {
		name   string
		base   domain.PromotionConditionType
		mutate func(*domain.PromotionInput)
	}{
		{"no name", domain.PromotionConditionOnTotal, func(in *domain.PromotionInput) { in.Name = "  " }},
		{"window ends before it starts", domain.PromotionConditionOnTotal, func(in *domain.PromotionInput) {
			in.ValidFrom, in.ValidTo = &now, &earlier
		}},
		{"percentage over 100", domain.PromotionConditionPerItem, func(in *domain.PromotionInput) {
			in.ActionValue = dec("100.01")
		}},
		{"zero figure", domain.PromotionConditionPerItem, func(in *domain.PromotionInput) {
			in.ActionValue = decimal.Zero
		}},
		{"three decimals", domain.PromotionConditionPerItem, func(in *domain.PromotionInput) {
			in.ActionValue = dec("1.005")
		}},
		{"special price on a total", domain.PromotionConditionOnTotal, func(in *domain.PromotionInput) {
			in.ActionType = domain.PromotionActionSpecialPrice
		}},
		{"special price on a set", domain.PromotionConditionItemSet, func(in *domain.PromotionInput) {
			in.ActionType = domain.PromotionActionSpecialPrice
		}},
		{"line rule with no target", domain.PromotionConditionPerItem, func(in *domain.PromotionInput) {
			in.Conditions = nil
		}},
		{"set of one", domain.PromotionConditionItemSet, func(in *domain.PromotionInput) {
			in.Conditions = in.Conditions[:1]
		}},
		{"total with a target", domain.PromotionConditionOnTotal, func(in *domain.PromotionInput) {
			in.Conditions = []domain.PromotionCondition{{FamilyID: &family}}
		}},
		{"condition with two targets", domain.PromotionConditionPerItem, func(in *domain.PromotionInput) {
			in.Conditions[0].FamilyID = &family
		}},
		{"repeated target", domain.PromotionConditionItemSet, func(in *domain.PromotionInput) {
			in.Conditions[1] = in.Conditions[0]
		}},
		{"threshold on a line rule", domain.PromotionConditionPerItem, func(in *domain.PromotionInput) {
			in.MinTotal = decPtr("1000")
		}},
		{"tiers on a line rule", domain.PromotionConditionPerItem, func(in *domain.PromotionInput) {
			in.Tiers = []domain.PromotionTier{{FromQuantity: dec("1"), Value: dec("5")}}
		}},
		{"tiered with no tiers", domain.PromotionConditionQuantityTier, func(in *domain.PromotionInput) {
			in.Tiers = nil
		}},
		{"overlapping tiers", domain.PromotionConditionQuantityTier, func(in *domain.PromotionInput) {
			in.Tiers[1].FromQuantity = dec("49")
		}},
		{"tier upper bound below its start", domain.PromotionConditionQuantityTier, func(in *domain.PromotionInput) {
			in.Tiers[0].ToQuantity = decPtr("5")
		}},
		{"unknown condition", domain.PromotionConditionOnTotal, func(in *domain.PromotionInput) {
			in.ConditionType = "COMBO"
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			in := validPromotion(tc.base)
			tc.mutate(&in)
			if _, err := normalizePromotion(in); !errors.Is(err, domain.ErrInvalidInput) {
				t.Fatalf("normalizePromotion() = %v, want ErrInvalidInput", err)
			}
		})
	}
}
