package openai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/santialemarino/coti/apps/api/internal/ai"
	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

var _ domain.StructuredGenerator = (*Generator)(nil)

// Generator produces schema-constrained answers through the Responses API.
type Generator struct {
	client *http.Client
	cfg    config.OpenAIGenerationConfig
	meter  *ai.Meter
}

// NewGenerator builds the language-model adapter with explicit server credentials.
func NewGenerator(cfg config.OpenAIGenerationConfig, meter *ai.Meter) *Generator {
	return &Generator{client: &http.Client{Timeout: cfg.Timeout}, cfg: cfg, meter: meter}
}

type generationReply struct {
	Status string `json:"status"`
	Output []struct {
		Type    string `json:"type"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"output"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
		InputDetails struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"input_tokens_details"`
	} `json:"usage"`
}

// Generate validates and decodes a forced-schema response without retaining provider-side state.
func (g *Generator) Generate(ctx context.Context, req domain.GenerationRequest, out any) (*domain.GenerationUsage, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	value := reflect.ValueOf(out)
	if !value.IsValid() || value.Kind() != reflect.Pointer || value.IsNil() {
		return nil, fmt.Errorf("%w: generation output must be a non-nil pointer", domain.ErrInvalidInput)
	}
	content, err := generationContent(req.Input)
	if err != nil {
		return nil, err
	}
	limit := req.MaxTokens
	if limit <= 0 || limit > g.cfg.MaxTokens {
		limit = g.cfg.MaxTokens
	}
	body := map[string]any{
		"model": g.cfg.Model, "instructions": req.Instructions, "store": false,
		"input":             []any{map[string]any{"role": "user", "content": content}},
		"max_output_tokens": limit,
		"text": map[string]any{"format": map[string]any{
			"type": "json_schema", "name": "answer", "strict": true, "schema": req.Schema,
		}},
	}
	usage := domain.GenerationUsage{Provider: string(config.AIProviderOpenAI), Model: g.cfg.Model}
	started := time.Now()
	attempts, err := ai.Retry(ctx, g.cfg.Retry, func(ctx context.Context) error {
		var reply generationReply
		if err := post(ctx, g.client, g.cfg.BaseURL, g.cfg.APIKey, "/responses", body, &reply); err != nil {
			return err
		}
		usage.InputTokens += reply.Usage.InputTokens - reply.Usage.InputDetails.CachedTokens
		usage.OutputTokens += reply.Usage.OutputTokens
		usage.CacheReadTokens += reply.Usage.InputDetails.CachedTokens
		return decodeGeneration(reply, out)
	})
	g.meter.Observe(ctx, ai.Call{Provider: usage.Provider, Model: usage.Model, Method: "generate",
		Attempts: attempts, Elapsed: time.Since(started), InputTokens: usage.InputTokens,
		OutputTokens: usage.OutputTokens, CacheReadTokens: usage.CacheReadTokens}, err)
	if err != nil {
		return nil, ai.Fail(err)
	}
	return &usage, nil
}

func decodeGeneration(reply generationReply, out any) error {
	var answer strings.Builder
	for _, item := range reply.Output {
		if item.Type != "message" {
			continue
		}
		for _, content := range item.Content {
			if content.Type == "refusal" {
				return ai.Rejected(errors.New("model refused the request"))
			}
			if content.Type == "output_text" {
				answer.WriteString(content.Text)
			}
		}
	}
	if reply.Status != "completed" || answer.Len() == 0 {
		return ai.Retryable(errors.New("model response is incomplete or empty"))
	}
	reflect.ValueOf(out).Elem().SetZero()
	decoder := json.NewDecoder(strings.NewReader(answer.String()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return ai.Retryable(fmt.Errorf("decode structured answer: %w", err))
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return ai.Retryable(errors.New("structured answer contains trailing data"))
	}
	return nil
}

func generationContent(input []domain.Content) ([]map[string]any, error) {
	content := make([]map[string]any, 0, len(input))
	for _, block := range input {
		switch block.Kind {
		case domain.ContentKindText:
			content = append(content, map[string]any{"type": "input_text", "text": block.Text})
		case domain.ContentKindImage:
			content = append(content, map[string]any{"type": "input_image", "image_url": "data:" + block.MediaType + ";base64," + base64.StdEncoding.EncodeToString(block.Data)})
		case domain.ContentKindDocument:
			switch block.MediaType {
			case "text/plain":
				content = append(content, map[string]any{"type": "input_text", "text": string(block.Data)})
			case "application/pdf":
				content = append(content, map[string]any{"type": "input_file", "filename": "attachment.pdf",
					"file_data": "data:application/pdf;base64," + base64.StdEncoding.EncodeToString(block.Data)})
			default:
				return nil, fmt.Errorf("%w: unsupported document media type %q", domain.ErrInvalidInput, block.MediaType)
			}
		default:
			return nil, fmt.Errorf("%w: unsupported content kind %q", domain.ErrInvalidInput, block.Kind)
		}
	}
	return content, nil
}
