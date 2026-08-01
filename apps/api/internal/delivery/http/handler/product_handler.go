package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// ProductService is the catalog surface the handler needs.
type ProductService interface {
	ListProducts(ctx context.Context, tenant domain.Tenant, filter domain.ProductFilter) (domain.ProductPage, error)
	GetProduct(ctx context.Context, tenant domain.Tenant, id uuid.UUID) (*domain.Product, error)
	CreateProduct(ctx context.Context, tenant domain.Tenant, in domain.NewProduct) (*domain.Product, error)
	UpdateProduct(ctx context.Context, tenant domain.Tenant, id uuid.UUID, in domain.ProductUpdate) (*domain.Product, error)
	DeleteProduct(ctx context.Context, tenant domain.Tenant, id uuid.UUID) error
	ListSynonyms(ctx context.Context, tenant domain.Tenant, productID uuid.UUID) ([]domain.ProductSynonym, error)
	AddSynonym(ctx context.Context, tenant domain.Tenant, productID uuid.UUID, term string, source domain.SynonymSource) (*domain.ProductSynonym, error)
	RemoveSynonym(ctx context.Context, tenant domain.Tenant, productID, synonymID uuid.UUID) error
	ListAlternatives(ctx context.Context, tenant domain.Tenant, productID uuid.UUID, direction domain.AlternativeDirection) ([]domain.ProductAlternativeView, error)
	AddAlternative(ctx context.Context, tenant domain.Tenant, baseProductID, alternativeProductID uuid.UUID, alternativeType domain.ProductAlternativeType) (*domain.ProductAlternative, error)
	RemoveAlternative(ctx context.Context, tenant domain.Tenant, productID, alternativeID uuid.UUID) error
}

// ProductHandler serves the account-level catalog: products, synonyms, and alternatives.
type ProductHandler struct {
	products ProductService
}

// NewProductHandler builds a ProductHandler.
func NewProductHandler(products ProductService) *ProductHandler {
	return &ProductHandler{products: products}
}

// List returns one page of the account's catalog.
//
//	@Summary		List products
//	@Description	Returns one page of the account's catalog, ordered by name. Soft-deleted
//	@Description	items are hidden unless include_inactive is set.
//	@Tags			catalog
//	@Produce		json
//	@Security		BearerAuth
//	@Param			search				query		string	false	"Case-insensitive match on canonical name or code"
//	@Param			category			query		string	false	"Exact category match"
//	@Param			include_inactive	query		boolean	false	"Include deactivated items"
//	@Param			limit				query		integer	false	"Page size; defaults to CATALOG_DEFAULT_PAGE_SIZE and is capped at CATALOG_MAX_PAGE_SIZE"
//	@Param			offset				query		integer	false	"Rows to skip"
//	@Success		200					{object}	dto.ProductListResponse
//	@Failure		400					{object}	dto.ErrorResponse
//	@Failure		401					{object}	dto.ErrorResponse
//	@Router			/products [get]
func (h *ProductHandler) List(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}

	var query dto.ListProductsQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		RespondBindError(c, err)
		return
	}

	page, err := h.products.ListProducts(c.Request.Context(), tenant, domain.ProductFilter{
		Search:          query.Search,
		Category:        query.Category,
		IncludeInactive: query.IncludeInactive,
		Limit:           query.Limit,
		Offset:          query.Offset,
	})
	if err != nil {
		Respond(c, err)
		return
	}

	items := make([]dto.ProductResponse, 0, len(page.Items))
	for _, p := range page.Items {
		items = append(items, toProductResponse(p))
	}
	c.JSON(http.StatusOK, dto.ProductListResponse{
		Items:  items,
		Total:  page.Total,
		Limit:  page.Limit,
		Offset: page.Offset,
	})
}

