package whatsapp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func TestMetaSender_SendQuote_PostsTextAndReturnsProviderReference(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v25.0/phone-id/messages" {
			t.Fatalf("request = %s %s, want POST /v25.0/phone-id/messages", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer access-token" {
			t.Errorf("authorization = %q, want bearer token", got)
		}
		var body metaTextMessageRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body.MessagingProduct != "whatsapp" || body.To != "5491112345678" ||
			body.Type != "text" || !body.Text.PreviewURL || body.Text.Body != "Your quote is ready" {
			t.Errorf("body = %#v, want WhatsApp text request", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"messages":[{"id":"wamid.123"}]}`))
	}))
	defer server.Close()

	sender := newMetaSender(server.Client(), server.URL, "v25.0")
	receipt, err := sender.SendQuote(context.Background(), domain.QuoteWhatsAppMessage{
		DeliveryID: uuid.New(), To: "+5491112345678", Body: "Your quote is ready",
		Credentials: domain.WhatsAppDeliveryCredentials{PhoneNumberID: "phone-id", AccessToken: "access-token"},
	})
	if err != nil {
		t.Fatalf("SendQuote() = %v, want no error", err)
	}
	if receipt == nil || receipt.ProviderReference != "wamid.123" {
		t.Errorf("receipt = %#v, want Meta message ID", receipt)
	}
}

func TestMetaSender_SendQuote_RefusesUnacknowledgedResponse(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"messages":[]}`))
	}))
	defer server.Close()

	sender := newMetaSender(&http.Client{Timeout: time.Second}, server.URL, "v25.0")
	_, err := sender.SendQuote(context.Background(), domain.QuoteWhatsAppMessage{
		To: "+5491112345678", Body: "message",
		Credentials: domain.WhatsAppDeliveryCredentials{PhoneNumberID: "phone-id", AccessToken: "access-token"},
	})
	if err == nil {
		t.Fatal("SendQuote() = nil, want an unacknowledged response error")
	}
}

func TestMetaSender_SendQuote_RetriesArgentineTestRecipientWithoutMobilePrefix(t *testing.T) {
	t.Parallel()
	var recipients []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body metaTextMessageRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		recipients = append(recipients, body.To)
		if len(recipients) == 1 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"code":131030}}`))
			return
		}
		_, _ = w.Write([]byte(`{"messages":[{"id":"wamid.fallback"}]}`))
	}))
	defer server.Close()

	sender := newMetaSender(server.Client(), server.URL, "v25.0").
		WithArgentineTestRecipientFallback(true)
	receipt, err := sender.SendQuote(context.Background(), domain.QuoteWhatsAppMessage{
		To: "+5491112345678", Body: "Your quote is ready",
		Credentials: domain.WhatsAppDeliveryCredentials{PhoneNumberID: "phone-id", AccessToken: "access-token"},
	})
	if err != nil {
		t.Fatalf("SendQuote() = %v, want no error", err)
	}
	if receipt == nil || receipt.ProviderReference != "wamid.fallback" {
		t.Errorf("receipt = %#v, want fallback message ID", receipt)
	}
	if len(recipients) != 2 || recipients[0] != "5491112345678" || recipients[1] != "541112345678" {
		t.Errorf("recipients = %v, want canonical then test fallback", recipients)
	}
}

func TestMetaSender_SendQuote_DoesNotRetryArgentineFallbackOutsideDevelopment(t *testing.T) {
	t.Parallel()
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"code":131030}}`))
	}))
	defer server.Close()

	sender := newMetaSender(server.Client(), server.URL, "v25.0")
	_, err := sender.SendQuote(context.Background(), domain.QuoteWhatsAppMessage{
		To: "+5491112345678", Body: "Your quote is ready",
		Credentials: domain.WhatsAppDeliveryCredentials{PhoneNumberID: "phone-id", AccessToken: "access-token"},
	})
	if err == nil {
		t.Fatal("SendQuote() = nil, want Meta error")
	}
	if calls != 1 {
		t.Errorf("calls = %d, want one outside development", calls)
	}
}
