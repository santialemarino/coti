package services

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/pgvector/pgvector-go"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// fakeCorrectionQueue serves one pending batch and records what was marked.
type fakeCorrectionQueue struct {
	quoteCorrectionRepository
	pending []domain.QuoteCorrectionMemory
	ready   []uuid.UUID
	failed  []uuid.UUID
}

func (f *fakeCorrectionQueue) HasReadyInterpretation(context.Context, repository.Querier,
	uuid.UUID) (bool, error) {
	return true, nil
}

func (f *fakeCorrectionQueue) FindInterpretationExamples(context.Context, repository.Querier,
	uuid.UUID, pgvector.Vector, float64, int) ([]domain.RFQInterpretationExample, error) {
	return nil, nil
}

func (f *fakeCorrectionQueue) RecordFailure(_ context.Context, _ repository.Querier, _,
	id uuid.UUID, _ string) error {
	f.failed = append(f.failed, id)
	return nil
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

// A sweep across accounts reports which one failed, and the others are still published.
func TestQuoteCorrectionJob_NamesTheAccountWhoseEmbeddingFailed(t *testing.T) {
	failing := uuid.New()
	memory := domain.QuoteCorrectionMemory{ID: uuid.New(), AccountID: failing, SourceText: "cal"}
	queue := &fakeCorrectionQueue{pending: []domain.QuoteCorrectionMemory{memory}}

	_, err := NewQuoteCorrectionJob(queue, &fakeEmbedder{err: errors.New("provider unavailable")},
		config.QuoteCorrectionConfig{ProcessingBatchSize: 10}).Run(context.Background(), nil)

	if err == nil || !strings.Contains(err.Error(), failing.String()) {
		t.Errorf("Run() = %v, want the failing account named", err)
	}
	if len(queue.failed) != 1 || queue.failed[0] != memory.ID {
		t.Errorf("failures recorded = %v, want the memory marked", queue.failed)
	}
}

// Looking up examples for a new order and learning a seller's correction are both paid for by the
// account, each under its own name.
func TestQuoteCorrectionService_AttributesTheLookupAndTheLearning(t *testing.T) {
	embedder := &fakeEmbedder{}
	service := NewQuoteCorrectionService(&fakeDB{}, &fakeCorrectionQueue{}, embedder,
		config.QuoteCorrectionConfig{SimilarityPercent: 80, MaxInterpretationExamples: 3},
		slog.New(slog.DiscardHandler))
	tenant := testSearchTenant()

	if _, err := service.FindInterpretationExamples(context.Background(), tenant,
		"10 bolsas de cemento"); err != nil {
		t.Fatalf("FindInterpretationExamples() = %v", err)
	}
	service.Process(context.Background(), tenant, []domain.QuoteCorrectionMemory{
		{ID: uuid.New(), AccountID: tenant.AccountID, SourceText: "bolsa de portland"}})

	want := []domain.AIOperation{domain.AIOperationInterpretationLookup,
		domain.AIOperationCorrectionLearning}
	if len(embedder.scopes) != len(want) {
		t.Fatalf("embedder calls = %d, want %d", len(embedder.scopes), len(want))
	}
	for i, scope := range embedder.scopes {
		if scope.AccountID != tenant.AccountID || scope.Operation != want[i] {
			t.Errorf("call %d scope = %+v, want the account's %s", i, scope, want[i])
		}
	}
}
