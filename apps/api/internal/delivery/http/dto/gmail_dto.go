package dto

// GmailAuthorizationResponse carries the Google consent URL for a branch administrator.
type GmailAuthorizationResponse struct {
	AuthorizationURL string `json:"authorization_url"`
}
