//go:build integration

package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// seedRFQFor writes one order to hang attachments off, and takes it away afterwards.
func seedRFQFor(t *testing.T, db *DB, accountID, branchID uuid.UUID) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	channelID, rfqID := uuid.New(), uuid.New()

	if _, err := db.CrossAccount().Exec(ctx,
		`INSERT INTO channel (id, account_id, branch_id, type, identifier)
		 VALUES ($1, $2, $3, 'WHATSAPP', $4)`,
		channelID, accountID, branchID, uuid.NewString()); err != nil {
		t.Fatalf("seed channel: %v", err)
	}
	if _, err := db.CrossAccount().Exec(ctx,
		`INSERT INTO rfq (id, account_id, branch_id, channel_id, status)
		 VALUES ($1, $2, $3, $4, 'RECEIVED')`,
		rfqID, accountID, branchID, channelID); err != nil {
		t.Fatalf("seed rfq: %v", err)
	}
	t.Cleanup(func() {
		mustCleanup(t, db.CrossAccount(), `DELETE FROM rfq_attachment WHERE rfq_id = $1`, rfqID)
		mustCleanup(t, db.CrossAccount(), `DELETE FROM rfq WHERE id = $1`, rfqID)
		mustCleanup(t, db.CrossAccount(), `DELETE FROM channel WHERE id = $1`, channelID)
	})
	return rfqID
}

func newAttachment(rfqID uuid.UUID) domain.NewRFQAttachment {
	id := uuid.New()
	return domain.NewRFQAttachment{
		ID:         id,
		RFQID:      rfqID,
		Type:       domain.AttachmentTypePDF,
		StorageKey: "accounts/x/rfqs/" + rfqID.String() + "/" + id.String() + ".pdf",
	}
}

// Row level security refuses another account's rfq before the application predicate is reached,
// so inside InTenantTx the two are indistinguishable. On the owner pool, which is RLS-exempt,
// the predicate is the only thing left to refuse — and that is what this pins.
func TestRFQAttachmentRepository_Create_RefusesAnotherAccountWithoutRowLevelSecurity(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	victimAccount := seedAccount(t, db, "Attachment victim no RLS")
	intruderAccount := seedAccount(t, db, "Attachment intruder no RLS")
	victimBranch := branchOf(t, db, victimAccount)
	victimRFQ := seedRFQFor(t, db, victimAccount, victimBranch)

	tx, err := db.AdminTx(ctx)
	if err != nil {
		t.Fatalf("AdminTx() = %v, want no error", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	_, err = NewRFQAttachmentRepository().Create(ctx, tx, intruderAccount, victimBranch,
		newAttachment(victimRFQ))
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Create() on the owner pool = %v, want ErrNotFound", err)
	}
}

// The branch is the boundary the database does not guard at all, so this one is refused by the
// application predicate whichever pool it runs on.
func TestRFQAttachmentRepository_Create_RefusesAnotherBranchOfTheSameAccount(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountID := seedAccount(t, db, "Attachment other branch")
	branchID := branchOf(t, db, accountID)
	otherBranch := uuid.New()
	if _, err := db.CrossAccount().Exec(ctx,
		`INSERT INTO branch (id, account_id, name) VALUES ($1, $2, 'Sucursal Norte')`,
		otherBranch, accountID); err != nil {
		t.Fatalf("seed branch: %v", err)
	}
	rfqID := seedRFQFor(t, db, accountID, branchID)

	tenant := domain.Tenant{AccountID: accountID, BranchID: otherBranch, Role: domain.UserRoleAdmin}
	err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		_, createErr := NewRFQAttachmentRepository().Create(ctx, q, accountID, otherBranch,
			newAttachment(rfqID))
		return createErr
	})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Create() into another branch's rfq = %v, want ErrNotFound", err)
	}
}

// The happy path, so the refusals above are known to be refusing something that otherwise works.
func TestRFQAttachmentRepository_CreateThenList_RoundTripsOneAttachment(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountID := seedAccount(t, db, "Attachment round trip")
	branchID := branchOf(t, db, accountID)
	rfqID := seedRFQFor(t, db, accountID, branchID)
	in := newAttachment(rfqID)

	tenant := domain.Tenant{AccountID: accountID, BranchID: branchID, Role: domain.UserRoleAdmin}
	repo := NewRFQAttachmentRepository()
	var listed []domain.RFQAttachment
	err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		created, createErr := repo.Create(ctx, q, accountID, branchID, in)
		if createErr != nil {
			return createErr
		}
		if created.ProcessingStatus != domain.AttachmentProcessingPending {
			t.Errorf("processing_status = %q, want PENDING", created.ProcessingStatus)
		}
		if created.StorageKey == nil || *created.StorageKey != in.StorageKey {
			t.Errorf("storage key = %v, want %q", created.StorageKey, in.StorageKey)
		}
		var listErr error
		listed, listErr = repo.ListByRFQ(ctx, q, accountID, branchID, rfqID)
		return listErr
	})
	if err != nil {
		t.Fatalf("round trip = %v, want no error", err)
	}
	if len(listed) != 1 || listed[0].ID != in.ID {
		t.Fatalf("listed = %#v, want the one created attachment", listed)
	}
}
