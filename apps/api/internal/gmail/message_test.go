package gmail

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestParseMessage_PrefersPlainTextAndKeepsAllAttachments(t *testing.T) {
	raw := strings.Join([]string{
		"From: =?UTF-8?Q?Mar=C3=ADa?= <maria@example.com>",
		"Subject: =?UTF-8?Q?Cotizaci=C3=B3n?=", "MIME-Version: 1.0",
		`Content-Type: multipart/mixed; boundary="mixed"`, "",
		"--mixed", `Content-Type: multipart/alternative; boundary="alternative"`, "",
		"--alternative", `Content-Type: text/plain; charset="UTF-8"`,
		"Content-Transfer-Encoding: quoted-printable", "", "20 bolsas de cemento",
		"--alternative", `Content-Type: text/html; charset="UTF-8"`, "",
		"<p>20 bolsas de cemento</p>", "--alternative--",
		"--mixed", "Content-Type: application/pdf", `Content-Disposition: attachment; filename="pedido.pdf"`,
		"Content-Transfer-Encoding: base64", "", "cGRmLWRhdGE=",
		"--mixed", "Content-Type: application/octet-stream", `Content-Disposition: attachment; filename="otros.bin"`,
		"Content-Transfer-Encoding: base64", "", "YWJj", "--mixed--", "",
	}, "\r\n")
	message, err := ParseMessage([]byte(raw), 1024, 20)
	if err != nil {
		t.Fatal(err)
	}
	if message.Sender != "maria@example.com" || message.SenderName != "María" ||
		message.Subject != "Cotización" || message.Text != "20 bolsas de cemento" {
		t.Fatalf("parsed message = %+v", message)
	}
	if len(message.Attachments) != 2 || message.Attachments[0].Filename != "pedido.pdf" ||
		message.Attachments[0].ContentType != "application/pdf" ||
		string(message.Attachments[0].Content) != "pdf-data" ||
		message.Attachments[1].Filename != "otros.bin" || string(message.Attachments[1].Content) != "abc" {
		t.Fatalf("attachments = %+v", message.Attachments)
	}
}

func TestParseMessage_ReadsHTMLWhenPlainTextIsAbsent(t *testing.T) {
	raw := "From: client@example.com\r\nContent-Type: text/html; charset=utf-8\r\n\r\n" +
		"<html><head><style>secret</style></head><body><p>Necesito &amp; cotizar</p><script>evil()</script></body></html>"
	message, err := ParseMessage([]byte(raw), 1024, 5)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(message.Text, "Necesito & cotizar") ||
		strings.Contains(message.Text, "secret") || strings.Contains(message.Text, "evil") {
		t.Fatalf("HTML body = %q", message.Text)
	}
}

func TestParseMessage_UsesHTMLWhenPlainTextIsEmpty(t *testing.T) {
	raw := "Content-Type: multipart/alternative; boundary=b\r\n\r\n" +
		"--b\r\nContent-Type: text/plain\r\n\r\n\r\n" +
		"--b\r\nContent-Type: text/html\r\n\r\nCemento<br/>10 bolsas\r\n--b--\r\n"
	message, err := ParseMessage([]byte(raw), 1024, 5)
	if err != nil || !strings.Contains(message.Text, "Cemento 10 bolsas") {
		t.Fatalf("HTML fallback = %q, error = %v", message.Text, err)
	}
}

func TestParseMessage_RejectsOversizedPartsAndTooManyParts(t *testing.T) {
	raw := "From: c@example.com\r\nContent-Type: text/plain\r\n\r\n" + strings.Repeat("a", 20)
	if _, err := ParseMessage([]byte(raw), 5, 1); err == nil {
		t.Fatal("oversized part must be refused")
	}
	multiple := "Content-Type: multipart/mixed; boundary=b\r\n\r\n--b\r\nContent-Type: text/plain\r\n\r\na\r\n--b\r\nContent-Type: text/plain\r\n\r\nb\r\n--b--\r\n"
	if _, err := ParseMessage([]byte(multiple), 100, 1); err == nil {
		t.Fatal("too many parts must be refused")
	}
}

func TestDecodeRaw_BoundsAndDecodesGmailBase64URL(t *testing.T) {
	content := []byte("From: client@example.com\r\n\r\n20 bolsas")
	encoded := base64.RawURLEncoding.EncodeToString(content)
	decoded, err := DecodeRaw(encoded, int64(len(content)))
	if err != nil || string(decoded) != string(content) {
		t.Fatalf("decoded = %q, %v", decoded, err)
	}
	if _, err := DecodeRaw(encoded, int64(len(content)-1)); err == nil {
		t.Fatal("oversized raw message must be refused")
	}
	if _, err := DecodeRaw("invalid!", 100); err == nil {
		t.Fatal("invalid base64url must be refused")
	}
}
