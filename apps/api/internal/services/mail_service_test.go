package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// mailTestDB refuses work on a context that is already done, the way a real query would — which
// is what makes the difference between the caller's context and the detached one observable.
type mailTestDB struct{}

func (mailTestDB) InTenantTx(
	ctx context.Context, _ domain.Tenant, fn func(repository.Querier) error,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return fn(nil)
}

type mailTestAccounts struct {
	// onRead stands in for the caller going away mid-flow, at the one point the service has
	// already committed to sending.
	onRead func()
}

func (a mailTestAccounts) GetByID(
	_ context.Context, _ repository.Querier, accountID uuid.UUID,
) (*domain.Account, error) {
	if a.onRead != nil {
		a.onRead()
	}
	return &domain.Account{ID: accountID, Name: "Corralón San Martín", IsActive: true}, nil
}

type mailTestTransport struct {
	sent []domain.EmailMessage
	err  error
}

func (m *mailTestTransport) Send(ctx context.Context, msg domain.EmailMessage) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.sent = append(m.sent, msg)
	return m.err
}

type mailTestNotifications struct {
	recorded []domain.Notification
}

func (n *mailTestNotifications) Create(
	_ context.Context, _ repository.Querier, record domain.Notification,
) error {
	n.recorded = append(n.recorded, record)
	return nil
}

func mailTestOutbound() OutboundMail {
	return OutboundMail{
		AccountID:  uuid.New(),
		Event:      domain.NotificationEventPasswordReset,
		To:         "vendedor@corralon.test",
		ToName:     "Iván Pérez",
		Subject:    "Restablecé tu contraseña",
		Heading:    "Restablecé tu contraseña",
		Paragraphs: []string{"El enlace vence en 1 hora."},
	}
}

// The send is not the caller's request. A client that hangs up after the account is read must
// not abort a delivery already committed to — the token is minted by then, so the alternative is
// a user holding a reset nobody ever mailed them, with no notification row saying so either.
func TestMailService_Send_OutlivesTheRequestThatTriggeredIt(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	transport := &mailTestTransport{}
	notifications := &mailTestNotifications{}
	sentAt := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	svc := NewMailService(mailTestDB{}, transport, notifications,
		mailTestAccounts{onRead: cancel}, func() time.Time { return sentAt })

	if err := svc.Send(ctx, mailTestOutbound()); err != nil {
		t.Fatalf("Send() = %v, want no error", err)
	}
	if len(transport.sent) != 1 {
		t.Fatalf("the transport was handed %d messages, want 1", len(transport.sent))
	}
	if len(notifications.recorded) != 1 {
		t.Fatalf("%d notification rows recorded, want 1", len(notifications.recorded))
	}
	got := notifications.recorded[0]
	if got.Status != domain.NotificationStatusSent {
		t.Errorf("notification status = %q, want %q", got.Status, domain.NotificationStatusSent)
	}
	if got.SentAt == nil || !got.SentAt.Equal(sentAt) {
		t.Errorf("notification sent_at = %v, want %v", got.SentAt, sentAt)
	}
}

// Detached from the transport onward, not throughout: a caller who is already gone before the
// account is even read has bought nothing, and should not cost a send.
func TestMailService_Send_StillRefusesACallerGoneBeforeItStarts(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	transport := &mailTestTransport{}
	notifications := &mailTestNotifications{}
	svc := NewMailService(mailTestDB{}, transport, notifications, mailTestAccounts{}, nil)

	if err := svc.Send(ctx, mailTestOutbound()); !errors.Is(err, context.Canceled) {
		t.Fatalf("Send() = %v, want context.Canceled", err)
	}
	if len(transport.sent) != 0 {
		t.Errorf("the transport was handed %d messages, want none", len(transport.sent))
	}
}

// A bounce is recorded and returned, not swallowed: the caller decides whether its own work
// survives the failure.
func TestMailService_Send_RecordsAFailedDeliveryAndReturnsIt(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("smtp server localhost offers no STARTTLS")
	transport := &mailTestTransport{err: wantErr}
	notifications := &mailTestNotifications{}
	svc := NewMailService(mailTestDB{}, transport, notifications, mailTestAccounts{}, nil)

	if err := svc.Send(context.Background(), mailTestOutbound()); !errors.Is(err, wantErr) {
		t.Fatalf("Send() = %v, want the transport's error", err)
	}
	if len(notifications.recorded) != 1 {
		t.Fatalf("%d notification rows recorded, want 1", len(notifications.recorded))
	}
	got := notifications.recorded[0]
	if got.Status != domain.NotificationStatusFailed {
		t.Errorf("notification status = %q, want %q", got.Status, domain.NotificationStatusFailed)
	}
	if got.SentAt != nil {
		t.Errorf("notification sent_at = %v on a failed delivery, want nil", got.SentAt)
	}
}
