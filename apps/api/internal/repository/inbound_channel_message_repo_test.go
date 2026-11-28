//go:build integration

package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

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
					RFQID:              &rfqID,
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

	if first.RFQID == nil || *first.RFQID != rfq.ID || second.RFQID == nil || *second.RFQID != rfq.ID {
		t.Errorf("reserved RFQ IDs = %v, %v; want %s", first.RFQID, second.RFQID, rfq.ID)
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

// An event becomes an order or a conversation message, never both and never neither.
func TestInboundChannelMessageRepository_ReserveNeedsExactlyOneTarget(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountID := seedAccount(t, db, "inbound target account")
	branchID := branchOf(t, db, accountID)
	channelID := seedChannel(t, db, accountID, branchID, domain.ChannelTypeWhatsApp, true)
	rfqID, messageID := uuid.New(), uuid.New()

	for name, in := range map[string]domain.NewInboundChannelMessage{
		"neither": {},
		"both":    {RFQID: &rfqID, QuoteMessageID: &messageID},
	} {
		in.ID, in.ExternalMessageID, in.SenderID = uuid.New(), "wamid."+name, "5491112345678"
		in.Body, in.Payload, in.ProviderReceivedAt = "cemento", []byte(`{}`), time.Now()
		err := db.InTenantTx(ctx, domain.Tenant{AccountID: accountID, BranchID: branchID},
			func(q Querier) error {
				_, err := NewInboundChannelMessageRepository().Reserve(ctx, q, accountID, branchID,
					channelID, in)
				return err
			})
		if !isCheckViolation(err, "ck_inbound_channel_message_target") {
			t.Errorf("%s: Reserve() = %v, want ck_inbound_channel_message_target", name, err)
		}
	}
}

// A message that is not an order is kept with its kind and no target; a kind beside a target, or
// a dismissal of a message that opened an order, is refused.
func TestInboundChannelMessage_KeepsANonOrderMessageWithItsKindOnly(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountID := seedAccount(t, db, "inbound kind account")
	branchID := branchOf(t, db, accountID)
	channelID := seedChannel(t, db, accountID, branchID, domain.ChannelTypeWhatsApp, true)
	t.Cleanup(func() {
		mustCleanup(t, db.CrossAccount(),
			`DELETE FROM inbound_channel_message WHERE channel_id = $1`, channelID)
	})
	insert := func(external string, rfqID *uuid.UUID, kind *string, dismissed bool) error {
		_, err := db.CrossAccount().Exec(ctx, `INSERT INTO inbound_channel_message
		  (account_id, branch_id, channel_id, rfq_id, external_message_id, sender_id, body, payload,
		   provider_received_at, kind, dismissed_at)
		 VALUES ($1, $2, $3, $4, $5, '5491112345678', 'hola', '{}', now(),
		         $6::inbound_message_kind, CASE WHEN $7 THEN now() END)`,
			accountID, branchID, channelID, rfqID, external, kind, dismissed)
		return err
	}
	greeting, rfqID := "GREETING", uuid.New()

	if err := insert("wamid.greeting", nil, &greeting, true); err != nil {
		t.Fatalf("insert a dismissed greeting = %v, want it kept", err)
	}
	if err := insert("wamid.kind-and-rfq", &rfqID, &greeting, false); !isCheckViolation(err,
		"ck_inbound_channel_message_target") {
		t.Errorf("kind beside an rfq = %v, want ck_inbound_channel_message_target", err)
	}
	if err := insert("wamid.dismissed-order", &rfqID, nil, true); !isCheckViolation(err,
		"ck_inbound_channel_message_dismissed") {
		t.Errorf("dismissed order = %v, want ck_inbound_channel_message_dismissed", err)
	}
}

func isCheckViolation(err error, name string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23514" && pgErr.ConstraintName == name
}
