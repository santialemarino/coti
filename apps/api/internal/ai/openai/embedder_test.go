package openai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
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

// policy keeps the waits short enough to keep a test instant.
func policy(attempts int) config.AIRetryPolicy {
	return config.AIRetryPolicy{
		MaxAttempts: attempts,
		Backoff:     time.Microsecond,
		MaxBackoff:  time.Millisecond,
	}
}

// newEmbedder points an Embedder at srv, sending every text in one batch unless a test says otherwise.
func newEmbedder(srv *httptest.Server, attempts int) *Embedder {
	return newEmbedderWithBatch(srv, attempts, 1000)
}

func newEmbedderWithBatch(srv *httptest.Server, attempts, batch int) *Embedder {
	return NewEmbedder(config.EmbeddingsConfig{
		APIKey:    "test-key",
		BaseURL:   srv.URL,
		Model:     "text-embedding-3-small",
		BatchSize: batch,
		Timeout:   5 * time.Second,
		Retry:     policy(attempts),
	}, slog.New(slog.DiscardHandler))
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
	if err == nil {
		t.Fatal("Embed() = nil, want the width refused")
	}
	// A model or configuration mismatch of ours, so it must not read as a provider outage.
	if errors.Is(err, domain.ErrAIUnavailable) {
		t.Fatalf("Embed() = %v, want a width mismatch NOT to read as a provider outage", err)
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
	if err == nil {
		t.Fatal("Embed() = nil, want the rejection reported")
	}
	if errors.Is(err, domain.ErrAIUnavailable) {
		t.Fatalf("Embed() = %v, want a bad key NOT to read as a provider outage", err)
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

// A catalog import hands over thousands of descriptions at once. One request carrying all of them
// exceeds the provider's per-request limits and is rejected wholesale, losing every vector in the
// batch — so the list is chunked.
func TestEmbedder_SendsLongListsInBatches(t *testing.T) {
	t.Parallel()

	// Each batch is answered with entries indexed from zero, which is what the provider does: the
	// indexes are per request, not per the caller's whole list.
	srv, provider := serve(t,
		reply{http.StatusOK, embeddings(domain.EmbeddingDimension, 0, 1)},
		reply{http.StatusOK, embeddings(domain.EmbeddingDimension, 0, 1)},
		reply{http.StatusOK, embeddings(domain.EmbeddingDimension, 0)},
	)

	texts := []string{"cemento", "hierro", "arena", "cal", "ladrillo"}
	vectors, err := newEmbedderWithBatch(srv, 3, 2).Embed(context.Background(), texts)
	if err != nil {
		t.Fatalf("Embed() = %v, want nil", err)
	}
	if len(vectors) != len(texts) {
		t.Fatalf("vectors = %d, want %d — one per text, in order", len(vectors), len(texts))
	}
	if provider.calls() != 3 {
		t.Fatalf("calls = %d, want 3: five texts at two per request", provider.calls())
	}
	// Each request carries only its own slice of the list.
	for i, want := range [][]any{
		{"cemento", "hierro"},
		{"arena", "cal"},
		{"ladrillo"},
	} {
		got, ok := provider.bodies[i]["input"].([]any)
		if !ok {
			t.Fatalf("request %d carries no input: %v", i, provider.bodies[i])
		}
		if len(got) != len(want) {
			t.Fatalf("request %d carried %d texts, want %d", i, len(got), len(want))
		}
		for j := range want {
			if got[j] != want[j] {
				t.Fatalf("request %d text %d = %v, want %v", i, j, got[j], want[j])
			}
		}
	}
}

// A batch that fails takes the call down with it rather than returning a short list the caller
// would read as complete.
func TestEmbedder_AFailedBatchFailsTheWholeCall(t *testing.T) {
	t.Parallel()

	srv, provider := serve(t,
		reply{http.StatusOK, embeddings(domain.EmbeddingDimension, 0, 1)},
		reply{http.StatusBadRequest, `{"error":{"message":"too many inputs"}}`},
	)

	texts := []string{"cemento", "hierro", "arena", "cal"}
	vectors, err := newEmbedderWithBatch(srv, 1, 2).Embed(context.Background(), texts)
	if err == nil {
		t.Fatal("Embed() = nil, want the failed batch reported")
	}
	if vectors != nil {
		t.Fatalf("vectors = %v, want nil rather than a short list", vectors)
	}
	// The third batch is never attempted: the call is already lost.
	if provider.calls() != 2 {
		t.Fatalf("calls = %d, want 2", provider.calls())
	}
	if !strings.Contains(err.Error(), "batch") {
		t.Fatalf("err = %v, want it to say which batch failed", err)
	}
}

// Tokens are charged per request, so the figure the pilot measures cost with has to sum every
// batch. Nothing on the port returns usage, so the log is where it lands.
func TestEmbedder_UsageLogSumsEveryBatch(t *testing.T) {
	t.Parallel()

	srv, _ := serve(t,
		reply{http.StatusOK, embeddings(domain.EmbeddingDimension, 0, 1)},
		reply{http.StatusOK, embeddings(domain.EmbeddingDimension, 0, 1)},
	)

	records := &capturingHandler{}
	embedder := NewEmbedder(config.EmbeddingsConfig{
		APIKey:    "test-key",
		BaseURL:   srv.URL,
		Model:     "text-embedding-3-small",
		BatchSize: 2,
		Timeout:   5 * time.Second,
		Retry:     policy(3),
	}, slog.New(records))

	texts := []string{"cemento", "hierro", "arena", "cal"}
	if _, err := embedder.Embed(context.Background(), texts); err != nil {
		t.Fatalf("Embed() = %v, want nil", err)
	}

	logged := records.only(t)
	// Each of the two batches reported 12 prompt tokens.
	if got := logged["input_tokens"]; got != int64(24) {
		t.Fatalf("input_tokens = %v, want 24 — the sum over both batches", got)
	}
	if got := logged["attempts"]; got != int64(2) {
		t.Fatalf("attempts = %v, want 2 — one per batch", got)
	}
	if got := logged["operation"]; got != "embed" {
		t.Fatalf("operation = %v, want embed", got)
	}
}

// capturingHandler keeps the records written to it, so a test can read the usage log the way an
// operator would.
type capturingHandler struct {
	mu      sync.Mutex
	records []map[string]any
}

func (h *capturingHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *capturingHandler) Handle(_ context.Context, record slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	fields := map[string]any{}
	record.Attrs(func(attr slog.Attr) bool {
		fields[attr.Key] = attr.Value.Any()
		return true
	})
	h.records = append(h.records, fields)
	return nil
}

func (h *capturingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }

func (h *capturingHandler) WithGroup(string) slog.Handler { return h }

// only returns the single record written, failing the test when the count is anything else.
func (h *capturingHandler) only(t *testing.T) map[string]any {
	t.Helper()

	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.records) != 1 {
		t.Fatalf("usage log holds %d records, want exactly 1 per call", len(h.records))
	}
	return h.records[0]
}
