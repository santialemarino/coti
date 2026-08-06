package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// passwordUserRepository is the app_user surface the credential flows need.
type passwordUserRepository interface {
	GetAuthSubjectByID(ctx context.Context, q repository.Querier, accountID, id uuid.UUID) (*domain.AuthSubject, error)
	GetAuthSubjectByEmailCrossAccount(ctx context.Context, q repository.Querier, email string) (*domain.AuthSubject, error)
	UpdatePassword(ctx context.Context, q repository.Querier, accountID, id uuid.UUID, passwordHash string) error
	UpdatePasswordIfCurrent(ctx context.Context, q repository.Querier, accountID, id uuid.UUID, currentHash, passwordHash string) error
	BumpSessionEpoch(ctx context.Context, q repository.Querier, accountID, id uuid.UUID) (int, error)
}

// sessionRevoker drops the refresh tokens a changed credential must not keep alive.
type sessionRevoker interface {
	RevokeAllForUser(ctx context.Context, q repository.Querier, accountID, userID uuid.UUID) error
}

// PasswordService owns the three ways a password changes after the user exists.
type PasswordService struct {
	db       tenantScoper
	users    passwordUserRepository
	tokens   authTokenRepository
	sessions sessionRevoker
	links    *authLinkIssuer
	issuer   tokenIssuer
	policy   domain.PasswordPolicy
	resetTTL time.Duration
}

// NewPasswordService builds a PasswordService. now is injectable so expiry is deterministic.
func NewPasswordService(
	db tenantScoper, users passwordUserRepository, tokens authTokenRepository,
	sessions sessionRevoker, mail mailSender, issuer tokenIssuer, log *slog.Logger,
	cfg config.AuthConfig, web config.WebConfig, now func() time.Time,
) *PasswordService {
	if now == nil {
		now = time.Now
	}
	if log == nil {
		log = slog.Default()
	}
	return &PasswordService{
		db: db, users: users, tokens: tokens, sessions: sessions, issuer: issuer,
		links: &authLinkIssuer{
			db: db, tokens: tokens, mail: mail, log: log,
			baseURL: web.BackofficeURL, now: now, newSecret: newTokenSecret,
		},
		policy: domain.PasswordPolicy{MinLength: cfg.PasswordMinLength}, resetTTL: cfg.PasswordResetTTL,
	}
}

// ChangeOwn replaces the caller's password after checking the current one, and returns a fresh
// token pair, because the change ends the caller's own session along with every other.
func (s *PasswordService) ChangeOwn(
	ctx context.Context, tenant domain.Tenant, current, next string,
) (*domain.TokenPair, error) {
	if err := s.policy.Validate(next); err != nil {
		return nil, err
	}

	var user *domain.AuthSubject
	if err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		u, err := s.users.GetAuthSubjectByID(ctx, q, tenant.AccountID, tenant.UserID)
		if err != nil {
			return err
		}
		user = u
		return nil
	}); err != nil {
		return nil, err
	}

	// Both bcrypt calls sit outside the transaction; the write below is conditional on the
	// hash read here, so nothing that moved the credential meanwhile can be overwritten.
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(current)) != nil {
		return nil, domain.ErrUnauthenticated
	}
	hash, err := HashPassword(next)
	if err != nil {
		return nil, err
	}

	var epoch int
	err = s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		if updateErr := s.users.UpdatePasswordIfCurrent(ctx, q, tenant.AccountID, user.ID,
			user.PasswordHash, hash); updateErr != nil {
			return updateErr
		}
		var endErr error
		epoch, endErr = s.endSessions(ctx, q, tenant.AccountID, user.ID)
		return endErr
	})
	// A recovery link redeemed inside the bcrypt window moved the credential, and letting the
	// older password win here would undo it.
	if errors.Is(err, domain.ErrConflict) {
		return nil, domain.ErrUnauthenticated
	}
	if err != nil {
		return nil, err
	}

	// The new pair has to carry the epoch the bump just wrote, or the token it signs is
	// stale the moment it is issued.
	user.SessionEpoch = epoch
	return s.issuer.IssueForUser(ctx, user.AppUser)
}

