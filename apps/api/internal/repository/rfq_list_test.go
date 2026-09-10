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

// listingSpec describes one inbox row: an RFQ that may hang a quote off, in whatever state and
// with whatever flags the test needs. A quote is present exactly when quoteStatus is set.
type listingSpec struct {
	rfqID         uuid.UUID
	clientID      *uuid.UUID
	clientLabel   *string
	quoteStatus   *domain.QuoteStatus
	sellerID      *uuid.UUID
	needsFollowup bool
	archived      bool
	createdAt     time.Time
}

// seedListingRFQ writes an RFQ (and, when requested, a quote) and takes the whole chain away.
func seedListingRFQ(
	t *testing.T, db *DB, accountID, branchID, channelID uuid.UUID, spec listingSpec,
) {
	t.Helper()
	ctx := context.Background()

	if _, err := db.CrossAccount().Exec(ctx,
		`INSERT INTO rfq (id, account_id, branch_id, channel_id, client_id, client_label,
		                  status, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, 'GENERATED', $7)`,
		spec.rfqID, accountID, branchID, channelID, spec.clientID, spec.clientLabel,
		spec.createdAt); err != nil {
		t.Fatalf("seed listing rfq: %v", err)
	}
	if spec.quoteStatus != nil {
		quoteID := uuid.New()
		var archivedAt *time.Time
		if spec.archived {
			now := time.Now()
			archivedAt = &now
		}
		if _, err := db.CrossAccount().Exec(ctx,
			`INSERT INTO quote (id, account_id, number, branch_id, rfq_id, seller_id, current_status,
			                    needs_followup, archived_at)
			 VALUES ($1, $2, (('x'||substr(replace($1::uuid::text,'-',''),1,15))::bit(60)::bigint), $3, $4, $5, $6, $7, $8)`,
			quoteID, accountID, branchID, spec.rfqID, spec.sellerID, *spec.quoteStatus,
			spec.needsFollowup, archivedAt); err != nil {
			t.Fatalf("seed listing quote: %v", err)
		}
		t.Cleanup(func() {
			mustCleanup(t, db.CrossAccount(), `DELETE FROM quote WHERE id = $1`, quoteID)
		})
	}
	t.Cleanup(func() {
		mustCleanup(t, db.CrossAccount(), `DELETE FROM quote WHERE rfq_id = $1`, spec.rfqID)
		mustCleanup(t, db.CrossAccount(), `DELETE FROM rfq WHERE id = $1`, spec.rfqID)
	})
}

// listByTenant runs the inbox query inside a tenant transaction and hands back the rows.
func listByTenant(
	t *testing.T, db *DB, accountID, branchID uuid.UUID,
) []domain.RfqListItem {
	t.Helper()
	return listByTenantFor(t, db, domain.Tenant{
		AccountID: accountID, BranchID: branchID, Role: domain.UserRoleAdmin,
	})
}

// listByTenantFor runs the inbox query for an arbitrary tenant, exercising the seller
// narrowing alongside the account row level security.
func listByTenantFor(t *testing.T, db *DB, tenant domain.Tenant) []domain.RfqListItem {
	t.Helper()
	ctx := context.Background()
	repo := NewRFQRepository()
	var items []domain.RfqListItem
	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		var err error
		items, err = repo.ListByTenant(ctx, q, tenant)
		return err
	}); err != nil {
		t.Fatalf("ListByTenant() = %v, want no error", err)
	}
	return items
}

