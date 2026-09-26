package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// ClientService is the client profile surface the handler needs.
type ClientService interface {
	ListClients(ctx context.Context, tenant domain.Tenant) ([]domain.ClientSummary, error)
	GetClient(ctx context.Context, tenant domain.Tenant,
		clientID uuid.UUID) (*domain.ClientProfile, error)
	ListTags(ctx context.Context, tenant domain.Tenant) ([]domain.ClientTag, error)
	GetQuoteAssociation(ctx context.Context, tenant domain.Tenant,
		quoteID uuid.UUID) (*domain.QuoteClientAssociation, error)
	CreateTag(ctx context.Context, tenant domain.Tenant, name string) (*domain.ClientTag, error)
	AssociateQuote(ctx context.Context, tenant domain.Tenant, quoteID uuid.UUID,
		in domain.AssociateQuoteClientInput) (*domain.ClientMatch, error)
	ReplaceClientTags(ctx context.Context, tenant domain.Tenant, clientID uuid.UUID,
		tagIDs []uuid.UUID) ([]domain.ClientTag, error)
}

// ClientHandler serves client profiles, tags, and accepted-sale association.
type ClientHandler struct {
	clients ClientService
}

// NewClientHandler builds a ClientHandler.
func NewClientHandler(clients ClientService) *ClientHandler {
	return &ClientHandler{clients: clients}
}

// List returns account clients with sales activity limited to the caller's branch reach.
//
//	@Summary		List client profiles
//	@Description	Returns account-level client profiles. Tags are account-wide; accepted sale counts and dates are limited to the branches and seller assignments the caller may inspect.
//	@Tags			clients
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Branch-Id	header		string	false	"Optional branch filter"
//	@Success		200			{object}	dto.ClientListResponse
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		403			{object}	dto.ErrorResponse
//	@Router			/v1/clients [get]
func (h *ClientHandler) List(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	clients, err := h.clients.ListClients(c.Request.Context(), tenant)
	if err != nil {
		Respond(c, err)
		return
	}
	items := make([]dto.ClientSummaryResponse, 0, len(clients))
	for i := range clients {
		items = append(items, toClientSummaryResponse(clients[i]))
	}
	c.JSON(http.StatusOK, dto.ClientListResponse{Items: items})
}

// Get returns one account client and accepted sales visible to the caller.
//
//	@Summary		Get a client profile
//	@Description	Returns one account-level profile and its currently accepted quotes inside the caller's branch and seller reach.
//	@Tags			clients
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Branch-Id	header		string	false	"Optional branch filter"
//	@Param			clientId	path		string	true	"Client id"
//	@Success		200			{object}	dto.ClientProfileResponse
//	@Failure		400			{object}	dto.ErrorResponse
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		403			{object}	dto.ErrorResponse
//	@Failure		404			{object}	dto.ErrorResponse
//	@Router			/v1/clients/{clientId} [get]
func (h *ClientHandler) Get(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	clientID, ok := pathUUID(c, "clientId")
	if !ok {
		return
	}
	profile, err := h.clients.GetClient(c.Request.Context(), tenant, clientID)
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, toClientProfileResponse(*profile))
}

// ListTags returns reusable labels in the caller's account.
//
//	@Summary		List client tags
//	@Tags			clients
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	dto.ClientTagListResponse
//	@Failure		401	{object}	dto.ErrorResponse
//	@Router			/v1/tags [get]
func (h *ClientHandler) ListTags(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	tags, err := h.clients.ListTags(c.Request.Context(), tenant)
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, dto.ClientTagListResponse{Items: toClientTagResponses(tags)})
}

// CreateTag creates or returns a reusable tag after normalized duplicate detection.
//
//	@Summary		Create a client tag
//	@Description	Creates a reusable account tag. Names that differ only by case or repeated whitespace resolve to the existing tag.
//	@Tags			clients
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		dto.CreateClientTagRequest	true	"Tag"
//	@Success		201		{object}	dto.ClientTagResponse
//	@Failure		400		{object}	dto.ErrorResponse
//	@Failure		401		{object}	dto.ErrorResponse
//	@Failure		422		{object}	dto.ErrorResponse
//	@Router			/v1/tags [post]
func (h *ClientHandler) CreateTag(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	var body dto.CreateClientTagRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondBindError(c, err)
		return
	}
	tag, err := h.clients.CreateTag(c.Request.Context(), tenant, body.Name)
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusCreated, toClientTagResponse(*tag))
}

// GetQuoteAssociation returns manual client-match context for an accepted quote.
//
//	@Summary		Get accepted-sale client association
//	@Description	Returns the current client, exact contact matches, delivery contact hints, and reusable tags. Suggestions never mutate or merge a profile.
//	@Tags			clients
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Branch-Id	header		string	true	"Active branch"
//	@Param			quoteId		path		string	true	"Quote id"
//	@Success		200			{object}	dto.QuoteClientAssociationResponse
//	@Failure		400			{object}	dto.ErrorResponse
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		403			{object}	dto.ErrorResponse
//	@Failure		404			{object}	dto.ErrorResponse
//	@Failure		409			{object}	dto.ErrorResponse	"Quote is not accepted"
//	@Failure		422			{object}	dto.ErrorResponse	"No active branch"
//	@Router			/v1/quotes/{quoteId}/client-association [get]
func (h *ClientHandler) GetQuoteAssociation(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	quoteID, ok := pathUUID(c, "quoteId")
	if !ok {
		return
	}
	association, err := h.clients.GetQuoteAssociation(c.Request.Context(), tenant, quoteID)
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, toQuoteClientAssociationResponse(*association))
}

