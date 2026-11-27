//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func TestQuoteSendRepository_ListByQuote_OrdersAndNarrowsToBranch(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountID := seedAccount(t, db, "Quote send tracking")
	branchID := branchOf(t, db, accountID)
	otherBranchID := seedExtraBranch(t, db, accountID, "Sucursal Sur")
	productID := seedProduct(t, db, accountID, "Cemento Portland 50kg")
	priceCleanup(t, db, productID)
	quoteID, versionID, _ := seedQuoteChain(t, db, accountID, branchID, productID)
	otherQuoteID, otherVersionID, _ := seedQuoteChain(t, db, accountID, otherBranchID, productID)

	t.Cleanup(func() {
		mustCleanup(t, db.CrossAccount(),
			`DELETE FROM quote_send WHERE version_id IN ($1, $2)`, versionID, otherVersionID)
	})

	channelID := channelForQuote(t, db, quoteID)
	otherChannelID := channelForQuote(t, db, otherQuoteID)
	olderID, newerID := uuid.New(), uuid.New()
	olderAt := time.Date(2026, time.September, 4, 9, 0, 0, 0, time.UTC)
	newerAt := olderAt.Add(time.Hour)
	expiresAt := newerAt.AddDate(0, 0, 7)
	if _, err := db.CrossAccount().Exec(ctx,
		`INSERT INTO quote_send (id, account_id, version_id, channel_id, idempotency_key,
		                         destination, public_token, format, validity_days, sent_at,
		                         expires_at, tracking_status, created_at)
		 VALUES ($1, $2, $3, $4, $5, '+5491155550101', 'older-token', 'WEBAPP_LINK', 7, $6,
		         $8, 'SENT', $6),
		        ($9, $2, $3, $4, $10, '+5491155550102', 'newer-token', 'WEBAPP_LINK', 7, $7,
		         $8, 'VIEWED', $7),
		        ($11, $2, $12, $13, $14, '+5491155550103', 'other-token', 'WEBAPP_LINK', 7, $7,
		         $8, 'DELIVERED', $7)`,
		olderID, accountID, versionID, channelID, uuid.New(), olderAt, newerAt, expiresAt,
		newerID, uuid.New(), uuid.New(), otherVersionID, otherChannelID, uuid.New()); err != nil {
		t.Fatalf("seed quote sends: %v", err)
	}

	repo := NewQuoteSendRepository()
	var sends []domain.QuoteSend
	if err := db.InTenantTx(ctx,
		domain.Tenant{AccountID: accountID, BranchID: branchID, Role: domain.UserRoleAdmin},
		func(q Querier) error {
			var readErr error
			sends, readErr = repo.ListByQuote(ctx, q, accountID, branchID, quoteID)
			return readErr
		}); err != nil {
		t.Fatalf("ListByQuote() = %v, want no error", err)
	}
	if len(sends) != 2 {
		t.Fatalf("sends = %d, want two attempts for the selected quote", len(sends))
	}
	if sends[0].ID != newerID || sends[0].TrackingStatus != domain.SendTrackingStatusViewed ||
		sends[1].ID != olderID || sends[1].TrackingStatus != domain.SendTrackingStatusSent {
		t.Errorf("sends = %+v, want newest VIEWED then older SENT", sends)
	}
	if sends[0].ChannelType != domain.ChannelTypeWhatsApp || sends[0].ExpiresAt == nil {
		t.Errorf("newest send = %+v, want channel and expiry populated", sends[0])
	}

	if err := db.InTenantTx(ctx,
		domain.Tenant{AccountID: accountID, BranchID: otherBranchID, Role: domain.UserRoleAdmin},
		func(q Querier) error {
			var readErr error
			sends, readErr = repo.ListByQuote(ctx, q, accountID, otherBranchID, quoteID)
			return readErr
		}); err != nil {
		t.Fatalf("wrong-branch ListByQuote() = %v, want no error", err)
	}
	if len(sends) != 0 {
		t.Errorf("wrong branch read %d sends, want none", len(sends))
	}
}

func channelForQuote(t *testing.T, db *DB, quoteID uuid.UUID) uuid.UUID {
	t.Helper()
	var channelID uuid.UUID
	if err := db.CrossAccount().QueryRow(context.Background(),
		`SELECT rfq.channel_id
		 FROM quote
		 JOIN rfq ON rfq.account_id = quote.account_id AND rfq.id = quote.rfq_id
		 WHERE quote.id = $1`,
		quoteID).Scan(&channelID); err != nil {
		t.Fatalf("read quote channel: %v", err)
	}
	return channelID
}

