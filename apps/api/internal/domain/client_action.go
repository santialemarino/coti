package domain

import (
	"time"

	"github.com/google/uuid"
)

// ClientActionType is the durable intent of one customer response to a published quote.
//
// REJECT is always the customer's own button or the seller's own decision — the assistant never
// infers one. Anything that is not an acceptance arrives as a change request instead.
type ClientActionType string

const (
	// ClientActionAccept is the customer's acceptance of the frozen version.
	ClientActionAccept ClientActionType = "ACCEPT"
	// ClientActionRequestChange is a customer request to revise the frozen version.
	ClientActionRequestChange ClientActionType = "REQUEST_CHANGE"
	// ClientActionReject is the customer's explicit rejection of the frozen version.
	ClientActionReject ClientActionType = "REJECT"
	// ClientActionComment is an internal annotation, never submitted from the public page.
	ClientActionComment ClientActionType = "COMMENT"
)

// ClientAction is one persisted customer response, frozen against the exact version it answers.
type ClientAction struct {
	ID        uuid.UUID
	AccountID uuid.UUID
	// QuoteSendID is nil on internal annotations that did not come from a public link.
	QuoteSendID *uuid.UUID
	VersionID   uuid.UUID
	// QuoteItemID is the line a per-item answer refers to, nil on whole-quote answers.
	QuoteItemID *uuid.UUID
	// VersionNumber is the version the action answered, filled when listed through a quote.
	VersionNumber int
	Type          ClientActionType
	Comment       *string
	CreatedAt     time.Time
}

// NewClientAction is a validated customer response ready for its insert.
//
// QuoteSendID is what ties the answer to the delivery it came back through. A quote sent twice —
// once by message and again by mail — would otherwise leave its answer with no origin.
type NewClientAction struct {
	ID          uuid.UUID
	QuoteSendID *uuid.UUID
	VersionID   uuid.UUID
	Type        ClientActionType
	Comment     *string
}

// ClientActionInput is the public-page response the service validates before persisting it.
type ClientActionInput struct {
	Type    ClientActionType
	Message *string
}

// PublicQuoteActionResult reports the answer the customer gave and the state it left the quote in.
type PublicQuoteActionResult struct {
	CustomerStatus ClientActionType
	CreatedAt      time.Time
	QuoteStatus    QuoteStatus
}

// ClientQuoteOutcome is the quote's state after a customer answered, for the caller that has to
// report it without holding the whole quote.
type ClientQuoteOutcome struct {
	QuoteID   uuid.UUID
	Reference string
	Status    QuoteStatus
	Action    ClientActionType
	// SellerID is nil on a quote nobody has taken, which has nobody to notify.
	SellerID *uuid.UUID
}