// AssociateQuote confirms an existing or new client for one accepted quote.
//
//	@Summary		Associate an accepted sale with a client
//	@Description	Links the quote and its RFQ to exactly one seller-confirmed profile, and atomically replaces that profile's tags. Contact suggestions are never accepted automatically.
//	@Tags			clients
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Branch-Id	header		string					true	"Active branch"
//	@Param			quoteId		path		string					true	"Quote id"
//	@Param			request		body		dto.AssociateQuoteClientRequest	true	"Client selection and tags"
//	@Success		200			{object}	dto.ClientMatchResponse
//	@Failure		400			{object}	dto.ErrorResponse
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		403			{object}	dto.ErrorResponse
//	@Failure		404			{object}	dto.ErrorResponse
//	@Failure		409			{object}	dto.ErrorResponse	"Quote is not accepted"
//	@Failure		422			{object}	dto.ErrorResponse
//	@Router			/v1/quotes/{quoteId}/client-association [put]
func (h *ClientHandler) AssociateQuote(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	quoteID, ok := pathUUID(c, "quoteId")
	if !ok {
		return
	}
	var body dto.AssociateQuoteClientRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondBindError(c, err)
		return
	}
	input := domain.AssociateQuoteClientInput{ClientID: body.ClientID, TagIDs: body.TagIDs}
	if body.NewClient != nil {
		input.New = &domain.NewClient{Name: body.NewClient.Name, Phone: body.NewClient.Phone,
			Email: body.NewClient.Email}
	}
	match, err := h.clients.AssociateQuote(c.Request.Context(), tenant, quoteID, input)
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, toClientMatchResponse(*match))
}

// ReplaceTags replaces the reusable tags on one account client.
//
//	@Summary		Replace a client's tags
//	@Tags			clients
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			clientId	path		string					true	"Client id"
//	@Param			request		body		dto.ReplaceClientTagsRequest	true	"Selected tags"
//	@Success		200			{object}	dto.ClientTagListResponse
//	@Failure		400			{object}	dto.ErrorResponse
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		404			{object}	dto.ErrorResponse
//	@Failure		422			{object}	dto.ErrorResponse
//	@Router			/v1/clients/{clientId}/tags [put]
func (h *ClientHandler) ReplaceTags(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	clientID, ok := pathUUID(c, "clientId")
	if !ok {
		return
	}
	var body dto.ReplaceClientTagsRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondBindError(c, err)
		return
	}
	tags, err := h.clients.ReplaceClientTags(c.Request.Context(), tenant, clientID, body.TagIDs)
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, dto.ClientTagListResponse{Items: toClientTagResponses(tags)})
}

func toClientResponse(client domain.Client) dto.ClientResponse {
	var origin *string
	if client.OriginChannel != nil {
		value := string(*client.OriginChannel)
		origin = &value
	}
	return dto.ClientResponse{ID: client.ID, Name: client.Name, Phone: client.Phone,
		Email: client.Email, OriginChannel: origin, Notes: client.Notes,
		CreatedAt: client.CreatedAt, UpdatedAt: client.UpdatedAt}
}

func toClientSummaryResponse(client domain.ClientSummary) dto.ClientSummaryResponse {
	return dto.ClientSummaryResponse{ClientResponse: toClientResponse(client.Client),
		Tags: toClientTagResponses(client.Tags), AcceptedQuoteCount: client.AcceptedQuoteCount,
		LastAcceptedAt: client.LastAcceptedAt}
}

func toClientProfileResponse(profile domain.ClientProfile) dto.ClientProfileResponse {
	sales := make([]dto.ClientSaleResponse, 0, len(profile.Sales))
	for i := range profile.Sales {
		sale := profile.Sales[i]
		sales = append(sales, dto.ClientSaleResponse{QuoteID: sale.QuoteID, RFQID: sale.RFQID,
			Number: sale.Number, BranchID: sale.BranchID, BranchName: sale.BranchName,
			Total: sale.Total.StringFixed(2), AcceptedAt: sale.AcceptedAt})
	}
	return dto.ClientProfileResponse{Client: toClientResponse(profile.Client),
		Tags: toClientTagResponses(profile.Tags), Sales: sales}
}

func toClientTagResponses(tags []domain.ClientTag) []dto.ClientTagResponse {
	items := make([]dto.ClientTagResponse, 0, len(tags))
	for i := range tags {
		items = append(items, toClientTagResponse(tags[i]))
	}
	return items
}

func toClientTagResponse(tag domain.ClientTag) dto.ClientTagResponse {
	return dto.ClientTagResponse{ID: tag.ID, Name: tag.Name, CreatedAt: tag.CreatedAt}
}

func toClientMatchResponse(match domain.ClientMatch) dto.ClientMatchResponse {
	return dto.ClientMatchResponse{Client: toClientResponse(match.Client),
		Tags: toClientTagResponses(match.Tags)}
}

func toQuoteClientAssociationResponse(
	association domain.QuoteClientAssociation,
) dto.QuoteClientAssociationResponse {
	suggestions := make([]dto.ClientMatchResponse, 0, len(association.Suggestions))
	for i := range association.Suggestions {
		suggestions = append(suggestions, toClientMatchResponse(association.Suggestions[i]))
	}
	response := dto.QuoteClientAssociationResponse{Suggestions: suggestions,
		ContactHints: dto.ClientContactHintsResponse{Phone: association.ContactHints.Phone,
			Email: association.ContactHints.Email},
		AvailableTags: toClientTagResponses(association.AvailableTags)}
	if association.CurrentClient != nil {
		current := toClientMatchResponse(*association.CurrentClient)
		response.CurrentClient = &current
	}
	return response
}
