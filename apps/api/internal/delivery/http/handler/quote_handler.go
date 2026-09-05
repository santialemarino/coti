package handler

import (
	"context"
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
}

// QuoteDeliveryService is the send and public-token surface the handler needs.
type QuoteDeliveryService interface {
	Send(ctx context.Context, tenant domain.Tenant, quoteID uuid.UUID,
		in domain.QuoteDeliveryInput) (*domain.QuoteDeliveryResult, error)
	ResolvePublic(ctx context.Context, token string) (*domain.PublicQuoteSend, error)
}

// QuoteHandler serves the quote lifecycle endpoints.
type QuoteHandler struct {
	quotes   QuoteService
	delivery QuoteDeliveryService
}

// NewQuoteHandler builds a QuoteHandler.
func NewQuoteHandler(quotes QuoteService, delivery QuoteDeliveryService) *QuoteHandler {
	return &QuoteHandler{quotes: quotes, delivery: delivery}
}

// Send delivers a frozen quote through WhatsApp and, when requested, email.
//
//	@Summary		Send a quote to its client
//	@Description	Freezes the current seller-approved version, always attempts WhatsApp with a public webapp link, optionally attempts email independently, and moves QUOTED to SENT when at least one selected channel succeeds.
//	@Tags			quotes
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Branch-Id	header		string				true	"Active branch"
//	@Param			Idempotency-Key	header		string				true	"Delivery operation UUID"
//	@Param			quoteId		path		string				true	"Quote id"
//	@Param			request		body		dto.QuoteSendRequest	true	"Delivery destinations and validity"
//	@Success		201			{object}	dto.QuoteSendResponse
//	@Success		200			{object}	dto.QuoteSendResponse	"Idempotent replay"
//	@Failure		400			{object}	dto.ErrorResponse
//	@Failure		404			{object}	dto.ErrorResponse
//	@Failure		409			{object}	dto.ErrorResponse
//	@Failure		422			{object}	dto.ErrorResponse
//	@Failure		503			{object}	dto.ErrorResponse	"Every selected channel failed"
//	@Router			/v1/quotes/{quoteId}/sends [post]
func (h *QuoteHandler) Send(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	quoteID, ok := pathUUID(c, "quoteId")
	if !ok {
		return
	}
	key, err := uuid.Parse(c.GetHeader("Idempotency-Key"))
	if err != nil {
		RespondBindError(c, err)
		return
	}
	var request dto.QuoteSendRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		RespondBindError(c, err)
		return
	}
	if h.delivery == nil {
		Respond(c, domain.ErrNotConfigured)
		return
	}
	var email *string
	if request.EmailDelivery != nil {
		email = &request.EmailDelivery.Address
	}
	result, err := h.delivery.Send(c.Request.Context(), tenant, quoteID,
		domain.QuoteDeliveryInput{IdempotencyKey: key, Phone: request.RecipientPhone,
			Email: email, ExpiryDays: request.ExpiryDays})
	if err != nil {
		Respond(c, err)
		return
	}
	status := http.StatusCreated
	if result.Replay {
		status = http.StatusOK
	}
	c.JSON(status, toQuoteSendResponse(*result))
}

// ResolvePublic reports whether a completed delivery token is active or expired.
//
//	@Summary	Resolve a public quote delivery token
//	@Tags		public quote sends
//	@Produce	json
//	@Param		token	path		string	true	"Public delivery token"
//	@Success	200		{object}	dto.PublicQuoteSendResponse
//	@Failure	404		{object}	dto.ErrorResponse
//	@Router		/v1/public/quote-sends/{token} [get]
func (h *QuoteHandler) ResolvePublic(c *gin.Context) {
	if h.delivery == nil {
		Respond(c, domain.ErrNotFound)
		return
	}
	result, err := h.delivery.ResolvePublic(c.Request.Context(), c.Param("token"))
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, dto.PublicQuoteSendResponse{Status: result.Status,
		ExpiresAt: result.ExpiresAt})
}

func toQuoteSendResponse(result domain.QuoteDeliveryResult) dto.QuoteSendResponse {
	deliveries := make([]dto.QuoteDeliveryResponse, 0, len(result.Deliveries))
	for _, delivery := range result.Deliveries {
		deliveries = append(deliveries, dto.QuoteDeliveryResponse{ID: delivery.ID,
			Channel: string(delivery.ChannelType), Destination: delivery.Destination,
			TrackingStatus: string(delivery.TrackingStatus), PublicURL: delivery.PublicURL,
			SentAt: delivery.SentAt})
	}
	return dto.QuoteSendResponse{QuoteID: result.QuoteID, VersionID: result.VersionID,
		CurrentStatus: string(result.CurrentStatus), ExpiresAt: result.ExpiresAt,
		Deliveries: deliveries}
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
