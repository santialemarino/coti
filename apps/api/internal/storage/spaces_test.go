package storage

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/santialemarino/coti/apps/api/internal/config"
)

// spacesSettings are DigitalOcean's own shape: an endpoint with no bucket in it, and the signing
// region Spaces requires whichever datacenter actually holds the Space.
func spacesSettings() config.SpacesSettings {
	return config.SpacesSettings{
		Endpoint:  "https://nyc3.digitaloceanspaces.com",
		Region:    "us-east-1",
		Bucket:    "coti-attachments",
		AccessKey: "DO00EXAMPLEKEY",
		SecretKey: "secret",
	}
}

// Presigning reaches no network, so the one thing that can be verified without a Space is the
// URL itself — and it is where the three DigitalOcean-specific settings would go wrong silently.
func TestSpacesStorage_GenerateSignedURL_AddressesTheBucketTheWaySpacesExpects(t *testing.T) {
	t.Parallel()
	const key = "accounts/11111111/rfqs/22222222/plan.pdf"

	raw, err := NewSpacesStorage(spacesSettings()).GenerateSignedURL(context.Background(), key, 15*time.Minute)
	if err != nil {
		t.Fatalf("presign: %v", err)
	}
	link, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse presigned url: %v", err)
	}

	// The bucket is prepended to the endpoint rather than put in the path, which is why
	// STORAGE_ENDPOINT must not name the bucket itself.
	if link.Host != "coti-attachments.nyc3.digitaloceanspaces.com" {
		t.Errorf("host = %q, want the bucket prepended to the endpoint", link.Host)
	}
	if link.Path != "/"+key {
		t.Errorf("path = %q, want /%s", link.Path, key)
	}
	if link.Scheme != "https" {
		t.Errorf("scheme = %q, want https", link.Scheme)
	}

	query := link.Query()
	// The region reaches the signing scope, and nothing else uses it. A real DigitalOcean
	// region here signs a payload Spaces rejects, which is the failure this pins.
	if got := query.Get("X-Amz-Credential"); !strings.Contains(got, "/us-east-1/s3/aws4_request") {
		t.Errorf("X-Amz-Credential = %q, want the us-east-1 signing scope", got)
	}
	if got := query.Get("X-Amz-Expires"); got != "900" {
		t.Errorf("X-Amz-Expires = %q, want 900 for a 15 minute link", got)
	}
	if got := query.Get("X-Amz-Algorithm"); got != "AWS4-HMAC-SHA256" {
		t.Errorf("X-Amz-Algorithm = %q, want AWS4-HMAC-SHA256", got)
	}
	if query.Get("X-Amz-Signature") == "" {
		t.Error("X-Amz-Signature is empty")
	}
}

// The link carries its own credential, so it must never be built from an ambient one: the
// adapter takes static keys and the AWS shared-config loader is deliberately not a dependency.
func TestSpacesStorage_GenerateSignedURL_SignsWithTheConfiguredKey(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIAAMBIENTKEYSHOULDNOTAPPEAR")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "ambient-secret")

	raw, err := NewSpacesStorage(spacesSettings()).GenerateSignedURL(
		context.Background(), "accounts/a/rfqs/b/plan.pdf", time.Minute)
	if err != nil {
		t.Fatalf("presign: %v", err)
	}
	if !strings.Contains(raw, "DO00EXAMPLEKEY") {
		t.Errorf("presigned url does not carry the configured Spaces key: %s", raw)
	}
	if strings.Contains(raw, "AKIAAMBIENTKEYSHOULDNOTAPPEAR") {
		t.Error("presigned url was signed with an ambient AWS credential")
	}
}

func TestSpacesStorage_GenerateSignedURL_RefusesANonPositiveLifetime(t *testing.T) {
	t.Parallel()

	_, err := NewSpacesStorage(spacesSettings()).GenerateSignedURL(
		context.Background(), "accounts/a/rfqs/b/plan.pdf", 0)
	if err == nil {
		t.Fatal("GenerateSignedURL() = nil error, want an error")
	}
}
