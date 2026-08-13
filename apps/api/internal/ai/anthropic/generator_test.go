package anthropic

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

// extraction is the shape a caller decodes into — the RFQ line the engine will ask for.
type extraction struct {
	Description string `json:"description"`
	Quantity    string `json:"quantity"`
}

var lineSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"description": map[string]any{"type": "string"},
		"quantity":    map[string]any{"type": "string"},
	},
	"required":             []string{"description", "quantity"},
	"additionalProperties": false,
}

func request() domain.GenerationRequest {
	return domain.GenerationRequest{
		Instructions: "extract the requested line from the message",
		Input:        []domain.Content{domain.TextContent("necesito 300 bolsas de cemento")},
		Schema:       lineSchema,
	}
}

// recorder is a stand-in provider that answers with the queued replies in order and keeps the
// request bodies it was sent.
type recorder struct {
	mu      sync.Mutex
	replies []reply
	bodies  []map[string]any
}

type reply struct {
	status int
	body   string
}

func (r *recorder) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()

	raw, _ := io.ReadAll(req.Body)
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
	return len(r.bodies)
}

// message renders a provider answer carrying one text block.
func message(stopReason, text string) string {
	body, _ := json.Marshal(map[string]any{
		"id":          "msg_1",
		"type":        "message",
		"role":        "assistant",
		"model":       "claude-opus-5",
		"stop_reason": stopReason,
		"content":     []map[string]any{{"type": "text", "text": text}},
		"usage": map[string]any{
			"input_tokens":            120,
			"output_tokens":           42,
			"cache_read_input_tokens": 100,
		},
	})
	return string(body)
}

// newGenerator points a Generator at srv, with waits short enough to keep the test instant.
func newGenerator(t *testing.T, srv *httptest.Server, attempts int) *Generator {
	t.Helper()

	return NewGenerator(config.AIConfig{
		AnthropicAPIKey:  "test-key",
		AnthropicBaseURL: srv.URL,
		LLMProvider:      config.AIProviderAnthropic,
		LLMModel:         "claude-opus-5",
		LLMEffort:        "low",
		LLMMaxTokens:     16000,
		LLMTimeout:       5 * time.Second,
		Retry:            config.AIRetryPolicy{MaxAttempts: attempts, Backoff: time.Microsecond},
	}, slog.New(slog.DiscardHandler))
}

func serve(t *testing.T, replies ...reply) (*httptest.Server, *recorder) {
	t.Helper()

	provider := &recorder{replies: replies}
	srv := httptest.NewServer(provider)
	t.Cleanup(srv.Close)
	return srv, provider
}

func TestGenerator_DecodesTheAnswerAndReportsUsage(t *testing.T) {
	t.Parallel()

	srv, provider := serve(t, reply{http.StatusOK,
		message("end_turn", `{"description":"cemento","quantity":"300"}`)})

	var got extraction
	usage, err := newGenerator(t, srv, 3).Generate(context.Background(), request(), &got)
	if err != nil {
		t.Fatalf("Generate() = %v, want nil", err)
	}
	if got.Description != "cemento" || got.Quantity != "300" {
		t.Fatalf("decoded %+v, want cemento/300", got)
	}
	if usage.InputTokens != 120 || usage.OutputTokens != 42 || usage.CachedInputTokens != 100 {
		t.Fatalf("usage = %+v, want 120/42/100 tokens", usage)
	}
	if usage.Provider != "anthropic" || usage.Model != "claude-opus-5" {
		t.Fatalf("usage = %+v, want it to name the provider and model", usage)
	}
	if provider.calls() != 1 {
		t.Fatalf("calls = %d, want 1", provider.calls())
	}
}

// The schema and the effort ride on every request: without them the answer is free text and the
// model spends far more than a mapping task needs.
func TestGenerator_SendsTheForcedSchemaAndTheConfiguredEffort(t *testing.T) {
	t.Parallel()

	srv, provider := serve(t, reply{http.StatusOK,
		message("end_turn", `{"description":"cemento","quantity":"300"}`)})

	var got extraction
	if _, err := newGenerator(t, srv, 3).Generate(context.Background(), request(), &got); err != nil {
		t.Fatalf("Generate() = %v, want nil", err)
	}

	body := provider.bodies[0]
	outputConfig, ok := body["output_config"].(map[string]any)
	if !ok {
		t.Fatalf("request carries no output_config: %v", body)
	}
	if outputConfig["effort"] != "low" {
		t.Fatalf("effort = %v, want low", outputConfig["effort"])
	}
	format, ok := outputConfig["format"].(map[string]any)
	if !ok {
		t.Fatalf("output_config carries no format: %v", outputConfig)
	}
	if format["type"] != "json_schema" {
		t.Fatalf("format type = %v, want json_schema", format["type"])
	}
	if format["schema"] == nil {
		t.Fatal("format carries no schema: the answer would not be constrained")
	}
	// The stable half is marked for caching, so a repeat call is charged for the variable half.
	system, ok := body["system"].([]any)
	if !ok || len(system) != 1 {
		t.Fatalf("request carries no system prompt: %v", body)
	}
	if system[0].(map[string]any)["cache_control"] == nil {
		t.Fatal("the system prompt is not marked for caching")
	}
}

