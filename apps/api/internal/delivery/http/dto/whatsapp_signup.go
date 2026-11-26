package dto

// WhatsAppSignupResponse is the safe configuration and receipt status for one channel.
type WhatsAppSignupResponse struct {
	AppID           string `json:"app_id,omitempty"`
	ConfigurationID string `json:"configuration_id,omitempty"`
	GraphVersion    string `json:"graph_version,omitempty"`
	Received        bool   `json:"received"`
}

// ConnectWhatsAppSignupRequest carries the Meta code and selected assets.
type ConnectWhatsAppSignupRequest struct {
	Code              string `json:"code" binding:"required,max=4096"`
	BusinessAccountID string `json:"business_account_id" binding:"required,max=32,numeric"`
	PhoneNumberID     string `json:"phone_number_id" binding:"required,max=32,numeric"`
}
