package mail

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	netmail "net/mail"
	"net/smtp"
	"net/textproto"
	"strconv"
	"strings"
	"time"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// SMTPMailer delivers over SMTP, authenticating with the credentials the environment carries.
type SMTPMailer struct {
	cfg config.MailConfig
}

// NewSMTPMailer builds an SMTPMailer from the transport configuration.
func NewSMTPMailer(cfg config.MailConfig) *SMTPMailer {
	return &SMTPMailer{cfg: cfg}
}

// Send composes the message and hands it to the server, giving up when the context ends.
func (m *SMTPMailer) Send(ctx context.Context, msg domain.EmailMessage) error {
	payload, err := m.compose(msg)
	if err != nil {
		return err
	}
	return m.deliver(ctx, msg.To, payload)
}

// compose renders the message: headers as RFC 2047 encoded words and every body
// quoted-printable, so Argentine Spanish survives a hop that assumes ASCII.
func (m *SMTPMailer) compose(msg domain.EmailMessage) ([]byte, error) {
	var body bytes.Buffer
	contentHeaders := []string{"Content-Type: text/plain; charset=utf-8", "Content-Transfer-Encoding: quoted-printable"}
	if msg.HTMLBody == "" {
		// A client shows the last alternative it understands, so an empty HTML part would blank the mail.
		if err := writeQuotedPrintable(&body, msg.TextBody); err != nil {
			return nil, err
		}
	} else {
		parts := multipart.NewWriter(&body)
		// Least capable alternative first: a reader that renders only one takes the last it understands.
		for _, part := range []struct{ contentType, content string }{
			{"text/plain; charset=utf-8", msg.TextBody},
			{"text/html; charset=utf-8", msg.HTMLBody},
		} {
			w, err := parts.CreatePart(textproto.MIMEHeader{
				"Content-Type":              {part.contentType},
				"Content-Transfer-Encoding": {"quoted-printable"},
			})
			if err != nil {
				return nil, fmt.Errorf("compose email: %w", err)
			}
			if err := writeQuotedPrintable(w, part.content); err != nil {
				return nil, err
			}
		}
		if err := parts.Close(); err != nil {
			return nil, fmt.Errorf("compose email: %w", err)
		}
		contentHeaders = []string{`Content-Type: multipart/alternative; boundary="` + parts.Boundary() + `"`}
	}
	if len(msg.Attachments) > 0 {
		mixed, boundary, err := withAttachments(contentHeaders, body.Bytes(), msg.Attachments)
		if err != nil {
			return nil, err
		}
		contentHeaders = []string{`Content-Type: multipart/mixed; boundary="` + boundary + `"`}
		body.Reset()
		body.Write(mixed)
	}

	fromName := m.cfg.FromName
	if msg.FromName != "" {
		fromName = msg.FromName
	}
	headers := []string{
		"From: " + (&netmail.Address{Name: fromName, Address: m.cfg.FromAddress}).String(),
		"To: " + (&netmail.Address{Name: msg.ToName, Address: msg.To}).String(),
		"Subject: " + mime.QEncoding.Encode("utf-8", msg.Subject),
		"Date: " + time.Now().Format(time.RFC1123Z),
		"MIME-Version: 1.0",
	}
	headers = append(headers, contentHeaders...)
	if msg.ReplyTo != "" {
		headers = append(headers,
			"Reply-To: "+(&netmail.Address{Name: msg.ReplyToName, Address: msg.ReplyTo}).String())
	}
	// The ID comes from inbound mail, so anything that could fold or end the header is refused.
	if msg.InReplyTo != "" && !strings.ContainsAny(msg.InReplyTo, "\r\n") {
		headers = append(headers, "In-Reply-To: "+msg.InReplyTo, "References: "+msg.InReplyTo)
	}
	return append([]byte(strings.Join(headers, "\r\n")+"\r\n\r\n"), body.Bytes()...), nil
}

