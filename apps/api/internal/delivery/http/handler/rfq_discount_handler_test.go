package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/delivery/http/middleware"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// stubDiscountRFQService implements ManualRFQService for the discount routes; the
// selectors an AddDiscount/UpdateDiscount/DeleteDiscount test never touches answer
// zero values so the stub satisfies the whole interface at once.
type stubDiscountRFQService struct {
	created         *domain.QuoteDiscount
	updated         *domain.QuoteDiscount
	createdTenant   domain.Tenant
	createdQuote    uuid.UUID
	createdInput    domain.QuoteDiscountCreate
	updatedTenant   domain.Tenant
	updatedQuote    uuid.UUID
	updatedID       uuid.UUID
	updatedInput    domain.QuoteDiscountUpdate
	deletedTenant   domain.Tenant
	deletedQuote    uuid.UUID
	deletedID       uuid.UUID
	createErr       error
	deleteErr       error
	setSeller       *domain.Quote
	setSellerErr    error
	setSellerTenant domain.Tenant
	setSellerRFQID  uuid.UUID
	setSellerID     *uuid.UUID
}

func (s *stubDiscountRFQService) List(context.Context, domain.Tenant) ([]domain.RfqListItem, error) {
	return nil, nil
}

func (s *stubDiscountRFQService) CreateManual(context.Context, domain.Tenant, domain.NewRfq) (*domain.RfqCreation, error) {
	return nil, nil
}

func (s *stubDiscountRFQService) GetDetail(context.Context, domain.Tenant, uuid.UUID) (*domain.RfqDetail, error) {
	return nil, nil
}

func (s *stubDiscountRFQService) AssignSeller(context.Context, domain.Tenant, uuid.UUID) (*domain.Quote, error) {
	return nil, nil
}

func (s *stubDiscountRFQService) SetSeller(
	_ context.Context, tenant domain.Tenant, rfqID uuid.UUID, sellerID *uuid.UUID,
) (*domain.Quote, error) {
	s.setSellerTenant = tenant
	s.setSellerRFQID = rfqID
	s.setSellerID = sellerID
	if s.setSellerErr != nil {
		return nil, s.setSellerErr
	}
	return s.setSeller, nil
}

func (s *stubDiscountRFQService) UpdateItem(context.Context, domain.Tenant, uuid.UUID, uuid.UUID, domain.QuoteItemUpdate) (*domain.QuoteItem, error) {
	return nil, nil
}

func (s *stubDiscountRFQService) DeleteItem(context.Context, domain.Tenant, uuid.UUID, uuid.UUID) error {
	return nil
}

func (s *stubDiscountRFQService) AddItem(context.Context, domain.Tenant, uuid.UUID, domain.QuoteItemCreate) (*domain.QuoteItem, error) {
	return nil, nil
}

func (s *stubDiscountRFQService) AddDiscount(
	_ context.Context, tenant domain.Tenant, quoteID uuid.UUID, in domain.QuoteDiscountCreate,
) (*domain.QuoteDiscount, error) {
	s.createdTenant = tenant
	s.createdQuote = quoteID
	s.createdInput = in
	return s.created, s.createErr
}

func (s *stubDiscountRFQService) UpdateDiscount(
	_ context.Context, tenant domain.Tenant, quoteID, discountID uuid.UUID, in domain.QuoteDiscountUpdate,
) (*domain.QuoteDiscount, error) {
	s.updatedTenant = tenant
	s.updatedQuote = quoteID
	s.updatedID = discountID
	s.updatedInput = in
	return s.updated, nil
}

func (s *stubDiscountRFQService) DeleteDiscount(
	_ context.Context, tenant domain.Tenant, quoteID, discountID uuid.UUID,
) error {
	s.deletedTenant = tenant
	s.deletedQuote = quoteID
	s.deletedID = discountID
	return s.deleteErr
}

