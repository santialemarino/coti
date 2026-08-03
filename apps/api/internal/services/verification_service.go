package services

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// verificationUserRepository is the app_user surface the confirmation flow needs.
type verificationUserRepository interface {
	GetAuthSubjectByID(ctx context.Context, q repository.Querier, accountID, id uuid.UUID) (*domain.AuthSubject, error)
	GetAuthSubjectByEmailCrossAccount(ctx context.Context, q repository.Querier, email string) (*domain.AuthSubject, error)
	MarkEmailVerified(ctx context.Context, q repository.Querier, accountID, id uuid.UUID) error
}

// VerificationService confirms that whoever registered owns the address they used. It does
// not close address squatting on its own — see docs/technical/authentication.md.
type VerificationService struct {
	db    tenantScoper
	users verificationUserRepository
	links *authLinkIssuer
	ttl   time.Duration
}

// NewVerificationService builds a VerificationService.
func NewVerificationService(
	db tenantScoper, users verificationUserRepository, tokens authTokenRepository,
	mail mailSender, log *slog.Logger, cfg config.AuthConfig, web config.WebConfig,
	now func() time.Time,
) *VerificationService {
	if now == nil {
		now = time.Now
	}
	if log == nil {
		log = slog.Default()
	}
	return &VerificationService{
		db:    db,
		users: users,
		links: &authLinkIssuer{
			db: db, tokens: tokens, mail: mail, log: log,
			baseURL: web.BackofficeURL, now: now, newSecret: newTokenSecret,
		},
		ttl: cfg.VerificationTTL,
	}
}

// Send mails a confirmation link to a freshly registered user.
func (s *VerificationService) Send(ctx context.Context, user domain.AppUser) error {
	return s.links.issue(ctx, user, domain.AuthTokenTypeEmailVerification, s.ttl,
		func(link string) OutboundMail {
			return OutboundMail{
				AccountID: user.AccountID,
				UserID:    &user.ID,
				Event:     domain.NotificationEventEmailVerification,
				To:        user.Email,
				ToName:    user.Name,
				Subject:   emailVerificationSubject,
				Heading:   emailVerificationHeading,
				Paragraphs: []string{
					emailVerificationIntro(user.Name),
					emailVerificationValidity(int(s.ttl.Hours())),
				},
				ActionLabel: emailVerificationAction,
				ActionURL:   link,
			}
		})
}

// Resend mails a fresh link, retiring the outstanding one. It answers the same whether or
// not the address is registered, for the reason Forgot does: the caller has no session, so
// telling them would be an enumeration oracle.
func (s *VerificationService) Resend(ctx context.Context, email string) error {
	user, err := s.users.GetAuthSubjectByEmailCrossAccount(ctx, s.db.CrossAccount(), domain.NormalizeEmail(email))
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil
		}
		return err
	}
	// Nothing to confirm for a dead account, and nothing to re-confirm for a live one that
	// already did.
	if !user.IsUsable() || user.EmailVerifiedAt != nil {
		return nil
	}
	return s.Send(ctx, user.AppUser)
}

// Confirm redeems a verification link and stamps the address as verified. An unknown,
// expired, already-used or wrong-type token all answer domain.ErrUnauthenticated alike.
func (s *VerificationService) Confirm(ctx context.Context, rawToken string) error {
	stored, err := s.links.redeem(ctx, rawToken, domain.AuthTokenTypeEmailVerification)
	if err != nil {
		return err
	}

	err = s.db.InTenantTx(ctx, domain.Tenant{AccountID: stored.AccountID}, func(q repository.Querier) error {
		// Redeeming first is what serializes two concurrent uses of one link.
		if consumeErr := s.links.tokens.Consume(ctx, q, stored.AccountID, stored.ID); consumeErr != nil {
			return consumeErr
		}
		user, getErr := s.users.GetAuthSubjectByID(ctx, q, stored.AccountID, stored.UserID)
		if getErr != nil {
			return getErr
		}
		if !user.IsUsable() {
			return domain.ErrUnauthenticated
		}
		// Already verified is a success, not a failure: the link did its job the first time
		// and the user has no way to tell the two clicks apart.
		if user.EmailVerifiedAt != nil {
			return nil
		}
		return s.users.MarkEmailVerified(ctx, q, stored.AccountID, stored.UserID)
	})
	if errors.Is(err, domain.ErrConflict) || errors.Is(err, domain.ErrNotFound) {
		return domain.ErrUnauthenticated
	}
	return err
}
