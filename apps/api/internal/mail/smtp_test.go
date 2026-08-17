package mail

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	netmail "net/mail"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// The copy is Argentine Spanish, so every one of these has to survive a transport that assumes
// ASCII — a mangled subject is what a recipient sees before they see anything else.
const (
	smtpFromName = "Coti"
	smtpToName   = "Iván Pérez"
	smtpSubject  = "Confirmá tu dirección de correo"
	smtpText     = "Hola Iván: confirmá tu dirección con este único enlace.\nVence en 48 horas.\n"
	smtpHTML     = `<html><body><p>Confirmá tu dirección — vence en 48 horas.</p></body></html>`
)

func TestSMTPMailer_Send_DeliversBothPartsWithEveryHeaderEncoded(t *testing.T) {
	t.Parallel()
	server := startFakeSMTP(t, nil)

	err := NewSMTPMailer(smtpConfig(t, server.addr, false)).Send(context.Background(), domain.EmailMessage{
		To: testTo, ToName: smtpToName, Subject: smtpSubject, TextBody: smtpText, HTMLBody: smtpHTML,
	})
	if err != nil {
		t.Fatalf("Send() = %v, want no error", err)
	}

	got := server.delivered(t)
	if got.from != testFrom {
		t.Errorf("MAIL FROM = %q, want %q", got.from, testFrom)
	}
	if len(got.to) != 1 || got.to[0] != testTo {
		t.Errorf("RCPT TO = %q, want [%q]", got.to, testTo)
	}

	msg, err := netmail.ReadMessage(strings.NewReader(got.data))
	if err != nil {
		t.Fatalf("the adapter wrote no parseable message: %v (%q)", err, got.data)
	}

	// A header a client cannot decode is worse than one it never got: it renders as the raw
	// encoded word, so assert the decoded value rather than the bytes on the wire.
	decoder := new(mime.WordDecoder)
	subject, err := decoder.DecodeHeader(msg.Header.Get("Subject"))
	if err != nil {
		t.Fatalf("Subject does not decode: %v (%q)", err, msg.Header.Get("Subject"))
	}
	if subject != smtpSubject {
		t.Errorf("Subject = %q, want %q", subject, smtpSubject)
	}
	if raw := msg.Header.Get("Subject"); strings.Contains(raw, smtpSubject) {
		t.Errorf("Subject crosses the wire unencoded: %q", raw)
	}

	from, err := netmail.ParseAddress(msg.Header.Get("From"))
	if err != nil {
		t.Fatalf("From does not parse: %v (%q)", err, msg.Header.Get("From"))
	}
	if from.Address != testFrom || from.Name != smtpFromName {
		t.Errorf("From = %q <%s>, want %q <%s>", from.Name, from.Address, smtpFromName, testFrom)
	}
	to, err := netmail.ParseAddress(msg.Header.Get("To"))
	if err != nil {
		t.Fatalf("To does not parse: %v (%q)", err, msg.Header.Get("To"))
	}
	if to.Address != testTo || to.Name != smtpToName {
		t.Errorf("To = %q <%s>, want %q <%s>", to.Name, to.Address, smtpToName, testTo)
	}
	if _, err := netmail.ParseDate(msg.Header.Get("Date")); err != nil {
		t.Errorf("Date does not parse: %v (%q)", err, msg.Header.Get("Date"))
	}

	mediaType, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil {
		t.Fatalf("Content-Type does not parse: %v", err)
	}
	if mediaType != "multipart/alternative" {
		t.Fatalf("Content-Type = %q, want multipart/alternative", mediaType)
	}

	// Both parts, in this order: a reader that renders one alternative takes the last it
	// understands, so a plain-text body listed second is what an HTML client would show.
	wantParts := []struct{ mediaType, body string }{
		{"text/plain", smtpText},
		{"text/html", smtpHTML},
	}
	// NextRawPart, because NextPart hides Content-Transfer-Encoding and decodes the body itself,
	// which would leave the encoding this test is here to check unasserted.
	parts := multipart.NewReader(msg.Body, params["boundary"])
	for _, want := range wantParts {
		part, err := parts.NextRawPart()
		if err != nil {
			t.Fatalf("the message carries no %s part: %v", want.mediaType, err)
		}
		gotType, gotParams, err := mime.ParseMediaType(part.Header.Get("Content-Type"))
		if err != nil {
			t.Fatalf("%s part Content-Type does not parse: %v", want.mediaType, err)
		}
		if gotType != want.mediaType {
			t.Errorf("part Content-Type = %q, want %q", gotType, want.mediaType)
		}
		if charset := strings.ToLower(gotParams["charset"]); charset != "utf-8" {
			t.Errorf("%s part charset = %q, want utf-8", want.mediaType, charset)
		}
		if encoding := part.Header.Get("Content-Transfer-Encoding"); encoding != "quoted-printable" {
			t.Errorf("%s part Content-Transfer-Encoding = %q, want quoted-printable",
				want.mediaType, encoding)
		}
		raw, err := io.ReadAll(part)
		if err != nil {
			t.Fatalf("%s part does not read: %v", want.mediaType, err)
		}
		// Decoding never-encoded UTF-8 returns it unchanged, so the round-trip below would pass
		// on a body the adapter sent raw under a header promising otherwise. Check the wire too.
		if bytes.ContainsRune(raw, 'á') {
			t.Errorf("%s part crosses the wire unencoded: %q", want.mediaType, raw)
		}
		body, err := io.ReadAll(quotedprintable.NewReader(bytes.NewReader(raw)))
		if err != nil {
			t.Fatalf("%s part does not decode: %v", want.mediaType, err)
		}
		// The encoder normalises bare newlines to CRLF, which is what SMTP requires.
		if decoded := strings.ReplaceAll(string(body), "\r\n", "\n"); decoded != want.body {
			t.Errorf("%s part = %q, want %q", want.mediaType, decoded, want.body)
		}
	}
	if _, err := parts.NextRawPart(); !errors.Is(err, io.EOF) {
		t.Errorf("the message carries a third part, want exactly two")
	}
}

