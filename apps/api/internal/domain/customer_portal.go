package domain

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/google/uuid"
	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
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

const portalLabelMaxLength = 63

var portalLabelSeparators = regexp.MustCompile(`[^a-z0-9]+`)

// DefaultPortalLabel derives the portal label an account starts with from its name. A name that
// leaves nothing usable, or only a reserved word, falls back to corralon-<the id's first 6 chars>.
func DefaultPortalLabel(name string, accountID uuid.UUID) string {
	// Built per call: a chained transformer keeps state and is not safe to share across goroutines.
	foldAccents := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	folded, _, err := transform.String(foldAccents, strings.ToLower(name))
	if err != nil {
		folded = strings.ToLower(name)
	}
	label := strings.Trim(portalLabelSeparators.ReplaceAllString(folded, "-"), "-")
	if len(label) > portalLabelMaxLength {
		label = strings.TrimRight(label[:portalLabelMaxLength], "-")
	}
	if _, err := NormalizeSubdomain(label); err != nil {
		return "corralon-" + accountID.String()[:6]
	}
	return label
}

// PortalLabelCandidates lists the labels an account may take in order of preference: the label,
// then label-2, label-3 and so on, each still a valid DNS label.
func PortalLabelCandidates(label string, count int) []string {
	candidates := make([]string, 0, count)
	for n := 1; n <= count; n++ {
		if n == 1 {
			candidates = append(candidates, label)
			continue
		}
		suffix := "-" + strconv.Itoa(n)
		base := label
		if len(base)+len(suffix) > portalLabelMaxLength {
			base = strings.TrimRight(base[:portalLabelMaxLength-len(suffix)], "-")
		}
		candidates = append(candidates, base+suffix)
	}
	return candidates
}

// CustomerPortalSettings describes the account's current portal and provisioning boundary.
type CustomerPortalSettings struct {
	BaseURL   string
	Hostname  string
	URL       string
	Subdomain string
	Legacy    bool
}

// PortalAssignment is a default portal given to an account, over the hostname it held before.
type PortalAssignment struct {
	AccountID   uuid.UUID
	AccountName string
	Previous    string // blank when the account had none.
	Hostname    string
}

// CustomerPortalUpdate confirms a hostname change against the version the administrator saw.
type CustomerPortalUpdate struct {
	Subdomain        string
	ExpectedHostname string
	ConfirmChange    bool
}
