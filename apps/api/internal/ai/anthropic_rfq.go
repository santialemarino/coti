package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

const anthropicMessagesPath = "/v1/messages"
const rfqExtractionToolName = "record_rfq_extraction"

// AnthropicRFQExtractor extracts RFQ line items with Claude strict tool use.
type AnthropicRFQExtractor struct {
	cfg    config.AIConfig
	client *http.Client
}

// NewAnthropicRFQExtractor builds an AnthropicRFQExtractor.
func NewAnthropicRFQExtractor(cfg config.AIConfig, client *http.Client) *AnthropicRFQExtractor {
	if client == nil {
		client = &http.Client{Timeout: cfg.RFQExtractorTimeout}
	}
	return &AnthropicRFQExtractor{cfg: cfg, client: client}
}

// Extract parses informal RFQ text into complete lines and blocking clarifications.
func (e *AnthropicRFQExtractor) Extract(ctx context.Context, raw string) (domain.RFQExtraction, error) {
	ctx, cancel := context.WithTimeout(ctx, e.cfg.RFQExtractorTimeout)
	defer cancel()

	body, err := json.Marshal(e.request(raw))
	if err != nil {
		return domain.RFQExtraction{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.messagesURL(), bytes.NewReader(body))
	if err != nil {
		return domain.RFQExtraction{}, err
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("x-api-key", e.cfg.AnthropicAPIKey)
	req.Header.Set("anthropic-version", e.cfg.AnthropicVersion)

	resp, err := e.client.Do(req)
	if err != nil {
		return domain.RFQExtraction{}, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return domain.RFQExtraction{}, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return domain.RFQExtraction{}, anthropicStatusError(resp.StatusCode, respBody)
	}

	var out anthropicMessageResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return domain.RFQExtraction{}, err
	}
	input, err := toolInput(out)
	if err != nil {
		return domain.RFQExtraction{}, err
	}
	return extractionResult(input)
}

type anthropicMessageRequest struct {
	Model      string              `json:"model"`
	MaxTokens  int                 `json:"max_tokens"`
	System     string              `json:"system"`
	Messages   []anthropicMessage  `json:"messages"`
	Tools      []anthropicTool     `json:"tools"`
	ToolChoice anthropicToolChoice `json:"tool_choice"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
	Strict      bool           `json:"strict"`
}

type anthropicToolChoice struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

type anthropicMessageResponse struct {
	Content []anthropicContentBlock `json:"content"`
}

type anthropicContentBlock struct {
	Type  string          `json:"type"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

type rfqExtractionInput struct {
	Items          []rfqExtractionItem          `json:"items"`
	Clarifications []rfqExtractionClarification `json:"clarifications"`
}

type rfqExtractionItem struct {
	RequestedDescription string  `json:"requested_description"`
	Quantity             string  `json:"quantity"`
	Unit                 *string `json:"unit"`
	QuantityRationale    *string `json:"quantity_rationale"`
}

type rfqExtractionClarification struct {
	IssueType            domain.RFQClarificationIssueType `json:"issue_type"`
	RequestedDescription string                           `json:"requested_description"`
	Question             string                           `json:"question"`
	Reason               string                           `json:"reason"`
}

type anthropicErrorResponse struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

func (e *AnthropicRFQExtractor) request(raw string) anthropicMessageRequest {
	return anthropicMessageRequest{
		Model:     e.cfg.RFQExtractorModel,
		MaxTokens: e.cfg.RFQExtractorMaxTokens,
		System: strings.Join([]string{
			"You extract construction-material RFQs for Coti.",
			"Return only the forced tool call.",
			"Extract only line items whose quantity is explicit or directly computable from the message.",
			"Do not infer default quantities, units, presentations, or product specifications.",
			"When a missing or unclear quantity, unit, presentation, or product description blocks a line, omit that line from items and propose one concise clarification question in the client's language.",
			"Do not ask for contact details or work type, and do not price, match, discount, or contact anyone.",
		}, " "),
		Messages: []anthropicMessage{
			{Role: "user", Content: raw},
		},
		Tools: []anthropicTool{{
			Name:        rfqExtractionToolName,
			Description: "Records complete RFQ material lines and reviewable questions for blocking ambiguities. Do not include prices, catalog matches, or outbound messages.",
			InputSchema: rfqExtractionSchema(),
			Strict:      true,
		}},
		ToolChoice: anthropicToolChoice{Type: "tool", Name: rfqExtractionToolName},
	}
}

func (e *AnthropicRFQExtractor) messagesURL() string {
	return strings.TrimRight(e.cfg.AnthropicBaseURL, "/") + anthropicMessagesPath
}

func rfqExtractionSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"items", "clarifications"},
		"properties": map[string]any{
			"items": map[string]any{
				"type":     "array",
				"minItems": 0,
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"requested_description", "quantity"},
					"properties": map[string]any{
						"requested_description": map[string]any{
							"type":        "string",
							"minLength":   1,
							"maxLength":   512,
							"description": "The exact material phrase from the client message, cleaned only for surrounding whitespace.",
						},
						"quantity": map[string]any{
							"type":        "string",
							"description": "Positive decimal quantity as a string with at most two decimals.",
						},
						"unit": map[string]any{
							"type":        "string",
							"maxLength":   64,
							"description": "Unit text from the message, such as bag, m2, m3, kg, or unit. Omit when absent.",
						},
						"quantity_rationale": map[string]any{
							"type":        "string",
							"maxLength":   512,
							"description": "Brief reason for the extracted quantity. Omit when it only repeats the quantity.",
						},
					},
				},
			},
			"clarifications": map[string]any{
				"type":     "array",
				"minItems": 0,
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required": []string{
						"issue_type", "requested_description", "question", "reason",
					},
					"properties": map[string]any{
						"issue_type": map[string]any{
							"type": "string",
							"enum": []string{
								string(domain.RFQClarificationMissingQuantity),
								string(domain.RFQClarificationMissingUnit),
								string(domain.RFQClarificationMissingPresentation),
								string(domain.RFQClarificationAmbiguousDescription),
							},
						},
						"requested_description": map[string]any{
							"type":        "string",
							"minLength":   1,
							"maxLength":   512,
							"description": "The incomplete or ambiguous material phrase from the client message.",
						},
						"question": map[string]any{
							"type":        "string",
							"minLength":   1,
							"maxLength":   512,
							"description": "One concise clarification question in the client's language.",
						},
						"reason": map[string]any{
							"type":        "string",
							"minLength":   1,
							"maxLength":   512,
							"description": "Why the missing or unclear value blocks a reliable RFQ line.",
						},
					},
				},
			},
		},
	}
}

