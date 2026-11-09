package services

import (
	"context"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type portalRepository interface {
	GetHostname(context.Context, repository.Querier, uuid.UUID) (string, error)
	IsAvailableCrossAccount(context.Context, repository.Querier, uuid.UUID, string) (bool, error)
	UpdateHostname(context.Context, repository.Querier, uuid.UUID, string, string) error
}

type portalDB interface {
	tenantScoper
	CrossAccount() repository.Querier
}

// CustomerPortalService manages public routing names without exposing other accounts.
type CustomerPortalService struct {
	db      portalDB
	repo    portalRepository
	baseURL string
}

// NewCustomerPortalService builds a CustomerPortalService.
func NewCustomerPortalService(db portalDB, repo portalRepository, cfg config.CustomerPortalConfig) *CustomerPortalService {
	return &CustomerPortalService{db: db, repo: repo, baseURL: cfg.BaseURL}
}

// Get reads the authenticated account's portal configuration.
func (s *CustomerPortalService) Get(ctx context.Context, tenant domain.Tenant) (*domain.CustomerPortalSettings, error) {
	if !tenant.IsAdmin() {
		return nil, domain.ErrForbidden
	}
	var hostname string
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var err error
		hostname, err = s.repo.GetHostname(ctx, q, tenant.AccountID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return s.settings(hostname), nil
}

// Available checks a normalized name without reserving it or disclosing its owner.
func (s *CustomerPortalService) Available(ctx context.Context, tenant domain.Tenant, raw string) (bool, error) {
	if !tenant.IsAdmin() {
		return false, domain.ErrForbidden
	}
	hostname, err := s.hostname(raw)
	if err != nil {
		return false, err
	}
	return s.repo.IsAvailableCrossAccount(ctx, s.db.CrossAccount(), tenant.AccountID, hostname)
}

// Update changes only the account's hostname after explicit acknowledgement of existing access changes.
func (s *CustomerPortalService) Update(ctx context.Context, tenant domain.Tenant, in domain.CustomerPortalUpdate) (*domain.CustomerPortalSettings, error) {
	if !tenant.IsAdmin() {
		return nil, domain.ErrForbidden
	}
	hostname, err := s.hostname(in.Subdomain)
	if err != nil {
		return nil, err
	}
	err = s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		current, err := s.repo.GetHostname(ctx, q, tenant.AccountID)
		if err != nil {
			return err
		}
		if current != in.ExpectedHostname {
			return domain.ErrConflict
		}
		if current == hostname {
			return nil
		}
		if s.settings(current).Legacy {
			return domain.ErrForbidden
		}
		if current != "" && !in.ConfirmChange {
			return domain.ErrConflict
		}
		return s.repo.UpdateHostname(ctx, q, tenant.AccountID, current, hostname)
	})
	if err != nil {
		return nil, err
	}
	return s.settings(hostname), nil
}

func (s *CustomerPortalService) hostname(raw string) (string, error) {
	if s.baseURL == "" {
		return "", domain.ErrNotConfigured
	}
	label, err := domain.NormalizeSubdomain(raw)
	if err != nil {
		return "", err
	}
	base, err := url.Parse(s.baseURL)
	if err != nil || base.Hostname() == "" {
		return "", domain.ErrNotConfigured
	}
	hostname := label + "." + base.Hostname()
	if len(hostname) > 253 {
		return "", domain.ErrInvalidInput
	}
	return hostname, nil
}

func (s *CustomerPortalService) settings(hostname string) *domain.CustomerPortalSettings {
	settings := &domain.CustomerPortalSettings{BaseURL: s.baseURL, Hostname: hostname}
	if hostname == "" {
		return settings
	}
	settings.URL = "https://" + hostname
	base, err := url.Parse(s.baseURL)
	if err == nil && base.Hostname() != "" {
		if hostname == base.Hostname() {
			settings.URL = base.String()
			return settings
		}
		label := strings.TrimSuffix(hostname, "."+base.Hostname())
		if normalized, err := domain.NormalizeSubdomain(label); err == nil && normalized+"."+base.Hostname() == hostname {
			settings.Subdomain = normalized
			base.Path = "/c/" + normalized
			settings.URL = base.String()
			return settings
		}
	}
	settings.Legacy = true
	return settings
}
