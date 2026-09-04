package domain

import (
	"context"
	"io"
	"time"
)

// StoredObject is an object's bytes together with the metadata the storage kept beside them.
// Body belongs to the caller and must be closed.
type StoredObject struct {
	Body        io.ReadCloser
	ContentType string
	Size        int64
}

// ObjectStorage stores and retrieves files without exposing the underlying provider. The key
// carries the account, so tenant isolation is visible in the object's own path.
type ObjectStorage interface {
	Upload(ctx context.Context, key, contentType string, content io.Reader) error
	Download(ctx context.Context, key string) (*StoredObject, error)
	GenerateSignedURL(ctx context.Context, key string, expiresIn time.Duration) (string, error)
}
