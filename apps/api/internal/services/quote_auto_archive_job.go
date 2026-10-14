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
}

// QuoteAutoArchiveJob archives the quotes that went quiet past the configured inactivity, across
// every account. Archiving is a flag, so nothing about the quote is lost and the seller can bring
// it back; unarchiving moves the quote, which starts its inactivity over.
type QuoteAutoArchiveJob struct {
	quotes inactiveQuoteArchiver
	cfg    config.QuoteArchiveConfig
	now    func() time.Time
	log    *slog.Logger
}

// NewQuoteAutoArchiveJob builds the inactivity sweep.
func NewQuoteAutoArchiveJob(
	quotes inactiveQuoteArchiver, cfg config.QuoteArchiveConfig, log *slog.Logger,
) *QuoteAutoArchiveJob {
	return &QuoteAutoArchiveJob{quotes: quotes, cfg: cfg, now: time.Now, log: log}
}

// Name identifies the scheduled inactivity sweep.
func (j *QuoteAutoArchiveJob) Name() string { return "quote-auto-archive" }

// Run archives batch after batch until one comes back short, so a backlog clears in one firing.
// Each batch commits on its own, and a quote already archived is never matched again.
func (j *QuoteAutoArchiveJob) Run(
	ctx context.Context, q repository.Querier,
) (domain.JobReport, error) {
	now := j.now()
	before := now.Add(-j.cfg.InactiveAfter)
	var report domain.JobReport
	for {
		archived, err := j.quotes.ArchiveInactiveCrossAccount(ctx, q, domain.AutoArchiveStatuses,
			before, now, j.cfg.BatchSize)
		if err != nil {
			return report, err
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
			return report, nil
		}
		if err := ctx.Err(); err != nil {
			return report, err
		}
	}
}

var _ Job = (*QuoteAutoArchiveJob)(nil)
