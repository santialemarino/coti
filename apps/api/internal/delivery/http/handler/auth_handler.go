package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/delivery/http/middleware"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// AuthService is the auth surface the handler needs.
type AuthService interface {
	Login(ctx context.Context, in domain.Credentials) (*domain.TokenPair, error)
	Refresh(ctx context.Context, rawToken string) (*domain.TokenPair, error)
	Logout(ctx context.Context, tenant domain.Tenant, rawToken string) error
}

// AuthHandler serves login, refresh, and logout.
type AuthHandler struct {
	auth AuthService
}

// NewAuthHandler builds an AuthHandler.
func NewAuthHandler(auth AuthService) *AuthHandler {
	return &AuthHandler{auth: auth}
}

// Login exchanges credentials for a token pair. Returns 401 on bad credentials and 429
// while an account is locked out.
func (h *AuthHandler) Login(c *gin.Context) {
	var body dto.LoginRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondBindError(c, err)
		return
	}

	pair, err := h.auth.Login(c.Request.Context(), domain.Credentials{
		Email:      body.Email,
		Password:   body.Password,
		RememberMe: body.RememberMe,
	})
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, toTokenResponse(pair))
}

// Refresh rotates a refresh token and returns a new pair. Returns 401 when the token is
// unknown, expired, revoked, or reused past the grace window.
func (h *AuthHandler) Refresh(c *gin.Context) {
	var body dto.RefreshRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondBindError(c, err)
		return
	}

	pair, err := h.auth.Refresh(c.Request.Context(), body.RefreshToken)
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, toTokenResponse(pair))
}

// Logout invalidates the caller's outstanding access tokens and revokes the presented
// refresh family. Returns 204.
func (h *AuthHandler) Logout(c *gin.Context) {
	tenant, ok := middleware.TenantFrom(c)
	if !ok {
		Respond(c, domain.ErrUnauthenticated)
		return
	}

	// The body is optional here: a client that lost its refresh token must still be
	// able to end the session.
	var body dto.LogoutRequest
	_ = c.ShouldBindJSON(&body)

	if err := h.auth.Logout(c.Request.Context(), tenant, body.RefreshToken); err != nil {
		Respond(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func toTokenResponse(pair *domain.TokenPair) dto.TokenResponse {
	return dto.TokenResponse{
		AccessToken:     pair.AccessToken,
		AccessExpiresAt: pair.AccessExpiresAt,
		RefreshToken:    pair.RefreshToken,
		User: dto.UserBrief{
			ID:        pair.Tenant.UserID.String(),
			AccountID: pair.Tenant.AccountID.String(),
			Role:      string(pair.Tenant.Role),
		},
	}
}
