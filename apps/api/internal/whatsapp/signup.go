package whatsapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// MetaSignup completes WhatsApp Embedded Signup on the server.
type MetaSignup struct {
	client                             *http.Client
	baseURL, version, appID, appSecret string
}

// NewMetaSignup builds the server-side Graph adapter.
func NewMetaSignup(client *http.Client, version, appID, appSecret string) *MetaSignup {
	return &MetaSignup{client: client, baseURL: graphAPIBaseURL, version: version, appID: appID, appSecret: appSecret}
}

// Authorize exchanges the code and verifies that the selected phone belongs to the granted account.
func (m *MetaSignup) Authorize(ctx context.Context, code, accountID, phoneID string) (domain.WhatsAppSignupGrant, error) {
	var grant domain.WhatsAppSignupGrant
	var token struct {
		AccessToken string `json:"access_token"`
	}
	if err := m.request(ctx, http.MethodGet, "oauth/access_token", url.Values{"client_id": {m.appID}, "client_secret": {m.appSecret}, "code": {code}}, "", nil, &token); err != nil {
		return grant, err
	}
	if token.AccessToken == "" {
		return grant, fmt.Errorf("Meta did not return a WhatsApp business token")
	}

	fields := url.Values{"fields": {"id,display_phone_number,platform_type,status,is_on_biz_app"}, "limit": {"100"}}
	path := url.PathEscape(accountID) + "/phone_numbers"
	for {
		var page struct {
			Data []struct {
				ID            string `json:"id"`
				Phone         string `json:"display_phone_number"`
				Platform      string `json:"platform_type"`
				Status        string `json:"status"`
				OnBusinessApp bool   `json:"is_on_biz_app"`
			} `json:"data"`
			Paging struct {
				Next    string `json:"next"`
				Cursors struct {
					After string `json:"after"`
				} `json:"cursors"`
			} `json:"paging"`
		}
		if err := m.request(ctx, http.MethodGet, path, fields, token.AccessToken, nil, &page); err != nil {
			return grant, err
		}
		for _, phone := range page.Data {
			if phone.ID == phoneID && phone.Phone != "" {
				if phone.OnBusinessApp && phone.Platform != "CLOUD_API" {
					return grant, domain.ErrInvalidInput
				}
				number := "+" + strings.Map(func(r rune) rune {
					if r >= '0' && r <= '9' {
						return r
					}
					return -1
				}, phone.Phone)
				return domain.WhatsAppSignupGrant{Identifier: number, AccessToken: token.AccessToken, NeedsRegistration: phone.Platform != "CLOUD_API" || (!phone.OnBusinessApp && phone.Status != "CONNECTED")}, nil
			}
		}
		if page.Paging.Next == "" || page.Paging.Cursors.After == "" || page.Paging.Cursors.After == fields.Get("after") {
			break
		}
		fields.Set("after", page.Paging.Cursors.After)
	}
	return grant, domain.ErrInvalidInput
}

// Subscribe enables delivery for the selected business account.
func (m *MetaSignup) Subscribe(ctx context.Context, accountID, token string) error {
	return m.accept(ctx, url.PathEscape(accountID)+"/subscribed_apps", token, nil)
}

// Register registers a new Cloud API number using its two-step verification PIN.
func (m *MetaSignup) Register(ctx context.Context, phoneID, token, pin string) error {
	return m.accept(ctx, url.PathEscape(phoneID)+"/register", token, map[string]string{"messaging_product": "whatsapp", "pin": pin})
}

func (m *MetaSignup) accept(ctx context.Context, path, token string, body any) error {
	var result struct {
		Success bool `json:"success"`
	}
	if err := m.request(ctx, http.MethodPost, path, nil, token, body, &result); err != nil {
		return err
	}
	if !result.Success {
		return fmt.Errorf("Meta did not confirm the WhatsApp signup operation")
	}
	return nil
}

func (m *MetaSignup) request(ctx context.Context, method, path string, query url.Values, token string, payload, result any) error {
	endpoint := m.baseURL + "/" + m.version + "/" + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return fmt.Errorf("build Meta signup request")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	response, err := m.client.Do(req)
	if err != nil {
		return fmt.Errorf("Meta signup request failed")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var failure struct {
			Error struct {
				Code int `json:"code"`
			} `json:"error"`
		}
		_ = json.NewDecoder(io.LimitReader(response.Body, maxMetaErrorResponseBytes)).Decode(&failure)
		return fmt.Errorf("Meta signup HTTP %d, code %d", response.StatusCode, failure.Error.Code)
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 256*1024)).Decode(result); err != nil {
		return fmt.Errorf("decode Meta signup response")
	}
	return nil
}
