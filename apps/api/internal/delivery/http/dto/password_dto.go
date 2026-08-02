package dto

// ChangePasswordRequest is the body for POST /v1/auth/change-password. The configured floor
// is applied by the service; the binding bound is only the one the schema can express.
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password" binding:"required,max=128"`
	NewPassword     string `json:"new_password" binding:"required,min=8,max=128"`
}

// ForgotPasswordRequest is the body for POST /v1/public/auth/forgot-password.
type ForgotPasswordRequest struct {
	Email string `json:"email" binding:"required,email,max=255"`
}

// ResetPasswordRequest is the body for POST /v1/public/auth/reset-password.
type ResetPasswordRequest struct {
	Token       string `json:"token" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,min=8,max=128"`
}
