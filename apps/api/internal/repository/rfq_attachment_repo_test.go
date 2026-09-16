//go:build integration

package repository

import (
	"context"
	"errors"
	"testing"
	"time"

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

// Two attachments written inside one transaction share created_at to the microsecond, so the id
// tiebreak is the only thing deciding their order. With one row there is nothing to order and
// the clause could be deleted unnoticed.
func TestRFQAttachmentRepository_ListByRFQ_OrdersOldestFirstAndBreaksTiesById(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountID := seedAccount(t, db, "Attachment ordering")
	branchID := branchOf(t, db, accountID)
	rfqID := seedRFQFor(t, db, accountID, branchID)

	first, second := newAttachment(rfqID), newAttachment(rfqID)
	// Written smaller-id first or not, the clause decides: sort the pair so the expectation is
	// the id order rather than the insertion order.
	if second.ID.String() < first.ID.String() {
		first, second = second, first
	}

	tenant := domain.Tenant{AccountID: accountID, BranchID: branchID, Role: domain.UserRoleAdmin}
	repo := NewRFQAttachmentRepository()
	var listed []domain.RFQAttachment
	err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		// Inserted larger id first, so insertion order and the expected order disagree.
		if _, createErr := repo.Create(ctx, q, accountID, branchID, second); createErr != nil {
			return createErr
		}
		if _, createErr := repo.Create(ctx, q, accountID, branchID, first); createErr != nil {
			return createErr
		}
		var listErr error
		listed, listErr = repo.ListByRFQ(ctx, q, accountID, branchID, rfqID)
		return listErr
	})
	if err != nil {
		t.Fatalf("list = %v, want no error", err)
	}
	if len(listed) != 2 {
		t.Fatalf("listed %d attachments, want 2", len(listed))
	}
	if listed[0].ID != first.ID || listed[1].ID != second.ID {
		t.Fatalf("order = %v, %v; want %v, %v", listed[0].ID, listed[1].ID, first.ID, second.ID)
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

// insertAttachment writes one attachment directly, so a test can choose the status and the claim
// timestamp the sweep is supposed to react to.
func insertAttachment(
	t *testing.T, db *DB, accountID, rfqID uuid.UUID, status string, startedAt *time.Time,
) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if _, err := db.CrossAccount().Exec(context.Background(),
		`INSERT INTO rfq_attachment (id, account_id, rfq_id, type, file_url, processing_status,
		                             processing_started_at)
		 VALUES ($1, $2, $3, 'PDF', $4, $5::attachment_processing_status, $6)`,
		id, accountID, rfqID, "accounts/x/"+id.String()+".pdf", status, startedAt); err != nil {
		t.Fatalf("seed attachment: %v", err)
	}
	return id
}

func statusOf(t *testing.T, db *DB, attachmentID uuid.UUID) string {
	t.Helper()
	var status string
	if err := db.CrossAccount().QueryRow(context.Background(),
		`SELECT processing_status FROM rfq_attachment WHERE id = $1`,
		attachmentID).Scan(&status); err != nil {
		t.Fatalf("read status: %v", err)
	}
	return status
}

// The claim is the whole contract of the sweep: it must take a pending row, mark it PROCESSING in
// the same statement, and carry the branch its RFQ belongs to — the attachment row has none, and
// every service the sweep calls is branch-scoped.
func TestRFQAttachmentRepository_ClaimPendingByRFQ_TakesPendingRowsAndCarriesTheirBranch(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountID := seedAccount(t, db, "Attachment claim pending")
	branchID := branchOf(t, db, accountID)
	rfqID := seedRFQFor(t, db, accountID, branchID)
	pendingID := insertAttachment(t, db, accountID, rfqID, "PENDING", nil)
	doneID := insertAttachment(t, db, accountID, rfqID, "DONE", nil)

	now := time.Now()
	claimed, err := NewRFQAttachmentRepository().ClaimPendingByRFQ(ctx, db.CrossAccount(), 10,
		15*time.Minute, now)
	if err != nil {
		t.Fatalf("ClaimPendingByRFQ() = %v, want no error", err)
	}

	var got *domain.ClaimedAttachment
	for i := range claimed {
		if claimed[i].ID == pendingID {
			got = &claimed[i]
		}
		if claimed[i].ID == doneID {
			t.Errorf("ClaimPendingByRFQ() took a DONE attachment")
		}
	}
	if got == nil {
		t.Fatalf("ClaimPendingByRFQ() did not take the pending attachment")
	}
	if got.AccountID != accountID || got.BranchID != branchID || got.RFQID != rfqID {
		t.Errorf("claimed = account %v branch %v rfq %v, want %v / %v / %v",
			got.AccountID, got.BranchID, got.RFQID, accountID, branchID, rfqID)
	}
	if got.StorageKey == "" {
		t.Errorf("claimed storage key is empty, want the stored object key")
	}
	if status := statusOf(t, db, pendingID); status != "PROCESSING" {
		t.Errorf("claimed row status = %q, want PROCESSING", status)
	}
	if status := statusOf(t, db, doneID); status != "DONE" {
		t.Errorf("untouched row status = %q, want DONE", status)
	}
}

