// Package mail holds the adapters behind the domain.Mailer port. Which one is in use is a
// startup decision the composition root makes; nothing above the port knows.
package mail

import (
	"context"
	"log/slog"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// ConsoleMailer writes the message to the log instead of delivering it.
type ConsoleMailer struct {
	log  *slog.Logger
	from string
}

// NewConsoleMailer builds a ConsoleMailer sending as the given address.
func NewConsoleMailer(log *slog.Logger, from string) *ConsoleMailer {
	return &ConsoleMailer{log: log, from: from}
}

// Send logs the message, body included: with no provider behind it, the log is the only
// place a recovery link can be read.
func (m *ConsoleMailer) Send(ctx context.Context, msg domain.EmailMessage) error {
	m.log.InfoContext(ctx, "outbound email",
		slog.String("from", m.from),
		slog.String("to", msg.To),
		slog.String("subject", msg.Subject),
		slog.String("body", msg.TextBody))
	return nil
}
