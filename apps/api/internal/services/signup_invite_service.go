package services

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// signupRoute is the backoffice screen an invite opens, with the token in signupInviteParam.
const (
	signupRoute       = "/signup"
	signupInviteParam = "invite"
)

// signupInviteRepository is the invite surface the team's command, the check and registration need.
type signupInviteRepository interface {
	GetByHash(ctx context.Context, q repository.Querier, hash string) (*domain.SignupInvite, error)
	GetByHashForUpdate(ctx context.Context, q repository.Querier, hash string) (*domain.SignupInvite, error)
	Create(ctx context.Context, q repository.Querier, in domain.NewSignupInvite, hash string, expiresAt time.Time) (*domain.SignupInvite, error)
	MarkUsed(ctx context.Context, q repository.Querier, id, accountID uuid.UUID, at time.Time) error
}

// ownerPool is the owner-role access a pre-account table needs: there is no tenant to scope to.
type ownerPool interface {
	AdminTx(ctx context.Context) (pgx.Tx, error)
	CrossAccount() repository.Querier
}

// SignupInviteService mints and checks the single-use links that open registration while public
// sign-up is closed. Registration itself redeems them, in its own transaction.
type SignupInviteService struct {
	db        ownerPool
	invites   signupInviteRepository
	ttl       time.Duration
	baseURL   string
	now       func() time.Time
	newSecret func() (string, error)
}

// NewSignupInviteService builds a SignupInviteService. now is injectable so expiry is testable.
func NewSignupInviteService(
	db ownerPool, invites signupInviteRepository, cfg config.SignupConfig, web config.WebConfig,
	now func() time.Time,
) *SignupInviteService {
	if now == nil {
		now = time.Now
	}
	return &SignupInviteService{db: db, invites: invites, ttl: cfg.InviteTTL,
		baseURL: web.BackofficeURL, now: now, newSecret: newTokenSecret}
}

// Check refuses an invite registration would refuse, without redeeming it.
func (s *SignupInviteService) Check(ctx context.Context, token string) error {
	_, err := redeemableInvite(ctx, s.invites.GetByHash, s.db.CrossAccount(), token, s.now())
	return err
}

// Create mints an invite and returns the link to hand over. The raw token exists only in that
// link: what is stored is its hash.
func (s *SignupInviteService) Create(
	ctx context.Context, in domain.NewSignupInvite,
) (string, *domain.SignupInvite, error) {
	raw, err := s.newSecret()
	if err != nil {
		return "", nil, err
	}
	tx, err := s.db.AdminTx(ctx)
	if err != nil {
		return "", nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	invite, err := s.invites.Create(ctx, tx, in, hashToken(raw), s.now().Add(s.ttl))
	if err != nil {
		return "", nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", nil, err
	}
	link := strings.TrimSuffix(s.baseURL, "/") + signupRoute + "?" +
		url.Values{signupInviteParam: {raw}}.Encode()
	return link, invite, nil
}

// redeemableInvite loads the invite a raw token names and refuses one that matches none, was used
// or has expired. lookup decides whether the row is locked.
func redeemableInvite(
	ctx context.Context,
	lookup func(context.Context, repository.Querier, string) (*domain.SignupInvite, error),
	q repository.Querier, token string, now time.Time,
) (*domain.SignupInvite, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, domain.WithCode(domain.CodeSignupInviteInvalid, domain.ErrForbidden)
	}
	invite, err := lookup(ctx, q, hashToken(token))
	if errors.Is(err, domain.ErrNotFound) {
		return nil, domain.WithCode(domain.CodeSignupInviteInvalid, domain.ErrForbidden)
	}
	if err != nil {
		return nil, err
	}
	if err := invite.Redeemable(now); err != nil {
		return nil, err
	}
	return invite, nil
}
