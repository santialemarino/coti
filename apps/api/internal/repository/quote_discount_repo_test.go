//go:build integration

package repository

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// seedQuoteDiscount creates one seller-typed total discount on a version and takes it away
// afterwards. The chain teardown registers first and runs last, so the discount cleanup
// registered here runs before the version itself is deleted.
func seedQuoteDiscount(
	t *testing.T, db *DB, accountID, versionID uuid.UUID, in domain.QuoteDiscountCreate,
) *domain.QuoteDiscount {
	t.Helper()
	repo := NewQuoteDiscountRepository()
	var created *domain.QuoteDiscount
	if err := db.InTenantTx(context.Background(),
		domain.Tenant{AccountID: accountID, Role: domain.UserRoleAdmin},
		func(q Querier) error {
			var createErr error
			created, createErr = repo.Create(context.Background(), q, accountID, versionID, in)
			return createErr
		}); err != nil {
		t.Fatalf("seed quote discount: %v", err)
	}
	t.Cleanup(func() {
		mustCleanup(t, db.CrossAccount(),
			`DELETE FROM quote_discount_item qdi USING quote_discount qd
			 WHERE qd.quote_version_id = $1 AND qdi.quote_discount_id = qd.id`, versionID)
		mustCleanup(t, db.CrossAccount(),
			`DELETE FROM quote_discount WHERE quote_version_id = $1`, versionID)
	})
	return created
}

// uuidSliceContains reports whether a set of line ids carries the one named.
func uuidSliceContains(ids []uuid.UUID, want uuid.UUID) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

// The discount joins promotion.name, so a seller-typed discount with no promotion has to
// display its own description, and a promotion-backed one the promotion's name over it.
func TestQuoteDiscountRepository_CreateAndList_RoundTripTheApplication(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountID := seedAccount(t, db, "Discount round trip")
	branchID := branchOf(t, db, accountID)
	productID := seedProduct(t, db, accountID, "Cemento Portland 50kg")
	priceCleanup(t, db, productID)
	_, versionID, _ := seedQuoteChain(t, db, accountID, branchID, productID)

	promotionID := uuid.New()
	if _, err := db.CrossAccount().Exec(ctx,
		`INSERT INTO promotion (id, account_id, condition_type, action_type, action_value, name)
		 VALUES ($1, $2, 'ON_TOTAL', 'FIXED_AMOUNT', 300, 'Promo contado mensual')`,
		promotionID, accountID); err != nil {
		t.Fatalf("seed promotion: %v", err)
	}
	t.Cleanup(func() {
		mustCleanup(t, db.CrossAccount(), `DELETE FROM promotion WHERE id = $1`, promotionID)
	})

	typed := seedQuoteDiscount(t, db, accountID, versionID, domain.QuoteDiscountCreate{
		Description: "Por volumen de obra",
		ActionType:  domain.PromotionActionFixedAmount,
		Value:       decimal.RequireFromString("250.00"),
		Amount:      decimal.RequireFromString("250.00"),
		Scope:       domain.DiscountScopeTotal,
	})

	var discounts []domain.QuoteDiscount
	if err := db.InTenantTx(ctx,
		domain.Tenant{AccountID: accountID, Role: domain.UserRoleAdmin},
		func(q Querier) error {
			var readErr error
			discounts, readErr = NewQuoteDiscountRepository().
				ListByVersionID(ctx, q, accountID, versionID)
			return readErr
		}); err != nil {
		t.Fatalf("ListByVersionID: %v", err)
	}
	if len(discounts) != 1 {
		t.Fatalf("discounts = %d, want 1", len(discounts))
	}
	got := discounts[0]
	if got.PromotionID != nil {
		t.Errorf("promotion id = %v, want nil", *got.PromotionID)
	}
	if got.PromotionName == nil || *got.PromotionName != "Por volumen de obra" {
		t.Errorf("promotion name = %v, want the discount's own description", got.PromotionName)
	}
	if got.ActionType != domain.PromotionActionFixedAmount ||
		got.ActionValue == nil || !got.ActionValue.Equal(decimal.RequireFromString("250.00")) {
		t.Errorf("action = (%q, %v), want (FIXED_AMOUNT, 250.00)", got.ActionType, got.ActionValue)
	}
	assertDiscountApply(t, got, domain.PromotionConditionOnTotal, domain.DiscountScopeTotal,
		domain.DiscountOriginManualSeller, "250.00", false)

	db.CrossAccount().Exec(ctx,
		`UPDATE quote_discount SET promotion_id = $2 WHERE id = $1`, typed.ID, promotionID)
	var promoted []domain.QuoteDiscount
	if err := db.InTenantTx(ctx,
		domain.Tenant{AccountID: accountID, Role: domain.UserRoleAdmin},
		func(q Querier) error {
			var readErr error
			promoted, readErr = NewQuoteDiscountRepository().
				ListByVersionID(ctx, q, accountID, versionID)
			return readErr
		}); err != nil {
		t.Fatalf("ListByVersionID after promotion: %v", err)
	}
	if promoted[0].PromotionName == nil || *promoted[0].PromotionName != "Promo contado mensual" {
		t.Errorf("promotion name = %v, want the promotion's name over the description",
			promoted[0].PromotionName)
	}
}

