package services

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

type emailMessageIngester interface {
	Ingest(context.Context, domain.InboundChannelRoute, domain.InboundEmailMessage) (uuid.UUID, error)
}

// EmailSyncService synchronizes a mailbox without advancing past unrecorded messages.
type EmailSyncService struct {
	ingester emailMessageIngester
}

// NewEmailSyncService builds a mailbox synchronizer.
func NewEmailSyncService(ingester emailMessageIngester) *EmailSyncService {
	return &EmailSyncService{ingester: ingester}
}

// Synchronize records each inbox addition and returns the cursor safe to persist.
func (s *EmailSyncService) Synchronize(ctx context.Context, mailbox domain.EmailMailboxReader,
	connection domain.EmailMailboxConnection,
) (string, error) {
	if s.ingester == nil || mailbox == nil || connection.Route.Validate() != nil ||
		connection.Route.ChannelType != domain.ChannelTypeEmail ||
		strings.TrimSpace(connection.HistoryID) == "" || connection.ConnectedAt.IsZero() {
		return "", fmt.Errorf("%w: mailbox connection is incomplete", domain.ErrInvalidInput)
	}
	if _, err := domain.NormalizeInboundChannelIdentifier(domain.ChannelTypeEmail, connection.Address); err != nil {
		return "", err
	}
	if connection.InitialSyncPending {
		return s.fullSync(ctx, mailbox, connection)
	}
	cursor, err := s.history(ctx, mailbox, connection)
	if !errors.Is(err, domain.ErrEmailHistoryExpired) {
		return cursor, err
	}
	return s.fullSync(ctx, mailbox, connection)
}

func (s *EmailSyncService) history(ctx context.Context, mailbox domain.EmailMailboxReader,
	connection domain.EmailMailboxConnection,
) (string, error) {
	var next, cursor string
	seenPages := map[string]bool{}
	seenMessages := map[string]bool{}
	for {
		page, err := mailbox.History(ctx, connection.HistoryID, next)
		if err != nil {
			return "", err
		}
		if err := s.ingestPage(ctx, mailbox, connection, page.MessageIDs, seenMessages); err != nil {
			return "", err
		}
		if page.HistoryID != "" {
			cursor = page.HistoryID
		}
		if page.NextPageToken == "" {
			break
		}
		if seenPages[page.NextPageToken] {
			return "", errors.New("mailbox history repeated a page token")
		}
		seenPages[page.NextPageToken] = true
		next = page.NextPageToken
	}
	if cursor == "" {
		return "", errors.New("mailbox history returned no cursor")
	}
	return cursor, nil
}

func (s *EmailSyncService) fullSync(ctx context.Context, mailbox domain.EmailMailboxReader,
	connection domain.EmailMailboxConnection,
) (string, error) {
	cursor, err := mailbox.CurrentCursor(ctx)
	if err != nil {
		return "", err
	}
	if cursor == "" {
		return "", errors.New("mailbox returned no current cursor")
	}
	var next string
	seenPages := map[string]bool{}
	seenMessages := map[string]bool{}
	for {
		page, err := mailbox.MessagesSince(ctx, connection.ConnectedAt, next)
		if err != nil {
			return "", err
		}
		if err := s.ingestPage(ctx, mailbox, connection, page.MessageIDs, seenMessages); err != nil {
			return "", err
		}
		if page.NextPageToken == "" {
			return cursor, nil
		}
		if seenPages[page.NextPageToken] {
			return "", errors.New("mailbox listing repeated a page token")
		}
		seenPages[page.NextPageToken] = true
		next = page.NextPageToken
	}
}

func (s *EmailSyncService) ingestPage(ctx context.Context, mailbox domain.EmailMailboxReader,
	connection domain.EmailMailboxConnection, ids []string, seen map[string]bool,
) error {
	for _, id := range ids {
		if strings.TrimSpace(id) == "" {
			return errors.New("mailbox page has an empty message ID")
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		message, err := mailbox.ReadMessage(ctx, id)
		if errors.Is(err, domain.ErrEmailMessageGone) {
			continue
		}
		if err != nil {
			return err
		}
		if message.ExternalMessageID != id || message.ProviderReceivedAt.IsZero() {
			return errors.New("mailbox returned an invalid message")
		}
		if message.ProviderReceivedAt.Before(connection.ConnectedAt) {
			continue
		}
		message.DestinationIdentifier = connection.Address
		if _, err := s.ingester.Ingest(ctx, connection.Route, message); err != nil {
			return err
		}
	}
	return nil
}
