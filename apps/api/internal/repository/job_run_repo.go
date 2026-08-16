package repository

import (
	"context"
	"hash/fnv"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// jobLockNamespace keeps these advisory locks from colliding with any other the database might
// grow: the pair (namespace, job) is the key, not the job's hash alone.
const jobLockNamespace = 4271

// lockKey hashes a job name into the int4 an advisory lock takes. Done here rather than with
// Postgres's hashtext(), which is an internal function with no compatibility promise.
func lockKey(jobName string) int32 {
	digest := fnv.New32a()
	_, _ = digest.Write([]byte(jobName))
	return int32(digest.Sum32())
}

// JobRunRepository owns the record of what each scheduled run swept, and the lock that keeps two
// runs of one job from overlapping.
type JobRunRepository struct{}

// NewJobRunRepository builds a JobRunRepository.
func NewJobRunRepository() *JobRunRepository {
	return &JobRunRepository{}
}

// Create opens a run record, before the work starts, so a run that dies without finishing is
// still visible as one that began.
func (r *JobRunRepository) Create(
	ctx context.Context, q Querier, jobName string,
) (*domain.JobRun, error) {
	var run domain.JobRun
	err := q.QueryRow(ctx,
		`INSERT INTO job_run (job_name, status)
		 VALUES ($1, $2)
		 RETURNING id, job_name, status, scanned, changed, error, started_at, finished_at, created_at`,
		jobName, domain.JobRunStatusRunning,
	).Scan(&run.ID, &run.JobName, &run.Status, &run.Scanned, &run.Changed, &run.Error,
		&run.StartedAt, &run.FinishedAt, &run.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &run, nil
}

// Finish closes a run record with what it swept and how it ended, returning the stored row so
// the caller reports the database's own finished_at rather than a second reading of the clock.
func (r *JobRunRepository) Finish(
	ctx context.Context, q Querier, id uuid.UUID, status domain.JobRunStatus,
	report domain.JobReport, failure *string,
) (*domain.JobRun, error) {
	var run domain.JobRun
	err := q.QueryRow(ctx,
		`UPDATE job_run
		 SET status = $2, scanned = $3, changed = $4, error = $5, finished_at = now()
		 WHERE id = $1
		 RETURNING id, job_name, status, scanned, changed, error, started_at, finished_at, created_at`,
		id, status, report.Scanned, report.Changed, failure,
	).Scan(&run.ID, &run.JobName, &run.Status, &run.Scanned, &run.Changed, &run.Error,
		&run.StartedAt, &run.FinishedAt, &run.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &run, nil
}

// TryLock takes the job's advisory lock for the life of the connection, reporting whether it got
// it. A scheduler that fires again while the previous run is still going gets false, which is how
// two runs of one job never overlap.
//
// It has to run on a bare connection rather than inside a transaction: a transaction-scoped lock
// would be released at the first commit, leaving the rest of the job unprotected.
func (r *JobRunRepository) TryLock(ctx context.Context, q Querier, jobName string) (bool, error) {
	var acquired bool
	if err := q.QueryRow(ctx,
		`SELECT pg_try_advisory_lock($1, $2)`, jobLockNamespace, lockKey(jobName),
	).Scan(&acquired); err != nil {
		return false, err
	}
	return acquired, nil
}

// Unlock releases the job's advisory lock.
func (r *JobRunRepository) Unlock(ctx context.Context, q Querier, jobName string) error {
	_, err := q.Exec(ctx, `SELECT pg_advisory_unlock($1, $2)`, jobLockNamespace, lockKey(jobName))
	return err
}
