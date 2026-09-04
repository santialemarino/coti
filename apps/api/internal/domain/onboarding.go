package domain

import (
	"time"

	"github.com/google/uuid"
)

// OnboardingStatus is the account setup lifecycle.
type OnboardingStatus string

const (
	OnboardingStatusInProgress OnboardingStatus = "IN_PROGRESS"
	OnboardingStatusCompleted  OnboardingStatus = "COMPLETED"
	OnboardingStatusDismissed  OnboardingStatus = "DISMISSED"
)

// OnboardingStepStatus records how an account resolved one setup step.
type OnboardingStepStatus string

const (
	OnboardingStepStatusCompleted OnboardingStepStatus = "COMPLETED"
	OnboardingStepStatusSkipped   OnboardingStepStatus = "SKIPPED"
)

// OnboardingStepKey is a stable identifier whose order is decided by the current flow version.
type OnboardingStepKey string

const (
	OnboardingStepWelcome       OnboardingStepKey = "WELCOME"
	OnboardingStepBrand         OnboardingStepKey = "BRAND"
	OnboardingStepFirstBranch   OnboardingStepKey = "FIRST_BRANCH"
	OnboardingStepCatalogUpload OnboardingStepKey = "CATALOG_UPLOAD"
	OnboardingStepCatalogReview OnboardingStepKey = "CATALOG_REVIEW"
	OnboardingStepTeam          OnboardingStepKey = "TEAM"
	OnboardingStepComplete      OnboardingStepKey = "COMPLETE"
)

// Onboarding is the versioned setup state for one account.
type Onboarding struct {
	ID          uuid.UUID
	AccountID   uuid.UUID
	FlowVersion int
	Status      OnboardingStatus
	CurrentStep OnboardingStepKey
	Steps       map[OnboardingStepKey]OnboardingStepStatus
	CompletedAt *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// OnboardingProgress resolves one step and points at the next screen to resume.
type OnboardingProgress struct {
	Step        OnboardingStepKey
	StepStatus  OnboardingStepStatus
	CurrentStep OnboardingStepKey
}
