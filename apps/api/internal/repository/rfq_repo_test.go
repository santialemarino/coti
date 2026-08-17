//go:build integration

package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func TestRFQRepository_CreateClarificationsStaysInsideTheRFQAccount(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountA := seedAccount(t, db, "RFQ clarification account A")
	accountB := seedAccount(t, db, "RFQ clarification account B")
	branchA := branchOf(t, db, accountA)
	channelA := seedChannel(t, db, accountA, branchA, domain.ChannelTypeWhatsApp, true)
	repo := NewRFQRepository()
	tenantA := domain.Tenant{AccountID: accountA, BranchID: branchA}

	var rfq *domain.RFQ
	if err := db.InTenantTx(ctx, tenantA, func(q Querier) error {
		var createErr error
		raw := "Necesito cemento"
		rfq, createErr = repo.Create(ctx, q, accountA, domain.NewRFQ{
			BranchID: branchA, ChannelID: channelA, RawText: &raw,
		})
		return createErr
	}); err != nil {
		t.Fatalf("Create() = %v, want no error", err)
	}
	t.Cleanup(func() {
		mustCleanup(t, db.CrossAccount(), `DELETE FROM rfq_clarification WHERE rfq_id = $1`, rfq.ID)
		mustCleanup(t, db.CrossAccount(), `DELETE FROM rfq WHERE id = $1`, rfq.ID)
	})

	proposal := domain.NewRFQClarification{
		IssueType:            domain.RFQClarificationMissingQuantity,
		RequestedDescription: "cemento",
		Question:             "Cuantas bolsas de cemento necesitas?",
		Reason:               "La cantidad no aparece en el pedido.",
	}
	var created []domain.RFQClarification
	if err := db.InTenantTx(ctx, tenantA, func(q Querier) error {
		var createErr error
		created, createErr = repo.CreateClarifications(ctx, q, accountA, rfq.ID,
			[]domain.NewRFQClarification{proposal})
		return createErr
	}); err != nil {
		t.Fatalf("CreateClarifications() = %v, want no error", err)
	}
	if len(created) != 1 || created[0].RFQID != rfq.ID ||
		created[0].Status != domain.RFQClarificationStatusProposed {
		t.Fatalf("CreateClarifications() = %#v, want one PROPOSED row", created)
	}

	err := db.InTenantTx(ctx, domain.Tenant{AccountID: accountB}, func(q Querier) error {
		_, createErr := repo.CreateClarifications(ctx, q, accountB, rfq.ID,
			[]domain.NewRFQClarification{proposal})
		return createErr
	})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("CreateClarifications() from another account = %v, want %v", err,
			domain.ErrNotFound)
	}

	var foreignRows int
	if err := db.CrossAccount().QueryRow(ctx,
		`SELECT count(*) FROM rfq_clarification WHERE rfq_id = $1 AND account_id = $2`,
		rfq.ID, accountB).Scan(&foreignRows); err != nil {
		t.Fatalf("count foreign clarifications: %v", err)
	}
	if foreignRows != 0 {
		t.Errorf("foreign clarification rows = %d, want 0", foreignRows)
	}
}