// equalTenant compares the four fields a discount route consumes; the tenant carries a
// slice that is meaningless here, on purpose.
func equalTenant(a, b domain.Tenant) bool {
	return a.AccountID == b.AccountID && a.BranchID == b.BranchID &&
		a.UserID == b.UserID && a.Role == b.Role
}

// The seller's intent reaches the service unchanged: the handler binds the DTO, maps the
// decimal string and the item ids, and answers 201 with the created row serialized.
func TestRfqHandler_AddDiscount_CreatesAndAnswers201(t *testing.T) {
	gin.SetMode(gin.TestMode)
	quoteID := uuid.MustParse("77777777-7777-4777-8777-777777777777")
	versionID := uuid.MustParse("88888888-8888-4888-8888-888888888888")
	itemID := uuid.MustParse("99999999-9999-4999-8999-999999999999")
	tenant := domain.Tenant{
		AccountID: uuid.MustParse("11111111-1111-4111-8111-111111111111"),
		BranchID:  uuid.MustParse("22222222-2222-4222-8222-222222222222"),
		UserID:    uuid.MustParse("33333333-3333-4333-8333-333333333333"),
		Role:      domain.UserRoleAdmin,
	}
	description := "Descuento por obra"
	created := &domain.QuoteDiscount{
		ID:                 uuid.New(),
		AccountID:          tenant.AccountID,
		QuoteVersionID:     versionID,
		ConditionType:      domain.PromotionConditionOnTotal,
		Scope:              domain.DiscountScopeTotal,
		Origin:             domain.DiscountOriginManualSeller,
		Amount:             decimal.RequireFromString("500.00"),
		ActionType:         domain.PromotionActionFixedAmount,
		Description:        &description,
		SuppressedBySeller: false,
	}
	service := &stubDiscountRFQService{created: created}
	handler := NewRfqHandler(service)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Params = gin.Params{{Key: "quoteId", Value: quoteID.String()}}
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/quotes/"+quoteID.String()+"/discounts",
		bytes.NewReader([]byte(`{"description":"Descuento por obra","action_type":"FIXED_AMOUNT","value":"500.00","scope":"TOTAL"}`)))
	c.Request.Header.Set("Content-Type", "application/json")
	middleware.SetTenant(c, tenant)

	handler.AddDiscount(c)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", recorder.Code, recorder.Body)
	}
	if !equalTenant(service.createdTenant, tenant) {
		t.Errorf("service tenant = %+v, want %+v", service.createdTenant, tenant)
	}
	if service.createdQuote != quoteID {
		t.Errorf("service quote = %v, want %v", service.createdQuote, quoteID)
	}
	if service.createdInput.Description != "Descuento por obra" ||
		service.createdInput.ActionType != domain.PromotionActionFixedAmount ||
		!service.createdInput.Value.Equal(decimal.RequireFromString("500.00")) ||
		service.createdInput.Scope != domain.DiscountScopeTotal || len(service.createdInput.ItemIDs) != 0 {
		t.Errorf("service input = %+v, want the body's rule", service.createdInput)
	}

	var response dto.QuoteDiscountResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response %s: %v", recorder.Body, err)
	}
	if response.Amount != "500.00" || response.ActionType != string(domain.PromotionActionFixedAmount) ||
		response.Origin != string(domain.DiscountOriginManualSeller) ||
		response.Scope != string(domain.DiscountScopeTotal) ||
		response.ConditionType != string(domain.PromotionConditionOnTotal) ||
		response.SuppressedBySeller {
		t.Errorf("response = %+v, want a $500.00 fixed manual discount, unsuppressed", response)
	}

	// The parser also carries item_ids to a scoped discount.
	recorder2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(recorder2)
	c2.Params = gin.Params{{Key: "quoteId", Value: quoteID.String()}}
	c2.Request = httptest.NewRequest(http.MethodPost, "/v1/quotes/"+quoteID.String()+"/discounts",
		bytes.NewReader([]byte(`{"description":"Bono por ítem","action_type":"PERCENTAGE","value":"5.00","scope":"ITEM","item_ids":["`+itemID.String()+`"]}`)))
	c2.Request.Header.Set("Content-Type", "application/json")
	middleware.SetTenant(c2, tenant)

	handler.AddDiscount(c2)

	if recorder2.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", recorder2.Code, recorder2.Body)
	}
	if len(service.createdInput.ItemIDs) != 1 || service.createdInput.ItemIDs[0] != itemID {
		t.Errorf("service item_ids = %v, want [%v]", service.createdInput.ItemIDs, itemID)
	}
	if service.createdInput.ActionType != domain.PromotionActionPercentage {
		t.Errorf("service action_type = %q, want PERCENTAGE", service.createdInput.ActionType)
	}
	if !service.createdInput.Value.Equal(decimal.RequireFromString("5.00")) {
		t.Errorf("service value = %v, want 5.00", service.createdInput.Value)
	}
}

