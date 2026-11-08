package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalizeSubdomain(t *testing.T) {
	for _, raw := range []string{"marko", " Marko ", "a", "corralon-12", strings.Repeat("a", 63)} {
		got, err := NormalizeSubdomain(raw)
		if err != nil || got != strings.ToLower(strings.TrimSpace(raw)) {
			t.Errorf("NormalizeSubdomain(%q) = %q, %v", raw, got, err)
		}
	}
	for _, raw := range []string{"", "www", " API ", "admin", "coti", "-marko", "marko-", "mark_o", "markó", "marko.localhost", "https://marko", "a/b", strings.Repeat("a", 64)} {
		if _, err := NormalizeSubdomain(raw); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("accepted %q: %v", raw, err)
		}
	}
}