// Forgot mails a single-use recovery link, answering the same whether or not the address is
// registered so the response cannot be used to enumerate users.
func (s *PasswordService) Forgot(ctx context.Context, email string) error {
	user, err := s.users.GetAuthSubjectByEmailCrossAccount(ctx, s.db.CrossAccount(), domain.NormalizeEmail(email))
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil
		}
		return err
	}
	if !user.IsUsable() {
		return nil
	}
	return s.issueResetLink(ctx, user.AppUser)
}

// Reset redeems a recovery link and sets the new password. An unknown, expired, already-used
// or wrong-type token all answer domain.ErrUnauthenticated alike.
func (s *PasswordService) Reset(ctx context.Context, rawToken, next string) error {
	if err := s.policy.Validate(next); err != nil {
		return err
	}

	stored, err := s.links.redeem(ctx, rawToken, domain.AuthTokenTypePasswordReset)
	if err != nil {
		return err
	}

	hash, err := HashPassword(next)
	if err != nil {
		return err
	}

	err = s.db.InTenantTx(ctx, domain.Tenant{AccountID: stored.AccountID}, func(q repository.Querier) error {
		// Redeeming first is what serializes two concurrent uses of one link: the loser's
		// UPDATE matches no row, so only one of them reaches the write below.
		if consumeErr := s.tokens.Consume(ctx, q, stored.AccountID, stored.ID); consumeErr != nil {
			return consumeErr
		}
		user, getErr := s.users.GetAuthSubjectByID(ctx, q, stored.AccountID, stored.UserID)
		if getErr != nil {
			return getErr
		}
		if !user.IsUsable() {
			return domain.ErrUnauthenticated
		}
		if updateErr := s.users.UpdatePassword(ctx, q, stored.AccountID, stored.UserID, hash); updateErr != nil {
			return updateErr
		}
		_, endErr := s.endSessions(ctx, q, stored.AccountID, stored.UserID)
		return endErr
	})
	if errors.Is(err, domain.ErrConflict) || errors.Is(err, domain.ErrNotFound) {
		return domain.ErrUnauthenticated
	}
	return err
}

// AdminReset mails the recovery link to another user of the administrator's own account, so
// the administrator never learns a password.
func (s *PasswordService) AdminReset(ctx context.Context, tenant domain.Tenant, userID uuid.UUID) error {
	var user *domain.AuthSubject
	if err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		// Reading inside the account scope is what confines the reset to it: a user of
		// another account is simply not there.
		u, err := s.users.GetAuthSubjectByID(ctx, q, tenant.AccountID, userID)
		if err != nil {
			return err
		}
		user = u
		return nil
	}); err != nil {
		return err
	}
	if !user.IsUsable() {
		return fmt.Errorf("%w: a deactivated user cannot be sent a recovery link",
			domain.ErrInvalidInput)
	}
	return s.issueResetLink(ctx, user.AppUser)
}

// issueResetLink mints a recovery link and mails it.
func (s *PasswordService) issueResetLink(ctx context.Context, user domain.AppUser) error {
	return s.links.issue(ctx, user, domain.AuthTokenTypePasswordReset, s.resetTTL,
		func(link string) OutboundMail {
			return OutboundMail{
				AccountID: user.AccountID,
				UserID:    &user.ID,
				Event:     domain.NotificationEventPasswordReset,
				To:        user.Email,
				ToName:    user.Name,
				Subject:   passwordResetSubject,
				Heading:   passwordResetHeading,
				Paragraphs: []string{
					passwordResetIntro(user.Name),
					passwordResetValidity(int(s.resetTTL.Minutes())),
					passwordResetIgnore,
				},
				ActionLabel: passwordResetAction,
				ActionURL:   link,
			}
		})
}

// endSessions drops every session the user holds, in the caller's transaction, and returns
// their new session epoch.
func (s *PasswordService) endSessions(
	ctx context.Context, q repository.Querier, accountID, userID uuid.UUID,
) (int, error) {
	epoch, err := s.users.BumpSessionEpoch(ctx, q, accountID, userID)
	if err != nil {
		return 0, err
	}
	// The epoch only kills access tokens. Without this, a refresh token on another device
	// mints a fresh one carrying the new epoch and the session simply continues.
	if err := s.sessions.RevokeAllForUser(ctx, q, accountID, userID); err != nil {
		return 0, err
	}
	return epoch, nil
}
