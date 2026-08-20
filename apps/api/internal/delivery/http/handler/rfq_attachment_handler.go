package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// RFQAttachmentService is the attachment surface the handler needs.
type RFQAttachmentService interface {
	List(ctx context.Context, tenant domain.Tenant, rfqID uuid.UUID) ([]domain.AttachmentLink, error)
	Upload(ctx context.Context, tenant domain.Tenant, rfqID uuid.UUID, file domain.AttachmentUpload) (*domain.AttachmentLink, error)
}

// RFQAttachmentHandler serves the files an RFQ arrived with.
type RFQAttachmentHandler struct {
	attachments RFQAttachmentService
	maxBytes    int64
}

// NewRFQAttachmentHandler builds an RFQAttachmentHandler.
func NewRFQAttachmentHandler(attachments RFQAttachmentService, maxBytes int64) *RFQAttachmentHandler {
	return &RFQAttachmentHandler{attachments: attachments, maxBytes: maxBytes}
}

// List returns the RFQ's attachments, each with a link that serves it until it expires. An RFQ
// that is not this caller's answers an empty list, which is what it looks like from here.
//
//	@Summary		List an RFQ's attachments
//	@Description	Returns each stored file with a freshly signed link. The link expires, so it is read from here rather than kept. An RFQ that does not exist, or belongs elsewhere, answers an empty list rather than 404.
//	@Tags			rfq
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Branch-Id	header		string	true	"Active branch"
//	@Param			rfqId		path		string	true	"RFQ id"
//	@Success		200			{object}	dto.RFQAttachmentListResponse
//	@Failure		400			{object}	dto.ErrorResponse
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		422			{object}	dto.ErrorResponse	"No active branch"
//	@Router			/v1/rfqs/{rfqId}/attachments [get]
func (h *RFQAttachmentHandler) List(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	rfqID, ok := pathUUID(c, "rfqId")
	if !ok {
		return
	}
	links, err := h.attachments.List(c.Request.Context(), tenant, rfqID)
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, dto.RFQAttachmentListResponse{Attachments: toAttachmentResponses(links)})
}

// Upload stores one file against an RFQ and returns it with a link.
//
//	@Summary		Upload an RFQ attachment
//	@Description	Stores one file for the RFQ and records the reference. The file is refused for its type or its size before any of it is stored, and the object key leads with the account.
//	@Tags			rfq
//	@Accept			multipart/form-data
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Branch-Id	header		string	true	"Active branch"
//	@Param			rfqId		path		string	true	"RFQ id"
//	@Param			file		formData	file	true	"The file to store"
//	@Success		201			{object}	dto.RFQAttachmentResponse
//	@Failure		400			{object}	dto.ErrorResponse
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		404			{object}	dto.ErrorResponse	"No such RFQ in the selected branch"
//	@Failure		413			{object}	dto.ErrorResponse	"FILE_TOO_LARGE"
//	@Failure		422			{object}	dto.ErrorResponse	"UNSUPPORTED_FILE_TYPE, or no active branch"
//	@Router			/v1/rfqs/{rfqId}/attachments [post]
func (h *RFQAttachmentHandler) Upload(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	rfqID, ok := pathUUID(c, "rfqId")
	if !ok {
		return
	}
	file, header, ok := openUpload(c, h.maxBytes)
	if !ok {
		return
	}
	defer file.Close()

	link, err := h.attachments.Upload(c.Request.Context(), tenant, rfqID, domain.AttachmentUpload{
		ContentType: header.Header.Get("Content-Type"),
		Size:        header.Size,
		Content:     file,
	})
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusCreated, toAttachmentResponse(*link))
}

func toAttachmentResponses(links []domain.AttachmentLink) []dto.RFQAttachmentResponse {
	responses := make([]dto.RFQAttachmentResponse, 0, len(links))
	for _, link := range links {
		responses = append(responses, toAttachmentResponse(link))
	}
	return responses
}

func toAttachmentResponse(link domain.AttachmentLink) dto.RFQAttachmentResponse {
	return dto.RFQAttachmentResponse{
		ID:               link.Attachment.ID,
		RFQID:            link.Attachment.RFQID,
		Type:             string(link.Attachment.Type),
		ProcessingStatus: string(link.Attachment.ProcessingStatus),
		URL:              link.URL,
		ExpiresAt:        link.ExpiresAt,
		CreatedAt:        link.Attachment.CreatedAt,
	}
}
