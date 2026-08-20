package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

var _ domain.ObjectStorage = (*LocalStorage)(nil)

// metaSuffix names the file holding an object's content type, which the filesystem has no
// place for and every signed link needs in order to serve the bytes as what they are.
const metaSuffix = ".meta"

// The modes client attachments are written with: readable by the process that stored them and
// by nobody else on the host.
const (
	objectMode os.FileMode = 0o600
	dirMode    os.FileMode = 0o700
)

// LocalStorage keeps objects on the filesystem and serves them through the API's own signed
// link route. It is the default, because a checkout with no object-store credentials has to work.
type LocalStorage struct {
	baseDir string
	linkURL *url.URL
	signer  *URLSigner
}

// NewLocalStorage builds a LocalStorage rooted at baseDir, signing links that point at
// apiBaseURL. baseDir is resolved to an absolute path so containment does not depend on the
// working directory the process happens to have.
func NewLocalStorage(baseDir string, apiBaseURL *url.URL, signer *URLSigner) (*LocalStorage, error) {
	absolute, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, fmt.Errorf("resolve storage directory: %w", err)
	}
	if err := os.MkdirAll(absolute, dirMode); err != nil {
		return nil, fmt.Errorf("create storage directory: %w", err)
	}
	link := *apiBaseURL
	link.Path = strings.TrimSuffix(link.Path, "/") + LinkPath
	return &LocalStorage{baseDir: absolute, linkURL: &link, signer: signer}, nil
}

// Upload writes the object together with the content type it must be served as.
func (s *LocalStorage) Upload(ctx context.Context, key, contentType string, content io.Reader) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path, err := s.objectPath(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		return fmt.Errorf("create storage directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, objectMode)
	if err != nil {
		return fmt.Errorf("create object: %w", err)
	}
	defer file.Close()

	if _, err := io.Copy(file, content); err != nil {
		return fmt.Errorf("write object: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("write object: %w", err)
	}
	if err := os.WriteFile(path+metaSuffix, []byte(contentType), objectMode); err != nil {
		return fmt.Errorf("write object metadata: %w", err)
	}
	return nil
}

// Download opens the object and reports the content type it was stored as.
func (s *LocalStorage) Download(ctx context.Context, key string) (*domain.StoredObject, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path, err := s.objectPath(key)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: object %s", domain.ErrNotFound, key)
		}
		return nil, fmt.Errorf("open object: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("stat object: %w", err)
	}
	contentType, err := os.ReadFile(path + metaSuffix)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		file.Close()
		return nil, fmt.Errorf("read object metadata: %w", err)
	}
	return &domain.StoredObject{
		Body:        file,
		ContentType: string(contentType),
		Size:        info.Size(),
	}, nil
}

// GenerateSignedURL returns a link that serves the object until expiresIn has passed. The
// deadline is signed alongside the key, so neither can be moved without invalidating the other.
func (s *LocalStorage) GenerateSignedURL(ctx context.Context, key string, expiresIn time.Duration) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if expiresIn <= 0 {
		return "", fmt.Errorf("%w: link lifetime must be greater than zero", domain.ErrInvalidInput)
	}
	path, err := s.objectPath(key)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("%w: object %s", domain.ErrNotFound, key)
		}
		return "", fmt.Errorf("stat object: %w", err)
	}
	expiresAt := s.signer.deadline(expiresIn)
	link := *s.linkURL
	link.Path += "/" + key
	link.RawQuery = url.Values{
		"expires":   {strconv.FormatInt(expiresAt.Unix(), 10)},
		"signature": {s.signer.Sign(key, expiresAt)},
	}.Encode()
	return link.String(), nil
}

// Verify reports whether a link's signature covers this key and deadline, and whether the
// deadline has passed. The adapter that signs a link is the one that checks it again.
func (s *LocalStorage) Verify(key string, expiresAt time.Time, signature string) error {
	return s.signer.Verify(key, expiresAt, signature)
}

// objectPath maps a storage key onto a path inside baseDir. Two things are refused, and they
// are different: a key that is not already canonical, because a bucket stores "a/./b" verbatim
// where a filesystem resolves it and the adapters would then disagree about where an object
// lives; and a key that climbs out of baseDir, which path.Clean leaves untouched.
func (s *LocalStorage) objectPath(key string) (string, error) {
	if key == "" {
		return "", fmt.Errorf("%w: storage key is empty", domain.ErrInvalidInput)
	}
	if strings.HasSuffix(key, metaSuffix) {
		return "", fmt.Errorf("%w: storage key may not end in %s", domain.ErrInvalidInput, metaSuffix)
	}
	if path.IsAbs(key) || path.Clean(key) != key {
		return "", fmt.Errorf("%w: storage key is not a canonical relative path: %s",
			domain.ErrInvalidInput, key)
	}
	joined := filepath.Join(s.baseDir, filepath.FromSlash(key))
	relative, err := filepath.Rel(s.baseDir, joined)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("%w: storage key escapes the base directory: %s",
			domain.ErrInvalidInput, key)
	}
	return joined, nil
}
