package services

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// The scheduler's decisions live here: what runs, what the record says afterwards, and what a
// second firing does while the first is still going. The SQL behind the record and the lock is
// covered by the integration suite.

func quietLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// fakeConn hands the callback a nil querier — every collaborator below ignores it — and records
// that the run was given a connection of its own rather than the pool.
type fakeConn struct{ calls int }

func (f *fakeConn) AdminConn(_ context.Context, fn func(repository.Querier) error) error {
	f.calls++
	return fn(nil)
}

// fakeRuns stands in for the run record and the advisory lock.
type fakeRuns struct {
	locked    bool
	lockErr   error
	created   []string
	unlocked  []string
	finished  []domain.JobRun
	createErr error
	finishErr error
	// finishCancelled and unlockCancelled record whether the closing writes arrived on a cancelled
	// context — the difference between a record that closes and one stuck at RUNNING, and between
	// a lock released and one held until the connection dies.
	finishCancelled bool
	unlockCancelled bool
}

func (f *fakeRuns) Create(
	_ context.Context, _ repository.Querier, jobName string,
) (*domain.JobRun, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	f.created = append(f.created, jobName)
	return &domain.JobRun{ID: uuid.New(), JobName: jobName, Status: domain.JobRunStatusRunning}, nil
}

func (f *fakeRuns) Finish(
	ctx context.Context, _ repository.Querier, id uuid.UUID, status domain.JobRunStatus,
	report domain.JobReport, failure *string,
) (*domain.JobRun, error) {
	if ctx.Err() != nil {
		f.finishCancelled = true
	}
	if f.finishErr != nil {
		return nil, f.finishErr
	}
	closed := domain.JobRun{
		ID: id, Status: status, Scanned: report.Scanned, Changed: report.Changed, Error: failure,
	}
	f.finished = append(f.finished, closed)
	return &closed, nil
}

func (f *fakeRuns) TryLock(_ context.Context, _ repository.Querier, _ string) (bool, error) {
	return f.locked, f.lockErr
}

func (f *fakeRuns) Unlock(ctx context.Context, _ repository.Querier, jobName string) error {
	if ctx.Err() != nil {
		f.unlockCancelled = true
	}
	f.unlocked = append(f.unlocked, jobName)
	return nil
}

// fakeJob reports whatever the test staged and counts how often it actually ran.
type fakeJob struct {
	name   string
	report domain.JobReport
	err    error
	runs   int
}

func (j *fakeJob) Name() string { return j.name }

func (j *fakeJob) Run(_ context.Context, _ repository.Querier) (domain.JobReport, error) {
	j.runs++
	return j.report, j.err
}

func newTestJobService(t *testing.T, runs *fakeRuns, jobs ...Job) (*JobService, *fakeConn) {
	t.Helper()
	conn := &fakeConn{}
	service, err := NewJobService(conn, runs, quietLog(), jobs...)
	if err != nil {
		t.Fatalf("NewJobService() = %v, want no error", err)
	}
	return service, conn
}

// The happy path, and the whole point of the record: what it swept and what it changed.
func TestJobService_RecordsWhatTheRunSwept(t *testing.T) {
	job := &fakeJob{name: "follow-up", report: domain.JobReport{Scanned: 40, Changed: 3}}
	runs := &fakeRuns{locked: true}
	service, conn := newTestJobService(t, runs, job)

	run, err := service.Run(context.Background(), "follow-up")
	if err != nil {
		t.Fatalf("Run() = %v, want no error", err)
	}
	if job.runs != 1 {
		t.Errorf("job ran %d times, want once", job.runs)
	}
	if conn.calls != 1 {
		t.Errorf("connections taken = %d, want one held for the run", conn.calls)
	}
	if !slices.Equal(runs.created, []string{"follow-up"}) {
		t.Errorf("records opened = %v, want one for the job", runs.created)
	}
	if len(runs.finished) != 1 {
		t.Fatalf("records closed = %d, want one", len(runs.finished))
	}
	finished := runs.finished[0]
	if finished.Status != domain.JobRunStatusSucceeded {
		t.Errorf("status = %q, want SUCCEEDED", finished.Status)
	}
	if finished.Scanned != 40 || finished.Changed != 3 {
		t.Errorf("record = %d scanned / %d changed, want 40 / 3",
			finished.Scanned, finished.Changed)
	}
	if run.Status != domain.JobRunStatusSucceeded || run.Changed != 3 {
		t.Errorf("returned run = %+v, want the closed record", run)
	}
	if !slices.Equal(runs.unlocked, []string{"follow-up"}) {
		t.Errorf("locks released = %v, want the job's", runs.unlocked)
	}
}

