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

// Extract parses informal RFQ text into line items.
func (e *AnthropicRFQExtractor) Extract(ctx context.Context, raw string) ([]domain.ExtractedRFQLine, error) {
	ctx, cancel := context.WithTimeout(ctx, e.cfg.RFQExtractorTimeout)
	defer cancel()

	body, err := json.Marshal(e.request(raw))
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.messagesURL(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("x-api-key", e.cfg.AnthropicAPIKey)
	req.Header.Set("anthropic-version", e.cfg.AnthropicVersion)

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, anthropicStatusError(resp.StatusCode, respBody)
	}

	var out anthropicMessageResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, err
	}
	input, err := toolInput(out)
	if err != nil {
		return nil, err
	}
	return extractedLines(input)
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
	Items []rfqExtractionItem `json:"items"`
}

type rfqExtractionItem struct {
	RequestedDescription string  `json:"requested_description"`
	Quantity             string  `json:"quantity"`
	Unit                 *string `json:"unit"`
	QuantityRationale    *string `json:"quantity_rationale"`
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
			"Do not infer a default quantity and do not price, match, discount, or contact anyone.",
			"Omit incomplete line items until clarification persistence exists.",
		}, " "),
		Messages: []anthropicMessage{
			{Role: "user", Content: raw},
		},
		Tools: []anthropicTool{{
			Name:        rfqExtractionToolName,
			Description: "Records complete RFQ material lines extracted from the client's message. Use it only for materials with a positive explicit quantity. Do not include prices, catalog matches, or clarification questions.",
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
		"required":             []string{"items"},
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

func extractedLines(input rfqExtractionInput) ([]domain.ExtractedRFQLine, error) {
	lines := make([]domain.ExtractedRFQLine, 0, len(input.Items))
	for i, item := range input.Items {
		quantity, err := decimal.NewFromString(item.Quantity)
		if err != nil {
			return nil, fmt.Errorf("%w: items[%d].quantity is not a decimal", domain.ErrInvalidInput, i)
		}
		lines = append(lines, domain.ExtractedRFQLine{
			RequestedDescription: item.RequestedDescription,
			Quantity:             quantity,
			Unit:                 item.Unit,
			QuantityRationale:    item.QuantityRationale,
		})
	}
	return lines, nil
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