func assertDiscountApply(
	t *testing.T, got domain.QuoteDiscount, condition domain.PromotionConditionType,
	scope domain.DiscountScope, origin domain.DiscountOrigin, amount string, suppressed bool,
) {
	t.Helper()
	if got.ConditionType != condition || got.Scope != scope || got.Origin != origin {
		t.Errorf("apply = (%q, %q, %q), want (%q, %q, %q)", got.ConditionType, got.Scope,
			got.Origin, condition, scope, origin)
	}
	if !got.Amount.Equal(decimal.RequireFromString(amount)) {
		t.Errorf("amount = %v, want %v", got.Amount, amount)
	}
	if got.SuppressedBySeller != suppressed {
		t.Errorf("suppressed = %v, want %v", got.SuppressedBySeller, suppressed)
	}
	if got.CreatedAt.IsZero() {
		t.Error("created_at = zero, want the database's timestamp")
	}
}

// The write guard is the version's is_immutable flag, checked inside the statement so a
// frozen version can never receive, patch or lose a discount.
func TestQuoteDiscountRepository_RefusesAFrozenVersion(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountID := seedAccount(t, db, "Discount frozen version")
	branchID := branchOf(t, db, accountID)
	productID := seedProduct(t, db, accountID, "Cemento Portland 50kg")
	priceCleanup(t, db, productID)
	_, versionID, _ := seedQuoteChain(t, db, accountID, branchID, productID)

	tenant := domain.Tenant{AccountID: accountID, Role: domain.UserRoleAdmin}
	repo := NewQuoteDiscountRepository()
	frozen := domain.QuoteDiscountCreate{
		Description: "Tarde",
		ActionType:  domain.PromotionActionFixedAmount,
		Value:       decimal.RequireFromString("10.00"),
		Amount:      decimal.RequireFromString("10.00"),
		Scope:       domain.DiscountScopeTotal,
	}

	created := seedQuoteDiscount(t, db, accountID, versionID, frozen)
	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		_, err := repo.Create(ctx, q, accountID, versionID, frozen)
		return err
	}); err != nil {
		t.Fatalf("Create on a mutable version = %v, want no error", err)
	}
	if _, err := db.CrossAccount().Exec(ctx,
		`UPDATE quote_version SET is_immutable = TRUE WHERE id = $1`, versionID); err != nil {
		t.Fatalf("freeze the version: %v", err)
	}

	if _, err := repo.Create(ctx, db.CrossAccount(),
		accountID, versionID, frozen); err == nil {
		t.Fatal("Create on a frozen version = nil, want the refusal")
	}
	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		_, err := repo.Create(ctx, q, accountID, versionID, frozen)
		if !errors.Is(err, domain.ErrNotFound) {
			return err
		}
		_, err = repo.UpdateByID(ctx, q, accountID, versionID, created.ID,
			domain.QuoteDiscountUpdate{})
		if !errors.Is(err, domain.ErrNotFound) {
			return err
		}
		return repo.DeleteByID(ctx, q, accountID, versionID, created.ID)
	}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("write path on a frozen version = %v, want ErrNotFound", err)
	}
}

