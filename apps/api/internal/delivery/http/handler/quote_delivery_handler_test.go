package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/delivery/http/middleware"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

type captureQuoteDelivery struct {
	input domain.QuoteDeliveryInput
}

func (s *captureQuoteDelivery) Send(_ context.Context, _ domain.Tenant, quoteID uuid.UUID,
	input domain.QuoteDeliveryInput) (*domain.QuoteDeliveryResult, error) {
	s.input = input
	return &domain.QuoteDeliveryResult{QuoteID: quoteID}, nil
}

func (*captureQuoteDelivery) ResolvePublic(context.Context, string) (*domain.PublicQuoteRepresentation, error) {
	return nil, nil
}

func (*captureQuoteDelivery) RespondPublic(context.Context, string,
	domain.ClientActionInput) (*domain.PublicQuoteActionResult, error) {
	return nil, nil
}

func TestQuoteHandler_SendAcceptsEmailWithoutPhone(t *testing.T) {
	service := &captureQuoteDelivery{}
	handler := NewQuoteHandler(nil, service, nil, decimal.Zero)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	quoteID := uuid.New()
	key := uuid.New()
	c.Params = gin.Params{{Key: "quoteId", Value: quoteID.String()}}
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/quotes/"+quoteID.String()+"/sends",
		strings.NewReader(`{"email_delivery":{"address":"client@example.com"}}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("Idempotency-Key", key.String())
	middleware.SetTenant(c, domain.Tenant{AccountID: uuid.New(), BranchID: uuid.New(), UserID: uuid.New()})

	handler.Send(c)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", recorder.Code, recorder.Body)
	}
	if service.input.Phone != "" || service.input.Email == nil ||
		*service.input.Email != "client@example.com" || service.input.IdempotencyKey != key {
		t.Errorf("input = %+v, want only email and the idempotency key", service.input)
	}
}
