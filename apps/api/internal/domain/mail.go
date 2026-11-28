package domain

import "context"

// EmailMessage is one rendered outbound message, ready for a transport to deliver.
type EmailMessage struct {
	To       string
	ToName   string
	Subject  string
	TextBody string
	HTMLBody string
	// ReplyTo is where an answer goes when it should not go to the sender, blank otherwise.
	ReplyTo     string
	ReplyToName string
	// FromName replaces the platform's display name; the sending address never changes.
	FromName string
	// InReplyTo is a validated Message-ID the message threads under, blank otherwise.
	InReplyTo string
	// AutoReplied marks a fixed reply to the client's own mail (RFC 3834), so their responder never answers it.
	AutoReplied bool
	Attachments []EmailAttachment
}

// EmailAttachment is a file delivered with a message, such as a quotation's PDF.
type EmailAttachment struct {
	Filename    string
	ContentType string
	Content     []byte
}

// Mailer delivers a rendered message. Adapters live in internal/mail and are bound in the
// composition root; nothing below the port knows which provider is behind it.
type Mailer interface {
	Send(ctx context.Context, msg EmailMessage) error
}
