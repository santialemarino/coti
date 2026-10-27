package whatsapp

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// VerifySignature reports whether Meta signed body with the configured app secret.
func VerifySignature(appSecret string, body []byte, signature string) bool {
	if strings.TrimSpace(appSecret) == "" {
		return false
	}
	given, found := strings.CutPrefix(strings.TrimSpace(signature), "sha256=")
	if !found {
		return false
	}
	decoded, err := hex.DecodeString(given)
	if err != nil || len(decoded) != sha256.Size {
		return false
	}
	mac := hmac.New(sha256.New, []byte(appSecret))
	_, _ = mac.Write(body)
	return hmac.Equal(decoded, mac.Sum(nil))
}

// ParseWebhook extracts inbound text messages from a verified WhatsApp Cloud API payload.
func ParseWebhook(body []byte) ([]domain.InboundTextMessage, error) {
	var payload metaWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode WhatsApp webhook: %w", err)
	}
	if payload.Object != "whatsapp_business_account" {
		return nil, fmt.Errorf("unexpected WhatsApp webhook object")
	}

	messages := make([]domain.InboundTextMessage, 0)
	for _, entry := range payload.Entry {
		for _, change := range entry.Changes {
			if change.Field != "messages" {
				continue
			}
			phoneNumberID := strings.TrimSpace(change.Value.Metadata.PhoneNumberID)
			if phoneNumberID == "" {
				return nil, fmt.Errorf("WhatsApp webhook message has no phone number ID")
			}
			profiles := make(map[string]string, len(change.Value.Contacts))
			for _, contact := range change.Value.Contacts {
				profiles[contact.WAID] = strings.TrimSpace(contact.Profile.Name)
			}
			for _, incoming := range change.Value.Messages {
				if incoming.Type != "text" || strings.TrimSpace(incoming.Text.Body) == "" {
					continue
				}
				if strings.TrimSpace(incoming.ID) == "" || strings.TrimSpace(incoming.From) == "" {
					return nil, fmt.Errorf("WhatsApp webhook text message is incomplete")
				}
				messagePayload, err := json.Marshal(struct {
					Metadata metaWebhookMetadata `json:"metadata"`
					Message  metaWebhookMessage  `json:"message"`
				}{Metadata: change.Value.Metadata, Message: incoming})
				if err != nil {
					return nil, err
				}
				var senderLabel *string
				if name := profiles[incoming.From]; name != "" {
					label := name + " (" + incoming.From + ")"
					senderLabel = &label
				}
				messages = append(messages, domain.InboundTextMessage{
					ExternalMessageID:     incoming.ID,
					DestinationIdentifier: phoneNumberID,
					SenderID:              incoming.From,
					SenderLabel:           senderLabel,
					Body:                  incoming.Text.Body,
					ProviderReceivedAt:    metaWebhookTime(incoming.Timestamp),
					Payload:               messagePayload,
				})
			}
		}
	}
	return messages, nil
}

func metaWebhookTime(value string) time.Time {
	seconds, err := strconv.ParseInt(value, 10, 64)
	if err != nil || seconds < 0 {
		return time.Time{}
	}
	return time.Unix(seconds, 0).UTC()
}

type metaWebhookPayload struct {
	Object string             `json:"object"`
	Entry  []metaWebhookEntry `json:"entry"`
}

type metaWebhookEntry struct {
	Changes []metaWebhookChange `json:"changes"`
}

type metaWebhookChange struct {
	Field string           `json:"field"`
	Value metaWebhookValue `json:"value"`
}

type metaWebhookValue struct {
	Metadata metaWebhookMetadata  `json:"metadata"`
	Contacts []metaWebhookContact `json:"contacts"`
	Messages []metaWebhookMessage `json:"messages"`
}

type metaWebhookMetadata struct {
	PhoneNumberID string `json:"phone_number_id"`
}

type metaWebhookContact struct {
	Profile struct {
		Name string `json:"name"`
	} `json:"profile"`
	WAID string `json:"wa_id"`
}

type metaWebhookMessage struct {
	From      string `json:"from"`
	ID        string `json:"id"`
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`
	Text      struct {
		Body string `json:"body"`
	} `json:"text"`
}
