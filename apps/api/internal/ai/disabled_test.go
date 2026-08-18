package ai

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func TestDisabledAdapters_RefuseWithTheDomainError(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		call func() error
		// names is the capability the message has to identify, so an operator reading the log
		// knows which provider was never configured.
		names string
	}{
		{
			name: "generator",
			call: func() error {
				_, err := DisabledGenerator{}.Generate(context.Background(), domain.GenerationRequest{}, nil)
				return err
			},
			names: "language model",
		},
		{
			name: "embedder",
			call: func() error {
				_, err := DisabledEmbedder{}.Embed(context.Background(), []string{"cemento"})
				return err
			},
			names: "embeddings",
		},
		{
			name: "transcriber",
			call: func() error {
				_, err := DisabledTranscriber{}.Transcribe(context.Background(), domain.Audio{})
				return err
			},
			names: "transcription",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.call()
			if !errors.Is(err, domain.ErrAIUnavailable) {
				t.Fatalf("err = %v, want it to match domain.ErrAIUnavailable", err)
			}
			if got := domain.CodeOf(err); got != domain.CodeAIUnavailable {
				t.Fatalf("CodeOf() = %q, want %q", got, domain.CodeAIUnavailable)
			}
			if !strings.Contains(err.Error(), tc.names) {
				t.Fatalf("err = %q, want it to name %q", err, tc.names)
			}
		})
	}
}
