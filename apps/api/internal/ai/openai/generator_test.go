package openai

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/santialemarino/coti/apps/api/internal/ai"
	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func TestGenerator_ForcedSchemaAndMultimodalInput(t *testing.T) {
	srv, recorded := serve(t, reply{200, `{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"{\"quantity\":\"2\"}"}]}],"usage":{"input_tokens":20,"output_tokens":5,"input_tokens_details":{"cached_tokens":3}}}`})
	g := NewGenerator(config.OpenAIGenerationConfig{APIKey: "test-key", BaseURL: srv.URL, Model: "gpt-4.1-mini",
		MaxTokens: 1000, Timeout: time.Second, Retry: policy(1)}, ai.NewMeter(slog.New(slog.DiscardHandler), nil))
	request := generationTestRequest()
	request.MaxTokens = 2000
	request.Input = append(request.Input, domain.ImageContent("image/png", []byte("image")),
		domain.DocumentContent("application/pdf", []byte("pdf")), domain.DocumentContent("text/plain", []byte("text")))
	var out struct {
		Quantity string `json:"quantity"`
	}
	usage, err := g.Generate(context.Background(), request, &out)
	if err != nil || out.Quantity != "2" || usage.InputTokens != 17 || usage.CacheReadTokens != 3 {
		t.Fatalf("answer = %+v / %+v, %v", out, usage, err)
	}
	body := recorded.bodies[0]
	format := body["text"].(map[string]any)["format"].(map[string]any)
	if format["strict"] != true || format["type"] != "json_schema" || body["store"] != false || body["max_output_tokens"] != float64(1000) {
		t.Fatalf("request = %+v", body)
	}
	if recorded.requests[0].URL.Path != "/responses" || recorded.requests[0].Header.Get("Authorization") != "Bearer test-key" {
		t.Fatal("wrong endpoint or credentials")
	}
	blocks := body["input"].([]any)[0].(map[string]any)["content"].([]any)
	if len(blocks) != 4 || blocks[1].(map[string]any)["image_url"] != "data:image/png;base64,aW1hZ2U=" || blocks[2].(map[string]any)["file_data"] != "data:application/pdf;base64,cGRm" {
		t.Fatalf("content = %+v", blocks)
	}
}

func TestGenerator_RejectsOrRetriesInvalidResponses(t *testing.T) {
	for _, test := range []struct {
		name, body    string
		status, calls int
	}{
		{"refusal", `{"status":"completed","output":[{"type":"message","content":[{"type":"refusal"}]}]}`, 200, 1},
		{"incomplete", `{"status":"incomplete"}`, 200, 2},
		{"unknown field", `{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"{\"extra\":1}"}]}]}`, 200, 2},
		{"trailing data", `{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"{\"quantity\":\"2\"} {}"}]}]}`, 200, 2},
		{"credentials", `{"error":{"message":"invalid key"}}`, 401, 1},
		{"provider outage", `{"error":{"message":"unavailable"}}`, 503, 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			srv, recorded := serve(t, reply{test.status, test.body})
			g := NewGenerator(config.OpenAIGenerationConfig{APIKey: "test-key", BaseURL: srv.URL, Model: "gpt-4.1-mini", MaxTokens: 1000, Timeout: time.Second, Retry: policy(2)}, ai.NewMeter(slog.New(slog.DiscardHandler), nil))
			var out struct {
				Quantity string `json:"quantity"`
			}
			_, err := g.Generate(context.Background(), generationTestRequest(), &out)
			if err == nil || recorded.calls() != test.calls {
				t.Fatalf("error/calls = %v/%d", err, recorded.calls())
			}
		})
	}
}

func TestGenerator_RejectsInvalidInputBeforeCallingProvider(t *testing.T) {
	srv, recorded := serve(t, reply{200, `{}`})
	g := NewGenerator(config.OpenAIGenerationConfig{APIKey: "test-key", BaseURL: srv.URL, MaxTokens: 1000, Timeout: time.Second, Retry: policy(1)}, ai.NewMeter(slog.New(slog.DiscardHandler), nil))
	request := generationTestRequest()
	if _, err := g.Generate(context.Background(), request, nil); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("nil output: %v", err)
	}
	request.Input = []domain.Content{domain.DocumentContent("application/zip", []byte("zip"))}
	var out any
	if _, err := g.Generate(context.Background(), request, &out); err == nil || !strings.Contains(err.Error(), "media type") {
		t.Fatalf("unsupported file: %v", err)
	}
	if recorded.calls() != 0 {
		t.Fatal("invalid input reached provider")
	}
}

func TestGenerator_RetryClearsPartialOutputAndMetersEveryAnswer(t *testing.T) {
	srv, recorded := serve(t,
		reply{200, `{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"{\"quantity\":\"9\",\"note\":\"discarded\",\"extra\":true}"}]}],"usage":{"input_tokens":20,"output_tokens":5}}`},
		reply{200, `{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"{\"quantity\":\"2\",\"note\":null}"}]}],"usage":{"input_tokens":30,"output_tokens":8}}`})
	g := NewGenerator(config.OpenAIGenerationConfig{APIKey: "test-key", BaseURL: srv.URL, Model: "gpt-4.1-mini",
		MaxTokens: 1000, Timeout: time.Second, Retry: policy(2)}, ai.NewMeter(slog.New(slog.DiscardHandler), nil))
	request := generationTestRequest()
	request.Schema["required"] = []string{"quantity", "note"}
	request.Schema["properties"].(map[string]any)["note"] = map[string]any{"type": []string{"string", "null"}}
	var out struct {
		Quantity string `json:"quantity"`
		Note     string `json:"note"`
	}
	usage, err := g.Generate(context.Background(), request, &out)
	if err != nil || recorded.calls() != 2 || out.Quantity != "2" || out.Note != "" || usage.InputTokens != 50 || usage.OutputTokens != 13 {
		t.Fatalf("answer = %+v / %+v, %v; calls=%d", out, usage, err, recorded.calls())
	}
}

func generationTestRequest() domain.GenerationRequest {
	return domain.GenerationRequest{Instructions: "Extract the requested quantity.", Input: []domain.Content{domain.TextContent("2 bags")}, Schema: map[string]any{
		"type": "object", "additionalProperties": false, "required": []string{"quantity"},
		"properties": map[string]any{"quantity": map[string]any{"type": "string"}},
	}}
}
