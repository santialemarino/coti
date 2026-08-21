//go:build integration

package integration

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
	"github.com/santialemarino/coti/apps/api/internal/services"
)

// The scheduler against a real database: the record it writes, the advisory lock that keeps two
// runs apart, and the grant that keeps the request role out of the audit trail.

// countingJob reports what the test staged and, when given a pair of channels, blocks inside its
// run so a second firing can be attempted while the first still holds the lock.
type countingJob struct {
	name    string
	report  domain.JobReport
	err     error
	entered chan struct{}
	release chan struct{}
	mu      sync.Mutex
	runs    int
}

func (j *countingJob) Name() string { return j.name }

func (j *countingJob) Run(_ context.Context, _ repository.Querier) (domain.JobReport, error) {
	j.mu.Lock()
	j.runs++
	first := j.runs == 1
	j.mu.Unlock()
	// Only the first run blocks. A second one has to be able to finish even though it never
	// should have started, or a lock that stopped working would hang the test instead of failing
	// it with a message — and a suite that dies on a timeout says nothing about what broke.
	if j.entered != nil && first {
		j.entered <- struct{}{}
		<-j.release
	}
	return j.report, j.err
}

func (j *countingJob) ran() int {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.runs
}

// jobName is unique per run: this package and internal/repository run in parallel against one
// database, so a shared name would let one test count another's rows.
func jobName(t *testing.T) string {
	t.Helper()
	return "test-" + uuid.NewString()
}

// jobService wires the real repository to the real owner pool, the way the command does.
func (e *env) jobService(t *testing.T, jobs ...services.Job) *services.JobService {
	t.Helper()
	service, err := services.NewJobService(e.db, repository.NewJobRunRepository(),
		slog.New(slog.NewTextHandler(io.Discard, nil)), jobs...)
	if err != nil {
		t.Fatalf("NewJobService() = %v, want no error", err)
	}
	return service
}

// dropRuns removes what this test wrote, so the suite leaves the table as it found it.
func (e *env) dropRuns(t *testing.T, name string) {
	t.Helper()
	t.Cleanup(func() {
		e.mustCleanup(t, `DELETE FROM job_run WHERE job_name = $1`, name)
	})
}

// readRun reads a run back through the owner pool. The repository ships no read method: the audit
// trail has no production reader yet, and one would be speculative surface.
func (e *env) readRun(t *testing.T, id uuid.UUID) domain.JobRun {
	t.Helper()
	var run domain.JobRun
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT id, job_name, status, scanned, changed, error, started_at, finished_at, created_at
		 FROM job_run WHERE id = $1`, id,
	).Scan(&run.ID, &run.JobName, &run.Status, &run.Scanned, &run.Changed, &run.Error,
		&run.StartedAt, &run.FinishedAt, &run.CreatedAt); err != nil {
		t.Fatalf("read run %v: %v", id, err)
	}
	return run
}

func (e *env) countRuns(t *testing.T, name string) int {
	t.Helper()
	var count int
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT count(*) FROM job_run WHERE job_name = $1`, name).Scan(&count); err != nil {
		t.Fatalf("count runs: %v", err)
	}
	return count
}

// The acceptance criterion: every execution leaves a record of what it swept and what it changed.
func TestJobRun_RecordsTheRunInTheDatabase(t *testing.T) {
	e := newEnv(t)
	name := jobName(t)
	e.dropRuns(t, name)
	job := &countingJob{name: name, report: domain.JobReport{Scanned: 17, Changed: 4}}

	run, err := e.jobService(t, job).Run(context.Background(), name)
	if err != nil {
		t.Fatalf("Run() = %v, want no error", err)
	}

	stored := e.readRun(t, run.ID)
	if stored.Status != domain.JobRunStatusSucceeded {
		t.Errorf("stored status = %q, want SUCCEEDED", stored.Status)
	}
	if stored.Scanned != 17 || stored.Changed != 4 {
		t.Errorf("stored record = %d scanned / %d changed, want 17 / 4",
			stored.Scanned, stored.Changed)
	}
	if stored.FinishedAt == nil {
		t.Fatal("stored record has no finished_at, so it reads as still running")
	}
	if stored.FinishedAt.Before(stored.StartedAt) {
		t.Errorf("finished_at %v is before started_at %v", stored.FinishedAt, stored.StartedAt)
	}
	if stored.Error != nil {
		t.Errorf("stored error = %q, want none on a run that succeeded", *stored.Error)
	}
	// The returned run is the stored row, not a second reading of the clock in Go: a caller that
	// logged its finished_at would otherwise print a time the database never recorded.
	if !run.FinishedAt.Equal(*stored.FinishedAt) {
		t.Errorf("returned finished_at = %v, want the stored %v", run.FinishedAt, stored.FinishedAt)
	}
	if run.StartedAt != stored.StartedAt || run.Status != stored.Status {
		t.Errorf("returned run = %+v, want the stored row", run)
	}
}

