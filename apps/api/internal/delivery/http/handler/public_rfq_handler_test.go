package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

type publicRFQHandlerFake struct{ input *domain.PublicRFQInput }

func (f *publicRFQHandlerFake) GetDestination(context.Context, uuid.UUID) (*domain.PublicRFQDestination, error) {
	return &domain.PublicRFQDestination{SupplierName: "Corralón Norte", BranchName: "Morón",
		Route: domain.InboundChannelRoute{AccountID: uuid.New(), BranchID: uuid.New(), ChannelID: uuid.New()}}, nil
}
func (f *publicRFQHandlerFake) Create(_ context.Context, _ uuid.UUID, in domain.PublicRFQInput) error {
	f.input = &in
	return nil
}

func TestPublicRFQHandlerReceiptContainsNoCustomerOrInternalIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fake := &publicRFQHandlerFake{}
	h := NewPublicRFQHandler(fake, 1024)
	router := gin.New()
	router.POST("/:channelId", h.Create)
	request := httptest.NewRequest(http.MethodPost, "/"+uuid.NewString(), strings.NewReader(
		`{"request_id":"`+uuid.NewString()+`","tax_id":"20123456786","name":"Obra Norte","email":"contacto@example.com","text":"cemento"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || response.Body.String() != `{"received":true}` {
		t.Fatalf("unexpected receipt: %d %s", response.Code, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("Set-Cookie") != "" {
		t.Fatalf("unexpected receipt caching or session: %v", response.Header())
	}
	if fake.input == nil || fake.input.Contact.TaxID != "20123456786" {
		t.Fatal("contact was not mapped")
	}
}

func TestPublicRFQHandlerDestinationDoesNotExposeRouting(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewPublicRFQHandler(&publicRFQHandlerFake{}, 1024)
	router := gin.New()
	router.GET("/:channelId", h.GetDestination)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/"+uuid.NewString(), nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status %d", response.Code)
	}
	for _, key := range []string{"account_id", "branch_id", "channel_id", "contact", "email"} {
		if strings.Contains(response.Body.String(), key) {
			t.Fatalf("public identity exposed %s", key)
		}
	}
}
