package dto

// CustomerPortalResponse describes the current portal and configured base origin.
type CustomerPortalResponse struct {
	BaseURL   string `json:"base_url"`
	Hostname  string `json:"hostname"`
	URL       string `json:"url"`
	Subdomain string `json:"subdomain"`
	Legacy    bool   `json:"legacy"`
}

// CustomerPortalAvailabilityResponse reports a public name's advisory availability.
type CustomerPortalAvailabilityResponse struct {
	Available bool `json:"available"`
}

// UpdateCustomerPortalRequest is the body for PUT /v1/account/portal.
type UpdateCustomerPortalRequest struct {
	Subdomain        string `json:"subdomain" binding:"required,max=255"`
	ExpectedHostname string `json:"expected_hostname" binding:"max=253"`
	ConfirmChange    bool   `json:"confirm_change"`
}
