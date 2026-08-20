package domain

import (
	"fmt"
	"strings"
	"time"

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

// ChannelTypes are the intake routes a branch may open, in the order a listing reads best.
var ChannelTypes = []ChannelType{
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

// NewChannel is an intake route an administrator opens on the selected branch. Config is the JSON
// as it will be stored, credentials already sealed, and nil when the channel is not configured.
type NewChannel struct {
	Type       ChannelType
	Identifier *string
	Config     []byte
}

// ChannelUpdate is the editable surface of a channel; the type is not part of it, because the
// shape of the configuration depends on it. Config nil leaves the stored settings alone and
// ClearConfig removes them: the API never returns a credential, so a form cannot send one back
// and an absent config must not mean "delete it".
type ChannelUpdate struct {
	Identifier  *string
	IsActive    *bool
	Config      []byte
	ClearConfig bool
}

// IsValidChannelType reports whether t is one of the four intake routes.
func IsValidChannelType(t ChannelType) bool {
	for _, known := range ChannelTypes {
		if t == known {
			return true
		}
	}
	return false
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

// ValidateChannelIdentifier refuses an identifier on a type a branch may hold only one of. WEBAPP
// and MANUAL_ENTRY are those two: there is one public link and one manual-entry route per branch,
// and the partial unique index behind them counts on the column staying NULL.
func ValidateChannelIdentifier(channelType ChannelType, identifier *string) error {
	switch channelType {
	case ChannelTypeWebApp, ChannelTypeManualEntry:
		if identifier != nil {
			return WithCode(CodeChannelIdentifier, fmt.Errorf(
				"%w: a %s channel carries no identifier", ErrInvalidInput, channelType))
		}
	}
	return nil
}
