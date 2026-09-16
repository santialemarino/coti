package domain

import (
	"time"

	"github.com/google/uuid"
)

// ClientActionType is what the customer did with the quote they were sent.
//
// REJECT is always the customer's own button or the seller's own decision — the assistant never
// infers one. Anything that is not an acceptance arrives as a change request instead.
type ClientActionType string

const (
	ClientActionAccept        ClientActionType = "ACCEPT"
	ClientActionReject        ClientActionType = "REJECT"
	ClientActionRequestChange ClientActionType = "REQUEST_CHANGE"
	ClientActionComment       ClientActionType = "COMMENT"
)

// ClientAction is one answer a customer gave to a quote, recorded against the version they were
// looking at and the send it reached them through.
type ClientAction struct {
	ID          uuid.UUID
	AccountID   uuid.UUID
	VersionID   uuid.UUID
	QuoteSendID *uuid.UUID
	QuoteItemID *uuid.UUID
	Type        ClientActionType
	Comment     *string
	CreatedAt   time.Time
}

// NewClientAction is the input for recording one customer answer.
//
// QuoteSendID is what ties the answer to the delivery it came back through. A quote sent twice —
// once by message and again by mail — would otherwise leave its answer with no origin.
type NewClientAction struct {
	VersionID   uuid.UUID
	QuoteSendID *uuid.UUID
	Type        ClientActionType
	Comment     *string
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
