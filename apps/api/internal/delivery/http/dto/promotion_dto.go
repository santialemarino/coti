package dto

import (
	"time"

	"github.com/google/uuid"
)

// PromotionRequest is the body for POST /v1/promotions and PUT /v1/promotions/{promotionId}: the
// whole rule. Money and quantities are decimal strings.
type PromotionRequest struct {
	Name          string                      `json:"name" binding:"required,min=1,max=128"`
	Description   *string                     `json:"description" binding:"omitempty,max=512"`
	BranchID      *uuid.UUID                  `json:"branch_id"`
	ConditionType string                      `json:"condition_type" binding:"required,oneof=PER_ITEM QUANTITY_TIERED ITEM_SET ON_TOTAL"`
	ActionType    string                      `json:"action_type" binding:"required,oneof=PERCENTAGE FIXED_AMOUNT SPECIAL_PRICE"`
	ActionValue   *string                     `json:"action_value" binding:"omitempty,numeric"` // ignored for QUANTITY_TIERED.
	MinTotal      *string                     `json:"min_total" binding:"omitempty,numeric"`
	ValidFrom     *time.Time                  `json:"valid_from"`
	ValidTo       *time.Time                  `json:"valid_to"`
	IsActive      *bool                       `json:"is_active"` // defaults to true.
	IsExclusive   bool                        `json:"is_exclusive"`
	Priority      int                         `json:"priority"`
	Conditions    []PromotionConditionRequest `json:"conditions" binding:"omitempty,dive"`
	Tiers         []PromotionTierRequest      `json:"tiers" binding:"omitempty,dive"`
}

// PromotionConditionRequest names one target of a rule: exactly one of the three ids.
type PromotionConditionRequest struct {
	ProductID   *uuid.UUID `json:"product_id"`
	FamilyID    *uuid.UUID `json:"family_id"`
	SubgroupID  *uuid.UUID `json:"subgroup_id"`
	MinQuantity *string    `json:"min_quantity" binding:"omitempty,numeric"`
}

// PromotionTierRequest is one quantity band of a QUANTITY_TIERED rule; both bounds are inclusive.
type PromotionTierRequest struct {
	FromQuantity string  `json:"from_quantity" binding:"required,numeric"`
	ToQuantity   *string `json:"to_quantity" binding:"omitempty,numeric"`
	Value        string  `json:"value" binding:"required,numeric"`
}

// PromotionResponse is returned by every promotion route.
type PromotionResponse struct {
	ID            uuid.UUID                    `json:"id"`
	BranchID      *uuid.UUID                   `json:"branch_id"`
	Name          string                       `json:"name"`
	Description   *string                      `json:"description"`
	ConditionType string                       `json:"condition_type"`
	ActionType    string                       `json:"action_type"`
	ActionValue   string                       `json:"action_value"`
	MinTotal      *string                      `json:"min_total"`
	ValidFrom     *time.Time                   `json:"valid_from"`
	ValidTo       *time.Time                   `json:"valid_to"`
	IsActive      bool                         `json:"is_active"`
	IsExclusive   bool                         `json:"is_exclusive"`
	Priority      int                          `json:"priority"`
	Conditions    []PromotionConditionResponse `json:"conditions"`
	Tiers         []PromotionTierResponse      `json:"tiers"`
	CreatedAt     time.Time                    `json:"created_at"`
	UpdatedAt     time.Time                    `json:"updated_at"`
}

// PromotionConditionResponse is one target of a rule.
type PromotionConditionResponse struct {
	ProductID   *uuid.UUID `json:"product_id"`
	FamilyID    *uuid.UUID `json:"family_id"`
	SubgroupID  *uuid.UUID `json:"subgroup_id"`
	MinQuantity *string    `json:"min_quantity"`
}

// PromotionTierResponse is one quantity band of a tiered rule.
type PromotionTierResponse struct {
	FromQuantity string  `json:"from_quantity"`
	ToQuantity   *string `json:"to_quantity"`
	Value        string  `json:"value"`
}

// PromotionListResponse is returned by GET /v1/promotions.
type PromotionListResponse struct {
	Items []PromotionResponse `json:"items"`
}