func TestSMTPMailer_Send_AuthenticatesWhenTheServerOffersIt(t *testing.T) {
	t.Parallel()
	server := startFakeSMTP(t, []string{"AUTH PLAIN"})

	err := NewSMTPMailer(smtpConfig(t, server.addr, false)).Send(context.Background(), domain.EmailMessage{
		To: testTo, Subject: smtpSubject, TextBody: smtpText, HTMLBody: smtpHTML,
	})
	if err != nil {
		t.Fatalf("Send() = %v, want no error", err)
	}

	got := server.delivered(t)
	if got.credentials != "\x00"+testUsername+"\x00"+testPassword {
		t.Errorf("credentials = %q, want the configured username and password", got.credentials)
	}
}

// A sandbox that asks for nothing gets nothing: sending PLAIN unprompted is how a password
// reaches a server that never wanted it.
func TestSMTPMailer_Send_SkipsAuthWhenTheServerDoesNotOfferIt(t *testing.T) {
	t.Parallel()
	server := startFakeSMTP(t, nil)

	err := NewSMTPMailer(smtpConfig(t, server.addr, false)).Send(context.Background(), domain.EmailMessage{
		To: testTo, Subject: smtpSubject, TextBody: smtpText, HTMLBody: smtpHTML,
	})
	if err != nil {
		t.Fatalf("Send() = %v, want no error", err)
	}
	if got := server.delivered(t); got.credentials != "" {
		t.Errorf("the adapter authenticated against a server offering no AUTH: %q", got.credentials)
	}
}

// The whole point of declaring TLS instead of negotiating it: a server that stops advertising
// STARTTLS must fail the send, not quietly deliver the message in the clear.
func TestSMTPMailer_Send_RefusesAServerWithNoSTARTTLSWhenTLSIsRequired(t *testing.T) {
	t.Parallel()
	server := startFakeSMTP(t, nil)

	err := NewSMTPMailer(smtpConfig(t, server.addr, true)).Send(context.Background(), domain.EmailMessage{
		To: testTo, Subject: smtpSubject, TextBody: smtpText, HTMLBody: smtpHTML,
	})
	if err == nil {
		t.Fatal("Send() = nil, want an error against a server offering no STARTTLS")
	}
	if !strings.Contains(err.Error(), "STARTTLS") {
		t.Errorf("Send() = %v, want the error to name STARTTLS", err)
	}
	if server.tookDelivery() {
		t.Error("the message was delivered anyway, in the clear")
	}
}

