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

func TestMeter_WithoutARecorderOnlyLogs(t *testing.T) {
	NewMeter(slog.New(slog.DiscardHandler), nil).Observe(context.Background(), Call{}, nil)
}