// The follow-up quote tops the list, an archived quote drops out, and an RFQ without a quote
// still shows, reading its own status where a quote would otherwise be read.
func TestRFQRepository_ListByTenant_FollowupFirstArchivedOut(t *testing.T) {
	db := testDB(t)
	accountID := seedAccount(t, db, "List inbox")
	branchID := branchOf(t, db, accountID)
	channelID := seedChannel(t, db, accountID, branchID, domain.ChannelTypeWhatsApp, true)
	clientID := seedClient(t, db, accountID)

	now := time.Now()
	draft := domain.QuoteStatusDraft
	sent := domain.QuoteStatusSent
	accepted := domain.QuoteStatusAccepted

	draftID := uuid.New()
	followupID := uuid.New()
	archivedID := uuid.New()
	noQuoteID := uuid.New()
	// Walked oldest-first but listed newest-first, with the follow-up pinned above everything.
	seedListingRFQ(t, db, accountID, branchID, channelID, listingSpec{
		rfqID: draftID, clientID: &clientID, quoteStatus: &draft, createdAt: now.Add(-4 * time.Hour),
	})
	seedListingRFQ(t, db, accountID, branchID, channelID, listingSpec{
		rfqID: followupID, clientID: &clientID, quoteStatus: &sent,
		needsFollowup: true, createdAt: now.Add(-3 * time.Hour),
	})
	seedListingRFQ(t, db, accountID, branchID, channelID, listingSpec{
		rfqID: archivedID, clientID: &clientID, quoteStatus: &accepted,
		archived: true, createdAt: now.Add(-2 * time.Hour),
	})
	seedListingRFQ(t, db, accountID, branchID, channelID, listingSpec{
		rfqID: noQuoteID, clientID: &clientID, createdAt: now.Add(-1 * time.Hour),
	})

	items := listByTenant(t, db, accountID, branchID)

	if len(items) != 3 {
		t.Fatalf("listed %d rows, want 3: the archived quote must drop out", len(items))
	}
	// Newest first, but the follow-up outranks recency.
	if items[0].ID != followupID {
		t.Errorf("first row = %v, want the follow-up %v", items[0].ID, followupID)
	}
	if !items[0].NeedsFollowup {
		t.Error("follow-up row is not flagged needs_followup")
	}
	if items[1].ID != noQuoteID || items[2].ID != draftID {
		t.Errorf("rest of order = %v, %v; want newest then oldest",
			items[1].ID, items[2].ID)
	}
	for _, id := range []uuid.UUID{followupID, noQuoteID, draftID} {
		if !containsID(t, items, id) {
			t.Errorf("row %v missing from the list", id)
		}
	}
	if containsID(t, items, archivedID) {
		t.Error("archived quote is in the list, want it excluded")
	}
}

// The display name is the client ficha's name when a client is linked; only a counter order with
// no ficha falls back on the label the seller typed.
func TestRFQRepository_ListByTenant_CoalescesClientNameFromTheFicha(t *testing.T) {
	db := testDB(t)
	accountID := seedAccount(t, db, "List client name")
	branchID := branchOf(t, db, accountID)
	channelID := seedChannel(t, db, accountID, branchID, domain.ChannelTypeManualEntry, true)
	clientID := seedClient(t, db, accountID)

	now := time.Now()
	quoted := domain.QuoteStatusQuoted
	fichaName := "Juan Ficha"
	if _, err := db.CrossAccount().Exec(context.Background(),
		`UPDATE client SET name = $2 WHERE id = $1`, clientID, fichaName); err != nil {
		t.Fatalf("name the ficha: %v", err)
	}

	withFicha := uuid.New()
	seedListingRFQ(t, db, accountID, branchID, channelID, listingSpec{
		rfqID: withFicha, clientID: &clientID, quoteStatus: &quoted,
		createdAt: now.Add(-time.Hour),
	})
	labelOnlyID := uuid.New()
	// A counter order without a ficha: client NULL, label set.
	seedListingRFQ(t, db, accountID, branchID, channelID, listingSpec{
		rfqID: labelOnlyID, clientLabel: strPtr("Sr. Almada (mostrador)"),
		quoteStatus: &quoted, createdAt: now.Add(-2 * time.Hour),
	})

	items := listByTenant(t, db, accountID, branchID)

	for _, item := range items {
		if item.ID == withFicha {
			if item.ClientLabel == nil || *item.ClientLabel != fichaName {
				t.Errorf("linked client display = %v, want the ficha name %q",
					item.ClientLabel, fichaName)
			}
		}
		if item.ID == labelOnlyID {
			if item.ClientLabel == nil || *item.ClientLabel != "Sr. Almada (mostrador)" {
				t.Errorf("no-ficha display = %v, want the typed label", item.ClientLabel)
			}
		}
	}
}

