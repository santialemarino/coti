package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// BranchService is the branch listing surface the handler needs.
type BranchService interface {
	ListBranches(ctx context.Context, tenant domain.Tenant) ([]domain.Branch, error)
}

// BranchHandler serves the branch list behind the branch switcher.
type BranchHandler struct {
	branches BranchService
}

// NewBranchHandler builds a BranchHandler.
func NewBranchHandler(branches BranchService) *BranchHandler {
	return &BranchHandler{branches: branches}
}

// List returns the branches the caller may operate on.
//
//	@Summary		List branches
//	@Description	Every active branch of the account for an admin; the assigned ones for a seller.
//	@Tags			branches
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	dto.BranchListResponse
//	@Failure		401	{object}	dto.ErrorResponse
//	@Router			/v1/branches [get]
func (h *BranchHandler) List(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}

	branches, err := h.branches.ListBranches(c.Request.Context(), tenant)
	if err != nil {
		Respond(c, err)
		return
	}

	items := make([]dto.BranchResponse, 0, len(branches))
	for _, b := range branches {
		items = append(items, toBranchResponse(b))
	}
	c.JSON(http.StatusOK, dto.BranchListResponse{Items: items})
}

func toBranchResponse(b domain.Branch) dto.BranchResponse {
	return dto.BranchResponse{
		ID:                b.ID,
		Name:              b.Name,
		Address:           b.Address,
		DefaultExpiryDays: b.DefaultExpiryDays,
		IsActive:          b.IsActive,
		CreatedAt:         b.CreatedAt,
		UpdatedAt:         b.UpdatedAt,
	}
}
