//go:build integration

package storage

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/config"
)

func TestNeonStorage_PrivateBucketRoundTrip(t *testing.T) {
	settings := config.S3Settings{
		Endpoint: os.Getenv("TEST_STORAGE_ENDPOINT"), Region: os.Getenv("TEST_STORAGE_REGION"),
		Bucket: os.Getenv("TEST_STORAGE_BUCKET"), AccessKey: os.Getenv("TEST_STORAGE_ACCESS_KEY"),
		SecretKey: os.Getenv("TEST_STORAGE_SECRET_KEY"),
	}
	if settings.Endpoint == "" || settings.Region == "" || settings.Bucket == "" || settings.AccessKey == "" || settings.SecretKey == "" {
		t.Skip("TEST_STORAGE_* settings are required for a private Neon test bucket")
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	store := NewNeonStorage(settings)
	key := "accounts/" + uuid.NewString() + "/storage-check/" + uuid.NewString() + ".txt"
	want := []byte("Coti private storage integration check")
	if err := store.Upload(ctx, key, "text/plain", bytes.NewReader(want)); err != nil {
		t.Fatalf("upload: %v", err)
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if _, err := store.client.DeleteObject(cleanup, &s3.DeleteObjectInput{Bucket: aws.String(settings.Bucket), Key: aws.String(key)}); err != nil {
			t.Errorf("delete test object: %v", err)
		}
	})
	object, err := store.Download(ctx, key)
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	got, err := io.ReadAll(object.Body)
	_ = object.Body.Close()
	if err != nil || !bytes.Equal(got, want) || object.ContentType != "text/plain" || object.Size != int64(len(want)) {
		t.Fatalf("download changed bytes or metadata: type=%q size=%d error=%v", object.ContentType, object.Size, err)
	}
	t.Log("upload and authenticated download preserve bytes and content type")

	raw, err := store.GenerateSignedURL(ctx, key, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Timeout: 15 * time.Second}
	response, err := client.Get(raw)
	if err != nil {
		t.Fatal("signed download request failed")
	}
	got, err = io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || err != nil || !bytes.Equal(got, want) {
		t.Fatalf("signed download: status=%d error=%v", response.StatusCode, err)
	}
	t.Log("signed download succeeds")

	link, err := url.Parse(raw)
	if err != nil {
		t.Fatal("invalid signed URL")
	}
	unsigned := *link
	unsigned.RawQuery = ""
	tampered := *link
	query := tampered.Query()
	query.Set("X-Amz-Signature", strings.Repeat("0", 64))
	tampered.RawQuery = query.Encode()
	for _, tc := range []struct{ name, target string }{{"unsigned", unsigned.String()}, {"tampered", tampered.String()}} {
		response, err := client.Get(tc.target)
		if err != nil {
			t.Fatalf("%s download request failed", tc.name)
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusForbidden && response.StatusCode != http.StatusUnauthorized {
			t.Fatalf("%s download: status=%d, want access denied", tc.name, response.StatusCode)
		}
		t.Logf("%s download rejected: %d", tc.name, response.StatusCode)
	}
}
