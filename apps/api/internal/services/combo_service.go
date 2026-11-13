package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

const (
	comboNameMaxLength        = 255
	comboDescriptionMaxLength = 512
	comboMinItems             = 2
)

type comboRepository interface {
	List(ctx context.Context, q repository.Querier, accountID uuid.UUID) ([]domain.Combo, error)
	GetByID(ctx context.Context, q repository.Querier, accountID, id uuid.UUID) (*domain.Combo, error)
	Create(ctx context.Context, q repository.Querier, accountID uuid.UUID, in domain.ComboInput) (uuid.UUID, error)
	Replace(ctx context.Context, q repository.Querier, accountID, id uuid.UUID, in domain.ComboInput) error
}

// ComboService administers the account's catalog combos.
type ComboService struct {
	db     tenantTxRunner
	combos comboRepository
}

// NewComboService builds a ComboService.
func NewComboService(db tenantTxRunner, combos comboRepository) *ComboService {
	return &ComboService{db: db, combos: combos}
}

// List returns every combo of the account, inactive ones included.
func (s *ComboService) List(ctx context.Context, tenant domain.Tenant) ([]domain.Combo, error) {
	var combos []domain.Combo
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var listErr error
		combos, listErr = s.combos.List(ctx, q, tenant.AccountID)
		return listErr
	})
	return combos, err
}

// Get returns one combo of the account.
func (s *ComboService) Get(ctx context.Context, tenant domain.Tenant, id uuid.UUID) (*domain.Combo, error) {
	var combo *domain.Combo
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var getErr error
		combo, getErr = s.combos.GetByID(ctx, q, tenant.AccountID, id)
		return getErr
	})
	return combo, err
}

// Create validates and stores a new combo.
func (s *ComboService) Create(ctx context.Context, tenant domain.Tenant, in domain.ComboInput) (*domain.Combo, error) {
	normalized, err := normalizeCombo(in)
	if err != nil {
		return nil, err
	}
	var combo *domain.Combo
	err = s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		id, createErr := s.combos.Create(ctx, q, tenant.AccountID, normalized)
		if createErr != nil {
			return comboTargetError(createErr)
		}
		var getErr error
		combo, getErr = s.combos.GetByID(ctx, q, tenant.AccountID, id)
		return getErr
	})
	return combo, err
}

// Replace validates and rewrites a combo's whole definition.
func (s *ComboService) Replace(
	ctx context.Context, tenant domain.Tenant, id uuid.UUID, in domain.ComboInput,
) (*domain.Combo, error) {
	normalized, err := normalizeCombo(in)
	if err != nil {
		return nil, err
	}
	var combo *domain.Combo
	err = s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		if replaceErr := s.combos.Replace(ctx, q, tenant.AccountID, id, normalized); replaceErr != nil {
			return comboTargetError(replaceErr)
		}
		var getErr error
		combo, getErr = s.combos.GetByID(ctx, q, tenant.AccountID, id)
		return getErr
	})
	return combo, err
}

// comboTargetError names what a refused write means: a product or branch outside the account.
func comboTargetError(err error) error {
	if err == domain.ErrInvalidInput {
		return fmt.Errorf("%w: a product or branch does not exist in this account", domain.ErrInvalidInput)
	}
	return err
}

func normalizeCombo(in domain.ComboInput) (domain.ComboInput, error) {
	invalid := func(format string, args ...any) (domain.ComboInput, error) {
		return domain.ComboInput{}, fmt.Errorf("%w: "+format, append([]any{domain.ErrInvalidInput}, args...)...)
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return invalid("name is required")
	}
	if err := requireMaxRunes(in.Name, "name", comboNameMaxLength); err != nil {
		return domain.ComboInput{}, err
	}
	if in.Description != nil {
		trimmed := strings.TrimSpace(*in.Description)
		in.Description = &trimmed
		if trimmed == "" {
			in.Description = nil
		} else if err := requireMaxRunes(trimmed, "description", comboDescriptionMaxLength); err != nil {
			return domain.ComboInput{}, err
		}
	}
	if len(in.Items) < comboMinItems {
		return invalid("a combo needs at least %d products", comboMinItems)
	}
	seen := make(map[uuid.UUID]bool, len(in.Items))
	for i, item := range in.Items {
		if seen[item.ProductID] {
			return invalid("items[%d] repeats a product", i)
		}
		seen[item.ProductID] = true
		if !item.Quantity.IsPositive() {
			return invalid("items[%d].quantity must be greater than zero", i)
		}
		if err := validateAmount(item.Quantity, fmt.Sprintf("items[%d].quantity", i)); err != nil {
			return domain.ComboInput{}, err
		}
	}
	if len(in.BranchIDs) == 0 {
		return invalid("a combo is sold in at least one branch")
	}
	branches := make(map[uuid.UUID]bool, len(in.BranchIDs))
	unique := in.BranchIDs[:0:0]
	for _, id := range in.BranchIDs {
		if !branches[id] {
			branches[id] = true
			unique = append(unique, id)
		}
	}
	in.BranchIDs = unique
	return in, nil
}
