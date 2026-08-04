package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// With no provider bound, the log is the only place a recovery link can be read — so what the
// adapter writes is the delivery, and a dropped field means a link nobody can reach.

const (
	testFrom    = "no-reply@coti.test"
	testTo      = "vendedor@corralon.test"
	testSubject = "Recuperá tu contraseña"
	testBody    = "https://backoffice.test/reset-password?token=abc123"
)

func TestConsoleMailer_Send_LogsEveryFieldOfTheMessage(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	mailer := NewConsoleMailer(slog.New(slog.NewJSONHandler(&out, nil)), testFrom)

	if err := mailer.Send(context.Background(), domain.EmailMessage{
		To: testTo, Subject: testSubject, TextBody: testBody,
	}); err != nil {
		t.Fatalf("Send() = %v, want no error", err)
	}

	var logged map[string]any
	if err := json.Unmarshal(out.Bytes(), &logged); err != nil {
		t.Fatalf("the adapter wrote no parseable record: %v (%q)", err, out.String())
	}
	for field, want := range map[string]string{
		"from":    testFrom,
		"to":      testTo,
		"subject": testSubject,
		"body":    testBody,
	} {
		got, ok := logged[field].(string)
		if !ok {
			t.Errorf("the record carries no %q field: %v", field, logged)
			continue
		}
		if got != want {
			t.Errorf("%q = %q, want %q", field, got, want)
		}
	}
}

// The sender comes from configuration, not from the message, so two mailers must not share one.
func TestConsoleMailer_Send_UsesItsOwnSender(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	mailer := NewConsoleMailer(slog.New(slog.NewTextHandler(&out, nil)), "otro@coti.test")

	if err := mailer.Send(context.Background(), domain.EmailMessage{To: testTo}); err != nil {
		t.Fatalf("Send() = %v, want no error", err)
	}
	if !strings.Contains(out.String(), "otro@coti.test") {
		t.Errorf("the record does not carry the configured sender: %q", out.String())
	}
}