func TestQuoteSendRepository_ApplyProviderStatuses_MovesOnlyForwardOnItsChannel(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountID := seedAccount(t, db, "Quote send provider statuses")
	branchID := branchOf(t, db, accountID)
	otherBranchID := seedExtraBranch(t, db, accountID, "Sucursal Oeste")
	productID := seedProduct(t, db, accountID, "Arena fina")
	priceCleanup(t, db, productID)
	quoteID, versionID, _ := seedQuoteChain(t, db, accountID, branchID, productID)
	otherQuoteID, otherVersionID, _ := seedQuoteChain(t, db, accountID, otherBranchID, productID)
	t.Cleanup(func() {
		mustCleanup(t, db.CrossAccount(),
			`DELETE FROM quote_send WHERE version_id IN ($1, $2)`, versionID, otherVersionID)
	})
	channelID := channelForQuote(t, db, quoteID)
	otherChannelID := channelForQuote(t, db, otherQuoteID)

	ids := map[string]uuid.UUID{}
	for _, seed := range []struct {
		reference, status string
		channel, version  uuid.UUID
	}{
		{"wamid.sent", "SENT", channelID, versionID},
		{"wamid.delivered", "DELIVERED", channelID, versionID},
		{"wamid.viewed", "VIEWED", channelID, versionID},
		{"wamid.refused", "SENT", channelID, versionID},
		{"wamid.late-failure", "DELIVERED", channelID, versionID},
		{"wamid.elsewhere", "SENT", otherChannelID, otherVersionID},
	} {
		ids[seed.reference] = uuid.New()
		if _, err := db.CrossAccount().Exec(ctx,
			`INSERT INTO quote_send (id, account_id, version_id, channel_id, destination,
			   provider_reference, format, sent_at, expires_at, tracking_status)
			 VALUES ($1, $2, $3, $4, '+5491155550101', $5, 'WEBAPP_LINK', now(), now() + interval '7 days',
			   $6::send_tracking_status)`,
			ids[seed.reference], accountID, seed.version, seed.channel, seed.reference, seed.status); err != nil {
			t.Fatalf("seed %s: %v", seed.reference, err)
		}
	}

	repo := NewQuoteSendRepository()
	var changed []domain.QuoteSend
	if err := db.InTenantTx(ctx, domain.Tenant{AccountID: accountID, BranchID: branchID,
		Role: domain.UserRoleAdmin}, func(q Querier) error {
		var err error
		changed, err = repo.ApplyProviderStatuses(ctx, q, accountID, channelID,
			[]domain.ProviderDeliveryStatus{
				{ProviderReference: "wamid.sent", Status: domain.SendTrackingStatusDelivered},
				{ProviderReference: "wamid.delivered", Status: domain.SendTrackingStatusViewed},
				{ProviderReference: "wamid.viewed", Status: domain.SendTrackingStatusDelivered},
				{ProviderReference: "wamid.refused", Status: domain.SendTrackingStatusFailed},
				{ProviderReference: "wamid.late-failure", Status: domain.SendTrackingStatusFailed},
				{ProviderReference: "wamid.elsewhere", Status: domain.SendTrackingStatusViewed},
				{ProviderReference: "wamid.unknown", Status: domain.SendTrackingStatusViewed},
			})
		return err
	}); err != nil {
		t.Fatalf("ApplyProviderStatuses() = %v, want no error", err)
	}
	if len(changed) != 3 {
		t.Errorf("changed = %d sends, want the three forward moves", len(changed))
	}

	want := map[string]string{"wamid.sent": "DELIVERED", "wamid.delivered": "VIEWED",
		"wamid.viewed": "VIEWED", "wamid.refused": "FAILED", "wamid.late-failure": "DELIVERED",
		"wamid.elsewhere": "SENT"}
	for reference, status := range want {
		var got string
		if err := db.CrossAccount().QueryRow(ctx,
			`SELECT tracking_status::text FROM quote_send WHERE id = $1`, ids[reference]).Scan(&got); err != nil {
			t.Fatalf("read %s: %v", reference, err)
		}
		if got != status {
			t.Errorf("%s = %s, want %s", reference, got, status)
		}
	}
}
