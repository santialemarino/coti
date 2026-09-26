package services

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type fakePendingAccounts struct{ accounts []uuid.UUID }

func (f fakePendingAccounts) ListAccountsPendingEmbedding(context.Context,
	repository.Querier) ([]uuid.UUID, error) {
	return f.accounts, nil
}

// fakeBackfiller embeds a fixed count per account and fails the ones it is told to.
type fakeBackfiller struct {
	embedded map[uuid.UUID]int
	fail     map[uuid.UUID]bool
	asked    []uuid.UUID
}

func (f *fakeBackfiller) Backfill(_ context.Context, tenant domain.Tenant,
	refreshAll bool) (domain.CatalogEmbeddingReport, error) {
	f.asked = append(f.asked, tenant.AccountID)
	report := domain.CatalogEmbeddingReport{Embedded: f.embedded[tenant.AccountID]}
	if refreshAll {
		return report, errors.New("a sweep must embed only what is pending")
	}
	if f.fail[tenant.AccountID] {
		return report, errors.New("provider unavailable")
	}
	return report, nil
}

// Each pending account is embedded in turn, and one that fails is named and does not stop the next.
func TestCatalogEmbeddingJob_EmbedsEveryPendingAccountAndNamesTheOneThatFailed(t *testing.T) {
	failing, working := uuid.New(), uuid.New()
	backfiller := &fakeBackfiller{
		embedded: map[uuid.UUID]int{failing: 2, working: 5},
		fail:     map[uuid.UUID]bool{failing: true},
	}
	job := NewCatalogEmbeddingJob(fakePendingAccounts{accounts: []uuid.UUID{failing, working}},
		backfiller)

	report, err := job.Run(context.Background(), nil)

	if len(backfiller.asked) != 2 {
		t.Fatalf("backfills = %v, want both accounts attempted", backfiller.asked)
	}
	if report.Scanned != 2 || report.Changed != 7 {
		t.Errorf("report = %+v, want 2 accounts scanned and 7 products embedded", report)
	}
	if err == nil || !strings.Contains(err.Error(), failing.String()) ||
		strings.Contains(err.Error(), working.String()) {
		t.Errorf("Run() = %v, want only the failing account named", err)
	}
}
