package handler

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// ManualRFQService is the request-to-quote surface the manual entry handler needs.
type ManualRFQService interface {
	List(ctx context.Context, tenant domain.Tenant) ([]domain.RfqListItem, error)
	CreateManual(ctx context.Context, tenant domain.Tenant, in domain.NewRfq) (*domain.RfqCreation, error)
}

// RfqHandler serves manual RFQ intake.
type RfqHandler struct {
	rfqs ManualRFQService
}

// NewRfqHandler builds an RfqHandler.
func NewRfqHandler(rfqs ManualRFQService) *RfqHandler {
	return &RfqHandler{rfqs: rfqs}
}

// List returns the RFQ list for the caller's tenant scope.
//
//	@Summary		List RFQs
//	@Description	Returns the RFQ list scoped to the tenant and, when set, the active branch.
//	@Tags			rfqs
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Branch-Id	header		string	false	"Active branch"
//	@Success		200			{object}	[]dto.RfqListItemResponse
//	@Failure		401			{object}	dto.ErrorResponse
//	@Router			/v1/rfqs [get]
func (h *RfqHandler) List(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	items, err := h.rfqs.List(c.Request.Context(), tenant)
	if err != nil {
		Respond(c, err)
		return
	}
	resp := make([]dto.RfqListItemResponse, len(items))
	for i, item := range items {
		resp[i] = toListItemResponse(item)
	}
	c.JSON(http.StatusOK, resp)
}

// Create records a manual RFQ.
//
//	@Summary		Create a manual RFQ
//	@Description	Records a counter or phone order.
//	@Tags			rfqs
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Branch-Id	header		string				true	"Active branch"
//	@Param			request		body		dto.CreateRfqRequest	true	"Manual RFQ"
//	@Success		201			{object}	dto.CreateRfqResponse
//	@Failure		400			{object}	dto.ErrorResponse
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		404			{object}	dto.ErrorResponse
//	@Failure		422			{object}	dto.ErrorResponse
//	@Router			/v1/rfqs [post]
func (h *RfqHandler) Create(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	var body dto.CreateRfqRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondBindError(c, err)
		return
	}

	in := domain.NewRfq{
		RawText:     body.RawText,
		WorkType:    body.WorkType,
		ClientLabel: body.ClientLabel,
		Items:       make([]domain.NewRfqItem, 0, len(body.Items)),
	}
	for _, item := range body.Items {
		quantity, err := decimal.NewFromString(item.Quantity)
		if err != nil {
			RespondBindError(c, fmt.Errorf("quantity is not a decimal: %w", err))
			return
		}
		in.Items = append(in.Items, domain.NewRfqItem{
			ProductID:            item.ProductID,
			RequestedDescription: item.RequestedDescription,
			Quantity:             quantity,
			Unit:                 item.Unit,
		})
	}

	creation, err := h.rfqs.CreateManual(c.Request.Context(), tenant, in)
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusCreated, dto.CreateRfqResponse{
		Rfq:   toRfqResponse(creation.Rfq),
		Quote: toQuoteResponse(creation.Quote),
	})
}

func toRfqResponse(r domain.RFQ) dto.RfqResponse {
	return dto.RfqResponse{
		ID:          r.ID,
		BranchID:    r.BranchID,
		ClientLabel: r.ClientLabel,
		ChannelID:   r.ChannelID,
		RawText:     r.RawText,
		Status:      string(r.Status),
		WorkType:    r.WorkType,
		ReceivedAt:  r.ReceivedAt,
		CreatedAt:   r.CreatedAt,
	}
}

func toListItemResponse(item domain.RfqListItem) dto.RfqListItemResponse {
	return dto.RfqListItemResponse{
		ID:         item.ID,
		Client:     item.ClientLabel,
		CreatedAt:  item.CreatedAt,
		Channel:    item.Channel,
		Seller:     item.SellerName,
		Branch:     item.BranchName,
		ItemCount:  item.ItemCount,
		Total:      item.Total,
		Status:     item.Status,
		ArchivedAt: item.ArchivedAt,
	}
}

// RFQService is the RFQ surface the AI pipeline handler needs.
type RFQService interface {
	CreateTextDraft(ctx context.Context, tenant domain.Tenant, in domain.TextRFQDraftInput) (*domain.TextRFQDraft, error)
	CreateWhatsAppMockDraft(ctx context.Context, tenant domain.Tenant, in domain.WhatsAppMockRFQInput) (*domain.TextRFQDraft, error)
}

