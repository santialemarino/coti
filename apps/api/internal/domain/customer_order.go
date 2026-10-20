package domain

import (
	"time"

	"github.com/google/uuid"
)

// CustomerOrder is the customer's projection over a request and its published quotation.
type CustomerOrder struct {
	ID            uuid.UUID
	BranchID      uuid.UUID
	BranchName    string
	ReceivedAt    time.Time
	Status        string
	OriginalText  string
	DeliveryToken *string
}
