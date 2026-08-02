package domain

import "context"

// EmailMessage is one rendered outbound message, ready for a transport to deliver.
type EmailMessage struct {
	To       string
	ToName   string
	Subject  string
	TextBody string
	HTMLBody string
}

// Mailer delivers a rendered message. Adapters live in internal/mail and are bound in the
// composition root; nothing below the port knows which provider is behind it.
type Mailer interface {
	Send(ctx context.Context, msg EmailMessage) error
}
