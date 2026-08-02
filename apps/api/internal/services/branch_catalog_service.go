package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// moneyMax is the largest value NUMERIC(14,2) holds. Rejecting past it here keeps an extra
// typed digit from becoming a 500.
var moneyMax = decimal.RequireFromString("999999999999.99")

// productLookup is the product read the per-branch use cases need: the in-account check that
// a foreign key does not perform, plus the locking variant a price write serializes on.
type productLookup interface {
	GetByID(ctx context.Context, q repository.Querier, accountID, id uuid.UUID) (*domain.Product, error)
	GetByIDForUpdate(ctx context.Context, q repository.Querier, accountID, id uuid.UUID) (*domain.Product, error)
}

// branchProductRepository is the availability persistence surface.
type branchProductRepository interface {
	ListByProduct(ctx context.Context, q repository.Querier, accountID, productID uuid.UUID, branchIDs []uuid.UUID) ([]domain.BranchProduct, error)
	Save(ctx context.Context, q repository.Querier, accountID, branchID, productID uuid.UUID, in domain.BranchAvailability) (*domain.BranchProduct, error)
}

// productPriceRepository is the price persistence surface.
type productPriceRepository interface {
	ListByProduct(ctx context.Context, q repository.Querier, accountID, productID uuid.UUID, branchIDs []uuid.UUID) ([]domain.ProductPrice, error)
	GetOpenPeriod(ctx context.Context, q repository.Querier, accountID, branchID, productID uuid.UUID) (*domain.ProductPrice, error)
	Create(ctx context.Context, q repository.Querier, accountID, branchID, productID uuid.UUID, userID *uuid.UUID, in domain.NewProductPrice) (*domain.ProductPrice, error)
	CloseOpenPeriod(ctx context.Context, q repository.Querier, accountID, branchID, productID uuid.UUID, at time.Time) (int64, error)
}

// BranchCatalogService owns the per-branch face of the catalog: what a branch carries, and
// at what price.
type BranchCatalogService struct {
	db           tenantTxRunner
	products     productLookup
	availability branchProductRepository
	prices       productPriceRepository
	now          func() time.Time
}

// NewBranchCatalogService builds a BranchCatalogService. now is injectable so the validity
// periods a price update writes are deterministic in tests.
func NewBranchCatalogService(
	db tenantTxRunner, products productLookup, availability branchProductRepository,
	prices productPriceRepository, now func() time.Time,
) *BranchCatalogService {
	if now == nil {
		now = time.Now
	}
	return &BranchCatalogService{
		db: db, products: products, availability: availability, prices: prices, now: now,
	}
}

// ListAvailability returns where the product is sold: the active branch when the request
// selected one, otherwise every branch the caller reaches.
func (s *BranchCatalogService) ListAvailability(
	ctx context.Context, tenant domain.Tenant, productID uuid.UUID,
) ([]domain.BranchProduct, error) {
	var availability []domain.BranchProduct
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		if _, getErr := s.products.GetByID(ctx, q, tenant.AccountID, productID); getErr != nil {
			return getErr
		}
		var listErr error
		availability, listErr = s.availability.ListByProduct(ctx, q, tenant.AccountID, productID,
			tenant.BranchFilter())
		return listErr
	})
	if err != nil {
		return nil, err
	}
	return availability, nil
}

// SetAvailability records whether the active branch sells the product and with how much
// stock, creating the row or updating it.
func (s *BranchCatalogService) SetAvailability(
	ctx context.Context, tenant domain.Tenant, productID uuid.UUID, in domain.BranchAvailability,
) (*domain.BranchProduct, error) {
	if err := requireBranch(tenant, "availability"); err != nil {
		return nil, err
	}
	if in.Stock.Valid {
		if err := validateAmount(in.Stock.Decimal, "stock"); err != nil {
			return nil, err
		}
	}

	var saved *domain.BranchProduct
	if err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		if _, getErr := s.products.GetByID(ctx, q, tenant.AccountID, productID); getErr != nil {
			return getErr
		}
		var saveErr error
		saved, saveErr = s.availability.Save(ctx, q, tenant.AccountID, tenant.BranchID, productID, in)
		return saveErr
	}); err != nil {
		return nil, err
	}
	return saved, nil
}

