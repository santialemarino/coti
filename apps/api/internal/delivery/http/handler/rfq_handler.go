package handler

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// ManualRFQService is the request-to-quote surface the manual entry handler needs.
type ManualRFQService interface {
	List(ctx context.Context, tenant domain.Tenant) ([]domain.RfqListItem, error)
	CreateManual(ctx context.Context, tenant domain.Tenant, in domain.NewRfq) (*domain.RfqCreation, error)
	GetDetail(ctx context.Context, tenant domain.Tenant, rfqID uuid.UUID) (*domain.RfqDetail, error)
	UpdateItem(ctx context.Context, tenant domain.Tenant, quoteID, itemID uuid.UUID, in domain.QuoteItemUpdate) (*domain.QuoteItem, error)
	DeleteItem(ctx context.Context, tenant domain.Tenant, quoteID, itemID uuid.UUID) error
	AddItem(ctx context.Context, tenant domain.Tenant, quoteID uuid.UUID, in domain.QuoteItemCreate) (*domain.QuoteItem, error)
	AddDiscount(ctx context.Context, tenant domain.Tenant, quoteID uuid.UUID, in domain.QuoteDiscountCreate) (*domain.QuoteDiscount, error)
	UpdateDiscount(ctx context.Context, tenant domain.Tenant, quoteID, discountID uuid.UUID, in domain.QuoteDiscountUpdate) (*domain.QuoteDiscount, error)
	DeleteDiscount(ctx context.Context, tenant domain.Tenant, quoteID, discountID uuid.UUID) error
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

// Get returns the full detail of one RFQ.
//
//	@Summary		Get RFQ detail
//	@Description	Returns the RFQ header, associated quote, version, items, and alternatives.
//	@Tags			rfqs
//	@Produce		json
//	@Security		BearerAuth
//	@Param			rfqId	path	string	true	"RFQ id"
//	@Success		200		{object}	dto.RfqDetailResponse
//	@Failure		401		{object}	dto.ErrorResponse
//	@Failure		404		{object}	dto.ErrorResponse
//	@Router			/v1/rfqs/{rfqId} [get]
func (h *RfqHandler) Get(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	rfqID, ok := pathUUID(c, "rfqId")
	if !ok {
		return
	}

	detail, err := h.rfqs.GetDetail(c.Request.Context(), tenant, rfqID)
	if err != nil {
		Respond(c, err)
		return
	}

	resp := toRfqDetailResponse(*detail)
	c.JSON(http.StatusOK, resp)
}

// UpdateItem patches a draft quote item.
//
//	@Summary		Update a quote item
//	@Description	Patches a mutable field on a draft quote item.
//	@Tags			rfqs
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Branch-Id	header		string						true	"Active branch"
//	@Param			rfqId		path		string						true	"RFQ id"
//	@Param			quoteId		path		string						true	"Quote id"
//	@Param			itemId		path		string						true	"Item id"
//	@Param			body		body		dto.UpdateQuoteItemRequest	true	"Fields to patch"
//	@Success		200			{object}	dto.QuoteItemResponse
//	@Failure		400			{object}	dto.ErrorResponse
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		404			{object}	dto.ErrorResponse
//	@Failure		409			{object}	dto.ErrorResponse
//	@Router			/v1/quotes/{quoteId}/items/{itemId} [patch]
func (h *RfqHandler) UpdateItem(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	quoteID, ok := pathUUID(c, "quoteId")
	if !ok {
		return
	}
	itemID, ok := pathUUID(c, "itemId")
	if !ok {
		return
	}

	var req dto.UpdateQuoteItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}

	in := domain.QuoteItemUpdate{
		ProductID:            req.ProductID,
		RequestedDescription: req.RequestedDescription,
		Unit:                 req.Unit,
	}
	if req.Quantity != nil {
		qty, qtyErr := decimalFromString(*req.Quantity)
		if qtyErr != nil {
			Respond(c, fmt.Errorf("%w: quantity must be a valid decimal", domain.ErrInvalidInput))
			return
		}
		in.Quantity = &qty
	}
	if req.UnitPriceSnapshot != nil {
		price, priceErr := decimalFromString(*req.UnitPriceSnapshot)
		if priceErr != nil {
			Respond(c, fmt.Errorf("%w: unit_price_snapshot must be a valid decimal",
				domain.ErrInvalidInput))
			return
		}
		in.UnitPriceSnapshot = &price
	}

	item, err := h.rfqs.UpdateItem(c.Request.Context(), tenant, quoteID, itemID, in)
	if err != nil {
		Respond(c, err)
		return
	}

	resp := toQuoteItemResponse(*item, nil, nil)
	c.JSON(http.StatusOK, resp)
}

