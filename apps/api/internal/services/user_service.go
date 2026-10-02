package services

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

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
	MarkEmailVerified(ctx context.Context, q repository.Querier, accountID, id uuid.UUID) error
	SellersForBranches(ctx context.Context, q repository.Querier, accountID uuid.UUID, branchIDs []uuid.UUID) ([]domain.Seller, error)
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

// userLinkRepository is the single-use-link surface user administration needs: the invite it
// mints and the links an address change retires.
type userLinkRepository interface {
	authTokenRepository
	LatestInvitesByUsers(ctx context.Context, q repository.Querier, accountID uuid.UUID, userIDs []uuid.UUID) (map[uuid.UUID]domain.AuthToken, error)
}

// addressVerifier mails what an address change owes both mailboxes.
type addressVerifier interface {
	SendForNewAddress(ctx context.Context, user domain.AppUser) error
	NotifyAddressChangedByAdmin(ctx context.Context, previous domain.AppUser, newEmail string)
}

// UserService owns the account's users: who exists, what role they carry, and which branches
// they may operate on.
type UserService struct {
	db           tenantScoper
	users        userAdminRepository
	assignments  userBranchRepository
	branches     branchExistence
	tokens       userLinkRepository
	invites      *authLinkIssuer
	verifier     addressVerifier
	policy       domain.PasswordPolicy
	inviteTTL    time.Duration
	mailDelivers bool
	now          func() time.Time
}

// NewUserService builds a UserService. mailDelivers is false while mail only reaches the log,
// which is when nobody could redeem an invite.
func NewUserService(
	db tenantScoper, users userAdminRepository, assignments userBranchRepository,
	branches branchExistence, tokens userLinkRepository, mail mailSender, verifier addressVerifier,
	log *slog.Logger, cfg config.AuthConfig, web config.WebConfig, mailDelivers bool,
	now func() time.Time,
) *UserService {
	if now == nil {
		now = time.Now
	}
	if log == nil {
		log = slog.Default()
	}
	return &UserService{
		db: db, users: users, assignments: assignments, branches: branches, tokens: tokens,
		invites: &authLinkIssuer{
			db: db, tokens: tokens, mail: mail, log: log,
			baseURL: web.BackofficeURL, now: now, newSecret: newTokenSecret,
		},
		verifier:     verifier,
		policy:       domain.PasswordPolicy{MinLength: cfg.PasswordMinLength},
		inviteTTL:    cfg.InviteTTL,
		mailDelivers: mailDelivers,
		now:          now,
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
		invites, inviteErr := s.tokens.LatestInvitesByUsers(ctx, q, tenant.AccountID, ids)
		if inviteErr != nil {
			return inviteErr
		}

		out = make([]domain.UserWithBranches, 0, len(users))
		for _, u := range users {
			out = append(out, s.withInvite(withBranches(u, assignments[u.ID]), invites))
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
		invites, inviteErr := s.tokens.LatestInvitesByUsers(ctx, q, tenant.AccountID, []uuid.UUID{id})
		if inviteErr != nil {
			return inviteErr
		}
		result := s.withInvite(withBranches(*user, assignments[id]), invites)
		out = &result
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ListSellers returns the account's active sellers, narrowed to the branches the caller
// reaches. It powers the manual RFQ assignee picker, so no seller a caller cannot assign to
// appears in it.
func (s *UserService) ListSellers(ctx context.Context, tenant domain.Tenant) ([]domain.Seller, error) {
	var out []domain.Seller
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var listErr error
		out, listErr = s.users.SellersForBranches(ctx, q, tenant.AccountID, tenant.BranchFilter())
		return listErr
	})
	return out, err
}

// CreateUser adds a user to the caller's account, assigning their branches in the same
// transaction. An invited user gets a link to choose their own password instead of one chosen
// for them. Returns domain.ErrConflict when the address is already in use.
func (s *UserService) CreateUser(
	ctx context.Context, tenant domain.Tenant, in domain.NewUser,
) (*domain.UserWithBranches, error) {
	in.Email = domain.NormalizeEmail(in.Email)
	if err := s.validateProfile(in.Name, in.Email, in.Role); err != nil {
		return nil, err
	}
	secret, err := s.initialSecret(in)
	if err != nil {
		return nil, err
	}
	branchIDs := dedupeUUIDs(in.BranchIDs)
	in.BranchIDs = branchIDs

	hash, err := HashPassword(secret)
	if err != nil {
		return nil, err
	}

	var out *domain.UserWithBranches
	var invite string
	if err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		if assignErr := s.assertBranchesInAccount(ctx, q, tenant.AccountID, branchIDs); assignErr != nil {
			return assignErr
		}
		user, createErr := s.users.Create(ctx, q, tenant.AccountID, in, hash)
		if createErr != nil {
			return createErr
		}
		if replaceErr := s.assignments.Replace(ctx, q, tenant.AccountID, user.ID, branchIDs); replaceErr != nil {
			return replaceErr
		}
		result := withBranches(*user, branchIDs)
		out = &result

		// An invited address is proved when the link is redeemed. A password handed over is
		// trusted on the admin's word: verification stops someone reserving an address they cannot
		// read, which an admin inside their own account cannot do.
		if in.Invite {
			var mintErr error
			invite, mintErr = s.invites.mint(ctx, q, *user, domain.AuthTokenTypeInvite, s.inviteTTL)
			out.InviteStatus = domain.InviteStatusPending
			return mintErr
		}
		return s.users.MarkEmailVerified(ctx, q, tenant.AccountID, user.ID)
	}); err != nil {
		return nil, err
	}
	if in.Invite {
		s.invites.deliver(ctx, out.AppUser, domain.AuthTokenTypeInvite, s.inviteMail(out.AppUser, invite))
	}
	return out, nil
}

