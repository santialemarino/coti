package domain

import (
	"time"

	"github.com/google/uuid"
)

// Client is the account-scoped customer attached to an RFQ and its quote.
type Client struct {
	ID        uuid.UUID
	AccountID uuid.UUID
	Name      *string
	Phone     *string
	Email     *string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewClient is the contact data first captured when a seller sends a quote.
type NewClient struct {
	Name  *string
	Phone string
	Email *string
}

// ClientContact is the contact data enriched immediately before delivery.
type ClientContact struct {
	Phone string
	Email *string
}
