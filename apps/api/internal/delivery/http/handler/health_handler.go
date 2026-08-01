// Package handler holds the HTTP layer: bind the DTO, call the service, translate the
// result or domain error into a response. No business logic, no SQL.
package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
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
//
//	@Summary		Liveness probe
//	@Description	Checks no dependency, so a failing database does not restart the container.
//	@Tags			health
//	@Produce		json
//	@Success		200	{object}	dto.HealthResponse
//	@Router			/health [get]
func (h *HealthHandler) Live(c *gin.Context) {
	c.JSON(http.StatusOK, dto.HealthResponse{Status: "ok"})
}

// Ready reports whether the process can serve traffic, which means both pools answer.
// Returns 503 when they do not.
//
//	@Summary		Readiness probe
//	@Description	Reports whether the process can serve traffic: both database pools answer.
//	@Tags			health
//	@Produce		json
//	@Success		200	{object}	dto.HealthResponse
//	@Failure		503	{object}	dto.HealthResponse
//	@Router			/ready [get]
func (h *HealthHandler) Ready(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), readinessTimeout)
	defer cancel()

	if err := h.db.Ping(ctx); err != nil {
		c.JSON(http.StatusServiceUnavailable, dto.HealthResponse{
			Status: "unavailable",
			Detail: "database",
		})
		return
	}
	c.JSON(http.StatusOK, dto.HealthResponse{Status: "ready"})
}
