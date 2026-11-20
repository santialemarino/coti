package domain

import (
	"time"

	"github.com/google/uuid"
)

// CustomerOrderStatusCompleted is the customer-visible status of an accepted sale that is closed.
const CustomerOrderStatusCompleted = "COMPLETED"

// CustomerOrder is the customer's projection over a request and its published quotation.
type CustomerOrder struct {
	ID            uuid.UUID
	BranchID      uuid.UUID
	BranchName    string
	ReceivedAt    time.Time
	Status        string
	OriginalText  string
	DeliveryToken *string
	// Closure is what has closed the sale so far; it decides Status and never reaches the customer.
	Closure SaleClosure
}

// CustomerOrderStatus is the status a customer sees: an accepted sale that is complete reads as
// COMPLETED, by the same rule that closes it in the backoffice.
func CustomerOrderStatus(order CustomerOrder) string {
	if order.Status == string(QuoteStatusAccepted) && SaleComplete(QuoteStatusAccepted, order.Closure) {
		return CustomerOrderStatusCompleted
	}
	return order.Status
}
