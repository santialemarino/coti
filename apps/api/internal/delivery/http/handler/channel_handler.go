package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// ChannelService is the channel surface the handler needs.
type ChannelService interface {
	ListChannels(ctx context.Context, tenant domain.Tenant) ([]domain.Channel, error)
	ListAllChannels(ctx context.Context, tenant domain.Tenant) ([]domain.Channel, error)
	CreateChannel(ctx context.Context, tenant domain.Tenant, in domain.NewChannel) (*domain.Channel, error)
	UpdateChannel(ctx context.Context, tenant domain.Tenant, channelID uuid.UUID, in domain.ChannelUpdate) (*domain.Channel, error)
	DeactivateChannel(ctx context.Context, tenant domain.Tenant, channelID uuid.UUID) error
}

// ChannelHandler serves intake channel discovery and administration.
type ChannelHandler struct {
	channels ChannelService
}

// NewChannelHandler builds a ChannelHandler.
func NewChannelHandler(channels ChannelService) *ChannelHandler {
	return &ChannelHandler{channels: channels}
}

// listFor picks the read the query asked for. The service refuses the administrative one to a
// seller, so the role check lives in one place rather than being repeated here.
func (h *ChannelHandler) listFor(
	ctx context.Context, tenant domain.Tenant, includeInactive bool,
) ([]domain.Channel, error) {
	if includeInactive {
		return h.channels.ListAllChannels(ctx, tenant)
	}
	return h.channels.ListChannels(ctx, tenant)
}

// List returns the active intake channels of the selected branch, or every one for an administrator.
//
//	@Summary		List intake channels
//	@Description	Returns active intake channels for the branch selected through X-Branch-Id. With include_inactive an administrator also gets the closed ones, which is for administering them rather than taking an order through one. No response carries a channel credential; is_configured reports only that one is stored.
//	@Tags			channels
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Branch-Id			header		string	true	"Active branch"
//	@Param			include_inactive	query		bool	false	"Include closed channels (administrators only)"
//	@Success		200					{object}	dto.ChannelListResponse
//	@Failure		400					{object}	dto.ErrorResponse
//	@Failure		401					{object}	dto.ErrorResponse
//	@Failure		403					{object}	dto.ErrorResponse
//	@Failure		422					{object}	dto.ErrorResponse	"No active branch"
//	@Router			/v1/channels [get]
func (h *ChannelHandler) List(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	var query dto.ListChannelsQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		RespondBindError(c, err)
		return
	}

	channels, err := h.listFor(c.Request.Context(), tenant, query.IncludeInactive)
	if err != nil {
		Respond(c, err)
		return
	}

	items := make([]dto.ChannelResponse, 0, len(channels))
	for _, channel := range channels {
		items = append(items, toChannelResponse(channel))
	}
	c.JSON(http.StatusOK, dto.ChannelListResponse{Items: items})
}

// Create opens an intake channel on the selected branch.
//
//	@Summary		Create an intake channel
//	@Description	Opens a channel on the branch selected through X-Branch-Id. config is validated against type and its credentials are encrypted before they are stored; a shape that does not match the type is refused here rather than when the first message arrives.
//	@Tags			channels
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Branch-Id	header		string						true	"Active branch"
//	@Param			request		body		dto.CreateChannelRequest	true	"Channel"
//	@Success		201			{object}	dto.ChannelResponse
//	@Failure		400			{object}	dto.ErrorResponse
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		403			{object}	dto.ErrorResponse
//	@Failure		409			{object}	dto.ErrorResponse	"The branch already holds that channel"
//	@Failure		422			{object}	dto.ErrorResponse
//	@Failure		503			{object}	dto.ErrorResponse	"No encryption key configured"
//	@Router			/v1/channels [post]
func (h *ChannelHandler) Create(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	var body dto.CreateChannelRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondBindError(c, err)
		return
	}

	channel, err := h.channels.CreateChannel(c.Request.Context(), tenant, domain.NewChannel{
		Type:       domain.ChannelType(body.Type),
		Identifier: body.Identifier,
		Config:     body.Config,
	})
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusCreated, toChannelResponse(*channel))
}

// Update replaces a channel's editable fields.
//
//	@Summary		Update an intake channel
//	@Description	Replaces the identifier and, when sent, the whole configuration. config omitted leaves the stored settings alone; an explicit null removes them. The type cannot change, because the shape of the configuration depends on it.
//	@Tags			channels
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Branch-Id	header		string						true	"Active branch"
//	@Param			channelId	path		string						true	"Channel id"
//	@Param			request		body		dto.UpdateChannelRequest	true	"Channel"
//	@Success		200			{object}	dto.ChannelResponse
//	@Failure		400			{object}	dto.ErrorResponse
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		403			{object}	dto.ErrorResponse
//	@Failure		404			{object}	dto.ErrorResponse
//	@Failure		409			{object}	dto.ErrorResponse	"The branch already holds that channel"
//	@Failure		422			{object}	dto.ErrorResponse
//	@Failure		503			{object}	dto.ErrorResponse	"No encryption key configured"
//	@Router			/v1/channels/{channelId} [put]
func (h *ChannelHandler) Update(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	channelID, ok := pathUUID(c, "channelId")
	if !ok {
		return
	}
	var body dto.UpdateChannelRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondBindError(c, err)
		return
	}

	channel, err := h.channels.UpdateChannel(c.Request.Context(), tenant, channelID,
		domain.ChannelUpdate{
			Identifier: body.Identifier,
			IsActive:   body.IsActive,
			Config:     body.Config,
		})
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, toChannelResponse(*channel))
}

// Delete closes an intake channel without removing it.
//
//	@Summary		Close an intake channel
//	@Description	Deactivates the channel so the orders that arrived through it stay explainable. Refuses to close the branch's manual-entry channel, which a counter or phone order has no other route to point at.
//	@Tags			channels
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Branch-Id	header	string	true	"Active branch"
//	@Param			channelId	path	string	true	"Channel id"
//	@Success		204			"No Content"
//	@Failure		400			{object}	dto.ErrorResponse
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		403			{object}	dto.ErrorResponse
//	@Failure		404			{object}	dto.ErrorResponse
//	@Failure		422			{object}	dto.ErrorResponse
//	@Router			/v1/channels/{channelId} [delete]
func (h *ChannelHandler) Delete(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	channelID, ok := pathUUID(c, "channelId")
	if !ok {
		return
	}
	if err := h.channels.DeactivateChannel(c.Request.Context(), tenant, channelID); err != nil {
		Respond(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func toChannelResponse(channel domain.Channel) dto.ChannelResponse {
	return dto.ChannelResponse{
		ID: channel.ID, BranchID: channel.BranchID, Type: string(channel.Type),
		Identifier: channel.Identifier, IsActive: channel.IsActive,
		IsConfigured: channel.IsConfigured,
		CreatedAt:    channel.CreatedAt, UpdatedAt: channel.UpdatedAt,
	}
}
