package handler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/storage"
)

const (
	fileTestSecret = "a-signing-secret-of-at-least-32-chars"
	fileTestKey    = "accounts/11111111/rfqs/22222222/plan.pdf"
	fileTestBody   = "%PDF-1.7 test document"
)

// storedFile is the storage layer as this route sees it: a real signer over a fixed clock, and
// one object. It is not a stand-in for the signature check — that is the real one.
type storedFile struct {
	signer      *storage.URLSigner
	contentType string
	missing     bool
}

func (s storedFile) Verify(key string, expiresAt time.Time, signature string) error {
	return s.signer.Verify(key, expiresAt, signature)
}

func (s storedFile) Download(ctx context.Context, key string) (*domain.StoredObject, error) {
	if s.missing {
		return nil, fmt.Errorf("%w: object %s", domain.ErrNotFound, key)
	}
	return &domain.StoredObject{
		Body:        io.NopCloser(strings.NewReader(fileTestBody)),
		ContentType: s.contentType,
		Size:        int64(len(fileTestBody)),
	}, nil
}

func serveFile(t *testing.T, source SignedObjectSource, target string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET(storage.LinkPath+"/*key", NewFileHandler(source).Get)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
	return recorder
}

// signedLink builds the link the local adapter would hand out, at the clock the signer reads.
func signedLink(t *testing.T, signer *storage.URLSigner, key string, expiresAt time.Time) string {
	t.Helper()
	return fmt.Sprintf("%s/%s?%s", storage.LinkPath, key, url.Values{
		"expires":   {strconv.FormatInt(expiresAt.Unix(), 10)},
		"signature": {signer.Sign(key, expiresAt)},
	}.Encode())
}

func fixedSigner(at time.Time) *storage.URLSigner {
	return storage.NewURLSigner([]byte(fileTestSecret), func() time.Time { return at })
}

func TestFileHandler_Get_ServesAValidLink(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	source := storedFile{signer: fixedSigner(now), contentType: "application/pdf"}

	recorder := serveFile(t, source, signedLink(t, source.signer, fileTestKey, now.Add(15*time.Minute)))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Body.String(); got != fileTestBody {
		t.Errorf("body = %q, want %q", got, fileTestBody)
	}
	// A PDF served as text/plain opens as mojibake, which is what makes the round-trip of the
	// content type through storage load-bearing rather than tidy.
	if got := recorder.Header().Get("Content-Type"); got != "application/pdf" {
		t.Errorf("Content-Type = %q, want %q", got, "application/pdf")
	}
	if got := recorder.Header().Get("Content-Length"); got != strconv.Itoa(len(fileTestBody)) {
		t.Errorf("Content-Length = %q, want %d", got, len(fileTestBody))
	}
}

func TestFileHandler_Get_StopsServingOnceTheLinkExpires(t *testing.T) {
	issuedAt := time.Unix(1_700_000_000, 0)
	expiresAt := issuedAt.Add(15 * time.Minute)
	link := signedLink(t, fixedSigner(issuedAt), fileTestKey, expiresAt)

	// The same link, unchanged, one second past its deadline.
	late := storedFile{signer: fixedSigner(expiresAt.Add(time.Second)), contentType: "application/pdf"}
	recorder := serveFile(t, late, link)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body: %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), string(domain.CodeLinkExpired)) {
		t.Errorf("body = %s, want the %s code", recorder.Body.String(), domain.CodeLinkExpired)
	}
	if strings.Contains(recorder.Body.String(), fileTestBody) {
		t.Error("an expired link served the object")
	}
}

func TestFileHandler_Get_RefusesALinkItDidNotIssue(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	expiresAt := now.Add(15 * time.Minute)
	signer := fixedSigner(now)
	source := storedFile{signer: signer, contentType: "application/pdf"}
	valid := signer.Sign(fileTestKey, expiresAt)

	cases := []struct {
		name   string
		target string
	}{
		{
			// The key names the account, so this is one tenant reaching into another's with a
			// link it was legitimately given.
			name: "another account's key on a valid signature",
			target: fmt.Sprintf("%s/accounts/99999999/rfqs/22222222/plan.pdf?expires=%d&signature=%s",
				storage.LinkPath, expiresAt.Unix(), valid),
		},
		{
			name: "a deadline pushed further out",
			target: fmt.Sprintf("%s/%s?expires=%d&signature=%s",
				storage.LinkPath, fileTestKey, expiresAt.Add(time.Hour).Unix(), valid),
		},
		{
			name: "a forged signature",
			target: fmt.Sprintf("%s/%s?expires=%d&signature=forged",
				storage.LinkPath, fileTestKey, expiresAt.Unix()),
		},
		{
			name:   "no signature at all",
			target: fmt.Sprintf("%s/%s?expires=%d", storage.LinkPath, fileTestKey, expiresAt.Unix()),
		},
		{
			name: "a deadline that is not a timestamp",
			target: fmt.Sprintf("%s/%s?expires=soon&signature=%s",
				storage.LinkPath, fileTestKey, valid),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := serveFile(t, source, tc.target)

			if recorder.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403; body: %s", recorder.Code, recorder.Body.String())
			}
			if strings.Contains(recorder.Body.String(), fileTestBody) {
				t.Error("an unsigned request served the object")
			}
		})
	}
}

func TestFileHandler_Get_MissingObjectIsNotFound(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	source := storedFile{signer: fixedSigner(now), missing: true}

	recorder := serveFile(t, source, signedLink(t, source.signer, fileTestKey, now.Add(time.Minute)))

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body: %s", recorder.Code, recorder.Body.String())
	}
}

func TestFileHandler_Get_ServesAnUntypedObjectAsBytes(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	source := storedFile{signer: fixedSigner(now)}

	recorder := serveFile(t, source, signedLink(t, source.signer, fileTestKey, now.Add(time.Minute)))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Content-Type"); got != defaultObjectContentType {
		t.Errorf("Content-Type = %q, want %q", got, defaultObjectContentType)
	}
}

// The link the adapter hands out, fetched through the route that serves it. Everything above
// is a fake somewhere; this is the loop end to end, with only the clock injected.
func TestFileHandler_Get_ServesTheLinkTheLocalAdapterIssued(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	base, err := url.Parse("http://localhost:8000")
	if err != nil {
		t.Fatalf("parse base url: %v", err)
	}
	dir := t.TempDir()
	local, err := storage.NewLocalStorage(dir, base, fixedSigner(now))
	if err != nil {
		t.Fatalf("new local storage: %v", err)
	}
	ctx := context.Background()
	if err := local.Upload(ctx, fileTestKey, "application/pdf", strings.NewReader(fileTestBody)); err != nil {
		t.Fatalf("upload: %v", err)
	}
	raw, err := local.GenerateSignedURL(ctx, fileTestKey, 15*time.Minute)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	link, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse link: %v", err)
	}

	recorder := serveFile(t, local, link.RequestURI())
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Body.String(); got != fileTestBody {
		t.Errorf("body = %q, want %q", got, fileTestBody)
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/pdf" {
		t.Errorf("Content-Type = %q, want %q", got, "application/pdf")
	}

	// The same link and the same directory, against an adapter whose clock has passed the
	// deadline: the object is still there, and only the clock has moved.
	expired, err := storage.NewLocalStorage(dir, base, fixedSigner(now.Add(16*time.Minute)))
	if err != nil {
		t.Fatalf("new local storage: %v", err)
	}
	recorder = serveFile(t, expired, link.RequestURI())
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body: %s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), fileTestBody) {
		t.Error("an expired link served the object")
	}
}
