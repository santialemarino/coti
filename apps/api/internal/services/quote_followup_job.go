package services

import (
	"context"
	"log/slog"
	"time"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type stalledQuoteFlagger interface {
	FlagStalledCrossAccount(ctx context.Context, q repository.Querier,
		statuses []domain.QuoteStatus, before time.Time, limit int) ([]domain.FlaggedQuote, error)
}

// QuoteFollowupJob flags the quotes a seller has to chase, across every account: the ones waiting
// on the seller that nobody moved, and the sent ones the client has not answered. It only sets the
// flag the queue sorts and highlights by; it never contacts anyone, and any later write to the
// quote clears it.
type QuoteFollowupJob struct {
	quotes stalledQuoteFlagger
	cfg    config.QuoteFollowupConfig
	now    func() time.Time
	log    *slog.Logger
}

// NewQuoteFollowupJob builds the follow-up sweep.
func NewQuoteFollowupJob(
	quotes stalledQuoteFlagger, cfg config.QuoteFollowupConfig, log *slog.Logger,
) *QuoteFollowupJob {
	return &QuoteFollowupJob{quotes: quotes, cfg: cfg, now: time.Now, log: log}
}

// Name identifies the scheduled follow-up sweep.
func (j *QuoteFollowupJob) Name() string { return "quote-followup" }

// Run sweeps the seller's stalls and then the client's, each batch after batch until one comes back
// short, so a backlog clears in one firing. A flagged quote is never matched again until a write
// clears its flag.
func (j *QuoteFollowupJob) Run(ctx context.Context, q repository.Querier) (domain.JobReport, error) {
	now := j.now()
	var report domain.JobReport
	sweeps := []struct {
		statuses []domain.QuoteStatus
		before   time.Time
	}{
		{domain.FollowupSellerStatuses, now.Add(-j.cfg.SellerIdleAfter)},
		{domain.FollowupCustomerStatuses, now.Add(-j.cfg.CustomerSilentAfter)},
	}
	for _, sweep := range sweeps {
		if err := j.drain(ctx, q, &report, sweep.statuses, sweep.before); err != nil {
			return report, err
		}
	}
	return report, nil
}

func (j *QuoteFollowupJob) drain(
	ctx context.Context, q repository.Querier, report *domain.JobReport,
	statuses []domain.QuoteStatus, before time.Time,
) error {
	for {
		flagged, err := j.quotes.FlagStalledCrossAccount(ctx, q, statuses, before, j.cfg.BatchSize)
		if err != nil {
			return err
		}
		for _, quote := range flagged {
			j.log.Info("quote flagged for follow-up",
				slog.String("account", quote.AccountID.String()),
				slog.String("branch", quote.BranchID.String()),
				slog.String("quote", quote.QuoteID.String()),
				slog.Int64("number", quote.Number),
				slog.String("status", string(quote.Status)),
				slog.Time("last_activity_at", quote.LastActivityAt))
		}
		report.Scanned += len(flagged)
		report.Changed += len(flagged)
		if len(flagged) < j.cfg.BatchSize {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
}

var _ Job = (*QuoteFollowupJob)(nil)
