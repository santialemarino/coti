//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func TestSellerReportRepository_GetScopesAndAggregatesTheSelectedSeller(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountID := seedAccount(t, db, "Seller report")
	branchID := branchOf(t, db, accountID)
	channelID := seedChannel(t, db, accountID, branchID, domain.ChannelTypeManualEntry, true)
	sellerID := seedUser(t, db, accountID, "SELLER")
	otherSellerID := seedUser(t, db, accountID, "SELLER")
	clientID := seedClient(t, db, accountID)
	if _, err := db.CrossAccount().Exec(ctx,
		`UPDATE client SET name = $2 WHERE id = $1`, clientID, "Obras Norte"); err != nil {
		t.Fatalf("name client: %v", err)
	}

	day := time.Now().UTC().Truncate(24 * time.Hour)
	sentStatus := domain.QuoteStatusSent
	acceptedStatus := domain.QuoteStatusAccepted
	ownSentRFQ := uuid.New()
	ownAcceptedRFQ := uuid.New()
	peerRFQ := uuid.New()
	oldRFQ := uuid.New()
	for _, item := range []struct {
		id       uuid.UUID
		status   domain.QuoteStatus
		sellerID *uuid.UUID
		received time.Time
	}{
		{ownSentRFQ, sentStatus, &sellerID, day.Add(9 * time.Hour)},
		{ownAcceptedRFQ, acceptedStatus, &sellerID, day.Add(10 * time.Hour)},
		{peerRFQ, sentStatus, &otherSellerID, day.Add(9 * time.Hour)},
		{oldRFQ, sentStatus, &sellerID, day.Add(-24 * time.Hour)},
	} {
		seedListingRFQ(t, db, accountID, branchID, channelID, listingSpec{
			rfqID: item.id, clientID: &clientID, quoteStatus: &item.status,
			sellerID: item.sellerID, createdAt: item.received,
		})
		if _, err := db.CrossAccount().Exec(ctx,
			`UPDATE rfq SET received_at = $2 WHERE id = $1`, item.id, item.received); err != nil {
			t.Fatalf("set RFQ receipt date: %v", err)
		}
	}

	quoteIDs := make(map[uuid.UUID]uuid.UUID, 4)
	for _, rfqID := range []uuid.UUID{ownSentRFQ, ownAcceptedRFQ, peerRFQ, oldRFQ} {
		var quoteID uuid.UUID
		if err := db.CrossAccount().QueryRow(ctx,
			`SELECT id FROM quote WHERE account_id = $1 AND rfq_id = $2`, accountID, rfqID,
		).Scan(&quoteID); err != nil {
			t.Fatalf("find quote for RFQ %s: %v", rfqID, err)
		}
		quoteIDs[rfqID] = quoteID
	}

	versionIDs := make([]uuid.UUID, 0, 2)
	for _, rfqID := range []uuid.UUID{ownSentRFQ, ownAcceptedRFQ} {
		versionID := uuid.New()
		versionIDs = append(versionIDs, versionID)
		if _, err := db.CrossAccount().Exec(ctx,
			`INSERT INTO quote_version (id, account_id, quote_id, version_number, total)
			 VALUES ($1, $2, $3, 1, 0)`,
			versionID, accountID, quoteIDs[rfqID]); err != nil {
			t.Fatalf("create quote version: %v", err)
		}
		if _, err := db.CrossAccount().Exec(ctx,
			`UPDATE quote SET current_version_id = $2 WHERE id = $1`,
			quoteIDs[rfqID], versionID); err != nil {
			t.Fatalf("set current version: %v", err)
		}
	}
	productID := uuid.New()
	if _, err := db.CrossAccount().Exec(ctx,
		`INSERT INTO product (id, account_id, canonical_name) VALUES ($1, $2, $3)`,
		productID, accountID, "Cemento"); err != nil {
		t.Fatalf("create product: %v", err)
	}
	for _, versionID := range versionIDs {
		if _, err := db.CrossAccount().Exec(ctx,
			`INSERT INTO quote_item
			   (account_id, version_id, product_id, requested_description, quantity, match_status)
			 VALUES ($1, $2, $3, 'cemento', 1, 'MATCHED')`,
			accountID, versionID, productID); err != nil {
			t.Fatalf("create quote item: %v", err)
		}
	}

	changeIDs := make([]uuid.UUID, 0, 5)
	appendStatus := func(rfqID uuid.UUID, status domain.QuoteStatus, changedAt time.Time) {
		t.Helper()
		changeID := uuid.New()
		changeIDs = append(changeIDs, changeID)
		if _, err := db.CrossAccount().Exec(ctx,
			`INSERT INTO quote_status_change
			   (id, account_id, quote_id, new_status, changed_at)
			 VALUES ($1, $2, $3, $4, $5)`,
			changeID, accountID, quoteIDs[rfqID], status, changedAt); err != nil {
			t.Fatalf("create quote status change: %v", err)
		}
	}
	appendStatus(ownSentRFQ, sentStatus, day.Add(11*time.Hour))
	appendStatus(ownAcceptedRFQ, sentStatus, day.Add(13*time.Hour))
	appendStatus(ownAcceptedRFQ, acceptedStatus, day.Add(14*time.Hour))
	appendStatus(peerRFQ, sentStatus, day.Add(11*time.Hour))
	appendStatus(oldRFQ, sentStatus, day.Add(-23*time.Hour))

	t.Cleanup(func() {
		mustCleanup(t, db.CrossAccount(), `UPDATE quote SET current_version_id = NULL
			WHERE id = ANY($1::uuid[])`, []uuid.UUID{quoteIDs[ownSentRFQ], quoteIDs[ownAcceptedRFQ]})
		mustCleanup(t, db.CrossAccount(), `DELETE FROM quote_status_change WHERE id = ANY($1::uuid[])`, changeIDs)
		mustCleanup(t, db.CrossAccount(), `DELETE FROM quote_item WHERE version_id = ANY($1::uuid[])`, versionIDs)
		mustCleanup(t, db.CrossAccount(), `DELETE FROM quote_version WHERE id = ANY($1::uuid[])`, versionIDs)
		mustCleanup(t, db.CrossAccount(), `DELETE FROM product WHERE id = $1`, productID)
	})

	filter := domain.SellerReportFilter{DateFrom: &day, DateTo: &day}
	tenant := domain.Tenant{
		AccountID: accountID,
		UserID:    sellerID,
		BranchID:  branchID,
		Role:      domain.UserRoleSeller,
	}
	var report *domain.SellerReport
	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		var err error
		report, err = NewSellerReportRepository().Get(
			ctx, q, accountID, sellerID, tenant.BranchFilter(), filter,
		)
		return err
	}); err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if report.OrdersReceived != 2 || report.QuotesSent != 2 || report.QuotesAccepted != 1 {
		t.Errorf("summary = (%d orders, %d sent, %d accepted), want (2, 2, 1)",
			report.OrdersReceived, report.QuotesSent, report.QuotesAccepted)
	}
	if report.AverageQuoteTimeSeconds == nil || *report.AverageQuoteTimeSeconds != 9000 {
		t.Errorf("average quote time = %v, want 9000 seconds", report.AverageQuoteTimeSeconds)
	}
	if len(report.Statuses) != 2 ||
		report.Statuses[0] != (domain.SellerReportStatus{Status: "ACCEPTED", Count: 1}) ||
		report.Statuses[1] != (domain.SellerReportStatus{Status: "SENT", Count: 1}) {
		t.Errorf("status counts = %v, want accepted 1 and sent 1", report.Statuses)
	}
	if len(report.TopMaterials) != 1 ||
		report.TopMaterials[0] != (domain.SellerReportMaterial{Name: "Cemento", OrderCount: 2}) {
		t.Errorf("top materials = %v, want Cemento in 2 orders", report.TopMaterials)
	}
	if len(report.TopClients) != 1 ||
		report.TopClients[0] != (domain.SellerReportClient{Name: "Obras Norte", OrderCount: 2}) {
		t.Errorf("top clients = %v, want Obras Norte with 2 orders", report.TopClients)
	}
}
