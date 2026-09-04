package handler

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// QuoteService is the quote lifecycle surface the handler needs.
type QuoteService interface {
	AcceptMaterials(ctx context.Context, tenant domain.Tenant, quoteID uuid.UUID) (*domain.PricedQuote, error)
	Transition(ctx context.Context, tenant domain.Tenant, quoteID uuid.UUID, to domain.QuoteStatus) (*domain.Quote, error)
	Archive(ctx context.Context, tenant domain.Tenant, quoteID uuid.UUID) (*domain.Quote, error)
	Unarchive(ctx context.Context, tenant domain.Tenant, quoteID uuid.UUID) (*domain.Quote, error)
}

// QuoteHandler serves the quote lifecycle endpoints.
type QuoteHandler struct {
	quotes QuoteService
}

// NewQuoteHandler builds a QuoteHandler.
func NewQuoteHandler(quotes QuoteService) *QuoteHandler {
	return &QuoteHandler{quotes: quotes}
}

// AcceptMaterials prices a draft quote's materials and moves it to QUOTED.
//
//	@Summary		Accept a quote's materials
//	@Description	Freezes each line's unit price and discount floor from the prices in force at the quote's branch, sums the version total, and moves the quote to QUOTED for review. A line with no matched product, or one whose product the branch cannot price, stays in the quote with an empty valuation and adds nothing to the total; the second is named by pricing_unavailable, which every line answers once valued. The version is not frozen: the seller still edits it.
//	@Tags			quotes
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Branch-Id	header		string	true	"Active branch"
//	@Param			quoteId		path		string	true	"Quote id"
//	@Success		200			{object}	dto.PricedQuoteResponse
//	@Failure		400			{object}	dto.ErrorResponse
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		404			{object}	dto.ErrorResponse	"No such quote in the selected branch"
//	@Failure		409			{object}	dto.ErrorResponse	"QUOTE_NOT_DRAFT once the materials were accepted, QUOTE_ARCHIVED on an archived quote"
//	@Failure		422			{object}	dto.ErrorResponse	"No active branch, or a total NUMERIC(14,2) cannot hold"
//	@Router			/v1/quotes/{quoteId}/accept-materials [post]
func (h *QuoteHandler) AcceptMaterials(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	quoteID, ok := pathUUID(c, "quoteId")
	if !ok {
		return
	}

	priced, err := h.quotes.AcceptMaterials(c.Request.Context(), tenant, quoteID)
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, toPricedQuoteResponse(*priced))
}

// Transition moves a quote along a seller-action edge of the state machine.
//
//	@Summary		Transition a quote's status
//	@Description	Moves the quote to the asked status, refusing a status its current one cannot
//	@Description	reach or an archived quote. Records the move in the status history.
//	@Tags			quotes
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Branch-Id	header		string						true	"Active branch"
//	@Param			quoteId		path		string						true	"Quote id"
//	@Param			body		body		dto.TransitionQuoteRequest	true	"Target status"
//	@Success		200			{object}	dto.QuoteResponse
//	@Failure		400			{object}	dto.ErrorResponse
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		404			{object}	dto.ErrorResponse	"No such quote in the selected branch"
//	@Failure		409			{object}	dto.ErrorResponse	"QUOTE_NOT_DRAFT when the edge is not allowed, QUOTE_ARCHIVED on an archived quote"
//	@Failure		422			{object}	dto.ErrorResponse	"No active branch, or an unknown status"
//	@Router			/v1/quotes/{quoteId}/transition [post]
func (h *QuoteHandler) Transition(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	quoteID, ok := pathUUID(c, "quoteId")
	if !ok {
		return
	}
	var body dto.TransitionQuoteRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondBindError(c, err)
		return
	}
	to, err := parseQuoteStatus(body.Status)
	if err != nil {
		Respond(c, err)
		return
	}

	quote, err := h.quotes.Transition(c.Request.Context(), tenant, quoteID, to)
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, toQuoteResponse(*quote))
}

// Archive boxes a quote away without changing its status.
//
//	@Summary		Archive a quote
//	@Description	Sets the archived flag. Refuses an archived quote and a terminal one.
//	@Tags			quotes
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Branch-Id	header		string	true	"Active branch"
//	@Param			quoteId		path		string	true	"Quote id"
//	@Success		200			{object}	dto.QuoteResponse
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		404			{object}	dto.ErrorResponse	"No such quote in the selected branch"
//	@Failure		409			{object}	dto.ErrorResponse	"QUOTE_ARCHIVED or a terminal status"
//	@Failure		422			{object}	dto.ErrorResponse	"No active branch"
//	@Router			/v1/quotes/{quoteId}/archive [post]
func (h *QuoteHandler) Archive(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	quoteID, ok := pathUUID(c, "quoteId")
	if !ok {
		return
	}

	quote, err := h.quotes.Archive(c.Request.Context(), tenant, quoteID)
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, toQuoteResponse(*quote))
}

