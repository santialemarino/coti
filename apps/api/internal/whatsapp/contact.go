package whatsapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// SendContact sends onboarding text or an approved authentication template for contact verification.
func (s *MetaSender) SendContact(ctx context.Context, message domain.ContactWhatsAppMessage) (*domain.DeliveryReceipt, error) {
	if message.Credentials.PhoneNumberID == "" || message.Credentials.AccessToken == "" {
		return nil, domain.ErrNotConfigured
	}
	if message.Code == "" {
		return s.sendText(ctx, domain.QuoteWhatsAppMessage{To: message.To, Body: message.Body, Credentials: message.Credentials})
	}
	if message.Template == "" || message.Language == "" {
		return nil, fmt.Errorf("%w: a WhatsApp authentication template is required", domain.ErrNotConfigured)
	}
	payload := map[string]any{"messaging_product": "whatsapp", "to": strings.TrimPrefix(message.To, "+"), "type": "template", "template": map[string]any{"name": message.Template, "language": map[string]string{"code": message.Language}, "components": []map[string]any{
		{"type": "body", "parameters": []map[string]string{{"type": "text", "text": message.Code}}},
		{"type": "button", "sub_type": "url", "index": "0", "parameters": []map[string]string{{"type": "text", "text": message.Code}}},
	}}}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	endpoint := s.baseURL + "/" + s.apiVersion + "/" + url.PathEscape(message.Credentials.PhoneNumberID) + "/messages"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+message.Credentials.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	response, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("contact verification delivery: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, readMetaResponseError(response)
	}
	var result metaTextMessageResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return nil, err
	}
	if len(result.Messages) != 1 || result.Messages[0].ID == "" {
		return nil, fmt.Errorf("contact verification delivery was not acknowledged")
	}
	return &domain.DeliveryReceipt{ProviderReference: result.Messages[0].ID}, nil
}
