package services

import (
	"strings"
	"testing"
)

func TestNewPublicToken(t *testing.T) {
	// 32 bytes base64url-encoded without padding always lands on 43 characters; this pins that
	// the token is unguessable and survives a URL untouched, not just that it decodes.
	t.Run("is a 43 character URL-safe token", func(t *testing.T) {
		for i := 0; i < 50; i++ {
			token, err := newPublicToken()
			if err != nil {
				t.Fatalf("newPublicToken() = %v", err)
			}
			if len(token) != 43 {
				t.Errorf("token length = %d, want 43", len(token))
			}
			if strings.Contains(token, "=") {
				t.Errorf("token %q contains base64 padding", token)
			}
			for _, r := range token {
				if !strings.ContainsRune("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_-", r) {
					t.Errorf("token %q carries the non URL-safe rune %q", token, r)
				}
			}
		}
	})

	// The token is the whole access control on the public page, so a collision is a misplaced quote.
	t.Run("never repeats", func(t *testing.T) {
		seen := make(map[string]bool, 128)
		for i := 0; i < 128; i++ {
			token, err := newPublicToken()
			if err != nil {
				t.Fatal(err)
			}
			if seen[token] {
				t.Errorf("newPublicToken() returned %q twice", token)
			}
			seen[token] = true
		}
	})
}