// The create reads the version through quote_version, so a discount named by another account
// is refused before any row can land; the version and the row both stay put.
func TestQuoteDiscountRepository_RefusesAnotherAccountsVersion(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	victimAccount := seedAccount(t, db, "Discount victim")
	intruderAccount := seedAccount(t, db, "Discount intruder")
	victimBranch := branchOf(t, db, victimAccount)
	productID := seedProduct(t, db, victimAccount, "Cemento Portland 50kg")
	priceCleanup(t, db, productID)
	_, versionID, _ := seedQuoteChain(t, db, victimAccount, victimBranch, productID)

	repo := NewQuoteDiscountRepository()
	err := db.InTenantTx(ctx,
		domain.Tenant{AccountID: intruderAccount, Role: domain.UserRoleAdmin},
		func(q Querier) error {
			_, createErr := repo.Create(ctx, q, intruderAccount, versionID,
				domain.QuoteDiscountCreate{
					Description: "Del intruso",
					ActionType:  domain.PromotionActionFixedAmount,
					Value:       decimal.RequireFromString("100.00"),
					Amount:      decimal.RequireFromString("100.00"),
					Scope:       domain.DiscountScopeTotal,
				})
			return createErr
		})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Create across accounts = %v, want ErrNotFound", err)
	}

	var written int
	if err := db.CrossAccount().QueryRow(ctx,
		`SELECT count(*) FROM quote_discount WHERE quote_version_id = $1`, versionID).
		Scan(&written); err != nil {
		t.Fatalf("count the victim's discounts: %v", err)
	}
	if written != 0 {
		t.Errorf("the victim's version carries %d discounts, want none", written)
	}
}

// Same guard on the update and delete: an unchanging is_immutable flag would silently write
// another account's row, so the write counts its own match and refuses zero like the create.
func TestQuoteDiscountRepository_RefusesToTouchAnotherTenantsDiscount(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	victimAccount := seedAccount(t, db, "Discount write victim")
	intruderAccount := seedAccount(t, db, "Discount write intruder")
	victimBranch := branchOf(t, db, victimAccount)
	productID := seedProduct(t, db, victimAccount, "Cemento Portland 50kg")
	priceCleanup(t, db, productID)
	_, versionID, _ := seedQuoteChain(t, db, victimAccount, victimBranch, productID)
	created := seedQuoteDiscount(t, db, victimAccount, versionID, domain.QuoteDiscountCreate{
		Description: "El premio",
		ActionType:  domain.PromotionActionFixedAmount,
		Value:       decimal.RequireFromString("500.00"),
		Amount:      decimal.RequireFromString("500.00"),
		Scope:       domain.DiscountScopeTotal,
	})

	repo := NewQuoteDiscountRepository()
	err := db.InTenantTx(ctx,
		domain.Tenant{AccountID: intruderAccount, Role: domain.UserRoleAdmin},
		func(q Querier) error {
			updateErr := repo.UpdateAmount(ctx, q, intruderAccount, versionID, created.ID,
				decimal.RequireFromString("1.00"))
			if !errors.Is(updateErr, domain.ErrNotFound) {
				return updateErr
			}
			return repo.DeleteByID(ctx, q, intruderAccount, versionID, created.ID)
		})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("write path across tenants = %v, want ErrNotFound", err)
	}

	var amount decimal.Decimal
	if err := db.CrossAccount().QueryRow(ctx,
		`SELECT amount FROM quote_discount WHERE id = $1`, created.ID).Scan(&amount); err != nil {
		t.Fatalf("read the victim's discount back: %v", err)
	}
	if !amount.Equal(decimal.RequireFromString("500.00")) {
		t.Errorf("the victim's discount = %v, want 500.00 untouched", amount)
	}
}

