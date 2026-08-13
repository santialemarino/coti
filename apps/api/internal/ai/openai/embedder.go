package openai

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/pgvector/pgvector-go"

	"github.com/santialemarino/coti/apps/api/internal/ai"
	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

var _ domain.Embedder = (*Embedder)(nil)

// Embedder vectorizes catalog and RFQ text for semantic search.
type Embedder struct {
	client *http.Client
	cfg    config.AIConfig
	log    *slog.Logger
}

// NewEmbedder builds an Embedder from the AI configuration.
func NewEmbedder(cfg config.AIConfig, log *slog.Logger) *Embedder {
	return &Embedder{
		client: &http.Client{Timeout: cfg.EmbeddingsTimeout},
		cfg:    cfg,
		log:    log,
	}
}

// embedRequest is the body of POST /embeddings.
type embedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
	// Dimensions is asked for explicitly so a model whose native width is wider still returns
	// vectors the catalog column can hold.
	Dimensions int `json:"dimensions"`
}

// embedReply is the answer to POST /embeddings. The entries carry their own index because the
// provider does not promise to return them in the order they were asked for.
type embedReply struct {
	Data []struct {
		Index     int       `json:"index"`
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
}

// Embed vectorizes texts in one call, returning vectors index-aligned with the input.
func (e *Embedder) Embed(ctx context.Context, texts []string) ([]pgvector.Vector, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	for i, text := range texts {
		if text == "" {
			return nil, fmt.Errorf("%w: text %d is empty", domain.ErrInvalidInput, i)
		}
	}

	body := embedRequest{
		Model:      e.cfg.EmbeddingsModel,
		Input:      texts,
		Dimensions: domain.EmbeddingDimension,
	}
	var (
		vectors []pgvector.Vector
		tokens  int
	)
	started := time.Now()
	attempts, err := ai.Retry(ctx, e.cfg.Retry, func(ctx context.Context) error {
		var reply embedReply
		if err := post(ctx, e.client, e.cfg.OpenAIBaseURL, e.cfg.OpenAIAPIKey,
			"/embeddings", body, &reply); err != nil {
			return err
		}
		tokens = reply.Usage.PromptTokens
		var readErr error
		vectors, readErr = readVectors(reply, len(texts))
		return readErr
	})
	ai.LogCall(ctx, e.log, ai.Call{
		Provider:    string(config.AIProviderOpenAI),
		Model:       e.cfg.EmbeddingsModel,
		Operation:   "embed",
		Attempts:    attempts,
		Elapsed:     time.Since(started),
		InputTokens: tokens,
	}, err)
	if err != nil {
		return nil, ai.Unavailable(err)
	}
	return vectors, nil
}

// readVectors places each returned embedding at the index it was asked for, and refuses a width
// the catalog column cannot hold — that is a model or configuration mismatch, which another
// attempt would reproduce.
func readVectors(reply embedReply, want int) ([]pgvector.Vector, error) {
	if len(reply.Data) == 0 {
		return nil, ai.Retryable(errEmptyReply)
	}
	if len(reply.Data) != want {
		return nil, ai.Retryable(fmt.Errorf("asked for %d embeddings, got %d", want, len(reply.Data)))
	}

	vectors := make([]pgvector.Vector, want)
	seen := make([]bool, want)
	for _, item := range reply.Data {
		if item.Index < 0 || item.Index >= want {
			return nil, fmt.Errorf("embedding index %d falls outside the %d texts asked for",
				item.Index, want)
		}
		if len(item.Embedding) != domain.EmbeddingDimension {
			return nil, fmt.Errorf("embedding has %d dimensions, the catalog column holds %d",
				len(item.Embedding), domain.EmbeddingDimension)
		}
		vectors[item.Index] = pgvector.NewVector(item.Embedding)
		seen[item.Index] = true
	}
	for i, ok := range seen {
		if !ok {
			return nil, fmt.Errorf("no embedding came back for text %d", i)
		}
	}
	return vectors, nil
}
