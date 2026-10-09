package domain

import (
	"fmt"
	"net/mail"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
)

// ChannelType is how an RFQ reached the branch.
type ChannelType string

const (
	ChannelTypeWhatsApp    ChannelType = "WHATSAPP"
	ChannelTypeEmail       ChannelType = "EMAIL"
	ChannelTypeWebApp      ChannelType = "WEBAPP"
	ChannelTypeManualEntry ChannelType = "MANUAL_ENTRY"
)

// channelTypes are the intake routes a branch may open, in the order a listing reads best.
var channelTypes = []ChannelType{
	ChannelTypeWhatsApp, ChannelTypeEmail, ChannelTypeWebApp, ChannelTypeManualEntry,
}

// Channel is an intake route configured for one branch.
type Channel struct {
	ID         uuid.UUID
	AccountID  uuid.UUID
	BranchID   uuid.UUID
	Type       ChannelType
	IsActive   bool
	Identifier *string
	// IsConfigured is whether channel.config holds anything. The config itself never reaches a
	// Channel: it carries provider credentials, and a struct that cannot hold one cannot leak
	// one into a response or a log line.
	IsConfigured bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// ChannelConfiguration is a stored channel configuration available only to backend adapters.
type ChannelConfiguration struct {
	ChannelID uuid.UUID
	AccountID uuid.UUID
	BranchID  uuid.UUID
	Type      ChannelType
	Config    []byte
}

// InboundChannelRoute identifies the tenant and branch an external message must enter through.
type InboundChannelRoute struct {
	AccountID   uuid.UUID
	BranchID    uuid.UUID
	ChannelID   uuid.UUID
	ChannelType ChannelType
}

// Tenant returns the database scope for the resolved external route.
func (r InboundChannelRoute) Tenant() Tenant {
	return Tenant{AccountID: r.AccountID, BranchID: r.BranchID}
}

// Validate refuses incomplete routes and channel types without an external inbox.
func (r InboundChannelRoute) Validate() error {
	if r.AccountID == uuid.Nil || r.BranchID == uuid.Nil || r.ChannelID == uuid.Nil {
		return fmt.Errorf("%w: an inbound route needs account, branch, and channel IDs", ErrInvalidInput)
	}
	if r.ChannelType != ChannelTypeWhatsApp && r.ChannelType != ChannelTypeEmail {
		return fmt.Errorf("%w: %s has no external inbox route", ErrInvalidInput, r.ChannelType)
	}
	return nil
}

// NewChannel is an intake route an administrator opens on the selected branch. Config is stored
// verbatim, so the service validates it and seals its credentials before the repository sees it.
type NewChannel struct {
	Type       ChannelType
	Identifier *string
	Config     []byte
}

// ChannelUpdate is the editable surface of a channel; the type is not, because the shape of the
// configuration depends on it. Config nil leaves the stored settings alone and ClearConfig removes
// them — an absent config cannot mean "delete it" when no response ever returned one.
type ChannelUpdate struct {
	Identifier  *string
	IsActive    *bool
	Config      []byte
	ClearConfig bool
}

// IsValidChannelType reports whether t is one of the four intake routes.
func IsValidChannelType(t ChannelType) bool {
	return slices.Contains(channelTypes, t)
}

// NormalizeChannelIdentifier turns a blank identifier into an absent one. An empty string is not
// NULL, so it would slip past the partial unique index the one-per-branch types rest on.
func NormalizeChannelIdentifier(identifier *string) *string {
	if identifier == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*identifier)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

// NormalizeInboundChannelIdentifier canonicalizes the provider identifier used before a tenant is known.
func NormalizeInboundChannelIdentifier(channelType ChannelType, identifier string) (string, error) {
	normalized := strings.TrimSpace(identifier)
	if normalized == "" {
		return "", fmt.Errorf("%w: an inbound channel identifier is required", ErrInvalidInput)
	}
	if strings.ContainsFunc(normalized, unicode.IsControl) {
		return "", fmt.Errorf("%w: an inbound channel identifier carries no control characters", ErrInvalidInput)
	}
	if len([]byte(normalized)) > maxChannelFieldLength {
		return "", fmt.Errorf("%w: an inbound channel identifier must be at most %d bytes",
			ErrInvalidInput, maxChannelFieldLength)
	}
	switch channelType {
	case ChannelTypeWhatsApp:
		return normalized, nil
	case ChannelTypeEmail:
		parsed, err := mail.ParseAddress(normalized)
		if err != nil || parsed.Address != normalized {
			return "", fmt.Errorf("%w: an inbound EMAIL identifier must be a bare email address", ErrInvalidInput)
		}
		return strings.ToLower(parsed.Address), nil
	default:
		return "", fmt.Errorf("%w: %s has no external inbox route", ErrInvalidInput, channelType)
	}
}

// ValidateChannelIdentifier holds both rules the identifier column carries: WEBAPP and MANUAL_ENTRY
// are one per branch, so the partial unique index behind them needs the column NULL; and a
// configured channel needs one, because credentials belong to the number or the mailbox it names.
func ValidateChannelIdentifier(channelType ChannelType, identifier *string, configured bool) error {
	switch channelType {
	case ChannelTypeWebApp, ChannelTypeManualEntry:
		if identifier != nil {
			return WithCode(CodeChannelIdentifier, fmt.Errorf(
				"%w: a %s channel carries no identifier", ErrInvalidInput, channelType))
		}
	default:
		if configured && identifier == nil {
			return WithCode(CodeChannelIdentifier, fmt.Errorf(
				"%w: a configured %s channel needs its identifier, which is what its credentials "+
					"belong to", ErrInvalidInput, channelType))
		}
	}
	// A control character is never part of a number or a mailbox, and trimming only reaches the
	// ends, so an embedded newline would otherwise be stored as part of the identifier.
	if identifier != nil && strings.ContainsFunc(*identifier, unicode.IsControl) {
		return WithCode(CodeChannelIdentifier, fmt.Errorf(
			"%w: a channel identifier carries no control characters", ErrInvalidInput))
	}
	// A mailbox has one unambiguous format and a malformed one guarantees the connector fails. A
	// phone number does not, so WHATSAPP's identifier is left to the provider to reject. The bare
	// address only: uniqueness is on this column verbatim, so "Pedidos <a@b>" beside "a@b" would
	// be two channels on one mailbox.
	if channelType == ChannelTypeEmail && identifier != nil {
		parsed, err := mail.ParseAddress(*identifier)
		if err != nil || parsed.Address != *identifier {
			return WithCode(CodeChannelIdentifier, fmt.Errorf(
				"%w: an EMAIL channel's identifier is its mailbox, so it must be a bare email "+
					"address", ErrInvalidInput))
		}
	}
	return nil
}