// UpdateByID patches only the fields present, so a suppression toggle alone leaves the
// amount where it was; a real delete removes the MANUAL_SELLER row for good.
func TestQuoteDiscountRepository_UpdateAndDelete_WriteOnlyWhatIsPresent(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountID := seedAccount(t, db, "Discount update and delete")
	branchID := branchOf(t, db, accountID)
	productID := seedProduct(t, db, accountID, "Cemento Portland 50kg")
	priceCleanup(t, db, productID)
	_, versionID, _ := seedQuoteChain(t, db, accountID, branchID, productID)

	tenant := domain.Tenant{AccountID: accountID, Role: domain.UserRoleAdmin}
	repo := NewQuoteDiscountRepository()
	created := seedQuoteDiscount(t, db, accountID, versionID, domain.QuoteDiscountCreate{
		Description: "Sin explicacion",
		ActionType:  domain.PromotionActionFixedAmount,
		Value:       decimal.RequireFromString("250.00"),
		Amount:      decimal.RequireFromString("250.00"),
		Scope:       domain.DiscountScopeTotal,
	})

	suppress := true
	var updated *domain.QuoteDiscount
	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		var updateErr error
		updated, updateErr = repo.UpdateByID(ctx, q, accountID, versionID, created.ID,
			domain.QuoteDiscountUpdate{SuppressedBySeller: &suppress})
		return updateErr
	}); err != nil {
		t.Fatalf("UpdateByID suppression toggle = %v, want no error", err)
	}
	if !updated.SuppressedBySeller {
		t.Errorf("suppressed = %v, want TRUE", updated.SuppressedBySeller)
	}
	if !updated.Amount.Equal(decimal.RequireFromString("250.00")) {
		t.Errorf("amount = %v, want 250.00: the toggle wrote the amount", updated.Amount)
	}

	freshAmount := decimal.RequireFromString("150.00")
	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		return repo.UpdateAmount(ctx, q, accountID, versionID, created.ID, freshAmount)
	}); err != nil {
		t.Fatalf("UpdateAmount = %v, want no error", err)
	}
	var stored decimal.Decimal
	if err := db.CrossAccount().QueryRow(ctx,
		`SELECT amount FROM quote_discount WHERE id = $1`, created.ID).Scan(&stored); err != nil {
		t.Fatalf("read the amount back: %v", err)
	}
	if !stored.Equal(freshAmount) {
		t.Errorf("amount = %v, want 150.00 written by UpdateAmount", stored)
	}

	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		return repo.DeleteByID(ctx, q, accountID, versionID, created.ID)
	}); err != nil {
		t.Fatalf("DeleteByID = %v, want no error", err)
	}
	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		return repo.DeleteByID(ctx, q, accountID, versionID, created.ID)
	}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("second DeleteByID = %v, want ErrNotFound", err)
	}

	var remaining int
	if err := db.CrossAccount().QueryRow(ctx,
		`SELECT count(*) FROM quote_discount WHERE id = $1`, created.ID).Scan(&remaining); err != nil {
		t.Fatalf("count the deleted discount: %v", err)
	}
	if remaining != 0 {
		t.Errorf("the deleted discount is still a row %d times over", remaining)
	}
}

