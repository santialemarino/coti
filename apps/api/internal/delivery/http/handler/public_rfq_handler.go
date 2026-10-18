package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// PublicRFQService is the stateless public intake surface.
type PublicRFQService interface {
	GetDestination(context.Context, uuid.UUID) (*domain.PublicRFQDestination, error)
	Create(context.Context, uuid.UUID, domain.PublicRFQInput) error
}

// PublicRFQHandler serves public request forms and receipt acknowledgements.
type PublicRFQHandler struct {
	service  PublicRFQService
	maxBytes int64
}

// NewPublicRFQHandler builds a PublicRFQHandler.
func NewPublicRFQHandler(service PublicRFQService, maxBytes int64) *PublicRFQHandler {
	return &PublicRFQHandler{service: service, maxBytes: maxBytes}
}

// GetDestination returns an active supplier's public intake identity.
//
// @Summary Get a public RFQ destination
// @Tags public-rfqs
// @Produce json
// @Param channelId path string true "WEBAPP channel ID"
// @Success 200 {object} dto.PublicRFQDestinationResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /v1/public/rfq-channels/{channelId} [get]
func (h *PublicRFQHandler) GetDestination(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, ok := pathUUID(c, "channelId")
	if !ok {
		return
	}
	destination, err := h.service.GetDestination(c.Request.Context(), id)
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, dto.PublicRFQDestinationResponse{SupplierName: destination.SupplierName,
		BranchName: destination.BranchName, LogoURL: destination.LogoURL,
		MaxTextCharacters: destination.MaxTextCharacters, MaxFileSize: destination.MaxFileSize,
		AcceptedContentTypes: destination.AcceptedContentTypes})
}

// Create accepts one text or file order without resolving a customer identity.
//
// @Summary Submit a public RFQ
// @Description Accepts JSON text or multipart contact fields and one file; request_id is a UUID v4 reused only for retries of the same submission.
// @Tags public-rfqs
// @Accept json,multipart/form-data
// @Produce json
// @Param channelId path string true "WEBAPP channel ID"
// @Param request body dto.CreatePublicRFQRequest true "Text request; multipart uses the same fields and a file part"
// @Success 201 {object} dto.PublicRFQReceiptResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Failure 409 {object} dto.ErrorResponse
// @Failure 413 {object} dto.ErrorResponse
// @Failure 422 {object} dto.ErrorResponse
// @Failure 429 {object} dto.ErrorResponse
// @Router /v1/public/rfq-channels/{channelId}/requests [post]
func (h *PublicRFQHandler) Create(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, ok := pathUUID(c, "channelId")
	if !ok {
		return
	}
	var body dto.CreatePublicRFQRequest
	var in domain.PublicRFQInput
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, h.maxBytes+64*1024)
	if strings.HasPrefix(c.ContentType(), "multipart/form-data") {
		if err := c.Request.ParseMultipartForm(64 * 1024); err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				Respond(c, domain.ErrTooLarge)
				return
			}
			RespondBindError(c, err)
			return
		}
		defer func() { _ = c.Request.MultipartForm.RemoveAll() }()
		if err := c.ShouldBind(&body); err != nil {
			RespondBindError(c, err)
			return
		}
		if len(c.Request.MultipartForm.File) != 1 || len(c.Request.MultipartForm.File["file"]) != 1 {
			Respond(c, domain.ErrInvalidInput)
			return
		}
		header := c.Request.MultipartForm.File["file"][0]
		file, err := header.Open()
		if err != nil {
			RespondBindError(c, err)
			return
		}
		defer file.Close()
		in.File = &domain.AttachmentUpload{ContentType: header.Header.Get("Content-Type"), Size: header.Size, Content: file}
		in.Filename = header.Filename
	} else {
		if err := c.ShouldBindJSON(&body); err != nil {
			RespondBindError(c, err)
			return
		}
	}
	in.RequestID, _ = uuid.Parse(body.RequestID)
	in.Contact = domain.PublicRFQContact{TaxID: body.TaxID, Name: body.Name, Email: body.Email, Phone: body.Phone}
	in.Text = body.Text
	if err := h.service.Create(c.Request.Context(), id, in); err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusCreated, dto.PublicRFQReceiptResponse{Received: true})
}
