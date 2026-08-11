package domain

import (
	"context"
	"io"
	"time"
)

// ObjectStorage stores and retrieves files without exposing the underlying provider.
type ObjectStorage interface {
	Upload(ctx context.Context, key string, content io.Reader, contentType string) error
	Download(ctx context.Context, key string) (io.ReadCloser, error)
	GenerateSignedURL(ctx context.Context, key string, expires time.Duration) (string, error)
}
