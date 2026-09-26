package ai

import (
	"context"
	"log/slog"
	"time"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// Call is one provider round trip as the adapter measured it. The counts are the sum over every
// attempt the call took, because every attempt was charged.
type Call struct {
	Provider string
	Model    string
	// Method is the provider method called; what the call was paid for comes from the context.
	Method   string
	Attempts int
	Elapsed  time.Duration
	// The token counts are zero where the provider reports none.
	InputTokens      int
	OutputTokens     int
	CacheReadTokens  int
	CacheWriteTokens int
	AudioSeconds     float64
}

// Meter reports what every provider call consumed, on success and on failure alike: to the log
// always, and to the usage ledger when one is bound.
type Meter struct {
	log      *slog.Logger
	recorder domain.AIUsageRecorder
}

// NewMeter builds a Meter. A nil recorder leaves the log as the only record.
func NewMeter(log *slog.Logger, recorder domain.AIUsageRecorder) *Meter {
	if log == nil {
		log = slog.Default()
	}
	return &Meter{log: log, recorder: recorder}
}

// Observe reports one call, attributed to the scope ctx carries.
func (m *Meter) Observe(ctx context.Context, call Call, err error) {
	scope := domain.AIUsageScopeFrom(ctx)
	fields := []any{
		slog.String("provider", call.Provider),
		slog.String("model", call.Model),
		slog.String("method", call.Method),
		slog.String("operation", string(scope.Operation)),
		slog.String("account_id", scope.AccountID.String()),
		slog.Int("attempts", call.Attempts),
		slog.Duration("elapsed", call.Elapsed),
		slog.Int("input_tokens", call.InputTokens),
		slog.Int("output_tokens", call.OutputTokens),
		slog.Int("cache_read_tokens", call.CacheReadTokens),
		slog.Int("cache_write_tokens", call.CacheWriteTokens),
		slog.Float64("audio_seconds", call.AudioSeconds),
	}
	if scope.RFQID != nil {
		fields = append(fields, slog.String("rfq_id", scope.RFQID.String()))
	}
	if err != nil {
		m.log.WarnContext(ctx, "ai call failed", append(fields, slog.Any("error", err))...)
	} else {
		m.log.InfoContext(ctx, "ai call", fields...)
	}

	if m.recorder == nil {
		return
	}
	m.recorder.Record(ctx, domain.AIUsage{
		AIUsageScope:     scope,
		Provider:         call.Provider,
		Model:            call.Model,
		Succeeded:        err == nil,
		Attempts:         call.Attempts,
		Elapsed:          call.Elapsed,
		InputTokens:      call.InputTokens,
		OutputTokens:     call.OutputTokens,
		CacheReadTokens:  call.CacheReadTokens,
		CacheWriteTokens: call.CacheWriteTokens,
		AudioSeconds:     call.AudioSeconds,
	})
}
