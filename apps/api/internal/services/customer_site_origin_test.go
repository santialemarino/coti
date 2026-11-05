package services

import (
	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"testing"
)

func TestCustomerSiteOriginKeepsLocalSuppliersSeparated(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ host, local, want string }{
		{"sanmartin.localhost", "http://localhost:3001", "http://sanmartin.localhost:3001"},
		{"otro.localhost", "http://localhost:3001", "http://otro.localhost:3001"},
		{"otro.localhost", "http://127.0.0.1:3118", "http://otro.localhost:3118"},
		{"supplier.example.com", "", "https://supplier.example.com"},
		{"supplier.example.com", "http://localhost:3001/", "http://localhost:3001"},
	} {
		if got := customerSiteOrigin(tc.host, tc.local); got != tc.want {
			t.Errorf("origin(%q,%q)=%q, want %q", tc.host, tc.local, got, tc.want)
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
	if got := service.customerOrigin(site); got != "http://sanmartin.localhost:3001" {
		t.Fatal(got)
	}
}
