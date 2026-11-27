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

// ParseWebhook extracts inbound text messages and reactions from a verified WhatsApp Cloud API payload.
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
				body, reaction := incoming.Text.Body, false
				switch incoming.Type {
				case "text":
				case "reaction":
					body, reaction = incoming.Reaction.Emoji, true
				default:
					continue
				}
				if strings.TrimSpace(body) == "" {
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
					Body:                  body,
					ConversationOnly:      reaction,
					ProviderReceivedAt:    metaWebhookTime(incoming.Timestamp),
					Payload:               messagePayload,
				})
			}
		}
	}
	return messages, nil
}

// ParseStatuses extracts the delivery outcomes Meta reports for messages a branch number sent;
// "sent" only repeats what the send call already recorded, so it is skipped.
func ParseStatuses(body []byte) ([]domain.ProviderDeliveryStatus, error) {
	var payload metaWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode WhatsApp webhook: %w", err)
	}
	if payload.Object != "whatsapp_business_account" {
		return nil, fmt.Errorf("unexpected WhatsApp webhook object")
	}
	statuses := make([]domain.ProviderDeliveryStatus, 0)
	for _, entry := range payload.Entry {
		for _, change := range entry.Changes {
			if change.Field != "messages" {
				continue
			}
			phoneNumberID := strings.TrimSpace(change.Value.Metadata.PhoneNumberID)
			for _, reported := range change.Value.Statuses {
				status, known := metaDeliveryStatuses[reported.Status]
				reference := strings.TrimSpace(reported.ID)
				if !known || reference == "" {
					continue
				}
				if phoneNumberID == "" {
					return nil, fmt.Errorf("WhatsApp webhook status has no phone number ID")
				}
				delivery := domain.ProviderDeliveryStatus{
					DestinationIdentifier: phoneNumberID,
					ProviderReference:     reference,
					Status:                status,
				}
				if len(reported.Errors) > 0 {
					delivery.ErrorCode = strconv.Itoa(reported.Errors[0].Code)
					delivery.OutsideWindow = reported.Errors[0].Code == metaErrorOutsideWindow
				}
				statuses = append(statuses, delivery)
			}
		}
	}
	return statuses, nil
}

// metaErrorOutsideWindow is Meta's refusal of free text sent more than 24 hours after the
// customer's last message.
const metaErrorOutsideWindow = 131047

var metaDeliveryStatuses = map[string]domain.SendTrackingStatus{
	"delivered": domain.SendTrackingStatusDelivered,
	"read":      domain.SendTrackingStatusViewed,
	"failed":    domain.SendTrackingStatusFailed,
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
	Statuses []metaWebhookStatus  `json:"statuses"`
}

type metaWebhookStatus struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
	Errors    []struct {
		Code int `json:"code"`
	} `json:"errors"`
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
	Reaction struct {
		MessageID string `json:"message_id"`
		Emoji     string `json:"emoji"`
	} `json:"reaction"`
}
