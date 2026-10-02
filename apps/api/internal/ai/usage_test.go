package ai

import (
	"context"
	"errors"
	"log/slog"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// usageLedger keeps what the meter recorded.
type usageLedger struct {
	mu      sync.Mutex
	entries []domain.AIUsage
}

func (l *usageLedger) Record(_ context.Context, usage domain.AIUsage) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, usage)
}

func (l *usageLedger) only(t *testing.T) domain.AIUsage {
	t.Helper()
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.entries) != 1 {
		t.Fatalf("recorded %d calls, want exactly 1: %+v", len(l.entries), l.entries)
	}
	return l.entries[0]
}

func TestMeter_RecordsTheCallUnderTheScopeTheContextCarries(t *testing.T) {
	branchID, rfqID := uuid.New(), uuid.New()
	tenant := domain.Tenant{AccountID: uuid.New(), BranchID: branchID}
	ctx := domain.WithAIOperation(domain.WithAIRFQ(domain.WithAIAccount(context.Background(),
		tenant), rfqID), domain.AIOperationCatalogMatchReview)
	ledger := &usageLedger{}

	NewMeter(slog.New(slog.DiscardHandler), ledger).Observe(ctx, Call{
		Provider: "anthropic", Model: "claude-opus-5", Method: "generate", Attempts: 2,
		Elapsed: 3 * time.Second, InputTokens: 900, OutputTokens: 120, CacheReadTokens: 400,
		CacheWriteTokens: 200,
	}, nil)

	got := ledger.only(t)
	want := domain.AIUsage{
		AIUsageScope: domain.AIUsageScope{AccountID: tenant.AccountID, BranchID: &branchID,
			RFQID: &rfqID, Operation: domain.AIOperationCatalogMatchReview},
		Provider: "anthropic", Model: "claude-opus-5", Succeeded: true, Attempts: 2,
		Elapsed: 3 * time.Second, InputTokens: 900, OutputTokens: 120, CacheReadTokens: 400,
		CacheWriteTokens: 200,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("usage = %+v, want %+v", got, want)
	}
}

// A call that failed after its attempts was still charged for them, so it is recorded as spend.
func TestMeter_RecordsAFailedCallAsSpend(t *testing.T) {
	ctx := domain.WithAIOperation(domain.WithAIAccount(context.Background(),
		domain.Tenant{AccountID: uuid.New()}), domain.AIOperationCatalogSearch)
	ledger := &usageLedger{}

	NewMeter(slog.New(slog.DiscardHandler), ledger).Observe(ctx, Call{
		Provider: "openai", Model: "text-embedding-3-small", Method: "embed", Attempts: 3,
		InputTokens: 36,
	}, errors.New("provider unavailable"))

	got := ledger.only(t)
	if got.Succeeded || got.Attempts != 3 || got.InputTokens != 36 {
		t.Errorf("usage = %+v, want a failed call with its three charged attempts", got)
	}
	if got.BranchID != nil || got.RFQID != nil {
		t.Errorf("scope = %+v, want no branch and no order where none was named", got.AIUsageScope)
	}
}

// A call whose context was already done made no attempt, so it cost nothing: logged, not recorded.
func TestMeter_RecordsNothingForACallThatNeverReachedTheProvider(t *testing.T) {
	ctx := domain.WithAIOperation(domain.WithAIAccount(context.Background(),
		domain.Tenant{AccountID: uuid.New()}), domain.AIOperationCatalogMatchReview)
	ledger, records := &usageLedger{}, &countingHandler{}

	NewMeter(slog.New(records), ledger).Observe(ctx, Call{Provider: "anthropic", Attempts: 0},
		context.Canceled)

	if len(ledger.entries) != 0 {
		t.Errorf("recorded %+v, want nothing for a call with no attempt", ledger.entries)
	}
	if records.count != 1 {
		t.Errorf("log records = %d, want the call still logged", records.count)
	}
}

func TestMeter_WithoutARecorderStillLogs(t *testing.T) {
	records := &countingHandler{}
	NewMeter(slog.New(records), nil).Observe(context.Background(), Call{Attempts: 1}, nil)
	if records.count != 1 {
		t.Errorf("log records = %d, want 1", records.count)
	}
}

// countingHandler counts the records written to it.
type countingHandler struct {
	mu    sync.Mutex
	count int
}

func (h *countingHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *countingHandler) Handle(context.Context, slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.count++
	return nil
}

func (h *countingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }

func (h *countingHandler) WithGroup(string) slog.Handler { return h }
