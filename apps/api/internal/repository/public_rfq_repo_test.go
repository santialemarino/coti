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

func TestPublicRFQRepositoryActiveDestinationAndContactIsolation(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountID := seedAccount(t, db, "public intake")
	otherAccountID := seedAccount(t, db, "other public intake")
	branchID := branchOf(t, db, accountID)
	otherBranchID := branchOf(t, db, otherAccountID)
	channelID := seedChannel(t, db, accountID, branchID, domain.ChannelTypeWebApp, true)
	repo := NewPublicRFQRepository()
	destination, err := repo.GetDestinationCrossAccount(ctx, db.CrossAccount(), channelID)
	if err != nil {
		t.Fatal(err)
	}
	if destination.Route.AccountID != accountID || destination.Route.BranchID != branchID || destination.Route.ChannelType != domain.ChannelTypeWebApp {
		t.Fatalf("wrong public route: %+v", destination.Route)
	}
	rfq := seedTextRFQ(t, db, accountID, branchID, channelID)
	messageRepo := NewInboundChannelMessageRepository()
	messageID := uuid.New()
	t.Cleanup(func() {
		mustCleanup(t, db.CrossAccount(), `DELETE FROM inbound_channel_message WHERE id = $1`, messageID)
	})
	err = db.InTenantTx(ctx, destination.Route.Tenant(), func(q Querier) error {
		_, reserveErr := messageRepo.Reserve(ctx, q, accountID, branchID, channelID, domain.NewInboundChannelMessage{
			ID: messageID, RFQID: &rfq.ID, ExternalMessageID: uuid.NewString(), SenderID: "contacto@example.com",
			Body: "cemento", Payload: []byte(`{"contact":{"tax_id":"20123456786","name":"Obra Norte","email":"contacto@example.com"}}`), ProviderReceivedAt: time.Now()})
		return reserveErr
	})
	if err != nil {
		t.Fatal(err)
	}
	err = db.InTenantTx(ctx, destination.Route.Tenant(), func(q Querier) error {
		contact, err := repo.FindPublicContactByRFQID(ctx, q, accountID, branchID, rfq.ID)
		if err != nil {
			return err
		}
		if contact == nil || contact.Email != "contacto@example.com" {
			t.Fatalf("missing public contact: %+v", contact)
		}
		contact, err = repo.FindPublicContactByRFQID(ctx, q, accountID, otherBranchID, rfq.ID)
		if contact != nil {
			t.Fatal("cross-branch contact leak")
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	err = db.InTenantTx(ctx, domain.Tenant{AccountID: otherAccountID, BranchID: otherBranchID}, func(q Querier) error {
		contact, err := repo.FindPublicContactByRFQID(ctx, q, accountID, branchID, rfq.ID)
		if contact != nil {
			t.Fatal("RLS allowed cross-account contact access")
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CrossAccount().Exec(ctx, `UPDATE channel SET is_active = FALSE WHERE id = $1`, channelID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetDestinationCrossAccount(ctx, db.CrossAccount(), channelID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("closed channel: %v", err)
	}
}
