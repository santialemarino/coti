package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// UserService is the user-administration surface the handler needs.
type UserService interface {
	ListUsers(ctx context.Context, tenant domain.Tenant) ([]domain.UserWithBranches, error)
	GetUser(ctx context.Context, tenant domain.Tenant, id uuid.UUID) (*domain.UserWithBranches, error)
	CreateUser(ctx context.Context, tenant domain.Tenant, in domain.NewUser) (*domain.UserWithBranches, error)
	UpdateUser(ctx context.Context, tenant domain.Tenant, id uuid.UUID, in domain.UserUpdate) (*domain.UserWithBranches, error)
	DeactivateUser(ctx context.Context, tenant domain.Tenant, id uuid.UUID) error
}

// UserHandler serves the admin-only user administration routes.
type UserHandler struct {
	users UserService
}

// NewUserHandler builds a UserHandler.
func NewUserHandler(users UserService) *UserHandler {
	return &UserHandler{users: users}
}

// List returns the account's users.
//
//	@Summary		List users
//	@Description	Admin only. Ordered by name, deactivated users included so they can be re-enabled.
//	@Tags			users
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	dto.UserListResponse
//	@Failure		401	{object}	dto.ErrorResponse
//	@Failure		403	{object}	dto.ErrorResponse
//	@Router			/v1/users [get]
func (h *UserHandler) List(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}

	users, err := h.users.ListUsers(c.Request.Context(), tenant)
	if err != nil {
		Respond(c, err)
		return
	}

	items := make([]dto.UserResponse, 0, len(users))
	for _, u := range users {
		items = append(items, toUserResponse(u))
	}
	c.JSON(http.StatusOK, dto.UserListResponse{Items: items})
}

// Get returns one user of the account.
//
//	@Summary	Get a user
//	@Tags		users
//	@Produce	json
//	@Security	BearerAuth
//	@Param		userId	path		string	true	"User id"
//	@Success	200		{object}	dto.UserResponse
//	@Failure	400		{object}	dto.ErrorResponse
//	@Failure	401		{object}	dto.ErrorResponse
//	@Failure	403		{object}	dto.ErrorResponse
//	@Failure	404		{object}	dto.ErrorResponse
//	@Router		/v1/users/{userId} [get]
func (h *UserHandler) Get(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	userID, ok := pathUUID(c, "userId")
	if !ok {
		return
	}

	user, err := h.users.GetUser(c.Request.Context(), tenant, userID)
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, toUserResponse(*user))
}

// Create adds a user to the caller's account.
//
//	@Summary		Create a user
//	@Description	Admin only. The account comes from the session; a duplicate email inside it is a 409.
//	@Tags			users
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		dto.CreateUserRequest	true	"User to create"
//	@Success		201		{object}	dto.UserResponse
//	@Failure		400		{object}	dto.ErrorResponse
//	@Failure		401		{object}	dto.ErrorResponse
//	@Failure		403		{object}	dto.ErrorResponse
//	@Failure		409		{object}	dto.ErrorResponse
//	@Failure		422		{object}	dto.ErrorResponse
//	@Router			/v1/users [post]
func (h *UserHandler) Create(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}

	var body dto.CreateUserRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondBindError(c, err)
		return
	}

	user, err := h.users.CreateUser(c.Request.Context(), tenant, domain.NewUser{
		Name:      body.Name,
		Email:     body.Email,
		Password:  body.Password,
		Role:      domain.UserRole(body.Role),
		BranchIDs: body.BranchIDs,
	})
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusCreated, toUserResponse(*user))
}

// Update replaces a user's profile, role and branch assignments.
//
//	@Summary		Update a user
//	@Description	Admin only. Replaces the profile; is_active omitted leaves the flag alone.
//	@Tags			users
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			userId	path		string					true	"User id"
//	@Param			request	body		dto.UpdateUserRequest	true	"Replacement values"
//	@Success		200		{object}	dto.UserResponse
//	@Failure		400		{object}	dto.ErrorResponse
//	@Failure		401		{object}	dto.ErrorResponse
//	@Failure		403		{object}	dto.ErrorResponse
//	@Failure		404		{object}	dto.ErrorResponse
//	@Failure		409		{object}	dto.ErrorResponse
//	@Failure		422		{object}	dto.ErrorResponse
//	@Router			/v1/users/{userId} [put]
func (h *UserHandler) Update(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	userID, ok := pathUUID(c, "userId")
	if !ok {
		return
	}

	var body dto.UpdateUserRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondBindError(c, err)
		return
	}

	user, err := h.users.UpdateUser(c.Request.Context(), tenant, userID, domain.UserUpdate{
		Name:      body.Name,
		Email:     body.Email,
		Role:      domain.UserRole(body.Role),
		BranchIDs: body.BranchIDs,
		IsActive:  body.IsActive,
	})
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, toUserResponse(*user))
}

// Delete deactivates a user.
//
//	@Summary		Deactivate a user
//	@Description	Admin only. Keeps the row so their quotes keep an author, and revokes their tokens at once.
//	@Tags			users
//	@Produce		json
//	@Security		BearerAuth
//	@Param			userId	path	string	true	"User id"
//	@Success		204		"Deactivated"
//	@Failure		400		{object}	dto.ErrorResponse
//	@Failure		401		{object}	dto.ErrorResponse
//	@Failure		403		{object}	dto.ErrorResponse
//	@Failure		404		{object}	dto.ErrorResponse
//	@Failure		422		{object}	dto.ErrorResponse
//	@Router			/v1/users/{userId} [delete]
func (h *UserHandler) Delete(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	userID, ok := pathUUID(c, "userId")
	if !ok {
		return
	}

	if err := h.users.DeactivateUser(c.Request.Context(), tenant, userID); err != nil {
		Respond(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func toUserResponse(u domain.UserWithBranches) dto.UserResponse {
	return dto.UserResponse{
		ID:          u.ID,
		Name:        u.Name,
		Email:       u.Email,
		Role:        string(u.Role),
		IsActive:    u.IsActive,
		BranchIDs:   u.BranchIDs,
		LastLoginAt: u.LastLoginAt,
		CreatedAt:   u.CreatedAt,
		UpdatedAt:   u.UpdatedAt,
	}
}
