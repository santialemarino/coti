//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func (e *env) seedKnownDeliveryClient(t *testing.T, tenant domain.Tenant, quoteID, rfqID uuid.UUID) {
	t.Helper()
	clientID := uuid.New()
	if _, err := e.db.CrossAccount().Exec(context.Background(), `INSERT INTO client(id,account_id,name,tax_id) VALUES($1,$2,'Known customer','20123456786')`, clientID, tenant.AccountID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.CrossAccount().Exec(context.Background(), `UPDATE quote SET client_id=$2 WHERE id=$1`, quoteID, clientID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.CrossAccount().Exec(context.Background(), `UPDATE rfq SET client_id=$2 WHERE id=$1`, rfqID, clientID); err != nil {
		t.Fatal(err)
	}
}