// Unarchive brings a boxed-away quote back to the list.
//
//	@Summary		Unarchive a quote
//	@Description	Clears the archived flag.
//	@Tags			quotes
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Branch-Id	header		string	true	"Active branch"
//	@Param			quoteId		path		string	true	"Quote id"
//	@Success		200			{object}	dto.QuoteResponse
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		404			{object}	dto.ErrorResponse	"No such quote or not archived"
//	@Failure		422			{object}	dto.ErrorResponse	"No active branch"
//	@Router			/v1/quotes/{quoteId}/unarchive [post]
func (h *QuoteHandler) Unarchive(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	quoteID, ok := pathUUID(c, "quoteId")
	if !ok {
		return
	}

	quote, err := h.quotes.Unarchive(c.Request.Context(), tenant, quoteID)
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, toQuoteResponse(*quote))
}

func parseQuoteStatus(raw string) (domain.QuoteStatus, error) {
	switch domain.QuoteStatus(raw) {
	case domain.QuoteStatusSent, domain.QuoteStatusAccepted, domain.QuoteStatusRejected,
		domain.QuoteStatusChangeRequested:
		return domain.QuoteStatus(raw), nil
	default:
		return "", fmt.Errorf("%w: %q is not a status a seller-action edge may move to",
			domain.ErrInvalidInput, raw)
	}
}

func toPricedQuoteResponse(priced domain.PricedQuote) dto.PricedQuoteResponse {
	unpriced := make(map[uuid.UUID]struct{}, len(priced.UnpricedItemIDs))
	for _, itemID := range priced.UnpricedItemIDs {
		unpriced[itemID] = struct{}{}
	}
	items := make([]dto.QuoteItemResponse, 0, len(priced.Items))
	for _, item := range priced.Items {
		// Valuation has run, so the gap is answered either way rather than left null: true names a
		// line the branch cannot price, false a line that was priced or had nothing to price.
		_, gap := unpriced[item.ID]
		items = append(items, toQuoteItemResponse(item, priced.Alternatives[item.ID], &gap))
	}
	return dto.PricedQuoteResponse{
		Quote:   toQuoteResponse(priced.Quote),
		Version: toQuoteVersionResponse(priced.Version),
		Items:   items,
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

// toQuoteItemResponse maps one line. pricingUnavailable is nil where nothing has been valued yet,
// which is a different answer from a line the branch can price.
func toQuoteItemResponse(
	item domain.QuoteItem, alternatives []domain.QuoteItemAlternative, pricingUnavailable *bool,
) dto.QuoteItemResponse {
	return dto.QuoteItemResponse{
		ID: item.ID, VersionID: item.VersionID, ProductID: item.ProductID,
		ProductCode: item.ProductCode, ProductName: item.ProductName,
		ProductUnit:          item.ProductUnit,
		RequestedDescription: item.RequestedDescription,
		Quantity:             item.Quantity.StringFixed(domain.MoneyScale),
		Unit:                 item.Unit,
		UnitPriceSnapshot:    amountString(item.UnitPriceSnapshot),
		MinPriceSnapshot:     amountString(item.MinPriceSnapshot),
		Subtotal:             amountString(item.Subtotal),
		ConfidenceScore:      confidenceString(item.ConfidenceScore),
		MatchStatus:          string(item.MatchStatus),
		Alternatives:         toQuoteItemAlternativeResponses(alternatives),
		PricingUnavailable:   pricingUnavailable,
		QuantityRationale:    item.QuantityRationale,
		CreatedAt:            item.CreatedAt,
	}
}

func toQuoteItemAlternativeResponses(
	alternatives []domain.QuoteItemAlternative,
) []dto.QuoteItemAlternativeResponse {
	responses := make([]dto.QuoteItemAlternativeResponse, 0, len(alternatives))
	for _, alternative := range alternatives {
		responses = append(responses, dto.QuoteItemAlternativeResponse{
			ID:               alternative.ID,
			ProductID:        alternative.ProductID,
			ComboID:          alternative.ComboID,
			Type:             string(alternative.Type),
			Origin:           string(alternative.Origin),
			Rank:             alternative.Rank,
			ConfidenceScore:  confidenceString(alternative.ConfidenceScore),
			PriceSnapshot:    amountString(alternative.PriceSnapshot),
			ApprovedBySeller: alternative.ApprovedBySeller,
			ChosenByClient:   alternative.ChosenByClient,
			Code:             alternative.Code,
			CanonicalName:    alternative.CanonicalName,
			Unit:             alternative.Unit,
		})
	}
	return responses
}

func confidenceString(confidence decimal.NullDecimal) *string {
	if !confidence.Valid {
		return nil
	}
	value := confidence.Decimal.StringFixed(4)
	return &value
}