// A non-zero branch narrows the inbox to that branch: another branch of the same account's work
// must not leak in, because row level security only guards the account.
func TestRFQRepository_ListByTenant_NarrowsToTheBranch(t *testing.T) {
	db := testDB(t)
	accountID := seedAccount(t, db, "List branch scope")
	branchID := branchOf(t, db, accountID)
	otherBranchID := seedExtraBranch(t, db, accountID, "Sucursal Norte")
	channelID := seedChannel(t, db, accountID, branchID, domain.ChannelTypeManualEntry, true)
	otherChannelID := seedChannel(t, db, accountID, otherBranchID, domain.ChannelTypeManualEntry, true)

	now := time.Now()
	quoted := domain.QuoteStatusQuoted
	ownID := uuid.New()
	otherID := uuid.New()
	seedListingRFQ(t, db, accountID, branchID, channelID, listingSpec{
		rfqID: ownID, quoteStatus: &quoted, createdAt: now.Add(-time.Hour),
	})
	seedListingRFQ(t, db, accountID, otherBranchID, otherChannelID, listingSpec{
		rfqID: otherID, quoteStatus: &quoted, createdAt: now.Add(-2 * time.Hour),
	})

	items := listByTenant(t, db, accountID, branchID)
	if len(items) != 1 || items[0].ID != ownID {
		t.Fatalf("listed %v, want only the selected branch's %v", items, ownID)
	}
}

// A seller on an active branch sees only the orders that branch hosts that are unassigned or
// their own. A peer's work stays hidden, and so does every other branch's inbox.
func TestRFQRepository_ListByTenant_SellerSeesOwnAndUnassignedInTheirBranch(t *testing.T) {
	db := testDB(t)
	accountID := seedAccount(t, db, "Seller list scope")
	branchID := branchOf(t, db, accountID)
	otherBranchID := seedExtraBranch(t, db, accountID, "Sucursal Lejos")
	channelID := seedChannel(t, db, accountID, branchID, domain.ChannelTypeManualEntry, true)
	otherChannelID := seedChannel(t, db, accountID, otherBranchID, domain.ChannelTypeManualEntry, true)
	seller := seedUser(t, db, accountID, "SELLER")
	peer := seedUser(t, db, accountID, "SELLER")

	now := time.Now()
	draft := domain.QuoteStatusDraft
	unassignedID := uuid.New()
	ownID := uuid.New()
	peerID := uuid.New()
	otherBranchID_ := uuid.New()
	seedListingRFQ(t, db, accountID, branchID, channelID, listingSpec{
		rfqID: unassignedID, quoteStatus: &draft, createdAt: now.Add(-time.Hour),
	})
	seedListingRFQ(t, db, accountID, branchID, channelID, listingSpec{
		rfqID: ownID, sellerID: &seller, quoteStatus: &draft, createdAt: now.Add(-2 * time.Hour),
	})
	seedListingRFQ(t, db, accountID, branchID, channelID, listingSpec{
		rfqID: peerID, sellerID: &peer, quoteStatus: &draft, createdAt: now.Add(-3 * time.Hour),
	})
	seedListingRFQ(t, db, accountID, otherBranchID, otherChannelID, listingSpec{
		rfqID: otherBranchID_, sellerID: &seller, quoteStatus: &draft,
		createdAt: now.Add(-4 * time.Hour),
	})

	items := listByTenantFor(t, db, domain.Tenant{
		AccountID: accountID, BranchID: branchID, UserID: seller, Role: domain.UserRoleSeller,
	})

	if len(items) != 2 {
		t.Fatalf("seller listed %d rows, want 2 (their own and the unassigned)", len(items))
	}
	for _, id := range []uuid.UUID{unassignedID, ownID} {
		if !containsID(t, items, id) {
			t.Errorf("row %v missing from the seller's list", id)
		}
	}
	for _, id := range []uuid.UUID{peerID, otherBranchID_} {
		if containsID(t, items, id) {
			t.Errorf("row %v leaked into the seller's list", id)
		}
	}
}

