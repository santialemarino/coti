package ai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func TestAnthropicRFQExtractor_Extract_MapsStrictToolUse(t *testing.T) {
	var got anthropicMessageRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != anthropicMessagesPath {
			t.Fatalf("path = %q, want %q", r.URL.Path, anthropicMessagesPath)
		}
		if r.Header.Get("x-api-key") != "secret" {
			t.Fatalf("x-api-key = %q, want secret", r.Header.Get("x-api-key"))
		}
		if r.Header.Get("anthropic-version") != "2023-06-01" {
			t.Fatalf("anthropic-version = %q, want 2023-06-01", r.Header.Get("anthropic-version"))
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{
			"content": [
				{
					"type": "tool_use",
					"name": "record_rfq_extraction",
					"input": {
						"items": [
							{
								"requested_description": "Portland cement",
								"quantity": "10.50",
								"unit": "bag",
								"quantity_rationale": "Explicitly requested as 10.5 bags"
							}
						],
						"clarifications": []
					}
				}
			]
		}`))
	}))
	defer server.Close()

	extractor := NewAnthropicRFQExtractor(testAnthropicConfig(server.URL), server.Client())

	extraction, err := extractor.Extract(context.Background(), "Need 10.5 bags of Portland cement")
	if err != nil {
		t.Fatalf("Extract() = %v, want no error", err)
	}

	if got.Model != "claude-test" {
		t.Errorf("model = %q, want claude-test", got.Model)
	}
	if got.ToolChoice.Type != "tool" || got.ToolChoice.Name != rfqExtractionToolName {
		t.Errorf("tool_choice = %#v, want forced %s", got.ToolChoice, rfqExtractionToolName)
	}
	if len(got.Tools) != 1 || !got.Tools[0].Strict {
		t.Fatalf("tools = %#v, want one strict tool", got.Tools)
	}
	if len(got.Messages) != 1 || got.Messages[0].Content != "Need 10.5 bags of Portland cement" {
		t.Errorf("messages = %#v, want raw RFQ text", got.Messages)
	}
	if len(extraction.Lines) != 1 {
		t.Fatalf("lines = %d, want 1", len(extraction.Lines))
	}
	line := extraction.Lines[0]
	if line.RequestedDescription != "Portland cement" {
		t.Errorf("description = %q, want Portland cement", line.RequestedDescription)
	}
	if line.Quantity.String() != "10.5" {
		t.Errorf("quantity = %s, want 10.5", line.Quantity)
	}
	if line.Unit == nil || *line.Unit != "bag" {
		t.Errorf("unit = %v, want bag", line.Unit)
	}
	if line.QuantityRationale == nil || *line.QuantityRationale != "Explicitly requested as 10.5 bags" {
		t.Errorf("rationale = %v, want the provider rationale", line.QuantityRationale)
	}
	if len(extraction.Clarifications) != 0 {
		t.Errorf("clarifications = %#v, want none", extraction.Clarifications)
	}
}

func TestAnthropicRFQExtractor_Extract_MapsBlockingClarification(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{
			"content": [{
				"type": "tool_use",
				"name": "record_rfq_extraction",
				"input": {
					"items": [],
					"clarifications": [{
						"issue_type": "MISSING_QUANTITY",
						"requested_description": "cemento",
						"question": "Cuantas bolsas de cemento necesitas?",
						"reason": "La cantidad no aparece en el pedido."
					}]
				}
			}]
		}`))
	}))
	defer server.Close()

	extractor := NewAnthropicRFQExtractor(testAnthropicConfig(server.URL), server.Client())
	extraction, err := extractor.Extract(context.Background(), "Necesito cemento")
	if err != nil {
		t.Fatalf("Extract() = %v, want no error", err)
	}
	if len(extraction.Lines) != 0 || len(extraction.Clarifications) != 1 {
		t.Fatalf("extraction = %#v, want one clarification and no lines", extraction)
	}
	clarification := extraction.Clarifications[0]
	if clarification.IssueType != domain.RFQClarificationMissingQuantity {
		t.Errorf("issue_type = %s, want %s", clarification.IssueType,
			domain.RFQClarificationMissingQuantity)
	}
	if clarification.Question != "Cuantas bolsas de cemento necesitas?" {
		t.Errorf("question = %q, want provider question", clarification.Question)
	}
}

func TestAnthropicRFQExtractor_Extract_RejectsMissingToolUse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"no tool"}]}`))
	}))
	defer server.Close()

	extractor := NewAnthropicRFQExtractor(testAnthropicConfig(server.URL), server.Client())

	_, err := extractor.Extract(context.Background(), "Need cement")
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("Extract() = %v, want %v", err, domain.ErrInvalidInput)
	}
}

func TestAnthropicRFQExtractor_Extract_RejectsInvalidQuantity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{
			"content": [
				{
					"type": "tool_use",
					"name": "record_rfq_extraction",
					"input": {"items": [{"requested_description": "cement", "quantity": "many"}]}
				}
			]
		}`))
	}))
	defer server.Close()

	extractor := NewAnthropicRFQExtractor(testAnthropicConfig(server.URL), server.Client())

	_, err := extractor.Extract(context.Background(), "Need many cement")
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("Extract() = %v, want %v", err, domain.ErrInvalidInput)
	}
}

func TestAnthropicRFQExtractor_Extract_ReturnsProviderErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"type":"invalid_request_error","message":"bad model"}}`))
	}))
	defer server.Close()

	extractor := NewAnthropicRFQExtractor(testAnthropicConfig(server.URL), server.Client())

	_, err := extractor.Extract(context.Background(), "Need cement")
	if err == nil {
		t.Fatal("Extract() = nil error, want provider error")
	}
	if errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("Extract() = %v, provider errors should stay internal", err)
	}
}

func testAnthropicConfig(baseURL string) config.AIConfig {
	return config.AIConfig{
		Provider:              config.AIProviderAnthropic,
		AnthropicAPIKey:       "secret",
		AnthropicBaseURL:      baseURL,
		AnthropicVersion:      "2023-06-01",
		RFQExtractorModel:     "claude-test",
		RFQExtractorMaxTokens: 256,
		RFQExtractorTimeout:   time.Second,
	}
}
