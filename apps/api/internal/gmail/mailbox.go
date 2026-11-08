package gmail

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// Mailbox adapts Gmail's mailbox API to the inbound synchronization service.
type Mailbox struct {
	client       *Client
	maxRawBytes  int64
	maxPartBytes int64
	maxParts     int
}

// NewMailbox binds message limits to an authorized Gmail client.
func NewMailbox(client *Client, maxRawBytes, maxPartBytes int64, maxParts int) (*Mailbox, error) {
	if client == nil || maxRawBytes <= 0 || maxPartBytes <= 0 || maxParts <= 0 {
		return nil, errors.New("gmail mailbox client and message limits are required")
	}
	return &Mailbox{client: client, maxRawBytes: maxRawBytes, maxPartBytes: maxPartBytes,
		maxParts: maxParts}, nil
}

// CurrentCursor returns the mailbox history position before a full synchronization.
func (m *Mailbox) CurrentCursor(ctx context.Context) (string, error) {
	profile, err := m.client.GetProfile(ctx)
	if err != nil {
		return "", err
	}
	if profile.HistoryID == "" {
		return "", errors.New("gmail profile has no history cursor")
	}
	return profile.HistoryID, nil
}

// History returns one page of inbox additions after a saved cursor.
func (m *Mailbox) History(ctx context.Context, cursor, pageToken string) (domain.EmailSyncPage, error) {
	page, err := m.client.ListHistory(ctx, cursor, pageToken)
	if err != nil {
		return domain.EmailSyncPage{}, err
	}
	result := domain.EmailSyncPage{NextPageToken: page.NextPageToken, HistoryID: page.HistoryID}
	for _, entry := range page.History {
		for _, added := range entry.MessagesAdded {
			result.MessageIDs = append(result.MessageIDs, added.Message.ID)
		}
	}
	return result, nil
}

// MessagesSince returns one inbox page for recovering an expired history cursor.
func (m *Mailbox) MessagesSince(ctx context.Context, since time.Time, pageToken string) (domain.EmailSyncPage, error) {
	refs, next, err := m.client.ListMessagesPage(ctx, since, pageToken)
	if err != nil {
		return domain.EmailSyncPage{}, err
	}
	page := domain.EmailSyncPage{NextPageToken: next}
	for _, ref := range refs {
		page.MessageIDs = append(page.MessageIDs, ref.ID)
	}
	return page, nil
}

// ReadMessage retains the original even when its MIME parts cannot be decoded.
func (m *Mailbox) ReadMessage(ctx context.Context, id string) (domain.InboundEmailMessage, error) {
	message, err := m.client.GetMessage(ctx, id, "raw")
	if err != nil {
		return domain.InboundEmailMessage{}, err
	}
	if message.ID != id {
		return domain.InboundEmailMessage{}, errors.New("gmail returned a different message")
	}
	millis, err := strconv.ParseInt(message.InternalDate, 10, 64)
	if err != nil || millis <= 0 {
		return domain.InboundEmailMessage{}, errors.New("gmail message has no valid receipt time")
	}
	original, err := DecodeRaw(message.Raw, m.maxRawBytes)
	if err != nil {
		return domain.InboundEmailMessage{}, fmt.Errorf("decode original email: %w", err)
	}
	result := domain.InboundEmailMessage{ExternalMessageID: id, Original: original,
		ProviderReceivedAt: time.UnixMilli(millis).UTC()}
	parsed, parseErr := ParseMessage(original, m.maxPartBytes, m.maxParts)
	if parseErr != nil {
		return result, nil
	}
	result.SenderID = parsed.Sender
	result.SenderLabel = parsed.SenderName
	result.Subject = parsed.Subject
	result.Body = parsed.Text
	result.InternetMessageID = parsed.MessageID
	result.Automated = parsed.Automated
	for _, part := range parsed.Attachments {
		result.Attachments = append(result.Attachments, domain.InboundEmailAttachment{
			Filename: part.Filename, ContentType: part.ContentType,
			Inline: part.Inline, Content: part.Content,
		})
	}
	return result, nil
}
