package gmail

import (
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestOAuth_AuthorizationUsesOfflineReadOnlyScopeAndPKCE(t *testing.T) {
	oauth, err := NewOAuth("client", "secret", "http://localhost:8000/v1/public/gmail/callback",
		time.Second, 1024)
	if err != nil {
		t.Fatal(err)
	}
	url, err := url.Parse(oauth.AuthorizationURL("opaque-state", strings.Repeat("a", 43)))
	if err != nil {
		t.Fatal(err)
	}
	query := url.Query()
	if url.Host != "accounts.google.com" || query.Get("state") != "opaque-state" ||
		query.Get("access_type") != "offline" || query.Get("code_challenge_method") != "S256" ||
		query.Get("code_challenge") == "" || query.Get("scope") != "https://www.googleapis.com/auth/gmail.readonly" {
		t.Fatalf("authorization query = %v", query)
	}
}
