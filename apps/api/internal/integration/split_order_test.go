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

func (e *env) splitOrderOf(t *testing.T, account, branch, rfqID uuid.UUID) *domain.SplitOrder {
	t.Helper()
	var split *domain.SplitOrder
	repo := repository.NewInboundChannelMessageRepository()
	if err := e.db.InTenantTx(context.Background(), domain.Tenant{AccountID: account, BranchID: branch},
		func(q repository.Querier) error {
			var err error
			split, err = repo.FindSplitOrder(context.Background(), q, account, branch, rfqID, time.Hour)
			return err
		}); err != nil {
		t.Fatal(err)
	}
	return split
}

func (e *env) orderOf(t *testing.T, account uuid.UUID, body string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT rfq_id FROM inbound_channel_message WHERE account_id=$1 AND body=$2`, account, body).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestSplitOrder_TwoOrdersFromOneSenderShowEachOther(t *testing.T) {
	e := newEnv(t)
	box := e.nameReplyInbox(t, "Split order", false)
	ctx := context.Background()
	sender, stranger := "+5491155550401", "+5491155550402"

	box.send(t, sender, "20 bolsas de cemento")
	box.send(t, sender, "agregá 10 ladrillos")
	box.send(t, stranger, "5 bolsas de cal")
	first, second := e.orderOf(t, box.account, "20 bolsas de cemento"), e.orderOf(t, box.account, "agregá 10 ladrillos")
	alone := e.orderOf(t, box.account, "5 bolsas de cal")

	if split := e.splitOrderOf(t, box.account, box.branch, second); split == nil || split.RFQID != first {
		t.Fatalf("second order's split = %+v, want the first", split)
	}
	if split := e.splitOrderOf(t, box.account, box.branch, first); split == nil || split.RFQID != second {
		t.Fatalf("first order's split = %+v, want the second", split)
	}
	if split := e.splitOrderOf(t, box.account, box.branch, alone); split != nil {
		t.Fatalf("another sender's order shows %+v, want nothing", split)
	}

	// An order put away, or one outside the window, is not a split.
	if _, err := e.db.CrossAccount().Exec(ctx, `UPDATE rfq SET archived_at=now() WHERE id=$1`, first); err != nil {
		t.Fatal(err)
	}
	if split := e.splitOrderOf(t, box.account, box.branch, second); split != nil {
		t.Fatalf("an archived order still shows: %+v", split)
	}
	if _, err := e.db.CrossAccount().Exec(ctx,
		`UPDATE rfq SET archived_at=NULL, created_at=created_at-interval '2 hours' WHERE id=$1`, first); err != nil {
		t.Fatal(err)
	}
	if split := e.splitOrderOf(t, box.account, box.branch, second); split != nil {
		t.Fatalf("an order two hours earlier still shows: %+v", split)
	}
	other := e.seedBranch(t, box.account, "Otra")
	if split := e.splitOrderOf(t, box.account, other, first); split != nil {
		t.Fatalf("another branch sees %+v, want nothing", split)
	}
}
