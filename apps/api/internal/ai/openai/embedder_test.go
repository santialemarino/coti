package openai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// recorder is a stand-in provider that answers with the queued replies in order and keeps the
// requests it was sent.
type recorder struct {
	mu       sync.Mutex
	replies  []reply
	bodies   []map[string]any
	requests []*http.Request
	uploads  [][]byte
}

type reply struct {
	status int
	body   string
}

func (r *recorder) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.requests = append(r.requests, req)
	raw, _ := io.ReadAll(req.Body)
	r.uploads = append(r.uploads, raw)
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	r.bodies = append(r.bodies, body)

	next := r.replies[0]
	if len(r.replies) > 1 {
		r.replies = r.replies[1:]
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(next.status)
	_, _ = io.WriteString(w, next.body)
}

func (r *recorder) calls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.requests)
}

func serve(t *testing.T, replies ...reply) (*httptest.Server, *recorder) {
	t.Helper()

	provider := &recorder{replies: replies}
	srv := httptest.NewServer(provider)
	t.Cleanup(srv.Close)
	return srv, provider
}

// settings points an adapter at srv, with waits short enough to keep the test instant.
func settings(srv *httptest.Server, attempts int) config.AIConfig {
	return config.AIConfig{
		OpenAIAPIKey:          "test-key",
		OpenAIBaseURL:         srv.URL,
		EmbeddingsProvider:    config.AIProviderOpenAI,
		EmbeddingsModel:       "text-embedding-3-small",
		EmbeddingsTimeout:     5 * time.Second,
		TranscriptionProvider: config.AIProviderOpenAI,
		TranscriptionModel:    "whisper-1",
		TranscriptionTimeout:  5 * time.Second,
		Retry:                 config.AIRetryPolicy{MaxAttempts: attempts, Backoff: time.Microsecond},
	}
}

func newEmbedder(srv *httptest.Server, attempts int) *Embedder {
	return NewEmbedder(settings(srv, attempts), slog.New(slog.DiscardHandler))
}

// vector renders one embedding of the given width, filled so the values are distinguishable.
func vector(width int, seed float32) []float32 {
	values := make([]float32, width)
	for i := range values {
		values[i] = seed + float32(i)/1000
	}
	return values
}

// embeddings renders a provider answer carrying one entry per index given, in that order.
func embeddings(width int, indexes ...int) string {
	data := make([]map[string]any, 0, len(indexes))
	for _, index := range indexes {
		data = append(data, map[string]any{
			"index":     index,
			"embedding": vector(width, float32(index)),
		})
	}
	body, _ := json.Marshal(map[string]any{
		"data":  data,
		"usage": map[string]any{"prompt_tokens": 12, "total_tokens": 12},
	})
	return string(body)
}

func TestEmbedder_ReturnsOneVectorPerText(t *testing.T) {
	t.Parallel()

	srv, provider := serve(t, reply{http.StatusOK, embeddings(domain.EmbeddingDimension, 0, 1)})

	vectors, err := newEmbedder(srv, 3).Embed(context.Background(),
		[]string{"cemento portland", "hierro del 8"})
	if err != nil {
		t.Fatalf("Embed() = %v, want nil", err)
	}
	if len(vectors) != 2 {
		t.Fatalf("vectors = %d, want 2", len(vectors))
	}
	for i, v := range vectors {
		if got := len(v.Slice()); got != domain.EmbeddingDimension {
			t.Fatalf("vector %d has %d dimensions, want %d", i, got, domain.EmbeddingDimension)
		}
	}
	if provider.calls() != 1 {
		t.Fatalf("calls = %d, want 1: the texts go in one round trip", provider.calls())
	}
}

// The provider does not promise to answer in the order it was asked, so the entries are placed by
// their own index. Sorted by arrival, an out-of-order answer would attach each vector to the
// wrong product.
func TestEmbedder_PlacesOutOfOrderEntriesByTheirIndex(t *testing.T) {
	t.Parallel()

	srv, _ := serve(t, reply{http.StatusOK, embeddings(domain.EmbeddingDimension, 1, 0)})

	vectors, err := newEmbedder(srv, 3).Embed(context.Background(), []string{"first", "second"})
	if err != nil {
		t.Fatalf("Embed() = %v, want nil", err)
	}
	// Each vector is seeded from its index, so the first value identifies which text it answers.
	if got := vectors[0].Slice()[0]; got != 0 {
		t.Fatalf("vector 0 starts at %v, want 0: it holds the entry for another text", got)
	}
	if got := vectors[1].Slice()[0]; got != 1 {
		t.Fatalf("vector 1 starts at %v, want 1: it holds the entry for another text", got)
	}
}