// DeleteItem removes a draft quote item.
//
//	@Summary		Delete a quote item
//	@Description	Removes a line from a draft quote version.
//	@Tags			rfqs
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Branch-Id	header	string	true	"Active branch"
//	@Param			rfqId		path	string	true	"RFQ id"
//	@Param			quoteId		path	string	true	"Quote id"
//	@Param			itemId		path	string	true	"Item id"
//	@Success		200			{object}	dto.SuccessResponse
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		404			{object}	dto.ErrorResponse
//	@Failure		409			{object}	dto.ErrorResponse
//	@Router			/v1/quotes/{quoteId}/items/{itemId} [delete]
func (h *RfqHandler) DeleteItem(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	quoteID, ok := pathUUID(c, "quoteId")
	if !ok {
		return
	}
	itemID, ok := pathUUID(c, "itemId")
	if !ok {
		return
	}

	if err := h.rfqs.DeleteItem(c.Request.Context(), tenant, quoteID, itemID); err != nil {
		Respond(c, err)
		return
	}

	c.JSON(http.StatusOK, dto.SuccessResponse{OK: true})
}

// AddItem appends one material line to a draft quote version.
//
//	@Summary		Add a quote item
//	@Description	Appends a new material line to a draft quote version.
//	@Tags			rfqs
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Branch-Id	header		string						true	"Active branch"
//	@Param			rfqId		path		string						true	"RFQ id"
//	@Param			quoteId		path		string						true	"Quote id"
//	@Param			body		body		dto.AddQuoteItemRequest		true	"New item data"
//	@Success		201			{object}	dto.QuoteItemResponse
//	@Failure		400			{object}	dto.ErrorResponse
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		404			{object}	dto.ErrorResponse
//	@Failure		409			{object}	dto.ErrorResponse
//	@Router			/v1/quotes/{quoteId}/items [post]
func (h *RfqHandler) AddItem(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	quoteID, ok := pathUUID(c, "quoteId")
	if !ok {
		return
	}

	var req dto.AddQuoteItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}

	quantity, err := decimalFromString(req.Quantity)
	if err != nil {
		Respond(c, fmt.Errorf("%w: quantity must be a valid decimal", domain.ErrInvalidInput))
		return
	}

	in := domain.QuoteItemCreate{
		ProductID:            req.ProductID,
		RequestedDescription: req.RequestedDescription,
		Quantity:             quantity,
		Unit:                 req.Unit,
	}

	item, err := h.rfqs.AddItem(c.Request.Context(), tenant, quoteID, in)
	if err != nil {
		Respond(c, err)
		return
	}

	c.JSON(http.StatusCreated, toQuoteItemResponse(*item, nil, nil))
}

