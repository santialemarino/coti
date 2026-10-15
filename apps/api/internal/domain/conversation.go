package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ErrBatchUnprocessable marks a handler failure no retry can fix, such as a quote archived or
// closed since its batch closed; the batch fails at once.
var ErrBatchUnprocessable = errors.New("conversation batch cannot be processed")

// MessageAuthorType says who wrote a conversation message; only CLIENT messages drive a window.
type MessageAuthorType string

const (
	MessageAuthorClient MessageAuthorType = "CLIENT"
	MessageAuthorSeller MessageAuthorType = "SELLER"
	MessageAuthorSystem MessageAuthorType = "SYSTEM"
)

// MessageBatchStatus is the lifecycle of a conversation window and its place in the queue.
type MessageBatchStatus string

const (
	MessageBatchStatusOpen       MessageBatchStatus = "OPEN"
	MessageBatchStatusClosed     MessageBatchStatus = "CLOSED"
	MessageBatchStatusProcessing MessageBatchStatus = "PROCESSING"
	MessageBatchStatusProcessed  MessageBatchStatus = "PROCESSED"
	MessageBatchStatusFailed     MessageBatchStatus = "FAILED"
)

// MessageBatchFailureReason says why a batch FAILED; only an exhausted one can be retried.
type MessageBatchFailureReason string

const (
	// MessageBatchFailureUnprocessable is a batch no retry can fix, such as one for a closed quote.
	MessageBatchFailureUnprocessable MessageBatchFailureReason = "UNPROCESSABLE"
	// MessageBatchFailureAttemptsExhausted is a batch that kept failing until its attempts ran out.
	MessageBatchFailureAttemptsExhausted MessageBatchFailureReason = "ATTEMPTS_EXHAUSTED"
)

// QuoteMessage is one message in a quote's conversation.
type QuoteMessage struct {
	ID              uuid.UUID
	AccountID       uuid.UUID
	QuoteID         uuid.UUID
	ChannelID       *uuid.UUID
	BatchID         *uuid.UUID
	ClientActionID  *uuid.UUID
	AuthorType      MessageAuthorType
	AuthorUserID    *uuid.UUID
	Body            string
	ReceivedAt      time.Time
	OtherLiveQuotes int
	CreatedAt       time.Time
}

// NewQuoteMessage is a message appended to a quote's conversation. The caller chooses ID.
type NewQuoteMessage struct {
	ID           uuid.UUID
	QuoteID      uuid.UUID
	ChannelID    *uuid.UUID
	AuthorType   MessageAuthorType
	AuthorUserID *uuid.UUID
	Body         string
	ReceivedAt   time.Time
	// OtherLiveQuotes counts the other quotes an inbound sender could have meant.
	OtherLiveQuotes int
}

// MessageBatch is a quote's conversation window and, once closed, its place in the FIFO queue.
type MessageBatch struct {
	ID          uuid.UUID
	AccountID   uuid.UUID
	QuoteID     uuid.UUID
	Status      MessageBatchStatus
	OpenedAt    time.Time
	ClosesAt    time.Time
	ClosedAt    *time.Time
	ProcessedAt *time.Time
	CreatedAt   time.Time
	// Attempts counts the processing attempts that failed or timed out.
	Attempts  int
	LastError *string
	// ProcessingStartedAt dates the latest claim; a write that finishes the claim must match it.
	ProcessingStartedAt *time.Time
	FailureReason       *MessageBatchFailureReason
}

// ConversationSweep is what one pass over the conversation queue did. Reclaimed claims are
// counted apart from the outcomes of the claims this pass made.
type ConversationSweep struct {
	Closed    int
	Reclaimed int
	Claimed   int
	Processed int
	Retried   int
	Failed    int
}

// ConversationCandidate is a quote whose latest live send went to an inbound sender's address.
type ConversationCandidate struct {
	QuoteID     uuid.UUID
	QuoteSendID uuid.UUID
	SentAt      time.Time
}

// ConversationRoute is the quote conversation an inbound message continues.
type ConversationRoute struct {
	QuoteID     uuid.UUID
	QuoteSendID uuid.UUID
	// OtherLiveQuotes counts the other quotes the sender could have meant; above zero the seller
	// should confirm the routing.
	OtherLiveQuotes int
}

// InboundSenderDestinations returns every quote_send.destination form an inbound sender may have
// been sent to on a channel type. A WhatsApp sender is Meta's digits-only wa_id; an Argentine
// mobile is written both with and without the 9 after the country code.
func InboundSenderDestinations(channelType ChannelType, senderID string) ([]string, error) {
	if channelType != ChannelTypeWhatsApp {
		return nil, fmt.Errorf("%w: %s has no inbound conversation routing", ErrInvalidInput, channelType)
	}
	digits := strings.TrimPrefix(strings.TrimSpace(senderID), "+")
	if digits == "" || strings.Trim(digits, "0123456789") != "" {
		return nil, fmt.Errorf("%w: a WhatsApp sender is a digits-only number", ErrInvalidInput)
	}
	forms := []string{"+" + digits}
	if national, mobile := strings.CutPrefix(digits, "549"); mobile && national != "" {
		forms = append(forms, "+54"+national)
	} else if national, argentine := strings.CutPrefix(digits, "54"); argentine && national != "" {
		forms = append(forms, "+549"+national)
	}
	return forms, nil
}

// ConversationBatch is one closed window of a quote's conversation as the seller reviews it.
type ConversationBatch struct {
	Batch    MessageBatch
	Messages []QuoteMessage
	// Decision is the handler's run on the batch, absent until one is recorded.
	Decision *HandlerDecisionReview
	// Retryable is a FAILED batch that ran out of attempts on a quote the handler may still act on.
	Retryable bool
}

// Conversation is a quote's customer messages grouped by the batch that carried them.
type Conversation struct {
	Batches []ConversationBatch
	// Pending are the messages of a window still open, not yet handed to the handler.
	Pending []QuoteMessage
	// SellerMessages are the quote's sends, as the seller's side of the timeline.
	SellerMessages []QuoteMessage
	// CanDiscardAIChanges is a change-requested draft carrying AI changes the seller has not
	// decided on.
	CanDiscardAIChanges bool
}
