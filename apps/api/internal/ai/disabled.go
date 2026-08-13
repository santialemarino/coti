package ai

import (
	"context"
	"errors"

	"github.com/pgvector/pgvector-go"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// The three stand-ins refuse rather than answer, so an environment with no provider keys still
// boots and the engine fails on the call it needed a model for instead of at startup.
var (
	_ domain.StructuredGenerator = DisabledGenerator{}
	_ domain.Embedder            = DisabledEmbedder{}
	_ domain.Transcriber         = DisabledTranscriber{}
)

// DisabledGenerator stands in when no language-model provider is bound.
type DisabledGenerator struct{}

// Generate refuses, because there is no model to ask.
func (DisabledGenerator) Generate(_ context.Context, _ domain.GenerationRequest, _ any) (*domain.GenerationUsage, error) {
	return nil, unbound("language model")
}

// DisabledEmbedder stands in when no embedding provider is bound.
type DisabledEmbedder struct{}

// Embed refuses, because there is no provider to vectorize with.
func (DisabledEmbedder) Embed(_ context.Context, _ []string) ([]pgvector.Vector, error) {
	return nil, unbound("embeddings")
}

// DisabledTranscriber stands in when no transcription provider is bound.
type DisabledTranscriber struct{}

// Transcribe refuses, because there is no provider to transcribe with.
func (DisabledTranscriber) Transcribe(_ context.Context, _ domain.Audio) (string, error) {
	return "", unbound("transcription")
}

func unbound(capability string) error {
	return Unavailable(errors.New("no " + capability + " provider is configured"))
}