// AddDiscount applies a seller-typed discount to the quote's current version.
//
//	@Summary		Add a discount
//	@Description	Applies a seller-typed discount — fixed amount or percentage, scoped to the total, one item, or a set — and recomputes the total.
//	@Tags			rfqs
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Branch-Id	header		string						true	"Active branch"
//	@Param			quoteId		path		string						true	"Quote id"
//	@Param			body		body		dto.CreateDiscountRequest	true	"New discount data"
//	@Success		201			{object}	dto.QuoteDiscountResponse
//	@Failure		400			{object}	dto.ErrorResponse
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		404			{object}	dto.ErrorResponse
//	@Failure		409			{object}	dto.ErrorResponse
//	@Router			/v1/quotes/{quoteId}/discounts [post]
func (h *RfqHandler) AddDiscount(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	quoteID, ok := pathUUID(c, "quoteId")
	if !ok {
		return
	}

	var req dto.CreateDiscountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}

	value, err := decimalFromString(req.Value)
	if err != nil {
		Respond(c, fmt.Errorf("%w: value must be a valid decimal", domain.ErrInvalidInput))
		return
	}
	itemIDs, err := parseUUIDList(req.ItemIDs)
	if err != nil {
		Respond(c, fmt.Errorf("%w: item_ids must contain valid UUIDs", domain.ErrInvalidInput))
		return
	}

	discount, err := h.rfqs.AddDiscount(c.Request.Context(), tenant, quoteID,
		domain.QuoteDiscountCreate{
			Description: req.Description,
			ActionType:  domain.PromotionActionType(req.ActionType),
			Value:       value,
			Scope:       domain.DiscountScope(req.Scope),
			ItemIDs:     itemIDs,
		})
	if err != nil {
		Respond(c, err)
		return
	}

	c.JSON(http.StatusCreated, toQuoteDiscountResponse(*discount))
}

// UpdateDiscount patches a quote discount.
//
//	@Summary		Update a discount
//	@Description	Patches a discount's rule (value, action type, scope, covered items), description, or suppressed flag.
//	@Tags			rfqs
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Branch-Id	header		string						true	"Active branch"
//	@Param			quoteId		path		string						true	"Quote id"
//	@Param			discountId	path		string						true	"Discount id"
//	@Param			body		body		dto.UpdateDiscountRequest	true	"Fields to patch"
//	@Success		200			{object}	dto.QuoteDiscountResponse
//	@Failure		400			{object}	dto.ErrorResponse
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		404			{object}	dto.ErrorResponse
//	@Failure		409			{object}	dto.ErrorResponse
//	@Router			/v1/quotes/{quoteId}/discounts/{discountId} [patch]
func (h *RfqHandler) UpdateDiscount(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	quoteID, ok := pathUUID(c, "quoteId")
	if !ok {
		return
	}
	discountID, ok := pathUUID(c, "discountId")
	if !ok {
		return
	}

	var req dto.UpdateDiscountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}

	in := domain.QuoteDiscountUpdate{
		Description:        req.Description,
		SuppressedBySeller: req.SuppressedBySeller,
	}
	if req.Value != nil {
		value, valueErr := decimalFromString(*req.Value)
		if valueErr != nil {
			Respond(c, fmt.Errorf("%w: value must be a valid decimal", domain.ErrInvalidInput))
			return
		}
		in.Value = &value
	}
	if req.ActionType != nil {
		actionType := domain.PromotionActionType(*req.ActionType)
		in.ActionType = &actionType
	}
	if req.Scope != nil {
		scope := domain.DiscountScope(*req.Scope)
		in.Scope = &scope
	}
	if len(req.ItemIDs) > 0 {
		itemIDs, itemErr := parseUUIDList(req.ItemIDs)
		if itemErr != nil {
			Respond(c, fmt.Errorf("%w: item_ids must contain valid UUIDs", domain.ErrInvalidInput))
			return
		}
		in.ItemIDs = itemIDs
	}

	discount, err := h.rfqs.UpdateDiscount(c.Request.Context(), tenant, quoteID, discountID, in)
	if err != nil {
		Respond(c, err)
		return
	}

	c.JSON(http.StatusOK, toQuoteDiscountResponse(*discount))
}

