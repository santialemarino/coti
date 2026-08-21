package services

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

var onboardingSteps = map[domain.OnboardingStepKey]struct{}{
	domain.OnboardingStepWelcome:       {},
	domain.OnboardingStepBrand:         {},
	domain.OnboardingStepFirstBranch:   {},
	domain.OnboardingStepCatalogUpload: {},
	domain.OnboardingStepCatalogReview: {},
	domain.OnboardingStepTeam:          {},
	domain.OnboardingStepComplete:      {},
}

type onboardingRepository interface {
	GetByAccountID(ctx context.Context, q repository.Querier, accountID uuid.UUID) (*domain.Onboarding, error)
	ListSteps(ctx context.Context, q repository.Querier, accountID, onboardingID uuid.UUID) (map[domain.OnboardingStepKey]domain.OnboardingStepStatus, error)
	UpdateCurrentStep(ctx context.Context, q repository.Querier, accountID uuid.UUID, step domain.OnboardingStepKey) error
	UpdateStatus(ctx context.Context, q repository.Querier, accountID uuid.UUID, status domain.OnboardingStatus, step domain.OnboardingStepKey) error
	UpsertStep(ctx context.Context, q repository.Querier, accountID, onboardingID uuid.UUID, step domain.OnboardingStepKey, status domain.OnboardingStepStatus) error
}

// OnboardingService maintains the account's resumable setup flow.
type OnboardingService struct {
	db         tenantScoper
	onboarding onboardingRepository
}

// NewOnboardingService builds an OnboardingService.
func NewOnboardingService(db tenantScoper, onboarding onboardingRepository) *OnboardingService {
	return &OnboardingService{db: db, onboarding: onboarding}
}

// Get returns the account's onboarding state and resolved steps.
func (s *OnboardingService) Get(
	ctx context.Context, tenant domain.Tenant,
) (*domain.Onboarding, error) {
	var onboarding *domain.Onboarding
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var err error
		onboarding, err = s.onboarding.GetByAccountID(ctx, q, tenant.AccountID)
		if err != nil {
			return err
		}
		onboarding.Steps, err = s.onboarding.ListSteps(ctx, q, tenant.AccountID, onboarding.ID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return onboarding, nil
}

// SaveProgress resolves one step and stores the next screen to resume.
func (s *OnboardingService) SaveProgress(
	ctx context.Context, tenant domain.Tenant, progress domain.OnboardingProgress,
) (*domain.Onboarding, error) {
	if !validOnboardingStep(progress.Step) || !validOnboardingStep(progress.CurrentStep) {
		return nil, fmt.Errorf("%w: unknown onboarding step", domain.ErrInvalidInput)
	}
	if progress.StepStatus != domain.OnboardingStepStatusCompleted &&
		progress.StepStatus != domain.OnboardingStepStatusSkipped {
		return nil, fmt.Errorf("%w: unknown onboarding step status", domain.ErrInvalidInput)
	}

	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		onboarding, err := s.onboarding.GetByAccountID(ctx, q, tenant.AccountID)
		if err != nil {
			return err
		}
		if err := s.onboarding.UpsertStep(ctx, q, tenant.AccountID, onboarding.ID,
			progress.Step, progress.StepStatus); err != nil {
			return err
		}
		return s.onboarding.UpdateCurrentStep(ctx, q, tenant.AccountID, progress.CurrentStep)
	})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, tenant)
}

// Complete marks setup finished for the account.
func (s *OnboardingService) Complete(ctx context.Context, tenant domain.Tenant) error {
	return s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		return s.onboarding.UpdateStatus(ctx, q, tenant.AccountID,
			domain.OnboardingStatusCompleted, domain.OnboardingStepComplete)
	})
}

// Dismiss lets an administrator leave setup without blocking the backoffice.
func (s *OnboardingService) Dismiss(ctx context.Context, tenant domain.Tenant) error {
	return s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		onboarding, err := s.onboarding.GetByAccountID(ctx, q, tenant.AccountID)
		if err != nil {
			return err
		}
		return s.onboarding.UpdateStatus(ctx, q, tenant.AccountID,
			domain.OnboardingStatusDismissed, onboarding.CurrentStep)
	})
}

// Resume returns a dismissed setup to its last screen.
func (s *OnboardingService) Resume(ctx context.Context, tenant domain.Tenant) error {
	return s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		onboarding, err := s.onboarding.GetByAccountID(ctx, q, tenant.AccountID)
		if err != nil {
			return err
		}
		return s.onboarding.UpdateStatus(ctx, q, tenant.AccountID,
			domain.OnboardingStatusInProgress, onboarding.CurrentStep)
	})
}

func validOnboardingStep(step domain.OnboardingStepKey) bool {
	_, ok := onboardingSteps[step]
	return ok
}
