package domain

import (
	"context"
	"regexp"
	"time"

	"github.com/google/uuid"
)

// ContactChallengePurpose says what redeeming a contact proof does.
type ContactChallengePurpose string

// Contact proof purposes: verifying a contact, or recovering the account through one already verified.
const (
	ContactChallengeVerify  ContactChallengePurpose = "VERIFY"
	ContactChallengeRecover ContactChallengePurpose = "RECOVER"
)

// ContactChallenge binds a one-use contact proof to a customer and supplier.
type ContactChallenge struct {
	ID         uuid.UUID
	AccountID  uuid.UUID
	CustomerID uuid.UUID
	ClientID   uuid.UUID
	Purpose    ContactChallengePurpose
	Kind       ChannelType
	Value      string
	TokenHash  string
	Attempts   int
	ExpiresAt  time.Time
	ConsumedAt *time.Time
	// NextPath is the portal page a verification returns to, empty for the default.
	NextPath string
}

// DestinationWindows are the instants each destination cap counts from.
type DestinationWindows struct {
	Challenges   time.Time
	Failures     time.Time
	AllSuppliers time.Time
}

// DestinationActivity is what the destination caps decide on: one supplier's proofs and wrong proofs
// for a destination, and every supplier's proofs for it.
type DestinationActivity struct {
	SupplierChallenges int
	SupplierFailures   int
	AllChallenges      int
}

// ContactPurpose names why a deterministic message goes to a contact; email subjects follow it.
type ContactPurpose string

// Contact message purposes.
const (
	ContactPurposeNameRequest      ContactPurpose = "NAME_REQUEST"
	ContactPurposeVerification     ContactPurpose = "VERIFICATION"
	ContactPurposeRegistrationLink ContactPurpose = "REGISTRATION_LINK"
	ContactPurposeRecovery         ContactPurpose = "RECOVERY"
)

// ContactMessage is a deterministic onboarding or verification message.
type ContactMessage struct {
	AccountID uuid.UUID
	BranchID  uuid.UUID
	ChannelID uuid.UUID
	Purpose   ContactPurpose
	Kind      ChannelType
	To        string
	Body      string
	Code      string
	// Link is the URL an email carries as its button, blank when the message has none.
	Link string
	// InReplyTo is the RFC 5322 Message-ID an email reply threads under, blank otherwise.
	InReplyTo string
}

// ContactReplyContext describes the inbound message an automatic name prompt would answer.
type ContactReplyContext struct {
	// Automated marks bulk, list, auto-submitted or no-reply mail, which never gets a prompt.
	Automated bool
	MessageID string
}

// ContactMailbox is the supplier identity a branch mailbox answers a contact with.
type ContactMailbox struct {
	Address      string
	SupplierName string
	BranchName   string
}

// ContactMessenger delivers deterministic contact messages through configured channels.
type ContactMessenger interface {
	SendContact(context.Context, ContactMessage) (string, error)
}

// ContactWhatsAppMessage carries one channel's credentials for a deterministic customer message.
type ContactWhatsAppMessage struct {
	To          string
	Body        string
	Code        string
	Template    string
	Language    string
	Credentials WhatsAppDeliveryCredentials
}

// ContactWhatsAppSender supports onboarding text and approved verification templates.
type ContactWhatsAppSender interface {
	SendContact(context.Context, ContactWhatsAppMessage) (*DeliveryReceipt, error)
}

// ContactChallengeStatus is where a verification stands for the customer who asked for it.
type ContactChallengeStatus string

// Verification states as the portal shows them.
const (
	ContactChallengePending  ContactChallengeStatus = "PENDING"
	ContactChallengeVerified ContactChallengeStatus = "VERIFIED"
	ContactChallengeExpired  ContactChallengeStatus = "EXPIRED"
)

// ContactChallengeView is what a customer may read of their own verification: never its secret.
type ContactChallengeView struct {
	Kind     ChannelType
	Value    string
	Status   ContactChallengeStatus
	NextPath string
}

var customerNextPathPattern = regexp.MustCompile(`^/(orders|profile|new-request|(orders|quotes|rfq)/[A-Za-z0-9_-]{1,128})$`)

// CustomerNextPath keeps a destination only when it is a portal page a customer returns to, and
// returns "" otherwise, so a stored path can never send anyone off the supplier's portal.
func CustomerNextPath(raw string) string {
	if customerNextPathPattern.MatchString(raw) {
		return raw
	}
	return ""
}
