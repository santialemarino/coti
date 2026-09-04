package storage

import (
	"context"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// testSigner builds a signer with a fixed clock, so a deadline in a test is the one the test
// chose rather than one the run's own timing decided.
func testSigner(t *testing.T, now time.Time) *URLSigner {
	t.Helper()
	return NewURLSigner([]byte("a-signing-secret-of-at-least-32-chars"), func() time.Time { return now })
}

func newTestLocalStorage(t *testing.T, signer *URLSigner) *LocalStorage {
	t.Helper()
	base, err := url.Parse("https://api.coti.test")
	if err != nil {
		t.Fatalf("parse base url: %v", err)
	}
	storage, err := NewLocalStorage(t.TempDir(), base, signer)
	if err != nil {
		t.Fatalf("new local storage: %v", err)
	}
	return storage
}

func isNotFound(err error) bool { return errors.Is(err, domain.ErrNotFound) }

func TestLocalStorage_Upload_RoundTripsBytesAndContentType(t *testing.T) {
	t.Parallel()
	storage := newTestLocalStorage(t, testSigner(t, time.Unix(1_700_000_000, 0)))
	const key = "accounts/11111111-1111-1111-1111-111111111111/rfqs/22222222/plan.pdf"
	const content = "test file"

	if err := storage.Upload(context.Background(), key, "application/pdf", strings.NewReader(content)); err != nil {
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
	// The content type is what a signed link serves the bytes as, so losing it is what makes a
	// downloaded PDF open as text.
	if object.ContentType != "application/pdf" {
		t.Fatalf("content type = %q, want %q", object.ContentType, "application/pdf")
	}
	if object.Size != int64(len(content)) {
		t.Fatalf("size = %d, want %d", object.Size, len(content))
	}
}

func TestLocalStorage_Upload_WritesObjectsUnreadableByOtherUsers(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not expose POSIX owner-only permission bits")
	}
	t.Parallel()
	storage := newTestLocalStorage(t, testSigner(t, time.Unix(1_700_000_000, 0)))
	const key = "accounts/a/rfqs/b/invoice.pdf"

	if err := storage.Upload(context.Background(), key, "application/pdf", strings.NewReader("x")); err != nil {
		t.Fatalf("upload: %v", err)
	}
	// The literal, not objectMode: asserting against the constant the code reads would pass
	// whatever that constant were changed to.
	for _, path := range []string{
		filepath.Join(storage.baseDir, filepath.FromSlash(key)),
		filepath.Join(storage.baseDir, filepath.FromSlash(key)) + metaSuffix,
	} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Errorf("%s mode = %#o, want %#o", path, got, 0o600)
		}
	}
	info, err := os.Stat(filepath.Dir(filepath.Join(storage.baseDir, filepath.FromSlash(key))))
	if err != nil {
		t.Fatalf("stat directory: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Errorf("directory mode = %#o, want %#o", got, 0o700)
	}
}

func TestLocalStorage_Upload_OverwritesRatherThanAppends(t *testing.T) {
	t.Parallel()
	storage := newTestLocalStorage(t, testSigner(t, time.Unix(1_700_000_000, 0)))
	const key = "accounts/a/rfqs/b/notes.txt"

	if err := storage.Upload(context.Background(), key, "text/plain", strings.NewReader("original")); err != nil {
		t.Fatalf("first upload: %v", err)
	}
	if err := storage.Upload(context.Background(), key, "text/plain", strings.NewReader("new")); err != nil {
		t.Fatalf("second upload: %v", err)
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
	if string(got) != "new" {
		t.Fatalf("body = %q, want %q", got, "new")
	}
}

func TestLocalStorage_Download_MissingObjectIsNotFound(t *testing.T) {
	t.Parallel()
	storage := newTestLocalStorage(t, testSigner(t, time.Unix(1_700_000_000, 0)))

	if _, err := storage.Download(context.Background(), "accounts/a/rfqs/b/absent.pdf"); !isNotFound(err) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestLocalStorage_RefusesKeysThatLeaveTheBaseDirectory(t *testing.T) {
	t.Parallel()
	storage := newTestLocalStorage(t, testSigner(t, time.Unix(1_700_000_000, 0)))
	for _, tc := range badKeys() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := storage.Upload(context.Background(), tc.key, "text/plain", strings.NewReader("x"))
			if !errors.Is(err, domain.ErrInvalidInput) {
				t.Fatalf("upload(%q) err = %v, want ErrInvalidInput", tc.key, err)
			}
			if _, err := storage.Download(context.Background(), tc.key); !errors.Is(err, domain.ErrInvalidInput) {
				t.Fatalf("download(%q) err = %v, want ErrInvalidInput", tc.key, err)
			}
		})
	}
}

