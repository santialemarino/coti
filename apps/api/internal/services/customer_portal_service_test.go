package services

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type portalFake struct {
	accountID uuid.UUID
	hostname  string
	collision bool
}

func (f *portalFake) GetHostname(_ context.Context, _ repository.Querier, accountID uuid.UUID) (string, error) {
	if accountID != f.accountID {
		return "", domain.ErrNotFound
	}
	return f.hostname, nil
}
func (f *portalFake) IsAvailableCrossAccount(_ context.Context, _ repository.Querier, _ uuid.UUID, _ string) (bool, error) {
	return !f.collision, nil
}
func (f *portalFake) UpdateHostname(_ context.Context, _ repository.Querier, accountID uuid.UUID, expected, hostname string) error {
	if f.collision || accountID != f.accountID || expected != f.hostname {
		return domain.ErrConflict
	}
	f.hostname = hostname
	return nil
}

func TestCustomerPortalService_ValidationIsolationAndConfirmation(t *testing.T) {
	ctx := context.Background()
	tenant := domain.Tenant{AccountID: uuid.New(), Role: domain.UserRoleAdmin}
	f := &portalFake{accountID: tenant.AccountID}
	s := NewCustomerPortalService(&fakeDB{}, f, config.CustomerPortalConfig{BaseURL: "http://localhost:3001"})
	if _, err := s.Update(ctx, tenant, domain.CustomerPortalUpdate{Subdomain: "API"}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatal(err)
	}
	p, err := s.Update(ctx, tenant, domain.CustomerPortalUpdate{Subdomain: " Marko "})
	if err != nil || p.URL != "http://marko.localhost:3001" {
		t.Fatalf("%+v %v", p, err)
	}
	in := domain.CustomerPortalUpdate{Subdomain: "otro", ExpectedHostname: "marko.localhost"}
	if _, err = s.Update(ctx, tenant, in); !errors.Is(err, domain.ErrConflict) {
		t.Fatal(err)
	}
	in.ConfirmChange = true
	f.collision = true
	if _, err = s.Update(ctx, tenant, in); !errors.Is(err, domain.ErrConflict) {
		t.Fatal(err)
	}
	f.collision = false
	if _, err = s.Update(ctx, tenant, in); err != nil {
		t.Fatal(err)
	}
	other := tenant
	other.AccountID = uuid.New()
	if _, err = s.Get(ctx, other); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal(err)
	}
	seller := tenant
	seller.Role = domain.UserRoleSeller
	if _, err = s.Update(ctx, seller, in); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal(err)
	}
	if _, err = s.Available(ctx, seller, "free"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal(err)
	}
	f.hostname = "temporary.ondigitalocean.app"
	p, err = s.Get(ctx, tenant)
	if err != nil || !p.Legacy || p.URL != "https://temporary.ondigitalocean.app" {
		t.Fatalf("%+v %v", p, err)
	}
	in.ExpectedHostname = f.hostname
	if _, err = s.Update(ctx, tenant, in); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal(err)
	}
	s.baseURL = ""
	if _, err = s.Update(ctx, tenant, in); !errors.Is(err, domain.ErrNotConfigured) {
		t.Fatal(err)
	}
}
