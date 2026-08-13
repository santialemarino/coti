// Package anthropic adapts Claude to the domain StructuredGenerator port: schema-forced
// generation over written messages, photographed lists and attached documents.
package anthropic

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/santialemarino/coti/apps/api/internal/ai"
	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

var _ domain.StructuredGenerator = (*Generator)(nil)

// Generator asks Claude for one schema-valid answer per call.
type Generator struct {
	client *anthropicsdk.Client
	cfg    config.AIConfig
	log    *slog.Logger
}

// NewGenerator builds a Generator from the AI configuration.
func NewGenerator(cfg config.AIConfig, log *slog.Logger) *Generator {
	opts := []option.RequestOption{
		// The API resolves its own credentials. Left to itself the SDK would also read
		// ANTHROPIC_* and any OAuth profile on disk, so a developer's own login could end up
		// paying for the server's calls.
		option.WithoutEnvironmentDefaults(),
		option.WithAPIKey(cfg.AnthropicAPIKey),
		// The retry policy is ours, so the configured attempt count is the real one and the
		// number in the usage log is not a third of the requests actually made.
		option.WithMaxRetries(0),
	}
	if cfg.AnthropicBaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.AnthropicBaseURL))
	}
	client := anthropicsdk.NewClient(opts...)
	return &Generator{client: &client, cfg: cfg, log: log}
}

// Generate sends req to Claude and decodes the answer into out. An answer that does not satisfy
// the schema is rejected and asked for again rather than repaired by hand.
func (g *Generator) Generate(ctx context.Context, req domain.GenerationRequest, out any) (*domain.GenerationUsage, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	blocks, err := contentBlocks(req.Input)
	if err != nil {
		return nil, err
	}

	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = g.cfg.LLMMaxTokens
	}
	params := anthropicsdk.MessageNewParams{
		Model:     anthropicsdk.Model(g.cfg.LLMModel),
		MaxTokens: int64(maxTokens),
		// The stable half of the prompt, marked for caching: a repeat call pays full price only
		// for the variable half that follows it.
		System: []anthropicsdk.TextBlockParam{{
			Text:         req.Instructions,
			CacheControl: anthropicsdk.NewCacheControlEphemeralParam(),
		}},
		Messages: []anthropicsdk.MessageParam{anthropicsdk.NewUserMessage(blocks...)},
		OutputConfig: anthropicsdk.OutputConfigParam{
			Effort: anthropicsdk.OutputConfigEffort(g.cfg.LLMEffort),
			Format: anthropicsdk.JSONOutputFormatParam{Schema: req.Schema},
		},
	}

	usage := domain.GenerationUsage{
		Provider: string(config.AIProviderAnthropic),
		Model:    g.cfg.LLMModel,
	}
	started := time.Now()
	attempts, err := ai.Retry(ctx, g.cfg.Retry, func(ctx context.Context) error {
		ctx, cancel := context.WithTimeout(ctx, g.cfg.LLMTimeout)
		defer cancel()

		message, callErr := g.client.Messages.New(ctx, params)
		if callErr != nil {
			return classify(callErr)
		}
		usage.InputTokens = int(message.Usage.InputTokens)
		usage.OutputTokens = int(message.Usage.OutputTokens)
		usage.CachedInputTokens = int(message.Usage.CacheReadInputTokens)
		return decode(message, out)
	})
	ai.LogCall(ctx, g.log, ai.Call{
		Provider:          usage.Provider,
		Model:             usage.Model,
		Operation:         "generate",
		Attempts:          attempts,
		Elapsed:           time.Since(started),
		InputTokens:       usage.InputTokens,
		OutputTokens:      usage.OutputTokens,
		CachedInputTokens: usage.CachedInputTokens,
	}, err)
	if err != nil {
		return nil, ai.Unavailable(err)
	}
	return &usage, nil
}

// decode reads the answer a schema-forced call returns and unmarshals it into out. A refusal, a
// truncation or a shape the caller does not recognise is rejected here; unknown fields are refused
// too, so a schema drifting away from the struct behind it fails loudly instead of silently
// dropping a field.
func decode(message *anthropicsdk.Message, out any) error {
	switch message.StopReason {
	case anthropicsdk.StopReasonRefusal:
		// Asking again would produce the same refusal, so this one is final.
		return fmt.Errorf("model refused the request (%s)", message.StopDetails.Category)
	case anthropicsdk.StopReasonMaxTokens:
		return ai.Retryable(errors.New("answer hit the token cap before the schema was satisfied"))
	}

	var answer strings.Builder
	for _, block := range message.Content {
		if text, ok := block.AsAny().(anthropicsdk.TextBlock); ok {
			answer.WriteString(text.Text)
		}
	}
	if answer.Len() == 0 {
		return ai.Retryable(errors.New("model returned no answer"))
	}

	decoder := json.NewDecoder(strings.NewReader(answer.String()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return ai.Retryable(fmt.Errorf("answer does not satisfy the schema: %w", err))
	}
	return nil
}

// classify decides whether a provider error is worth another attempt. A rate limit, an overload,
// a server fault or a connection that never produced a response is transient; a request the
// provider rejected outright is not, and repeating it would only spend the allowance.
func classify(err error) error {
	var apiErr *anthropicsdk.Error
	if !errors.As(err, &apiErr) {
		return ai.Retryable(err)
	}
	if apiErr.StatusCode == http.StatusTooManyRequests || apiErr.StatusCode >= http.StatusInternalServerError {
		return ai.Retryable(err)
	}
	return err
}

// contentBlocks maps the domain's input onto the provider's, encoding the binary blocks because
// the wire format carries text.
func contentBlocks(input []domain.Content) ([]anthropicsdk.ContentBlockParamUnion, error) {
	blocks := make([]anthropicsdk.ContentBlockParamUnion, 0, len(input))
	for i, block := range input {
		switch block.Kind {
		case domain.ContentKindText:
			blocks = append(blocks, anthropicsdk.NewTextBlock(block.Text))
		case domain.ContentKindImage:
			blocks = append(blocks, anthropicsdk.NewImageBlockBase64(block.MediaType,
				base64.StdEncoding.EncodeToString(block.Data)))
		case domain.ContentKindDocument:
			switch block.MediaType {
			case "application/pdf":
				blocks = append(blocks, anthropicsdk.NewDocumentBlock(anthropicsdk.Base64PDFSourceParam{
					Data: base64.StdEncoding.EncodeToString(block.Data),
				}))
			case "text/plain":
				blocks = append(blocks, anthropicsdk.NewDocumentBlock(anthropicsdk.PlainTextSourceParam{
					Data: string(block.Data),
				}))
			default:
				return nil, fmt.Errorf("%w: input block %d has document media type %q, which the "+
					"provider does not read", domain.ErrInvalidInput, i, block.MediaType)
			}
		default:
			return nil, fmt.Errorf("%w: input block %d has unknown kind %q",
				domain.ErrInvalidInput, i, block.Kind)
		}
	}
	return blocks, nil
}
