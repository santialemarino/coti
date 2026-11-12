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

// ComboService is the catalog combo administration the handler needs.
type ComboService interface {
	List(ctx context.Context, tenant domain.Tenant) ([]domain.Combo, error)
	Get(ctx context.Context, tenant domain.Tenant, id uuid.UUID) (*domain.Combo, error)
	Create(ctx context.Context, tenant domain.Tenant, in domain.ComboInput) (*domain.Combo, error)
	Replace(ctx context.Context, tenant domain.Tenant, id uuid.UUID, in domain.ComboInput) (*domain.Combo, error)
}

// QuoteComboService is the combo completion a priced quote offers.
type QuoteComboService interface {
	ComboSuggestions(ctx context.Context, tenant domain.Tenant, quoteID uuid.UUID) ([]domain.ComboSuggestion, error)
	AddComboItems(ctx context.Context, tenant domain.Tenant, quoteID, comboID uuid.UUID) ([]domain.QuoteItem, error)
}

// ComboHandler serves catalog combos and their completion on a quote.
type ComboHandler struct {
	combos         ComboService
	quotes         QuoteComboService
	highConfidence decimal.Decimal
}

// NewComboHandler builds a ComboHandler.
func NewComboHandler(combos ComboService, quotes QuoteComboService, highConfidence decimal.Decimal) *ComboHandler {
	return &ComboHandler{combos: combos, quotes: quotes, highConfidence: highConfidence}
}

// List returns every combo of the account.
//
//	@Summary		List combos
//	@Description	Returns every catalog combo of the account, inactive ones included, by name.
//	@Tags			combos
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	dto.ComboListResponse
//	@Failure		401	{object}	dto.ErrorResponse
//	@Failure		403	{object}	dto.ErrorResponse
//	@Router			/v1/combos [get]
func (h *ComboHandler) List(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	combos, err := h.combos.List(c.Request.Context(), tenant)
	if err != nil {
		Respond(c, err)
		return
	}
	items := make([]dto.ComboResponse, 0, len(combos))
	for _, combo := range combos {
		items = append(items, toComboResponse(combo))
	}
	c.JSON(http.StatusOK, dto.ComboListResponse{Items: items})
}

// Get returns one combo of the account.
//
//	@Summary		Get a combo
//	@Tags			combos
//	@Produce		json
//	@Security		BearerAuth
//	@Param			comboId	path		string	true	"Combo id"
//	@Success		200		{object}	dto.ComboResponse
//	@Failure		400		{object}	dto.ErrorResponse
//	@Failure		401		{object}	dto.ErrorResponse
//	@Failure		403		{object}	dto.ErrorResponse
//	@Failure		404		{object}	dto.ErrorResponse
//	@Router			/v1/combos/{comboId} [get]
func (h *ComboHandler) Get(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "comboId")
	if !ok {
		return
	}
	combo, err := h.combos.Get(c.Request.Context(), tenant, id)
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, toComboResponse(*combo))
}

// Create adds a combo to the account.
//
//	@Summary		Create a combo
//	@Description	Adds a combo of at least two distinct products, sold in the given branches.
//	@Tags			combos
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		dto.ComboRequest	true	"Combo"
//	@Success		201		{object}	dto.ComboResponse
//	@Failure		400		{object}	dto.ErrorResponse
//	@Failure		401		{object}	dto.ErrorResponse
//	@Failure		403		{object}	dto.ErrorResponse
//	@Failure		422		{object}	dto.ErrorResponse
//	@Router			/v1/combos [post]
func (h *ComboHandler) Create(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	in, ok := bindCombo(c)
	if !ok {
		return
	}
	combo, err := h.combos.Create(c.Request.Context(), tenant, in)
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusCreated, toComboResponse(*combo))
}

// Replace rewrites a combo's whole definition.
//
//	@Summary		Replace a combo
//	@Description	Replaces the combo's name, products and branches. Leaving a branch out stops selling it there.
//	@Tags			combos
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			comboId	path		string				true	"Combo id"
//	@Param			request	body		dto.ComboRequest	true	"Combo"
//	@Success		200		{object}	dto.ComboResponse
//	@Failure		400		{object}	dto.ErrorResponse
//	@Failure		401		{object}	dto.ErrorResponse
//	@Failure		403		{object}	dto.ErrorResponse
//	@Failure		404		{object}	dto.ErrorResponse
//	@Failure		422		{object}	dto.ErrorResponse
//	@Router			/v1/combos/{comboId} [put]
func (h *ComboHandler) Replace(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "comboId")
	if !ok {
		return
	}
	in, ok := bindCombo(c)
	if !ok {
		return
	}
	combo, err := h.combos.Replace(c.Request.Context(), tenant, id, in)
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, toComboResponse(*combo))
}

