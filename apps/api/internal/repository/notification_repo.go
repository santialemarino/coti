package repository

import (
	"context"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// NotificationRepository owns persistence for notification.
type NotificationRepository struct{}

// NewNotificationRepository builds a NotificationRepository.
func NewNotificationRepository() *NotificationRepository {
	return &NotificationRepository{}
}

// Create records one delivery attempt and its outcome.
func (r *NotificationRepository) Create(ctx context.Context, q Querier, n domain.Notification) error {
	_, err := q.Exec(ctx,
		`INSERT INTO notification (account_id, user_id, client_id, quote_id, event, medium, status, sent_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		n.AccountID, n.UserID, n.ClientID, n.QuoteID, n.Event, n.Medium, n.Status, n.SentAt)
	return err
}
