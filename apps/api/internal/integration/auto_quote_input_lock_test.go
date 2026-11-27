//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

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