// Suggestions lists the combos a priced quote holds part of, with what each still lacks.
//
//	@Summary		List combo suggestions for a quote
//	@Description	Returns the active combos the quote's branch sells that the priced quote holds at least one product of and lacks at least one, every missing product active and priced. Empty for any other quote.
//	@Tags			quotes
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Branch-Id	header		string	true	"Active branch"
//	@Param			quoteId		path		string	true	"Quote id"
//	@Success		200			{object}	dto.ComboSuggestionListResponse
//	@Failure		400			{object}	dto.ErrorResponse
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		404			{object}	dto.ErrorResponse
//	@Router			/v1/quotes/{quoteId}/combo-suggestions [get]
func (h *ComboHandler) Suggestions(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	quoteID, ok := pathUUID(c, "quoteId")
	if !ok {
		return
	}
	suggestions, err := h.quotes.ComboSuggestions(c.Request.Context(), tenant, quoteID)
	if err != nil {
		Respond(c, err)
		return
	}
	items := make([]dto.ComboSuggestionResponse, 0, len(suggestions))
	for _, s := range suggestions {
		items = append(items, dto.ComboSuggestionResponse{
			ComboID: s.ComboID, Name: s.Name, Description: s.Description,
			Present: toComboItemResponses(s.Present), Missing: toComboItemResponses(s.Missing),
		})
	}
	c.JSON(http.StatusOK, dto.ComboSuggestionListResponse{Items: items})
}

// AddItems adds a suggested combo's missing products to the quote as priced lines.
//
//	@Summary		Complete a combo on a quote
//	@Description	Adds the suggested combo's missing products as priced lines in one transaction and re-runs the promotions. 409 when the combo is no longer suggested for the quote.
//	@Tags			quotes
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Branch-Id	header		string	true	"Active branch"
//	@Param			quoteId		path		string	true	"Quote id"
//	@Param			comboId		path		string	true	"Combo id"
//	@Success		201			{object}	dto.ComboItemsAddedResponse
//	@Failure		400			{object}	dto.ErrorResponse
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		404			{object}	dto.ErrorResponse
//	@Failure		409			{object}	dto.ErrorResponse
//	@Router			/v1/quotes/{quoteId}/combo-suggestions/{comboId}/add [post]
func (h *ComboHandler) AddItems(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	quoteID, ok := pathUUID(c, "quoteId")
	if !ok {
		return
	}
	comboID, ok := pathUUID(c, "comboId")
	if !ok {
		return
	}
	added, err := h.quotes.AddComboItems(c.Request.Context(), tenant, quoteID, comboID)
	if err != nil {
		Respond(c, err)
		return
	}
	items := make([]dto.QuoteItemResponse, 0, len(added))
	for _, item := range added {
		items = append(items, toQuoteItemResponse(item, nil, nil, h.highConfidence))
	}
	c.JSON(http.StatusCreated, dto.ComboItemsAddedResponse{Items: items})
}

func bindCombo(c *gin.Context) (domain.ComboInput, bool) {
	var body dto.ComboRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondBindError(c, err)
		return domain.ComboInput{}, false
	}
	in := domain.ComboInput{
		Name: body.Name, Description: body.Description, IsActive: true, BranchIDs: body.BranchIDs,
	}
	if body.IsActive != nil {
		in.IsActive = *body.IsActive
	}
	for i, item := range body.Items {
		quantity, err := decimalFromString(item.Quantity)
		if err != nil {
			Respond(c, fmt.Errorf("%w: items[%d].quantity must be a valid decimal", domain.ErrInvalidInput, i))
			return domain.ComboInput{}, false
		}
		in.Items = append(in.Items, domain.ComboItemInput{ProductID: item.ProductID, Quantity: quantity})
	}
	return in, true
}

func toComboResponse(combo domain.Combo) dto.ComboResponse {
	branches := combo.BranchIDs
	if branches == nil {
		branches = []uuid.UUID{}
	}
	return dto.ComboResponse{
		ID: combo.ID, Name: combo.Name, Description: combo.Description, IsActive: combo.IsActive,
		Items: toComboItemResponses(combo.Items), BranchIDs: branches,
		CreatedAt: combo.CreatedAt, UpdatedAt: combo.UpdatedAt,
	}
}

func toComboItemResponses(items []domain.ComboItem) []dto.ComboItemResponse {
	out := make([]dto.ComboItemResponse, 0, len(items))
	for _, item := range items {
		out = append(out, dto.ComboItemResponse{
			ProductID: item.ProductID, Quantity: item.Quantity.StringFixed(2),
			ProductCode: item.ProductCode, ProductName: item.ProductName,
			ProductUnit: item.ProductUnit, ProductIsActive: item.ProductIsActive,
		})
	}
	return out
}