func TestGenerator_RetriesAnAnswerThatDoesNotSatisfyTheSchema(t *testing.T) {
	t.Parallel()

	srv, provider := serve(t,
		// A field the caller's shape does not carry: accepted silently, this would drop data.
		reply{http.StatusOK, message("end_turn", `{"description":"cemento","cantidad":"300"}`)},
		reply{http.StatusOK, message("end_turn", `{"description":"cemento","quantity":"300"}`)},
	)

	var got extraction
	if _, err := newGenerator(t, srv, 3).Generate(context.Background(), request(), &got); err != nil {
		t.Fatalf("Generate() = %v, want nil", err)
	}
	if provider.calls() != 2 {
		t.Fatalf("calls = %d, want 2: the malformed answer should have been asked for again",
			provider.calls())
	}
	if got.Quantity != "300" {
		t.Fatalf("decoded %+v, want the second answer", got)
	}
}

func TestGenerator_RetriesATruncatedAnswer(t *testing.T) {
	t.Parallel()

	srv, provider := serve(t,
		reply{http.StatusOK, message("max_tokens", `{"description":"ceme`)},
		reply{http.StatusOK, message("end_turn", `{"description":"cemento","quantity":"300"}`)},
	)

	var got extraction
	if _, err := newGenerator(t, srv, 3).Generate(context.Background(), request(), &got); err != nil {
		t.Fatalf("Generate() = %v, want nil", err)
	}
	if provider.calls() != 2 {
		t.Fatalf("calls = %d, want 2", provider.calls())
	}
}

func TestGenerator_RetriesARateLimitAndAServerFault(t *testing.T) {
	t.Parallel()

	for _, status := range []int{http.StatusTooManyRequests, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()

			srv, provider := serve(t,
				reply{status, `{"type":"error","error":{"type":"api_error","message":"later"}}`},
				reply{http.StatusOK, message("end_turn", `{"description":"cemento","quantity":"300"}`)},
			)

			var got extraction
			if _, err := newGenerator(t, srv, 3).Generate(context.Background(), request(), &got); err != nil {
				t.Fatalf("Generate() = %v, want nil", err)
			}
			if provider.calls() != 2 {
				t.Fatalf("calls = %d, want 2", provider.calls())
			}
		})
	}
}

func TestGenerator_DoesNotRepeatARejectedRequest(t *testing.T) {
	t.Parallel()

	srv, provider := serve(t, reply{http.StatusBadRequest,
		`{"type":"error","error":{"type":"invalid_request_error","message":"bad schema"}}`})

	var got extraction
	_, err := newGenerator(t, srv, 3).Generate(context.Background(), request(), &got)
	if !errors.Is(err, domain.ErrAIUnavailable) {
		t.Fatalf("Generate() = %v, want it to match domain.ErrAIUnavailable", err)
	}
	if provider.calls() != 1 {
		t.Fatalf("calls = %d, want 1: a rejected request must not be repeated", provider.calls())
	}
}

func TestGenerator_DoesNotRepeatARefusal(t *testing.T) {
	t.Parallel()

	refusal, _ := json.Marshal(map[string]any{
		"id":           "msg_1",
		"type":         "message",
		"role":         "assistant",
		"model":        "claude-opus-5",
		"stop_reason":  "refusal",
		"stop_details": map[string]any{"type": "refusal", "category": "cyber"},
		"content":      []map[string]any{},
		"usage":        map[string]any{"input_tokens": 10, "output_tokens": 0},
	})
	srv, provider := serve(t, reply{http.StatusOK, string(refusal)})

	var got extraction
	_, err := newGenerator(t, srv, 3).Generate(context.Background(), request(), &got)
	if !errors.Is(err, domain.ErrAIUnavailable) {
		t.Fatalf("Generate() = %v, want it to match domain.ErrAIUnavailable", err)
	}
	if provider.calls() != 1 {
		t.Fatalf("calls = %d, want 1: the same prompt would be refused again", provider.calls())
	}
}

