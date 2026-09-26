package services

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pgvector/pgvector-go"

	"github.com/santialemarino/coti/apps/api/internal/config"
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

func TestAIUsageService_WritesNothingWithoutAttribution(t *testing.T) {
	for name, usage := range map[string]domain.AIUsage{
		"no account":   {AIUsageScope: domain.AIUsageScope{Operation: domain.AIOperationCatalogSearch}},
		"no operation": {AIUsageScope: domain.AIUsageScope{AccountID: uuid.New()}},
	} {
		t.Run(name, func(t *testing.T) {
			db, repo := &fakeDB{}, &fakeAIUsageRepository{}
			NewAIUsageService(db, repo, time.Second, slog.New(slog.DiscardHandler)).
				Record(context.Background(), usage)
			if len(db.scopes) != 0 || len(repo.ctxErrs) != 0 {
				t.Errorf("opened %d transactions, want none for unattributable usage",
					len(db.scopes))
			}
		})
	}
}

// Recording is reported, never raised: the call it describes already happened.
func TestAIUsageService_AbsorbsAFailedWrite(t *testing.T) {
	repo := &fakeAIUsageRepository{failWith: errors.New("database unavailable")}
	NewAIUsageService(&fakeDB{}, repo, time.Second, slog.New(slog.DiscardHandler)).
		Record(context.Background(), attributedUsage())
	if len(repo.ctxErrs) != 1 {
		t.Fatalf("writes attempted = %d, want 1", len(repo.ctxErrs))
	}
}

// fakeCorrectionQueue serves one pending batch and records what the job marked.
type fakeCorrectionQueue struct {
	quoteCorrectionRepository
	pending []domain.QuoteCorrectionMemory
	ready   []uuid.UUID
}

func (f *fakeCorrectionQueue) ListPending(context.Context, repository.Querier,
	int) ([]domain.QuoteCorrectionMemory, error) {
	return f.pending, nil
}

func (f *fakeCorrectionQueue) MarkReady(_ context.Context, _ repository.Querier, _,
	id uuid.UUID, _ pgvector.Vector) error {
	f.ready = append(f.ready, id)
	return nil
}

// The retry job sweeps every account, but one provider call may only serve one of them, or its
// spend could not be attributed.
func TestQuoteCorrectionJob_EmbedsEachAccountInItsOwnCall(t *testing.T) {
	first, second := uuid.New(), uuid.New()
	queue := &fakeCorrectionQueue{pending: []domain.QuoteCorrectionMemory{
		{ID: uuid.New(), AccountID: first, SourceText: "bolsa de portland"},
		{ID: uuid.New(), AccountID: second, SourceText: "placa de yeso"},
		{ID: uuid.New(), AccountID: first, SourceText: "hierro del 8"},
	}}
	embedder := &fakeEmbedder{}

	report, err := NewQuoteCorrectionJob(queue, embedder,
		config.QuoteCorrectionConfig{ProcessingBatchSize: 10}).Run(
		context.Background(), nil)
	if err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}

	if report.Changed != 3 || len(queue.ready) != 3 {
		t.Errorf("report = %+v with %d ready, want all three published", report, len(queue.ready))
	}
	if len(embedder.calls) != 2 {
		t.Fatalf("embedder calls = %v, want one per account", embedder.calls)
	}
	wantAccounts := []uuid.UUID{first, second}
	wantTexts := [][]string{{"bolsa de portland", "hierro del 8"}, {"placa de yeso"}}
	for i, scope := range embedder.scopes {
		if scope.AccountID != wantAccounts[i] ||
			scope.Operation != domain.AIOperationCorrectionLearning {
			t.Errorf("call %d scope = %+v, want account %v learning a correction", i, scope,
				wantAccounts[i])
		}
		if len(embedder.calls[i]) != len(wantTexts[i]) || embedder.calls[i][0] != wantTexts[i][0] {
			t.Errorf("call %d texts = %v, want %v", i, embedder.calls[i], wantTexts[i])
		}
	}
}