// ResendInvite mails an invited user a fresh link, retiring the previous one. Only a user who
// has not chosen a password yet has an invite to resend.
func (s *UserService) ResendInvite(ctx context.Context, tenant domain.Tenant, id uuid.UUID) error {
	if !s.mailDelivers {
		return domain.WithCode(domain.CodeMailNotConfigured, domain.ErrNotConfigured)
	}

	var user *domain.AppUser
	var invite string
	resentMeanwhile := false
	if err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		u, getErr := s.users.GetByID(ctx, q, tenant.AccountID, id)
		if getErr != nil {
			return getErr
		}
		invites, inviteErr := s.tokens.LatestInvitesByUsers(ctx, q, tenant.AccountID, []uuid.UUID{id})
		if inviteErr != nil {
			return inviteErr
		}
		if !u.IsActive || s.inviteStatus(id, invites) == domain.InviteStatusNone {
			return domain.WithCode(domain.CodeInviteNotPending,
				fmt.Errorf("%w: the user has no outstanding invite", domain.ErrInvalidInput))
		}
		user = u
		// The read above can predate a redeem or another resend still in flight; retiring waits on
		// that row, and nothing left to retire means one of them won. A fresh read tells which.
		retired, retireErr := s.tokens.InvalidateActive(ctx, q, tenant.AccountID, id, domain.AuthTokenTypeInvite)
		if retireErr != nil {
			return retireErr
		}
		if retired == 0 {
			latest, latestErr := s.tokens.LatestInvitesByUsers(ctx, q, tenant.AccountID, []uuid.UUID{id})
			if latestErr != nil {
				return latestErr
			}
			if s.inviteStatus(id, latest) != domain.InviteStatusNone {
				resentMeanwhile = true
				return nil
			}
			return domain.WithCode(domain.CodeInviteNotPending,
				fmt.Errorf("%w: the invite was redeemed meanwhile", domain.ErrInvalidInput))
		}
		// An address change committed before the retire minted the invite just retired; the link
		// goes to whatever address the user holds now.
		current, getErr := s.users.GetByID(ctx, q, tenant.AccountID, id)
		if getErr != nil {
			return getErr
		}
		user = current
		var storeErr error
		invite, storeErr = s.invites.store(ctx, q, *current, domain.AuthTokenTypeInvite, s.inviteTTL)
		return storeErr
	}); err != nil {
		return err
	}
	// The concurrent resend already mailed a live link; a second one would only retire it.
	if resentMeanwhile {
		return nil
	}
	s.invites.deliver(ctx, *user, domain.AuthTokenTypeInvite, s.inviteMail(*user, invite))
	return nil
}

