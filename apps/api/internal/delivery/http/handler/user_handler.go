package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/delivery/http/middleware"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// UserService is the user-administration surface the handler needs.
type UserService interface {
	ListUsers(ctx context.Context, tenant domain.Tenant) ([]domain.UserWithBranches, error)
	GetUser(ctx context.Context, tenant domain.Tenant, id uuid.UUID) (*domain.UserWithBranches, error)
	CreateUser(ctx context.Context, tenant domain.Tenant, in domain.NewUser) (*domain.UserWithBranches, error)
	UpdateUser(ctx context.Context, tenant domain.Tenant, id uuid.UUID, in domain.UserUpdate) (*domain.UserWithBranches, error)
	DeactivateUser(ctx context.Context, tenant domain.Tenant, id uuid.UUID) error
	ResendInvite(ctx context.Context, tenant domain.Tenant, id uuid.UUID) error
	ListSellers(ctx context.Context, tenant domain.Tenant) ([]domain.Seller, error)
}

// SessionPolicy is what the installation decides about every session, which /me reports so a
// screen never has to guess it.
type SessionPolicy struct {
	RequireVerifiedEmail bool
	MailDelivers         bool
}

// UserHandler serves the admin-only user administration routes.
type UserHandler struct {
	users       UserService
	policy      SessionPolicy
	mailAllowed middleware.Allowance
}

// NewUserHandler builds a UserHandler. mailAllowed is the caller's mail allowance, spent only by
// a creation that mails an invite.
func NewUserHandler(users UserService, policy SessionPolicy, mailAllowed middleware.Allowance) *UserHandler {
	return &UserHandler{users: users, policy: policy, mailAllowed: mailAllowed}
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

// ListSellers returns the active sellers the caller can assign a new RFQ to.
//
//	@Summary		List sellers
//	@Description	The manual RFQ assignee picklist: active sellers, narrowed to the active branch when one is set.
//	@Tags			users
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Branch-Id	header	string	false	"Active branch"
//	@Success		200			{object}	dto.SellerListResponse
//	@Failure		401			{object}	dto.ErrorResponse
//	@Router			/v1/sellers [get]
func (h *UserHandler) ListSellers(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}

	sellers, err := h.users.ListSellers(c.Request.Context(), tenant)
	if err != nil {
		Respond(c, err)
		return
	}

	items := make([]dto.SellerResponse, 0, len(sellers))
	for _, seller := range sellers {
		items = append(items, dto.SellerResponse{ID: seller.ID, Name: seller.Name})
	}
	c.JSON(http.StatusOK, dto.SellerListResponse{Items: items})
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
//	@Description	Admin only. Either sets the password or, with invite, mails a link to choose one (503 MAIL_NOT_CONFIGURED while mail only reaches the log). A duplicate email is a 409.
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
//	@Failure		429		{object}	dto.RateLimitResponse	"An invite past the caller's mail allowance"
//	@Failure		503		{object}	dto.ErrorResponse
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
	if body.Invite && !h.mailAllowed(c) {
		return
	}

	user, err := h.users.CreateUser(c.Request.Context(), tenant, domain.NewUser{
		Name:      body.Name,
		Email:     body.Email,
		Password:  body.Password,
		Invite:    body.Invite,
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

// ResendInvite mails an invited user a fresh link to choose their password. Returns 204.
//
//	@Summary		Resend a user's invite
//	@Description	Admin only. Retires the previous link. 422 INVITE_NOT_PENDING once the user has chosen a password; 503 MAIL_NOT_CONFIGURED while mail only reaches the log.
//	@Tags			users
//	@Produce		json
//	@Security		BearerAuth
//	@Param			userId	path	string	true	"User id"
//	@Success		204		"Invite sent"
//	@Failure		400		{object}	dto.ErrorResponse
//	@Failure		401		{object}	dto.ErrorResponse
//	@Failure		403		{object}	dto.ErrorResponse
//	@Failure		404		{object}	dto.ErrorResponse
//	@Failure		422		{object}	dto.ErrorResponse
//	@Failure		503		{object}	dto.ErrorResponse
//	@Router			/v1/users/{userId}/invite [post]
func (h *UserHandler) ResendInvite(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	userID, ok := pathUUID(c, "userId")
	if !ok {
		return
	}

	if err := h.users.ResendInvite(c.Request.Context(), tenant, userID); err != nil {
		Respond(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func toUserResponse(u domain.UserWithBranches) dto.UserResponse {
	var inviteStatus *string
	if u.InviteStatus != domain.InviteStatusNone {
		status := string(u.InviteStatus)
		inviteStatus = &status
	}
	return dto.UserResponse{
		ID:           u.ID,
		Name:         u.Name,
		Email:        u.Email,
		Role:         string(u.Role),
		IsActive:     u.IsActive,
		BranchIDs:    u.BranchIDs,
		InviteStatus: inviteStatus,
		LastLoginAt:  u.LastLoginAt,
		CreatedAt:    u.CreatedAt,
		UpdatedAt:    u.UpdatedAt,
	}
}

// Me returns the authenticated caller's own identity.
//
//	@Summary		Get the current user
//	@Description	Returns the caller's identity and branch reach, plus whether an unconfirmed address closes the product and whether mail reaches a mailbox, so the frontend never has to read the access token or guess the installation.
//	@Tags			users
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	dto.MeResponse
//	@Failure		401	{object}	dto.ErrorResponse
//	@Failure		404	{object}	dto.ErrorResponse
//	@Router			/v1/me [get]
func (h *UserHandler) Me(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	user, err := h.users.GetUser(c.Request.Context(), tenant, tenant.UserID)
	if err != nil {
		Respond(c, err)
		return
	}
	branchIDs := user.BranchIDs
	if branchIDs == nil {
		branchIDs = []uuid.UUID{}
	}
	c.JSON(http.StatusOK, dto.MeResponse{
		ID:            user.ID,
		Name:          user.Name,
		Email:         user.Email,
		EmailVerified: user.EmailVerifiedAt != nil,
		Role:          string(user.Role),
		AccountID:     user.AccountID,
		BranchIDs:     branchIDs,

		EmailVerificationRequired: h.policy.RequireVerifiedEmail,
		MailDelivery:              h.policy.MailDelivers,
	})
}
