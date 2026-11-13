//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
	"github.com/santialemarino/coti/apps/api/internal/services"
)

func TestCombo_SuggestAndCompleteOnAPricedQuote(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	accountID, branchID := e.seedAccount(t, "Combos")
	seller := e.seedUser(t, accountID, domain.UserRoleSeller)
	cement := e.seedPricedProduct(t, accountID, branchID, "Cemento", "100.00", nil)
	sand := e.seedPricedProduct(t, accountID, branchID, "Arena", "50.00", nil)
	t.Cleanup(func() {
		e.mustCleanup(t, `DELETE FROM combo_item WHERE account_id = $1`, accountID)
		e.mustCleanup(t, `DELETE FROM branch_combo WHERE account_id = $1`, accountID)
		e.mustCleanup(t, `DELETE FROM combo WHERE account_id = $1`, accountID)
	})
	admin := domain.Tenant{AccountID: accountID, BranchID: branchID, UserID: seller.ID}
	combos := services.NewComboService(e.db, repository.NewComboRepository())

	otherAccount, otherBranch := e.seedAccount(t, "Other account")
	foreign := e.seedPricedProduct(t, otherAccount, otherBranch, "Ajeno", "10.00", nil)
	_, err := combos.Create(ctx, admin, domain.ComboInput{Name: "Robado", IsActive: true,
		Items: []domain.ComboItemInput{
			{ProductID: cement, Quantity: decimal.NewFromInt(1)}, {ProductID: foreign, Quantity: decimal.NewFromInt(1)},
		}, BranchIDs: []uuid.UUID{branchID}})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("combo with another account's product: %v, want invalid input", err)
	}

	_, err = combos.Create(ctx, admin, domain.ComboInput{Name: "Sucursal ajena", IsActive: true,
		Items: []domain.ComboItemInput{
			{ProductID: cement, Quantity: decimal.NewFromInt(1)}, {ProductID: sand, Quantity: decimal.NewFromInt(1)},
		}, BranchIDs: []uuid.UUID{otherBranch}})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("combo sold at another account's branch: %v, want invalid input", err)
	}

	combo, err := combos.Create(ctx, admin, domain.ComboInput{Name: "Obra gruesa", IsActive: true,
		Items: []domain.ComboItemInput{
			{ProductID: cement, Quantity: decimal.NewFromInt(10)}, {ProductID: sand, Quantity: decimal.NewFromInt(2)},
		}, BranchIDs: []uuid.UUID{branchID}})
	if err != nil || len(combo.Items) != 2 || len(combo.BranchIDs) != 1 {
		t.Fatalf("Create() = %+v, %v", combo, err)
	}
	// Items keep the order they were entered in, not the alphabetical one.
	if combo.Items[0].ProductID != cement || combo.Items[1].ProductID != sand {
		t.Fatalf("combo items = %+v, want cement then sand", combo.Items)
	}

	seeded := e.seedDraftQuote(t, accountID, branchID, seller, []draftLine{{description: "Cemento", quantity: "10", productID: &cement}})
	if _, err := e.db.CrossAccount().Exec(ctx, `UPDATE quote SET current_status='QUOTED' WHERE id=$1`, seeded.quoteID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.CrossAccount().Exec(ctx, `UPDATE quote_item SET unit_price_snapshot=100, subtotal=1000
		WHERE version_id=$1`, seeded.versionID); err != nil {
		t.Fatal(err)
	}
	quotes := services.NewRFQService(e.db, repository.NewRFQRepository(), repository.NewQuoteRepository(),
		repository.NewQuoteSendRepository(), repository.NewQuoteAIGenerationRepository(),
		repository.NewChannelRepository(), repository.NewUserRepository(), nil, nil, nil, rfqConfig()).
		WithPrices(repository.NewProductPriceRepository()).
		WithCombos(repository.NewComboRepository())

	suggestions, err := quotes.ComboSuggestions(ctx, admin, seeded.quoteID)
	if err != nil || len(suggestions) != 1 || len(suggestions[0].Missing) != 1 || suggestions[0].Missing[0].ProductID != sand {
		t.Fatalf("ComboSuggestions() = %+v, %v; want the sand missing", suggestions, err)
	}
	if _, err := quotes.ComboSuggestions(ctx, domain.Tenant{AccountID: otherAccount, BranchID: otherBranch}, seeded.quoteID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("other account: %v, want not found", err)
	}

	added, err := quotes.AddComboItems(ctx, admin, seeded.quoteID, combo.ID)
	if err != nil || len(added) != 1 || added[0].ProductID == nil || *added[0].ProductID != sand ||
		!added[0].UnitPriceSnapshot.Valid || !added[0].UnitPriceSnapshot.Decimal.Equal(decimal.NewFromInt(50)) ||
		!added[0].Quantity.Equal(decimal.NewFromInt(2)) {
		t.Fatalf("AddComboItems() = %+v, %v; want 2 sand at 50", added, err)
	}
	var total decimal.Decimal
	if err := e.db.CrossAccount().QueryRow(ctx, `SELECT total FROM quote_version WHERE id=$1`, seeded.versionID).Scan(&total); err != nil {
		t.Fatal(err)
	}
	if !total.Equal(decimal.NewFromInt(1100)) {
		t.Fatalf("version total = %s, want 1100 (1000 cement + 100 sand)", total)
	}

	// Complete now, so it is no longer suggested and a second press adds nothing.
	if _, err := quotes.AddComboItems(ctx, admin, seeded.quoteID, combo.ID); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("second AddComboItems() = %v, want conflict", err)
	}
	if suggestions, err = quotes.ComboSuggestions(ctx, admin, seeded.quoteID); err != nil || len(suggestions) != 0 {
		t.Fatalf("suggestions after completing = %+v, %v; want none", suggestions, err)
	}

	// A branch left out stops selling the combo there.
	other := e.seedBranch(t, accountID, "Sucursal 2")
	if _, err := combos.Replace(ctx, admin, combo.ID, domain.ComboInput{Name: "Obra gruesa", IsActive: true,
		Items: []domain.ComboItemInput{
			{ProductID: cement, Quantity: decimal.NewFromInt(10)}, {ProductID: sand, Quantity: decimal.NewFromInt(2)},
		}, BranchIDs: []uuid.UUID{other}}); err != nil {
		t.Fatal(err)
	}
	sold, err := repository.NewComboRepository().ListSoldAtBranch(ctx, e.db.CrossAccount(), accountID, branchID)
	if err != nil || len(sold) != 0 {
		t.Fatalf("combos sold at the first branch = %+v, %v; want none", sold, err)
	}
}