func TestLocalStorage_RefusedKeyWritesNothingOutsideTheBaseDirectory(t *testing.T) {
	t.Parallel()
	storage := newTestLocalStorage(t, testSigner(t, time.Unix(1_700_000_000, 0)))
	outside := filepath.Join(filepath.Dir(storage.baseDir), "escaped.txt")

	if err := storage.Upload(context.Background(), "../escaped.txt", "text/plain", strings.NewReader("x")); err == nil {
		t.Fatal("upload of a climbing key was accepted")
	}
	if _, err := os.Stat(outside); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stat %s = %v, want the file never to have been written", outside, err)
	}
}

func TestLocalStorage_GenerateSignedURL_ProducesALinkItsOwnSignerAccepts(t *testing.T) {
	t.Parallel()
	now := time.Unix(1_700_000_000, 0)
	signer := testSigner(t, now)
	storage := newTestLocalStorage(t, signer)
	const key = "accounts/a/rfqs/b/plan.pdf"

	if err := storage.Upload(context.Background(), key, "application/pdf", strings.NewReader("x")); err != nil {
		t.Fatalf("upload: %v", err)
	}
	raw, err := storage.GenerateSignedURL(context.Background(), key, 15*time.Minute)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	link, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse link: %v", err)
	}
	if want := LinkPath + "/" + key; link.Path != want {
		t.Fatalf("link path = %q, want %q", link.Path, want)
	}
	if link.Host != "api.coti.test" {
		t.Fatalf("link host = %q, want %q", link.Host, "api.coti.test")
	}
	expiresAt := parseExpires(t, link)
	if want := now.Add(15 * time.Minute); !expiresAt.Equal(want) {
		t.Fatalf("expires = %s, want %s", expiresAt, want)
	}
	if err := signer.Verify(key, expiresAt, link.Query().Get("signature")); err != nil {
		t.Fatalf("verify: %v", err)
	}
}

func TestLocalStorage_GenerateSignedURL_MissingObjectIsNotFound(t *testing.T) {
	t.Parallel()
	storage := newTestLocalStorage(t, testSigner(t, time.Unix(1_700_000_000, 0)))

	_, err := storage.GenerateSignedURL(context.Background(), "accounts/a/rfqs/b/absent.pdf", time.Minute)
	if !isNotFound(err) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestLocalStorage_GenerateSignedURL_RefusesANonPositiveLifetime(t *testing.T) {
	t.Parallel()
	storage := newTestLocalStorage(t, testSigner(t, time.Unix(1_700_000_000, 0)))
	const key = "accounts/a/rfqs/b/plan.pdf"

	if err := storage.Upload(context.Background(), key, "application/pdf", strings.NewReader("x")); err != nil {
		t.Fatalf("upload: %v", err)
	}
	if _, err := storage.GenerateSignedURL(context.Background(), key, 0); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

func parseExpires(t *testing.T, link *url.URL) time.Time {
	t.Helper()
	raw := link.Query().Get("expires")
	seconds, err := time.ParseDuration(raw + "s")
	if err != nil {
		t.Fatalf("expires %q is not a number of seconds: %v", raw, err)
	}
	return time.Unix(int64(seconds.Seconds()), 0)
}
