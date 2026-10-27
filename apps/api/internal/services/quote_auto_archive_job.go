package services

import (
	"context"
	"log/slog"
	"time"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type inactiveQuoteArchiver interface {
	ArchiveInactiveCrossAccount(ctx context.Context, q repository.Querier,
		statuses []domain.QuoteStatus, before, now time.Time, limit int) ([]domain.ArchivedQuote, error)
	ArchiveClosedCrossAccount(ctx context.Context, q repository.Querier, env domain.ARCAEnvironment,
		before time.Time, limit int) ([]domain.ArchivedQuote, error)
}

// QuoteAutoArchiveJob archives the quotes that went quiet past the configured inactivity, across
// every account: the stalls follow-up chases, and the closed outcomes (complete sales and rejected
// quotes) on their own window. Archiving is a flag, so nothing about the quote is lost and the
// seller can bring it back; unarchiving moves the quote, which starts its inactivity over.
type QuoteAutoArchiveJob struct {
	quotes inactiveQuoteArchiver
	cfg    config.QuoteArchiveConfig
	// env is the ARCA environment whose invoices close a sale, beside production's.
	env domain.ARCAEnvironment
	now func() time.Time
	log *slog.Logger
}

// NewQuoteAutoArchiveJob builds the inactivity sweep.
func NewQuoteAutoArchiveJob(
	quotes inactiveQuoteArchiver, cfg config.QuoteArchiveConfig, env domain.ARCAEnvironment,
	log *slog.Logger,
) *QuoteAutoArchiveJob {
	return &QuoteAutoArchiveJob{quotes: quotes, cfg: cfg, env: env, now: time.Now, log: log}
}

// Name identifies the scheduled inactivity sweep.
func (j *QuoteAutoArchiveJob) Name() string { return "quote-auto-archive" }

// Run sweeps the stalls and then the closed outcomes, each batch after batch until one comes back
// short, so a backlog clears in one firing. Each batch commits on its own, and a quote already
// archived is never matched again.
func (j *QuoteAutoArchiveJob) Run(
	ctx context.Context, q repository.Querier,
) (domain.JobReport, error) {
	now := j.now()
	stalledBefore := now.Add(-j.cfg.InactiveAfter)
	closedBefore := now.Add(-j.cfg.ClosedAfter)
	var report domain.JobReport
	err := j.drain(ctx, &report, func() ([]domain.ArchivedQuote, error) {
		return j.quotes.ArchiveInactiveCrossAccount(ctx, q, domain.AutoArchiveStatuses,
			stalledBefore, now, j.cfg.BatchSize)
	})
	if err != nil {
		return report, err
	}
	err = j.drain(ctx, &report, func() ([]domain.ArchivedQuote, error) {
		return j.quotes.ArchiveClosedCrossAccount(ctx, q, j.env, closedBefore, j.cfg.BatchSize)
	})
	return report, err
}

func (j *QuoteAutoArchiveJob) drain(
	ctx context.Context, report *domain.JobReport, batch func() ([]domain.ArchivedQuote, error),
) error {
	for {
		archived, err := batch()
		if err != nil {
			return err
		}
		for _, quote := range archived {
			j.log.Info("quote archived for inactivity",
				slog.String("account", quote.AccountID.String()),
				slog.String("branch", quote.BranchID.String()),
				slog.String("quote", quote.QuoteID.String()),
				slog.Int64("number", quote.Number),
				slog.String("status", string(quote.Status)),
				slog.Time("last_activity_at", quote.LastActivityAt))
		}
		report.Scanned += len(archived)
		report.Changed += len(archived)
		if len(archived) < j.cfg.BatchSize {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
}

var _ Job = (*QuoteAutoArchiveJob)(nil)
