package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

var _ domain.ObjectStorage = (*FakeStorage)(nil)

// FakeStorage is an in-memory ObjectStorage. It lives in a test file on purpose: an adapter
// that accepts an upload and stores it nowhere durable must not be reachable from a binary. It
// enforces the same key rules as the real ones, so a test cannot pass on a key they would refuse.
type FakeStorage struct {
	mu      sync.RWMutex
	objects map[string]domain.StoredObject
	bodies  map[string][]byte
}

// NewFakeStorage builds an empty FakeStorage.
func NewFakeStorage() *FakeStorage {
	return &FakeStorage{
		objects: make(map[string]domain.StoredObject),
		bodies:  make(map[string][]byte),
	}
}

// Upload keeps the bytes and the content type in memory.
func (s *FakeStorage) Upload(ctx context.Context, key, contentType string, content io.Reader) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateKey(key); err != nil {
		return err
	}
	data, err := io.ReadAll(content)
	if err != nil {
		return fmt.Errorf("read object: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.bodies[key] = data
	s.objects[key] = domain.StoredObject{ContentType: contentType, Size: int64(len(data))}
	return nil
}

// Download returns what Upload kept, or ErrNotFound.
func (s *FakeStorage) Download(ctx context.Context, key string) (*domain.StoredObject, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateKey(key); err != nil {
		return nil, err
	}
	s.mu.RLock()
	object, ok := s.objects[key]
	data := s.bodies[key]
	s.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("%w: object %s", domain.ErrNotFound, key)
	}
	object.Body = io.NopCloser(bytes.NewReader(data))
	return &object, nil
}

// GenerateSignedURL returns a link shaped like a real one, which nothing serves.
func (s *FakeStorage) GenerateSignedURL(ctx context.Context, key string, expiresIn time.Duration) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := validateKey(key); err != nil {
		return "", err
	}
	s.mu.RLock()
	_, ok := s.objects[key]
	s.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("%w: object %s", domain.ErrNotFound, key)
	}
	return fmt.Sprintf("fake://storage/%s?expires=%d", key, time.Now().Add(expiresIn).Unix()), nil
}

func TestFakeStorage_Upload_RoundTripsBytesAndContentType(t *testing.T) {
	t.Parallel()
	storage := NewFakeStorage()
	const key = "accounts/1/rfqs/1/test.txt"
	const content = "test file"

	if err := storage.Upload(context.Background(), key, "text/plain", strings.NewReader(content)); err != nil {
		t.Fatalf("upload: %v", err)
	}
	object, err := storage.Download(context.Background(), key)
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	defer object.Body.Close()

	got, err := io.ReadAll(object.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(got) != content {
		t.Fatalf("body = %q, want %q", got, content)
	}
	if object.ContentType != "text/plain" {
		t.Fatalf("content type = %q, want %q", object.ContentType, "text/plain")
	}
	if object.Size != int64(len(content)) {
		t.Fatalf("size = %d, want %d", object.Size, len(content))
	}
}

func TestFakeStorage_Download_MissingObjectIsNotFound(t *testing.T) {
	t.Parallel()
	storage := NewFakeStorage()

	if _, err := storage.Download(context.Background(), "does-not-exist.txt"); !isNotFound(err) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestFakeStorage_GenerateSignedURL_MissingObjectIsNotFound(t *testing.T) {
	t.Parallel()
	storage := NewFakeStorage()

	if _, err := storage.GenerateSignedURL(context.Background(), "missing.pdf", time.Minute); !isNotFound(err) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// A double that accepted what the real adapters refuse would let a service test pass on a key
// that fails the moment it reaches either of them.
func TestFakeStorage_RefusesTheSameKeysAsTheRealAdapters(t *testing.T) {
	t.Parallel()
	storage := NewFakeStorage()
	ctx := context.Background()

	for _, tc := range badKeys() {
		if err := storage.Upload(ctx, tc.key, "text/plain", strings.NewReader("x")); !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("Upload(%q) = %v, want ErrInvalidInput", tc.key, err)
		}
	}
}