// A seller who picked no active branch is confined to the branches they are assigned, exactly
// like the branch switcher reads them: a single assignment behaves as their branch.
func TestRFQRepository_ListByTenant_SellerWithoutActiveBranchUsesAssignments(t *testing.T) {
	db := testDB(t)
	accountID := seedAccount(t, db, "Seller assignment scope")
	branchID := branchOf(t, db, accountID)
	otherBranchID := seedExtraBranch(t, db, accountID, "Sucursal Norte")
	channelID := seedChannel(t, db, accountID, branchID, domain.ChannelTypeManualEntry, true)
	otherChannelID := seedChannel(t, db, accountID, otherBranchID, domain.ChannelTypeManualEntry, true)
	seller := seedUser(t, db, accountID, "SELLER")

	now := time.Now()
	draft := domain.QuoteStatusDraft
	ownUnassigned := uuid.New()
	otherBranchOrder := uuid.New()
	seedListingRFQ(t, db, accountID, branchID, channelID, listingSpec{
		rfqID: ownUnassigned, quoteStatus: &draft, createdAt: now.Add(-time.Hour),
	})
	seedListingRFQ(t, db, accountID, otherBranchID, otherChannelID, listingSpec{
		rfqID: otherBranchOrder, quoteStatus: &draft, createdAt: now.Add(-2 * time.Hour),
	})

	items := listByTenantFor(t, db, domain.Tenant{
		AccountID: accountID, UserID: seller, Role: domain.UserRoleSeller,
		AllowedBranchIDs: []uuid.UUID{branchID},
	})

	if len(items) != 1 || items[0].ID != ownUnassigned {
		t.Fatalf("seller without an active branch listed %v, want only the assigned branch's %v",
			items, ownUnassigned)
	}
}

// A seller with no assignments and no active branch reads nothing at all: the branch scope
// fails closed instead of widening to the account.
func TestRFQRepository_ListByTenant_SellerWithoutAssignmentsReadsNothing(t *testing.T) {
	db := testDB(t)
	accountID := seedAccount(t, db, "Seller empty scope")
	branchID := branchOf(t, db, accountID)
	channelID := seedChannel(t, db, accountID, branchID, domain.ChannelTypeManualEntry, true)
	seller := seedUser(t, db, accountID, "SELLER")

	draft := domain.QuoteStatusDraft
	seedListingRFQ(t, db, accountID, branchID, channelID, listingSpec{
		rfqID: uuid.New(), quoteStatus: &draft, createdAt: time.Now(),
	})

	items := listByTenantFor(t, db, domain.Tenant{
		AccountID: accountID, UserID: seller, Role: domain.UserRoleSeller,
	})
	if len(items) != 0 {
		t.Fatalf("seller with no assignments listed %d rows, want none", len(items))
	}
}

// getByRFQID runs the detail query for a tenant and returns the row or the exact error.
func getByRFQID(t *testing.T, db *DB, tenant domain.Tenant, rfqID uuid.UUID) (*domain.RfqListItem, error) {
	t.Helper()
	repo := NewRFQRepository()
	var out *domain.RfqListItem
	err := db.InTenantTx(context.Background(), tenant, func(q Querier) error {
		var err error
		out, err = repo.GetByRFQID(context.Background(), q, tenant, rfqID)
		return err
	})
	return out, err
}

// Detail follows the same visibility rule as the list: a seller reads their own or an
// unassigned order in their branch, and a hidden one answers not found rather than 403.
func TestRFQRepository_GetByRFQID_SellerVisibility(t *testing.T) {
	db := testDB(t)
	accountID := seedAccount(t, db, "Seller detail scope")
	branchID := branchOf(t, db, accountID)
	otherBranchID := seedExtraBranch(t, db, accountID, "Sucursal Lejos")
	channelID := seedChannel(t, db, accountID, branchID, domain.ChannelTypeManualEntry, true)
	otherChannelID := seedChannel(t, db, accountID, otherBranchID, domain.ChannelTypeManualEntry, true)
	seller := seedUser(t, db, accountID, "SELLER")
	peer := seedUser(t, db, accountID, "SELLER")

	now := time.Now()
	draft := domain.QuoteStatusDraft
	unassignedID := uuid.New()
	ownID := uuid.New()
	peerID := uuid.New()
	otherBranchOrder := uuid.New()
	seedListingRFQ(t, db, accountID, branchID, channelID, listingSpec{
		rfqID: unassignedID, quoteStatus: &draft, createdAt: now.Add(-time.Hour),
	})
	seedListingRFQ(t, db, accountID, branchID, channelID, listingSpec{
		rfqID: ownID, sellerID: &seller, quoteStatus: &draft, createdAt: now.Add(-2 * time.Hour),
	})
	seedListingRFQ(t, db, accountID, branchID, channelID, listingSpec{
		rfqID: peerID, sellerID: &peer, quoteStatus: &draft, createdAt: now.Add(-3 * time.Hour),
	})
	seedListingRFQ(t, db, accountID, otherBranchID, otherChannelID, listingSpec{
		rfqID: otherBranchOrder, quoteStatus: &draft, createdAt: now.Add(-4 * time.Hour),
	})

	tenant := domain.Tenant{
		AccountID: accountID, BranchID: branchID, UserID: seller, Role: domain.UserRoleSeller,
	}
	for _, id := range []uuid.UUID{unassignedID, ownID} {
		if _, err := getByRFQID(t, db, tenant, id); err != nil {
			t.Errorf("GetByRFQID(%v) = %v, want the row", id, err)
		}
	}
	for _, id := range []uuid.UUID{peerID, otherBranchOrder} {
		if _, err := getByRFQID(t, db, tenant, id); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("GetByRFQID(%v) = %v, want ErrNotFound", id, err)
		}
	}

	admin := domain.Tenant{
		AccountID: accountID, BranchID: branchID, UserID: peer, Role: domain.UserRoleAdmin,
	}
	for _, id := range []uuid.UUID{peerID, otherBranchOrder} {
		if _, err := getByRFQID(t, db, admin, id); err != nil {
			t.Errorf("admin GetByRFQID(%v) = %v, want the cross-branch row", id, err)
		}
	}
}

