// Package provider binds each AI port to the adapter selected for it. It sits above the
// adapters so every command that needs a model — the server, the catalog backfill — makes the
// same choices from the same settings.
package provider

import (
	"fmt"
	"log/slog"

	"github.com/santialemarino/coti/apps/api/internal/ai"
	"github.com/santialemarino/coti/apps/api/internal/ai/anthropic"
	"github.com/santialemarino/coti/apps/api/internal/ai/openai"
	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// Set is the AI adapters bound at startup. Services take them through the domain ports, so
// none of them ever names a provider.
type Set struct {
	Generator   domain.StructuredGenerator
	Embedder    domain.Embedder
	Transcriber domain.Transcriber
}

// Bind selects an adapter per capability. No provider covers all three, so each is chosen on
// its own and any of them can be left unbound. config.Load rejects a provider with no adapter.
func Bind(cfg config.AIConfig, log *slog.Logger) (Set, error) {
	set := Set{
		Generator:   ai.DisabledGenerator{},
		Embedder:    ai.DisabledEmbedder{},
		Transcriber: ai.DisabledTranscriber{},
	}

	switch cfg.LLMProvider {
	case config.AIProviderDisabled:
		log.Warn("no language model is bound: extraction and the change handler will refuse")
	case config.AIProviderAnthropic:
		set.Generator = anthropic.NewGenerator(cfg.Anthropic(), log)
	default:
		return set, fmt.Errorf("no language model adapter for provider %q", cfg.LLMProvider)
	}

	switch cfg.EmbeddingsProvider {
	case config.AIProviderDisabled:
		log.Warn("no embedding provider is bound: semantic catalog search will refuse")
	case config.AIProviderOpenAI:
		set.Embedder = openai.NewEmbedder(cfg.Embeddings(), log)
	default:
		return set, fmt.Errorf("no embedding adapter for provider %q", cfg.EmbeddingsProvider)
	}

	switch cfg.TranscriptionProvider {
	case config.AIProviderDisabled:
		log.Warn("no transcription provider is bound: audio ingest will refuse")
	case config.AIProviderOpenAI:
		set.Transcriber = openai.NewTranscriber(cfg.Transcription(), log)
	default:
		return set, fmt.Errorf("no transcription adapter for provider %q", cfg.TranscriptionProvider)
	}
	return set, nil
}

// Describe records which adapter ended up behind each port, so a deployment can be read back
// from its own startup log instead of from the environment it was given.
func (s Set) Describe(log *slog.Logger) {
	log.Info("ai providers bound",
		slog.String("language_model", fmt.Sprintf("%T", s.Generator)),
		slog.String("embeddings", fmt.Sprintf("%T", s.Embedder)),
		slog.String("transcription", fmt.Sprintf("%T", s.Transcriber)))
}
