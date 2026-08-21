package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// OnboardingService is the account setup surface the handler needs.
type OnboardingService interface {
	Get(ctx context.Context, tenant domain.Tenant) (*domain.Onboarding, error)
	SaveProgress(ctx context.Context, tenant domain.Tenant, progress domain.OnboardingProgress) (*domain.Onboarding, error)
	Complete(ctx context.Context, tenant domain.Tenant) error
	Dismiss(ctx context.Context, tenant domain.Tenant) error
	Resume(ctx context.Context, tenant domain.Tenant) error
}

// OnboardingHandler serves the account's resumable setup flow.
type OnboardingHandler struct {
	onboarding OnboardingService
}

// NewOnboardingHandler builds an OnboardingHandler.
func NewOnboardingHandler(onboarding OnboardingService) *OnboardingHandler {
	return &OnboardingHandler{onboarding: onboarding}
}

// Get returns the account's onboarding state.
//
//	@Summary		Get onboarding progress
//	@Description	Returns the current flow version, resume step, and resolved steps for the account.
//	@Tags			onboarding
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	dto.OnboardingResponse
//	@Failure		401	{object}	dto.ErrorResponse
//	@Failure		403	{object}	dto.ErrorResponse
//	@Failure		404	{object}	dto.ErrorResponse
//	@Router			/v1/onboarding [get]
func (h *OnboardingHandler) Get(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	onboarding, err := h.onboarding.Get(c.Request.Context(), tenant)
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, toOnboardingResponse(*onboarding))
}

// SaveProgress resolves one onboarding step and selects the next one.
//
//	@Summary		Save onboarding progress
//	@Description	Records a completed or skipped step and the stable step key to resume from.
//	@Tags			onboarding
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		dto.SaveOnboardingProgressRequest	true	"Progress"
//	@Success		200		{object}	dto.OnboardingResponse
//	@Failure		400		{object}	dto.ErrorResponse
//	@Failure		401		{object}	dto.ErrorResponse
//	@Failure		403		{object}	dto.ErrorResponse
//	@Failure		422		{object}	dto.ErrorResponse
//	@Router			/v1/onboarding [put]
func (h *OnboardingHandler) SaveProgress(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	var body dto.SaveOnboardingProgressRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondBindError(c, err)
		return
	}
	onboarding, err := h.onboarding.SaveProgress(c.Request.Context(), tenant,
		domain.OnboardingProgress{
			Step:        domain.OnboardingStepKey(body.Step),
			StepStatus:  domain.OnboardingStepStatus(body.StepStatus),
			CurrentStep: domain.OnboardingStepKey(body.CurrentStep),
		})
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, toOnboardingResponse(*onboarding))
}

// Complete marks account setup finished.
//
//	@Summary	Complete onboarding
//	@Tags		onboarding
//	@Security	BearerAuth
//	@Success	204
//	@Failure	401	{object}	dto.ErrorResponse
//	@Failure	403	{object}	dto.ErrorResponse
//	@Router		/v1/onboarding/complete [post]
func (h *OnboardingHandler) Complete(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	if err := h.onboarding.Complete(c.Request.Context(), tenant); err != nil {
		Respond(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// Dismiss leaves onboarding without blocking the backoffice.
//
//	@Summary	Dismiss onboarding
//	@Tags		onboarding
//	@Security	BearerAuth
//	@Success	204
//	@Failure	401	{object}	dto.ErrorResponse
//	@Failure	403	{object}	dto.ErrorResponse
//	@Router		/v1/onboarding/dismiss [post]
func (h *OnboardingHandler) Dismiss(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	if err := h.onboarding.Dismiss(c.Request.Context(), tenant); err != nil {
		Respond(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// Resume returns a dismissed onboarding to its last step.
//
//	@Summary	Resume onboarding
//	@Tags		onboarding
//	@Security	BearerAuth
//	@Success	204
//	@Failure	401	{object}	dto.ErrorResponse
//	@Failure	403	{object}	dto.ErrorResponse
//	@Router		/v1/onboarding/resume [post]
func (h *OnboardingHandler) Resume(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	if err := h.onboarding.Resume(c.Request.Context(), tenant); err != nil {
		Respond(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func toOnboardingResponse(onboarding domain.Onboarding) dto.OnboardingResponse {
	steps := make(map[string]string, len(onboarding.Steps))
	for key, status := range onboarding.Steps {
		steps[string(key)] = string(status)
	}
	return dto.OnboardingResponse{
		FlowVersion: onboarding.FlowVersion,
		Status:      string(onboarding.Status),
		CurrentStep: string(onboarding.CurrentStep),
		Steps:       steps,
		CompletedAt: onboarding.CompletedAt,
	}
}
