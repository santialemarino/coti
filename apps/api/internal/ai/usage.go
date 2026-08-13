package ai

import (
	"context"
	"log/slog"
	"time"
)

// Call is one provider round trip, as the usage log records it.
type Call struct {
	Provider  string
	Model     string
	Operation string
	Attempts  int
	Elapsed   time.Duration
	// The token counts are zero where the provider reports none, as transcription does.
	InputTokens       int
	OutputTokens      int
	CachedInputTokens int
}

// LogCall records what one provider round trip consumed. Every adapter calls it on success and on
// failure alike: the pilot's AI spend has to be measurable per operation, and a call that failed
// after three attempts still cost something.
func LogCall(ctx context.Context, log *slog.Logger, call Call, err error) {
	fields := []any{
		slog.String("provider", call.Provider),
		slog.String("model", call.Model),
		slog.String("operation", call.Operation),
		slog.Int("attempts", call.Attempts),
		slog.Duration("elapsed", call.Elapsed),
		slog.Int("input_tokens", call.InputTokens),
		slog.Int("output_tokens", call.OutputTokens),
		slog.Int("cached_input_tokens", call.CachedInputTokens),
	}
	if err != nil {
		log.WarnContext(ctx, "ai call failed", append(fields, slog.Any("error", err))...)
		return
	}
	log.InfoContext(ctx, "ai call", fields...)
}