func TestGenerator_ExhaustedAttemptsReportAControlledError(t *testing.T) {
	t.Parallel()

	srv, provider := serve(t, reply{http.StatusServiceUnavailable,
		`{"type":"error","error":{"type":"overloaded_error","message":"overloaded"}}`})

	var got extraction
	_, err := newGenerator(t, srv, 2).Generate(context.Background(), request(), &got)
	if !errors.Is(err, domain.ErrAIUnavailable) {
		t.Fatalf("Generate() = %v, want it to match domain.ErrAIUnavailable", err)
	}
	if got := domain.CodeOf(err); got != domain.CodeAIUnavailable {
		t.Fatalf("CodeOf() = %q, want %q", got, domain.CodeAIUnavailable)
	}
	if provider.calls() != 2 {
		t.Fatalf("calls = %d, want the configured 2 attempts", provider.calls())
	}
}

// A caller's own mistake is not a provider failure: it must not be reported as one, and must not
// cost a round trip.
func TestGenerator_RejectsAnUnsendableRequestWithoutCallingTheProvider(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		request domain.GenerationRequest
	}{
		{
			name:    "no schema",
			request: domain.GenerationRequest{Instructions: "extract", Input: []domain.Content{domain.TextContent("x")}},
		},
		{
			name: "document the provider does not read",
			request: domain.GenerationRequest{
				Instructions: "read the attachment",
				Input:        []domain.Content{domain.DocumentContent("application/vnd.ms-excel", []byte("x"))},
				Schema:       lineSchema,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv, provider := serve(t, reply{http.StatusOK, message("end_turn", `{}`)})

			var got extraction
			_, err := newGenerator(t, srv, 3).Generate(context.Background(), tc.request, &got)
			if !errors.Is(err, domain.ErrInvalidInput) {
				t.Fatalf("Generate() = %v, want it to match domain.ErrInvalidInput", err)
			}
			if errors.Is(err, domain.ErrAIUnavailable) {
				t.Fatal("a caller's mistake was reported as a provider failure")
			}
			if provider.calls() != 0 {
				t.Fatalf("calls = %d, want 0", provider.calls())
			}
		})
	}
}

func TestGenerator_SendsPhotographedListsAndPDFsAsTheirOwnBlocks(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		content   domain.Content
		blockType string
	}{
		{"photo", domain.ImageContent("image/jpeg", []byte{0xFF, 0xD8}), "image"},
		{"pdf", domain.DocumentContent("application/pdf", []byte("%PDF-1.7")), "document"},
		{"plain text", domain.DocumentContent("text/plain", []byte("300 bolsas")), "document"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv, provider := serve(t, reply{http.StatusOK,
				message("end_turn", `{"description":"cemento","quantity":"300"}`)})

			req := request()
			req.Input = []domain.Content{tc.content}

			var got extraction
			if _, err := newGenerator(t, srv, 3).Generate(context.Background(), req, &got); err != nil {
				t.Fatalf("Generate() = %v, want nil", err)
			}

			messages := provider.bodies[0]["messages"].([]any)
			content := messages[0].(map[string]any)["content"].([]any)
			if blockType := content[0].(map[string]any)["type"]; blockType != tc.blockType {
				t.Fatalf("block type = %v, want %s", blockType, tc.blockType)
			}
		})
	}
}

// The SDK retries on its own by default, which would silently multiply every configured attempt.
func TestGenerator_MakesExactlyTheConfiguredNumberOfAttempts(t *testing.T) {
	t.Parallel()

	srv, provider := serve(t, reply{http.StatusInternalServerError,
		`{"type":"error","error":{"type":"api_error","message":"boom"}}`})

	var got extraction
	if _, err := newGenerator(t, srv, 3).Generate(context.Background(), request(), &got); err == nil {
		t.Fatal("Generate() = nil, want the attempts to run out")
	}
	if provider.calls() != 3 {
		t.Fatalf("calls = %d, want exactly the configured 3", provider.calls())
	}
}

func TestGenerator_AuthenticatesWithTheConfiguredKey(t *testing.T) {
	t.Parallel()

	var header string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header = r.Header.Get("X-Api-Key")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, message("end_turn", `{"description":"cemento","quantity":"300"}`))
	}))
	t.Cleanup(srv.Close)

	var got extraction
	if _, err := newGenerator(t, srv, 3).Generate(context.Background(), request(), &got); err != nil {
		t.Fatalf("Generate() = %v, want nil", err)
	}
	if !strings.Contains(header, "test-key") {
		t.Fatalf("X-Api-Key = %q, want the configured key", header)
	}
}
