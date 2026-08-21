package services

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// verificationUserRepository is the app_user surface the confirmation flow needs.
type verificationUserRepository interface {
	GetAuthSubjectByID(ctx context.Context, q repository.Querier, accountID, id uuid.UUID) (*domain.AuthSubject, error)
	GetAuthSubjectByEmailCrossAccount(ctx context.Context, q repository.Querier, email string) (*domain.AuthSubject, error)
	UpdateEmail(ctx context.Context, q repository.Querier, accountID, id uuid.UUID, email string) (*domain.AppUser, error)
	MarkEmailVerified(ctx context.Context, q repository.Querier, accountID, id uuid.UUID) error
}

// VerificationService owns the address a user is reachable at and the proof of it: mailing the
// link, redeeming it, and the self-service correction that drops it. It does not close address
// squatting on its own — see docs/technical/authentication.md.
type VerificationService struct {
	db    tenantScoper
	users verificationUserRepository
	mail  mailSender
	log   *slog.Logger
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
		mail:  mail,
		log:   log,
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
			return domain.WithCode(domain.CodeInvalidLink, domain.ErrUnauthenticated)
		}
		// Already verified is a success, not a failure: the link did its job the first time
		// and the user has no way to tell the two clicks apart.
		if user.EmailVerifiedAt != nil {
			return nil
		}
		return s.users.MarkEmailVerified(ctx, q, stored.AccountID, stored.UserID)
	})
	if errors.Is(err, domain.ErrConflict) || errors.Is(err, domain.ErrNotFound) {
		return domain.WithCode(domain.CodeInvalidLink, domain.ErrUnauthenticated)
	}
	return err
}

// ChangeOwnEmail replaces the caller's own address once their current password checks out. The
// change drops the confirmation and mails the new address a link, so the account closes again
// until that one is redeemed. It is exempt from the requirement it re-imposes: closing it would
// leave whoever mistyped their address at signup with no way to correct it.
func (s *VerificationService) ChangeOwnEmail(
	ctx context.Context, tenant domain.Tenant, currentPassword, newEmail string,
) error {
	email := domain.NormalizeEmail(newEmail)

	var user *domain.AuthSubject
	if err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		u, err := s.users.GetAuthSubjectByID(ctx, q, tenant.AccountID, tenant.UserID)
		if err != nil {
			return err
		}
		user = u
		return nil
	}); err != nil {
		return err
	}

	// Outside the transaction, the way PasswordService.ChangeOwn does it: bcrypt is slow by
	// design and a connection held across it is a pool slot spent waiting.
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(currentPassword)) != nil {
		return domain.ErrUnauthenticated
	}
	// The caller's own address answers as a taken one rather than as a rule of its own: the
	// remedy is the same, and a separate answer would report whether a stranger holds it.
	if domain.NormalizeEmail(user.Email) == email {
		return domain.WithCode(domain.CodeEmailTaken, domain.ErrConflict)
	}

	var updated *domain.AppUser
	if err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		u, updateErr := s.users.UpdateEmail(ctx, q, tenant.AccountID, tenant.UserID, email)
		if updateErr != nil {
			return updateErr
		}
		updated = u
		// A recovery link already mailed to the old address stays redeemable, and that mailbox
		// is not the account's any more.
		return s.links.tokens.InvalidateActive(ctx, q, tenant.AccountID, tenant.UserID,
			domain.AuthTokenTypePasswordReset)
	}); err != nil {
		return err
	}

	s.notifyPreviousAddress(ctx, user.AppUser, updated.Email)
	return s.Send(ctx, *updated)
}

// notifyPreviousAddress tells the old mailbox that the account left it, because a silent change
// is how a takeover goes unnoticed. A failed delivery does not fail the change: the address has
// already moved and there is nothing here to undo.
func (s *VerificationService) notifyPreviousAddress(
	ctx context.Context, previous domain.AppUser, newEmail string,
) {
	if err := s.mail.Send(ctx, OutboundMail{
		AccountID: previous.AccountID,
		UserID:    &previous.ID,
		Event:     domain.NotificationEventEmailChanged,
		To:        previous.Email,
		ToName:    previous.Name,
		Subject:   emailChangedSubject,
		Heading:   emailChangedHeading,
		Paragraphs: []string{
			emailChangedIntro(previous.Name, newEmail),
			emailChangedWarning,
		},
	}); err != nil {
		s.log.ErrorContext(ctx, "outbound mail not delivered",
			slog.String("event", string(domain.NotificationEventEmailChanged)),
			slog.String("user_id", previous.ID.String()), slog.Any("error", err))
	}
}
