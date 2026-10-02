package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type fakeOnboardingRepository struct {
	onboarding domain.Onboarding
	steps      map[domain.OnboardingStepKey]domain.OnboardingStepStatus
	evidence   domain.OnboardingEvidence
}

func (f *fakeOnboardingRepository) GetByAccountID(
	_ context.Context, _ repository.Querier, accountID uuid.UUID,
) (*domain.Onboarding, error) {
	if accountID != f.onboarding.AccountID {
		return nil, domain.ErrNotFound
	}
	copy := f.onboarding
	return &copy, nil
}

func (f *fakeOnboardingRepository) ListSteps(
	_ context.Context, _ repository.Querier, accountID, onboardingID uuid.UUID,
) (map[domain.OnboardingStepKey]domain.OnboardingStepStatus, error) {
	if accountID != f.onboarding.AccountID || onboardingID != f.onboarding.ID {
		return nil, domain.ErrNotFound
	}
	copy := make(map[domain.OnboardingStepKey]domain.OnboardingStepStatus, len(f.steps))
	for step, status := range f.steps {
		copy[step] = status
	}
	return copy, nil
}

func (f *fakeOnboardingRepository) GetEvidence(
	_ context.Context, _ repository.Querier, accountID uuid.UUID,
) (domain.OnboardingEvidence, error) {
	if accountID != f.onboarding.AccountID {
		return nil, domain.ErrNotFound
	}
	return f.evidence, nil
}

func (f *fakeOnboardingRepository) UpdateChecklistHidden(
	_ context.Context, _ repository.Querier, accountID uuid.UUID, hidden bool,
) error {
	if accountID != f.onboarding.AccountID {
		return domain.ErrNotFound
	}
	f.onboarding.ChecklistHiddenAt = nil
	if hidden {
		now := time.Now()
		f.onboarding.ChecklistHiddenAt = &now
	}
	return nil
}

func (f *fakeOnboardingRepository) UpdateCurrentStep(
	_ context.Context, _ repository.Querier, accountID uuid.UUID, step domain.OnboardingStepKey,
) error {
	if accountID != f.onboarding.AccountID {
		return domain.ErrNotFound
	}
	f.onboarding.Status = domain.OnboardingStatusInProgress
	f.onboarding.CurrentStep = step
	return nil
}

func (f *fakeOnboardingRepository) UpdateStatus(
	_ context.Context, _ repository.Querier, accountID uuid.UUID, status domain.OnboardingStatus,
	step domain.OnboardingStepKey,
) error {
	if accountID != f.onboarding.AccountID {
		return domain.ErrNotFound
	}
	f.onboarding.Status = status
	f.onboarding.CurrentStep = step
	return nil
}

func (f *fakeOnboardingRepository) UpsertStep(
	_ context.Context, _ repository.Querier, accountID, onboardingID uuid.UUID,
	step domain.OnboardingStepKey, status domain.OnboardingStepStatus,
) error {
	if accountID != f.onboarding.AccountID || onboardingID != f.onboarding.ID {
		return domain.ErrNotFound
	}
	f.steps[step] = status
	return nil
}

func newOnboardingHarness() (*OnboardingService, *fakeOnboardingRepository) {
	repo := &fakeOnboardingRepository{
		onboarding: domain.Onboarding{
			ID:          uuid.New(),
			AccountID:   testAccountID,
			FlowVersion: 1,
			Status:      domain.OnboardingStatusInProgress,
			CurrentStep: domain.OnboardingStepWelcome,
		},
		steps: make(map[domain.OnboardingStepKey]domain.OnboardingStepStatus),
	}
	return NewOnboardingService(&fakeDB{}, repo), repo
}

func TestOnboardingService_SaveProgressPersistsStableStepAndResumePoint(t *testing.T) {
	t.Parallel()
	svc, repo := newOnboardingHarness()

	got, err := svc.SaveProgress(context.Background(), adminTenant(), domain.OnboardingProgress{
		Step:        domain.OnboardingStepBrand,
		StepStatus:  domain.OnboardingStepStatusSkipped,
		CurrentStep: domain.OnboardingStepFirstBranch,
	})
	if err != nil {
		t.Fatalf("SaveProgress: %v", err)
	}
	if got.CurrentStep != domain.OnboardingStepFirstBranch ||
		got.Steps[domain.OnboardingStepBrand] != domain.OnboardingStepStatusSkipped {
		t.Fatalf("unexpected progress: %#v", got)
	}
	if repo.onboarding.CurrentStep != domain.OnboardingStepFirstBranch {
		t.Fatal("resume point was not persisted")
	}
}

