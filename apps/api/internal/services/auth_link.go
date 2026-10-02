package services

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// authTokenRepository is the single-use-token surface.
type authTokenRepository interface {
	GetByHashCrossAccount(ctx context.Context, q repository.Querier, hash string) (*domain.AuthToken, error)
	Create(ctx context.Context, q repository.Querier, t domain.AuthToken) error
	Consume(ctx context.Context, q repository.Querier, accountID, id uuid.UUID) error
	InvalidateActive(ctx context.Context, q repository.Querier, accountID, userID uuid.UUID, tokenType domain.AuthTokenType) error
	InvalidateAllForUser(ctx context.Context, q repository.Querier, accountID, userID uuid.UUID) error
}

// mailSender is the outbound-mail surface.
type mailSender interface {
	Send(ctx context.Context, out OutboundMail) error
}

// authLinkIssuer is what every mailed single-use link shares: retire the outstanding ones,
// mint one, mail it, and resolve a presented one.
type authLinkIssuer struct {
	db        tenantScoper
	tokens    authTokenRepository
	mail      mailSender
	log       *slog.Logger
	baseURL   string
	now       func() time.Time
	newSecret func() (string, error)
}

// issue mints the link and hands it to compose, which turns it into the message to send. A
// delivery failure is recorded by the mail service and deliberately does not fail the caller.
func (i *authLinkIssuer) issue(
	ctx context.Context,
	user domain.AppUser,
	tokenType domain.AuthTokenType,
	ttl time.Duration,
	compose func(link string) OutboundMail,
) error {
	var link string
	if err := i.db.InTenantTx(ctx, domain.Tenant{AccountID: user.AccountID}, func(q repository.Querier) error {
		var mintErr error
		link, mintErr = i.mint(ctx, q, user, tokenType, ttl)
		return mintErr
	}); err != nil {
		return err
	}
	i.deliver(ctx, user, tokenType, compose(link))
	return nil
}

// mint retires the user's outstanding links of the type and stores a fresh one in the caller's
// transaction, returning the link to mail once that transaction commits.
func (i *authLinkIssuer) mint(
	ctx context.Context, q repository.Querier, user domain.AppUser,
	tokenType domain.AuthTokenType, ttl time.Duration,
) (string, error) {
	raw, err := i.newSecret()
	if err != nil {
		return "", err
	}
	if err := i.tokens.InvalidateActive(ctx, q, user.AccountID, user.ID, tokenType); err != nil {
		return "", err
	}
	if err := i.tokens.Create(ctx, q, domain.AuthToken{
		AccountID: user.AccountID,
		UserID:    user.ID,
		Type:      tokenType,
		TokenHash: hashToken(raw),
		ExpiresAt: i.now().Add(ttl),
	}); err != nil {
		return "", err
	}
	return i.link(tokenType, raw), nil
}

// deliver sends a message carrying a minted link. A failure is logged, not returned: the link
// is already stored and can be mailed again.
func (i *authLinkIssuer) deliver(
	ctx context.Context, user domain.AppUser, tokenType domain.AuthTokenType, out OutboundMail,
) {
	if err := i.mail.Send(ctx, out); err != nil {
		i.log.ErrorContext(ctx, "outbound mail not delivered",
			slog.String("token_type", string(tokenType)),
			slog.String("user_id", user.ID.String()), slog.Any("error", err))
	}
}

// redeem resolves a presented link and reports the token behind it. An unknown, expired,
// already-used or wrong-type token is domain.ErrUnauthenticated alike.
func (i *authLinkIssuer) redeem(
	ctx context.Context, rawToken string, tokenTypes ...domain.AuthTokenType,
) (*domain.AuthToken, error) {
	stored, err := i.tokens.GetByHashCrossAccount(ctx, i.db.CrossAccount(), hashToken(rawToken))
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.WithCode(domain.CodeInvalidLink, domain.ErrUnauthenticated)
		}
		return nil, err
	}
	if !slices.Contains(tokenTypes, stored.Type) || !stored.IsUsable(i.now()) {
		return nil, domain.WithCode(domain.CodeInvalidLink, domain.ErrUnauthenticated)
	}
	return stored, nil
}

// link is a backoffice route, not an API one: the user clicks it and lands on a screen. An
// invite shares the reset screen, which only words itself differently for it.
func (i *authLinkIssuer) link(tokenType domain.AuthTokenType, rawToken string) string {
	link := strings.TrimSuffix(i.baseURL, "/") + routeFor(tokenType) +
		"?token=" + url.QueryEscape(rawToken)
	if tokenType == domain.AuthTokenTypeInvite {
		link += "&invite=1"
	}
	return link
}

func routeFor(tokenType domain.AuthTokenType) string {
	if tokenType == domain.AuthTokenTypeEmailVerification {
		return "/verify-email"
	}
	return "/reset-password"
}
