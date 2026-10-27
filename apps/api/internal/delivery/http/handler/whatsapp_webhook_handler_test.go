package handler

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

type stubWhatsAppWebhookService struct {
	verified bool
	token    string
	messages []domain.InboundTextMessage
	intakes  int
}

func (s *stubWhatsAppWebhookService) Verify(_ context.Context, token string) (bool, error) {
	s.token = token
	return s.verified, nil
}

func (s *stubWhatsAppWebhookService) Ingest(_ context.Context,
	messages []domain.InboundTextMessage,
) error {
	s.intakes++
	s.messages = messages
	return nil
}

func TestWhatsAppWebhookHandler_VerifyReturnsMetaChallenge(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &stubWhatsAppWebhookService{verified: true}
	router := gin.New()
	handler := NewWhatsAppWebhookHandler("app-secret", 1024, service)
	router.GET("/webhook", handler.Verify)

	request := httptest.NewRequest(http.MethodGet,
		"/webhook?hub.mode=subscribe&hub.verify_token=verify-token&hub.challenge=challenge", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK || response.Body.String() != "challenge" {
		t.Errorf("response = %d %q, want 200 challenge", response.Code, response.Body.String())
	}
	if service.token != "verify-token" {
		t.Errorf("verify token = %q, want supplied token", service.token)
	}
}

func TestWhatsAppWebhookHandler_ReceiveRejectsAnInvalidSignature(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &stubWhatsAppWebhookService{}
	router := gin.New()
	handler := NewWhatsAppWebhookHandler("app-secret", 1024, service)
	router.POST("/webhook", handler.Receive)

	request := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewBufferString(`{}`))
	request.Header.Set("X-Hub-Signature-256", "sha256=0000")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized || len(service.messages) != 0 {
		t.Errorf("response/messages = %d/%d, want 401/no intake", response.Code, len(service.messages))
	}
}

func TestWhatsAppWebhookHandler_ReceiveRejectsUnsafeDeliveriesBeforeIntake(t *testing.T) {
	gin.SetMode(gin.TestMode)
	original := []byte(`{"object":"whatsapp_business_account","entry":[]}`)
	mac := hmac.New(sha256.New, []byte("app-secret"))
	_, _ = mac.Write(original)
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	for _, test := range []struct {
		name     string
		secret   string
		body     []byte
		limit    int64
		wantCode int
	}{
		{"tampered body", "app-secret", []byte(`{"object":"whatsapp_business_account","entry":[],"changed":true}`), 1024, http.StatusUnauthorized},
		{"missing app secret", "", original, 1024, http.StatusServiceUnavailable},
		{"body exceeds limit", "app-secret", original, 10, http.StatusRequestEntityTooLarge},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := &stubWhatsAppWebhookService{}
			router := gin.New()
			router.POST("/webhook", NewWhatsAppWebhookHandler(test.secret, test.limit, service).Receive)
			request := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(test.body))
			request.Header.Set("X-Hub-Signature-256", signature)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			if response.Code != test.wantCode || service.intakes != 0 {
				t.Errorf("status/intakes = %d/%d, want %d/0", response.Code, service.intakes, test.wantCode)
			}
		})
	}
}

func TestWhatsAppWebhookHandler_VerifyRejectsUnknownToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &stubWhatsAppWebhookService{}
	router := gin.New()
	router.GET("/webhook", NewWhatsAppWebhookHandler("app-secret", 1024, service).Verify)
	request := httptest.NewRequest(http.MethodGet,
		"/webhook?hub.mode=subscribe&hub.verify_token=unknown&hub.challenge=challenge", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden || response.Body.String() == "challenge" {
		t.Errorf("response = %d %q, want 403 without the challenge", response.Code, response.Body.String())
	}
}

func TestWhatsAppWebhookHandler_ReceiveVerifiesAndIngestsText(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &stubWhatsAppWebhookService{}
	router := gin.New()
	handler := NewWhatsAppWebhookHandler("app-secret", 4096, service)
	router.POST("/webhook", handler.Receive)
	body := []byte(`{"object":"whatsapp_business_account","entry":[{"changes":[{"field":"messages","value":{"metadata":{"phone_number_id":"phone-id"},"messages":[{"from":"5491112345678","id":"wamid.inbound","timestamp":"1760000000","type":"text","text":{"body":"cemento"}}]}}]}]}`)
	mac := hmac.New(sha256.New, []byte("app-secret"))
	_, _ = mac.Write(body)
	request := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	request.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK || len(service.messages) != 1 {
		t.Errorf("response/messages = %d/%d, want 200/one text", response.Code, len(service.messages))
	}
}
