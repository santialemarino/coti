package config

import (
	"net"
	"net/url"
	"os"
	"regexp"
	"strings"
)

var portalBaseHostname = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]*[a-z0-9])?)*$`)

// CustomerPortalConfig carries the shared origin for supplier portal paths.
type CustomerPortalConfig struct {
	BaseURL string
}

func loadCustomerPortal(env Environment, problems *[]string) CustomerPortalConfig {
	base := os.Getenv("CUSTOMER_PORTAL_BASE_URL")
	if base == "" && env == EnvironmentProduction {
		base = os.Getenv("WEB_WEBAPP_URL")
	}
	if base == "" && env == EnvironmentDevelopment {
		base = "http://localhost:3001"
	}
	base = strings.ToLower(strings.TrimRight(base, "/"))
	if base != "" {
		u, err := url.Parse(base)
		valid := err == nil && portalBaseHostname.MatchString(u.Hostname()) && net.ParseIP(u.Hostname()) == nil && len(u.Hostname()) <= 189 && u.User == nil && u.Path == "" && u.RawQuery == "" && u.Fragment == ""
		if valid {
			valid = (env == EnvironmentDevelopment && u.Scheme == "http" && u.Hostname() == "localhost") ||
				(u.Scheme == "https" && u.Port() == "" && strings.Contains(u.Hostname(), ".") && !strings.HasSuffix(u.Hostname(), ".localhost"))
		}
		if !valid {
			*problems = append(*problems, "CUSTOMER_PORTAL_BASE_URL must be a provisioned HTTPS base origin, or HTTP localhost in development")
		}
	}
	return CustomerPortalConfig{BaseURL: base}
}