func TestSMTPMailer_Send_StopsOnACancelledContext(t *testing.T) {
	t.Parallel()
	server := startFakeSMTP(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := NewSMTPMailer(smtpConfig(t, server.addr, false)).Send(ctx, domain.EmailMessage{
		To: testTo, Subject: smtpSubject, TextBody: smtpText, HTMLBody: smtpHTML,
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Send() = %v, want context.Canceled", err)
	}
}

const (
	testUsername = "coti"
	testPassword = "s3cret"
)

func smtpConfig(t *testing.T, addr string, startTLS bool) config.MailConfig {
	t.Helper()
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("the fake server's address %q does not split: %v", addr, err)
	}
	n, err := strconv.Atoi(port)
	if err != nil {
		t.Fatalf("the fake server's port %q is not a number: %v", port, err)
	}
	return config.MailConfig{
		Provider:     config.MailProviderSMTP,
		FromAddress:  testFrom,
		FromName:     smtpFromName,
		SMTPHost:     host,
		SMTPPort:     n,
		SMTPUsername: testUsername,
		SMTPPassword: testPassword,
		SMTPStartTLS: startTLS,
		SMTPTimeout:  5 * time.Second,
	}
}

// receivedMail is one delivery as the server saw it, envelope included: the addresses SMTP
// routes on are the ones in MAIL FROM and RCPT TO, not the ones in the headers.
type receivedMail struct {
	from        string
	to          []string
	data        string
	credentials string // the decoded AUTH PLAIN payload, empty when the client never authenticated.
}

// fakeSMTP speaks just enough SMTP for a client to deliver one message. A real server is not
// available to a unit test, and neither is one that would report what actually arrived.
type fakeSMTP struct {
	addr       string
	extensions []string
	received   chan receivedMail
}

func startFakeSMTP(t *testing.T, extensions []string) *fakeSMTP {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	server := &fakeSMTP{
		addr:       listener.Addr().String(),
		extensions: extensions,
		received:   make(chan receivedMail, 1),
	}
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		server.converse(conn)
	}()
	return server
}

// delivered returns the one message the server took, failing the test if none arrived.
func (s *fakeSMTP) delivered(t *testing.T) receivedMail {
	t.Helper()
	select {
	case got := <-s.received:
		return got
	case <-time.After(5 * time.Second):
		t.Fatal("no message reached the server")
		return receivedMail{}
	}
}

// tookDelivery reports whether a message arrived, without waiting for one that should not.
func (s *fakeSMTP) tookDelivery() bool {
	select {
	case <-s.received:
		return true
	case <-time.After(100 * time.Millisecond):
		return false
	}
}

func (s *fakeSMTP) converse(conn net.Conn) {
	reader := bufio.NewReader(conn)
	write := func(line string) bool {
		_, err := io.WriteString(conn, line+"\r\n")
		return err == nil
	}
	if !write("220 fake ESMTP") {
		return
	}

	var got receivedMail
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		command := strings.TrimRight(line, "\r\n")
		verb, rest, _ := strings.Cut(command, " ")

		switch strings.ToUpper(verb) {
		case "EHLO":
			// The first line is the server's identity, not an extension — a client that
			// followed this fake in reading it as one would find no extension at all.
			lines := append([]string{"fake"}, s.extensions...)
			for _, line := range lines[:len(lines)-1] {
				if !write("250-" + line) {
					return
				}
			}
			if !write("250 " + lines[len(lines)-1]) {
				return
			}
		case "HELO":
			if !write("250 fake") {
				return
			}
		case "AUTH":
			_, payload, _ := strings.Cut(rest, " ")
			decoded, err := base64.StdEncoding.DecodeString(payload)
			if err != nil {
				if !write("501 bad payload") {
					return
				}
				continue
			}
			got.credentials = string(decoded)
			if !write("235 2.7.0 authenticated") {
				return
			}
		case "MAIL":
			got.from = addressOf(rest)
			if !write("250 2.1.0 ok") {
				return
			}
		case "RCPT":
			got.to = append(got.to, addressOf(rest))
			if !write("250 2.1.5 ok") {
				return
			}
		case "DATA":
			if !write("354 go ahead") {
				return
			}
			var body strings.Builder
			for {
				dataLine, err := reader.ReadString('\n')
				if err != nil {
					return
				}
				if dataLine == ".\r\n" {
					break
				}
				// Undo the transparency dot the client adds to a line that starts with one.
				body.WriteString(strings.TrimPrefix(dataLine, "."))
			}
			got.data = body.String()
			if !write("250 2.0.0 queued") {
				return
			}
			s.received <- got
		case "QUIT":
			write("221 2.0.0 bye")
			return
		default:
			if !write("250 2.0.0 ok") {
				return
			}
		}
	}
}

// addressOf pulls the address out of a `FROM:<addr>` / `TO:<addr>` argument and its parameters.
func addressOf(arg string) string {
	_, rest, _ := strings.Cut(arg, ":")
	rest = strings.TrimSpace(rest)
	if open := strings.Index(rest, "<"); open >= 0 {
		if end := strings.Index(rest[open:], ">"); end >= 0 {
			return rest[open+1 : open+end]
		}
	}
	address, _, _ := strings.Cut(rest, " ")
	return address
}
