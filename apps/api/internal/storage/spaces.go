package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

var _ domain.ObjectStorage = (*SpacesStorage)(nil)

// SpacesStorage stores objects in an S3-compatible bucket — DigitalOcean Spaces in this
// deployment — and hands out presigned links the provider serves itself, so the API never
// carries the bytes on the way out.
type SpacesStorage struct {
	client    *s3.Client
	presigner *s3.PresignClient
	bucket    string
}

// NewSpacesStorage builds a SpacesStorage against the configured bucket.
func NewSpacesStorage(settings config.SpacesSettings) *SpacesStorage {
	client := s3.New(s3.Options{
		Region:       settings.Region,
		BaseEndpoint: aws.String(settings.Endpoint),
		Credentials: credentials.NewStaticCredentialsProvider(
			settings.AccessKey, settings.SecretKey, ""),
		// Spaces rejects the aws-chunked body the SDK sends when it adds a checksum of its
		// own accord, so ask for one only where the operation requires it.
		RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired,
		ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired,
	})
	return &SpacesStorage{
		client:    client,
		presigner: s3.NewPresignClient(client),
		bucket:    settings.Bucket,
	}
}

// Upload stores the object privately, tagged with the content type it must be served as.
func (s *SpacesStorage) Upload(ctx context.Context, key, contentType string, content io.Reader) error {
	if err := validateKey(key); err != nil {
		return err
	}
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        content,
		ContentType: aws.String(contentType),
		ACL:         types.ObjectCannedACLPrivate,
	})
	if err != nil {
		return fmt.Errorf("put object: %w", err)
	}
	return nil
}

// Download opens the object and reports the content type it was stored with.
func (s *SpacesStorage) Download(ctx context.Context, key string) (*domain.StoredObject, error) {
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
func (s *SpacesStorage) GenerateSignedURL(ctx context.Context, key string, expiresIn time.Duration) (string, error) {
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