// Get returns one catalog item.
//
//	@Summary	Get a product
//	@Tags		catalog
//	@Produce	json
//	@Security	BearerAuth
//	@Param		productId	path		string	true	"Product id"
//	@Success	200			{object}	dto.ProductResponse
//	@Failure	400			{object}	dto.ErrorResponse
//	@Failure	401			{object}	dto.ErrorResponse
//	@Failure	404			{object}	dto.ErrorResponse
//	@Router		/products/{productId} [get]
func (h *ProductHandler) Get(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	productID, ok := pathUUID(c, "productId")
	if !ok {
		return
	}

	product, err := h.products.GetProduct(c.Request.Context(), tenant, productID)
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, toProductResponse(*product))
}

// Create adds a catalog item to the account.
//
//	@Summary		Create a product
//	@Description	Creates an account-level catalog item. Availability, stock, and price are
//	@Description	set per branch by their own endpoints.
//	@Tags			catalog
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			body	body		dto.CreateProductRequest	true	"Product to create"
//	@Success		201		{object}	dto.ProductResponse
//	@Failure		400		{object}	dto.ErrorResponse
//	@Failure		401		{object}	dto.ErrorResponse
//	@Failure		409		{object}	dto.ErrorResponse	"The account already has a product with that code"
//	@Failure		422		{object}	dto.ErrorResponse
//	@Router			/products [post]
func (h *ProductHandler) Create(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}

	var body dto.CreateProductRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondBindError(c, err)
		return
	}

	product, err := h.products.CreateProduct(c.Request.Context(), tenant, domain.NewProduct{
		Code:          body.Code,
		CanonicalName: body.CanonicalName,
		Description:   body.Description,
		Unit:          body.Unit,
		Category:      body.Category,
	})
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusCreated, toProductResponse(*product))
}

// Update replaces a catalog item's editable attributes.
//
//	@Summary		Update a product
//	@Description	Replaces the editable attributes: an omitted nullable field clears the
//	@Description	column. is_active is the exception — omitted leaves it untouched.
//	@Tags			catalog
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			productId	path		string						true	"Product id"
//	@Param			body		body		dto.UpdateProductRequest	true	"Product as it should end up"
//	@Success		200			{object}	dto.ProductResponse
//	@Failure		400			{object}	dto.ErrorResponse
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		404			{object}	dto.ErrorResponse
//	@Failure		409			{object}	dto.ErrorResponse	"The account already has a product with that code"
//	@Failure		422			{object}	dto.ErrorResponse
//	@Router			/products/{productId} [put]
func (h *ProductHandler) Update(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	productID, ok := pathUUID(c, "productId")
	if !ok {
		return
	}

	var body dto.UpdateProductRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondBindError(c, err)
		return
	}

	product, err := h.products.UpdateProduct(c.Request.Context(), tenant, productID, domain.ProductUpdate{
		Code:          body.Code,
		CanonicalName: body.CanonicalName,
		Description:   body.Description,
		Unit:          body.Unit,
		Category:      body.Category,
		IsActive:      body.IsActive,
	})
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, toProductResponse(*product))
}

