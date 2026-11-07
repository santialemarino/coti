package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

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