/*
 * A run killed between claiming a row and finishing it leaves that row PROCESSING. Without an
 * expiring claim the queue leaks exactly the way it leaks at PENDING, so a stale claim is taken
 * again and a fresh one is left alone. Both halves are asserted here: pinning only the reclaim
 * would pass on a query that ignores processing_started_at entirely and re-claims everything.
 */
func TestRFQAttachmentRepository_ClaimPendingByRFQ_ReclaimsAStaleClaimAndLeavesAFreshOne(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountID := seedAccount(t, db, "Attachment claim reclaim")
	branchID := branchOf(t, db, accountID)
	rfqID := seedRFQFor(t, db, accountID, branchID)

	now := time.Now()
	stale := now.Add(-30 * time.Minute)
	fresh := now.Add(-1 * time.Minute)
	staleID := insertAttachment(t, db, accountID, rfqID, "PROCESSING", &stale)
	freshID := insertAttachment(t, db, accountID, rfqID, "PROCESSING", &fresh)

	claimed, err := NewRFQAttachmentRepository().ClaimPendingByRFQ(ctx, db.CrossAccount(), 10,
		15*time.Minute, now)
	if err != nil {
		t.Fatalf("ClaimPendingByRFQ() = %v, want no error", err)
	}

	took := map[uuid.UUID]bool{}
	for _, a := range claimed {
		took[a.ID] = true
	}
	if !took[staleID] {
		t.Errorf("ClaimPendingByRFQ() left a claim older than the window, want it reclaimed")
	}
	if took[freshID] {
		t.Errorf("ClaimPendingByRFQ() took a claim inside the window, want it left to its worker")
	}
}

// The batch size is what keeps one firing bounded, so a queue longer than the limit is drained
// across runs rather than in one that outlives its own timeout. It counts ORDERS.
func TestRFQAttachmentRepository_ClaimPendingByRFQ_TakesNoMoreOrdersThanTheLimit(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountID := seedAccount(t, db, "Attachment claim limit")
	branchID := branchOf(t, db, accountID)
	for range 5 {
		insertAttachment(t, db, accountID, seedRFQFor(t, db, accountID, branchID), "PENDING", nil)
	}

	claimed, err := NewRFQAttachmentRepository().ClaimPendingByRFQ(ctx, db.CrossAccount(), 2,
		15*time.Minute, time.Now())
	if err != nil {
		t.Fatalf("ClaimPendingByRFQ() = %v, want no error", err)
	}
	orders := map[uuid.UUID]bool{}
	for _, a := range claimed {
		orders[a.RFQID] = true
	}
	if len(orders) != 2 {
		t.Fatalf("ClaimPendingByRFQ(limit 2) took %d orders, want 2", len(orders))
	}
}

/*
 * A whole order is claimed at once, however many files it is holding and whatever the limit is.
 * An order is extracted a single time over everything it arrived with, so a limit that cut across
 * one would send the model half a client's message and close the other half out as read.
 */
func TestRFQAttachmentRepository_ClaimPendingByRFQ_TakesEveryFileOfAnOrder(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountID := seedAccount(t, db, "Attachment claim whole order")
	branchID := branchOf(t, db, accountID)
	rfqID := seedRFQFor(t, db, accountID, branchID)
	for range 3 {
		insertAttachment(t, db, accountID, rfqID, "PENDING", nil)
	}

	claimed, err := NewRFQAttachmentRepository().ClaimPendingByRFQ(ctx, db.CrossAccount(), 1,
		15*time.Minute, time.Now())
	if err != nil {
		t.Fatalf("ClaimPendingByRFQ() = %v, want no error", err)
	}
	if len(claimed) != 3 {
		t.Fatalf("ClaimPendingByRFQ(limit 1 order) took %d files, want all 3 of the order",
			len(claimed))
	}
}

/*
 * rfq_attachment.rfq_id references rfq(id) alone, so nothing in the database stops a row naming
 * another account's order. The sweep runs as the owner, where row level security refuses nothing,
 * and it reads the branch off that RFQ — so a mismatched pair would carry a foreign branch into
 * everything the sweep then does. The claim refuses instead of defaulting.
 */
