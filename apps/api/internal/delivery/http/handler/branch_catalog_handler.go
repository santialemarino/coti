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

// BranchCatalogService is the per-branch catalog surface the handler needs.
type BranchCatalogService interface {
	ListAvailability(ctx context.Context, tenant domain.Tenant, productID uuid.UUID) ([]domain.BranchProduct, error)
	SetAvailability(ctx context.Context, tenant domain.Tenant, productID uuid.UUID, in domain.BranchAvailability) (*domain.BranchProduct, error)
	ListPrices(ctx context.Context, tenant domain.Tenant, productID uuid.UUID) ([]domain.ProductPrice, error)
	SetPrice(ctx context.Context, tenant domain.Tenant, productID uuid.UUID, in domain.NewProductPrice) (*domain.ProductPrice, error)
}

// BranchCatalogHandler serves per-branch availability and pricing for catalog products.
type BranchCatalogHandler struct {
	catalog BranchCatalogService
}

// NewBranchCatalogHandler builds a BranchCatalogHandler.
func NewBranchCatalogHandler(catalog BranchCatalogService) *BranchCatalogHandler {
	return &BranchCatalogHandler{catalog: catalog}
}

// ListAvailability returns where the product is sold and with how much stock.
//
//	@Summary		List per-branch availability
//	@Description	Returns the active branch's row when the request carries X-Branch-Id, and
//	@Description	every branch of the account when it does not.
//	@Tags			catalog
//	@Produce		json
//	@Security		BearerAuth
//	@Param			productId	path		string	true	"Product id"
//	@Param			X-Branch-Id	header		string	false	"Active branch; omit to read every branch"
//	@Success		200			{object}	dto.AvailabilityListResponse
//	@Failure		400			{object}	dto.ErrorResponse
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		404			{object}	dto.ErrorResponse
//	@Router			/v1/products/{productId}/availability [get]
func (h *BranchCatalogHandler) ListAvailability(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	productID, ok := pathUUID(c, "productId")
	if !ok {
		return
	}

	availability, err := h.catalog.ListAvailability(c.Request.Context(), tenant, productID)
	if err != nil {
		Respond(c, err)
		return
	}

	items := make([]dto.AvailabilityResponse, 0, len(availability))
	for _, a := range availability {
		items = append(items, toAvailabilityResponse(a))
	}
	c.JSON(http.StatusOK, dto.AvailabilityListResponse{Items: items})
}

// SetAvailability records whether the active branch sells the product, and with how much stock.
//
//	@Summary		Set per-branch availability
//	@Description	Creates the branch's row or updates it. Deactivating is how a branch stops
//	@Description	offering an item the account still catalogs.
//	@Tags			catalog
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			productId	path		string						true	"Product id"
//	@Param			X-Branch-Id	header		string						true	"Active branch"
//	@Param			body		body		dto.SetAvailabilityRequest	true	"Availability to record"
//	@Success		200			{object}	dto.AvailabilityResponse
//	@Failure		400			{object}	dto.ErrorResponse
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		404			{object}	dto.ErrorResponse
//	@Failure		422			{object}	dto.ErrorResponse	"No active branch, or a stock NUMERIC(14,2) cannot hold"
//	@Router			/v1/products/{productId}/availability [put]
func (h *BranchCatalogHandler) SetAvailability(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	productID, ok := pathUUID(c, "productId")
	if !ok {
		return
	}

	var body dto.SetAvailabilityRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondBindError(c, err)
		return
	}

	stock, err := parseNullableAmount(body.Stock, "stock")
	if err != nil {
		RespondBindError(c, err)
		return
	}
	isActive := true
	if body.IsActive != nil {
		isActive = *body.IsActive
	}

	saved, err := h.catalog.SetAvailability(c.Request.Context(), tenant, productID,
		domain.BranchAvailability{Stock: stock, IsActive: isActive})
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, toAvailabilityResponse(*saved))
}

