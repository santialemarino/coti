package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

const onboardingColumns = `id, account_id, flow_version, status, current_step, completed_at,
	created_at, updated_at`

// OnboardingRepository owns persistence for account onboarding progress.
type OnboardingRepository struct{}

// NewOnboardingRepository builds an OnboardingRepository.
func NewOnboardingRepository() *OnboardingRepository {
	return &OnboardingRepository{}
}

// GetByAccountID returns the account's onboarding state.
func (r *OnboardingRepository) GetByAccountID(
	ctx context.Context, q Querier, accountID uuid.UUID,
) (*domain.Onboarding, error) {
	onboarding, err := scanOnboarding(q.QueryRow(ctx,
		`SELECT `+onboardingColumns+` FROM account_onboarding WHERE account_id = $1`, accountID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return onboarding, nil
}

// ListSteps returns every resolved step for the account's onboarding.
func (r *OnboardingRepository) ListSteps(
	ctx context.Context, q Querier, accountID, onboardingID uuid.UUID,
) (map[domain.OnboardingStepKey]domain.OnboardingStepStatus, error) {
	rows, err := q.Query(ctx,
		`SELECT step_key, status
		 FROM onboarding_step_progress
		 WHERE account_id = $1 AND onboarding_id = $2`,
		accountID, onboardingID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	steps := make(map[domain.OnboardingStepKey]domain.OnboardingStepStatus)
	for rows.Next() {
		var key domain.OnboardingStepKey
		var status domain.OnboardingStepStatus
		if err := rows.Scan(&key, &status); err != nil {
			return nil, err
		}
		steps[key] = status
	}
	return steps, rows.Err()
}

// Create starts onboarding for a newly registered account.
func (r *OnboardingRepository) Create(
	ctx context.Context, q Querier, accountID uuid.UUID,
) (*domain.Onboarding, error) {
	return scanOnboarding(q.QueryRow(ctx,
		`INSERT INTO account_onboarding (account_id)
		 VALUES ($1)
		 RETURNING `+onboardingColumns,
		accountID))
}

// UpdateCurrentStep stores the screen the account should resume on.
func (r *OnboardingRepository) UpdateCurrentStep(
	ctx context.Context, q Querier, accountID uuid.UUID, step domain.OnboardingStepKey,
) error {
	tag, err := q.Exec(ctx,
		`UPDATE account_onboarding
		 SET current_step = $2, status = 'IN_PROGRESS', completed_at = NULL
		 WHERE account_id = $1`,
		accountID, step)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// UpdateStatus moves the account onboarding to a terminal or resumed state.
func (r *OnboardingRepository) UpdateStatus(
	ctx context.Context, q Querier, accountID uuid.UUID, status domain.OnboardingStatus,
	step domain.OnboardingStepKey,
) error {
	tag, err := q.Exec(ctx,
		`UPDATE account_onboarding
		 SET status = $2::onboarding_status, current_step = $3,
		     completed_at = CASE WHEN $2::onboarding_status = 'COMPLETED' THEN now() ELSE NULL END
		 WHERE account_id = $1`,
		accountID, status, step)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// UpsertStep records whether one stable step was completed or skipped.
func (r *OnboardingRepository) UpsertStep(
	ctx context.Context, q Querier, accountID, onboardingID uuid.UUID,
	step domain.OnboardingStepKey, status domain.OnboardingStepStatus,
) error {
	_, err := q.Exec(ctx,
		`INSERT INTO onboarding_step_progress (account_id, onboarding_id, step_key, status)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (onboarding_id, step_key)
		 DO UPDATE SET status = EXCLUDED.status`,
		accountID, onboardingID, step, status)
	return err
}

func scanOnboarding(row pgx.Row) (*domain.Onboarding, error) {
	var onboarding domain.Onboarding
	if err := row.Scan(&onboarding.ID, &onboarding.AccountID, &onboarding.FlowVersion,
		&onboarding.Status, &onboarding.CurrentStep, &onboarding.CompletedAt,
		&onboarding.CreatedAt, &onboarding.UpdatedAt); err != nil {
		return nil, err
	}
	onboarding.Steps = make(map[domain.OnboardingStepKey]domain.OnboardingStepStatus)
	return &onboarding, nil
}
