package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// AccountService is the account surface the handler needs.
type AccountService interface {
	Get(ctx context.Context, tenant domain.Tenant) (*domain.Account, error)
	Register(ctx context.Context, in domain.Signup) (*domain.SignupResult, *domain.TokenPair, error)
	Update(ctx context.Context, tenant domain.Tenant, in domain.AccountUpdate) (*domain.Account, error)
}

// AccountHandler serves registration and the account record.
type AccountHandler struct {
	accounts AccountService
}

// NewAccountHandler builds an AccountHandler.
func NewAccountHandler(accounts AccountService) *AccountHandler {
	return &AccountHandler{accounts: accounts}
}

// Get returns the caller's own account.
//
//	@Summary		Get the current account
//	@Description	Returns the corralón the caller belongs to, brand included.
//	@Tags			accounts
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	dto.AccountResponse
//	@Failure		401	{object}	dto.ErrorResponse
//	@Failure		404	{object}	dto.ErrorResponse
//	@Router			/v1/account [get]
func (h *AccountHandler) Get(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	account, err := h.accounts.Get(c.Request.Context(), tenant)
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, toAccountResponse(*account))
}

// Register opens an account with its first branch and administrator.
//
//	@Summary		Register a corralón
//	@Description	Creates the account, its first branch with that branch's manual-entry channel, and the administrator, then returns a session.
//	@Tags			accounts
//	@Accept			json
//	@Produce		json
//	@Param			request	body		dto.SignupRequest	true	"Registration"
//	@Success		201		{object}	dto.SignupResponse
//	@Failure		400		{object}	dto.ErrorResponse
//	@Failure		409		{object}	dto.ErrorResponse
//	@Failure		422		{object}	dto.ErrorResponse
//	@Router			/v1/public/accounts [post]
func (h *AccountHandler) Register(c *gin.Context) {
	var body dto.SignupRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondBindError(c, err)
		return
	}

	result, pair, err := h.accounts.Register(c.Request.Context(), domain.Signup{
		AccountName:   body.AccountName,
		LegalName:     body.LegalName,
		TaxID:         body.TaxID,
		BranchName:    body.BranchName,
		BranchAddress: body.BranchAddress,
		AdminName:     body.AdminName,
		AdminEmail:    body.AdminEmail,
		AdminPassword: body.AdminPassword,
	})
	if err != nil {
		Respond(c, err)
		return
	}

	c.JSON(http.StatusCreated, dto.SignupResponse{
		Account: toAccountResponse(result.Account),
		Branch:  toBranchResponse(result.Branch),
		Tokens:  toTokenResponse(pair),
	})
}

// Update replaces the caller's account record.
//
//	@Summary		Update the current account
//	@Description	Replaces the corralón's record, including the brand the client webapp renders a quote with.
//	@Tags			accounts
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		dto.UpdateAccountRequest	true	"Account"
//	@Success		200		{object}	dto.AccountResponse
//	@Failure		400		{object}	dto.ErrorResponse
//	@Failure		401		{object}	dto.ErrorResponse
//	@Failure		403		{object}	dto.ErrorResponse
//	@Failure		404		{object}	dto.ErrorResponse
//	@Failure		422		{object}	dto.ErrorResponse
//	@Router			/v1/account [put]
func (h *AccountHandler) Update(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	var body dto.UpdateAccountRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondBindError(c, err)
		return
	}

	account, err := h.accounts.Update(c.Request.Context(), tenant, domain.AccountUpdate{
		Name:         body.Name,
		LegalName:    body.LegalName,
		TaxID:        body.TaxID,
		BrandLogoURL: body.BrandLogoURL,
		BrandColor:   body.BrandColor,
	})
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, toAccountResponse(*account))
}

func toAccountResponse(a domain.Account) dto.AccountResponse {
	return dto.AccountResponse{
		ID:           a.ID,
		Name:         a.Name,
		LegalName:    a.LegalName,
		TaxID:        a.TaxID,
		BrandLogoURL: a.BrandLogoURL,
		BrandColor:   a.BrandColor,
		IsActive:     a.IsActive,
		CreatedAt:    a.CreatedAt,
		UpdatedAt:    a.UpdatedAt,
	}
}
