//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
	"github.com/santialemarino/coti/apps/api/internal/secrets"
	"github.com/santialemarino/coti/apps/api/internal/services"
)

// Points of sale through the service and the real database: two branches swap numbers in one save,
// and a number two branches would share is refused with nothing written.
func TestInvoicing_PointsOfSaleSwapAndRefuseADuplicate(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	accountID, moron := e.seedAccount(t, "Corralón A")
	bosch := uuid.New()
	if _, err := e.db.CrossAccount().Exec(ctx,
		`INSERT INTO branch (id, account_id, name) VALUES ($1, $2, 'Villa Bosch')`, bosch, accountID); err != nil {
		t.Fatalf("seed branch: %v", err)
	}
	sealer, err := secrets.NewAESGCM(nil)
	if err != nil {
		t.Fatal(err)
	}
	invoices := services.NewInvoiceService(e.db, repository.NewInvoicingRepository(), repository.NewQuoteRepository(),
		repository.NewQuoteDiscountRepository(), nil, sealer, services.InvoiceSettings{}, nil, nil)
	tenant := domain.Tenant{AccountID: accountID, Role: domain.UserRoleAdmin}
	read := func() map[uuid.UUID]*int32 {
		points := map[uuid.UUID]*int32{}
		rows, err := e.db.CrossAccount().Query(ctx, `SELECT id, point_of_sale FROM branch WHERE account_id = $1`, accountID)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		for rows.Next() {
			var id uuid.UUID
			var point *int32
			if err := rows.Scan(&id, &point); err != nil {
				t.Fatal(err)
			}
			points[id] = point
		}
		return points
	}
	one, two := 1, 2

	if _, err := invoices.UpdatePointsOfSale(ctx, tenant, map[uuid.UUID]*int{moron: &one, bosch: &two}); err != nil {
		t.Fatal(err)
	}
	if _, err := invoices.UpdatePointsOfSale(ctx, tenant, map[uuid.UUID]*int{moron: &two, bosch: &one}); err != nil {
		t.Fatalf("swap = %v, want the numbers exchanged", err)
	}
	if got := read(); *got[moron] != 2 || *got[bosch] != 1 {
		t.Fatalf("after the swap Morón = %d, Villa Bosch = %d", *got[moron], *got[bosch])
	}

	_, err = invoices.UpdatePointsOfSale(ctx, tenant, map[uuid.UUID]*int{moron: &one, bosch: &one})
	if domain.CodeOf(err) != domain.CodePointOfSaleTaken {
		t.Fatalf("duplicate = %v, want POINT_OF_SALE_TAKEN", err)
	}
	_, err = invoices.UpdatePointsOfSale(ctx, tenant, map[uuid.UUID]*int{moron: &one})
	if domain.CodeOf(err) != domain.CodePointOfSaleTaken {
		t.Fatalf("number Villa Bosch keeps = %v, want POINT_OF_SALE_TAKEN", err)
	}
	if got := read(); *got[moron] != 2 || *got[bosch] != 1 {
		t.Fatal("a refused save changed a point of sale")
	}

	// A save carries only the rows it changed; a branch it leaves out keeps its number.
	if _, err := invoices.UpdatePointsOfSale(ctx, tenant, map[uuid.UUID]*int{bosch: nil}); err != nil {
		t.Fatal(err)
	}
	if got := read(); got[moron] == nil || *got[moron] != 2 || got[bosch] != nil {
		t.Fatalf("after clearing Villa Bosch alone: Morón = %v, Villa Bosch = %v", got[moron], got[bosch])
	}
}