// assignRuns claims an order under the given tenant and reports the exact error.
func assignRuns(
	t *testing.T, db *DB, tenant domain.Tenant, rfqID uuid.UUID,
) (*domain.Quote, error) {
	t.Helper()
	repo := NewRFQRepository()
	var out *domain.Quote
	err := db.InTenantTx(context.Background(), tenant, func(q Querier) error {
		var err error
		out, err = repo.AssignSeller(context.Background(), q, tenant, rfqID)
		return err
	})
	return out, err
}

// The first seller to claim an unassigned order owns it; whoever comes after reads a conflict
// instead of taking the order away. It is the same guard a concurrent race would trip.
func TestRFQRepository_AssignSeller_OneClaimWins(t *testing.T) {
	db := testDB(t)
	accountID := seedAccount(t, db, "Seller assign scope")
	branchID := branchOf(t, db, accountID)
	channelID := seedChannel(t, db, accountID, branchID, domain.ChannelTypeManualEntry, true)
	seller1 := seedUser(t, db, accountID, "SELLER")
	seller2 := seedUser(t, db, accountID, "SELLER")

	rfqID := uuid.New()
	draft := domain.QuoteStatusDraft
	seedListingRFQ(t, db, accountID, branchID, channelID, listingSpec{
		rfqID: rfqID, quoteStatus: &draft, createdAt: time.Now(),
	})

	tenant1 := domain.Tenant{
		AccountID: accountID, BranchID: branchID, UserID: seller1, Role: domain.UserRoleSeller,
	}
	quote, err := assignRuns(t, db, tenant1, rfqID)
	if err != nil {
		t.Fatalf("first claim = %v, want the row", err)
	}
	if quote.SellerID == nil || *quote.SellerID != seller1 {
		t.Errorf("claimed seller_id = %v, want %v", quote.SellerID, seller1)
	}

	tenant2 := domain.Tenant{
		AccountID: accountID, BranchID: branchID, UserID: seller2, Role: domain.UserRoleSeller,
	}
	if _, err := assignRuns(t, db, tenant2, rfqID); !errors.Is(err, domain.ErrConflict) {
		t.Errorf("second claim = %v, want ErrConflict", err)
	}
	if _, err := assignRuns(t, db, tenant1, rfqID); !errors.Is(err, domain.ErrConflict) {
		t.Errorf("re-claim by the owner = %v, want ErrConflict", err)
	}
}

// An order outside the caller's branch scope answers not found, exactly like the read paths:
// a hidden order must not become claimable by reaching past the list.
func TestRFQRepository_AssignSeller_HiddenOrderIsNotFound(t *testing.T) {
	db := testDB(t)
	accountID := seedAccount(t, db, "Seller assign hidden")
	branchID := branchOf(t, db, accountID)
	otherBranchID := seedExtraBranch(t, db, accountID, "Sucursal Lejos")
	channelID := seedChannel(t, db, accountID, otherBranchID, domain.ChannelTypeManualEntry, true)
	seller := seedUser(t, db, accountID, "SELLER")

	rfqID := uuid.New()
	draft := domain.QuoteStatusDraft
	seedListingRFQ(t, db, accountID, otherBranchID, channelID, listingSpec{
		rfqID: rfqID, quoteStatus: &draft, createdAt: time.Now(),
	})

	tenant := domain.Tenant{
		AccountID: accountID, BranchID: branchID, UserID: seller, Role: domain.UserRoleSeller,
	}
	if _, err := assignRuns(t, db, tenant, rfqID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("claim of a hidden order = %v, want ErrNotFound", err)
	}
}

