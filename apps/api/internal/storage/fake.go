package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// FakeStorage is an in-memory implementation of ObjectStorage used for tests
// and local development.
type FakeStorage struct {
	mu      sync.RWMutex
	objects map[string][]byte
}

func NewFakeStorage() *FakeStorage {
	return &FakeStorage{
		objects: make(map[string][]byte),
	}
}

func (s *FakeStorage) Upload(
	ctx context.Context,
	key string,
	content io.Reader,
	contentType string,
) error {
	data, err := io.ReadAll(content)
	if err != nil {
		return fmt.Errorf("read object: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.objects[key] = data

	return nil
}

func (s *FakeStorage) Download(
	ctx context.Context,
	key string,
) (io.ReadCloser, error) {
	s.mu.RLock()
	data, ok := s.objects[key]
	s.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("object not found: %s", key)
	}

	return io.NopCloser(bytes.NewReader(data)), nil
}

func (s *FakeStorage) GenerateSignedURL(
	ctx context.Context,
	key string,
	expires time.Duration,
) (string, error) {
	s.mu.RLock()
	_, ok := s.objects[key]
	s.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("object not found: %s", key)
	}

	return fmt.Sprintf(
		"fake://storage/%s?expires=%d",
		key,
		time.Now().Add(expires).Unix(),
	), nil
}

var _ domain.ObjectStorage = (*FakeStorage)(nil)
