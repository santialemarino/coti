package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/delivery/http/middleware"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

type fakeRecommendationRequest struct {
	tenant                     domain.Tenant
	quoteID, itemID, productID uuid.UUID
	kind                       domain.ProductAlternativeType
	proposals                  []domain.RecommendationProposal
	filled                     []domain.FilledRecommendation
	err                        error
}

func (f *fakeRecommendationRequest) FillVacant(_ context.Context, tenant domain.Tenant, quoteID uuid.UUID) ([]domain.FilledRecommendation, error) {
	f.tenant, f.quoteID = tenant, quoteID
	return f.filled, f.err
}

func (f *fakeRecommendationRequest) Search(_ context.Context, tenant domain.Tenant, quoteID uuid.UUID) ([]domain.RecommendationProposal, error) {
	f.tenant, f.quoteID = tenant, quoteID
	return f.proposals, f.err
}

func (f *fakeRecommendationRequest) AddIfVacant(_ context.Context, tenant domain.Tenant, quoteID, itemID, productID uuid.UUID, kind domain.ProductAlternativeType) (*domain.QuoteItemAlternative, error) {
	f.tenant, f.quoteID, f.itemID, f.productID, f.kind = tenant, quoteID, itemID, productID, kind
	return &domain.QuoteItemAlternative{ID: uuid.New(), ProductID: &productID}, f.err
}

func TestRecommendationRequestHandler_Search(t *testing.T) {
	itemID, productID, quoteID := uuid.New(), uuid.New(), uuid.New()
	tenant := domain.Tenant{AccountID: uuid.New(), BranchID: uuid.New()}
	fake := &fakeRecommendationRequest{proposals: []domain.RecommendationProposal{{
		ItemID: itemID, ProductID: productID, RecommendationType: domain.ProductAlternativeEconomy,
		Name: "Alternative", UnitPrice: decimal.RequireFromString("12.50"), Currency: "ARS",
	}}}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	c.Params = gin.Params{{Key: "quoteId", Value: quoteID.String()}}
	middleware.SetTenant(c, tenant)
	NewRecommendationRequestHandler(fake).Search(c)
	if recorder.Code != http.StatusOK || fake.tenant.AccountID != tenant.AccountID || fake.tenant.BranchID != tenant.BranchID || fake.quoteID != quoteID {
		t.Fatalf("response=%d tenant=%+v quote=%s", recorder.Code, fake.tenant, fake.quoteID)
	}
	var response dto.RecommendationSearchResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Items) != 1 || response.Items[0].ItemID != itemID || response.Items[0].ProductID != productID || response.Items[0].UnitPrice != "12.50" {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestRecommendationRequestHandler_AddIfVacant(t *testing.T) {
	quoteID, itemID, productID := uuid.New(), uuid.New(), uuid.New()
	tenant := domain.Tenant{AccountID: uuid.New(), BranchID: uuid.New()}
	fake := &fakeRecommendationRequest{err: domain.ErrConflict}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"product_id":"`+productID.String()+`"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "quoteId", Value: quoteID.String()}, {Key: "itemId", Value: itemID.String()}, {Key: "recommendationType", Value: "PREMIUM"}}
	middleware.SetTenant(c, tenant)
	NewRecommendationRequestHandler(fake).AddIfVacant(c)
	if recorder.Code != http.StatusConflict || fake.tenant.AccountID != tenant.AccountID || fake.tenant.BranchID != tenant.BranchID || fake.quoteID != quoteID || fake.itemID != itemID || fake.productID != productID || fake.kind != domain.ProductAlternativePremium {
		t.Fatalf("response=%d fake=%+v", recorder.Code, fake)
	}
}

func TestRecommendationRequestHandler_FillVacant(t *testing.T) {
	quoteID, itemID, productID, alternativeID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	tenant := domain.Tenant{AccountID: uuid.New(), BranchID: uuid.New()}
	premium := domain.ProductAlternativePremium
	fake := &fakeRecommendationRequest{filled: []domain.FilledRecommendation{{
		ItemID: itemID,
		Alternative: domain.QuoteItemAlternative{
			ID: alternativeID, ProductID: &productID, Type: domain.QuoteItemAlternativeTypeProduct,
			Origin: domain.QuoteItemAlternativeOriginSeller, RecommendationType: &premium,
		},
	}}}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	c.Params = gin.Params{{Key: "quoteId", Value: quoteID.String()}}
	middleware.SetTenant(c, tenant)
	NewRecommendationRequestHandler(fake).FillVacant(c)
	if recorder.Code != http.StatusOK || fake.tenant.AccountID != tenant.AccountID || fake.tenant.BranchID != tenant.BranchID || fake.quoteID != quoteID {
		t.Fatalf("response=%d tenant=%+v quote=%s", recorder.Code, fake.tenant, fake.quoteID)
	}
	var response dto.RecommendationFillResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Items) != 1 || response.Items[0].ItemID != itemID ||
		response.Items[0].Alternative.ID != alternativeID ||
		response.Items[0].Alternative.RecommendationType == nil ||
		*response.Items[0].Alternative.RecommendationType != "PREMIUM" {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestRecommendationRequestHandler_FillVacant_ReportsRefusal(t *testing.T) {
	fake := &fakeRecommendationRequest{err: domain.ErrConflict}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	c.Params = gin.Params{{Key: "quoteId", Value: uuid.NewString()}}
	middleware.SetTenant(c, domain.Tenant{AccountID: uuid.New(), BranchID: uuid.New()})
	NewRecommendationRequestHandler(fake).FillVacant(c)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("response=%d, want 409", recorder.Code)
	}
}
