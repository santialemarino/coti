package gmail

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/mail"
	"strings"

	_ "github.com/emersion/go-message/charset"
	mimemail "github.com/emersion/go-message/mail"
	"golang.org/x/net/html"
)

// maxMessageIDLength bounds an echoed Message-ID well inside RFC 5322's 998-octet line limit.
const maxMessageIDLength = 250

// ParsedMessage is the readable material retained from one original email.
type ParsedMessage struct {
	Sender      string
	SenderName  string
	Subject     string
	Text        string
	Attachments []Attachment
	// MessageID is the Message-ID header with its angle brackets, blank when absent or malformed.
	MessageID string
	// Automated marks mail no automatic reply may answer (RFC 3834 and mailing-list headers).
	Automated bool
}

// Attachment holds a decoded MIME part and its original name and media type.
type Attachment struct {
	Filename    string
	ContentType string
	Inline      bool
	Content     []byte
}

// DecodeRaw decodes the base64url RFC 5322 message Gmail returns in raw format.
func DecodeRaw(raw string, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 || int64(len(raw)) > (maxBytes+2)/3*4 {
		return nil, errors.New("gmail message exceeds the configured limit")
	}
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(raw, "="))
	if err != nil {
		return nil, errors.New("gmail message has invalid base64url encoding")
	}
	if int64(len(decoded)) > maxBytes {
		return nil, errors.New("gmail message exceeds the configured limit")
	}
	return decoded, nil
}

// ParseMessage decodes the sender, body and attachments without changing the original bytes.
func ParseMessage(raw []byte, maxPartBytes int64, maxParts int) (ParsedMessage, error) {
	var result ParsedMessage
	if maxPartBytes <= 0 || maxParts <= 0 {
		return result, errors.New("email part limits are required")
	}
	reader, err := mimemail.CreateReader(bytes.NewReader(raw))
	if err != nil {
		return result, fmt.Errorf("read email headers: %w", err)
	}
	defer reader.Close()
	from, err := reader.Header.AddressList("From")
	if err == nil && len(from) == 1 {
		if address, parseErr := mail.ParseAddress(from[0].Address); parseErr == nil &&
			address.Address == from[0].Address {
			result.Sender = address.Address
			result.SenderName = from[0].Name
		}
	}
	if subject, subjectErr := reader.Header.Subject(); subjectErr == nil {
		result.Subject = subject
	}
	if id, idErr := reader.Header.MessageID(); idErr == nil && validMessageID(id) {
		result.MessageID = "<" + id + ">"
	}
	result.Automated = automatedMail(reader.Header.Get("Auto-Submitted"), reader.Header.Get("Precedence"),
		reader.Header.Get("List-Id") != "" || reader.Header.Get("List-Unsubscribe") != "")
	var plain, rich []string
	for count := 0; ; count++ {
		part, nextErr := reader.NextPart()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			return ParsedMessage{}, fmt.Errorf("read email part: %w", nextErr)
		}
		if count >= maxParts {
			return ParsedMessage{}, errors.New("email has too many MIME parts")
		}
		data, readErr := io.ReadAll(io.LimitReader(part.Body, maxPartBytes+1))
		if readErr != nil {
			return ParsedMessage{}, fmt.Errorf("read email part: %w", readErr)
		}
		if int64(len(data)) > maxPartBytes {
			return ParsedMessage{}, errors.New("email part exceeds the configured limit")
		}
		switch header := part.Header.(type) {
		case *mimemail.InlineHeader:
			contentType, _, typeErr := header.ContentType()
			if typeErr != nil {
				return ParsedMessage{}, fmt.Errorf("read email part type: %w", typeErr)
			}
			switch strings.ToLower(contentType) {
			case "text/plain":
				plain = append(plain, string(data))
			case "text/html":
				rich = append(rich, htmlText(data))
			default:
				result.Attachments = append(result.Attachments, Attachment{
					ContentType: contentType, Inline: true, Content: data})
			}
		case *mimemail.AttachmentHeader:
			contentType, _, typeErr := header.ContentType()
			if typeErr != nil {
				return ParsedMessage{}, fmt.Errorf("read attachment type: %w", typeErr)
			}
			filename, nameErr := header.Filename()
			if nameErr != nil {
				return ParsedMessage{}, fmt.Errorf("read attachment name: %w", nameErr)
			}
			disposition, _, dispositionErr := header.ContentDisposition()
			if dispositionErr != nil {
				return ParsedMessage{}, fmt.Errorf("read attachment disposition: %w", dispositionErr)
			}
			result.Attachments = append(result.Attachments, Attachment{
				Filename: filename, ContentType: contentType,
				Inline: strings.EqualFold(disposition, "inline"), Content: data,
			})
		}
	}
	result.Text = strings.TrimSpace(strings.Join(plain, "\n\n"))
	if result.Text == "" {
		result.Text = strings.TrimSpace(strings.Join(rich, "\n\n"))
	}
	return result, nil
}

// validMessageID keeps a header value safe to echo in In-Reply-To: one token, no folding.
func validMessageID(id string) bool {
	if len(id) == 0 || len(id) > maxMessageIDLength || !strings.Contains(id, "@") {
		return false
	}
	return !strings.ContainsFunc(id, func(r rune) bool {
		return r <= ' ' || r >= 0x7f || r == '<' || r == '>'
	})
}

func automatedMail(autoSubmitted, precedence string, list bool) bool {
	autoSubmitted = strings.ToLower(strings.TrimSpace(autoSubmitted))
	switch strings.ToLower(strings.TrimSpace(precedence)) {
	case "bulk", "list", "junk":
		return true
	}
	return list || (autoSubmitted != "" && autoSubmitted != "no")
}

func htmlText(data []byte) string {
	tokenizer := html.NewTokenizer(bytes.NewReader(data))
	var result strings.Builder
	skip := 0
	for {
		token := tokenizer.Next()
		switch token {
		case html.ErrorToken:
			return strings.TrimSpace(result.String())
		case html.SelfClosingTagToken:
			if skip == 0 {
				result.WriteByte(' ')
			}
		case html.StartTagToken:
			name, _ := tokenizer.TagName()
			if strings.EqualFold(string(name), "script") || strings.EqualFold(string(name), "style") ||
				strings.EqualFold(string(name), "head") {
				skip++
			}
			if skip == 0 {
				result.WriteByte(' ')
			}
		case html.EndTagToken:
			name, _ := tokenizer.TagName()
			if skip > 0 && (strings.EqualFold(string(name), "script") ||
				strings.EqualFold(string(name), "style") || strings.EqualFold(string(name), "head")) {
				skip--
			}
			if skip == 0 {
				result.WriteByte(' ')
			}
		case html.TextToken:
			if skip == 0 {
				result.Write(tokenizer.Text())
			}
		}
	}
}
