package domain

import (
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
)

var contactPhonePattern = regexp.MustCompile(`^\+[1-9][0-9]{6,14}$`)

// ContactReviewStatus is the persisted outcome of an inbound commercial declaration.
type ContactReviewStatus string

const (
	ContactReviewAssociated ContactReviewStatus = "ASSOCIATED"
	ContactReviewMerged     ContactReviewStatus = "MERGED"
	ContactReviewPending    ContactReviewStatus = "PENDING"
	ContactReviewRecorded   ContactReviewStatus = "RECORDED"
	ContactReviewSeparate   ContactReviewStatus = "SEPARATE"
)

// ClientContact is one commercial destination, independent of customer credentials.
type ClientContact struct {
	ID      uuid.UUID
	Kind    ChannelType
	Value   string
	Primary bool
}

// ClientContactProfile is the authenticated customer's unique fiscal profile.
type ClientContactProfile struct {
	ReviewPending bool
	ClientID      *uuid.UUID
	TaxID         string
	Name          string
	Contacts      []ClientContact
}

// ClientContactUpdate replaces the customer's commercial contact collection.
type ClientContactUpdate struct {
	Name     string
	Contacts []ClientContact
}

// ContactObservation preserves inbound declarations and a seller's association decision.
type ContactObservation struct {
	ID                uuid.UUID
	BranchID          uuid.UUID
	SourceClientID    *uuid.UUID
	CandidateClientID *uuid.UUID
	CandidateName     *string
	TaxID             *string
	Kind              ChannelType
	Value             string
	Name              *string
	Original          string
	Status            ContactReviewStatus
	CreatedAt         time.Time
}

// NormalizeClientContact validates one destination without inferring ownership.
func NormalizeClientContact(kind ChannelType, value string) (string, error) {
	value = strings.TrimSpace(value)
	if strings.ContainsFunc(value, unicode.IsControl) {
		return "", ErrInvalidInput
	}
	switch kind {
	case ChannelTypeWhatsApp:
		value = strings.NewReplacer(" ", "", "-", "", "(", "", ")", "", ".", "").Replace(value)
		if !contactPhonePattern.MatchString(value) {
			return "", fmt.Errorf("%w: phone must use E.164 format", ErrInvalidInput)
		}
	case ChannelTypeEmail:
		value = NormalizeEmail(value)
		parsed, err := mail.ParseAddress(value)
		if err != nil || parsed.Address != value || len(value) > 255 {
			return "", fmt.Errorf("%w: invalid email", ErrInvalidInput)
		}
	default:
		return "", ErrInvalidInput
	}
	return value, nil
}

// NormalizeClientContacts rejects duplicates and requires one primary per populated type.
func NormalizeClientContacts(contacts []ClientContact) ([]ClientContact, error) {
	result := make([]ClientContact, 0, len(contacts))
	seen := make(map[string]bool, len(contacts))
	counts := make(map[ChannelType]int)
	primary := make(map[ChannelType]int)
	for _, contact := range contacts {
		value, err := NormalizeClientContact(contact.Kind, contact.Value)
		if err != nil {
			return nil, err
		}
		key := string(contact.Kind) + ":" + value
		if seen[key] {
			return nil, fmt.Errorf("%w: duplicate contact", ErrInvalidInput)
		}
		seen[key] = true
		counts[contact.Kind]++
		if contact.Primary {
			primary[contact.Kind]++
		}
		contact.Value = value
		result = append(result, contact)
	}
	for kind := range counts {
		if primary[kind] != 1 {
			return nil, fmt.Errorf("%w: choose one primary contact per type", ErrInvalidInput)
		}
	}
	return result, nil
}