// The rule round trip: a scoped discount carries its action into the row, the covered lines
// are linked and readable in batches, replacing the lines swaps them, and the delete removes
// the links with the row.
func TestQuoteDiscountRepository_ScopeAndLines_RoundTripTheRule(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountID := seedAccount(t, db, "Discount scope and lines")
	branchID := branchOf(t, db, accountID)
	productID := seedProduct(t, db, accountID, "Cemento Portland 50kg")
	priceCleanup(t, db, productID)
	_, versionID, _ := seedQuoteChain(t, db, accountID, branchID, productID)

	tenant := domain.Tenant{AccountID: accountID, Role: domain.UserRoleAdmin}
	repo := NewQuoteDiscountRepository()
	firstItemID := uuid.New()
	secondItemID := uuid.New()
	if _, err := db.CrossAccount().Exec(ctx,
		`INSERT INTO quote_item (id, account_id, version_id, requested_description,
		                         quantity, match_status, subtotal)
		 VALUES ($1, $2, $3, 'Primera línea', 1, 'MATCHED', 100),
		        ($4, $2, $3, 'Segunda línea', 1, 'MATCHED', 150)`,
		firstItemID, accountID, versionID, secondItemID); err != nil {
		t.Fatalf("seed two quote items: %v", err)
	}
	t.Cleanup(func() {
		mustCleanup(t, db.CrossAccount(),
			`DELETE FROM quote_discount_item qdi USING quote_item qi
			 WHERE qi.version_id = $1 AND qdi.quote_item_id = qi.id`, versionID)
		mustCleanup(t, db.CrossAccount(),
			`DELETE FROM quote_item WHERE version_id = $1`, versionID)
	})

	created := seedQuoteDiscount(t, db, accountID, versionID, domain.QuoteDiscountCreate{
		Description: "Bono por líneas",
		ActionType:  domain.PromotionActionPercentage,
		Value:       decimal.RequireFromString("5.00"),
		Amount:      decimal.Zero,
		Scope:       domain.DiscountScopeItemSet,
	})
	if got := created.Scope; got != domain.DiscountScopeItemSet {
		t.Errorf("scope = %q, want ITEM_SET", got)
	}
	if got := created.ConditionType; got != domain.PromotionConditionItemSet {
		t.Errorf("condition_type = %q, want ITEM_SET derived from scope", got)
	}
	if created.ActionValue == nil || !created.ActionValue.Equal(decimal.RequireFromString("5.00")) {
		t.Errorf("action_value = %v, want 5.00", created.ActionValue)
	}
	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		return repo.CreateItemLinks(ctx, q, accountID, versionID, created.ID,
			[]uuid.UUID{firstItemID, secondItemID})
	}); err != nil {
		t.Fatalf("CreateItemLinks = %v, want no error", err)
	}

	var linked, batch map[uuid.UUID][]uuid.UUID
	var readIDs []uuid.UUID
	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		var err error
		readIDs, err = repo.ListItemIDs(ctx, q, accountID, created.ID)
		if err == nil {
			linked, err = repo.ListItemIDsByDiscountIDs(ctx, q, accountID, versionID,
				[]uuid.UUID{created.ID})
		}
		if err == nil {
			batch, err = repo.ListItemIDsByDiscountIDs(ctx, q, accountID, versionID,
				[]uuid.UUID{uuid.New()})
		}
		return err
	}); err != nil {
		t.Fatalf("read the discount's lines: %v", err)
	}
	if len(readIDs) != 2 ||
		!uuidSliceContains(readIDs, firstItemID) || !uuidSliceContains(readIDs, secondItemID) {
		t.Errorf("ListItemIDs = %v, want both lines", readIDs)
	}
	if len(linked[created.ID]) != 2 {
		t.Errorf("batch for the discount = %v, want both lines", linked[created.ID])
	}
	if len(batch) != 0 {
		t.Errorf("batch for an unknown discount = %v, want empty", batch)
	}

	item := domain.QuoteDiscountCreate{
		Description: "Bono por líneas",
		ActionType:  domain.PromotionActionPercentage,
		Value:       decimal.RequireFromString("5.00"),
		Amount:      decimal.Zero,
		Scope:       domain.DiscountScopeItem,
		ItemIDs:     []uuid.UUID{firstItemID},
	}
	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		return repo.ReplaceItemLinks(ctx, q, accountID, versionID, created.ID, item.ItemIDs)
	}); err != nil {
		t.Fatalf("ReplaceItemLinks = %v, want no error", err)
	}
	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		ids, err := repo.ListItemIDs(ctx, q, accountID, created.ID)
		if err != nil {
			return err
		}
		if len(ids) != 1 || ids[0] != firstItemID {
			return fmt.Errorf("links after replace = %v, want only the first line", ids)
		}
		return nil
	}); err != nil {
		t.Fatalf("links after replace: %v", err)
	}

	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		return repo.DeleteByID(ctx, q, accountID, versionID, created.ID)
	}); err != nil {
		t.Fatalf("DeleteByID = %v, want no error", err)
	}
	var linkRows int
	if err := db.CrossAccount().QueryRow(ctx,
		`SELECT count(*) FROM quote_discount_item WHERE quote_discount_id = $1`,
		created.ID).Scan(&linkRows); err != nil {
		t.Fatalf("count the links after delete: %v", err)
	}
	if linkRows != 0 {
		t.Errorf("the delete left %d link rows behind", linkRows)
	}
}
