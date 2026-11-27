//go:build integration

package repository

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func TestOnboardingReadinessTracksCurrentBranchPricesAndIsolation(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	account := seedAccount(t, db, "Readiness")
	other := seedAccount(t, db, "Foreign readiness")
	product := seedProduct(t, db, account, "Cement")
	seedUser(t, db, account, "ADMIN")
	var branch uuid.UUID
	if err := db.CrossAccount().QueryRow(ctx, `SELECT id FROM branch WHERE account_id=$1`, account).Scan(&branch); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		mustCleanup(t, db.CrossAccount(), `DELETE FROM product_price WHERE account_id=$1`, account)
		mustCleanup(t, db.CrossAccount(), `DELETE FROM branch_product WHERE account_id=$1`, account)
	})
	read := func() domain.BranchReadiness {
		t.Helper()
		var result *domain.OnboardingReadiness
		err := db.InTenantTx(ctx, domain.Tenant{AccountID: account}, func(q Querier) error {
			var err error
			result, err = NewOnboardingRepository().GetReadiness(ctx, q, account, []uuid.UUID{branch})
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Branches) != 1 {
			t.Fatalf("branches=%+v", result.Branches)
		}
		return result.Branches[0]
	}
	if got := read(); got.Products != 0 || got.MissingPrices != 0 {
		t.Fatal("empty catalog was reported as populated")
	}
	if _, err := db.CrossAccount().Exec(ctx, `INSERT INTO branch_product(account_id,branch_id,product_id,is_active) VALUES($1,$2,$3,TRUE)`, account, branch, product); err != nil {
		t.Fatal(err)
	}
	if got := read(); got.Products != 1 || got.MissingPrices != 1 || !got.HasStaff {
		t.Fatalf("missing price: %+v", got)
	}
	if _, err := db.CrossAccount().Exec(ctx, `INSERT INTO product_price(account_id,branch_id,product_id,price,currency,valid_from) VALUES($1,$2,$3,0,'ARS',now())`, account, branch, product); err != nil {
		t.Fatal(err)
	}
	if got := read(); got.MissingPrices != 0 {
		t.Fatal("zero price is valid in the existing domain")
	}
	if _, err := db.CrossAccount().Exec(ctx, `UPDATE product_price SET valid_to=now() WHERE account_id=$1 AND product_id=$2`, account, product); err != nil {
		t.Fatal(err)
	}
	if got := read(); got.MissingPrices != 1 {
		t.Fatal("expired price was counted")
	}
	if _, err := db.CrossAccount().Exec(ctx, `UPDATE product SET is_active=FALSE WHERE account_id=$1 AND id=$2`, account, product); err != nil {
		t.Fatal(err)
	}
	if got := read(); got.Products != 0 || got.MissingPrices != 0 {
		t.Fatal("inactive product was included")
	}
	result, err := NewOnboardingRepository().GetReadiness(ctx, db.CrossAccount(), other, []uuid.UUID{branch})
	if err != nil || len(result.Branches) != 0 {
		t.Fatalf("foreign branch was exposed: %+v %v", result, err)
	}
}
