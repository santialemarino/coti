package domain

// WhatsAppSignupGrant holds a business token and phone verified by Meta.
type WhatsAppSignupGrant struct {
	Identifier        string
	AccessToken       string
	NeedsRegistration bool
}
