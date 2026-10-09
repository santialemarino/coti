//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func TestInboundChannelMessageRepository_ReserveReturnsOriginalRFQOnRetry(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountID := seedAccount(t, db, "inbound message account")
	branchID := branchOf(t, db, accountID)
	channelID := seedChannel(t, db, accountID, branchID, domain.ChannelTypeWhatsApp, true)
	rfq := seedTextRFQ(t, db, accountID, branchID, channelID)
	repo := NewInboundChannelMessageRepository()
	tenant := domain.Tenant{AccountID: accountID, BranchID: branchID}
	receivedAt := time.Date(2026, time.October, 9, 12, 0, 0, 0, time.UTC)

	reserve := func(rfqID uuid.UUID) *domain.InboundChannelMessage {
		t.Helper()
		var message *domain.InboundChannelMessage
		err := db.InTenantTx(ctx, tenant, func(q Querier) error {
			var reserveErr error
			message, reserveErr = repo.Reserve(ctx, q, accountID, branchID, channelID,
				domain.NewInboundChannelMessage{
					ID:                 uuid.New(),
					RFQID:              rfqID,
					ExternalMessageID:  "wamid.retry-safe",
					SenderID:           "5491112345678",
					Body:               "Necesito cemento",
					Payload:            []byte(`{"object":"whatsapp_business_account"}`),
					ProviderReceivedAt: receivedAt,
				})
			return reserveErr
		})
		if err != nil {
			t.Fatalf("Reserve() = %v, want no error", err)
		}
		return message
	}

	first := reserve(rfq.ID)
	t.Cleanup(func() {
		mustCleanup(t, db.CrossAccount(), `DELETE FROM inbound_channel_message WHERE id = $1`, first.ID)
	})
	second := reserve(uuid.New())

	if first.RFQID != rfq.ID || second.RFQID != rfq.ID {
		t.Errorf("reserved RFQ IDs = %s, %s; want %s", first.RFQID, second.RFQID, rfq.ID)
	}
	if first.ID != second.ID {
		t.Errorf("reserved event IDs = %s, %s; want the original event", first.ID, second.ID)
	}

	var senderID *string
	err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		var findErr error
		senderID, findErr = repo.FindSenderIDByRFQID(ctx, q, accountID, branchID, rfq.ID)
		return findErr
	})
	if err != nil {
		t.Fatalf("FindSenderIDByRFQID() = %v, want no error", err)
	}
	if senderID == nil || *senderID != "5491112345678" {
		t.Errorf("sender ID = %v, want %q", senderID, "5491112345678")
	}

	var missing *string
	err = db.InTenantTx(ctx, tenant, func(q Querier) error {
		var findErr error
		missing, findErr = repo.FindSenderIDByRFQID(ctx, q, accountID, branchID, uuid.New())
		return findErr
	})
	if err != nil {
		t.Fatalf("FindSenderIDByRFQID(missing) = %v, want no error", err)
	}
	if missing != nil {
		t.Errorf("missing sender ID = %q, want nil", *missing)
	}
}
