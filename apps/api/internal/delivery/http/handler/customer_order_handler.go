package handler

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/delivery/http/middleware"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// CustomerOrderService is the history authorization surface used by customer routes.
type CustomerOrderService interface {
	List(context.Context, domain.CustomerPrincipal, int) ([]domain.CustomerOrder, error)
	Get(context.Context, domain.CustomerPrincipal, uuid.UUID) (*domain.CustomerOrder, error)
	GetByToken(context.Context, domain.CustomerPrincipal, string) (*domain.CustomerOrder, error)
}

// CustomerOrderHandler serves authenticated customer history and guards delivery tokens.
type CustomerOrderHandler struct{ orders CustomerOrderService }

// NewCustomerOrderHandler builds the customer history handler.
func NewCustomerOrderHandler(orders CustomerOrderService) *CustomerOrderHandler {
	return &CustomerOrderHandler{orders: orders}
}

// List returns a page of the authenticated customer's history across supplier branches.
// @Summary List customer requests
// @Tags customer-orders
// @Produce json
// @Security BearerAuth
// @Param hostname path string true "Registered supplier hostname"
// @Param page query int false "Page number starting at one"
// @Success 200 {array} dto.CustomerOrderResponse
// @Failure 401 {object} dto.ErrorResponse
// @Router /v1/customer/sites/{hostname}/orders [get]
func (h *CustomerOrderHandler) List(c *gin.Context) {
	p, ok := middleware.CustomerFrom(c)
	if !ok {
		Respond(c, domain.ErrUnauthenticated)
		return
	}
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil {
		Respond(c, domain.ErrInvalidInput)
		return
	}
	items, err := h.orders.List(c.Request.Context(), p, page)
	if err != nil {
		Respond(c, err)
		return
	}
	response := make([]dto.CustomerOrderResponse, 0, len(items))
	for _, item := range items {
		response = append(response, toCustomerOrder(item))
	}
	c.JSON(http.StatusOK, response)
}

// Get returns the original request and its latest sent quotation reference.
// @Summary Get a customer request
// @Tags customer-orders
// @Produce json
// @Security BearerAuth
// @Param hostname path string true "Registered supplier hostname"
// @Param rfqId path string true "Request ID"
// @Success 200 {object} dto.CustomerOrderResponse
// @Failure 401,404 {object} dto.ErrorResponse
// @Router /v1/customer/sites/{hostname}/orders/{rfqId} [get]
func (h *CustomerOrderHandler) Get(c *gin.Context) {
	p, ok := middleware.CustomerFrom(c)
	if !ok {
		Respond(c, domain.ErrUnauthenticated)
		return
	}
	id, ok := pathUUID(c, "rfqId")
	if !ok {
		return
	}
	item, err := h.orders.Get(c.Request.Context(), p, id)
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, toCustomerOrder(*item))
}

// GetByToken resolves a legacy link to the same request used by customer history.
// @Summary Resolve a customer delivery to its request
// @Tags customer-orders
// @Produce json
// @Security BearerAuth
// @Param hostname path string true "Registered supplier hostname"
// @Param token path string true "Delivery token"
// @Success 200 {object} dto.CustomerOrderResponse
// @Failure 401,404 {object} dto.ErrorResponse
// @Router /v1/customer/sites/{hostname}/quote-sends/{token}/order [get]
func (h *CustomerOrderHandler) GetByToken(c *gin.Context) {
	p, ok := middleware.CustomerFrom(c)
	if !ok {
		Respond(c, domain.ErrUnauthenticated)
		return
	}
	item, err := h.orders.GetByToken(c.Request.Context(), p, c.Param("token"))
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, toCustomerOrder(*item))
}

// AuthorizeDelivery blocks token reads and actions outside the authenticated CUIT/CUIL's history.
func (h *CustomerOrderHandler) AuthorizeDelivery(c *gin.Context) {
	p, ok := middleware.CustomerFrom(c)
	if !ok {
		Respond(c, domain.ErrUnauthenticated)
		c.Abort()
		return
	}
	if _, err := h.orders.GetByToken(c.Request.Context(), p, c.Param("token")); err != nil {
		Respond(c, err)
		c.Abort()
		return
	}
	c.Next()
}

func toCustomerOrder(item domain.CustomerOrder) dto.CustomerOrderResponse {
	return dto.CustomerOrderResponse{ID: item.ID, BranchName: item.BranchName, ReceivedAt: item.ReceivedAt, Status: item.Status, OriginalText: item.OriginalText, DeliveryToken: item.DeliveryToken}
}
