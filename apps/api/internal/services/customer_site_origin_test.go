package services

import (
	"testing"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func TestCustomerSiteOriginKeepsLocalSuppliersSeparated(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ host, local, want string }{
		{"localhost", "http://localhost:3001", "http://localhost:3001"},
		{"localhost", "http://127.0.0.1:3118", "http://localhost:3118"},
		{"sanmartin.localhost", "http://localhost:3001", "http://localhost:3001/c/sanmartin"},
		{"otro.localhost", "http://localhost:3001", "http://localhost:3001/c/otro"},
		{"otro.localhost", "http://127.0.0.1:3118", "http://otro.localhost:3118"},
		{"supplier.example.com", "", "https://supplier.example.com"},
		{"supplier.example.com", "http://localhost:3001/", "https://supplier.example.com"},
		{"marko.coti-portal-8zcrm.ondigitalocean.app", "https://coti-portal-8zcrm.ondigitalocean.app", "https://coti-portal-8zcrm.ondigitalocean.app/c/marko"},
		{"localhost", "", "https://localhost"},
		{"supplier.localhost.example.com", "http://localhost:3001", "https://supplier.localhost.example.com"},
	} {
		if got := customerSiteOrigin(tc.host, tc.local); got != tc.want {
			t.Errorf("origin(%q,%q)=%q, want %q", tc.host, tc.local, got, tc.want)
		}
	}
}

func TestCustomerGoogleUsesOneSharedProductionCallbackForPathPortals(t *testing.T) {
	t.Parallel()
	service := CustomerAuthService{cfg: config.CustomerAuthConfig{PortalLocalOrigin: "https://portal.test"}}
	for _, label := range []string{"marko", "otro"} {
		site := domain.CustomerSite{Hostname: label + ".portal.test"}
		if got := service.customerGoogleRedirectURI(site); got != "https://portal.test/api/customer/google/callback" {
			t.Fatal(got)
		}
		if got := service.customerOrigin(site); got != "https://portal.test/c/"+label {
			t.Fatal(got)
		}
	}
}

func TestCustomerGoogleUsesSharedLocalCallbackAndSupplierLinks(t *testing.T) {
	t.Parallel()
	service := CustomerAuthService{cfg: config.CustomerAuthConfig{GoogleLocalOrigin: "http://localhost:3001"}}
	site := domain.CustomerSite{Hostname: "sanmartin.localhost"}
	if got := service.customerGoogleRedirectURI(site); got != "http://localhost:3001/api/customer/google/callback" {
		t.Fatal(got)
	}
	if got := service.customerOrigin(site); got != "http://localhost:3001/c/sanmartin" {
		t.Fatal(got)
	}
}