// A job that fails still has to leave a record. A run with no record reads exactly like one that
// never started, which is the single thing the record exists to tell apart.
func TestJobService_RecordsAFailedRunWithItsReason(t *testing.T) {
	failure := errors.New("the sweep query timed out")
	job := &fakeJob{name: "expiry", report: domain.JobReport{Scanned: 12}, err: failure}
	runs := &fakeRuns{locked: true}
	service, _ := newTestJobService(t, runs, job)

	_, err := service.Run(context.Background(), "expiry")
	if !errors.Is(err, failure) {
		t.Fatalf("Run() = %v, want the job's own error", err)
	}
	if len(runs.finished) != 1 {
		t.Fatalf("records closed = %d, want one even on failure", len(runs.finished))
	}
	finished := runs.finished[0]
	if finished.Status != domain.JobRunStatusFailed {
		t.Errorf("status = %q, want FAILED", finished.Status)
	}
	if finished.Error == nil || *finished.Error != failure.Error() {
		t.Errorf("recorded error = %v, want %q", finished.Error, failure)
	}
	// The partial count is worth keeping: it says how far the run got before it stopped.
	if finished.Scanned != 12 {
		t.Errorf("scanned = %d, want the 12 it reached before failing", finished.Scanned)
	}
	if !slices.Equal(runs.unlocked, []string{"expiry"}) {
		t.Errorf("locks released = %v, want the lock freed after a failure too", runs.unlocked)
	}
}

// The acceptance criterion about running twice: a firing that lands while the previous run is
// still going does nothing at all, rather than sweeping the same rows alongside it.
func TestJobService_DoesNothingWhileTheSameJobIsRunning(t *testing.T) {
	job := &fakeJob{name: "follow-up"}
	runs := &fakeRuns{locked: false}
	service, _ := newTestJobService(t, runs, job)

	run, err := service.Run(context.Background(), "follow-up")
	if err != nil {
		t.Fatalf("Run() = %v, want the refused lock to be no error", err)
	}
	if run != nil {
		t.Errorf("run = %+v, want none: nothing was done", run)
	}
	if job.runs != 0 {
		t.Errorf("job ran %d times, want none while the lock is held", job.runs)
	}
	if len(runs.created) != 0 {
		t.Errorf("records opened = %v, want none for a firing that did nothing", runs.created)
	}
	// Nothing was taken, so nothing may be released: releasing here would free the lock the run
	// that is still going is holding.
	if len(runs.unlocked) != 0 {
		t.Errorf("locks released = %v, want none — the lock is not ours", runs.unlocked)
	}
}

// A run stopped by a shutdown signal or by its own timeout is exactly when the record matters,
// and writing it on the cancelled context would leave the row at RUNNING for good.
func TestJobService_ClosesTheRecordEvenWhenTheRunWasCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	job := &fakeJob{name: "expiry", err: context.Canceled}
	runs := &fakeRuns{locked: true}
	service, _ := newTestJobService(t, runs, job)
	cancel()

	if _, err := service.Run(ctx, "expiry"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() = %v, want the cancellation", err)
	}
	if runs.finishCancelled {
		t.Error("the closing write arrived on a cancelled context, so the record would stay RUNNING")
	}
	if runs.unlockCancelled {
		t.Error("the release arrived on a cancelled context, so the lock would outlive the run")
	}
	if len(runs.finished) != 1 || runs.finished[0].Status != domain.JobRunStatusFailed {
		t.Errorf("records closed = %+v, want one FAILED", runs.finished)
	}
}

// A mistyped --job on a schedule nobody watches would otherwise do nothing quietly.
func TestJobService_RefusesAJobItDoesNotKnow(t *testing.T) {
	runs := &fakeRuns{locked: true}
	service, _ := newTestJobService(t, runs, &fakeJob{name: "follow-up"}, &fakeJob{name: "expiry"})

	_, err := service.Run(context.Background(), "followup")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Run() = %v, want %v", err, domain.ErrNotFound)
	}
	// The known names belong in the message: the caller mistyped one and cannot see the registry.
	for _, want := range []string{"follow-up", "expiry"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name the known job %q", err, want)
		}
	}
}

// Two jobs answering to one name means one of them can never be run, and the schedule would
// silently sweep the wrong thing.
func TestJobService_RefusesTwoJobsWithOneName(t *testing.T) {
	_, err := NewJobService(&fakeConn{}, &fakeRuns{}, quietLog(),
		&fakeJob{name: "follow-up"}, &fakeJob{name: "follow-up"})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("NewJobService() = %v, want %v", err, domain.ErrInvalidInput)
	}
}

// job_run.job_name is VARCHAR(64). A name too long to record would fail at the first firing, in
// the middle of the night, instead of at the startup that registered it.
func TestJobService_RefusesANameItCouldNotRecord(t *testing.T) {
	for _, tc := range []struct {
		name    string
		jobName string
	}{
		{"empty", ""},
		{"past the column", strings.Repeat("a", 65)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewJobService(&fakeConn{}, &fakeRuns{}, quietLog(), &fakeJob{name: tc.jobName})
			if !errors.Is(err, domain.ErrInvalidInput) {
				t.Errorf("NewJobService() = %v, want %v", err, domain.ErrInvalidInput)
			}
		})
	}
	// The boundary itself is legal, or the message would be off by one.
	if _, err := NewJobService(&fakeConn{}, &fakeRuns{}, quietLog(),
		&fakeJob{name: strings.Repeat("a", 64)}); err != nil {
		t.Errorf("NewJobService() = %v, want a 64-character name accepted", err)
	}
}

func TestJobService_ListsItsJobsSorted(t *testing.T) {
	service, _ := newTestJobService(t, &fakeRuns{},
		&fakeJob{name: "follow-up"}, &fakeJob{name: "attachments"}, &fakeJob{name: "expiry"})

	if want := []string{"attachments", "expiry", "follow-up"}; !slices.Equal(service.Names(), want) {
		t.Errorf("Names() = %v, want %v", service.Names(), want)
	}
}
