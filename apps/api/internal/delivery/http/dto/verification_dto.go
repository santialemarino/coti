package dto

// ConfirmEmailRequest is the body for POST /v1/public/auth/verify-email.
type ConfirmEmailRequest struct {
	Token string `json:"token" binding:"required"`
}

// ResendVerificationRequest is the body for POST /v1/public/auth/resend-verification.
type ResendVerificationRequest struct {
	Email string `json:"email" binding:"required,email,max=255"`
}

// ChangeEmailRequest is the body for POST /v1/auth/change-email. The current password is the
// proof of identity; the user comes from the tenant context, never the body.
type ChangeEmailRequest struct {
	NewEmail        string `json:"new_email" binding:"required,email,max=255"`
	CurrentPassword string `json:"current_password" binding:"required,max=128"`
}
