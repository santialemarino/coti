package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// tokenSecretBytes is the entropy behind every opaque token the API mints — refresh tokens
// and the single-use links alike — high enough that a fast hash is enough to store it.
const tokenSecretBytes = 32

// dummyHash is compared against when the email is unknown, so response latency does not
// leak which addresses are registered.
const dummyHash = "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"

// userRepository is the persistence surface the auth flow needs. Defined here, in the
// consumer, so a test can fake it without a database.
type userRepository interface {
	GetAuthSubjectByID(ctx context.Context, q repository.Querier, accountID, id uuid.UUID) (*domain.AuthSubject, error)
	GetAuthSubjectByEmailCrossAccount(ctx context.Context, q repository.Querier, email string) (*domain.AuthSubject, error)
	RegisterFailedAttempt(ctx context.Context, q repository.Querier, accountID, id uuid.UUID, maxAttempts int, lockFor time.Duration) (int, error)
	RegisterSuccessfulLogin(ctx context.Context, q repository.Querier, accountID, id uuid.UUID) error
	BumpSessionEpoch(ctx context.Context, q repository.Querier, accountID, id uuid.UUID) (int, error)
}

// refreshTokenRepository is the refresh-token persistence surface.
type refreshTokenRepository interface {
	GetByHashCrossAccount(ctx context.Context, q repository.Querier, hash string) (*domain.RefreshToken, error)
	Create(ctx context.Context, q repository.Querier, t domain.RefreshToken) error
	Consume(ctx context.Context, q repository.Querier, accountID, id uuid.UUID) error
	RevokeFamily(ctx context.Context, q repository.Querier, accountID, familyID uuid.UUID) error
}

// branchRepository is the branch-access surface the tenant resolution needs.
type branchRepository interface {
	IsAccessibleBy(ctx context.Context, q repository.Querier, accountID, userID, branchID uuid.UUID, isAdmin bool) (bool, error)
	ListIDsForUser(ctx context.Context, q repository.Querier, accountID, userID uuid.UUID, isAdmin bool) ([]uuid.UUID, error)
}

// tenantScoper is the database surface: a tenant-scoped transaction, plus the owner
// pool for the lookups that cannot know the account yet.
type tenantScoper interface {
	InTenantTx(ctx context.Context, tenant domain.Tenant, fn func(repository.Querier) error) error
	CrossAccount() repository.Querier
}

// AuthService owns login, refresh, and logout.
type AuthService struct {
	db        tenantScoper
	users     userRepository
	branches  branchRepository
	tokens    refreshTokenRepository
	access    *TokenService
	cfg       config.AuthConfig
	now       func() time.Time
	newSecret func() (string, error)
}

// NewAuthService builds an AuthService. now and newSecret are injectable so expiry and
// rotation are testable deterministically.
func NewAuthService(
	db tenantScoper, users userRepository, branches branchRepository,
	tokens refreshTokenRepository, access *TokenService, cfg config.AuthConfig,
	now func() time.Time,
) *AuthService {
	if now == nil {
		now = time.Now
	}
	return &AuthService{
		db: db, users: users, branches: branches, tokens: tokens, access: access, cfg: cfg,
		now: now, newSecret: newTokenSecret,
	}
}

// Login verifies credentials and issues a token pair. A bad email, a bad password, a
// deactivated user and a deactivated account all return domain.ErrUnauthenticated,
// indistinguishably on purpose.
func (s *AuthService) Login(ctx context.Context, in domain.Credentials) (*domain.TokenPair, error) {
	now := s.now()

	user, err := s.users.GetAuthSubjectByEmailCrossAccount(ctx, s.db.CrossAccount(), in.Email)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			// Spend the same time as a real comparison before failing.
			_ = bcrypt.CompareHashAndPassword([]byte(dummyHash), []byte(in.Password))
			return nil, domain.ErrUnauthenticated
		}
		return nil, err
	}

	if user.IsLocked(now) {
		return nil, domain.ErrLocked
	}

	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(in.Password)) != nil {
		tenant := domain.Tenant{AccountID: user.AccountID}
		if err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
			_, regErr := s.users.RegisterFailedAttempt(ctx, q, user.AccountID, user.ID,
				s.cfg.MaxFailedAttempts, s.cfg.LockoutDuration)
			return regErr
		}); err != nil {
			return nil, err
		}
		return nil, domain.ErrUnauthenticated
	}

	// Checked after the password so an attacker cannot enumerate disabled users or
	// deactivated corralones.
	if !user.IsUsable() {
		return nil, domain.ErrUnauthenticated
	}

	pair, err := s.issuePair(ctx, user.AppUser, uuid.New(), in.RememberMe, func(q repository.Querier) error {
		return s.users.RegisterSuccessfulLogin(ctx, q, user.AccountID, user.ID)
	})
	if err != nil {
		return nil, err
	}
	return pair, nil
}

