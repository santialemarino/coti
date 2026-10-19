package dto

import (
	"github.com/google/uuid"
	"time"
)

// CustomerOrderResponse is an authorized request with its latest published delivery reference.
type CustomerOrderResponse struct {
	ID            uuid.UUID `json:"id"`
	BranchName    string    `json:"branch_name"`
	ReceivedAt    time.Time `json:"received_at"`
	Status        string    `json:"status"`
	OriginalText  string    `json:"original_text"`
	DeliveryToken *string   `json:"delivery_token"`
}
