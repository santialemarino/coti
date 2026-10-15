package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Promotion is an account's reusable discount rule. The deterministic engine evaluates it against a
// quote version; its application to one version is a QuoteDiscount.
type Promotion struct {
	ID            uuid.UUID
	AccountID     uuid.UUID
	BranchID      *uuid.UUID // nil = every branch of the account.
	Name          string
	Description   *string
	ConditionType PromotionConditionType
	ActionType    PromotionActionType
	// ActionValue is the rule's figure; a QUANTITY_TIERED rule carries its figures on the tiers.
	ActionValue decimal.Decimal
	// MinTotal is the net an ON_TOTAL rule needs before it applies; nil = no threshold.
	MinTotal    *decimal.Decimal
	ValidFrom   *time.Time
	ValidTo     *time.Time
	IsActive    bool
	IsExclusive bool
	Priority    int
	Conditions  []PromotionCondition
	Tiers       []PromotionTier
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// PromotionCondition names one catalog target a rule covers: exactly one of a product, a family
// or a subgroup, with an optional minimum quantity on the line.
type PromotionCondition struct {
	ProductID   *uuid.UUID
	FamilyID    *uuid.UUID
	SubgroupID  *uuid.UUID
	MinQuantity *decimal.Decimal
}

// PromotionTier is one quantity band of a QUANTITY_TIERED rule. ToQuantity nil = open-ended; both
// bounds are inclusive.
type PromotionTier struct {
	FromQuantity decimal.Decimal
	ToQuantity   *decimal.Decimal
	Value        decimal.Decimal
}

// PromotionInput is the whole editable rule, used to create a promotion and to replace one.
type PromotionInput struct {
	BranchID      *uuid.UUID
	Name          string
	Description   *string
	ConditionType PromotionConditionType
	ActionType    PromotionActionType
	ActionValue   decimal.Decimal
	MinTotal      *decimal.Decimal
	ValidFrom     *time.Time
	ValidTo       *time.Time
	IsActive      bool
	IsExclusive   bool
	Priority      int
	Conditions    []PromotionCondition
	Tiers         []PromotionTier
}

// ProductGroup is where a catalog product sits in the taxonomy, which a family or subgroup
// condition matches against.
type ProductGroup struct {
	FamilyID   *uuid.UUID
	SubgroupID *uuid.UUID
}

// ScopeOf derives the scope an application of a rule of this condition type covers.
func ScopeOf(condition PromotionConditionType) DiscountScope {
	switch condition {
	case PromotionConditionOnTotal:
		return DiscountScopeTotal
	case PromotionConditionItemSet:
		return DiscountScopeItemSet
	default:
		return DiscountScopeItem
	}
}
