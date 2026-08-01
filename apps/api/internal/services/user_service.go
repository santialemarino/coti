package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// userAdminRepository is the app_user surface the admin use cases need.
type userAdminRepository interface {
	List(ctx context.Context, q repository.Querier, accountID uuid.UUID) ([]domain.AppUser, error)
	GetByID(ctx context.Context, q repository.Querier, accountID, id uuid.UUID) (*domain.AppUser, error)
	Create(ctx context.Context, q repository.Querier, accountID uuid.UUID, in domain.NewUser, passwordHash string) (*domain.AppUser, error)
	Update(ctx context.Context, q repository.Querier, accountID, id uuid.UUID, in domain.UserUpdate) (*domain.AppUser, error)
	Deactivate(ctx context.Context, q repository.Querier, accountID, id uuid.UUID) error
	BumpSessionEpoch(ctx context.Context, q repository.Querier, accountID, id uuid.UUID) (int, error)
}

// userBranchRepository is the seller-to-branch assignment surface.
type userBranchRepository interface {
	ListByUsers(ctx context.Context, q repository.Querier, accountID uuid.UUID, userIDs []uuid.UUID) (map[uuid.UUID][]uuid.UUID, error)
	Replace(ctx context.Context, q repository.Querier, accountID, userID uuid.UUID, branchIDs []uuid.UUID) error
}

// branchExistence checks that branch ids belong to the account before they are written.
type branchExistence interface {
	ExistAllInAccount(ctx context.Context, q repository.Querier, accountID uuid.UUID, ids []uuid.UUID) (bool, error)
}

// UserService owns the account's users: who exists, what role they carry, and which branches
// they may operate on.
type UserService struct {
	db           tenantTxRunner
	users        userAdminRepository
	assignments  userBranchRepository
	branches     branchExistence
	passwordMinL int
}

// NewUserService builds a UserService.
func NewUserService(
	db tenantTxRunner, users userAdminRepository, assignments userBranchRepository,
	branches branchExistence, cfg config.AuthConfig,
) *UserService {
	return &UserService{
		db: db, users: users, assignments: assignments, branches: branches,
		passwordMinL: cfg.PasswordMinLength,
	}
}

// ListUsers returns the account's users with their branch assignments.
func (s *UserService) ListUsers(ctx context.Context, tenant domain.Tenant) ([]domain.UserWithBranches, error) {
	var out []domain.UserWithBranches
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		users, listErr := s.users.List(ctx, q, tenant.AccountID)
		if listErr != nil {
			return listErr
		}

		ids := make([]uuid.UUID, 0, len(users))
		for _, u := range users {
			ids = append(ids, u.ID)
		}
		assignments, assignErr := s.assignments.ListByUsers(ctx, q, tenant.AccountID, ids)
		if assignErr != nil {
			return assignErr
		}

		out = make([]domain.UserWithBranches, 0, len(users))
		for _, u := range users {
			out = append(out, withBranches(u, assignments[u.ID]))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// GetUser returns one user of the account with their branch assignments.
func (s *UserService) GetUser(ctx context.Context, tenant domain.Tenant, id uuid.UUID) (*domain.UserWithBranches, error) {
	var out *domain.UserWithBranches
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		user, getErr := s.users.GetByID(ctx, q, tenant.AccountID, id)
		if getErr != nil {
			return getErr
		}
		assignments, assignErr := s.assignments.ListByUsers(ctx, q, tenant.AccountID, []uuid.UUID{id})
		if assignErr != nil {
			return assignErr
		}
		result := withBranches(*user, assignments[id])
		out = &result
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// CreateUser adds a user to the caller's account, assigning their branches in the same
// transaction. Returns domain.ErrConflict when the email is taken inside the account.
func (s *UserService) CreateUser(
	ctx context.Context, tenant domain.Tenant, in domain.NewUser,
) (*domain.UserWithBranches, error) {
	in.Email = normalizeEmail(in.Email)
	if err := s.validateProfile(in.Name, in.Email, in.Role); err != nil {
		return nil, err
	}
	if len([]rune(in.Password)) < s.passwordMinL {
		return nil, fmt.Errorf("%w: password must be at least %d characters",
			domain.ErrInvalidInput, s.passwordMinL)
	}
	branchIDs := dedupeUUIDs(in.BranchIDs)
	in.BranchIDs = branchIDs

	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	var out *domain.UserWithBranches
	if err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		if assignErr := s.assertBranchesInAccount(ctx, q, tenant.AccountID, branchIDs); assignErr != nil {
			return assignErr
		}
		user, createErr := s.users.Create(ctx, q, tenant.AccountID, in, string(hash))
		if createErr != nil {
			return createErr
		}
		if replaceErr := s.assignments.Replace(ctx, q, tenant.AccountID, user.ID, branchIDs); replaceErr != nil {
			return replaceErr
		}
		result := withBranches(*user, branchIDs)
		out = &result
		return nil
	}); err != nil {
		return nil, err
	}
	return out, nil
}

// UpdateUser replaces the user's profile, role and branch assignments. An admin may not demote
// or deactivate themselves: either drops the last admin out of the account with no way back.
func (s *UserService) UpdateUser(
	ctx context.Context, tenant domain.Tenant, id uuid.UUID, in domain.UserUpdate,
) (*domain.UserWithBranches, error) {
	in.Email = normalizeEmail(in.Email)
	if err := s.validateProfile(in.Name, in.Email, in.Role); err != nil {
		return nil, err
	}
	isSelf := id == tenant.UserID
	if isSelf && in.IsActive != nil && !*in.IsActive {
		return nil, fmt.Errorf("%w: an admin cannot deactivate themselves", domain.ErrInvalidInput)
	}
	branchIDs := dedupeUUIDs(in.BranchIDs)
	in.BranchIDs = branchIDs

	var out *domain.UserWithBranches
	if err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		current, getErr := s.users.GetByID(ctx, q, tenant.AccountID, id)
		if getErr != nil {
			return getErr
		}
		if isSelf && in.Role != current.Role {
			return fmt.Errorf("%w: an admin cannot change their own role", domain.ErrInvalidInput)
		}
		if assignErr := s.assertBranchesInAccount(ctx, q, tenant.AccountID, branchIDs); assignErr != nil {
			return assignErr
		}

		user, updateErr := s.users.Update(ctx, q, tenant.AccountID, id, in)
		if updateErr != nil {
			return updateErr
		}
		if replaceErr := s.assignments.Replace(ctx, q, tenant.AccountID, id, branchIDs); replaceErr != nil {
			return replaceErr
		}
		if current.IsActive && !user.IsActive {
			if _, bumpErr := s.users.BumpSessionEpoch(ctx, q, tenant.AccountID, id); bumpErr != nil {
				return bumpErr
			}
		}
		result := withBranches(*user, branchIDs)
		out = &result
		return nil
	}); err != nil {
		return nil, err
	}
	return out, nil
}

