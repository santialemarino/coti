package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ClientOrigin records where the account first met a client.
type ClientOrigin string

const (
	ClientOriginWhatsApp ClientOrigin = "WHATSAPP"
	ClientOriginEmail    ClientOrigin = "EMAIL"
	ClientOriginWebApp   ClientOrigin = "WEBAPP"
	ClientOriginPhone    ClientOrigin = "PHONE"
	ClientOriginWalkIn   ClientOrigin = "WALK_IN"
)

// Client is the account-scoped customer attached to an RFQ and its quote.
type Client struct {
	ID            uuid.UUID
	AccountID     uuid.UUID
	Name          *string
	Phone         *string
	Email         *string
	OriginChannel *ClientOrigin
	Notes         *string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// NewClient is the profile data a seller confirms before associating a sale.
type NewClient struct {
	Name          *string
	Phone         *string
	Email         *string
	OriginChannel *ClientOrigin
}

// ClientTag is a reusable account-scoped label assigned to client profiles.
type ClientTag struct {
	ID        uuid.UUID
	AccountID uuid.UUID
	Name      string
	Color     *string
	CreatedAt time.Time
}

// ClientSummary is one client row with sales activity in the caller's branch reach.
type ClientSummary struct {
	Client
	Tags               []ClientTag
	AcceptedQuoteCount int
	LastAcceptedAt     *time.Time
}

// ClientSale is one currently accepted quote associated with a client.
type ClientSale struct {
	QuoteID    uuid.UUID
	RFQID      uuid.UUID
	Number     int64
	BranchID   uuid.UUID
	BranchName string
	Total      decimal.Decimal
	AcceptedAt time.Time
}

// ClientProfile joins a client, their tags, and accepted sales visible to the caller.
type ClientProfile struct {
	Client Client
	Tags   []ClientTag
	Sales  []ClientSale
}

// ClientContactHints are the latest destinations used to deliver a quote.
type ClientContactHints struct {
	Phone *string
	Email *string
}

// ClientMatch is a manually reviewable client candidate with its current tags.
type ClientMatch struct {
	Client Client
	Tags   []ClientTag
}

// QuoteClientAssociation is the context needed to confirm a sale's client profile.
type QuoteClientAssociation struct {
	CurrentClient *ClientMatch
	Suggestions   []ClientMatch
	ContactHints  ClientContactHints
	AvailableTags []ClientTag
}

// AssociateQuoteClientInput selects an existing profile or creates one, then replaces its tags.
type AssociateQuoteClientInput struct {
	ClientID *uuid.UUID
	New      *NewClient
	TagIDs   []uuid.UUID
}
