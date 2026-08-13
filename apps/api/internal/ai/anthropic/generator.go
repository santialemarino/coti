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
	"reflect"
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
	cfg    config.AnthropicConfig
	log    *slog.Logger
}

// NewGenerator builds a Generator from the language-model settings.
func NewGenerator(cfg config.AnthropicConfig, log *slog.Logger) *Generator {
	opts := []option.RequestOption{
		// The API resolves its own credentials. Left to itself the SDK would also read
		// ANTHROPIC_* and any OAuth profile on disk, so a developer's own login could end up
		// paying for the server's calls.
		option.WithoutEnvironmentDefaults(),
		option.WithAPIKey(cfg.APIKey),
		// The retry policy is ours, so the configured attempt count is the real one and the
		// number in the usage log is not a third of the requests actually made.
		option.WithMaxRetries(0),
	}
	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
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
	if err := writable(out); err != nil {
		return nil, err
	}
	blocks, err := contentBlocks(req.Input)
	if err != nil {
		return nil, err
	}

	params := anthropicsdk.MessageNewParams{
		Model:     anthropicsdk.Model(g.cfg.Model),
		MaxTokens: int64(g.maxTokens(req.MaxTokens)),
		// The stable half of the prompt, marked for caching: a repeat call pays full price only
		// for the variable half that follows it.
		System: []anthropicsdk.TextBlockParam{{
			Text:         req.Instructions,
			CacheControl: anthropicsdk.NewCacheControlEphemeralParam(),
		}},
		Messages: []anthropicsdk.MessageParam{anthropicsdk.NewUserMessage(blocks...)},
		OutputConfig: anthropicsdk.OutputConfigParam{
			Effort: anthropicsdk.OutputConfigEffort(g.cfg.Effort),
			Format: anthropicsdk.JSONOutputFormatParam{Schema: req.Schema},
		},
	}

	usage := domain.GenerationUsage{
		Provider: string(config.AIProviderAnthropic),
		Model:    g.cfg.Model,
	}
	started := time.Now()
	attempts, err := ai.Retry(ctx, g.cfg.Retry, func(ctx context.Context) error {
		ctx, cancel := context.WithTimeout(ctx, g.cfg.Timeout)
		defer cancel()

		message, callErr := g.client.Messages.New(ctx, params)
		if callErr != nil {
			return classify(callErr)
		}
		// Added rather than assigned: a call that succeeds on its third attempt submitted the
		// prompt three times and was charged for all three.
		usage.InputTokens += int(message.Usage.InputTokens)
		usage.OutputTokens += int(message.Usage.OutputTokens)
		usage.CacheReadTokens += int(message.Usage.CacheReadInputTokens)
		usage.CacheWriteTokens += int(message.Usage.CacheCreationInputTokens)
		return decode(message, out)
	})
	ai.LogCall(ctx, g.log, ai.Call{
		Provider:         usage.Provider,
		Model:            usage.Model,
		Operation:        "generate",
		Attempts:         attempts,
		Elapsed:          time.Since(started),
		InputTokens:      usage.InputTokens,
		OutputTokens:     usage.OutputTokens,
		CacheReadTokens:  usage.CacheReadTokens,
		CacheWriteTokens: usage.CacheWriteTokens,
	}, err)
	if err != nil {
		return nil, ai.Fail(err)
	}
	return &usage, nil
}

// maxTokens bounds a caller's own ceiling by the configured one. The env key is the operational
// limit, so a request may ask for less but never for more.
func (g *Generator) maxTokens(requested int) int {
	if requested <= 0 || requested > g.cfg.MaxTokens {
		return g.cfg.MaxTokens
	}
	return requested
}

// writable reports a caller that passed something an answer cannot be decoded into, before the
// mistake costs a provider round trip.
func writable(out any) error {
	value := reflect.ValueOf(out)
	if !value.IsValid() || value.Kind() != reflect.Pointer || value.IsNil() {
		return fmt.Errorf("%w: generation output must be a non-nil pointer", domain.ErrInvalidInput)
	}
	return nil
}

// decode reads the answer a schema-forced call returns and unmarshals it into out. A refusal, a
// truncation or a shape the caller does not recognise is rejected here; unknown fields are refused
// too, so a schema drifting away from the struct behind it fails loudly instead of silently
// dropping a field.
func decode(message *anthropicsdk.Message, out any) error {
	switch message.StopReason {
	case anthropicsdk.StopReasonRefusal:
		// The provider is healthy and the same prompt would be refused again, so this is final.
		return ai.Rejected(fmt.Errorf("model refused the request (%s)", message.StopDetails.Category))
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

	// Zeroed first: encoding/json assigns as it walks, so a rejected answer leaves its values
	// behind and the next attempt would hand back a struct mixing two answers — a quantity from
	// the answer we threw away.
	reflect.ValueOf(out).Elem().SetZero()

	decoder := json.NewDecoder(strings.NewReader(answer.String()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return ai.Retryable(fmt.Errorf("answer does not satisfy the schema: %w", err))
	}
	return nil
}

// classify decides what kind of failure a provider error is. A rate limit or a provider-side fault
// is transient; a request the provider rejected is our fault and final; a caller who walked away is
// neither, and must not be reported as an outage it should retry.
func classify(err error) error {
	if errors.Is(err, context.Canceled) {
		return err
	}

	var apiErr *anthropicsdk.Error
	if !errors.As(err, &apiErr) {
		// No HTTP response at all: our own deadline, or a dial, TLS or read failure. Retried
		// rather than sorted further, because a blip on the wire is far likelier here than an
		// unsendable request — req.Validate already caught those before any of this ran.
		return ai.Retryable(err)
	}
	if !ai.RetryableStatus(apiErr.StatusCode) {
		return ai.Rejected(err)
	}
	if apiErr.Response != nil {
		// The provider's own window beats our ladder: 1s and 2s are nothing against a real
		// rate-limit window, and three attempts inside it would all fail.
		if after := ai.RetryAfter(apiErr.Response.Header); after > 0 {
			return ai.RetryableAfter(err, after)
		}
	}
	return ai.Retryable(err)
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
