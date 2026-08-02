package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/delivery/http/middleware"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// BranchService is the branch-list surface the handler needs.
type BranchService interface {
	List(ctx context.Context, tenant domain.Tenant) ([]domain.Branch, error)
}

// BranchHandler serves branches available to the authenticated user.
type BranchHandler struct {
	branches BranchService
}

// NewBranchHandler builds a BranchHandler.
func NewBranchHandler(branches BranchService) *BranchHandler {
	return &BranchHandler{branches: branches}
}

// List returns the branches the caller may operate on.
func (h *BranchHandler) List(c *gin.Context) {
	tenant, ok := middleware.TenantFrom(c)
	if !ok {
		Respond(c, domain.ErrUnauthenticated)
		return
	}
	branches, err := h.branches.List(c.Request.Context(), tenant)
	if err != nil {
		Respond(c, err)
		return
	}
	response := make([]dto.BranchResponse, len(branches))
	for i, branch := range branches {
		response[i] = dto.BranchResponse{ID: branch.ID.String(), Name: branch.Name}
	}
	c.JSON(http.StatusOK, response)
}