// ListPrices returns the product's price history.
//
//	@Summary		List per-branch price history
//	@Description	Every validity period, newest first, open and closed. Scoped to the active
//	@Description	branch when the request carries X-Branch-Id, account-wide otherwise.
//	@Tags			catalog
//	@Produce		json
//	@Security		BearerAuth
//	@Param			productId	path		string	true	"Product id"
//	@Param			X-Branch-Id	header		string	false	"Active branch; omit to read every branch"
//	@Success		200			{object}	dto.PriceListResponse
//	@Failure		400			{object}	dto.ErrorResponse
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		404			{object}	dto.ErrorResponse
//	@Router			/v1/products/{productId}/prices [get]
func (h *BranchCatalogHandler) ListPrices(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	productID, ok := pathUUID(c, "productId")
	if !ok {
		return
	}

	prices, err := h.catalog.ListPrices(c.Request.Context(), tenant, productID)
	if err != nil {
		Respond(c, err)
		return
	}

	items := make([]dto.PriceResponse, 0, len(prices))
	for _, p := range prices {
		items = append(items, toPriceResponse(p))
	}
	c.JSON(http.StatusOK, dto.PriceListResponse{Items: items})
}

// SetPrice prices the product at the active branch.
//
//	@Summary		Set a per-branch price
//	@Description	Opens a new validity period and closes the previous one at the same
//	@Description	instant. Nothing is overwritten, so the history of what was quoted when
//	@Description	survives. min_price is the floor the discount engine may not cross, so it
//	@Description	cannot exceed price.
//	@Tags			catalog
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			productId	path		string				true	"Product id"
//	@Param			X-Branch-Id	header		string				true	"Active branch"
//	@Param			body		body		dto.SetPriceRequest	true	"Price to put in force"
//	@Success		201			{object}	dto.PriceResponse
//	@Failure		400			{object}	dto.ErrorResponse
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		404			{object}	dto.ErrorResponse
//	@Failure		422			{object}	dto.ErrorResponse	"No active branch, min_price above price, or valid_from before the current period"
//	@Router			/v1/products/{productId}/prices [post]
func (h *BranchCatalogHandler) SetPrice(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	productID, ok := pathUUID(c, "productId")
	if !ok {
		return
	}

	var body dto.SetPriceRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondBindError(c, err)
		return
	}

	price, err := decimal.NewFromString(body.Price)
	if err != nil {
		RespondBindError(c, fmt.Errorf("price is not a decimal: %w", err))
		return
	}
	minPrice, err := parseNullableAmount(body.MinPrice, "min_price")
	if err != nil {
		RespondBindError(c, err)
		return
	}

	in := domain.NewProductPrice{
		Price:      price,
		Currency:   body.Currency,
		Conditions: body.Conditions,
		MinPrice:   minPrice,
	}
	if body.ValidFrom != nil {
		in.ValidFrom = *body.ValidFrom
	}

	created, err := h.catalog.SetPrice(c.Request.Context(), tenant, productID, in)
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusCreated, toPriceResponse(*created))
}

// parseNullableAmount turns an optional decimal string into a NullDecimal. An absent field
// stays invalid, which is how "not tracked" differs from zero.
func parseNullableAmount(raw *string, field string) (decimal.NullDecimal, error) {
	if raw == nil {
		return decimal.NullDecimal{}, nil
	}
	parsed, err := decimal.NewFromString(*raw)
	if err != nil {
		return decimal.NullDecimal{}, fmt.Errorf("%s is not a decimal: %w", field, err)
	}
	return decimal.NullDecimal{Decimal: parsed, Valid: true}, nil
}

// amountString renders a NUMERIC(14,2) column at its stored scale, or nil when the column
// is NULL. Money leaves the API as a string so no client parses it into a float.
func amountString(amount decimal.NullDecimal) *string {
	if !amount.Valid {
		return nil
	}
	s := amount.Decimal.StringFixed(domain.MoneyScale)
	return &s
}

func toAvailabilityResponse(a domain.BranchProduct) dto.AvailabilityResponse {
	return dto.AvailabilityResponse{
		ID:        a.ID,
		BranchID:  a.BranchID,
		ProductID: a.ProductID,
		Stock:     amountString(a.Stock),
		IsActive:  a.IsActive,
		CreatedAt: a.CreatedAt,
		UpdatedAt: a.UpdatedAt,
	}
}

func toPriceResponse(p domain.ProductPrice) dto.PriceResponse {
	return dto.PriceResponse{
		ID:         p.ID,
		BranchID:   p.BranchID,
		ProductID:  p.ProductID,
		Price:      p.Price.StringFixed(domain.MoneyScale),
		Currency:   p.Currency,
		MinPrice:   amountString(p.MinPrice),
		Conditions: p.Conditions,
		ValidFrom:  p.ValidFrom,
		ValidTo:    p.ValidTo,
		SetBy:      p.UserID,
		CreatedAt:  p.CreatedAt,
	}
}
