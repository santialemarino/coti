package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// promotionRepository is the promotion persistence the administration needs.
type promotionRepository interface {
	List(ctx context.Context, q repository.Querier, accountID uuid.UUID) ([]domain.Promotion, error)
	GetByID(ctx context.Context, q repository.Querier, accountID, id uuid.UUID) (*domain.Promotion, error)
	CountTargets(ctx context.Context, q repository.Querier, accountID uuid.UUID, branchIDs, productIDs, familyIDs, subgroupIDs []uuid.UUID) (int, int, int, int, error)
	Create(ctx context.Context, q repository.Querier, accountID uuid.UUID, in domain.PromotionInput) (*domain.Promotion, error)
	Replace(ctx context.Context, q repository.Querier, accountID, id uuid.UUID, in domain.PromotionInput) (*domain.Promotion, error)
}

// PromotionService administers an account's promotion rules. Changing one reaches a quote at its
// next sweep; a frozen version keeps the discounts it was sent with.
type PromotionService struct {
	db         tenantTxRunner
	promotions promotionRepository
}

// NewPromotionService builds a PromotionService.
func NewPromotionService(db tenantTxRunner, promotions promotionRepository) *PromotionService {
	return &PromotionService{db: db, promotions: promotions}
}

// List returns every promotion of the caller's account.
func (s *PromotionService) List(ctx context.Context, tenant domain.Tenant) ([]domain.Promotion, error) {
	var promotions []domain.Promotion
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var err error
		promotions, err = s.promotions.List(ctx, q, tenant.AccountID)
		return err
	})
	return promotions, err
}

// Get returns one promotion of the caller's account.
func (s *PromotionService) Get(ctx context.Context, tenant domain.Tenant, id uuid.UUID) (*domain.Promotion, error) {
	var promotion *domain.Promotion
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var err error
		promotion, err = s.promotions.GetByID(ctx, q, tenant.AccountID, id)
		return err
	})
	return promotion, err
}

// Create validates a rule and stores it on the caller's account.
func (s *PromotionService) Create(
	ctx context.Context, tenant domain.Tenant, in domain.PromotionInput,
) (*domain.Promotion, error) {
	normalized, err := normalizePromotion(in)
	if err != nil {
		return nil, err
	}
	var promotion *domain.Promotion
	err = s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		if err := s.requireTargets(ctx, q, tenant.AccountID, normalized); err != nil {
			return err
		}
		promotion, err = s.promotions.Create(ctx, q, tenant.AccountID, normalized)
		return err
	})
	return promotion, err
}

// Replace validates a rule and rewrites a promotion of the caller's account with it. Deactivating
// and reactivating a promotion is this: is_active or the validity window changes.
func (s *PromotionService) Replace(
	ctx context.Context, tenant domain.Tenant, id uuid.UUID, in domain.PromotionInput,
) (*domain.Promotion, error) {
	normalized, err := normalizePromotion(in)
	if err != nil {
		return nil, err
	}
	var promotion *domain.Promotion
	err = s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		if err := s.requireTargets(ctx, q, tenant.AccountID, normalized); err != nil {
			return err
		}
		promotion, err = s.promotions.Replace(ctx, q, tenant.AccountID, id, normalized)
		return err
	})
	return promotion, err
}

// requireTargets refuses a rule naming a branch or product outside the account, or a taxonomy node
// that does not exist: a foreign key alone would accept another account's row.
func (s *PromotionService) requireTargets(
	ctx context.Context, q repository.Querier, accountID uuid.UUID, in domain.PromotionInput,
) error {
	var branchIDs, productIDs, familyIDs, subgroupIDs []uuid.UUID
	if in.BranchID != nil {
		branchIDs = append(branchIDs, *in.BranchID)
	}
	for _, c := range in.Conditions {
		switch {
		case c.ProductID != nil:
			productIDs = append(productIDs, *c.ProductID)
		case c.FamilyID != nil:
			familyIDs = append(familyIDs, *c.FamilyID)
		case c.SubgroupID != nil:
			subgroupIDs = append(subgroupIDs, *c.SubgroupID)
		}
	}
	branches, products, families, subgroups, err := s.promotions.CountTargets(ctx, q, accountID,
		branchIDs, productIDs, familyIDs, subgroupIDs)
	if err != nil {
		return err
	}
	if branches != len(branchIDs) || products != len(productIDs) || families != len(familyIDs) ||
		subgroups != len(subgroupIDs) {
		return fmt.Errorf("%w: a branch, product, family or subgroup does not exist in this account",
			domain.ErrInvalidInput)
	}
	return nil
}

