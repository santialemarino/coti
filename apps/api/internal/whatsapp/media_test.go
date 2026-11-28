package whatsapp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func mediaServer(t *testing.T, fileSize string, file []byte) *httptest.Server {
	t.Helper()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer access-token" {
			t.Errorf("authorization = %q, want the channel's bearer token", got)
		}
		switch r.URL.Path {
		case "/v25.0/media-1":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"url":"` + server.URL + `/download/media-1",` +
				`"mime_type":"audio/ogg; codecs=opus","file_size":` + fileSize + `}`))
		case "/download/media-1":
			_, _ = w.Write(file)
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func TestMetaMediaFetcher_ResolvesTheIDThenDownloadsWithTheSameToken(t *testing.T) {
	t.Parallel()
	server := mediaServer(t, "5", []byte("voice"))
	media, err := newMetaMediaFetcher(server.Client(), server.URL, "v25.0").
		FetchMedia(context.Background(), "access-token", "media-1", 10)
	if err != nil {
		t.Fatalf("FetchMedia() = %v, want no error", err)
	}
	if media.MediaType != "audio/ogg; codecs=opus" || string(media.Data) != "voice" {
		t.Errorf("media = %+v, want the declared type and the bytes", media)
	}
}

func TestMetaMediaFetcher_RefusesAFileOverTheLimit(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, declared string }{
		{"declared over the limit", "11"},
		{"declared under it but larger", "3"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := mediaServer(t, tc.declared, []byte(strings.Repeat("x", 11)))
			_, err := newMetaMediaFetcher(server.Client(), server.URL, "v25.0").
				FetchMedia(context.Background(), "access-token", "media-1", 10)
			if !errors.Is(err, domain.ErrInvalidInput) {
				t.Fatalf("FetchMedia() = %v, want ErrInvalidInput", err)
			}
		})
	}
}
