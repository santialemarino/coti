// Package http wires the routes onto a Gin engine. It owns no business logic — the
// handlers it mounts do that, and the composition root builds them.
package http

import (
	"log/slog"

	"github.com/gin-gonic/gin"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/delivery/http/handler"
	"github.com/santialemarino/coti/apps/api/internal/delivery/http/middleware"
)

// Handlers carries every handler the router mounts, so adding a feature is one field
// here instead of a new router parameter.
type Handlers struct {
	Health *handler.HealthHandler
	Auth   *handler.AuthHandler
}

// Auth carries what the authentication middleware needs to resolve a tenant.
type Auth struct {
	Verifier middleware.AccessVerifier
	Sessions middleware.SessionChecker
}

// NewRouter builds the engine with the global middleware and mounts every route.
func NewRouter(cfg *config.Config, log *slog.Logger, h Handlers, auth Auth) *gin.Engine {
	if cfg.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(middleware.RequestID(), middleware.Logger(log), middleware.Recovery(log))

	// Probes stay outside /v1 and outside auth: an orchestrator has no credentials.
	r.GET("/health", h.Health.Live)
	r.GET("/ready", h.Health.Ready)

	// Authenticate runs for every /v1 route and resolves a tenant when a valid token is
	// present. It does not reject anonymous requests — RequireTenant does — so a public
	// route can still see who the caller is when they happen to be logged in.
	v1 := r.Group("/v1", middleware.Authenticate(auth.Verifier, auth.Sessions))

	// Works without a session. Each route resolves its own scope before touching
	// tenant-scoped data.
	public := v1.Group("/public")
	public.POST("/auth/login", h.Auth.Login)
	public.POST("/auth/refresh", h.Auth.Refresh)

	// Everything else needs a resolved tenant: a request-scoped query without an account
	// reads nothing under row level security.
	authed := v1.Group("", middleware.RequireTenant())
	authed.POST("/auth/logout", h.Auth.Logout)

	return r
}