func TestOnboardingService_SaveProgressRejectsUnknownKeys(t *testing.T) {
	t.Parallel()
	svc, _ := newOnboardingHarness()

	_, err := svc.SaveProgress(context.Background(), adminTenant(), domain.OnboardingProgress{
		Step:        "FUTURE_UNREGISTERED_STEP",
		StepStatus:  domain.OnboardingStepStatusCompleted,
		CurrentStep: domain.OnboardingStepComplete,
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("got %v, want ErrInvalidInput", err)
	}
}

func TestOnboardingService_DismissAndResumeKeepTheCurrentStep(t *testing.T) {
	t.Parallel()
	svc, repo := newOnboardingHarness()
	repo.onboarding.CurrentStep = domain.OnboardingStepCatalogUpload

	if err := svc.Dismiss(context.Background(), adminTenant()); err != nil {
		t.Fatalf("Dismiss: %v", err)
	}
	if repo.onboarding.Status != domain.OnboardingStatusDismissed ||
		repo.onboarding.CurrentStep != domain.OnboardingStepCatalogUpload {
		t.Fatal("dismissal did not preserve the resume point")
	}
	if err := svc.Resume(context.Background(), adminTenant()); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if repo.onboarding.Status != domain.OnboardingStatusInProgress ||
		repo.onboarding.CurrentStep != domain.OnboardingStepCatalogUpload {
		t.Fatal("resume did not restore the saved step")
	}
}

func TestOnboardingService_GetRecordsStepsTheDataProvesAndKeepsThem(t *testing.T) {
	t.Parallel()
	svc, repo := newOnboardingHarness()
	repo.onboarding.Status = domain.OnboardingStatusCompleted
	repo.steps[domain.OnboardingStepCatalogUpload] = domain.OnboardingStepStatusSkipped
	repo.evidence = domain.OnboardingEvidence{domain.OnboardingStepCatalogUpload: true}

	got, err := svc.Get(context.Background(), adminTenant())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Steps[domain.OnboardingStepCatalogUpload] != domain.OnboardingStepStatusCompleted {
		t.Fatalf("a catalog imported outside the wizard still reads %q",
			got.Steps[domain.OnboardingStepCatalogUpload])
	}
	if _, ok := got.Steps[domain.OnboardingStepBrand]; ok {
		t.Fatal("an unproven step was recorded")
	}

	repo.evidence = domain.OnboardingEvidence{}
	again, err := svc.Get(context.Background(), adminTenant())
	if err != nil {
		t.Fatalf("Get again: %v", err)
	}
	if again.Steps[domain.OnboardingStepCatalogUpload] != domain.OnboardingStepStatusCompleted {
		t.Fatal("a done step came undone when its evidence went away")
	}
}

func TestOnboardingService_GetClosesADismissedSetupWithNothingPending(t *testing.T) {
	t.Parallel()
	svc, repo := newOnboardingHarness()
	repo.onboarding.Status = domain.OnboardingStatusDismissed
	repo.onboarding.CurrentStep = domain.OnboardingStepBrand
	repo.steps[domain.OnboardingStepBrand] = domain.OnboardingStepStatusCompleted
	repo.evidence = domain.OnboardingEvidence{
		domain.OnboardingStepCatalogUpload: true,
		domain.OnboardingStepTeam:          true,
	}

	got, err := svc.Get(context.Background(), adminTenant())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != domain.OnboardingStatusCompleted || got.CurrentStep != domain.OnboardingStepComplete {
		t.Fatalf("got %s at %s, want COMPLETED at COMPLETE", got.Status, got.CurrentStep)
	}
}

func TestOnboardingService_GetLeavesADismissedSetupOpenWhileAStepIsPending(t *testing.T) {
	t.Parallel()
	svc, repo := newOnboardingHarness()
	repo.onboarding.Status = domain.OnboardingStatusDismissed
	repo.steps[domain.OnboardingStepTeam] = domain.OnboardingStepStatusSkipped
	repo.evidence = domain.OnboardingEvidence{
		domain.OnboardingStepBrand:         true,
		domain.OnboardingStepCatalogUpload: true,
	}

	got, err := svc.Get(context.Background(), adminTenant())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != domain.OnboardingStatusDismissed {
		t.Fatalf("a skipped step counted as done: status %s", got.Status)
	}
	if got.Steps[domain.OnboardingStepBrand] != domain.OnboardingStepStatusCompleted {
		t.Fatal("the proven steps were not recorded")
	}
}

func TestOnboardingService_GetNeverClosesTheWizardWhileItIsOpen(t *testing.T) {
	t.Parallel()
	svc, repo := newOnboardingHarness()
	repo.evidence = domain.OnboardingEvidence{
		domain.OnboardingStepBrand:         true,
		domain.OnboardingStepCatalogUpload: true,
		domain.OnboardingStepTeam:          true,
	}

	got, err := svc.Get(context.Background(), adminTenant())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != domain.OnboardingStatusInProgress {
		t.Fatalf("got %s, want the wizard left IN_PROGRESS", got.Status)
	}
}

func TestOnboardingService_SetChecklistHiddenTogglesTheCard(t *testing.T) {
	t.Parallel()
	svc, repo := newOnboardingHarness()

	if err := svc.SetChecklistHidden(context.Background(), adminTenant(), true); err != nil {
		t.Fatalf("hide: %v", err)
	}
	if repo.onboarding.ChecklistHiddenAt == nil {
		t.Fatal("the checklist was not hidden")
	}
	if err := svc.SetChecklistHidden(context.Background(), adminTenant(), false); err != nil {
		t.Fatalf("show: %v", err)
	}
	if repo.onboarding.ChecklistHiddenAt != nil {
		t.Fatal("the checklist was not shown again")
	}
}
