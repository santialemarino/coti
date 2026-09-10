package services

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"path"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type accountRepository interface {
	GetByID(ctx context.Context, q repository.Querier, accountID uuid.UUID) (*domain.Account, error)
	Create(ctx context.Context, q repository.Querier, name string, legalName, taxID *string) (*domain.Account, error)
	Update(ctx context.Context, q repository.Querier, accountID uuid.UUID, in domain.AccountUpdate) (*domain.Account, error)
}

type signupBranchRepository interface {
	Create(ctx context.Context, q repository.Querier, accountID uuid.UUID, in domain.NewBranch) (*domain.Branch, error)
}

type channelRepository interface {
	CreateManualEntry(ctx context.Context, q repository.Querier, accountID, branchID uuid.UUID) error
}

type signupUserRepository interface {
	ExistsByEmailCrossAccount(ctx context.Context, q repository.Querier, email string) (bool, error)
	Create(ctx context.Context, q repository.Querier, accountID uuid.UUID, in domain.NewUser, passwordHash string) (*domain.AppUser, error)
}

type signupOnboardingRepository interface {
	Create(ctx context.Context, q repository.Querier, accountID uuid.UUID) (*domain.Onboarding, error)
}

type tokenIssuer interface {
	IssueForUser(ctx context.Context, user domain.AppUser) (*domain.TokenPair, error)
}

// emailVerifier mails the new administrator a confirmation link. Narrow on purpose:
// registration triggers verification, it does not own it.
type emailVerifier interface {
	Send(ctx context.Context, user domain.AppUser) error
}

type adminTxScoper interface {
	tenantScoper
	AdminTx(ctx context.Context) (pgx.Tx, error)
}

// AccountService registers corralones and maintains the account record.
type AccountService struct {
	db                adminTxScoper
	accounts          accountRepository
	branches          signupBranchRepository
	channels          channelRepository
	users             signupUserRepository
	onboarding        signupOnboardingRepository
	tokens            tokenIssuer
	verifier          emailVerifier
	log               *slog.Logger
	policy            domain.PasswordPolicy
	defaultExpiryDays int
	logoStorage       domain.ObjectStorage
	logoMaxBytes      int64
}

// NewAccountService builds an AccountService.
func NewAccountService(
	db adminTxScoper, accounts accountRepository, branches signupBranchRepository,
	channels channelRepository, users signupUserRepository, onboarding signupOnboardingRepository,
	tokens tokenIssuer,
	verifier emailVerifier, log *slog.Logger, cfg config.AuthConfig, branchCfg config.BranchConfig,
) *AccountService {
	if log == nil {
		log = slog.Default()
	}
	return &AccountService{db: db, accounts: accounts, branches: branches, channels: channels,
		users: users, onboarding: onboarding, tokens: tokens, verifier: verifier, log: log,
		policy:            domain.PasswordPolicy{MinLength: cfg.PasswordMinLength},
		defaultExpiryDays: branchCfg.DefaultExpiryDays}
}

// WithLogoStorage enables account-logo uploads and public reads.
func (s *AccountService) WithLogoStorage(storage domain.ObjectStorage, maxBytes int64) *AccountService {
	s.logoStorage = storage
	s.logoMaxBytes = maxBytes
	return s
}

// Get returns the caller's own account.
func (s *AccountService) Get(ctx context.Context, tenant domain.Tenant) (*domain.Account, error) {
	var account *domain.Account
	if err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var err error
		account, err = s.accounts.GetByID(ctx, q, tenant.AccountID)
		return err
	}); err != nil {
		return nil, err
	}
	return account, nil
}

// DownloadLogo returns one public account logo by its unguessable identifier.
func (s *AccountService) DownloadLogo(
	ctx context.Context, accountID, logoID uuid.UUID,
) (*domain.StoredObject, error) {
	if s.logoStorage == nil {
		return nil, fmt.Errorf("account logo storage is unavailable")
	}
	return s.logoStorage.Download(ctx, brandLogoKey(accountID, logoID))
}