// IssueForUser mints a fresh token family for a user, so a registration can hand back a
// session instead of bouncing the caller through login.
func (s *AuthService) IssueForUser(ctx context.Context, user domain.AppUser) (*domain.TokenPair, error) {
	return s.issuePair(ctx, user, uuid.New(), false, nil)
}

// Refresh consumes the presented token and mints its successor in the same family.
// Replaying a consumed token inside the grace window is a benign race; past it, the whole
// family is revoked as theft.
func (s *AuthService) Refresh(ctx context.Context, rawToken string) (*domain.TokenPair, error) {
	now := s.now()

	stored, err := s.tokens.GetByHashCrossAccount(ctx, s.db.CrossAccount(), hashToken(rawToken))
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.ErrUnauthenticated
		}
		return nil, err
	}
	if stored.RevokedAt != nil || !stored.ExpiresAt.After(now) {
		return nil, domain.ErrUnauthenticated
	}

	tenant := domain.Tenant{AccountID: stored.AccountID}

	if stored.ConsumedAt != nil {
		if now.Sub(*stored.ConsumedAt) > s.cfg.RefreshReuseGrace {
			if err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
				return s.tokens.RevokeFamily(ctx, q, stored.AccountID, stored.FamilyID)
			}); err != nil {
				return nil, err
			}
			return nil, domain.ErrUnauthenticated
		}
		// Inside the grace window: treat it as a race and hand out a fresh rotation.
	}

	var user *domain.AuthSubject
	if err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		u, getErr := s.users.GetAuthSubjectByID(ctx, q, stored.AccountID, stored.UserID)
		if getErr != nil {
			return getErr
		}
		user = u
		return nil
	}); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.ErrUnauthenticated
		}
		return nil, err
	}
	if !user.IsUsable() {
		return nil, domain.ErrUnauthenticated
	}
	if user.IsLocked(now) {
		return nil, domain.ErrLocked
	}

	// Consuming and minting the successor in one transaction keeps the family
	// consistent: a crash between them cannot strand a session with no live token.
	return s.issuePair(ctx, user.AppUser, stored.FamilyID, s.isRememberMe(*stored), func(q repository.Querier) error {
		if stored.ConsumedAt == nil {
			return s.tokens.Consume(ctx, q, stored.AccountID, stored.ID)
		}
		return nil
	})
}

// Logout bumps the session epoch, which invalidates every outstanding access token for
// the user, and revokes the presented refresh family. rawToken may be empty.
func (s *AuthService) Logout(ctx context.Context, tenant domain.Tenant, rawToken string) error {
	var familyID *uuid.UUID
	if rawToken != "" {
		stored, err := s.tokens.GetByHashCrossAccount(ctx, s.db.CrossAccount(), hashToken(rawToken))
		// A token that is unknown or belongs to someone else is ignored: logout must
		// still invalidate the caller's own session.
		if err == nil && stored.UserID == tenant.UserID {
			familyID = &stored.FamilyID
		}
	}

	return s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		if _, err := s.users.BumpSessionEpoch(ctx, q, tenant.AccountID, tenant.UserID); err != nil {
			return err
		}
		if familyID != nil {
			return s.tokens.RevokeFamily(ctx, q, tenant.AccountID, *familyID)
		}
		return nil
	})
}

