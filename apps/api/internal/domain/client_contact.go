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

// argentineNationalNumber is a ten-digit Argentine number: area 11, 2xx or 3xx plus the subscriber.
var argentineNationalNumber = regexp.MustCompile(`^(11[0-9]{8}|[23][0-9]{9})$`)

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
	ID       uuid.UUID
	Kind     ChannelType
	Value    string
	Primary  bool
	Verified bool
	// Main marks the profile's one contact account recovery preselects.
	Main bool
}

// ClientContactProfile is the authenticated customer's unique fiscal profile.
type ClientContactProfile struct {
	// OwnershipUnproved hides an earlier profile's data until a contact it already had is verified.
	OwnershipUnproved bool
	ClientID          *uuid.UUID
	TaxID             string
	Name              string
	Contacts          []ClientContact
	LegalName         *string
	IVACondition      *IVACondition
	Address           *string
}

// CustomerProfileAccess is what a customer identity may see of the profile carrying its CUIT.
type CustomerProfileAccess struct {
	// Proved is true for a profile born with the identity, or once it verified a contact the profile
	// already had: the same proof that opens the profile's earlier orders.
	Proved bool
	// Name is the name the customer entered, shown in place of an unproved profile's own.
	Name *string
}

// ClientContactUpdate replaces the customer's commercial contact collection.
type ClientContactUpdate struct {
	Name         string
	Contacts     []ClientContact
	LegalName    *string
	IVACondition *IVACondition
	Address      *string
}

// ContactObservation preserves inbound declarations and a seller's association decision.
type ContactObservation struct {
	ID                uuid.UUID
	BranchID          uuid.UUID
	SourceClientID    *uuid.UUID
	CandidateClientID *uuid.UUID
	CandidateName     *string
	SourceName        *string
	// SourceRegistered and CandidateRegistered report a customer account on that profile, which no
	// seller merges.
	SourceRegistered    bool
	CandidateRegistered bool
	TaxID               *string
	Kind                ChannelType
	Value               string
	Name                *string
	Original            string
	Status              ContactReviewStatus
	CreatedAt           time.Time
}

// MergeProfile is what a seller merge checks on one profile before moving anything.
type MergeProfile struct {
	ID    uuid.UUID
	TaxID *string
	// Registered is true when a customer account holds the profile's CUIT.
	Registered bool
	MergedInto *uuid.UUID
	// InReach is true when every order of the profile is in the caller's branches and, for a seller,
	// no quote is another seller's.
	InReach bool
}

// CanonicalPhone strips a phone's separators and writes an Argentine number in the +549 form WhatsApp
// reports, so +54 11… and +54 9 11… are one contact. Any other number keeps its digits.
func CanonicalPhone(phone string) string {
	phone = strings.NewReplacer(" ", "", "-", "", "(", "", ")", "", ".", "").Replace(strings.TrimSpace(phone))
	if national, ok := strings.CutPrefix(phone, "+54"); ok && argentineNationalNumber.MatchString(national) {
		return "+549" + national
	}
	return phone
}

// NormalizeClientContact validates one destination without inferring ownership.
func NormalizeClientContact(kind ChannelType, value string) (string, error) {
	value = strings.TrimSpace(value)
	if strings.ContainsFunc(value, unicode.IsControl) {
		return "", ErrInvalidInput
	}
	switch kind {
	case ChannelTypeWhatsApp:
		value = CanonicalPhone(value)
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
