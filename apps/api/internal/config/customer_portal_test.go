package config

import "testing"

func TestCustomerPortalConfiguration(t *testing.T) {
	t.Setenv("WEB_WEBAPP_URL", "")
	for _, tc := range []struct {
		env       Environment
		raw, want string
		invalid   bool
	}{
		{EnvironmentDevelopment, "", "http://localhost:3001", false},
		{EnvironmentProduction, "", "", false},
		{EnvironmentDevelopment, "http://localhost:3110/", "http://localhost:3110", false},
		{EnvironmentProduction, "https://portal.example.com", "https://portal.example.com", false},
		{EnvironmentProduction, "http://localhost:3001", "", true},
		{EnvironmentProduction, "http://example.com", "", true},
		{EnvironmentDevelopment, "https://example.com/path", "", true},
		{EnvironmentDevelopment, "https://user@example.com", "", true},
	} {
		t.Setenv("CUSTOMER_PORTAL_BASE_URL", tc.raw)
		var problems []string
		got := loadCustomerPortal(tc.env, &problems)
		if (len(problems) > 0) != tc.invalid || (!tc.invalid && got.BaseURL != tc.want) {
			t.Errorf("%s %q: %+v, %v", tc.env, tc.raw, got, problems)
		}
	}
}

func TestCustomerPortalProductionUsesSharedWebappOrigin(t *testing.T) {
	t.Setenv("CUSTOMER_PORTAL_BASE_URL", "")
	t.Setenv("WEB_WEBAPP_URL", "https://portal.ondigitalocean.app")
	var problems []string
	got := loadCustomerPortal(EnvironmentProduction, &problems)
	if len(problems) != 0 || got.BaseURL != "https://portal.ondigitalocean.app" {
		t.Fatalf("%+v %v", got, problems)
	}
}
