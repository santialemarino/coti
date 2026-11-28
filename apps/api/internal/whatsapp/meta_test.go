package whatsapp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
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

// Outside the customer-service window the quote goes as the approved template: the PDF is uploaded
// first and the template's document header points at the returned media ID.
func TestMetaSender_SendQuote_UploadsThePDFAndSendsTheTemplate(t *testing.T) {
	t.Parallel()
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v25.0/phone-id/media":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Fatalf("parse upload: %v", err)
			}
			file, header, err := r.FormFile("file")
			if err != nil {
				t.Fatalf("upload has no file: %v", err)
			}
			content, _ := io.ReadAll(file)
			if r.FormValue("messaging_product") != "whatsapp" || r.FormValue("type") != "application/pdf" ||
				header.Filename != "Cotizacion-COT-000042.pdf" || string(content) != "%PDF" {
				t.Errorf("upload = %q %q %q %q", r.FormValue("messaging_product"), r.FormValue("type"),
					header.Filename, content)
			}
			_, _ = w.Write([]byte(`{"id":"media-1"}`))
		case "/v25.0/phone-id/messages":
			var body metaTemplateMessageRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			want := metaTemplateMessageRequest{MessagingProduct: "whatsapp", To: "5491112345678", Type: "template",
				Template: metaTemplate{Name: "coti_cotizacion", Language: metaTemplateLanguage{Code: "es_AR"},
					Components: []metaTemplateComponent{
						{Type: "header", Parameters: []metaTemplateParameter{{Type: "document",
							Document: &metaTemplateDocument{ID: "media-1", Filename: "Cotizacion-COT-000042.pdf"}}}},
						{Type: "body", Parameters: []metaTemplateParameter{{Type: "text", Text: "Corralón"},
							{Type: "text", Text: "https://c.test/quotes/tok"}}},
					}}}
			if !reflect.DeepEqual(body, want) {
				t.Errorf("template request = %+v, want %+v", body, want)
			}
			_, _ = w.Write([]byte(`{"messages":[{"id":"wamid.9"}]}`))
		default:
			t.Fatalf("unexpected request %s", r.URL.Path)
		}
	}))
	defer server.Close()

	sender := newMetaSender(server.Client(), server.URL, "v25.0")
	receipt, err := sender.SendQuote(context.Background(), domain.QuoteWhatsAppMessage{
		To: "+5491112345678", Body: "recorded text",
		Credentials: domain.WhatsAppDeliveryCredentials{PhoneNumberID: "phone-id", AccessToken: "access-token"},
		Template: &domain.WhatsAppTemplate{Name: "coti_cotizacion", Language: "es_AR",
			Parameters: []string{"Corralón", "https://c.test/quotes/tok"},
			Document: &domain.WhatsAppDocument{Filename: "Cotizacion-COT-000042.pdf",
				ContentType: "application/pdf", Content: []byte("%PDF")}},
	})
	if err != nil || receipt == nil || receipt.ProviderReference != "wamid.9" {
		t.Fatalf("SendQuote() = %+v, %v; want the template's message ID", receipt, err)
	}
	if len(calls) != 2 || calls[0] != "/v25.0/phone-id/media" {
		t.Errorf("calls = %v, want the upload before the message", calls)
	}
}

// A failed upload sends nothing: a template without its document header would be refused anyway.
func TestMetaSender_SendQuote_StopsWhenTheUploadFails(t *testing.T) {
	t.Parallel()
	var messages int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v25.0/phone-id/messages" {
			messages++
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"code":131053}}`))
	}))
	defer server.Close()

	sender := newMetaSender(server.Client(), server.URL, "v25.0")
	_, err := sender.SendQuote(context.Background(), domain.QuoteWhatsAppMessage{
		To:          "+5491112345678",
		Credentials: domain.WhatsAppDeliveryCredentials{PhoneNumberID: "phone-id", AccessToken: "access-token"},
		Template: &domain.WhatsAppTemplate{Name: "coti_cotizacion", Language: "es_AR",
			Document: &domain.WhatsAppDocument{Filename: "c.pdf", ContentType: "application/pdf"}},
	})
	if err == nil || messages != 0 {
		t.Errorf("SendQuote() = %v after %d message calls, want the upload error and no message", err, messages)
	}
}
