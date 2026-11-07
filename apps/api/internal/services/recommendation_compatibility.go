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

type recommendationProductReader interface {
	GetByIDs(ctx context.Context, q repository.Querier, accountID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]domain.Product, error)
}

func compatibleRecommendation(base, alternative domain.Product) bool {
	return base.FamilyID != nil && alternative.FamilyID != nil && *base.FamilyID == *alternative.FamilyID &&
		base.SubgroupID != nil && alternative.SubgroupID != nil && *base.SubgroupID == *alternative.SubgroupID &&
		base.Unit != nil && alternative.Unit != nil && strings.TrimSpace(*base.Unit) != "" &&
		strings.EqualFold(strings.TrimSpace(*base.Unit), strings.TrimSpace(*alternative.Unit))
}

func requireCompatibleRecommendation(base, alternative domain.Product) error {
	if !compatibleRecommendation(base, alternative) {
		return fmt.Errorf("%w: alternative products must share a family, subgroup and unit", domain.ErrInvalidInput)
	}
	return nil
}

func validRecommendationPrice(kind domain.ProductAlternativeType, base, alternative decimal.NullDecimal) bool {
	if !base.Valid || !alternative.Valid {
		return false
	}
	switch kind {
	case domain.ProductAlternativeEconomy:
		return alternative.Decimal.LessThan(base.Decimal)
	case domain.ProductAlternativePremium:
		return alternative.Decimal.GreaterThan(base.Decimal)
	default:
		return false
	}
}

func requireRecommendationPrice(kind domain.ProductAlternativeType, base, alternative decimal.NullDecimal) error {
	if !base.Valid {
		return domain.WithCode(domain.CodeRecommendationBasePriceMissing, domain.ErrInvalidInput)
	}
	if validRecommendationPrice(kind, base, alternative) {
		return nil
	}
	code := domain.CodeRecommendationEconomyPriceInvalid
	if kind == domain.ProductAlternativePremium {
		code = domain.CodeRecommendationPremiumPriceInvalid
	}
	return domain.WithCode(code, domain.ErrInvalidInput)
}
