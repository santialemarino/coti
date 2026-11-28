package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// ListTagRules returns the account's automatic client tag settings.
//
//	@Summary	List automatic client tag rules
//	@Tags		clients
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{object}	dto.ClientTagRulesResponse
//	@Failure	401	{object}	dto.ErrorResponse
//	@Failure	403	{object}	dto.ErrorResponse
//	@Router		/v1/tags/rules [get]
func (h *ClientHandler) ListTagRules(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	rules, err := h.clients.ListTagRules(c.Request.Context(), tenant)
	if err != nil {
		Respond(c, err)
		return
	}

	items := make([]dto.ClientTagRuleResponse, 0, len(rules))
	for _, rule := range rules {
		items = append(items, dto.ClientTagRuleResponse{
			Kind: string(rule.Kind), Enabled: rule.Enabled, Minimum: rule.Minimum, WindowDays: rule.WindowDays,
		})
	}
	c.JSON(http.StatusOK, dto.ClientTagRulesResponse{Items: items})
}

// UpdateTagRule updates an automatic rule for the account.
//
//	@Summary	Update an automatic client tag rule
//	@Tags		clients
//	@Accept		json
//	@Security	BearerAuth
//	@Param		kind	path	string							true	"Rule kind" Enums(RECURRING,ABANDONER)
//	@Param		body	body	dto.UpdateClientTagRuleRequest	true	"Rule settings"
//	@Success	204
//	@Failure	400	{object}	dto.ErrorResponse
//	@Failure	401	{object}	dto.ErrorResponse
//	@Failure	403	{object}	dto.ErrorResponse
//	@Failure	404	{object}	dto.ErrorResponse
//	@Failure	422	{object}	dto.ErrorResponse
//	@Router		/v1/tags/rules/{kind} [put]
func (h *ClientHandler) UpdateTagRule(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	var body dto.UpdateClientTagRuleRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		Respond(c, domain.ErrInvalidInput)
		return
	}
	rule := domain.ClientTagRule{
		Kind: domain.ClientTagRuleKind(c.Param("kind")), Enabled: *body.Enabled,
		Minimum: body.Minimum, WindowDays: *body.WindowDays,
	}
	if err := h.clients.UpdateTagRule(c.Request.Context(), tenant, rule); err != nil {
		Respond(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// SetTagExclusion toggles an automatic tag exclusion on a client profile.
//
//	@Summary	Set or clear a client's automatic tag exclusion
//	@Tags		clients
//	@Accept		json
//	@Security	BearerAuth
//	@Param		clientId	path	string								true	"Client id"
//	@Param		tagId		path	string								true	"Automatic tag id"
//	@Param		body		body	dto.SetClientTagExclusionRequest	true	"Exclusion state"
//	@Success	204
//	@Failure	400	{object}	dto.ErrorResponse
//	@Failure	401	{object}	dto.ErrorResponse
//	@Failure	404	{object}	dto.ErrorResponse
//	@Failure	422	{object}	dto.ErrorResponse
//	@Router		/v1/clients/{clientId}/tag-exclusions/{tagId} [put]
func (h *ClientHandler) SetTagExclusion(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	clientID, ok := pathUUID(c, "clientId")
	if !ok {
		return
	}
	tagID, ok := pathUUID(c, "tagId")
	if !ok {
		return
	}
	var body dto.SetClientTagExclusionRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		Respond(c, domain.ErrInvalidInput)
		return
	}
	if err := h.clients.SetTagExclusion(c.Request.Context(), tenant, clientID, tagID, *body.Excluded); err != nil {
		Respond(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
