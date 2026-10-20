package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/delivery/http/middleware"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// ClientContactService is the commercial contact surface exposed through HTTP.
type ClientContactService interface {
	GetProfile(context.Context, domain.CustomerPrincipal) (*domain.ClientContactProfile, error)
	UpdateProfile(context.Context, domain.CustomerPrincipal, domain.ClientContactUpdate) error
	ListSuggestions(context.Context, domain.Tenant) ([]domain.ContactObservation, error)
	ResolveSuggestion(context.Context, domain.Tenant, uuid.UUID, string) error
}

// ClientContactHandler serves authorized customer edits and seller association decisions.
type ClientContactHandler struct{ contacts ClientContactService }

// NewClientContactHandler builds the commercial contact handler.
func NewClientContactHandler(contacts ClientContactService) *ClientContactHandler {
	return &ClientContactHandler{contacts: contacts}
}

// GetProfile returns the authenticated customer's commercial data.
// @Summary Get customer commercial profile
// @Tags customer-profile
// @Produce json
// @Security BearerAuth
// @Param hostname path string true "Registered supplier hostname"
// @Success 200 {object} dto.CustomerProfileResponse
// @Failure 401,409 {object} dto.ErrorResponse
// @Router /v1/customer/sites/{hostname}/profile [get]
func (h *ClientContactHandler) GetProfile(c *gin.Context) {
	p, ok := middleware.CustomerFrom(c)
	if !ok {
		Respond(c, domain.ErrUnauthenticated)
		return
	}
	profile, err := h.contacts.GetProfile(c.Request.Context(), p)
	if err != nil {
		Respond(c, err)
		return
	}
	contacts := make([]dto.ClientContactResponse, 0, len(profile.Contacts))
	for _, contact := range profile.Contacts {
		contacts = append(contacts, dto.ClientContactResponse{Kind: string(contact.Kind), Value: contact.Value, Primary: contact.Primary})
	}
	c.JSON(http.StatusOK, dto.CustomerProfileResponse{ClientID: profile.ClientID, TaxID: profile.TaxID, Name: profile.Name, Contacts: contacts, ReviewPending: profile.ReviewPending})
}

// UpdateProfile saves only the authenticated customer's commercial data.
// @Summary Update customer commercial profile
// @Tags customer-profile
// @Accept json
// @Security BearerAuth
// @Param hostname path string true "Registered supplier hostname"
// @Param body body dto.UpdateCustomerProfileRequest true "Commercial profile"
// @Success 204
// @Failure 400,401,409,422 {object} dto.ErrorResponse
// @Router /v1/customer/sites/{hostname}/profile [put]
func (h *ClientContactHandler) UpdateProfile(c *gin.Context) {
	p, ok := middleware.CustomerFrom(c)
	if !ok {
		Respond(c, domain.ErrUnauthenticated)
		return
	}
	var body dto.UpdateCustomerProfileRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		Respond(c, domain.ErrInvalidInput)
		return
	}
	in := domain.ClientContactUpdate{Name: body.Name, Contacts: make([]domain.ClientContact, 0, len(body.Contacts))}
	for _, contact := range body.Contacts {
		in.Contacts = append(in.Contacts, domain.ClientContact{Kind: domain.ChannelType(contact.Kind), Value: contact.Value, Primary: contact.Primary})
	}
	if err := h.contacts.UpdateProfile(c.Request.Context(), p, in); err != nil {
		Respond(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// ListSuggestions returns persistent client cases visible in the current branch scope.
// @Summary List client contact suggestions
// @Tags clients
// @Produce json
// @Security BearerAuth
// @Param X-Branch-Id header string false "Branch filter"
// @Success 200 {array} dto.ContactSuggestionResponse
// @Failure 401,403 {object} dto.ErrorResponse
// @Router /v1/client-contact-suggestions [get]
func (h *ClientContactHandler) ListSuggestions(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	items, err := h.contacts.ListSuggestions(c.Request.Context(), tenant)
	if err != nil {
		Respond(c, err)
		return
	}
	result := make([]dto.ContactSuggestionResponse, 0, len(items))
	for _, o := range items {
		result = append(result, dto.ContactSuggestionResponse{ID: o.ID, SourceClientID: o.SourceClientID, CandidateClientID: o.CandidateClientID, CandidateName: o.CandidateName, TaxID: o.TaxID, Kind: string(o.Kind), Value: o.Value, DeclaredName: o.Name, OriginalText: o.Original, CreatedAt: o.CreatedAt})
	}
	c.JSON(http.StatusOK, result)
}

// ResolveSuggestion records an explicit seller decision inside the permitted branch scope.
// @Summary Resolve a client contact suggestion
// @Tags clients
// @Accept json
// @Security BearerAuth
// @Param X-Branch-Id header string false "Branch filter"
// @Param suggestionId path string true "Suggestion ID"
// @Param body body dto.ResolveContactSuggestionRequest true "Seller decision"
// @Success 204
// @Failure 400,401,403,404,409,422 {object} dto.ErrorResponse
// @Router /v1/client-contact-suggestions/{suggestionId} [put]
func (h *ClientContactHandler) ResolveSuggestion(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "suggestionId")
	if !ok {
		return
	}
	var body dto.ResolveContactSuggestionRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		Respond(c, domain.ErrInvalidInput)
		return
	}
	if err := h.contacts.ResolveSuggestion(c.Request.Context(), tenant, id, body.Action); err != nil {
		Respond(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
