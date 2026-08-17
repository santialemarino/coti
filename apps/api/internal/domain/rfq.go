package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// RFQStatus is the pre-quote lifecycle state.
type RFQStatus string

const (
	RFQStatusReceived  RFQStatus = "RECEIVED"
	RFQStatusGenerated RFQStatus = "GENERATED"
)

// RFQClarificationIssueType is the blocking ambiguity an RFQ question addresses.
type RFQClarificationIssueType string

const (
	RFQClarificationMissingQuantity       RFQClarificationIssueType = "MISSING_QUANTITY"
	RFQClarificationMissingUnit           RFQClarificationIssueType = "MISSING_UNIT"
	RFQClarificationMissingPresentation   RFQClarificationIssueType = "MISSING_PRESENTATION"
	RFQClarificationAmbiguousDescription  RFQClarificationIssueType = "AMBIGUOUS_DESCRIPTION"
	RFQClarificationAmbiguousCatalogMatch RFQClarificationIssueType = "AMBIGUOUS_CATALOG_MATCH"
)

// RFQClarificationStatus is the seller-controlled lifecycle of a proposed question.
type RFQClarificationStatus string

const (
	RFQClarificationStatusProposed  RFQClarificationStatus = "PROPOSED"
	RFQClarificationStatusApproved  RFQClarificationStatus = "APPROVED"
	RFQClarificationStatusSent      RFQClarificationStatus = "SENT"
	RFQClarificationStatusAnswered  RFQClarificationStatus = "ANSWERED"
	RFQClarificationStatusDismissed RFQClarificationStatus = "DISMISSED"
)

// RFQ is the original request the quote is built from.
type RFQ struct {
	ID          uuid.UUID
	AccountID   uuid.UUID
	BranchID    uuid.UUID
	ClientID    *uuid.UUID
	ChannelID   uuid.UUID
	RawText     *string
	Status      RFQStatus
	WorkType    *string
	ReceivedAt  time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
	ClientLabel *string
}

// NewRFQ is the input for creating an RFQ source record.
type NewRFQ struct {
	BranchID    uuid.UUID
	ClientID    *uuid.UUID
	ChannelID   uuid.UUID
	RawText     *string
	Status      RFQStatus
	WorkType    *string
	ClientLabel *string
}

// RFQStatusChange records an RFQ lifecycle transition.
type RFQStatusChange struct {
	ID             uuid.UUID
	AccountID      uuid.UUID
	RFQID          uuid.UUID
	PreviousStatus *RFQStatus
	NewStatus      RFQStatus
	UserID         *uuid.UUID
	ChangedAt      time.Time
	CreatedAt      time.Time
}

// RFQClarification is a reviewable question proposed for a blocking RFQ ambiguity.
type RFQClarification struct {
	ID                   uuid.UUID
	AccountID            uuid.UUID
	RFQID                uuid.UUID
	QuoteItemID          *uuid.UUID
	IssueType            RFQClarificationIssueType
	RequestedDescription string
	Question             string
	Reason               string
	Status               RFQClarificationStatus
	ApprovedQuestion     *string
	DecidedBy            *uuid.UUID
	DecidedAt            *time.Time
	SentAt               *time.Time
	Answer               *string
	AnsweredAt           *time.Time
	CreatedAt            time.Time
}

// NewRFQClarification is the input for storing an AI-proposed clarification.
type NewRFQClarification struct {
	QuoteItemID          *uuid.UUID
	IssueType            RFQClarificationIssueType
	RequestedDescription string
	Question             string
	Reason               string
}

// ExtractedRFQLine is one schema-forced line item proposed from informal RFQ text.
type ExtractedRFQLine struct {
	RequestedDescription string
	Quantity             decimal.Decimal
	Unit                 *string
	QuantityRationale    *string
}

// ProposedRFQClarification is one blocking question returned by the extractor.
type ProposedRFQClarification struct {
	IssueType            RFQClarificationIssueType
	RequestedDescription string
	Question             string
	Reason               string
}

// RFQExtraction is the schema-forced result proposed from informal RFQ text.
type RFQExtraction struct {
	Lines          []ExtractedRFQLine
	Clarifications []ProposedRFQClarification
}

// RFQExtractor parses informal RFQ text into structured line items.
type RFQExtractor interface {
	Extract(ctx context.Context, raw string) (RFQExtraction, error)
}

// TextRFQDraftInput creates the first reviewable quote draft from plain RFQ text.
type TextRFQDraftInput struct {
	ChannelID   uuid.UUID
	ClientID    *uuid.UUID
	ClientLabel *string
	RawText     string
	WorkType    *string
}

// WhatsAppMockRFQInput simulates one inbound WhatsApp text message in development.
type WhatsAppMockRFQInput struct {
	ChannelID   *uuid.UUID
	From        string
	ProfileName *string
	Text        string
}

// TextRFQDraft is the persisted result of the RFQ text pipeline.
type TextRFQDraft struct {
	RFQ            RFQ
	Quote          *Quote
	Version        *QuoteVersion
	Items          []QuoteItem
	Clarifications []RFQClarification
}
