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
}

// NewRouter builds the engine with the global middleware and mounts every route.
func NewRouter(cfg *config.Config, log *slog.Logger, h Handlers) *gin.Engine {
	if cfg.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(middleware.RequestID(), middleware.Logger(log), middleware.Recovery(log))

	// Probes stay outside /v1 and outside auth: an orchestrator has no credentials.
	r.GET("/health", h.Health.Live)
	r.GET("/ready", h.Health.Ready)

	// Feature routes mount under /v1 in two groups: /v1/public for what works without
	// a session (login, and the tokenized quote the public webapp reads — both resolve
	// their own scope before touching tenant-scoped data), and a group behind
	// middleware.RequireTenant() for everything else, because a request-scoped query
	// without an account reads nothing under row level security.

	return r
}
