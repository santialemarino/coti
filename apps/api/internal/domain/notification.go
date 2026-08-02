package domain

import (
	"time"

	"github.com/google/uuid"
)

// NotificationStatus is the delivery outcome recorded for an outbound notification.
type NotificationStatus string

const (
	NotificationStatusPending NotificationStatus = "PENDING"
	NotificationStatusSent    NotificationStatus = "SENT"
	NotificationStatusFailed  NotificationStatus = "FAILED"
)

// NotificationMedium is the transport a notification went out on.
type NotificationMedium string

const NotificationMediumEmail NotificationMedium = "EMAIL"

// NotificationEvent is what the message was about, and is what a delivery report groups by.
type NotificationEvent string

const NotificationEventPasswordReset NotificationEvent = "PASSWORD_RESET"

// Notification is the delivery record of one outbound message. Append-only: SentAt is the
// timestamp of the attempt that succeeded, and stays null on a failure.
type Notification struct {
	ID        uuid.UUID
	AccountID uuid.UUID
	UserID    *uuid.UUID
	ClientID  *uuid.UUID
	QuoteID   *uuid.UUID
	Event     NotificationEvent
	Medium    NotificationMedium
	Status    NotificationStatus
	CreatedAt time.Time
	SentAt    *time.Time
}
