package domain

import (
	"strings"

	"github.com/google/uuid"
)

// maskVisibleDigits is how many trailing digits of a phone a recovery mask keeps.
const maskVisibleDigits = 4

// RecoveryContact is one verified contact offered for account recovery, shown only masked.
type RecoveryContact struct {
	ID     uuid.UUID
	Kind   ChannelType
	Masked string
	Main   bool
	// Available is false while no code can reach the contact's channel.
	Available   bool
	Preselected bool
}

// MaskContact hides a contact down to what lets its owner recognise it: an email keeps its first
// character and its domain, a phone its last four digits. The mask has a fixed width, so it never
// tells how long the hidden part is.
func MaskContact(kind ChannelType, value string) string {
	if kind == ChannelTypeEmail {
		local, domain, ok := strings.Cut(value, "@")
		if !ok {
			return "•••"
		}
		prefix := ""
		// A local part of one or two characters would be given away by its first one.
		if len([]rune(local)) > 2 {
			prefix = string([]rune(local)[:1])
		}
		return prefix + "•••@" + domain
	}
	digits := strings.TrimPrefix(value, "+")
	if len(digits) <= maskVisibleDigits {
		return "•••"
	}
	// Every customer of an Argentine supplier shares +54 9, so showing it reveals nothing.
	prefix := ""
	if strings.HasPrefix(value, "+549") {
		prefix = "+54 9 "
	}
	return prefix + "••• ••••-" + digits[len(digits)-maskVisibleDigits:]
}