// RFQHandler serves AI pipeline RFQ intake endpoints.
type RFQHandler struct {
	rfqs RFQService
}

// NewRFQHandler builds an RFQHandler.
func NewRFQHandler(rfqs RFQService) *RFQHandler {
	return &RFQHandler{rfqs: rfqs}
}

// CreateTextDraft creates a quote draft from plain RFQ text.
//
//	@Summary		Create an RFQ text draft
//	@Description	Persists the original text before reading it, then returns a quote DRAFT with one line per material, each carrying its catalog match, its confidence, and the candidates a flagged line was decided against.
//	@Tags			rfq
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Branch-Id	header		string							true	"Active branch"
//	@Param			body		body		dto.CreateTextRFQDraftRequest	true	"RFQ text to process"
//	@Success		201			{object}	dto.TextRFQDraftResponse
//	@Failure		400			{object}	dto.ErrorResponse
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		422			{object}	dto.ErrorResponse
//	@Failure		429			{object}	dto.RateLimitResponse
//	@Failure		503			{object}	dto.ErrorResponse
//	@Router			/v1/rfqs/text-drafts [post]
func (h *RFQHandler) CreateTextDraft(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}

	var body dto.CreateTextRFQDraftRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondBindError(c, err)
		return
	}

	draft, err := h.rfqs.CreateTextDraft(c.Request.Context(), tenant, domain.TextRFQDraftInput{
		ChannelID:   body.ChannelID,
		ClientID:    body.ClientID,
		ClientLabel: body.ClientLabel,
		RawText:     body.RawText,
		WorkType:    body.WorkType,
	})
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusCreated, toTextRFQDraftResponse(*draft))
}

// CreateWhatsAppMockDraft simulates one inbound WhatsApp message outside production.
//
//	@Summary		Simulate an inbound WhatsApp message
//	@Description	Development-only intake that resolves an active WhatsApp channel and creates the same seller-reviewable RFQ draft as the production text flow.
//	@Tags			development
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Branch-Id	header		string									true	"Active branch"
//	@Param			body		body		dto.CreateWhatsAppMockRFQDraftRequest	true	"Inbound WhatsApp message"
//	@Success		201			{object}	dto.TextRFQDraftResponse
//	@Failure		400			{object}	dto.ErrorResponse
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		404			{object}	dto.ErrorResponse
//	@Failure		422			{object}	dto.ErrorResponse
//	@Failure		429			{object}	dto.RateLimitResponse
//	@Failure		503			{object}	dto.ErrorResponse
//	@Router			/v1/dev/whatsapp/messages [post]
func (h *RFQHandler) CreateWhatsAppMockDraft(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}

	var body dto.CreateWhatsAppMockRFQDraftRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondBindError(c, err)
		return
	}

	draft, err := h.rfqs.CreateWhatsAppMockDraft(c.Request.Context(), tenant,
		domain.WhatsAppMockRFQInput{
			ChannelID: body.ChannelID, From: body.From, ProfileName: body.ProfileName,
			Text: body.Text,
		})
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusCreated, toTextRFQDraftResponse(*draft))
}

func toTextRFQDraftResponse(draft domain.TextRFQDraft) dto.TextRFQDraftResponse {
	items := make([]dto.QuoteItemResponse, 0, len(draft.Items))
	for _, item := range draft.Items {
		items = append(items, toQuoteItemResponse(item, draft.Alternatives[item.ID], nil))
	}
	var quote *dto.QuoteResponse
	if draft.Quote != nil {
		response := toQuoteResponse(*draft.Quote)
		quote = &response
	}
	var version *dto.QuoteVersionResponse
	if draft.Version != nil {
		response := toQuoteVersionResponse(*draft.Version)
		version = &response
	}
	return dto.TextRFQDraftResponse{
		RFQ:     toRFQAIResponse(draft.RFQ),
		Quote:   quote,
		Version: version,
		Items:   items,
	}
}

func toRFQAIResponse(rfq domain.RFQ) dto.RFQResponse {
	return dto.RFQResponse{
		ID: rfq.ID, BranchID: rfq.BranchID, ClientID: rfq.ClientID, ChannelID: rfq.ChannelID,
		RawText: rfq.RawText, Status: string(rfq.Status), WorkType: rfq.WorkType,
		ClientLabel: rfq.ClientLabel, ReceivedAt: rfq.ReceivedAt, CreatedAt: rfq.CreatedAt,
		UpdatedAt: rfq.UpdatedAt,
	}
}
