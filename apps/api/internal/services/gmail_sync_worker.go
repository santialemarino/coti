package services

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type gmailMailboxStore interface {
	ListConnectedCrossAccount(context.Context, repository.Querier) ([]domain.GmailMailbox, error)
	GetConnected(context.Context, repository.Querier, uuid.UUID, uuid.UUID, uuid.UUID) (*domain.GmailMailbox, error)
	SaveCursor(context.Context, repository.Querier, domain.GmailMailbox, string, time.Time) error
}

type gmailReaderFactory interface {
	NewReader(context.Context, string) (domain.EmailMailboxReader, error)
}

// GmailSyncWorker periodically synchronizes each connected and active branch mailbox.
type GmailSyncWorker struct {
	db       gmailAuthorizationDB
	store    gmailMailboxStore
	cipher   gmailCredentialCipher
	factory  gmailReaderFactory
	sync     *EmailSyncService
	interval time.Duration
	log      *slog.Logger
	now      func() time.Time
}

// NewGmailSyncWorker builds a periodic mailbox synchronizer.
func NewGmailSyncWorker(db gmailAuthorizationDB, store gmailMailboxStore, cipher gmailCredentialCipher,
	factory gmailReaderFactory, sync *EmailSyncService, interval time.Duration, log *slog.Logger,
) *GmailSyncWorker {
	return &GmailSyncWorker{db: db, store: store, cipher: cipher, factory: factory, sync: sync,
		interval: interval, log: log, now: time.Now}
}

// Run synchronizes immediately and until the process context is cancelled.
func (w *GmailSyncWorker) Run(ctx context.Context) {
	if w.interval <= 0 {
		return
	}
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		if err := w.runOnce(ctx); err != nil && ctx.Err() == nil {
			w.log.ErrorContext(ctx, "gmail mailbox discovery failed", slog.Any("error", err))
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (w *GmailSyncWorker) runOnce(ctx context.Context) error {
	mailboxes, err := w.store.ListConnectedCrossAccount(ctx, w.db.CrossAccount())
	if err != nil {
		return err
	}
	for _, mailbox := range mailboxes {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := w.syncMailbox(ctx, mailbox.Route); err != nil && !errors.Is(err, domain.ErrNotFound) {
			w.log.ErrorContext(ctx, "gmail mailbox synchronization failed",
				slog.String("channel_id", mailbox.Route.ChannelID.String()), slog.Any("error", err))
		}
	}
	return nil
}

func (w *GmailSyncWorker) syncMailbox(ctx context.Context, route domain.InboundChannelRoute) error {
	var mailbox *domain.GmailMailbox
	if err := w.db.InTenantTx(ctx, route.Tenant(), func(q repository.Querier) error {
		var err error
		mailbox, err = w.store.GetConnected(ctx, q, route.AccountID, route.BranchID, route.ChannelID)
		return err
	}); err != nil {
		return err
	}
	if mailbox.LastSyncedAt != nil && w.now().Sub(*mailbox.LastSyncedAt) < w.interval {
		return nil
	}
	token, err := w.cipher.Open(mailbox.RefreshTokenCiphertext)
	if err != nil {
		return err
	}
	reader, err := w.factory.NewReader(ctx, token)
	if err != nil {
		return err
	}
	cursor, err := w.sync.Synchronize(ctx, reader, mailbox.EmailMailboxConnection)
	if err != nil {
		return err
	}
	return w.db.InTenantTx(ctx, route.Tenant(), func(q repository.Querier) error {
		return w.store.SaveCursor(ctx, q, *mailbox, cursor, w.now().UTC())
	})
}
