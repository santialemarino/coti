package domain

import (
	"time"

	"github.com/google/uuid"
)

// JobRunStatus is how a scheduled run ended.
type JobRunStatus string

const (
	JobRunStatusRunning   JobRunStatus = "RUNNING"
	JobRunStatusSucceeded JobRunStatus = "SUCCEEDED"
	JobRunStatusFailed    JobRunStatus = "FAILED"
)

// JobReport is what one run of a job swept and what it changed. Scanned counts the rows it
// considered and Changed the ones it wrote, so a run that found nothing to do reads differently
// from one that never looked.
type JobReport struct {
	Scanned int
	Changed int
}

// JobRun is the record a scheduled run leaves behind, so "which run changed this row?" still has
// an answer once the run's logs have rolled off.
type JobRun struct {
	ID         uuid.UUID
	JobName    string
	Status     JobRunStatus
	Scanned    int
	Changed    int
	Error      *string
	StartedAt  time.Time
	FinishedAt *time.Time
	CreatedAt  time.Time
}