// UpdateUser replaces the user's profile, role and branch assignments. An admin may not demote
// or deactivate themselves: either drops the last admin out of the account with no way back.
func (s *UserService) UpdateUser(
	ctx context.Context, tenant domain.Tenant, id uuid.UUID, in domain.UserUpdate,
) (*domain.UserWithBranches, error) {
	in.Email = domain.NormalizeEmail(in.Email)
	if err := s.validateProfile(in.Name, in.Email, in.Role); err != nil {
		return nil, err
	}
	isSelf := id == tenant.UserID
	if isSelf && in.IsActive != nil && !*in.IsActive {
		return nil, domain.WithCode(domain.CodeSelfDeactivation,
			fmt.Errorf("%w: an admin cannot deactivate themselves", domain.ErrInvalidInput))
	}
	branchIDs := dedupeUUIDs(in.BranchIDs)
	in.BranchIDs = branchIDs

	var out *domain.UserWithBranches
	var previous *domain.AppUser
	var invite string
	if err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		current, getErr := s.users.GetByID(ctx, q, tenant.AccountID, id)
		if getErr != nil {
			return getErr
		}
		previous = current
		if isSelf && in.Role != current.Role {
			return domain.WithCode(domain.CodeSelfRoleChange,
				fmt.Errorf("%w: an admin cannot change their own role", domain.ErrInvalidInput))
		}
		// Their own address changes through the self-service flow, which re-checks the password.
		if isSelf && emailChanged(current.Email, in.Email) {
			return domain.WithCode(domain.CodeSelfEmailChange,
				fmt.Errorf("%w: an admin changes their own email from their account settings", domain.ErrInvalidInput))
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
		invites, inviteErr := s.tokens.LatestInvitesByUsers(ctx, q, tenant.AccountID, []uuid.UUID{id})
		if inviteErr != nil {
			return inviteErr
		}
		result := s.withInvite(withBranches(*user, branchIDs), invites)
		out = &result
		if !emailChanged(current.Email, user.Email) {
			return nil
		}

		// Every link already mailed went to a mailbox that is no longer the account's. An invite
		// still outstanding follows the address, since it is the only way in the user has.
		if invalidateErr := s.tokens.InvalidateAllForUser(ctx, q, tenant.AccountID, id); invalidateErr != nil {
			return invalidateErr
		}
		if result.InviteStatus == domain.InviteStatusNone {
			return nil
		}
		var mintErr error
		invite, mintErr = s.invites.mint(ctx, q, *user, domain.AuthTokenTypeInvite, s.inviteTTL)
		out.InviteStatus = domain.InviteStatusPending
		return mintErr
	}); err != nil {
		return nil, err
	}

	// A deactivated user could redeem nothing, so they are mailed nothing; the invite stays on
	// record for a resend once they are back.
	if emailChanged(previous.Email, out.Email) && out.IsActive {
		// An address never proved is no one's to warn, and the notice would name the new one.
		if previous.EmailVerifiedAt != nil {
			s.verifier.NotifyAddressChangedByAdmin(ctx, *previous, out.Email)
		}
		if invite != "" {
			s.invites.deliver(ctx, out.AppUser, domain.AuthTokenTypeInvite, s.inviteMail(out.AppUser, invite))
		} else if err := s.verifier.SendForNewAddress(ctx, out.AppUser); err != nil {
			// The change has committed; the user can ask for the link again from the confirmation screen.
			s.invites.log.ErrorContext(ctx, "verification link not issued after an address change",
				slog.String("user_id", out.ID.String()), slog.Any("error", err))
		}
	}
	return out, nil
}

// DeactivateUser disables a user and bumps their session epoch in one transaction, so the
// tokens they already hold stop working at once. An admin cannot deactivate themselves.
func (s *UserService) DeactivateUser(ctx context.Context, tenant domain.Tenant, id uuid.UUID) error {
	if id == tenant.UserID {
		return domain.WithCode(domain.CodeSelfDeactivation,
			fmt.Errorf("%w: an admin cannot deactivate themselves", domain.ErrInvalidInput))
	}

	return s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		if err := s.users.Deactivate(ctx, q, tenant.AccountID, id); err != nil {
			return err
		}
		_, err := s.users.BumpSessionEpoch(ctx, q, tenant.AccountID, id)
		return err
	})
}

// initialSecret is the password the new user is stored with: the admin's choice, or a random
// one nobody holds when the user is invited to choose their own.
func (s *UserService) initialSecret(in domain.NewUser) (string, error) {
	if !in.Invite {
		if err := s.policy.Validate(in.Password); err != nil {
			return "", err
		}
		return in.Password, nil
	}
	if in.Password != "" {
		return "", fmt.Errorf("%w: an invited user chooses their own password", domain.ErrInvalidInput)
	}
	if !s.mailDelivers {
		return "", domain.WithCode(domain.CodeMailNotConfigured, domain.ErrNotConfigured)
	}
	return newTokenSecret()
}

// inviteMail is the message carrying an invite link.
func (s *UserService) inviteMail(user domain.AppUser, link string) OutboundMail {
	return OutboundMail{
		AccountID: user.AccountID,
		UserID:    &user.ID,
		Event:     domain.NotificationEventInvite,
		To:        user.Email,
		ToName:    user.Name,
		Subject:   inviteSubject,
		Heading:   inviteHeading,
		Paragraphs: []string{
			inviteIntro(user.Name),
			inviteValidity(int(s.inviteTTL.Hours())),
			inviteIgnore,
		},
		ActionLabel: inviteAction,
		ActionURL:   link,
	}
}

// inviteStatus derives where a user stands from the latest invites loaded for them.
func (s *UserService) inviteStatus(id uuid.UUID, invites map[uuid.UUID]domain.AuthToken) domain.InviteStatus {
	latest, ok := invites[id]
	if !ok {
		return domain.InviteStatusNone
	}
	return domain.InviteStatusOf(&latest, s.now())
}

// withInvite stamps a user with where their invite stands.
func (s *UserService) withInvite(
	u domain.UserWithBranches, invites map[uuid.UUID]domain.AuthToken,
) domain.UserWithBranches {
	u.InviteStatus = s.inviteStatus(u.ID, invites)
	return u
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

// emailChanged compares two addresses the way the unique index does.
func emailChanged(before, after string) bool {
	return domain.NormalizeEmail(before) != domain.NormalizeEmail(after)
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
