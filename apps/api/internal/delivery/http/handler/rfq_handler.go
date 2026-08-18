package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// RFQService is the RFQ surface the handler needs.
type RFQService interface {
	CreateTextDraft(ctx context.Context, tenant domain.Tenant, in domain.TextRFQDraftInput) (*domain.TextRFQDraft, error)
	CreateWhatsAppMockDraft(ctx context.Context, tenant domain.Tenant, in domain.WhatsAppMockRFQInput) (*domain.TextRFQDraft, error)
}

// RFQHandler serves RFQ intake endpoints.
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
//	@Description	Persists the original text before reading it, then returns a quote DRAFT with one line per material, each carrying its catalog match and confidence. It does not price or send the quote.
//	@Tags			rfq
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Branch-Id	header		string							true	"Active branch"
//	@Param			body		body		dto.CreateTextRFQDraftRequest	true	"RFQ text to process"
//	@Success		201			{object}	dto.TextRFQDraftResponse
//	@Failure		400			{object}	dto.ErrorResponse
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		422			{object}	dto.ErrorResponse	"No active branch, or an answer the model could not shape"
//	@Failure		503			{object}	dto.ErrorResponse	"No language model is bound, or it could not answer"
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
//	@Failure		404			{object}	dto.ErrorResponse	"No active WhatsApp channel"
//	@Failure		422			{object}	dto.ErrorResponse	"Ambiguous channel, or an answer the model could not shape"
//	@Failure		503			{object}	dto.ErrorResponse	"No language model is bound, or it could not answer"
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
		items = append(items, toQuoteItemResponse(item))
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
		RFQ:     toRFQResponse(draft.RFQ),
		Quote:   quote,
		Version: version,
		Items:   items,
	}
}

func toRFQResponse(rfq domain.RFQ) dto.RFQResponse {
	return dto.RFQResponse{
		ID: rfq.ID, BranchID: rfq.BranchID, ClientID: rfq.ClientID, ChannelID: rfq.ChannelID,
		RawText: rfq.RawText, Status: string(rfq.Status), WorkType: rfq.WorkType,
		ClientLabel: rfq.ClientLabel, ReceivedAt: rfq.ReceivedAt, CreatedAt: rfq.CreatedAt,
		UpdatedAt: rfq.UpdatedAt,
	}
}

func toQuoteResponse(quote domain.Quote) dto.QuoteResponse {
	return dto.QuoteResponse{
		ID: quote.ID, BranchID: quote.BranchID, ClientID: quote.ClientID, RFQID: quote.RFQID,
		SellerID: quote.SellerID, CurrentVersionID: quote.CurrentVersionID,
		CurrentStatus: string(quote.CurrentStatus), ExpiresAt: quote.ExpiresAt,
		ArchivedAt: quote.ArchivedAt, NeedsFollowup: quote.NeedsFollowup,
		FollowupFlaggedAt: quote.FollowupFlaggedAt, CreatedAt: quote.CreatedAt,
		UpdatedAt: quote.UpdatedAt,
	}
}

func toQuoteVersionResponse(version domain.QuoteVersion) dto.QuoteVersionResponse {
	return dto.QuoteVersionResponse{
		ID: version.ID, QuoteID: version.QuoteID, AuthorID: version.AuthorID,
		VersionNumber: version.VersionNumber, Total: version.Total.StringFixed(domain.MoneyScale),
		IsImmutable: version.IsImmutable, Comment: version.Comment, CreatedAt: version.CreatedAt,
	}
}

func toQuoteItemResponse(item domain.QuoteItem) dto.QuoteItemResponse {
	return dto.QuoteItemResponse{
		ID: item.ID, VersionID: item.VersionID, ProductID: item.ProductID,
		RequestedDescription: item.RequestedDescription,
		Quantity:             item.Quantity.StringFixed(domain.MoneyScale),
		Unit:                 item.Unit,
		UnitPriceSnapshot:    amountString(item.UnitPriceSnapshot),
		MinPriceSnapshot:     amountString(item.MinPriceSnapshot),
		Subtotal:             amountString(item.Subtotal),
		ConfidenceScore:      confidenceString(item.ConfidenceScore),
		MatchStatus:          string(item.MatchStatus),
		QuantityRationale:    item.QuantityRationale,
		CreatedAt:            item.CreatedAt,
	}
}

func confidenceString(confidence decimal.NullDecimal) *string {
	if !confidence.Valid {
		return nil
	}
	value := confidence.Decimal.StringFixed(4)
	return &value
}