// A failed run has to be readable afterwards, with the reason it stopped and how far it got.
func TestJobRun_RecordsAFailureWithItsReason(t *testing.T) {
	e := newEnv(t)
	name := jobName(t)
	e.dropRuns(t, name)
	job := &countingJob{
		name: name, report: domain.JobReport{Scanned: 5}, err: errors.New("the sweep gave up"),
	}

	if _, err := e.jobService(t, job).Run(context.Background(), name); err == nil {
		t.Fatal("Run() = nil error, want the job's failure")
	}

	var id uuid.UUID
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT id FROM job_run WHERE job_name = $1`, name).Scan(&id); err != nil {
		t.Fatalf("find the failed run: %v", err)
	}
	stored := e.readRun(t, id)
	if stored.Status != domain.JobRunStatusFailed {
		t.Errorf("status = %q, want FAILED", stored.Status)
	}
	if stored.Error == nil || *stored.Error != "the sweep gave up" {
		t.Errorf("recorded error = %v, want the job's message", stored.Error)
	}
	if stored.Scanned != 5 {
		t.Errorf("scanned = %d, want how far it got before failing", stored.Scanned)
	}
}

// The acceptance criterion about running twice. The lock is a real Postgres advisory lock held on
// one connection, so only a real database can show it: a firing that lands while the previous run
// is still inside its job must do nothing at all.
func TestJobRun_ASecondFiringDoesNothingWhileTheFirstRuns(t *testing.T) {
	e := newEnv(t)
	name := jobName(t)
	e.dropRuns(t, name)
	held := &countingJob{name: name, entered: make(chan struct{}), release: make(chan struct{})}
	service := e.jobService(t, held)

	first := make(chan error, 1)
	go func() {
		_, err := service.Run(context.Background(), name)
		first <- err
	}()
	<-held.entered // the first run now holds the lock and is inside the job

	second, err := service.Run(context.Background(), name)
	if err != nil {
		t.Fatalf("the second firing = %v, want the refused lock to be no error", err)
	}
	if second != nil {
		t.Errorf("the second firing produced run %+v, want none", second)
	}

	close(held.release)
	if err := <-first; err != nil {
		t.Fatalf("the first run = %v, want no error", err)
	}
	if held.ran() != 1 {
		t.Errorf("the job ran %d times, want once across both firings", held.ran())
	}
	if got := e.countRuns(t, name); got != 1 {
		t.Errorf("records written = %d, want one: the second firing did nothing", got)
	}
}

// Once a run finishes the lock is free again, or the schedule would work once and never again.
func TestJobRun_RunsAgainOnceTheLockIsReleased(t *testing.T) {
	e := newEnv(t)
	name := jobName(t)
	e.dropRuns(t, name)
	job := &countingJob{name: name}
	service := e.jobService(t, job)

	for round := 1; round <= 2; round++ {
		if _, err := service.Run(context.Background(), name); err != nil {
			t.Fatalf("run %d = %v, want no error", round, err)
		}
	}
	if job.ran() != 2 {
		t.Errorf("the job ran %d times, want both rounds", job.ran())
	}
	if got := e.countRuns(t, name); got != 2 {
		t.Errorf("records written = %d, want one per run", got)
	}
}

// The audit trail belongs to the owner. The schema's default privileges grant the request role
// every new table, so this is the REVOKE being real rather than merely written down.
func TestJobRun_IsUnreachableFromTheRequestRole(t *testing.T) {
	e := newEnv(t)
	account, _ := e.seedAccount(t, "Corralon Jobs")

	err := e.db.InTenantTx(context.Background(), domain.Tenant{AccountID: account},
		func(q repository.Querier) error {
			var count int
			return q.QueryRow(context.Background(), `SELECT count(*) FROM job_run`).Scan(&count)
		})
	if err == nil {
		t.Fatal("the request role read job_run, want permission denied")
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("error = %v, want a permission denial", err)
	}
}