// A body without the required description is refused before the service is reached.
func TestRfqHandler_AddDiscount_RejectsAnIncompletePayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &stubDiscountRFQService{created: &domain.QuoteDiscount{}}
	handler := NewRfqHandler(service)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Params = gin.Params{{Key: "quoteId", Value: uuid.New().String()}}
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/quotes/x/discounts",
		bytes.NewReader([]byte(`{"value":"500.00","scope":"TOTAL"}`)))
	c.Request.Header.Set("Content-Type", "application/json")
	middleware.SetTenant(c, domain.Tenant{
		AccountID: uuid.MustParse("11111111-1111-4111-8111-111111111111"),
		BranchID:  uuid.MustParse("22222222-2222-4222-8222-222222222222"),
		Role:      domain.UserRoleAdmin,
	})

	handler.AddDiscount(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", recorder.Code, recorder.Body)
	}
	if service.createErr == nil && service.createdQuote != uuid.Nil {
		t.Error("service reached: the invalid payload must be refused in the handler")
	}
}

// The service answers ErrNotFound for a quote that does not belong to the caller, and the
// handler maps that to 404 before any row can land.
func TestRfqHandler_AddDiscount_UnknownQuoteAnswers404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &stubDiscountRFQService{createErr: domain.ErrNotFound}
	handler := NewRfqHandler(service)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	quoteID := uuid.New()
	c.Params = gin.Params{{Key: "quoteId", Value: quoteID.String()}}
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/quotes/"+quoteID.String()+"/discounts",
		bytes.NewReader([]byte(`{"description":"Obra","action_type":"FIXED_AMOUNT","value":"100.00","scope":"TOTAL"}`)))
	c.Request.Header.Set("Content-Type", "application/json")
	middleware.SetTenant(c, domain.Tenant{
		AccountID: uuid.MustParse("11111111-1111-4111-8111-111111111111"),
		BranchID:  uuid.MustParse("22222222-2222-4222-8222-222222222222"),
		Role:      domain.UserRoleAdmin,
	})

	handler.AddDiscount(c)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", recorder.Code, recorder.Body)
	}
}

