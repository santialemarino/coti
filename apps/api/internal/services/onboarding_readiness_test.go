package services

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type readinessFake struct {
	account  uuid.UUID
	branches []uuid.UUID
	err      error
}

func (f *readinessFake) GetReadiness(_ context.Context, _ repository.Querier, account uuid.UUID, branches []uuid.UUID) (*domain.OnboardingReadiness, error) {
	f.account, f.branches = account, branches
	return &domain.OnboardingReadiness{}, f.err
}

func TestOnboardingReadinessRequiresAdminAndPreservesScope(t *testing.T) {
	service, _ := newOnboardingHarness()
	reader := &readinessFake{}
	service.WithReadinessReader(reader)
	tenant := domain.Tenant{AccountID: uuid.New(), BranchID: uuid.New(), UserID: uuid.New(), Role: domain.UserRoleSeller}
	if _, err := service.GetReadiness(context.Background(), tenant); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal(err)
	}
	tenant.Role = domain.UserRoleAdmin
	if _, err := service.GetReadiness(context.Background(), tenant); err != nil {
		t.Fatal(err)
	}
	if reader.account != tenant.AccountID || len(reader.branches) != 1 || reader.branches[0] != tenant.BranchID {
		t.Fatal("diagnostic lost scope")
	}
	reader.err = errors.New("database unavailable")
	if _, err := service.GetReadiness(context.Background(), tenant); err == nil {
		t.Fatal("failure reported as complete")
	}
}
