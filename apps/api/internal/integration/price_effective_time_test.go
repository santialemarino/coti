//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

/*
 * A price set without a start is in force for the very next read. The start comes from the
 * database clock the reads compare against; stamped with the API's clock, a few milliseconds
 * ahead, the next read missed it in a large share of attempts, so this repeats the attempt.
 */
func TestPriceEffectiveTime_NewProductPriceIsInForceAtOnce(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	accountID, branchID := e.seedAccount(t, "Price effective time")
	admin := e.seedUser(t, accountID, domain.UserRoleAdmin)
	token := e.tokenFor(t, admin)
	var familyID uuid.UUID
	if err := e.db.CrossAccount().QueryRow(ctx, `SELECT id FROM product_family ORDER BY sort_order LIMIT 1`).Scan(&familyID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		e.mustCleanup(t, `DELETE FROM product_price WHERE account_id = $1`, accountID)
		e.mustCleanup(t, `DELETE FROM branch_product WHERE account_id = $1`, accountID)
		e.mustCleanup(t, `DELETE FROM product WHERE account_id = $1`, accountID)
	})
	const attempts = 40
	for i := range attempts {
		rec := e.do(t, request{method: http.MethodPost, path: "/v1/products", token: token,
			branch: branchID.String(), body: map[string]any{
				"canonical_name": "Producto " + uuid.NewString(), "family_id": familyID, "price": "100.00",
			}})
		if rec.Code != http.StatusCreated {
			t.Fatalf("create %d = %d %s", i, rec.Code, rec.Body)
		}
		var product dto.ProductResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &product); err != nil {
			t.Fatal(err)
		}
		var inForce bool
		if err := e.db.CrossAccount().QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM product_price
			WHERE account_id = $1 AND branch_id = $2 AND product_id = $3
			  AND valid_from <= now() AND (valid_to IS NULL OR valid_to > now()))`,
			accountID, branchID, product.ID).Scan(&inForce); err != nil {
			t.Fatal(err)
		}
		if !inForce {
			t.Fatalf("attempt %d: the new product's price is not in force on the next read", i)
		}
	}
}
