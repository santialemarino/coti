package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// passwordUserRepository is the app_user surface the credential flows need.
type passwordUserRepository interface {
	GetByID(ctx context.Context, q repository.Querier, accountID, id uuid.UUID) (*domain.AppUser, error)
	GetByEmailCrossAccount(ctx context.Context, q repository.Querier, email string) (*domain.AppUser, error)
	UpdatePassword(ctx context.Context, q repository.Querier, accountID, id uuid.UUID, passwordHash string) error
	UpdatePasswordIfCurrent(ctx context.Context, q repository.Querier, accountID, id uuid.UUID, currentHash, passwordHash string) error
	BumpSessionEpoch(ctx context.Context, q repository.Querier, accountID, id uuid.UUID) (int, error)
}

// authTokenRepository is the single-use-token surface.
type authTokenRepository interface {
	GetByHashCrossAccount(ctx context.Context, q repository.Querier, hash string) (*domain.AuthToken, error)
	Create(ctx context.Context, q repository.Querier, t domain.AuthToken) error
	Consume(ctx context.Context, q repository.Querier, accountID, id uuid.UUID) error
	InvalidateActive(ctx context.Context, q repository.Querier, accountID, userID uuid.UUID, tokenType domain.AuthTokenType) error
}

// sessionRevoker drops the refresh tokens a changed credential must not keep alive.
type sessionRevoker interface {
	RevokeAllForUser(ctx context.Context, q repository.Querier, accountID, userID uuid.UUID) error
}

// mailSender is the outbound-mail surface.
type mailSender interface {
	Send(ctx context.Context, out OutboundMail) error
}

// PasswordService owns the three ways a password changes after the user exists.
type PasswordService struct {
	db            tenantScoper
	users         passwordUserRepository
	tokens        authTokenRepository
	sessions      sessionRevoker
	mail          mailSender
	issuer        tokenIssuer
	log           *slog.Logger
	minLength     int
	resetTTL      time.Duration
	backofficeURL string
	now           func() time.Time
	newSecret     func() (string, error)
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
		db: db, users: users, tokens: tokens, sessions: sessions, mail: mail, issuer: issuer,
		log: log, minLength: cfg.PasswordMinLength, resetTTL: cfg.PasswordResetTTL,
		backofficeURL: web.BackofficeURL, now: now, newSecret: newTokenSecret,
	}
}

// ChangeOwn replaces the caller's password after checking the current one, and returns a fresh
// token pair, because the change ends the caller's own session along with every other.
func (s *PasswordService) ChangeOwn(
	ctx context.Context, tenant domain.Tenant, current, next string,
) (*domain.TokenPair, error) {
	if err := s.validateLength(next); err != nil {
		return nil, err
	}

	var user *domain.AppUser
	if err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		u, err := s.users.GetByID(ctx, q, tenant.AccountID, tenant.UserID)
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
	return s.issuer.IssueForUser(ctx, *user)
}

// Forgot mails a single-use recovery link, answering the same whether or not the address is
// registered so the response cannot be used to enumerate users.
func (s *PasswordService) Forgot(ctx context.Context, email string) error {
	user, err := s.users.GetByEmailCrossAccount(ctx, s.db.CrossAccount(), normalizeEmail(email))
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil
		}
		return err
	}
	if !user.IsActive {
		return nil
	}
	return s.issueResetLink(ctx, *user)
}

// Reset redeems a recovery link and sets the new password. An unknown, expired, already-used
// or wrong-type token all answer domain.ErrUnauthenticated alike.
func (s *PasswordService) Reset(ctx context.Context, rawToken, next string) error {
	if err := s.validateLength(next); err != nil {
		return err
	}

	stored, err := s.tokens.GetByHashCrossAccount(ctx, s.db.CrossAccount(), hashToken(rawToken))
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ErrUnauthenticated
		}
		return err
	}
	if stored.Type != domain.AuthTokenTypePasswordReset || !stored.IsUsable(s.now()) {
		return domain.ErrUnauthenticated
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
		user, getErr := s.users.GetByID(ctx, q, stored.AccountID, stored.UserID)
		if getErr != nil {
			return getErr
		}
		if !user.IsActive {
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
	var user *domain.AppUser
	if err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		// Reading inside the account scope is what confines the reset to it: a user of
		// another account is simply not there.
		u, err := s.users.GetByID(ctx, q, tenant.AccountID, userID)
		if err != nil {
			return err
		}
		user = u
		return nil
	}); err != nil {
		return err
	}
	if !user.IsActive {
		return fmt.Errorf("%w: a deactivated user cannot be sent a recovery link",
			domain.ErrInvalidInput)
	}
	return s.issueResetLink(ctx, *user)
}

// issueResetLink retires the user's outstanding links, mints one, and mails it. A delivery
// failure is recorded and deliberately does not fail the caller.
func (s *PasswordService) issueResetLink(ctx context.Context, user domain.AppUser) error {
	raw, err := s.newSecret()
	if err != nil {
		return err
	}

	if err := s.db.InTenantTx(ctx, domain.Tenant{AccountID: user.AccountID}, func(q repository.Querier) error {
		if invalidateErr := s.tokens.InvalidateActive(ctx, q, user.AccountID, user.ID,
			domain.AuthTokenTypePasswordReset); invalidateErr != nil {
			return invalidateErr
		}
		return s.tokens.Create(ctx, q, domain.AuthToken{
			AccountID: user.AccountID,
			UserID:    user.ID,
			Type:      domain.AuthTokenTypePasswordReset,
			TokenHash: hashToken(raw),
			ExpiresAt: s.now().Add(s.resetTTL),
		})
	}); err != nil {
		return err
	}

	if err := s.mail.Send(ctx, OutboundMail{
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
		ActionURL:   s.resetURL(raw),
	}); err != nil {
		s.log.ErrorContext(ctx, "password recovery mail not delivered",
			slog.String("user_id", user.ID.String()), slog.Any("error", err))
	}
	return nil
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

// resetURL is a backoffice route, not an API one: the user clicks it and lands on a screen.
func (s *PasswordService) resetURL(rawToken string) string {
	return fmt.Sprintf("%s/reset-password?token=%s",
		strings.TrimSuffix(s.backofficeURL, "/"), url.QueryEscape(rawToken))
}

// validateLength applies the configured floor, which is the same in all three paths.
func (s *PasswordService) validateLength(password string) error {
	if len([]rune(password)) < s.minLength {
		return fmt.Errorf("%w: password must be at least %d characters",
			domain.ErrInvalidInput, s.minLength)
	}
	return nil
}