// withAttachments wraps the rendered body as the first part of a multipart/mixed message and adds
// each file after it, base64 in 76-column lines as RFC 2045 requires.
func withAttachments(contentHeaders []string, content []byte, attachments []domain.EmailAttachment) ([]byte, string, error) {
	var result bytes.Buffer
	fail := func(err error) ([]byte, string, error) { return nil, "", fmt.Errorf("compose email: %w", err) }
	parts := multipart.NewWriter(&result)
	header := textproto.MIMEHeader{}
	for _, line := range contentHeaders {
		key, value, _ := strings.Cut(line, ": ")
		header.Add(key, value)
	}
	w, err := parts.CreatePart(header)
	if err != nil {
		return fail(err)
	}
	if _, err := w.Write(content); err != nil {
		return fail(err)
	}
	for _, attachment := range attachments {
		contentType := mime.FormatMediaType(attachment.ContentType, map[string]string{"name": attachment.Filename})
		if contentType == "" {
			contentType = mime.FormatMediaType("application/octet-stream", map[string]string{"name": attachment.Filename})
		}
		w, err := parts.CreatePart(textproto.MIMEHeader{
			"Content-Type":              {contentType},
			"Content-Disposition":       {mime.FormatMediaType("attachment", map[string]string{"filename": attachment.Filename})},
			"Content-Transfer-Encoding": {"base64"},
		})
		if err != nil {
			return fail(err)
		}
		encoded := base64.StdEncoding.EncodeToString(attachment.Content)
		for len(encoded) > 76 {
			if _, err := io.WriteString(w, encoded[:76]+"\r\n"); err != nil {
				return fail(err)
			}
			encoded = encoded[76:]
		}
		if _, err := io.WriteString(w, encoded+"\r\n"); err != nil {
			return fail(err)
		}
	}
	if err := parts.Close(); err != nil {
		return fail(err)
	}
	return result.Bytes(), parts.Boundary(), nil
}

func writeQuotedPrintable(w io.Writer, content string) error {
	encoded := quotedprintable.NewWriter(w)
	if _, err := encoded.Write([]byte(content)); err != nil {
		return fmt.Errorf("compose email: %w", err)
	}
	if err := encoded.Close(); err != nil {
		return fmt.Errorf("compose email: %w", err)
	}
	return nil
}

// deliver runs the SMTP conversation. The connection carries a deadline of its own because
// net/smtp takes no context and offers no way to interrupt an exchange already under way.
func (m *SMTPMailer) deliver(ctx context.Context, to string, payload []byte) error {
	addr := net.JoinHostPort(m.cfg.SMTPHost, strconv.Itoa(m.cfg.SMTPPort))
	conn, err := (&net.Dialer{Timeout: m.cfg.SMTPTimeout}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("dial %s: %w", addr, err)
	}
	defer func() { _ = conn.Close() }()

	// Closing under it is the only thing a cancellation can do to a conversation in flight.
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()

	deadline := time.Now().Add(m.cfg.SMTPTimeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return fmt.Errorf("set smtp deadline: %w", err)
	}

	client, err := smtp.NewClient(conn, m.cfg.SMTPHost)
	if err != nil {
		return fmt.Errorf("smtp greeting: %w", err)
	}
	defer func() { _ = client.Close() }()

	if err := m.secure(client); err != nil {
		return err
	}
	if err := m.authenticate(client); err != nil {
		return err
	}

	if err := client.Mail(m.cfg.FromAddress); err != nil {
		return fmt.Errorf("smtp MAIL FROM: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("smtp RCPT TO: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp DATA: %w", err)
	}
	if _, err := w.Write(payload); err != nil {
		return fmt.Errorf("smtp body: %w", err)
	}
	// Closing the writer is what reads the server's acceptance, so the message is delivered
	// here. A QUIT that then fails is teardown, and reporting it would file a queued message
	// as FAILED in the notification row.
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp body: %w", err)
	}
	_ = client.Quit()
	return nil
}

// secure upgrades the connection when TLS is required. A server that does not offer STARTTLS is
// refused rather than fallen back on: a stripped advertisement is how a message and the password
// behind it end up crossing the network in the clear.
func (m *SMTPMailer) secure(client *smtp.Client) error {
	if !m.cfg.SMTPStartTLS {
		return nil
	}
	if ok, _ := client.Extension("STARTTLS"); !ok {
		return fmt.Errorf("smtp server %s offers no STARTTLS", m.cfg.SMTPHost)
	}
	if err := client.StartTLS(&tls.Config{ServerName: m.cfg.SMTPHost, MinVersion: tls.VersionTLS12}); err != nil {
		return fmt.Errorf("smtp starttls: %w", err)
	}
	return nil
}

// authenticate logs in when the server asks for it — a local sandbox accepting anything does not.
// net/smtp refuses to send the password to an unencrypted non-loopback server on its own.
func (m *SMTPMailer) authenticate(client *smtp.Client) error {
	if ok, _ := client.Extension("AUTH"); !ok {
		return nil
	}
	if err := client.Auth(smtp.PlainAuth("", m.cfg.SMTPUsername, m.cfg.SMTPPassword, m.cfg.SMTPHost)); err != nil {
		return fmt.Errorf("smtp auth: %w", err)
	}
	return nil
}
