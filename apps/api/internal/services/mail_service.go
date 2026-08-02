package services

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// OutboundMail is a message to render, deliver and record: a heading, some paragraphs, and at
// most one call to action.
type OutboundMail struct {
	AccountID   uuid.UUID
	UserID      *uuid.UUID
	Event       domain.NotificationEvent
	To          string
	ToName      string
	Subject     string
	Heading     string
	Paragraphs  []string
	ActionLabel string
	ActionURL   string
}

// mailAccountReader supplies the brand the templates render with.
type mailAccountReader interface {
	GetByID(ctx context.Context, q repository.Querier, accountID uuid.UUID) (*domain.Account, error)
}

// notificationRepository records what went out and how it ended.
type notificationRepository interface {
	Create(ctx context.Context, q repository.Querier, n domain.Notification) error
}

// MailService renders an outbound message with the account's brand, hands it to the
// transport, and records the attempt in notification.
type MailService struct {
	db            tenantTxRunner
	mailer        domain.Mailer
	notifications notificationRepository
	accounts      mailAccountReader
	now           func() time.Time
}

// NewMailService builds a MailService. now is injectable so the recorded timestamp is
// deterministic in tests.
func NewMailService(
	db tenantTxRunner, mailer domain.Mailer, notifications notificationRepository,
	accounts mailAccountReader, now func() time.Time,
) *MailService {
	if now == nil {
		now = time.Now
	}
	return &MailService{db: db, mailer: mailer, notifications: notifications,
		accounts: accounts, now: now}
}

// Send delivers one message, records the transport's outcome either way, and returns that
// outcome so a caller whose own operation must survive a bounce can log it and carry on.
func (s *MailService) Send(ctx context.Context, out OutboundMail) error {
	tenant := domain.Tenant{AccountID: out.AccountID}

	var account *domain.Account
	if err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var err error
		account, err = s.accounts.GetByID(ctx, q, out.AccountID)
		return err
	}); err != nil {
		return err
	}

	msg, err := renderEmail(*account, out)
	if err != nil {
		return err
	}

	// Outside the transaction: the transport is off-process and can take as long as it likes.
	sendErr := s.mailer.Send(ctx, msg)

	record := domain.Notification{
		AccountID: out.AccountID,
		UserID:    out.UserID,
		Event:     out.Event,
		Medium:    domain.NotificationMediumEmail,
		Status:    domain.NotificationStatusSent,
	}
	if sendErr != nil {
		record.Status = domain.NotificationStatusFailed
	} else {
		sentAt := s.now()
		record.SentAt = &sentAt
	}
	// Joined: a failed record must not make a failed send look fine, nor the reverse.
	recordErr := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		return s.notifications.Create(ctx, q, record)
	})
	return errors.Join(sendErr, recordErr)
}
