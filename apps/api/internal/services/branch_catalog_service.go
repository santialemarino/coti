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

// moneyMax is the largest value NUMERIC(14,2) holds — 12 integer digits and two decimals.
// Like domain.MoneyScale it is a schema fact: a value past it is rejected here rather than
// blowing up in the database as a 500.
var moneyMax = decimal.RequireFromString("999999999999.99")

// productLookup is the single product read the per-branch use cases need: the in-account
// check that a foreign key does not perform.
type productLookup interface {
	GetByID(ctx context.Context, q repository.Querier, accountID, id uuid.UUID) (*domain.Product, error)
}

// branchProductRepository is the availability persistence surface.
type branchProductRepository interface {
	ListByProduct(ctx context.Context, q repository.Querier, accountID, productID uuid.UUID, branchID *uuid.UUID) ([]domain.BranchProduct, error)
	Save(ctx context.Context, q repository.Querier, accountID, branchID, productID uuid.UUID, in domain.BranchAvailability) (*domain.BranchProduct, error)
}

// productPriceRepository is the price persistence surface.
type productPriceRepository interface {
	ListByProduct(ctx context.Context, q repository.Querier, accountID, productID uuid.UUID, branchID *uuid.UUID) ([]domain.ProductPrice, error)
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
// selected one, every branch of the account otherwise.
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
			branchFilter(tenant))
		return listErr
	})
	if err != nil {
		return nil, err
	}
	return availability, nil
}

// SetAvailability records whether the active branch sells the product and with how much
// stock, creating the row or updating it.
//
// Deactivating is how a branch stops offering an item the account still catalogs.
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
// for the active branch, or for every branch of the account when none is selected.
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
			branchFilter(tenant))
		return listErr
	})
	if err != nil {
		return nil, err
	}
	return prices, nil
}

// SetPrice prices the product at the active branch by opening a new validity period.
//
// It never overwrites: the open period is closed at the instant the new one starts, and a
// new row is inserted, both in the same transaction. That is what keeps the history of
// what was quoted when — a quote frozen last month has to stay explainable.
//
// min_price is the floor the deterministic discount engine may not cross, so it cannot
// exceed the price it floors. Returns domain.ErrInvalidInput when the amounts do not hold
// up or when valid_from would start the new period before the one it replaces.
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
		if _, getErr := s.products.GetByID(ctx, q, tenant.AccountID, productID); getErr != nil {
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

// branchFilter narrows a per-branch read to the request's active branch, or returns nil for
// the account-wide read an admin comparing branches performs.
func branchFilter(tenant domain.Tenant) *uuid.UUID {
	if !tenant.HasBranch() {
		return nil
	}
	branchID := tenant.BranchID
	return &branchID
}

// requireBranch rejects a per-branch write that arrived without an active branch. Writing
// availability or a price to a guessed branch is never right, so the request has to name
// one through the X-Branch-Id header.
func requireBranch(tenant domain.Tenant, what string) error {
	if !tenant.HasBranch() {
		return fmt.Errorf("%w: setting %s needs an active branch, sent as the X-Branch-Id header",
			domain.ErrInvalidInput, what)
	}
	return nil
}

// validateAmount rejects what NUMERIC(14,2) cannot hold exactly, plus negatives.
//
// Without the scale check the database would round a third decimal away silently, which on
// money is a defect and not a rounding preference; without the range check an extra typed
// digit becomes a 500 instead of a message the seller can act on.
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
