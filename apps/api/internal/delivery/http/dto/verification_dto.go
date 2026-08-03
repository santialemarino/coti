package dto

// ConfirmEmailRequest is the body for POST /v1/public/auth/verify-email.
type ConfirmEmailRequest struct {
	Token string `json:"token" binding:"required"`
}

// ResendVerificationRequest is the body for POST /v1/public/auth/resend-verification.
type ResendVerificationRequest struct {
	Email string `json:"email" binding:"required,email,max=255"`
}