// DeactivateUser disables a user and bumps their session epoch in one transaction, so the
// tokens they already hold stop working at once. An admin cannot deactivate themselves.
func (s *UserService) DeactivateUser(ctx context.Context, tenant domain.Tenant, id uuid.UUID) error {
	if id == tenant.UserID {
		return fmt.Errorf("%w: an admin cannot deactivate themselves", domain.ErrInvalidInput)
	}

	return s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		if err := s.users.Deactivate(ctx, q, tenant.AccountID, id); err != nil {
			return err
		}
		_, err := s.users.BumpSessionEpoch(ctx, q, tenant.AccountID, id)
		return err
	})
}

// assertBranchesInAccount rejects a branch id from another account, read inside the tenant
// transaction: a foreign key does not confine a child row, because it bypasses row level security.
func (s *UserService) assertBranchesInAccount(
	ctx context.Context, q repository.Querier, accountID uuid.UUID, branchIDs []uuid.UUID,
) error {
	if len(branchIDs) == 0 {
		return nil
	}
	ok, err := s.branches.ExistAllInAccount(ctx, q, accountID, branchIDs)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: one or more branch ids are not active branches of this account",
			domain.ErrInvalidInput)
	}
	return nil
}

// validateProfile checks what DTO binding cannot: a role outside the enum, and a name or
// email that is blank once trimmed.
func (s *UserService) validateProfile(name, email string, role domain.UserRole) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("%w: name is required", domain.ErrInvalidInput)
	}
	if email == "" {
		return fmt.Errorf("%w: email is required", domain.ErrInvalidInput)
	}
	if !role.IsValid() {
		return fmt.Errorf("%w: role must be ADMIN or SELLER", domain.ErrInvalidInput)
	}
	return nil
}

// withBranches pairs a user with their assignments, keeping the slice non-nil so the response
// carries an empty list rather than null.
func withBranches(u domain.AppUser, branchIDs []uuid.UUID) domain.UserWithBranches {
	if branchIDs == nil {
		branchIDs = []uuid.UUID{}
	}
	return domain.UserWithBranches{AppUser: u, BranchIDs: branchIDs}
}

// normalizeEmail lowercases and trims, because uq_app_user_email compares the stored value
// and login looks up case-insensitively.
func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// dedupeUUIDs returns the distinct ids, order preserved, never nil.
func dedupeUUIDs(ids []uuid.UUID) []uuid.UUID {
	seen := make(map[uuid.UUID]struct{}, len(ids))
	out := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