// Register opens an account with its first branch and administrator.
//
// It is the one write that cannot run under row level security: there is no account yet, so
// there is no scope to set. It therefore runs on the owner pool, in a single transaction —
// an account without a branch, or a branch without its manual-entry channel, is not a usable
// half-state to leave behind.
func (s *AccountService) Register(
	ctx context.Context, in domain.Signup,
) (*domain.SignupResult, *domain.TokenPair, error) {
	email := domain.NormalizeEmail(in.AdminEmail)
	if email == "" {
		return nil, nil, fmt.Errorf("%w: email is required", domain.ErrInvalidInput)
	}
	if err := s.policy.Validate(in.AdminPassword); err != nil {
		return nil, nil, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(in.AdminPassword), bcrypt.DefaultCost)
	if err != nil {
		return nil, nil, fmt.Errorf("hash password: %w", err)
	}

	tx, err := s.db.AdminTx(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Login resolves a user by email across every account, so an address already in use
	// would make the resulting session ambiguous.
	taken, err := s.users.ExistsByEmailCrossAccount(ctx, tx, email)
	if err != nil {
		return nil, nil, err
	}
	if taken {
		return nil, nil, domain.WithCode(domain.CodeEmailTaken, domain.ErrConflict)
	}

	account, err := s.accounts.Create(ctx, tx, strings.TrimSpace(in.AccountName),
		in.LegalName, in.TaxID)
	if err != nil {
		return nil, nil, err
	}
	if _, err := s.onboarding.Create(ctx, tx, account.ID); err != nil {
		return nil, nil, err
	}
	branch, err := s.branches.Create(ctx, tx, account.ID, domain.NewBranch{
		Name:              strings.TrimSpace(in.BranchName),
		Address:           in.BranchAddress,
		DefaultExpiryDays: s.defaultExpiryDays,
	})
	if err != nil {
		return nil, nil, err
	}
	if err := s.channels.CreateManualEntry(ctx, tx, account.ID, branch.ID); err != nil {
		return nil, nil, err
	}
	admin, err := s.users.Create(ctx, tx, account.ID, domain.NewUser{
		Name:  strings.TrimSpace(in.AdminName),
		Email: email,
		Role:  domain.UserRoleAdmin,
	}, string(hash))
	if err != nil {
		return nil, nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, nil, err
	}

	// After the commit, and not fatal: a transport that is down must not undo an account
	// that already exists. The unsent link is recoverable from the resend route.
	if err := s.verifier.Send(ctx, *admin); err != nil {
		s.log.ErrorContext(ctx, "verification mail not issued",
			slog.String("user_id", admin.ID.String()), slog.Any("error", err))
	}

	pair, err := s.tokens.IssueForUser(ctx, *admin)
	if err != nil {
		return nil, nil, err
	}
	return &domain.SignupResult{Account: *account, Branch: *branch, Admin: *admin}, pair, nil
}

// Update replaces the caller's account record, including the brand the client webapp renders.
func (s *AccountService) Update(
	ctx context.Context, tenant domain.Tenant, in domain.AccountUpdate,
) (*domain.Account, error) {
	if strings.TrimSpace(in.Name) == "" {
		return nil, fmt.Errorf("%w: name is required", domain.ErrInvalidInput)
	}
	var account *domain.Account
	if err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var err error
		account, err = s.accounts.Update(ctx, q, tenant.AccountID, in)
		return err
	}); err != nil {
		return nil, err
	}
	return account, nil
}

// UploadLogo validates and stores one PNG or JPEG account logo.
func (s *AccountService) UploadLogo(
	ctx context.Context, tenant domain.Tenant, file domain.AccountLogoUpload,
) (*domain.AccountLogo, error) {
	if s.logoStorage == nil {
		return nil, fmt.Errorf("account logo storage is unavailable")
	}
	file.ContentType = normalizeContentType(file.ContentType)
	if file.Size <= 0 {
		return nil, fmt.Errorf("%w: the logo is empty", domain.ErrInvalidInput)
	}
	if file.Size > s.logoMaxBytes {
		return nil, fmt.Errorf("%w: the logo exceeds %d bytes", domain.ErrTooLarge, s.logoMaxBytes)
	}
	if file.ContentType != "image/png" && file.ContentType != "image/jpeg" {
		return nil, domain.WithCode(domain.CodeUnsupportedFileType,
			fmt.Errorf("%w: account logos must be PNG or JPEG", domain.ErrInvalidInput))
	}

	content := bufio.NewReader(file.Content)
	head, err := content.Peek(512)
	if err != nil && len(head) == 0 {
		return nil, fmt.Errorf("%w: read logo header", domain.ErrInvalidInput)
	}
	if detected := http.DetectContentType(head); detected != file.ContentType {
		return nil, domain.WithCode(domain.CodeUnsupportedFileType,
			fmt.Errorf("%w: logo bytes are %s, not %s", domain.ErrInvalidInput, detected, file.ContentType))
	}

	logo := &domain.AccountLogo{ID: uuid.New(), AccountID: tenant.AccountID}
	if err := s.logoStorage.Upload(ctx, brandLogoKey(logo.AccountID, logo.ID), file.ContentType, content); err != nil {
		return nil, err
	}
	return logo, nil
}

func brandLogoKey(accountID, logoID uuid.UUID) string {
	return path.Join("accounts", accountID.String(), "brand", logoID.String())
}