// ResolveTenant turns verified token claims into the request's tenant scope, confirming
// what the signature cannot: the user exists, both they and their account are active, they
// are not locked out, and they carry the token's session epoch.
//
// Reading the account here is what makes deactivating a corralón reach the tokens it has
// already issued, rather than only the logins it has yet to grant.
//
// Validating requestedBranch is load-bearing — row level security guards the account
// boundary, not the branch one, so a trusted branch id would let a caller read another
// branch of their own account. Inaccessible returns domain.ErrForbidden, never a silent
// downgrade to account-wide. A seller who selects no branch is confined to the ones they are
// assigned, so omitting the header cannot widen their reach either.
func (s *AuthService) ResolveTenant(
	ctx context.Context, claims domain.AccessClaims, requestedBranch uuid.UUID,
) (domain.Tenant, error) {
	scope := domain.Tenant{AccountID: claims.AccountID}

	var user *domain.AuthSubject
	var allowedBranches []uuid.UUID
	branchOK := true

	if err := s.db.InTenantTx(ctx, scope, func(q repository.Querier) error {
		u, err := s.users.GetAuthSubjectByID(ctx, q, claims.AccountID, claims.UserID)
		if err != nil {
			return err
		}
		user = u
		isAdmin := u.Role == domain.UserRoleAdmin

		if requestedBranch != uuid.Nil {
			ok, brErr := s.branches.IsAccessibleBy(ctx, q, claims.AccountID, claims.UserID,
				requestedBranch, isAdmin)
			if brErr != nil {
				return brErr
			}
			branchOK = ok
			return nil
		}

		// An admin reaches the whole account, so there is no set to load.
		if !isAdmin {
			ids, listErr := s.branches.ListIDsForUser(ctx, q, claims.AccountID, claims.UserID, false)
			if listErr != nil {
				return listErr
			}
			allowedBranches = ids
		}
		return nil
	}); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.Tenant{}, domain.ErrUnauthenticated
		}
		return domain.Tenant{}, err
	}

	if !user.IsUsable() || user.SessionEpoch != claims.SessionEpoch {
		return domain.Tenant{}, domain.ErrUnauthenticated
	}
	if user.IsLocked(s.now()) {
		return domain.Tenant{}, domain.ErrLocked
	}
	if !branchOK {
		return domain.Tenant{}, domain.ErrForbidden
	}

	return domain.Tenant{
		AccountID:        user.AccountID,
		UserID:           user.ID,
		Role:             user.Role,
		EmailVerified:    user.EmailVerifiedAt != nil,
		BranchID:         requestedBranch,
		AllowedBranchIDs: allowedBranches,
	}, nil
}

// issuePair mints a refresh token in the given family and signs an access token,
// running extra alongside the insert in the same transaction.
func (s *AuthService) issuePair(
	ctx context.Context, user domain.AppUser, familyID uuid.UUID, rememberMe bool,
	extra func(repository.Querier) error,
) (*domain.TokenPair, error) {
	raw, err := s.newSecret()
	if err != nil {
		return nil, err
	}

	ttl := s.cfg.RefreshTTL
	if rememberMe {
		ttl = s.cfg.RefreshRememberTTL
	}

	tenant := domain.Tenant{AccountID: user.AccountID, UserID: user.ID, Role: user.Role}
	if err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		if extra != nil {
			if err := extra(q); err != nil {
				return err
			}
		}
		return s.tokens.Create(ctx, q, domain.RefreshToken{
			AccountID: user.AccountID,
			UserID:    user.ID,
			FamilyID:  familyID,
			TokenHash: hashToken(raw),
			ExpiresAt: s.now().Add(ttl),
		})
	}); err != nil {
		return nil, err
	}

	accessToken, expiresAt, err := s.access.IssueAccessToken(user)
	if err != nil {
		return nil, err
	}

	return &domain.TokenPair{
		AccessToken:     accessToken,
		AccessExpiresAt: expiresAt,
		RefreshToken:    raw,
		Tenant:          tenant,
	}, nil
}

// isRememberMe infers the original session length from the stored token's lifetime, so
// a rotation keeps it instead of silently downgrading a "remember me" session.
func (s *AuthService) isRememberMe(t domain.RefreshToken) bool {
	lifetime := t.ExpiresAt.Sub(t.CreatedAt)
	return lifetime > s.cfg.RefreshTTL
}

// hashToken returns the hex SHA-256 of a raw token. The raw value is high-entropy, so a
// fast hash is sufficient — unlike a password, it is not guessable.
func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// newTokenSecret returns a URL-safe random string to hand out as an opaque token.
func newTokenSecret() (string, error) {
	buf := make([]byte, tokenSecretBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate token secret: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// HashPassword hashes a plaintext password for storage. The cost is a cryptographic
// constant, so it is not env-configurable.
func HashPassword(plain string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(hash), nil
}
