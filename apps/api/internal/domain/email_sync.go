package domain

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// EmailMailboxReader reads the provider pages and originals of one authorized mailbox.
type EmailMailboxReader interface {
	CurrentCursor(context.Context) (string, error)
	History(context.Context, string, string) (EmailSyncPage, error)
	MessagesSince(context.Context, time.Time, string) (EmailSyncPage, error)
	ReadMessage(context.Context, string) (InboundEmailMessage, error)
}

// ErrEmailHistoryExpired means the provider requires a full mailbox synchronization.
var ErrEmailHistoryExpired = errors.New("email history is no longer available")

// ErrEmailMessageGone means the provider no longer has a requested message.
var ErrEmailMessageGone = errors.New("email message no longer exists")

// EmailSyncPage is one page of mailbox message identifiers and its synchronization cursor.
type EmailSyncPage struct {
	MessageIDs    []string
	NextPageToken string
	HistoryID     string
}

// GmailMailbox is an encrypted mailbox credential and its durable synchronization state.
type GmailMailbox struct {
	EmailMailboxConnection
	RefreshTokenCiphertext string
	LastSyncedAt           *time.Time
}

// GmailAuthorization is one expiring, single-use OAuth authorization attempt.
type GmailAuthorization struct {
	StateHash          []byte
	AccountID          uuid.UUID
	BranchID           uuid.UUID
	ChannelID          uuid.UUID
	UserID             uuid.UUID
	SessionEpoch       int
	VerifierCiphertext string
	ExpiresAt          time.Time
}

// GmailGrant is the mailbox identity and offline credential obtained from Google.
type GmailGrant struct {
	Address      string
	HistoryID    string
	RefreshToken string
}

// EmailMailboxConnection identifies when a mailbox began supplying incoming messages.
type EmailMailboxConnection struct {
	Route              InboundChannelRoute
	Address            string
	HistoryID          string
	ConnectedAt        time.Time
	InitialSyncPending bool
}
