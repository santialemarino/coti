package handler

import (
	"context"
	"net/http"
	"time"

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
	SetMainContact(context.Context, domain.CustomerPrincipal, domain.ChannelType, string) error
	ListSuggestions(context.Context, domain.Tenant) ([]domain.ContactObservation, error)
	ResolveSuggestion(context.Context, domain.Tenant, uuid.UUID, string) error
	SendRegistrationLink(context.Context, domain.Tenant, uuid.UUID, domain.ChannelType) error
}

// SendRegistrationLink sends a seller-confirmed request to complete the customer's own portal data.
// @Summary Send customer registration or profile link
// @Tags clients
// @Accept json
// @Security BearerAuth
// @Param clientId path string true "Client ID"
// @Param X-Branch-Id header string true "Selected branch"
// @Param body body dto.ContactRegistrationLinkRequest true "Contact channel"
// @Success 204
// @Failure 401,403,404,422,503 {object} dto.ErrorResponse
// @Router /v1/clients/{clientId}/registration-link [post]
func (h *ClientContactHandler) SendRegistrationLink(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "clientId")
	if !ok {
		return
	}
	var in dto.ContactRegistrationLinkRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		Respond(c, domain.ErrInvalidInput)
		return
	}
	if err := h.contacts.SendRegistrationLink(c.Request.Context(), tenant, id, domain.ChannelType(in.Kind)); err != nil {
		Respond(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// ClientContactHandler serves authorized customer edits and seller association decisions.
type ClientContactHandler struct {
	contacts     ClientContactService
	verification contactVerificationService
}

type contactVerificationService interface {
	Request(context.Context, domain.CustomerPrincipal, domain.ChannelType, string) (*domain.ContactChallenge, error)
	Confirm(context.Context, domain.CustomerPrincipal, uuid.UUID, string) error
}

// NewClientContactHandler builds the commercial contact handler.
func NewClientContactHandler(contacts ClientContactService) *ClientContactHandler {
	return &ClientContactHandler{contacts: contacts}
}

// WithVerification enables contact ownership proofs for authenticated customers.
func (h *ClientContactHandler) WithVerification(service contactVerificationService) *ClientContactHandler {
	h.verification = service
	return h
}

// RequestVerification sends an expiring ownership proof for a saved customer contact.
// @Summary Request customer contact verification
// @Tags customer-profile
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param hostname path string true "Registered supplier hostname"
// @Param body body dto.ContactVerificationRequest true "Saved contact destination"
// @Success 200 {object} dto.ContactVerificationResponse
// @Failure 401,404,409,422,423,503 {object} dto.ErrorResponse
// @Router /v1/customer/sites/{hostname}/profile/contacts/verification [post]
func (h *ClientContactHandler) RequestVerification(c *gin.Context) {
	p, ok := middleware.CustomerFrom(c)
	if !ok {
		Respond(c, domain.ErrUnauthenticated)
		return
	}
	if h.verification == nil {
		Respond(c, domain.ErrNotConfigured)
		return
	}
	var in dto.ContactVerificationRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		Respond(c, domain.ErrInvalidInput)
		return
	}
	challenge, err := h.verification.Request(c.Request.Context(), p, domain.ChannelType(in.Kind), in.Value)
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, dto.ContactVerificationResponse{ID: challenge.ID, ExpiresAt: challenge.ExpiresAt.Format(time.RFC3339)})
}

// ConfirmVerification verifies contact control and attaches its previous provisional orders.
// @Summary Confirm customer contact verification
// @Tags customer-profile
// @Accept json
// @Security BearerAuth
// @Param hostname path string true "Registered supplier hostname"
// @Param body body dto.ContactVerificationConfirmation true "Challenge and received proof"
// @Success 204
// @Failure 401,403,404,409,422 {object} dto.ErrorResponse
// @Router /v1/customer/sites/{hostname}/profile/contacts/verification/confirm [post]
func (h *ClientContactHandler) ConfirmVerification(c *gin.Context) {
	p, ok := middleware.CustomerFrom(c)
	if !ok {
		Respond(c, domain.ErrUnauthenticated)
		return
	}
	if h.verification == nil {
		Respond(c, domain.ErrNotConfigured)
		return
	}
	var in dto.ContactVerificationConfirmation
	if err := c.ShouldBindJSON(&in); err != nil {
		Respond(c, domain.ErrInvalidInput)
		return
	}
	if err := h.verification.Confirm(c.Request.Context(), p, in.ID, in.Token); err != nil {
		Respond(c, err)
		return
	}
	c.Status(http.StatusNoContent)
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
		contacts = append(contacts, dto.ClientContactResponse{Kind: string(contact.Kind), Value: contact.Value, Primary: contact.Primary, Verified: contact.Verified, Main: contact.Main})
	}
	var condition *string
	if profile.IVACondition != nil {
		value := string(*profile.IVACondition)
		condition = &value
	}
	c.JSON(http.StatusOK, dto.CustomerProfileResponse{ClientID: profile.ClientID, TaxID: profile.TaxID, Name: profile.Name, Contacts: contacts, ReviewPending: profile.ReviewPending, OwnershipUnproved: profile.OwnershipUnproved, LegalName: profile.LegalName, Address: profile.Address, IVACondition: condition})
}

// UpdateProfile saves only the authenticated customer's commercial data.
// @Summary Update customer commercial profile
// @Tags customer-profile
// @Accept json
// @Security BearerAuth
// @Param hostname path string true "Registered supplier hostname"
// @Param body body dto.UpdateCustomerProfileRequest true "Commercial profile"
// @Success 204
// @Failure 400,401,403,409,422 {object} dto.ErrorResponse
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
	in := domain.ClientContactUpdate{Name: body.Name, Contacts: make([]domain.ClientContact, 0, len(body.Contacts)), LegalName: body.LegalName, Address: body.Address}
	if body.IVACondition != nil {
		condition := domain.IVACondition(*body.IVACondition)
		in.IVACondition = &condition
	}
	for _, contact := range body.Contacts {
		in.Contacts = append(in.Contacts, domain.ClientContact{Kind: domain.ChannelType(contact.Kind), Value: contact.Value, Primary: contact.Primary})
	}
	if err := h.contacts.UpdateProfile(c.Request.Context(), p, in); err != nil {
		Respond(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// SetMainContact makes one of the customer's verified contacts the one account recovery preselects.
// @Summary Choose the customer's main contact
// @Tags customer-profile
// @Accept json
// @Security BearerAuth
// @Param hostname path string true "Registered supplier hostname"
// @Param body body dto.CustomerMainContactRequest true "A verified contact of the profile"
// @Success 204
// @Failure 401,404,422 {object} dto.ErrorResponse
// @Router /v1/customer/sites/{hostname}/profile/main-contact [put]
func (h *ClientContactHandler) SetMainContact(c *gin.Context) {
	p, ok := middleware.CustomerFrom(c)
	if !ok {
		Respond(c, domain.ErrUnauthenticated)
		return
	}
	var in dto.CustomerMainContactRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		Respond(c, domain.ErrInvalidInput)
		return
	}
	if err := h.contacts.SetMainContact(c.Request.Context(), p, domain.ChannelType(in.Kind), in.Value); err != nil {
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

// ResolveSuggestion rejects retired staff decisions that bypass contact ownership verification.
// @Summary Reject retired contact association decisions
// @Deprecated
// @Tags clients
// @Accept json
// @Security BearerAuth
// @Param X-Branch-Id header string false "Branch filter"
// @Param suggestionId path string true "Suggestion ID"
// @Param body body dto.ResolveContactSuggestionRequest true "Seller decision"
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
