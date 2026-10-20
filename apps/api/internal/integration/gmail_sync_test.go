//go:build integration

package integration

import (
	"context"
	"io"
	"log/slog"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
	"github.com/santialemarino/coti/apps/api/internal/secrets"
	"github.com/santialemarino/coti/apps/api/internal/services"
	"github.com/santialemarino/coti/apps/api/internal/storage"
)

type testMailbox struct {
	message domain.InboundEmailMessage
}

func (m *testMailbox) CurrentCursor(context.Context) (string, error) { return "200", nil }
func (m *testMailbox) History(context.Context, string, string) (domain.EmailSyncPage, error) {
	return domain.EmailSyncPage{HistoryID: "200"}, nil
}
func (m *testMailbox) MessagesSince(context.Context, time.Time, string) (domain.EmailSyncPage, error) {
	return domain.EmailSyncPage{MessageIDs: []string{m.message.ExternalMessageID}}, nil
}
func (m *testMailbox) ReadMessage(context.Context, string) (domain.InboundEmailMessage, error) {
	return m.message, nil
}

type testMailboxFactory struct{ mailbox *testMailbox }

func (f testMailboxFactory) NewReader(_ context.Context, token string) (domain.EmailMailboxReader, error) {
	if token != "refresh-secret" {
		panic("mailbox credential was not decrypted")
	}
	return f.mailbox, nil
}

func TestGmailWorker_SynchronizesOriginalIntoTheConnectedBranch(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Gmail worker")
	t.Cleanup(func() {
		e.mustCleanup(t, `DELETE FROM gmail_authorization_state WHERE account_id = $1`, accountID)
		e.mustCleanup(t, `DELETE FROM gmail_mailbox WHERE account_id = $1`, accountID)
	})
	channelID := uuid.New()
	address := uuid.NewString() + "@example.test"
	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`INSERT INTO channel (id, account_id, branch_id, type, identifier)
		 VALUES ($1, $2, $3, 'EMAIL', $4)`, channelID, accountID, branchID, address); err != nil {
		t.Fatal(err)
	}
	cipher, err := secrets.NewAESGCM([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	token, err := cipher.Seal("refresh-secret")
	if err != nil {
		t.Fatal(err)
	}
	store := repository.NewGmailMailboxRepository()
	route := domain.InboundChannelRoute{AccountID: accountID, BranchID: branchID,
		ChannelID: channelID, ChannelType: domain.ChannelTypeEmail}
	connectedAt := time.Now().Add(-time.Minute)
	if err := e.db.InTenantTx(context.Background(), route.Tenant(), func(q repository.Querier) error {
		return store.SaveConnected(context.Background(), q, domain.GmailMailbox{
			EmailMailboxConnection: domain.EmailMailboxConnection{Route: route, Address: address,
				HistoryID: "100", ConnectedAt: connectedAt}, RefreshTokenCiphertext: token})
	}); err != nil {
		t.Fatal(err)
	}
	baseURL, _ := url.Parse("http://localhost:8080")
	objects, err := storage.NewLocalStorage(t.TempDir(), baseURL,
		storage.NewURLSigner([]byte("email-worker-test-secret-at-least-32-bytes"), nil))
	if err != nil {
		t.Fatal(err)
	}
	intake := services.NewEmailInboundService(e.db, repository.NewInboundChannelMessageRepository(),
		repository.NewRFQRepository(), repository.NewRFQAttachmentRepository(),
		repository.NewChannelRepository(), objects, 1024)
	factory := testMailboxFactory{mailbox: &testMailbox{message: domain.InboundEmailMessage{
		ExternalMessageID: "test-message", SenderID: "client@example.test", Body: "20 cement bags",
		Original: []byte("From: client@example.test\r\n\r\n20 cement bags"), ProviderReceivedAt: time.Now(),
	}}}
	worker := services.NewGmailSyncWorker(e.db, store, cipher, factory,
		services.NewEmailSyncService(intake), time.Hour, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); worker.Run(ctx) }()
	t.Cleanup(func() { cancel(); <-done })
	deadline := time.Now().Add(10 * time.Second)
	for {
		var cursor string
		var pending bool
		err := e.db.CrossAccount().QueryRow(context.Background(),
			`SELECT history_id, initial_sync_pending FROM gmail_mailbox WHERE channel_id = $1`, channelID).
			Scan(&cursor, &pending)
		if err != nil {
			t.Fatal(err)
		}
		if cursor == "200" && !pending {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("worker did not finish synchronization")
		}
		time.Sleep(25 * time.Millisecond)
	}
	cancel()
	<-done
	var count int
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT count(*) FROM inbound_channel_message WHERE account_id = $1 AND branch_id = $2 AND channel_id = $3`,
		accountID, branchID, channelID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("ingested message count = %d, error = %v", count, err)
	}
}
