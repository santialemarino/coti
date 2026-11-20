package services

import (
	"context"
	"fmt"
	"net/url"

	"github.com/google/uuid"
	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// portalCandidateLimit is how many suffixed labels an account tries before allocation gives up.
const portalCandidateLimit = 100

// maxHostnameLength is the DNS limit the account.public_hostname column carries.
const maxHostnameLength = 253

type defaultPortalRepository interface {
	ListTakenCrossAccount(context.Context, repository.Querier, []string) (map[string]bool, error)
	ListWithoutPortalCrossAccount(context.Context, repository.Querier, string) ([]domain.PortalAssignment, error)
	UpdateHostname(context.Context, repository.Querier, uuid.UUID, string, string) error
	AssignHostnamesCrossAccount(context.Context, repository.Querier, []domain.PortalAssignment) error
	LockHostnames(context.Context, repository.Querier) error
}

// DefaultPortalService gives every account a /c/<label> portal derived from its name, so no
// customer link ever points at the bare shared host.
type DefaultPortalService struct {
	db           adminTxScoper
	repo         defaultPortalRepository
	baseHostname string
}

// NewDefaultPortalService builds a DefaultPortalService over the configured portal base URL.
func NewDefaultPortalService(db adminTxScoper, repo defaultPortalRepository, cfg config.CustomerPortalConfig) *DefaultPortalService {
	s := &DefaultPortalService{db: db, repo: repo}
	if base, err := url.Parse(cfg.BaseURL); err == nil {
		s.baseHostname = base.Hostname()
	}
	return s
}

// AssignDefault gives a new account its default portal inside the caller's registration
// transaction. Without a configured base it leaves the account without one, as before.
func (s *DefaultPortalService) AssignDefault(ctx context.Context, q repository.Querier, accountID uuid.UUID, name string) (string, error) {
	if s.baseHostname == "" {
		return "", nil
	}
	if err := s.repo.LockHostnames(ctx, q); err != nil {
		return "", err
	}
	candidates := s.candidates(name, accountID)
	taken, err := s.repo.ListTakenCrossAccount(ctx, q, candidates)
	if err != nil {
		return "", err
	}
	hostname, err := firstFreeHostname(candidates, taken)
	if err != nil {
		return "", err
	}
	return hostname, s.repo.UpdateHostname(ctx, q, accountID, "", hostname)
}

// Backfill plans a default portal for every account without one of its own, and writes the plan
// when apply is set. Returns: the assignments, written or not.
func (s *DefaultPortalService) Backfill(ctx context.Context, apply bool) ([]domain.PortalAssignment, error) {
	if s.baseHostname == "" {
		return nil, domain.ErrNotConfigured
	}
	tx, err := s.db.AdminTx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := s.repo.LockHostnames(ctx, tx); err != nil {
		return nil, err
	}
	accounts, err := s.repo.ListWithoutPortalCrossAccount(ctx, tx, s.baseHostname)
	if err != nil || len(accounts) == 0 {
		return accounts, err
	}
	candidates := make([][]string, len(accounts))
	all := make([]string, 0, len(accounts)*portalCandidateLimit)
	for i, a := range accounts {
		candidates[i] = s.candidates(a.AccountName, a.AccountID)
		all = append(all, candidates[i]...)
	}
	taken, err := s.repo.ListTakenCrossAccount(ctx, tx, all)
	if err != nil {
		return nil, err
	}
	for i := range accounts {
		hostname, err := firstFreeHostname(candidates[i], taken)
		if err != nil {
			return nil, fmt.Errorf("account %s: %w", accounts[i].AccountID, err)
		}
		accounts[i].Hostname = hostname
		taken[hostname] = true
	}
	if !apply {
		return accounts, nil
	}
	if err := s.repo.AssignHostnamesCrossAccount(ctx, tx, accounts); err != nil {
		return nil, err
	}
	return accounts, tx.Commit(ctx)
}

func (s *DefaultPortalService) candidates(name string, accountID uuid.UUID) []string {
	labels := domain.PortalLabelCandidates(domain.DefaultPortalLabel(name, accountID), portalCandidateLimit)
	hostnames := make([]string, 0, len(labels))
	for _, label := range labels {
		if hostname := label + "." + s.baseHostname; len(hostname) <= maxHostnameLength {
			hostnames = append(hostnames, hostname)
		}
	}
	return hostnames
}

func firstFreeHostname(candidates []string, taken map[string]bool) (string, error) {
	for _, hostname := range candidates {
		if !taken[hostname] {
			return hostname, nil
		}
	}
	return "", fmt.Errorf("%w: every default portal name is taken", domain.ErrConflict)
}
