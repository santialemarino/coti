package handler

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/services"
)

type whatsappWebhookIngestor interface {
	IngestTextMessages(
		ctx context.Context, messages []services.WhatsAppInboundTextMessage,
	) (services.WhatsAppIngestResult, error)
}

// WhatsAppWebhookHandler receives WhatsApp Cloud API webhook verification and messages.
type WhatsAppWebhookHandler struct {
	cfg     config.WhatsAppConfig
	ingests whatsappWebhookIngestor
}

// NewWhatsAppWebhookHandler builds a WhatsAppWebhookHandler.
func NewWhatsAppWebhookHandler(
	cfg config.WhatsAppConfig, ingests whatsappWebhookIngestor,
) *WhatsAppWebhookHandler {
	return &WhatsAppWebhookHandler{cfg: cfg, ingests: ingests}
}

// Verify answers Meta's webhook subscription challenge.
//
//	@Summary		Verify WhatsApp webhook
//	@Description	Answers Meta's subscription challenge when the verify token matches.
//	@Tags			whatsapp
//	@Produce		plain
//	@Param			hub.mode			query		string	true	"Subscription mode"
//	@Param			hub.verify_token	query		string	true	"Webhook verify token"
//	@Param			hub.challenge		query		string	true	"Challenge to echo"
//	@Success		200					{string}	string
//	@Failure		403					{object}	dto.ErrorResponse
//	@Router			/v1/public/webhooks/whatsapp [get]
func (h *WhatsAppWebhookHandler) Verify(c *gin.Context) {
	if c.Query("hub.mode") != "subscribe" ||
		h.cfg.WebhookVerifyToken == "" ||
		c.Query("hub.verify_token") != h.cfg.WebhookVerifyToken {
		c.JSON(http.StatusForbidden, dto.ErrorResponse{Error: "forbidden"})
		return
	}
	c.String(http.StatusOK, c.Query("hub.challenge"))
}

// Receive accepts webhook deliveries and persists text messages as RFQs.
//
//	@Summary		Receive WhatsApp webhook
//	@Description	Accepts signed WhatsApp Cloud API deliveries and persists text messages as RFQs.
//	@Tags			whatsapp
//	@Accept			json
//	@Produce		json
//	@Param			X-Hub-Signature-256	header		string	true	"Meta HMAC-SHA256 signature"
//	@Success		200					{object}	dto.WhatsAppWebhookResponse
//	@Failure		400					{object}	dto.ErrorResponse
//	@Failure		401					{object}	dto.ErrorResponse
//	@Failure		503					{object}	dto.ErrorResponse
//	@Router			/v1/public/webhooks/whatsapp [post]
func (h *WhatsAppWebhookHandler) Receive(c *gin.Context) {
	if h.cfg.AppSecret == "" {
		c.JSON(http.StatusServiceUnavailable, dto.ErrorResponse{Error: "whatsapp webhook not configured"})
		return
	}

	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "invalid request body"})
		return
	}
	if err := h.verifySignature(raw, c.GetHeader("X-Hub-Signature-256")); err != nil {
		c.JSON(http.StatusUnauthorized, dto.ErrorResponse{Error: "invalid signature"})
		return
	}

	var payload whatsappWebhookPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:  "invalid request body",
			Detail: err.Error(),
		})
		return
	}

	result, err := h.ingests.IngestTextMessages(c.Request.Context(), payload.textMessages(raw))
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, dto.WhatsAppWebhookResponse{
		Received:    result.Received,
		CreatedRFQs: result.CreatedRFQs,
		Duplicates:  result.Duplicates,
		Ignored:     result.Ignored,
	})
}

func (h *WhatsAppWebhookHandler) verifySignature(raw []byte, signature string) error {
	if signature == "" {
		return domain.ErrUnauthenticated
	}
	mac := hmac.New(sha256.New, []byte(h.cfg.AppSecret))
	if _, err := mac.Write(raw); err != nil {
		return err
	}
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(expected), []byte(signature)) != 1 {
		return domain.ErrUnauthenticated
	}
	return nil
}

type whatsappWebhookPayload struct {
	Entry []whatsappWebhookEntry `json:"entry"`
}

type whatsappWebhookEntry struct {
	Changes []whatsappWebhookChange `json:"changes"`
}

type whatsappWebhookChange struct {
	Field string               `json:"field"`
	Value whatsappWebhookValue `json:"value"`
}

type whatsappWebhookValue struct {
	Contacts []whatsappContact `json:"contacts"`
	Messages []whatsappMessage `json:"messages"`
	Metadata whatsappMetadata  `json:"metadata"`
}

type whatsappContact struct {
	Profile whatsappProfile `json:"profile"`
	WaID    string          `json:"wa_id"`
}

type whatsappProfile struct {
	Name string `json:"name"`
}

type whatsappMetadata struct {
	PhoneNumberID string `json:"phone_number_id"`
}

type whatsappMessage struct {
	From      string        `json:"from"`
	ID        string        `json:"id"`
	Text      *whatsappText `json:"text"`
	Timestamp string        `json:"timestamp"`
	Type      string        `json:"type"`
}

type whatsappText struct {
	Body string `json:"body"`
}

func (p whatsappWebhookPayload) textMessages(raw []byte) []services.WhatsAppInboundTextMessage {
	var messages []services.WhatsAppInboundTextMessage
	for _, entry := range p.Entry {
		for _, change := range entry.Changes {
			if change.Field != "messages" {
				continue
			}
			contacts := whatsappContactNames(change.Value.Contacts)
			for _, msg := range change.Value.Messages {
				if msg.Type != "text" || msg.Text == nil {
					continue
				}
				messages = append(messages, services.WhatsAppInboundTextMessage{
					PhoneNumberID:     change.Value.Metadata.PhoneNumberID,
					ExternalMessageID: msg.ID,
					SenderID:          msg.From,
					SenderName:        contacts[msg.From],
					Body:              msg.Text.Body,
					ReceivedAt:        whatsappTimestamp(msg.Timestamp),
					RawPayload:        json.RawMessage(raw),
				})
			}
		}
	}
	return messages
}

func whatsappContactNames(contacts []whatsappContact) map[string]string {
	names := make(map[string]string, len(contacts))
	for _, contact := range contacts {
		names[contact.WaID] = contact.Profile.Name
	}
	return names
}

func whatsappTimestamp(raw string) time.Time {
	seconds, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return time.Time{}
	}
	if seconds <= 0 {
		return time.Time{}
	}
	return time.Unix(seconds, 0).UTC()
}
