// Package dto holds the wire contract: request and response structs with snake_case
// json tags and binding validation. Mapping to and from domain types happens in the
// handler, never deeper.
package dto

import "time"

// LoginRequest is the body for POST /v1/public/auth/login.
type LoginRequest struct {
	Email      string `json:"email" binding:"required,email,max=255"`
	Password   string `json:"password" binding:"required,min=8,max=128"`
	RememberMe bool   `json:"remember_me"`
}

// RefreshRequest is the body for POST /v1/public/auth/refresh.
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// LogoutRequest is the body for POST /v1/auth/logout. The refresh token is optional:
// without it the session epoch still advances, which invalidates the access token.
type LogoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// TokenResponse is returned by login and refresh.
//
// The refresh token is returned in the body and shown exactly once — the caller stores
// it. The API is bearer-based; the web app is what puts the access token in an httpOnly
// cookie.
type TokenResponse struct {
	AccessToken     string    `json:"access_token"`
	AccessExpiresAt time.Time `json:"access_expires_at"`
	RefreshToken    string    `json:"refresh_token"`
	User            UserBrief `json:"user"`
}

// UserBrief is the caller identity embedded in a token response, so the client does not
// need a second request to render the shell.
type UserBrief struct {
	ID        string `json:"id"`
	AccountID string `json:"account_id"`
	Role      string `json:"role"`
}
