package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// BranchService is the branch surface the handler needs.
type BranchService interface {
	ListBranches(ctx context.Context, tenant domain.Tenant) ([]domain.Branch, error)
	CreateBranch(ctx context.Context, tenant domain.Tenant, in domain.NewBranch) (*domain.Branch, error)
	UpdateBranch(ctx context.Context, tenant domain.Tenant, branchID uuid.UUID, in domain.BranchUpdate) (*domain.Branch, error)
	DeactivateBranch(ctx context.Context, tenant domain.Tenant, branchID uuid.UUID) error
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

// Create opens a branch under the caller's account.
//
//	@Summary		Create a branch
//	@Description	Opens a branch together with its manual-entry channel, which a counter or phone order needs.
//	@Tags			branches
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		dto.CreateBranchRequest	true	"Branch"
//	@Success		201		{object}	dto.BranchResponse
//	@Failure		400		{object}	dto.ErrorResponse
//	@Failure		401		{object}	dto.ErrorResponse
//	@Failure		403		{object}	dto.ErrorResponse
//	@Failure		422		{object}	dto.ErrorResponse
//	@Router			/v1/branches [post]
func (h *BranchHandler) Create(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	var body dto.CreateBranchRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondBindError(c, err)
		return
	}

	branch, err := h.branches.CreateBranch(c.Request.Context(), tenant, domain.NewBranch{
		Name:              body.Name,
		Address:           body.Address,
		DefaultExpiryDays: body.DefaultExpiryDays,
	})
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusCreated, toBranchResponse(*branch))
}

// Update replaces a branch's editable fields.
//
//	@Summary		Update a branch
//	@Description	Replaces name, address and default quote expiry. is_active omitted leaves the flag alone.
//	@Tags			branches
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			branchId	path		string					true	"Branch id"
//	@Param			request		body		dto.UpdateBranchRequest	true	"Branch"
//	@Success		200			{object}	dto.BranchResponse
//	@Failure		400			{object}	dto.ErrorResponse
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		403			{object}	dto.ErrorResponse
//	@Failure		404			{object}	dto.ErrorResponse
//	@Failure		422			{object}	dto.ErrorResponse
//	@Router			/v1/branches/{branchId} [put]
func (h *BranchHandler) Update(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	branchID, ok := pathUUID(c, "branchId")
	if !ok {
		return
	}
	var body dto.UpdateBranchRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondBindError(c, err)
		return
	}

	branch, err := h.branches.UpdateBranch(c.Request.Context(), tenant, branchID,
		domain.BranchUpdate{
			Name:              body.Name,
			Address:           body.Address,
			DefaultExpiryDays: body.DefaultExpiryDays,
			IsActive:          body.IsActive,
		})
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, toBranchResponse(*branch))
}

// Delete closes a branch without removing it.
//
//	@Summary		Close a branch
//	@Description	Deactivates the branch so its quotes and prices stay explainable. Refuses to close the account's last active branch.
//	@Tags			branches
//	@Produce		json
//	@Security		BearerAuth
//	@Param			branchId	path	string	true	"Branch id"
//	@Success		204			"No Content"
//	@Failure		400			{object}	dto.ErrorResponse
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		403			{object}	dto.ErrorResponse
//	@Failure		404			{object}	dto.ErrorResponse
//	@Failure		422			{object}	dto.ErrorResponse
//	@Router			/v1/branches/{branchId} [delete]
func (h *BranchHandler) Delete(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	branchID, ok := pathUUID(c, "branchId")
	if !ok {
		return
	}
	if err := h.branches.DeactivateBranch(c.Request.Context(), tenant, branchID); err != nil {
		Respond(c, err)
		return
	}
	c.Status(http.StatusNoContent)
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
