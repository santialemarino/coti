package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type gmailAuthorizationDB interface {
	tenantTxRunner
	CrossAccount() repository.Querier
	WithAdvisoryLock(context.Context, string, func() error) error
}

type gmailAuthorizationStore interface {
	CreateAuthorization(context.Context, repository.Querier, domain.GmailAuthorization) error
	ConsumeAuthorization(context.Context, repository.Querier, []byte, time.Time) (*domain.GmailAuthorization, error)
	SaveConnected(context.Context, repository.Querier, domain.GmailMailbox) error
}

type gmailAuthorizationUsers interface {
	GetAuthSubjectByID(context.Context, repository.Querier, uuid.UUID, uuid.UUID) (*domain.AuthSubject, error)
}

type gmailAuthorizationProvider interface {
	AuthorizationURL(string, string) string
	Exchange(context.Context, string, string) (domain.GmailGrant, error)
}

type gmailCredentialCipher interface {
	Enabled() bool
	Seal(string) (string, error)
	Open(string) (string, error)
}

// GmailAuthorizationService connects an administrator-approved mailbox to its branch.
type GmailAuthorizationService struct {
	db              gmailAuthorizationDB
	store           gmailAuthorizationStore
	channels        emailInboundChannels
	users           gmailAuthorizationUsers
	provider        gmailAuthorizationProvider
	cipher          gmailCredentialCipher
	stateTTL        time.Duration
	requireVerified bool
	now             func() time.Time
}

// NewGmailAuthorizationService builds an OAuth connection service.
func NewGmailAuthorizationService(db gmailAuthorizationDB, store gmailAuthorizationStore,
	channels emailInboundChannels, users gmailAuthorizationUsers, provider gmailAuthorizationProvider,
	cipher gmailCredentialCipher, stateTTL time.Duration, requireVerified bool,
) *GmailAuthorizationService {
	return &GmailAuthorizationService{db: db, store: store, channels: channels, users: users,
		provider: provider, cipher: cipher, stateTTL: stateTTL, requireVerified: requireVerified, now: time.Now}
}

// Start creates a single-use OAuth state for the selected branch's email channel.
func (s *GmailAuthorizationService) Start(ctx context.Context, tenant domain.Tenant, channelID uuid.UUID) (string, error) {
	if s.provider == nil || s.cipher == nil || !s.cipher.Enabled() || s.stateTTL <= 0 {
		return "", domain.ErrNotConfigured
	}
	if !tenant.IsAdmin() || tenant.UserID == uuid.Nil {
		return "", domain.ErrForbidden
	}
	if err := requireBranch(tenant, "gmail connection"); err != nil {
		return "", err
	}
	state, err := randomOAuthValue()
	if err != nil {
		return "", err
	}
	verifier, err := randomOAuthValue()
	if err != nil {
		return "", err
	}
	sealed, err := s.cipher.Seal(verifier)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256([]byte(state))
	err = s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		channel, err := s.channels.GetActiveByID(ctx, q, tenant.AccountID, tenant.BranchID, channelID)
		if err != nil {
			return err
		}
		if channel.Type != domain.ChannelTypeEmail || channel.Identifier == nil {
			return fmt.Errorf("%w: an email channel with an address is required", domain.ErrInvalidInput)
		}
		user, err := s.users.GetAuthSubjectByID(ctx, q, tenant.AccountID, tenant.UserID)
		if err != nil {
			return err
		}
		if !s.authorizedUser(user, user.SessionEpoch) {
			return domain.ErrForbidden
		}
		return s.store.CreateAuthorization(ctx, q, domain.GmailAuthorization{StateHash: hash[:],
			AccountID: tenant.AccountID, BranchID: tenant.BranchID, ChannelID: channelID,
			UserID: tenant.UserID, SessionEpoch: user.SessionEpoch, VerifierCiphertext: sealed,
			ExpiresAt: s.now().Add(s.stateTTL)})
	})
	if err != nil {
		return "", err
	}
	return s.provider.AuthorizationURL(state, verifier), nil
}

// Complete consumes the state and connects only the mailbox named by the selected channel.
func (s *GmailAuthorizationService) Complete(ctx context.Context, state, code string) error {
	if s.provider == nil || s.cipher == nil || !s.cipher.Enabled() {
		return domain.ErrNotConfigured
	}
	if len(state) < 32 || len(state) > 256 || strings.TrimSpace(code) == "" || len(code) > 4096 {
		return domain.ErrInvalidInput
	}
	hash := sha256.Sum256([]byte(state))
	a, err := s.store.ConsumeAuthorization(ctx, s.db.CrossAccount(), hash[:], s.now())
	if err != nil {
		return err
	}
	verifier, err := s.cipher.Open(a.VerifierCiphertext)
	if err != nil {
		return err
	}
	connectedAt := s.now().UTC()
	grant, err := s.provider.Exchange(ctx, code, verifier)
	if err != nil {
		return err
	}
	address, err := domain.NormalizeInboundChannelIdentifier(domain.ChannelTypeEmail, grant.Address)
	if err != nil || strings.TrimSpace(grant.HistoryID) == "" || grant.RefreshToken == "" {
		return domain.ErrInvalidInput
	}
	sealed, err := s.cipher.Seal(grant.RefreshToken)
	if err != nil {
		return err
	}
	route := domain.InboundChannelRoute{AccountID: a.AccountID, BranchID: a.BranchID,
		ChannelID: a.ChannelID, ChannelType: domain.ChannelTypeEmail}
	return s.db.WithAdvisoryLock(ctx, "gmail-mailbox:"+route.ChannelID.String(), func() error {
		return s.db.InTenantTx(ctx, route.Tenant(), func(q repository.Querier) error {
			user, err := s.users.GetAuthSubjectByID(ctx, q, a.AccountID, a.UserID)
			if err != nil || !s.authorizedUser(user, a.SessionEpoch) {
				return domain.ErrForbidden
			}
			channel, err := s.channels.GetActiveByIDForShare(ctx, q, a.AccountID, a.BranchID, a.ChannelID)
			if err != nil {
				return err
			}
			if !matchesEmailChannel(channel, address) {
				return fmt.Errorf("%w: authorized mailbox does not match the branch email", domain.ErrInvalidInput)
			}
			return s.store.SaveConnected(ctx, q, domain.GmailMailbox{EmailMailboxConnection: domain.EmailMailboxConnection{Route: route, Address: address, HistoryID: grant.HistoryID,
				ConnectedAt: connectedAt}, RefreshTokenCiphertext: sealed})
		})
	})
}

func (s *GmailAuthorizationService) authorizedUser(user *domain.AuthSubject, epoch int) bool {
	return user != nil && user.IsUsable() && user.Role == domain.UserRoleAdmin &&
		user.SessionEpoch == epoch && (!s.requireVerified || user.EmailVerifiedAt != nil)
}

func randomOAuthValue() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}