// ListPrices returns the product's price history — open and closed periods, newest first —
// for the active branch, or for every branch the caller reaches when none is selected.
func (s *BranchCatalogService) ListPrices(
	ctx context.Context, tenant domain.Tenant, productID uuid.UUID,
) ([]domain.ProductPrice, error) {
	var prices []domain.ProductPrice
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		if _, getErr := s.products.GetByID(ctx, q, tenant.AccountID, productID); getErr != nil {
			return getErr
		}
		var listErr error
		prices, listErr = s.prices.ListByProduct(ctx, q, tenant.AccountID, productID,
			tenant.BranchFilter())
		return listErr
	})
	if err != nil {
		return nil, err
	}
	return prices, nil
}

// SetPrice prices the product at the active branch by opening a new validity period.
//
// It never overwrites: the open period is closed at the instant the new one starts and a
// new row is inserted, in one transaction serialized on the product row, so a quote frozen
// last month stays explainable. Returns domain.ErrInvalidInput when the amounts do not hold
// up or when valid_from would precede the period it replaces.
func (s *BranchCatalogService) SetPrice(
	ctx context.Context, tenant domain.Tenant, productID uuid.UUID, in domain.NewProductPrice,
) (*domain.ProductPrice, error) {
	if err := requireBranch(tenant, "prices"); err != nil {
		return nil, err
	}
	if err := validateAmount(in.Price, "price"); err != nil {
		return nil, err
	}
	if in.MinPrice.Valid {
		if err := validateAmount(in.MinPrice.Decimal, "min_price"); err != nil {
			return nil, err
		}
		if in.MinPrice.Decimal.GreaterThan(in.Price) {
			return nil, fmt.Errorf("%w: min_price cannot exceed price", domain.ErrInvalidInput)
		}
	}
	if in.Currency == "" {
		in.Currency = domain.DefaultCurrency
	}
	if in.ValidFrom.IsZero() {
		in.ValidFrom = s.now()
	}

	var price *domain.ProductPrice
	if err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		// Locking, not just checking: two repricings racing here would each read the same
		// open period, each fail to close the other's, and leave the product with two.
		if _, getErr := s.products.GetByIDForUpdate(ctx, q, tenant.AccountID, productID); getErr != nil {
			return getErr
		}

		open, getErr := s.prices.GetOpenPeriod(ctx, q, tenant.AccountID, tenant.BranchID, productID)
		switch {
		case getErr == nil:
			// A period cannot end before it began, and back-dating past the price it
			// replaces would rewrite which price applied at a moment already quoted.
			if in.ValidFrom.Before(open.ValidFrom) {
				return fmt.Errorf("%w: valid_from cannot precede the current period, which starts at %s",
					domain.ErrInvalidInput, open.ValidFrom.Format(time.RFC3339))
			}
			if _, closeErr := s.prices.CloseOpenPeriod(ctx, q, tenant.AccountID, tenant.BranchID,
				productID, in.ValidFrom); closeErr != nil {
				return closeErr
			}
		case errors.Is(getErr, domain.ErrNotFound):
			// First price for this product at this branch: nothing to close.
		default:
			return getErr
		}

		var createErr error
		price, createErr = s.prices.Create(ctx, q, tenant.AccountID, tenant.BranchID, productID,
			&tenant.UserID, in)
		return createErr
	}); err != nil {
		return nil, err
	}
	return price, nil
}

// requireBranch rejects a per-branch write that arrived without an active branch: a
// guessed branch would price the wrong one.
func requireBranch(tenant domain.Tenant, what string) error {
	if !tenant.HasBranch() {
		return fmt.Errorf("%w: setting %s needs an active branch, sent as the X-Branch-Id header",
			domain.ErrInvalidInput, what)
	}
	return nil
}

// validateAmount rejects what NUMERIC(14,2) cannot hold exactly, plus negatives. The scale
// check matters because the database would round a third decimal away silently.
func validateAmount(amount decimal.Decimal, field string) error {
	if amount.IsNegative() {
		return fmt.Errorf("%w: %s cannot be negative", domain.ErrInvalidInput, field)
	}
	if amount.Exponent() < -domain.MoneyScale {
		return fmt.Errorf("%w: %s takes at most %d decimals", domain.ErrInvalidInput, field,
			domain.MoneyScale)
	}
	if amount.GreaterThan(moneyMax) {
		return fmt.Errorf("%w: %s cannot exceed %s", domain.ErrInvalidInput, field, moneyMax)
	}
	return nil
}
