package handler

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/whatsapp"
)

// WhatsAppWebhookService is the verified WhatsApp inbound intake surface.
type WhatsAppWebhookService interface {
	Verify(ctx context.Context, verifyToken string) (bool, error)
	Ingest(ctx context.Context, messages []domain.InboundTextMessage) error
}

// WhatsAppWebhookHandler serves the public Meta verification and message webhook endpoints.
type WhatsAppWebhookHandler struct {
	appSecret string
	maxBytes  int64
	service   WhatsAppWebhookService
}

// NewWhatsAppWebhookHandler builds a WhatsAppWebhookHandler.
func NewWhatsAppWebhookHandler(appSecret string, maxBytes int64,
	service WhatsAppWebhookService,
) *WhatsAppWebhookHandler {
	return &WhatsAppWebhookHandler{appSecret: appSecret, maxBytes: maxBytes, service: service}
}

// Verify answers Meta's callback challenge after matching a configured channel token.
func (h *WhatsAppWebhookHandler) Verify(c *gin.Context) {
	if h.service == nil || strings.TrimSpace(c.Query("hub.mode")) != "subscribe" {
		c.Status(http.StatusBadRequest)
		return
	}
	challenge := c.Query("hub.challenge")
	if challenge == "" {
		c.Status(http.StatusBadRequest)
		return
	}
	verified, err := h.service.Verify(c.Request.Context(), c.Query("hub.verify_token"))
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	if !verified {
		c.Status(http.StatusForbidden)
		return
	}
	c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(challenge))
}

// Receive verifies Meta's HMAC and acknowledges its message delivery after durable intake.
func (h *WhatsAppWebhookHandler) Receive(c *gin.Context) {
	if h.service == nil || strings.TrimSpace(h.appSecret) == "" {
		c.Status(http.StatusServiceUnavailable)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, h.maxBytes)
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			c.Status(http.StatusRequestEntityTooLarge)
			return
		}
		c.Status(http.StatusBadRequest)
		return
	}
	if !whatsapp.VerifySignature(h.appSecret, body, c.GetHeader("X-Hub-Signature-256")) {
		c.Status(http.StatusUnauthorized)
		return
	}
	messages, err := whatsapp.ParseWebhook(body)
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}
	if err := h.service.Ingest(c.Request.Context(), messages); err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	c.Status(http.StatusOK)
}
