package storage

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func badKeys() []struct {
	name string
	key  string
} {
	return []struct {
		name string
		key  string
	}{
		{"climbing", "../escaped.txt"},
		{"climbing through a prefix", "accounts/a/../../../escaped.txt"},
		{"absolute", "/etc/passwd"},
		{"leading slash", "/accounts/a/rfqs/b/plan.pdf"},
		{"dot segment", "accounts/a/./rfqs/b/plan.pdf"},
		{"doubled separator", "accounts/a//rfqs/b/plan.pdf"},
		{"trailing separator", "accounts/a/rfqs/b/"},
		{"empty", ""},
		{"metadata sidecar", "accounts/a/rfqs/b/plan.pdf" + metaSuffix},
	}
}

func TestValidateKey_AcceptsACanonicalAccountScopedKey(t *testing.T) {
	t.Parallel()

	if err := validateKey("accounts/11111111/rfqs/22222222/33333333.pdf"); err != nil {
		t.Fatalf("validateKey() = %v, want no error", err)
	}
}

func TestValidateKey_RefusesAnythingTheAdaptersWouldReadDifferently(t *testing.T) {
	t.Parallel()

	for _, tc := range badKeys() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := validateKey(tc.key); !errors.Is(err, domain.ErrInvalidInput) {
				t.Fatalf("validateKey(%q) = %v, want ErrInvalidInput", tc.key, err)
			}
		})
	}
}

// The rules are the port's, not one adapter's: a key stored through one has to be reachable
// through the other, so the bucket adapter refuses the same keys — before any network call.
func TestSpacesStorage_RefusesTheSameKeysAsTheLocalAdapter(t *testing.T) {
	t.Parallel()
	// An endpoint nothing listens on: the key check runs before any request, so a guard that
	// stopped refusing would fail this on a dead connection instead of reaching a real bucket.
	spaces := NewSpacesStorage(config.SpacesSettings{
		Endpoint:  "http://127.0.0.1:1",
		Region:    "us-east-1",
		Bucket:    "coti-attachments",
		AccessKey: "access",
		SecretKey: "secret",
	})

	// Presigning reaches nothing, so every key is cheap to assert here.
	ctx := context.Background()
	for _, tc := range badKeys() {
		if _, err := spaces.GenerateSignedURL(ctx, tc.key, time.Minute); !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("GenerateSignedURL(%q) = %v, want ErrInvalidInput", tc.key, err)
		}
	}

	// The two that would otherwise send a request get one representative key: a guard that
	// stopped refusing has to fail this quickly rather than spend the suite's budget retrying.
	const climbing = "../escaped.txt"
	if err := spaces.Upload(ctx, climbing, "text/plain", strings.NewReader("x")); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("Upload(%q) = %v, want ErrInvalidInput", climbing, err)
	}
	if _, err := spaces.Download(ctx, climbing); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("Download(%q) = %v, want ErrInvalidInput", climbing, err)
	}
}