// Delete deactivates a catalog item.
//
//	@Summary		Deactivate a product
//	@Description	Soft delete: the row survives because quote and price history point at
//	@Description	it. Repeating the call is harmless.
//	@Tags			catalog
//	@Produce		json
//	@Security		BearerAuth
//	@Param			productId	path	string	true	"Product id"
//	@Success		204			"Deactivated"
//	@Failure		400			{object}	dto.ErrorResponse
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		404			{object}	dto.ErrorResponse
//	@Router			/products/{productId} [delete]
func (h *ProductHandler) Delete(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	productID, ok := pathUUID(c, "productId")
	if !ok {
		return
	}

	if err := h.products.DeleteProduct(c.Request.Context(), tenant, productID); err != nil {
		Respond(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// ListSynonyms returns the colloquial terms attached to a product.
//
//	@Summary		List product synonyms
//	@Description	Trade vocabulary that improves lexical catalog matching.
//	@Tags			catalog
//	@Produce		json
//	@Security		BearerAuth
//	@Param			productId	path		string	true	"Product id"
//	@Success		200			{object}	dto.SynonymListResponse
//	@Failure		400			{object}	dto.ErrorResponse
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		404			{object}	dto.ErrorResponse
//	@Router			/products/{productId}/synonyms [get]
func (h *ProductHandler) ListSynonyms(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	productID, ok := pathUUID(c, "productId")
	if !ok {
		return
	}

	synonyms, err := h.products.ListSynonyms(c.Request.Context(), tenant, productID)
	if err != nil {
		Respond(c, err)
		return
	}

	items := make([]dto.SynonymResponse, 0, len(synonyms))
	for _, s := range synonyms {
		items = append(items, toSynonymResponse(s))
	}
	c.JSON(http.StatusOK, dto.SynonymListResponse{Items: items})
}

// AddSynonym attaches a colloquial term to a product.
//
//	@Summary		Add a product synonym
//	@Description	source records where the term came from: MANUAL for one a person loaded,
//	@Description	LEARNED for one the matching pipeline proposed. Defaults to MANUAL.
//	@Tags			catalog
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			productId	path		string					true	"Product id"
//	@Param			body		body		dto.AddSynonymRequest	true	"Term to attach"
//	@Success		201			{object}	dto.SynonymResponse
//	@Failure		400			{object}	dto.ErrorResponse
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		404			{object}	dto.ErrorResponse
//	@Failure		409			{object}	dto.ErrorResponse	"The product already carries that term"
//	@Failure		422			{object}	dto.ErrorResponse
//	@Router			/products/{productId}/synonyms [post]
func (h *ProductHandler) AddSynonym(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	productID, ok := pathUUID(c, "productId")
	if !ok {
		return
	}

	var body dto.AddSynonymRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondBindError(c, err)
		return
	}

	synonym, err := h.products.AddSynonym(c.Request.Context(), tenant, productID, body.Term,
		domain.SynonymSource(body.Source))
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusCreated, toSynonymResponse(*synonym))
}

// RemoveSynonym detaches a term from a product.
//
//	@Summary	Remove a product synonym
//	@Tags		catalog
//	@Produce	json
//	@Security	BearerAuth
//	@Param		productId	path	string	true	"Product id"
//	@Param		synonymId	path	string	true	"Synonym id"
//	@Success	204			"Removed"
//	@Failure	400			{object}	dto.ErrorResponse
//	@Failure	401			{object}	dto.ErrorResponse
//	@Failure	404			{object}	dto.ErrorResponse
//	@Router		/products/{productId}/synonyms/{synonymId} [delete]
func (h *ProductHandler) RemoveSynonym(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	productID, ok := pathUUID(c, "productId")
	if !ok {
		return
	}
	synonymID, ok := pathUUID(c, "synonymId")
	if !ok {
		return
	}

	if err := h.products.RemoveSynonym(c.Request.Context(), tenant, productID, synonymID); err != nil {
		Respond(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// ListAlternatives returns the products linked to this one as alternatives.
//
//	@Summary		List product alternatives
//	@Description	direction picks which end of the relation to read: OUTGOING is what can
//	@Description	be offered instead of this product, INCOMING is what this product stands
//	@Description	in for. Defaults to OUTGOING.
//	@Tags			catalog
//	@Produce		json
//	@Security		BearerAuth
//	@Param			productId	path		string	true	"Product id"
//	@Param			direction	query		string	false	"OUTGOING or INCOMING"	Enums(OUTGOING, INCOMING)
//	@Success		200			{object}	dto.AlternativeListResponse
//	@Failure		400			{object}	dto.ErrorResponse
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		404			{object}	dto.ErrorResponse
//	@Router			/products/{productId}/alternatives [get]
func (h *ProductHandler) ListAlternatives(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	productID, ok := pathUUID(c, "productId")
	if !ok {
		return
	}

	var query dto.ListAlternativesQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		RespondBindError(c, err)
		return
	}
	direction := domain.AlternativeDirection(query.Direction)
	if direction == "" {
		direction = domain.AlternativeDirectionOutgoing
	}

	views, err := h.products.ListAlternatives(c.Request.Context(), tenant, productID, direction)
	if err != nil {
		Respond(c, err)
		return
	}

	items := make([]dto.AlternativeResponse, 0, len(views))
	for _, v := range views {
		product := toProductResponse(v.Product)
		items = append(items, dto.AlternativeResponse{
			ID:                   v.Link.ID,
			BaseProductID:        v.Link.BaseProductID,
			AlternativeProductID: v.Link.AlternativeProductID,
			Type:                 string(v.Link.Type),
			Product:              &product,
			CreatedAt:            v.Link.CreatedAt,
		})
	}
	c.JSON(http.StatusOK, dto.AlternativeListResponse{Items: items, Direction: string(direction)})
}

// AddAlternative links a product to another that can stand in for it.
//
//	@Summary		Add a product alternative
//	@Description	The product in the path is the base. type says what the alternative is
//	@Description	relative to it: EQUIVALENT, PREMIUM, or ECONOMY.
//	@Tags			catalog
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			productId	path		string						true	"Base product id"
//	@Param			body		body		dto.AddAlternativeRequest	true	"Alternative to link"
//	@Success		201			{object}	dto.AlternativeResponse
//	@Failure		400			{object}	dto.ErrorResponse
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		404			{object}	dto.ErrorResponse	"Either product is unknown to the account"
//	@Failure		409			{object}	dto.ErrorResponse	"The pair is already linked"
//	@Failure		422			{object}	dto.ErrorResponse	"A product cannot be its own alternative"
//	@Router			/products/{productId}/alternatives [post]
func (h *ProductHandler) AddAlternative(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	productID, ok := pathUUID(c, "productId")
	if !ok {
		return
	}

	var body dto.AddAlternativeRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondBindError(c, err)
		return
	}

	link, err := h.products.AddAlternative(c.Request.Context(), tenant, productID,
		body.AlternativeProductID, domain.ProductAlternativeType(body.Type))
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusCreated, dto.AlternativeResponse{
		ID:                   link.ID,
		BaseProductID:        link.BaseProductID,
		AlternativeProductID: link.AlternativeProductID,
		Type:                 string(link.Type),
		CreatedAt:            link.CreatedAt,
	})
}

// RemoveAlternative drops one alternative link.
//
//	@Summary		Remove a product alternative
//	@Description	The link is addressable from either of the two products it joins.
//	@Tags			catalog
//	@Produce		json
//	@Security		BearerAuth
//	@Param			productId		path	string	true	"Either product in the link"
//	@Param			alternativeId	path	string	true	"Alternative link id"
//	@Success		204				"Removed"
//	@Failure		400				{object}	dto.ErrorResponse
//	@Failure		401				{object}	dto.ErrorResponse
//	@Failure		404				{object}	dto.ErrorResponse
//	@Router			/products/{productId}/alternatives/{alternativeId} [delete]
func (h *ProductHandler) RemoveAlternative(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	productID, ok := pathUUID(c, "productId")
	if !ok {
		return
	}
	alternativeID, ok := pathUUID(c, "alternativeId")
	if !ok {
		return
	}

	if err := h.products.RemoveAlternative(c.Request.Context(), tenant, productID, alternativeID); err != nil {
		Respond(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func toProductResponse(p domain.Product) dto.ProductResponse {
	return dto.ProductResponse{
		ID:            p.ID,
		Code:          p.Code,
		CanonicalName: p.CanonicalName,
		Description:   p.Description,
		Unit:          p.Unit,
		Category:      p.Category,
		IsActive:      p.IsActive,
		CreatedAt:     p.CreatedAt,
		UpdatedAt:     p.UpdatedAt,
	}
}

func toSynonymResponse(s domain.ProductSynonym) dto.SynonymResponse {
	return dto.SynonymResponse{
		ID:        s.ID,
		ProductID: s.ProductID,
		Term:      s.Term,
		Source:    string(s.Source),
		CreatedAt: s.CreatedAt,
	}
}