// AC: the width has to match what the catalog column declares. A model returning anything else is
// a configuration mismatch, so it is refused rather than stored and compared against 1536-wide rows.
func TestEmbedder_RefusesAWidthTheCatalogCannotHold(t *testing.T) {
	t.Parallel()

	srv, provider := serve(t, reply{http.StatusOK, embeddings(768, 0)})

	_, err := newEmbedder(srv, 3).Embed(context.Background(), []string{"cemento"})
	if !errors.Is(err, domain.ErrAIUnavailable) {
		t.Fatalf("Embed() = %v, want it to match domain.ErrAIUnavailable", err)
	}
	if provider.calls() != 1 {
		t.Fatalf("calls = %d, want 1: another attempt would return the same width", provider.calls())
	}
}

// The width is asked for rather than assumed, so a model whose native width is wider still answers
// with vectors the column can hold.
func TestEmbedder_AsksForTheCatalogWidth(t *testing.T) {
	t.Parallel()

	srv, provider := serve(t, reply{http.StatusOK, embeddings(domain.EmbeddingDimension, 0)})

	if _, err := newEmbedder(srv, 3).Embed(context.Background(), []string{"cemento"}); err != nil {
		t.Fatalf("Embed() = %v, want nil", err)
	}
	if got := provider.bodies[0]["dimensions"]; got != float64(domain.EmbeddingDimension) {
		t.Fatalf("dimensions = %v, want %d", got, domain.EmbeddingDimension)
	}
	if got := provider.bodies[0]["model"]; got != "text-embedding-3-small" {
		t.Fatalf("model = %v, want the configured one", got)
	}
	if got := provider.requests[0].Header.Get("Authorization"); got != "Bearer test-key" {
		t.Fatalf("Authorization = %q, want the configured key", got)
	}
}

func TestEmbedder_RefusesAShortAnswer(t *testing.T) {
	t.Parallel()

	// Two texts asked for, one entry returned: the missing one would leave a zero vector behind.
	srv, _ := serve(t, reply{http.StatusOK, embeddings(domain.EmbeddingDimension, 0)})

	_, err := newEmbedder(srv, 1).Embed(context.Background(), []string{"cemento", "hierro"})
	if !errors.Is(err, domain.ErrAIUnavailable) {
		t.Fatalf("Embed() = %v, want it to match domain.ErrAIUnavailable", err)
	}
}

func TestEmbedder_RetriesARateLimitAndAServerFault(t *testing.T) {
	t.Parallel()

	for _, status := range []int{http.StatusTooManyRequests, http.StatusBadGateway} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()

			srv, provider := serve(t,
				reply{status, `{"error":{"message":"later"}}`},
				reply{http.StatusOK, embeddings(domain.EmbeddingDimension, 0)},
			)

			if _, err := newEmbedder(srv, 3).Embed(context.Background(), []string{"cemento"}); err != nil {
				t.Fatalf("Embed() = %v, want nil", err)
			}
			if provider.calls() != 2 {
				t.Fatalf("calls = %d, want 2", provider.calls())
			}
		})
	}
}

func TestEmbedder_DoesNotRepeatARejectedRequest(t *testing.T) {
	t.Parallel()

	srv, provider := serve(t, reply{http.StatusUnauthorized, `{"error":{"message":"bad key"}}`})

	_, err := newEmbedder(srv, 3).Embed(context.Background(), []string{"cemento"})
	if !errors.Is(err, domain.ErrAIUnavailable) {
		t.Fatalf("Embed() = %v, want it to match domain.ErrAIUnavailable", err)
	}
	if provider.calls() != 1 {
		t.Fatalf("calls = %d, want 1: a rejected key will be rejected again", provider.calls())
	}
}

func TestEmbedder_NoTextsMakesNoCall(t *testing.T) {
	t.Parallel()

	srv, provider := serve(t, reply{http.StatusOK, embeddings(domain.EmbeddingDimension, 0)})

	vectors, err := newEmbedder(srv, 3).Embed(context.Background(), nil)
	if err != nil {
		t.Fatalf("Embed() = %v, want nil", err)
	}
	if vectors != nil {
		t.Fatalf("vectors = %v, want nil", vectors)
	}
	if provider.calls() != 0 {
		t.Fatalf("calls = %d, want 0", provider.calls())
	}
}

func TestEmbedder_RejectsAnEmptyTextWithoutCallingTheProvider(t *testing.T) {
	t.Parallel()

	srv, provider := serve(t, reply{http.StatusOK, embeddings(domain.EmbeddingDimension, 0, 1)})

	_, err := newEmbedder(srv, 3).Embed(context.Background(), []string{"cemento", ""})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("Embed() = %v, want it to match domain.ErrInvalidInput", err)
	}
	if provider.calls() != 0 {
		t.Fatalf("calls = %d, want 0", provider.calls())
	}
}
