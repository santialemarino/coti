package storage

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

type LocalStorage struct {
	baseDir string
}

func NewLocalStorage(baseDir string) *LocalStorage {
	return &LocalStorage{
		baseDir: baseDir,
	}
}

func (s *LocalStorage) Upload(
	ctx context.Context,
	key string,
	content io.Reader,
	contentType string,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	path, err := s.objectPath(key)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create storage directory: %w", err)
	}

	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create object: %w", err)
	}
	defer file.Close()

	if _, err := io.Copy(file, content); err != nil {
		return fmt.Errorf("write object: %w", err)
	}

	return nil
}

func (s *LocalStorage) Download(
	ctx context.Context,
	key string,
) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	path, err := s.objectPath(key)
	if err != nil {
		return nil, err
	}

	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("object not found: %s", key)
		}

		return nil, fmt.Errorf("open object: %w", err)
	}

	return file, nil
}

func (s *LocalStorage) GenerateSignedURL(
	ctx context.Context,
	key string,
	expires time.Duration,
) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	if expires <= 0 {
		return "", fmt.Errorf("expiration must be greater than zero")
	}

	path, err := s.objectPath(key)
	if err != nil {
		return "", err
	}

	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("object not found: %s", key)
		}

		return "", fmt.Errorf("stat object: %w", err)
	}

	// En desarrollo local no necesitamos un servidor de archivos real.
	// La URL sirve como referencia al objeto almacenado.
	return fmt.Sprintf(
		"local://storage/%s?expires=%d",
		url.PathEscape(key),
		time.Now().Add(expires).Unix(),
	), nil
}

func (s *LocalStorage) objectPath(key string) (string, error) {
	cleanKey := filepath.Clean(key)

	if cleanKey == "." ||
		cleanKey == ".." ||
		filepath.IsAbs(cleanKey) ||
		cleanKey == ".."+string(os.PathSeparator) {
		return "", fmt.Errorf("invalid storage key: %s", key)
	}

	// Evita traversal del tipo ../../archivo
	if filepath.HasPrefix(cleanKey, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("invalid storage key: %s", key)
	}

	return filepath.Join(s.baseDir, cleanKey), nil
}

var _ domain.ObjectStorage = (*LocalStorage)(nil)
