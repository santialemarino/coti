package services

import (
	"context"
	"fmt"
	"log/slog"
	"slices"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// Job is one unit of scheduled work.
//
// Run is handed a database handle and nothing else, which is the whole outbound story: a job
// updates state and has no way to reach a channel, because nothing reaches a client without a
// seller deciding it should.
type Job interface {
	// Name identifies the job on the command line and in its run records.
	Name() string
	// Run does the work and reports what it swept. Running it twice must not change anything the
	// first run already did.
	Run(ctx context.Context, q repository.Querier) (domain.JobReport, error)
}

// maxJobNameLength is job_run.job_name's own.
const maxJobNameLength = 64

// jobRunner is the owner-pool surface the scheduler needs: one connection held for the run, so
// the advisory lock stays on the connection that took it.
type jobRunner interface {
	AdminConn(ctx context.Context, fn func(repository.Querier) error) error
}

// jobRunRepository is the run-record surface the scheduler needs.
type jobRunRepository interface {
	Create(ctx context.Context, q repository.Querier, jobName string) (*domain.JobRun, error)
	Finish(ctx context.Context, q repository.Querier, id uuid.UUID, status domain.JobRunStatus,
		report domain.JobReport, failure *string) (*domain.JobRun, error)
	TryLock(ctx context.Context, q repository.Querier, jobName string) (bool, error)
	Unlock(ctx context.Context, q repository.Querier, jobName string) error
}

// JobService runs one scheduled job at a time and records what it did.
//
// It runs as the owner across every account on purpose — a sweep that stopped at one account's
// boundary would need a caller per account and a schedule per account.
type JobService struct {
	db   jobRunner
	runs jobRunRepository
	jobs map[string]Job
	log  *slog.Logger
}

// NewJobService builds a JobService over the jobs it may run. A name registered twice is a
// configuration error the caller could not otherwise see, so it is refused here.
func NewJobService(
	db jobRunner, runs jobRunRepository, log *slog.Logger, jobs ...Job,
) (*JobService, error) {
	registry := make(map[string]Job, len(jobs))
	for _, job := range jobs {
		if _, taken := registry[job.Name()]; taken {
			return nil, fmt.Errorf("%w: two jobs are named %q", domain.ErrInvalidInput, job.Name())
		}
		// job_run.job_name is VARCHAR(64), and a name too long to record would fail at the first
		// firing rather than here, in the middle of the night with nobody reading.
		if job.Name() == "" || len(job.Name()) > maxJobNameLength {
			return nil, fmt.Errorf("%w: job name %q must be 1 to %d characters",
				domain.ErrInvalidInput, job.Name(), maxJobNameLength)
		}
		registry[job.Name()] = job
	}
	return &JobService{db: db, runs: runs, jobs: registry, log: log}, nil
}

// Names lists the registered jobs, sorted, so a caller who mistyped one can be told the choices.
func (s *JobService) Names() []string {
	names := make([]string, 0, len(s.jobs))
	for name := range s.jobs {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// Run executes one job under its lock and records the outcome. A run refused the lock is not an
// error: the previous one is still going, and the schedule firing again is expected.
func (s *JobService) Run(ctx context.Context, name string) (*domain.JobRun, error) {
	job, known := s.jobs[name]
	if !known {
		return nil, fmt.Errorf("%w: no job named %q, known jobs are %v",
			domain.ErrNotFound, name, s.Names())
	}

	var run *domain.JobRun
	err := s.db.AdminConn(ctx, func(q repository.Querier) error {
		acquired, lockErr := s.runs.TryLock(ctx, q, name)
		if lockErr != nil {
			return lockErr
		}
		if !acquired {
			s.log.Info("job already running, this firing does nothing",
				slog.String("job", name))
			return nil
		}
		// Released explicitly rather than left to the connection closing, so the next firing is
		// not blocked by a pooled connection that has not been recycled yet. On a context that
		// is already cancelled — a shutdown signal, or the run timing out — the release still has
		// to be attempted, which is what detaching from the cancellation buys.
		defer func() {
			if unlockErr := s.runs.Unlock(context.WithoutCancel(ctx), q, name); unlockErr != nil {
				s.log.Error("could not release the job lock",
					slog.String("job", name), slog.Any("error", unlockErr))
			}
		}()

		var runErr error
		run, runErr = s.execute(ctx, q, job)
		return runErr
	})
	if err != nil {
		return nil, err
	}
	return run, nil
}

// execute opens the record, runs the job, and closes the record whichever way it went.
func (s *JobService) execute(
	ctx context.Context, q repository.Querier, job Job,
) (*domain.JobRun, error) {
	// Opened before the work so a run killed mid-flight is still visible as one that began,
	// rather than leaving no trace of the night it stopped.
	run, err := s.runs.Create(ctx, q, job.Name())
	if err != nil {
		return nil, err
	}
	s.log.Info("job started", slog.String("job", job.Name()), slog.String("run", run.ID.String()))

	report, jobErr := job.Run(ctx, q)
	status, failure := domain.JobRunStatusSucceeded, (*string)(nil)
	if jobErr != nil {
		message := jobErr.Error()
		status, failure = domain.JobRunStatusFailed, &message
	}
	// Written even when the job failed, and on a context detached from the caller's: a run that
	// stopped because it was cancelled or timed out is exactly when the record matters, and
	// writing it on the cancelled context would leave the row stuck at RUNNING forever.
	closed, finishErr := s.runs.Finish(context.WithoutCancel(ctx), q, run.ID, status, report,
		failure)
	if finishErr != nil {
		return nil, finishErr
	}
	run = closed

	if jobErr != nil {
		s.log.Error("job failed", slog.String("job", job.Name()),
			slog.String("run", run.ID.String()), slog.Int("scanned", report.Scanned),
			slog.Int("changed", report.Changed), slog.Any("error", jobErr))
		return run, jobErr
	}
	s.log.Info("job finished", slog.String("job", job.Name()),
		slog.String("run", run.ID.String()), slog.Int("scanned", report.Scanned),
		slog.Int("changed", report.Changed))
	return run, nil
}
