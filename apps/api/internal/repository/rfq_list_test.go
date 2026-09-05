//go:build integration

package repository

import (
	"context"
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
			`INSERT INTO quote (id, account_id, branch_id, rfq_id, current_status,
			                    needs_followup, archived_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			quoteID, accountID, branchID, spec.rfqID, *spec.quoteStatus, spec.needsFollowup,
			archivedAt); err != nil {
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
	ctx := context.Background()
	repo := NewRFQRepository()
	var items []domain.RfqListItem
	if err := db.InTenantTx(ctx,
		domain.Tenant{AccountID: accountID, BranchID: branchID, Role: domain.UserRoleAdmin},
		func(q Querier) error {
			var err error
			items, err = repo.ListByTenant(ctx, q, domain.Tenant{
				AccountID: accountID, BranchID: branchID, Role: domain.UserRoleAdmin,
			})
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
