package services

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// fakeAIUsageRepository keeps what reached the ledger and whether its context was still alive.
type fakeAIUsageRepository struct {
	created  []domain.AIUsage
	ctxErrs  []error
	failWith error
}

func (f *fakeAIUsageRepository) Create(ctx context.Context, _ repository.Querier,
	usage domain.AIUsage) error {
	f.ctxErrs = append(f.ctxErrs, ctx.Err())
	if f.failWith != nil {
		return f.failWith
	}
	f.created = append(f.created, usage)
	return nil
}

func attributedUsage() domain.AIUsage {
	return domain.AIUsage{
		AIUsageScope: domain.AIUsageScope{AccountID: testAccountID,
			Operation: domain.AIOperationRFQExtraction},
		Provider: "anthropic", Model: "claude-opus-5", Succeeded: true, Attempts: 1,
		InputTokens: 1200, OutputTokens: 300,
	}
}

// A request that timed out still paid for the call it was waiting on, so its cancellation must not
// cancel the write that records it.
func TestAIUsageService_RecordsUnderTheCallsAccountAfterTheCallerGaveUp(t *testing.T) {
	db, repo := &fakeDB{}, &fakeAIUsageRepository{}
	service := NewAIUsageService(db, repo, time.Second, slog.New(slog.DiscardHandler))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	service.Record(ctx, attributedUsage())

	if len(repo.created) != 1 || repo.created[0] != attributedUsage() {
		t.Fatalf("created = %+v, want the usage recorded", repo.created)
	}
	if repo.ctxErrs[0] != nil {
		t.Errorf("write context = %v, want it detached from the caller's cancellation",
			repo.ctxErrs[0])
	}
	if len(db.scopes) != 1 || db.scopes[0] != testAccountID {
		t.Errorf("transaction scopes = %v, want the usage's own account", db.scopes)
	}
}

// A call site that forgot to say whom it spends for is a bug to see, so it is refused loudly.
func TestAIUsageService_RefusesUsageWithoutAttributionLoudly(t *testing.T) {
	for name, usage := range map[string]domain.AIUsage{
		"no account":   {AIUsageScope: domain.AIUsageScope{Operation: domain.AIOperationCatalogSearch}},
		"no operation": {AIUsageScope: domain.AIUsageScope{AccountID: uuid.New()}},
	} {
		t.Run(name, func(t *testing.T) {
			repo, records := &fakeAIUsageRepository{}, &levelHandler{}
			NewAIUsageService(&fakeDB{}, repo, time.Second, slog.New(records)).
				Record(context.Background(), usage)
			if len(repo.ctxErrs) != 0 {
				t.Errorf("writes = %d, want none for unattributable usage", len(repo.ctxErrs))
			}
			if len(records.messages) != 1 || records.messages[0] != "ai usage without attribution" ||
				records.levels[0] != slog.LevelError {
				t.Errorf("logged %v at %v, want one attribution error", records.messages,
					records.levels)
			}
		})
	}
}

// levelHandler keeps each record's message and level.
type levelHandler struct {
	messages []string
	levels   []slog.Level
}

func (h *levelHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *levelHandler) Handle(_ context.Context, record slog.Record) error {
	h.messages = append(h.messages, record.Message)
	h.levels = append(h.levels, record.Level)
	return nil
}

func (h *levelHandler) WithAttrs([]slog.Attr) slog.Handler { return h }

func (h *levelHandler) WithGroup(string) slog.Handler { return h }

// Recording is reported, never raised: the call it describes already happened.
func TestAIUsageService_AbsorbsAFailedWrite(t *testing.T) {
	repo := &fakeAIUsageRepository{failWith: errors.New("database unavailable")}
	NewAIUsageService(&fakeDB{}, repo, time.Second, slog.New(slog.DiscardHandler)).
		Record(context.Background(), attributedUsage())
	if len(repo.ctxErrs) != 1 {
		t.Fatalf("writes attempted = %d, want 1", len(repo.ctxErrs))
	}
}
