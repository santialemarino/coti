package handler

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/services"
)

type fakeWhatsAppIngestor struct {
	messages []services.WhatsAppInboundTextMessage
	result   services.WhatsAppIngestResult
	err      error
}

func (f *fakeWhatsAppIngestor) IngestTextMessages(
	_ context.Context, messages []services.WhatsAppInboundTextMessage,
) (services.WhatsAppIngestResult, error) {
	f.messages = messages
	return f.result, f.err
}

func TestWhatsAppWebhookHandler_Verify(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewWhatsAppWebhookHandler(config.WhatsAppConfig{
		WebhookVerifyToken: "verify-token",
	}, &fakeWhatsAppIngestor{})

	router := gin.New()
	router.GET("/webhooks/whatsapp", handler.Verify)

	req := httptest.NewRequest(http.MethodGet,
		"/webhooks/whatsapp?hub.mode=subscribe&hub.verify_token=verify-token&hub.challenge=challenge-123",
		nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got := rec.Body.String(); got != "challenge-123" {
		t.Errorf("body = %q, want challenge", got)
	}
}

func TestWhatsAppWebhookHandler_ReceiveRejectsInvalidSignature(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewWhatsAppWebhookHandler(config.WhatsAppConfig{
		AppSecret: "secret",
	}, &fakeWhatsAppIngestor{})

	router := gin.New()
	router.POST("/webhooks/whatsapp", handler.Receive)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/whatsapp", strings.NewReader(`{}`))
	req.Header.Set("X-Hub-Signature-256", "sha256=bad")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestWhatsAppWebhookHandler_ReceiveIngestsTextMessages(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ingestor := &fakeWhatsAppIngestor{result: services.WhatsAppIngestResult{
		Received: 1, CreatedRFQs: 1,
	}}
	handler := NewWhatsAppWebhookHandler(config.WhatsAppConfig{
		AppSecret: "secret",
	}, ingestor)

	router := gin.New()
	router.POST("/webhooks/whatsapp", handler.Receive)

	payload := []byte(`{
	  "entry": [{
	    "changes": [{
	      "field": "messages",
	      "value": {
	        "metadata": { "phone_number_id": "123456789" },
	        "contacts": [{
	          "wa_id": "5491112345678",
	          "profile": { "name": "Test Client" }
	        }],
	        "messages": [{
	          "from": "5491112345678",
	          "id": "wamid.test",
	          "timestamp": "1785614400",
	          "type": "text",
	          "text": { "body": "I need 20 cement bags" }
	        }]
	      }
	    }]
	  }]
	}`)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/whatsapp", strings.NewReader(string(payload)))
	req.Header.Set("X-Hub-Signature-256", testWhatsAppSignature("secret", payload))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if len(ingestor.messages) != 1 {
		t.Fatalf("messages = %d, want 1", len(ingestor.messages))
	}
	got := ingestor.messages[0]
	if got.PhoneNumberID != "123456789" || got.ExternalMessageID != "wamid.test" ||
		got.SenderID != "5491112345678" || got.SenderName != "Test Client" ||
		got.Body != "I need 20 cement bags" {
		t.Fatalf("message = %#v", got)
	}

	var body map[string]int
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response json = %v", err)
	}
	if body["created_rfqs"] != 1 {
		t.Errorf("created_rfqs = %d, want 1", body["created_rfqs"])
	}
}

func testWhatsAppSignature(secret string, raw []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	if _, err := mac.Write(raw); err != nil {
		panic(err)
	}
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}
