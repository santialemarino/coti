package provider

import (
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/storage"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestBind_LocalProviderServesTheLinksItSigns(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	set, err := Bind(config.StorageConfig{
		Provider:      config.StorageProviderLocal,
		Dir:           dir,
		APIBaseURL:    "http://localhost:8000",
		SigningSecret: "a-signing-secret-of-at-least-32-chars",
	}, discardLogger())
	if err != nil {
		t.Fatalf("Bind() = %v, want no error", err)
	}
	local, ok := set.Storage.(*storage.LocalStorage)
	if !ok {
		t.Fatalf("Storage = %T, want *storage.LocalStorage", set.Storage)
	}
	// The router mounts a route for exactly this adapter, so the two fields have to be the
	// same object rather than two adapters over one directory.
	if set.Local != local {
		t.Fatalf("Local = %p, want the bound adapter %p", set.Local, local)
	}
}

// Spaces signs its own links, so there is nothing for the API to serve and nothing to mount.
func TestBind_SpacesProviderLeavesNothingForTheAPIToServe(t *testing.T) {
	t.Parallel()

	set, err := Bind(config.StorageConfig{
		Provider:  config.StorageProviderSpaces,
		Endpoint:  "https://nyc3.digitaloceanspaces.com",
		Region:    "us-east-1",
		Bucket:    "coti-attachments",
		AccessKey: "access",
		SecretKey: "secret",
	}, discardLogger())
	if err != nil {
		t.Fatalf("Bind() = %v, want no error", err)
	}
	if _, ok := set.Storage.(*storage.SpacesStorage); !ok {
		t.Fatalf("Storage = %T, want *storage.SpacesStorage", set.Storage)
	}
	if set.Local != nil {
		t.Fatalf("Local = %v, want nil", set.Local)
	}
}

func TestBind_UnknownProviderIsRefused(t *testing.T) {
	t.Parallel()

	_, err := Bind(config.StorageConfig{Provider: "gcs"}, discardLogger())
	if err == nil {
		t.Fatal("Bind() = nil error, want an error")
	}
	if !strings.Contains(err.Error(), "gcs") {
		t.Errorf("Bind() error does not name the provider; got: %v", err)
	}
}
