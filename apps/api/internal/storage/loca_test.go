package storage

import (
	"context"
	"io"
	"strings"
	"testing"
)

func TestLocalStorageUploadAndDownload(t *testing.T) {
	dir := t.TempDir()

	storage := NewLocalStorage(dir)

	content := "archivo de prueba"
	key := "accounts/1/rfqs/1/test.txt"

	err := storage.Upload(
		context.Background(),
		key,
		strings.NewReader(content),
		"text/plain",
	)
	if err != nil {
		t.Fatalf("upload failed: %v", err)
	}

	reader, err := storage.Download(
		context.Background(),
		key,
	)
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}
	defer reader.Close()

	result, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("reading downloaded content failed: %v", err)
	}

	if string(result) != content {
		t.Fatalf(
			"expected %q, got %q",
			content,
			string(result),
		)
	}
}
