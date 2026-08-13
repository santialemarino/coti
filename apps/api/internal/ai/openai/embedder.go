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
	cfg    config.EmbeddingsConfig
	log    *slog.Logger
}

// NewEmbedder builds an Embedder from the embedding settings.
func NewEmbedder(cfg config.EmbeddingsConfig, log *slog.Logger) *Embedder {
	return &Embedder{
		client: &http.Client{Timeout: cfg.Timeout},
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

// Embed vectorizes texts, returning vectors index-aligned with the input.
//
// The list is sent in batches: a catalog import hands over thousands of descriptions at once, and
// one request carrying all of them would exceed the provider's per-request limits and be rejected
// wholesale, losing every vector in the batch.
func (e *Embedder) Embed(ctx context.Context, texts []string) ([]pgvector.Vector, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	for i, text := range texts {
		if text == "" {
			return nil, fmt.Errorf("%w: text %d is empty", domain.ErrInvalidInput, i)
		}
	}

	batch := e.cfg.BatchSize
	if batch < 1 {
		batch = len(texts)
	}

	var (
		vectors = make([]pgvector.Vector, 0, len(texts))
		tokens  int
		batches int
		err     error
	)
	started := time.Now()
	attempts := 0
	for start := 0; start < len(texts) && err == nil; start += batch {
		end := min(start+batch, len(texts))
		chunk := texts[start:end]
		batches++

		var chunkVectors []pgvector.Vector
		var chunkAttempts int
		chunkAttempts, err = ai.Retry(ctx, e.cfg.Retry, func(ctx context.Context) error {
			var reply embedReply
			if callErr := e.request(ctx, chunk, &reply); callErr != nil {
				return callErr
			}
			// Added rather than assigned: every attempt submitted the text and was charged.
			tokens += reply.Usage.PromptTokens
			var readErr error
			chunkVectors, readErr = readVectors(reply, len(chunk))
			return readErr
		})
		attempts += chunkAttempts
		if err == nil {
			vectors = append(vectors, chunkVectors...)
		}
	}

	ai.LogCall(ctx, e.log, ai.Call{
		Provider:  string(config.AIProviderOpenAI),
		Model:     e.cfg.Model,
		Operation: "embed",
		// Summed over the batches, so the count reflects the requests actually made.
		Attempts:    attempts,
		Elapsed:     time.Since(started),
		InputTokens: tokens,
	}, err)
	if err != nil {
		return nil, ai.Fail(fmt.Errorf("embedding batch %d of %d: %w", batches, batchCount(len(texts), batch), err))
	}
	return vectors, nil
}

// request sends one batch.
func (e *Embedder) request(ctx context.Context, chunk []string, reply *embedReply) error {
	return post(ctx, e.client, e.cfg.BaseURL, e.cfg.APIKey, "/embeddings", embedRequest{
		Model:      e.cfg.Model,
		Input:      chunk,
		Dimensions: domain.EmbeddingDimension,
	}, reply)
}

// batchCount is how many requests a list of this length takes.
func batchCount(texts, batch int) int {
	if batch < 1 {
		return 1
	}
	return (texts + batch - 1) / batch
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
			return nil, ai.Rejected(fmt.Errorf("embedding index %d falls outside the %d texts asked for",
				item.Index, want))
		}
		if len(item.Embedding) != domain.EmbeddingDimension {
			return nil, ai.Rejected(fmt.Errorf("embedding has %d dimensions, the catalog column holds %d",
				len(item.Embedding), domain.EmbeddingDimension))
		}
		vectors[item.Index] = pgvector.NewVector(item.Embedding)
		seen[item.Index] = true
	}
	for i, ok := range seen {
		if !ok {
			return nil, ai.Rejected(fmt.Errorf("no embedding came back for text %d", i))
		}
	}
	return vectors, nil
}
