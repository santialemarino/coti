package domain

import (
	"fmt"
	"regexp"
	"strings"
)

var subdomainPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
var reservedSubdomains = map[string]bool{
	"admin": true, "api": true, "app": true, "auth": true, "backoffice": true,
	"cdn": true, "coti": true, "dev": true, "docs": true, "localhost": true,
	"mail": true, "portal": true, "public": true, "smtp": true, "staging": true,
	"static": true, "status": true, "support": true, "www": true,
}

// NormalizeSubdomain validates one DNS label without accepting a URL or hostname.
func NormalizeSubdomain(raw string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if !subdomainPattern.MatchString(value) || reservedSubdomains[value] {
		return "", fmt.Errorf("%w: invalid or reserved subdomain", ErrInvalidInput)
	}
	return value, nil
}

// CustomerPortalSettings describes the account's current portal and provisioning boundary.
type CustomerPortalSettings struct {
	BaseURL   string
	Hostname  string
	URL       string
	Subdomain string
	Legacy    bool
}

// CustomerPortalUpdate confirms a hostname change against the version the administrator saw.
type CustomerPortalUpdate struct {
	Subdomain        string
	ExpectedHostname string
	ConfirmChange    bool
}