// DeleteDiscount removes a seller-typed quote discount.
//
//	@Summary		Delete a discount
//	@Description	Removes one seller-typed discount application and recomputes the total.
//	@Tags			rfqs
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Branch-Id	header	string	true	"Active branch"
//	@Param			quoteId		path	string	true	"Quote id"
//	@Param			discountId	path	string	true	"Discount id"
//	@Success		200			{object}	dto.SuccessResponse
//	@Failure		400			{object}	dto.ErrorResponse
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		404			{object}	dto.ErrorResponse
//	@Failure		409			{object}	dto.ErrorResponse
//	@Router			/v1/quotes/{quoteId}/discounts/{discountId} [delete]
func (h *RfqHandler) DeleteDiscount(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	quoteID, ok := pathUUID(c, "quoteId")
	if !ok {
		return
	}
	discountID, ok := pathUUID(c, "discountId")
	if !ok {
		return
	}

	if err := h.rfqs.DeleteDiscount(c.Request.Context(), tenant, quoteID, discountID); err != nil {
		Respond(c, err)
		return
	}

	c.JSON(http.StatusOK, dto.SuccessResponse{OK: true})
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
		ID:            item.ID,
		ClientID:      item.ClientID,
		Client:        item.ClientLabel,
		CreatedAt:     item.CreatedAt,
		Channel:       item.Channel,
		SellerID:      item.SellerID,
		Seller:        item.SellerName,
		BranchID:      item.BranchID,
		Branch:        item.BranchName,
		ItemCount:     item.ItemCount,
		Total:         item.Total,
		Status:        item.Status,
		ArchivedAt:    item.ArchivedAt,
		NeedsFollowup: item.NeedsFollowup,
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

func decimalFromString(s string) (decimal.Decimal, error) {
	return decimal.NewFromString(s)
}

func toRfqDetailResponse(detail domain.RfqDetail) dto.RfqDetailResponse {
	resp := dto.RfqDetailResponse{
		Rfq:          toListItemResponse(detail.Rfq),
		RFQHistory:   toRFQStatusChangeResponses(detail.RFQStatusChanges),
		QuoteHistory: toQuoteStatusChangeResponses(detail.QuoteStatusChanges),
		Deliveries:   toQuoteSendTrackingResponses(detail.Deliveries),
	}

	if detail.Quote != nil {
		quoteResp := toQuoteResponse(*detail.Quote)
		resp.Quote = &quoteResp
	}

	if detail.Version != nil {
		versionResp := toQuoteVersionResponse(*detail.Version)
		resp.Version = &versionResp
	}

	resp.Items = make([]dto.QuoteItemResponse, 0, len(detail.Items))
	for _, item := range detail.Items {
		resp.Items = append(resp.Items, toQuoteItemResponse(item, detail.Alternatives[item.ID], nil))
	}

	resp.Alternatives = make(map[string][]dto.QuoteItemAlternativeResponse, len(detail.Alternatives))
	for itemID, alts := range detail.Alternatives {
		key := itemID.String()
		resp.Alternatives[key] = toQuoteItemAlternativeResponses(alts)
	}

	resp.Discounts = make([]dto.QuoteDiscountResponse, 0, len(detail.Discounts))
	for _, discount := range detail.Discounts {
		resp.Discounts = append(resp.Discounts, toQuoteDiscountResponse(discount))
	}

	if detail.ChangesRequested != nil {
		diff := toChangeRequestDiffResponse(*detail.ChangesRequested)
		resp.ChangesRequested = &diff
	}

	return resp
}

func toChangeRequestDiffResponse(diff domain.ChangeRequestDiff) dto.ChangeRequestDiffResponse {
	return dto.ChangeRequestDiffResponse{
		Reason:    diff.Reason,
		Original:  toChangeRequestSideResponse(diff.Original),
		Requested: toChangeRequestSideResponse(diff.Requested),
	}
}

func toChangeRequestSideResponse(side domain.ChangeRequestSide) dto.ChangeRequestSideResponse {
	lineItems := make([]dto.DiffLineItemResponse, 0, len(side.Items))
	for _, item := range side.Items {
		lineItems = append(lineItems, dto.DiffLineItemResponse{
			Description: item.Description,
			Quantity:    item.Quantity,
			Unit:        item.Unit,
			UnitPrice:   item.UnitPrice,
			Changed:     item.Changed,
			ChangeType:  changeTypePtr(item.ChangeType),
		})
	}
	discountLines := make([]dto.DiffDiscountLineResponse, 0, len(side.Discounts))
	for _, discount := range side.Discounts {
		discountLines = append(discountLines, dto.DiffDiscountLineResponse{
			Name:    discount.Name,
			Amount:  discount.Amount,
			Changed: discount.Changed,
		})
	}
	return dto.ChangeRequestSideResponse{
		Items:     lineItems,
		Discounts: discountLines,
		Total:     side.Total.StringFixed(domain.MoneyScale),
	}
}

func changeTypePtr(changeType string) *string {
	if changeType == "" {
		return nil
	}
	return &changeType
}

func toQuoteDiscountResponse(discount domain.QuoteDiscount) dto.QuoteDiscountResponse {
	response := dto.QuoteDiscountResponse{
		ID:                 discount.ID,
		QuoteVersionID:     discount.QuoteVersionID,
		PromotionID:        discount.PromotionID,
		PromotionName:      discount.PromotionName,
		ConditionType:      string(discount.ConditionType),
		Scope:              string(discount.Scope),
		Origin:             string(discount.Origin),
		Amount:             discount.Amount.StringFixed(domain.MoneyScale),
		ActionType:         string(discount.ActionType),
		Description:        discount.Description,
		SuppressedBySeller: discount.SuppressedBySeller,
		CreatedAt:          discount.CreatedAt,
	}
	response.ItemIDs = make([]uuid.UUID, 0, len(discount.ItemIDs))
	response.ItemIDs = append(response.ItemIDs, discount.ItemIDs...)
	if discount.ActionValue != nil {
		value := discount.ActionValue.StringFixed(domain.MoneyScale)
		response.ActionValue = &value
	}
	return response
}

// parseUUIDList turns the item_ids a discount payload carries into UUIDs.
func parseUUIDList(in []string) ([]uuid.UUID, error) {
	ids := make([]uuid.UUID, 0, len(in))
	for _, raw := range in {
		id, err := uuid.Parse(strings.TrimSpace(raw))
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func toRFQStatusChangeResponses(
	changes []domain.RFQStatusChange,
) []dto.RFQStatusChangeResponse {
	responses := make([]dto.RFQStatusChangeResponse, 0, len(changes))
	for _, change := range changes {
		var previous *string
		if change.PreviousStatus != nil {
			value := string(*change.PreviousStatus)
			previous = &value
		}
		responses = append(responses, dto.RFQStatusChangeResponse{
			ID: change.ID, RFQID: change.RFQID, PreviousStatus: previous,
			NewStatus: string(change.NewStatus), UserID: change.UserID,
			ChangedAt: change.ChangedAt, CreatedAt: change.CreatedAt,
		})
	}
	return responses
}

func toQuoteStatusChangeResponses(
	changes []domain.QuoteStatusChange,
) []dto.QuoteStatusChangeResponse {
	responses := make([]dto.QuoteStatusChangeResponse, 0, len(changes))
	for _, change := range changes {
		var previous *string
		if change.PreviousStatus != nil {
			value := string(*change.PreviousStatus)
			previous = &value
		}
		responses = append(responses, dto.QuoteStatusChangeResponse{
			ID: change.ID, QuoteID: change.QuoteID, PreviousStatus: previous,
			NewStatus: string(change.NewStatus), UserID: change.UserID,
			ChangedAt: change.ChangedAt, CreatedAt: change.CreatedAt,
		})
	}
	return responses
}

func toQuoteSendTrackingResponses(sends []domain.QuoteSend) []dto.QuoteSendTrackingResponse {
	responses := make([]dto.QuoteSendTrackingResponse, 0, len(sends))
	for _, send := range sends {
		responses = append(responses, dto.QuoteSendTrackingResponse{
			ID: send.ID, VersionID: send.VersionID, Channel: string(send.ChannelType),
			Destination: send.Destination, Format: string(send.Format),
			TrackingStatus: string(send.TrackingStatus), SentAt: send.SentAt,
			ExpiresAt: send.ExpiresAt, CreatedAt: send.CreatedAt,
		})
	}
	return responses
}
