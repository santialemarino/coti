//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

func TestAutomaticQuoteInputsUsePricesCommittedAfterTransactionStart(t *testing.T) {
	for _, firstPrice := range []bool{false, true} {
		t.Run(map[bool]string{false: "replacement", true: "first-price"}[firstPrice], func(t *testing.T) {
			e := newEnv(t)
			account, branch := e.seedAccount(t, "Concurrent automatic pricing")
			product := e.seedPricedProduct(t, account, branch, "Cement", "10", nil)
			ctx := context.Background()
			t.Cleanup(func() {
				e.mustCleanup(t, `DELETE FROM product_price WHERE account_id=$1 AND product_id=$2`, account, product)
			})
			if firstPrice {
				if _, err := e.db.CrossAccount().Exec(ctx, `DELETE FROM product_price WHERE account_id=$1 AND product_id=$2`, account, product); err != nil {
					t.Fatal(err)
				}
			}
			tenant := domain.Tenant{AccountID: account, BranchID: branch}
			prices := repository.NewProductPriceRepository()
			err := e.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
				var transactionStart time.Time
				if err := q.QueryRow(ctx, `SELECT now()`).Scan(&transactionStart); err != nil {
					return err
				}
				if err := e.db.InTenantTx(ctx, tenant, func(writer repository.Querier) error {
					if _, err := repository.NewProductRepository().GetByIDForUpdate(ctx, writer, account, product); err != nil {
						return err
					}
					effectiveAt, err := prices.DatabaseTime(ctx, writer)
					if err != nil {
						return err
					}
					if _, err := prices.CloseOpenPeriod(ctx, writer, account, branch, product, effectiveAt); err != nil {
						return err
					}
					_, err = prices.Create(ctx, writer, account, branch, product, nil, domain.NewProductPrice{Price: decimal.NewFromInt(20), Currency: "ARS", ValidFrom: effectiveAt})
					return err
				}); err != nil {
					return err
				}
				if err := prices.LockAutomaticQuoteInputs(ctx, q, account, branch, []uuid.UUID{product}); err != nil {
					return err
				}
				current, err := prices.GetCurrentByProductIDs(ctx, q, account, branch, []uuid.UUID{product})
				if err != nil {
					return err
				}
				if got, ok := current[product]; !ok || !got.Price.Equal(decimal.NewFromInt(20)) {
					t.Errorf("price after catalog lock = %v (found %v), want newly committed price 20", got.Price, ok)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestAutomaticQuoteInputsLockCatalogAndBranchRows(t *testing.T) {
	e := newEnv(t)
	account, branch := e.seedAccount(t, "Auto quote input locking")
	product := e.seedPricedProduct(t, account, branch, "Cement", "10", nil)
	ctx := context.Background()
	err := e.db.InTenantTx(ctx, domain.Tenant{AccountID: account, BranchID: branch}, func(q repository.Querier) error {
		if err := repository.NewProductPriceRepository().LockAutomaticQuoteInputs(ctx, q, account, branch, []uuid.UUID{product}); err != nil {
			return err
		}
		for _, query := range []string{`UPDATE product SET canonical_name='Changed' WHERE account_id=$1 AND id=$2`, `UPDATE branch_product SET is_active=FALSE WHERE account_id=$1 AND product_id=$2`} {
			lockedCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
			_, writeErr := e.db.CrossAccount().Exec(lockedCtx, query, account, product)
			cancel()
			if writeErr == nil {
				t.Fatal("catalog input changed during automatic valuation")
			}
			if lockedCtx.Err() == nil {
				t.Fatalf("writer failed without waiting for the row lock: %v", writeErr)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.CrossAccount().Exec(ctx, `UPDATE product SET canonical_name='Changed after valuation' WHERE account_id=$1 AND id=$2`, account, product); err != nil {
		t.Fatal(err)
	}
}