// setSellerRuns writes the order's owner under the given tenant and reports the exact error.
func setSellerRuns(
	t *testing.T, db *DB, tenant domain.Tenant, rfqID uuid.UUID, sellerID *uuid.UUID,
) (*domain.Quote, error) {
	t.Helper()
	repo := NewRFQRepository()
	var out *domain.Quote
	err := db.InTenantTx(context.Background(), tenant, func(q Querier) error {
		var err error
		out, err = repo.SetSeller(context.Background(), q, tenant, rfqID, sellerID)
		return err
	})
	return out, err
}

// An admin overwrites whoever owns the order, and a nil id clears it: SetSeller is the
// steering control, not a claim guard.
func TestRFQRepository_SetSeller_ReassignsAndClears(t *testing.T) {
	db := testDB(t)
	accountID := seedAccount(t, db, "Seller set scope")
	branchID := branchOf(t, db, accountID)
	channelID := seedChannel(t, db, accountID, branchID, domain.ChannelTypeManualEntry, true)
	seller1 := seedUser(t, db, accountID, "SELLER")
	seller2 := seedUser(t, db, accountID, "SELLER")

	rfqID := uuid.New()
	quoted := domain.QuoteStatusQuoted
	seedListingRFQ(t, db, accountID, branchID, channelID, listingSpec{
		rfqID: rfqID, sellerID: &seller1, quoteStatus: &quoted, createdAt: time.Now(),
	})

	admin := domain.Tenant{
		AccountID: accountID, BranchID: branchID, UserID: seller2, Role: domain.UserRoleAdmin,
	}
	reassigned, err := setSellerRuns(t, db, admin, rfqID, &seller2)
	if err != nil {
		t.Fatalf("reassign = %v, want the row", err)
	}
	if reassigned.SellerID == nil || *reassigned.SellerID != seller2 {
		t.Errorf("reassigned seller_id = %v, want %v", reassigned.SellerID, seller2)
	}

	cleared, err := setSellerRuns(t, db, admin, rfqID, nil)
	if err != nil {
		t.Fatalf("clear = %v, want the row", err)
	}
	if cleared.SellerID != nil {
		t.Errorf("cleared seller_id = %v, want nil", cleared.SellerID)
	}
}

// SetSeller keeps the same hidden-order boundary as the claim: an order outside the admin's
// active branch scope is not reachable, while the account-wide admin reaches every branch.
func TestRFQRepository_SetSeller_HiddenOrderIsNotFound(t *testing.T) {
	db := testDB(t)
	accountID := seedAccount(t, db, "Seller set hidden")
	branchID := branchOf(t, db, accountID)
	otherBranchID := seedExtraBranch(t, db, accountID, "Sucursal Lejos")
	channelID := seedChannel(t, db, accountID, otherBranchID, domain.ChannelTypeManualEntry, true)
	seller := seedUser(t, db, accountID, "SELLER")

	rfqID := uuid.New()
	draft := domain.QuoteStatusDraft
	seedListingRFQ(t, db, accountID, otherBranchID, channelID, listingSpec{
		rfqID: rfqID, quoteStatus: &draft, createdAt: time.Now(),
	})

	scoped := domain.Tenant{
		AccountID: accountID, BranchID: branchID, UserID: seller, Role: domain.UserRoleAdmin,
	}
	if _, err := setSellerRuns(t, db, scoped, rfqID, &seller); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("set on an out-of-branch order = %v, want ErrNotFound", err)
	}

	wholeAccount := domain.Tenant{
		AccountID: accountID, UserID: seller, Role: domain.UserRoleAdmin,
	}
	reached, err := setSellerRuns(t, db, wholeAccount, rfqID, &seller)
	if err != nil {
		t.Fatalf("account-wide set = %v, want the row", err)
	}
	if reached.SellerID == nil || *reached.SellerID != seller {
		t.Errorf("account-wide seller_id = %v, want %v", reached.SellerID, seller)
	}
}

func containsID(t *testing.T, items []domain.RfqListItem, id uuid.UUID) bool {
	t.Helper()
	for _, item := range items {
		if item.ID == id {
			return true
		}
	}
	return false
}

func strPtr(s string) *string { return &s }
