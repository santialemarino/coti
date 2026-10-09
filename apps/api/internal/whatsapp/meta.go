package whatsapp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

const graphAPIBaseURL = "https://graph.facebook.com"

const maxMetaErrorResponseBytes = 8 * 1024

// MetaSender delivers seller-approved quote links through the WhatsApp Cloud API.
type MetaSender struct {
	client                              *http.Client
	baseURL                             string
	apiVersion                          string
	allowArgentineTestRecipientFallback bool
}

// NewMetaSender builds a MetaSender for the requested Graph API version.
func NewMetaSender(client *http.Client, apiVersion string) *MetaSender {
	if client == nil {
		client = http.DefaultClient
	}
	return newMetaSender(client, graphAPIBaseURL, apiVersion)
}

func newMetaSender(client *http.Client, baseURL, apiVersion string) *MetaSender {
	return &MetaSender{client: client, baseURL: strings.TrimRight(baseURL, "/"), apiVersion: apiVersion}
}

// WithArgentineTestRecipientFallback retries Meta's development allow-list representation.
func (s *MetaSender) WithArgentineTestRecipientFallback(enabled bool) *MetaSender {
	s.allowArgentineTestRecipientFallback = enabled
	return s
}

// SendQuote submits one seller-approved quote link and returns Meta's durable message identifier.
func (s *MetaSender) SendQuote(ctx context.Context,
	message domain.QuoteWhatsAppMessage,
) (*domain.DeliveryReceipt, error) {
	if s == nil || s.client == nil || strings.TrimSpace(message.Credentials.PhoneNumberID) == "" ||
		strings.TrimSpace(message.Credentials.AccessToken) == "" {
		return nil, fmt.Errorf("%w: WhatsApp channel credentials are unavailable", domain.ErrNotConfigured)
	}
	receipt, err := s.sendText(ctx, message)
	if err == nil || !s.allowArgentineTestRecipientFallback {
		return receipt, err
	}
	var providerErr *metaResponseError
	if !errors.As(err, &providerErr) || providerErr.StatusCode != http.StatusBadRequest ||
		providerErr.Code != 131030 {
		return nil, err
	}

	fallbackRecipient, ok := argentineTestRecipientFallback(message.To)
	if !ok {
		return nil, err
	}
	message.To = fallbackRecipient
	return s.sendText(ctx, message)
}

func (s *MetaSender) sendText(ctx context.Context,
	message domain.QuoteWhatsAppMessage,
) (*domain.DeliveryReceipt, error) {
	body, err := json.Marshal(metaTextMessageRequest{
		MessagingProduct: "whatsapp",
		To:               strings.TrimPrefix(message.To, "+"),
		Type:             "text",
		Text: metaTextBody{
			PreviewURL: true,
			Body:       message.Body,
		},
	})
	if err != nil {
		return nil, err
	}
	endpoint := s.baseURL + "/" + s.apiVersion + "/" +
		url.PathEscape(message.Credentials.PhoneNumberID) + "/messages"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+message.Credentials.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	response, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("WhatsApp Cloud API request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, readMetaResponseError(response)
	}

	var parsed metaTextMessageResponse
	if err := json.NewDecoder(response.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decode WhatsApp Cloud API response: %w", err)
	}
	if len(parsed.Messages) != 1 || strings.TrimSpace(parsed.Messages[0].ID) == "" {
		return nil, fmt.Errorf("WhatsApp Cloud API response did not acknowledge a message")
	}
	return &domain.DeliveryReceipt{ProviderReference: parsed.Messages[0].ID}, nil
}

// argentineTestRecipientFallback uses the alternate representation Meta's development allow-list
// expects after an inbound wa_id has included Argentina's mobile prefix.
func argentineTestRecipientFallback(phone string) (string, bool) {
	digits := strings.TrimPrefix(strings.TrimSpace(phone), "+")
	if !strings.HasPrefix(digits, "549") || len(digits) < 9 {
		return "", false
	}
	return "+54" + strings.TrimPrefix(digits, "549"), true
}

func readMetaResponseError(response *http.Response) error {
	var payload struct {
		Error struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxMetaErrorResponseBytes))
	if err == nil {
		_ = json.Unmarshal(data, &payload)
	}
	return &metaResponseError{StatusCode: response.StatusCode, Code: payload.Error.Code}
}

type metaResponseError struct {
	StatusCode int
	Code       int
}

func (e *metaResponseError) Error() string {
	if e.Code == 0 {
		return fmt.Sprintf("WhatsApp Cloud API returned status %d", e.StatusCode)
	}
	return fmt.Sprintf("WhatsApp Cloud API returned status %d (code %d)", e.StatusCode, e.Code)
}

type metaTextMessageRequest struct {
	MessagingProduct string       `json:"messaging_product"`
	To               string       `json:"to"`
	Type             string       `json:"type"`
	Text             metaTextBody `json:"text"`
}

type metaTextBody struct {
	PreviewURL bool   `json:"preview_url"`
	Body       string `json:"body"`
}

type metaTextMessageResponse struct {
	Messages []struct {
		ID string `json:"id"`
	} `json:"messages"`
}

var _ domain.QuoteWhatsAppSender = (*MetaSender)(nil)
