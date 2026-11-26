package whatsapp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMetaSignupUsesBusinessTokenForSelectedAssets(t *testing.T) {
	var registered, subscribed bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v25.0/oauth/access_token":
			if r.Method != "GET" || r.URL.Query().Get("client_id") != "app" || r.URL.Query().Get("client_secret") != "secret" || r.URL.Query().Get("code") != "one-use" {
				t.Error("invalid token exchange")
			}
			_, _ = w.Write([]byte(`{"access_token":"business-token"}`))
		case "/v25.0/456/phone_numbers":
			if r.Header.Get("Authorization") != "Bearer business-token" {
				t.Error("missing business token")
			}
			_, _ = w.Write([]byte(`{"data":[{"id":"123","display_phone_number":"+54 9 11 5555-0101","platform_type":"NOT_APPLICABLE","status":"PENDING"},{"id":"124","display_phone_number":"+54 9 11 5555-0102","platform_type":"CLOUD_API","is_on_biz_app":true}]}`))
		case "/v25.0/456/subscribed_apps", "/v25.0/123/register":
			if r.Header.Get("Authorization") != "Bearer business-token" || r.Method != "POST" {
				t.Error("invalid write authorization")
			}
			if strings.HasSuffix(r.URL.Path, "/register") {
				var body map[string]string
				if json.NewDecoder(r.Body).Decode(&body) != nil || body["pin"] != "581063" || body["messaging_product"] != "whatsapp" {
					t.Error("invalid registration payload")
				}
				registered = true
			} else {
				subscribed = true
			}
			_, _ = w.Write([]byte(`{"success":true}`))
		default:
			t.Errorf("unexpected route: %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	adapter := NewMetaSignup(server.Client(), "v25.0", "app", "secret")
	adapter.baseURL = server.URL
	grant, err := adapter.Authorize(t.Context(), "one-use", "456", "123")
	if err != nil || grant.Identifier != "+5491155550101" || !grant.NeedsRegistration {
		t.Fatalf("wrong grant: %v", err)
	}
	existing, err := adapter.Authorize(t.Context(), "one-use", "456", "124")
	if err != nil || existing.NeedsRegistration {
		t.Fatalf("coexistence phone must not be re-registered: %v", err)
	}
	if _, err := adapter.Authorize(t.Context(), "one-use", "456", "999"); err == nil {
		t.Fatal("accepted a phone outside the selected business account")
	}
	if err := adapter.Subscribe(t.Context(), "456", grant.AccessToken); err != nil {
		t.Fatal(err)
	}
	if err := adapter.Register(t.Context(), "123", grant.AccessToken, "581063"); err != nil {
		t.Fatal(err)
	}
	if !registered || !subscribed {
		t.Fatal("missing onboarding steps")
	}
}