// normalizePromotion checks a rule is one the engine can evaluate, and trims its text. Each
// condition type takes its own shape: line rules name targets, a tiered rule carries its figures
// on the tiers, a total rule names nothing and may set a threshold.
func normalizePromotion(in domain.PromotionInput) (domain.PromotionInput, error) {
	invalid := func(format string, args ...any) (domain.PromotionInput, error) {
		return domain.PromotionInput{}, fmt.Errorf("%w: "+format,
			append([]any{domain.ErrInvalidInput}, args...)...)
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return invalid("name is required")
	}
	if err := requireMaxRunes(in.Name, "name", 128); err != nil {
		return domain.PromotionInput{}, err
	}
	if in.Description != nil {
		trimmed := strings.TrimSpace(*in.Description)
		in.Description = &trimmed
		if trimmed == "" {
			in.Description = nil
		} else if err := requireMaxRunes(trimmed, "description", 512); err != nil {
			return domain.PromotionInput{}, err
		}
	}
	if in.ValidFrom != nil && in.ValidTo != nil && !in.ValidTo.After(*in.ValidFrom) {
		return invalid("valid_to must be after valid_from")
	}

	switch in.ActionType {
	case domain.PromotionActionPercentage, domain.PromotionActionFixedAmount:
	case domain.PromotionActionSpecialPrice:
		if in.ConditionType != domain.PromotionConditionPerItem &&
			in.ConditionType != domain.PromotionConditionQuantityTier {
			return invalid("SPECIAL_PRICE applies only to PER_ITEM and QUANTITY_TIERED")
		}
	default:
		return invalid("action_type must be PERCENTAGE, FIXED_AMOUNT or SPECIAL_PRICE")
	}

	switch in.ConditionType {
	case domain.PromotionConditionPerItem, domain.PromotionConditionQuantityTier:
		if len(in.Conditions) == 0 {
			return invalid("%s needs at least one condition", in.ConditionType)
		}
	case domain.PromotionConditionItemSet:
		if len(in.Conditions) < 2 {
			return invalid("ITEM_SET needs at least two conditions")
		}
	case domain.PromotionConditionOnTotal:
		if len(in.Conditions) > 0 {
			return invalid("ON_TOTAL takes no conditions")
		}
	default:
		return invalid("condition_type must be PER_ITEM, QUANTITY_TIERED, ITEM_SET or ON_TOTAL")
	}

	if in.MinTotal != nil {
		if in.ConditionType != domain.PromotionConditionOnTotal {
			return invalid("min_total applies only to ON_TOTAL")
		}
		if err := requirePositiveMoney(*in.MinTotal, "min_total"); err != nil {
			return domain.PromotionInput{}, err
		}
	}

	if in.ConditionType == domain.PromotionConditionQuantityTier {
		if err := validateTiers(in.Tiers, in.ActionType); err != nil {
			return domain.PromotionInput{}, err
		}
		in.ActionValue = decimal.Zero
	} else {
		if len(in.Tiers) > 0 {
			return invalid("only QUANTITY_TIERED takes tiers")
		}
		if err := validateFigure(in.ActionValue, in.ActionType, "action_value"); err != nil {
			return domain.PromotionInput{}, err
		}
	}

	seen := make(map[uuid.UUID]bool, len(in.Conditions))
	for i, c := range in.Conditions {
		targets := 0
		for _, id := range []*uuid.UUID{c.ProductID, c.FamilyID, c.SubgroupID} {
			if id != nil {
				targets++
				if seen[*id] {
					return invalid("conditions[%d] repeats a target", i)
				}
				seen[*id] = true
			}
		}
		if targets != 1 {
			return invalid("conditions[%d] names exactly one of product_id, family_id or subgroup_id", i)
		}
		if c.MinQuantity != nil {
			if err := requirePositiveMoney(*c.MinQuantity, fmt.Sprintf("conditions[%d].min_quantity", i)); err != nil {
				return domain.PromotionInput{}, err
			}
		}
	}
	return in, nil
}

// validateTiers checks a tiered rule's bands: non-overlapping, ordered bounds, valid figures.
func validateTiers(tiers []domain.PromotionTier, action domain.PromotionActionType) error {
	if len(tiers) == 0 {
		return fmt.Errorf("%w: QUANTITY_TIERED needs at least one tier", domain.ErrInvalidInput)
	}
	for i, tier := range tiers {
		field := fmt.Sprintf("tiers[%d]", i)
		if err := requirePositiveMoney(tier.FromQuantity, field+".from_quantity"); err != nil {
			return err
		}
		if tier.ToQuantity != nil {
			if err := validateAmount(*tier.ToQuantity, field+".to_quantity"); err != nil {
				return err
			}
			if tier.ToQuantity.LessThan(tier.FromQuantity) {
				return fmt.Errorf("%w: %s.to_quantity cannot be below from_quantity",
					domain.ErrInvalidInput, field)
			}
		}
		if err := validateFigure(tier.Value, action, field+".value"); err != nil {
			return err
		}
		for j := range i {
			if tiersOverlap(tiers[j], tier) {
				return fmt.Errorf("%w: tiers[%d] and tiers[%d] overlap", domain.ErrInvalidInput, j, i)
			}
		}
	}
	return nil
}

func tiersOverlap(a, b domain.PromotionTier) bool {
	aBelowB := a.ToQuantity != nil && a.ToQuantity.LessThan(b.FromQuantity)
	bBelowA := b.ToQuantity != nil && b.ToQuantity.LessThan(a.FromQuantity)
	return !aBelowB && !bBelowA
}

// validateFigure checks a rule's figure: positive money, and a percentage no larger than 100.
func validateFigure(value decimal.Decimal, action domain.PromotionActionType, field string) error {
	if err := requirePositiveMoney(value, field); err != nil {
		return err
	}
	if action == domain.PromotionActionPercentage && value.GreaterThan(decimal.NewFromInt(100)) {
		return fmt.Errorf("%w: %s cannot exceed 100 for a percentage", domain.ErrInvalidInput, field)
	}
	return nil
}

func requirePositiveMoney(value decimal.Decimal, field string) error {
	if !value.IsPositive() {
		return fmt.Errorf("%w: %s must be greater than zero", domain.ErrInvalidInput, field)
	}
	return validateAmount(value, field)
}
