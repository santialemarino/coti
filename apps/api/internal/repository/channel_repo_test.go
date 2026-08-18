//go:build integration

package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func seedChannel(
	t *testing.T, db *DB, accountID, branchID uuid.UUID, channelType domain.ChannelType,
	isActive bool,
) uuid.UUID {
	t.Helper()
	id := uuid.New()
	identifier := uuid.NewString()
	if _, err := db.CrossAccount().Exec(context.Background(),
		`INSERT INTO channel (id, account_id, branch_id, type, identifier, is_active)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		id, accountID, branchID, channelType, identifier, isActive); err != nil {
		t.Fatalf("seed channel: %v", err)
	}
	t.Cleanup(func() {
		mustCleanup(t, db.CrossAccount(), `DELETE FROM channel WHERE id = $1`, id)
	})
	return id
}

func TestChannelRepository_ActiveReadsStayInsideAccountAndBranch(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountA := seedAccount(t, db, "Channel account A")
	accountB := seedAccount(t, db, "Channel account B")
	branchA := branchOf(t, db, accountA)
	branchB := branchOf(t, db, accountB)
	branchASecond := seedExtraBranch(t, db, accountA, "Channel account A second")
	activeA := seedChannel(t, db, accountA, branchA, domain.ChannelTypeWhatsApp, true)
	inactiveA := seedChannel(t, db, accountA, branchA, domain.ChannelTypeEmail, false)
	activeASecond := seedChannel(t, db, accountA, branchASecond, domain.ChannelTypeWhatsApp, true)
	activeB := seedChannel(t, db, accountB, branchB, domain.ChannelTypeWhatsApp, true)
	repo := NewChannelRepository()

	var channels []domain.Channel
	if err := db.InTenantTx(ctx, domain.Tenant{AccountID: accountA, BranchID: branchA},
		func(q Querier) error {
			var listErr error
			channels, listErr = repo.ListActiveByBranch(ctx, q, accountA, branchA)
			return listErr
		}); err != nil {
		t.Fatalf("ListActiveByBranch() = %v, want no error", err)
	}
	if len(channels) != 1 || channels[0].ID != activeA {
		t.Fatalf("ListActiveByBranch() = %#v, want only active channel %v", channels, activeA)
	}

	for _, test := range []struct {
		name      string
		accountID uuid.UUID
		branchID  uuid.UUID
		channelID uuid.UUID
	}{
		{name: "inactive", accountID: accountA, branchID: branchA, channelID: inactiveA},
		{name: "other account", accountID: accountA, branchID: branchA, channelID: activeB},
		{name: "other branch", accountID: accountA, branchID: branchA, channelID: activeASecond},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := db.InTenantTx(ctx, domain.Tenant{AccountID: accountA, BranchID: branchA},
				func(q Querier) error {
					_, getErr := repo.GetActiveByID(ctx, q, test.accountID, test.branchID,
						test.channelID)
					return getErr
				})
			if !errors.Is(err, domain.ErrNotFound) {
				t.Errorf("GetActiveByID() = %v, want %v", err, domain.ErrNotFound)
			}
		})
	}
}

func TestChannelRepository_ListActiveByTypeFiltersTheSelectedBranch(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountID := seedAccount(t, db, "Typed channels")
	branchID := branchOf(t, db, accountID)
	whatsAppID := seedChannel(t, db, accountID, branchID, domain.ChannelTypeWhatsApp, true)
	seedChannel(t, db, accountID, branchID, domain.ChannelTypeEmail, true)
	repo := NewChannelRepository()

	var channels []domain.Channel
	err := db.InTenantTx(ctx, domain.Tenant{AccountID: accountID, BranchID: branchID},
		func(q Querier) error {
			var listErr error
			channels, listErr = repo.ListActiveByType(ctx, q, accountID, branchID,
				domain.ChannelTypeWhatsApp)
			return listErr
		})
	if err != nil {
		t.Fatalf("ListActiveByType() = %v, want no error", err)
	}
	if len(channels) != 1 || channels[0].ID != whatsAppID {
		t.Fatalf("ListActiveByType() = %#v, want WhatsApp channel %v", channels, whatsAppID)
	}
}
