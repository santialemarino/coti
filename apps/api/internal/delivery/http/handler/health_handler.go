// Package handler holds the HTTP layer: bind the DTO, call the service, translate the
// result or domain error into a response. No business logic, no SQL.
package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// readinessTimeout bounds the dependency check so a hung database cannot hold the
// probe open past the orchestrator's own timeout.
const readinessTimeout = 3 * time.Second

// Pinger is the dependency-liveness surface the readiness probe needs.
type Pinger interface {
	Ping(ctx context.Context) error
}

// HealthHandler serves the liveness and readiness probes.
type HealthHandler struct {
	db Pinger
}

// NewHealthHandler builds a HealthHandler.
func NewHealthHandler(db Pinger) *HealthHandler {
	return &HealthHandler{db: db}
}

// Live reports that the process is up. It checks no dependency on purpose: a failing
// database should not get the container restarted.
func (h *HealthHandler) Live(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// Ready reports whether the process can serve traffic, which means both pools answer.
// Returns 503 when they do not.
func (h *HealthHandler) Ready(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), readinessTimeout)
	defer cancel()

	if err := h.db.Ping(ctx); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable", "detail": "database"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ready"})
}