func toolInput(out anthropicMessageResponse) (rfqExtractionInput, error) {
	for _, block := range out.Content {
		if block.Type != "tool_use" || block.Name != rfqExtractionToolName {
			continue
		}
		var input rfqExtractionInput
		if err := json.Unmarshal(block.Input, &input); err != nil {
			return rfqExtractionInput{}, fmt.Errorf("%w: invalid rfq extractor tool input: %v",
				domain.ErrInvalidInput, err)
		}
		return input, nil
	}
	return rfqExtractionInput{}, fmt.Errorf("%w: rfq extractor returned no tool call",
		domain.ErrInvalidInput)
}

func extractionResult(input rfqExtractionInput) (domain.RFQExtraction, error) {
	lines := make([]domain.ExtractedRFQLine, 0, len(input.Items))
	for i, item := range input.Items {
		quantity, err := decimal.NewFromString(item.Quantity)
		if err != nil {
			return domain.RFQExtraction{}, fmt.Errorf("%w: items[%d].quantity is not a decimal", domain.ErrInvalidInput, i)
		}
		lines = append(lines, domain.ExtractedRFQLine{
			RequestedDescription: item.RequestedDescription,
			Quantity:             quantity,
			Unit:                 item.Unit,
			QuantityRationale:    item.QuantityRationale,
		})
	}
	clarifications := make([]domain.ProposedRFQClarification, 0, len(input.Clarifications))
	for _, clarification := range input.Clarifications {
		clarifications = append(clarifications, domain.ProposedRFQClarification{
			IssueType:            clarification.IssueType,
			RequestedDescription: clarification.RequestedDescription,
			Question:             clarification.Question,
			Reason:               clarification.Reason,
		})
	}
	return domain.RFQExtraction{Lines: lines, Clarifications: clarifications}, nil
}

func anthropicStatusError(status int, body []byte) error {
	var apiErr anthropicErrorResponse
	if err := json.Unmarshal(body, &apiErr); err == nil && apiErr.Error.Message != "" {
		return fmt.Errorf("anthropic rfq extractor returned %d: %s", status, apiErr.Error.Message)
	}
	bodyText := strings.TrimSpace(string(body))
	if bodyText == "" {
		return fmt.Errorf("anthropic rfq extractor returned %d", status)
	}
	return fmt.Errorf("anthropic rfq extractor returned %d: %s", status, bodyText)
}

var _ domain.RFQExtractor = (*AnthropicRFQExtractor)(nil)
var _ domain.RFQExtractor = (*DisabledRFQExtractor)(nil)
