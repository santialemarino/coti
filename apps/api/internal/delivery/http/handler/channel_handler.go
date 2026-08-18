package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// ChannelService is the channel surface the handler needs.
type ChannelService interface {
	ListChannels(ctx context.Context, tenant domain.Tenant) ([]domain.Channel, error)
}

// ChannelHandler serves intake channel discovery.
type ChannelHandler struct {
	channels ChannelService
}

// NewChannelHandler builds a ChannelHandler.
func NewChannelHandler(channels ChannelService) *ChannelHandler {
	return &ChannelHandler{channels: channels}
}

// List returns the active intake channels of the selected branch.
//
//	@Summary		List intake channels
//	@Description	Returns active intake channels for the branch selected through X-Branch-Id.
//	@Tags			channels
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Branch-Id	header		string	true	"Active branch"
//	@Success		200			{object}	dto.ChannelListResponse
//	@Failure		400			{object}	dto.ErrorResponse
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		422			{object}	dto.ErrorResponse	"No active branch"
//	@Router			/v1/channels [get]
func (h *ChannelHandler) List(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}

	channels, err := h.channels.ListChannels(c.Request.Context(), tenant)
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

func toChannelResponse(channel domain.Channel) dto.ChannelResponse {
	return dto.ChannelResponse{
		ID: channel.ID, BranchID: channel.BranchID, Type: string(channel.Type),
		Identifier: channel.Identifier, IsActive: channel.IsActive,
		CreatedAt: channel.CreatedAt, UpdatedAt: channel.UpdatedAt,
	}
}
