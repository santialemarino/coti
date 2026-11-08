package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

var _ domain.ObjectStorage = (*S3Storage)(nil)

// S3Storage stores objects in a remote bucket and generates temporary download links.
type S3Storage struct {
	client    *s3.Client
	presigner *s3.PresignClient
	bucket    string
	acl       types.ObjectCannedACL
}

// NewNeonStorage uses path-style addressing and the bucket's configured access policy.
func NewNeonStorage(settings config.S3Settings) *S3Storage {
	return newS3Storage(settings, true, "")
}

// NewSpacesStorage stores objects with a private ACL on DigitalOcean Spaces.
func NewSpacesStorage(settings config.S3Settings) *S3Storage {
	return newS3Storage(settings, false, types.ObjectCannedACLPrivate)
}

func newS3Storage(settings config.S3Settings, pathStyle bool, acl types.ObjectCannedACL) *S3Storage {
	client := s3.New(s3.Options{
		Region:       settings.Region,
		BaseEndpoint: aws.String(settings.Endpoint),
		UsePathStyle: pathStyle,
		Credentials: credentials.NewStaticCredentialsProvider(
			settings.AccessKey, settings.SecretKey, ""),
		// Spaces rejects the aws-chunked body the SDK sends when it adds a checksum of its
		// own accord, so ask for one only where the operation requires it.
		RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired,
		ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired,
	})
	return &S3Storage{
		client:    client,
		presigner: s3.NewPresignClient(client),
		bucket:    settings.Bucket,
		acl:       acl,
	}
}

// Upload stores bytes with their content type and the configured access policy.
func (s *S3Storage) Upload(ctx context.Context, key, contentType string, content io.Reader) error {
	if err := validateKey(key); err != nil {
		return err
	}
	body, cleanup, err := seekableUpload(content)
	if err != nil {
		return err
	}
	defer cleanup()
	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        body,
		ContentType: aws.String(contentType),
		ACL:         s.acl,
	})
	if err != nil {
		return fmt.Errorf("put object: %w", err)
	}
	return nil
}

// Download opens the object and reports the content type it was stored with.
func (s *S3Storage) Download(ctx context.Context, key string) (*domain.StoredObject, error) {
	if err := validateKey(key); err != nil {
		return nil, err
	}
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var missing *types.NoSuchKey
		if errors.As(err, &missing) {
			return nil, fmt.Errorf("%w: object %s", domain.ErrNotFound, key)
		}
		return nil, fmt.Errorf("get object: %w", err)
	}
	return &domain.StoredObject{
		Body:        out.Body,
		ContentType: aws.ToString(out.ContentType),
		Size:        aws.ToInt64(out.ContentLength),
	}, nil
}

// GenerateSignedURL returns a presigned link the bucket serves until expiresIn has passed.
func (s *S3Storage) GenerateSignedURL(ctx context.Context, key string, expiresIn time.Duration) (string, error) {
	if err := validateKey(key); err != nil {
		return "", err
	}
	if expiresIn <= 0 {
		return "", fmt.Errorf("%w: link lifetime must be greater than zero", domain.ErrInvalidInput)
	}
	request, err := s.presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(expiresIn))
	if err != nil {
		return "", fmt.Errorf("presign object: %w", err)
	}
	return request.URL, nil
}

func seekableUpload(content io.Reader) (io.ReadSeeker, func(), error) {
	if body, ok := content.(io.ReadSeeker); ok {
		return body, func() {}, nil
	}
	// S3 needs a known length and a rewindable body for retries; spool streams off-heap.
	file, err := os.CreateTemp("", "coti-upload-*")
	if err != nil {
		return nil, nil, fmt.Errorf("prepare upload: %w", err)
	}
	cleanup := func() {
		_ = file.Close()
		_ = os.Remove(file.Name())
	}
	if _, err := io.Copy(file, content); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("buffer upload: %w", err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("rewind upload: %w", err)
	}
	return file, cleanup, nil
}
