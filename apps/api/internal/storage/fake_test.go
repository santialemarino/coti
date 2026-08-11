package storage

import (
	"io"
	"strings"
	"testing"
	"time"
)

func TestFakeStorageUploadAndDownload(t *testing.T) {
	storage := NewFakeStorage()

	content := "archivo de prueba"
	key := "accounts/1/rfqs/1/test.txt"

	err := storage.Upload(
		nil,
		key,
		strings.NewReader(content),
		"text/plain",
	)
	if err != nil {
		t.Fatalf("upload failed: %v", err)
	}

	reader, err := storage.Download(nil, key)
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}
	defer reader.Close()

	result, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("reading downloaded content failed: %v", err)
	}

	if string(result) != content {
		t.Fatalf("expected %q, got %q", content, string(result))
	}
}

func TestFakeStorageDownloadMissingObject(t *testing.T) {
	storage := NewFakeStorage()

	_, err := storage.Download(nil, "does-not-exist.txt")
	if err == nil {
		t.Fatal("expected error when downloading a missing object")
	}
}

func TestFakeStorageGenerateSignedURL(t *testing.T) {
	storage := NewFakeStorage()

	key := "accounts/1/rfqs/1/test.pdf"

	err := storage.Upload(
		nil,
		key,
		strings.NewReader("contenido"),
		"application/pdf",
	)
	if err != nil {
		t.Fatalf("upload failed: %v", err)
	}

	url, err := storage.GenerateSignedURL(
		nil,
		key,
		15*time.Minute,
	)
	if err != nil {
		t.Fatalf("generating signed URL failed: %v", err)
	}

	if url == "" {
		t.Fatal("expected a signed URL")
	}

	if !strings.HasPrefix(url, "fake://storage/") {
		t.Fatalf("unexpected URL: %s", url)
	}
}