func TestRFQAttachmentRepository_ClaimPendingByRFQ_RefusesAnAttachmentNamingAnotherAccountsRFQ(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	victimAccount := seedAccount(t, db, "Attachment claim victim")
	intruderAccount := seedAccount(t, db, "Attachment claim intruder")
	victimRFQ := seedRFQFor(t, db, victimAccount, branchOf(t, db, victimAccount))

	// The attachment claims the intruder's account while pointing at the victim's order.
	insertAttachment(t, db, intruderAccount, victimRFQ, "PENDING", nil)

	_, err := NewRFQAttachmentRepository().ClaimPendingByRFQ(ctx, db.CrossAccount(),
		10, 15*time.Minute, time.Now())
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("ClaimPendingByRFQ() = %v, want ErrNotFound for a cross-account attachment", err)
	}
}

// insertReadAttachment seeds an attachment that has already been read, with what it yielded.
func insertReadAttachment(
	t *testing.T, db *DB, accountID, rfqID uuid.UUID, status, extracted string,
) uuid.UUID {
	t.Helper()
	id := uuid.New()
	var text *string
	if extracted != "" {
		text = &extracted
	}
	if _, err := db.CrossAccount().Exec(context.Background(),
		`INSERT INTO rfq_attachment (id, account_id, rfq_id, type, file_url, extracted_text,
		                             processing_status)
		 VALUES ($1, $2, $3, 'TEXT', $4, $5, $6::attachment_processing_status)`,
		id, accountID, rfqID, "accounts/x/"+id.String()+".txt", text, status); err != nil {
		t.Fatalf("seed read attachment: %v", err)
	}
	return id
}

/*
 * A later file is extracted alongside what the order's earlier ones yielded, so this returns only
 * the readings that exist: a row still PENDING has not been read, and a DONE row with no text is
 * an image or a PDF, whose content is the file itself rather than anything stored here. Returning
 * either would hand the model an empty block and count a file nothing has read.
 */
func TestRFQAttachmentRepository_ListReadByRFQIDs_ReturnsOnlyStoredReadings(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountID := seedAccount(t, db, "Attachment earlier readings")
	branchID := branchOf(t, db, accountID)
	rfqID := seedRFQFor(t, db, accountID, branchID)

	readID := insertReadAttachment(t, db, accountID, rfqID, "DONE", "20 bolsas de cemento")
	pendingID := insertReadAttachment(t, db, accountID, rfqID, "PENDING", "not read yet")
	noTextID := insertReadAttachment(t, db, accountID, rfqID, "DONE", "")

	byRFQ, err := NewRFQAttachmentRepository().ListReadByRFQIDs(ctx, db.CrossAccount(),
		[]uuid.UUID{rfqID}, []uuid.UUID{accountID})
	if err != nil {
		t.Fatalf("ListReadByRFQIDs() = %v, want no error", err)
	}

	got := map[uuid.UUID]bool{}
	for _, a := range byRFQ[rfqID] {
		got[a.ID] = true
	}
	if !got[readID] {
		t.Errorf("ListReadByRFQIDs() dropped the attachment that was read")
	}
	if got[pendingID] {
		t.Errorf("ListReadByRFQIDs() returned a PENDING attachment, want only what was read")
	}
	if got[noTextID] {
		t.Errorf("ListReadByRFQIDs() returned a DONE attachment with no text")
	}
}

/*
 * The orders are matched by the (rfq_id, account_id) pair. rfq_attachment.rfq_id references
 * rfq(id) alone and the sweep runs as the owner, so matching on the order alone would let one
 * account's extraction be fed another account's material.
 */
func TestRFQAttachmentRepository_ListReadByRFQIDs_RefusesAnotherAccountsMaterial(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	victimAccount := seedAccount(t, db, "Earlier readings victim")
	intruderAccount := seedAccount(t, db, "Earlier readings intruder")
	victimRFQ := seedRFQFor(t, db, victimAccount, branchOf(t, db, victimAccount))
	insertReadAttachment(t, db, victimAccount, victimRFQ, "DONE", "cemento del vecino")

	// The intruder asks for the victim's order under its own account.
	byRFQ, err := NewRFQAttachmentRepository().ListReadByRFQIDs(ctx, db.CrossAccount(),
		[]uuid.UUID{victimRFQ}, []uuid.UUID{intruderAccount})
	if err != nil {
		t.Fatalf("ListReadByRFQIDs() = %v, want no error", err)
	}
	if len(byRFQ) != 0 {
		t.Errorf("ListReadByRFQIDs() returned %d orders, want none for a mismatched pair",
			len(byRFQ))
	}
}
