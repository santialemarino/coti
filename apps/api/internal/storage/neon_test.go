package storage

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func TestS3Storage_StreamedUploadCanRetryWithoutChangingContent(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	t.Setenv("TMP", os.Getenv("TMPDIR"))
	content := "private streamed content"
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		body, err := io.ReadAll(r.Body)
		if err != nil || string(body) != content || r.ContentLength != int64(len(content)) {
			t.Errorf("attempt %d: body=%q length=%d error=%v", attempts, body, r.ContentLength, err)
		}
		if attempts == 1 {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, "<Error><Code>SlowDown</Code></Error>")
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	store := NewNeonStorage(config.S3Settings{Endpoint: server.URL, Region: "us-east-1", Bucket: "coti-files", AccessKey: "key", SecretKey: "secret"})
	if err := store.Upload(t.Context(), "accounts/a/stream.txt", "text/plain", bufio.NewReader(strings.NewReader(content))); err != nil {
		t.Fatal(err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
	files, err := os.ReadDir(os.Getenv("TMPDIR"))
	if err != nil || len(files) != 0 {
		t.Fatalf("temporary uploads remain: files=%v error=%v", files, err)
	}
}

func TestNeonStorage_SignedLinksUseTheBranchEndpointAndBucketPath(t *testing.T) {
	t.Parallel()
	settings := config.S3Settings{
		Endpoint: "https://branch.storage.us-east-1.aws.neon.tech",
		Region:   "us-east-1", Bucket: "coti-files", AccessKey: "neon-key", SecretKey: "secret",
	}
	key := "accounts/a/rfqs/b/plan with spaces.pdf"
	raw, err := NewNeonStorage(settings).GenerateSignedURL(context.Background(), key, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	link, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if link.Host != "branch.storage.us-east-1.aws.neon.tech" || link.Path != "/coti-files/"+key {
		t.Fatalf("unexpected bucket addressing: %s", raw)
	}
	if !strings.HasPrefix(link.Query().Get("X-Amz-Credential"), "neon-key/") || link.Query().Get("X-Amz-Expires") != "60" {
		t.Fatalf("unexpected signing credential or expiry: %s", raw)
	}
}

func TestS3Storage_UploadPreservesContentAndProviderPrivacyHeaders(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		build func(config.S3Settings) *S3Storage
		acl   string
	}{
		{"neon", NewNeonStorage, ""},
		{"spaces", NewSpacesStorage, "private"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
				}
				if r.Method != http.MethodPut || r.URL.Path != "/coti-files/accounts/a/test.txt" {
					t.Errorf("unexpected upload request: %s %s", r.Method, r.URL.Path)
				}
				if string(body) != "private content" || r.Header.Get("Content-Type") != "text/plain" {
					t.Error("upload did not preserve bytes and content type")
				}
				if r.Header.Get("X-Amz-Acl") != tc.acl {
					t.Errorf("ACL = %q, want %q", r.Header.Get("X-Amz-Acl"), tc.acl)
				}
				if !strings.Contains(r.Header.Get("Authorization"), "Credential=configured-key/") {
					t.Error("upload lacks the configured signing credential")
				}
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()
			store := tc.build(config.S3Settings{Endpoint: server.URL, Region: "us-east-1", Bucket: "coti-files", AccessKey: "configured-key", SecretKey: "secret"})
			if err := store.Upload(context.Background(), "accounts/a/test.txt", "text/plain", strings.NewReader("private content")); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestNeonStorage_DownloadPreservesMetadataAndMapsMissingObjects(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/missing.txt") {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, "<Error><Code>NoSuchKey</Code></Error>")
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, "private content")
	}))
	defer server.Close()
	store := NewNeonStorage(config.S3Settings{Endpoint: server.URL, Region: "us-east-1", Bucket: "coti-files", AccessKey: "key", SecretKey: "secret"})
	object, err := store.Download(context.Background(), "accounts/a/test.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer object.Body.Close()
	body, err := io.ReadAll(object.Body)
	if err != nil || string(body) != "private content" || object.ContentType != "text/plain" || object.Size != 15 {
		t.Fatalf("download metadata or body changed: size=%d type=%q error=%v", object.Size, object.ContentType, err)
	}
	if _, err := store.Download(context.Background(), "accounts/a/missing.txt"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing object error = %v, want ErrNotFound", err)
	}
}
