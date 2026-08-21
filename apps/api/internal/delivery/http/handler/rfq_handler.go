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

// RfqService is the request-to-quote surface the handler needs.
type RfqService interface {
	List(ctx context.Context, tenant domain.Tenant) ([]domain.RfqListItem, error)
	CreateManual(ctx context.Context, tenant domain.Tenant, in domain.NewRfq) (*domain.RfqCreation, error)
}

// RfqHandler serves RFQ intake.
type RfqHandler struct {
	rfqs RfqService
}

// NewRfqHandler builds an RfqHandler.
func NewRfqHandler(rfqs RfqService) *RfqHandler {
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
//	@Description	Records a counter or phone order. The RFQ is born GENERATED and its quote DRAFT in one transaction; typed lines become the quote's first version. Needs an active branch in X-Branch-Id.
//	@Tags			rfqs
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Branch-Id	header		string				true	"Active branch"
//	@Param			request		body		dto.CreateRfqRequest	true	"Manual RFQ"
//	@Success		201			{object}	dto.CreateRfqResponse
//	@Failure		400			{object}	dto.ErrorResponse
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		404			{object}	dto.ErrorResponse	"Branch has no manual-entry channel, or an item names a product outside the account"
//	@Failure		422			{object}	dto.ErrorResponse	"No active branch, no raw_text and no items, or a quantity NUMERIC(14,2) cannot hold"
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

func toRfqResponse(r domain.Rfq) dto.RfqResponse {
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

func toQuoteResponse(q domain.Quote) dto.QuoteResponse {
	return dto.QuoteResponse{
		ID:               q.ID,
		BranchID:         q.BranchID,
		RfqID:            q.RfqID,
		SellerID:         q.SellerID,
		CurrentVersionID: q.CurrentVersionID,
		CurrentStatus:    string(q.CurrentStatus),
		CreatedAt:        q.CreatedAt,
		UpdatedAt:        q.UpdatedAt,
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