// The update route forwards a rule patch: value, action type, scope and the covered lines
// reach the service parsed, and the response answers 200.
func TestRfqHandler_UpdateDiscount_ForwardsTheRulePatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	quoteID := uuid.MustParse("77777777-7777-4777-8777-777777777777")
	discountID := uuid.MustParse("aaaaaaa1-aaaa-4aaa-8aaa-aaaaaaaaaaaa")
	itemID := uuid.MustParse("bbbbbbb1-bbbb-4bbb-8bbb-bbbbbbbbbbbb")
	tenant := domain.Tenant{
		AccountID: uuid.MustParse("11111111-1111-4111-8111-111111111111"),
		BranchID:  uuid.MustParse("22222222-2222-4222-8222-222222222222"),
		Role:      domain.UserRoleAdmin,
	}
	description := "Bono por obra"
	suppressed := false
	service := &stubDiscountRFQService{updated: &domain.QuoteDiscount{
		ID: discountID, Scope: domain.DiscountScopeTotal,
		Origin: domain.DiscountOriginManualSeller, Amount: decimal.RequireFromString("250.00"),
		ActionType: domain.PromotionActionPercentage, Description: &description,
		SuppressedBySeller: suppressed,
	}}
	handler := NewRfqHandler(service)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Params = gin.Params{{Key: "quoteId", Value: quoteID.String()},
		{Key: "discountId", Value: discountID.String()}}
	c.Request = httptest.NewRequest(http.MethodPatch,
		"/v1/quotes/"+quoteID.String()+"/discounts/"+discountID.String(),
		bytes.NewReader([]byte(`{"action_type":"PERCENTAGE","value":"10.00","scope":"ITEM","item_ids":["`+itemID.String()+`"]}`)))
	c.Request.Header.Set("Content-Type", "application/json")
	middleware.SetTenant(c, tenant)

	handler.UpdateDiscount(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", recorder.Code, recorder.Body)
	}
	if !equalTenant(service.updatedTenant, tenant) || service.updatedQuote != quoteID ||
		service.updatedID != discountID {
		t.Errorf("service update target = %+v / %v / %v", service.updatedTenant,
			service.updatedQuote, service.updatedID)
	}
	if service.updatedInput.ActionType == nil ||
		*service.updatedInput.ActionType != domain.PromotionActionPercentage {
		t.Errorf("forwarded action_type = %v, want PERCENTAGE", service.updatedInput.ActionType)
	}
	if service.updatedInput.Value == nil ||
		!service.updatedInput.Value.Equal(decimal.RequireFromString("10.00")) {
		t.Errorf("forwarded value = %v, want 10.00", service.updatedInput.Value)
	}
	if service.updatedInput.Scope == nil || *service.updatedInput.Scope != domain.DiscountScopeItem {
		t.Errorf("forwarded scope = %v, want ITEM", service.updatedInput.Scope)
	}
	if len(service.updatedInput.ItemIDs) != 1 || service.updatedInput.ItemIDs[0] != itemID {
		t.Errorf("forwarded item_ids = %v, want [%v]", service.updatedInput.ItemIDs, itemID)
	}
	if service.updatedInput.ConditionType != nil {
		t.Errorf("forwarded condition_type = %v, want nil: the service derives it", service.updatedInput.ConditionType)
	}
}

// Deletion maps the same way: a seller-typed row the caller owns answers 200, and the
// service error surfaces unchanged when the row is gone or belongs elsewhere.
func TestRfqHandler_DeleteDiscount_RemovesTheRow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &stubDiscountRFQService{}
	handler := NewRfqHandler(service)

	tenant := domain.Tenant{
		AccountID: uuid.MustParse("11111111-1111-4111-8111-111111111111"),
		BranchID:  uuid.MustParse("22222222-2222-4222-8222-222222222222"),
		Role:      domain.UserRoleAdmin,
	}
	quoteID := uuid.New()
	discountID := uuid.New()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Params = gin.Params{{Key: "quoteId", Value: quoteID.String()},
		{Key: "discountId", Value: discountID.String()}}
	c.Request = httptest.NewRequest(http.MethodDelete,
		"/v1/quotes/"+quoteID.String()+"/discounts/"+discountID.String(), nil)
	middleware.SetTenant(c, tenant)

	handler.DeleteDiscount(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", recorder.Code, recorder.Body)
	}
	if !equalTenant(service.deletedTenant, tenant) || service.deletedQuote != quoteID ||
		service.deletedID != discountID {
		t.Errorf("service deletion = %+v / %v / %v, want the caller's scope and the named row",
			service.deletedTenant, service.deletedQuote, service.deletedID)
	}
}
