package http

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/delivery/http/handler"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/ratelimit"
)

const contactRouteLimit = 2

type tokenCustomers struct{}

// Authenticate treats the bearer as the customer id, so a test picks who is calling.
func (tokenCustomers) Authenticate(_ context.Context, _ string, token string) (*domain.CustomerPrincipal, error) {
	id, err := uuid.Parse(token)
	if err != nil {
		return nil, domain.ErrUnauthenticated
	}
	return &domain.CustomerPrincipal{CustomerID: id, AccountID: uuid.Nil}, nil
}

type acceptingContacts struct{}

func (acceptingContacts) GetProfile(context.Context, domain.CustomerPrincipal) (*domain.ClientContactProfile, error) {
	return &domain.ClientContactProfile{}, nil
}
func (acceptingContacts) UpdateProfile(context.Context, domain.CustomerPrincipal, domain.ClientContactUpdate) error {
	return nil
}
func (acceptingContacts) SetMainContact(context.Context, domain.CustomerPrincipal, domain.ChannelType, string) error {
	return nil
}
func (acceptingContacts) ListSuggestions(context.Context, domain.Tenant) ([]domain.ContactObservation, error) {
	return nil, nil
}
func (acceptingContacts) ResolveSuggestion(context.Context, domain.Tenant, uuid.UUID, string) error {
	return nil
}
func (acceptingContacts) SendRegistrationLink(context.Context, domain.Tenant, uuid.UUID, domain.ChannelType) error {
	return nil
}
func (acceptingContacts) Request(context.Context, domain.CustomerPrincipal, domain.ChannelType, string) (*domain.ContactChallenge, error) {
	return &domain.ContactChallenge{ID: uuid.New(), ExpiresAt: time.Now()}, nil
}
func (acceptingContacts) Confirm(context.Context, domain.CustomerPrincipal, uuid.UUID, string) error {
	return nil
}

func contactLimitRouter() http.Handler {
	cfg := &config.Config{RateLimit: config.RateLimitConfig{Enabled: true, Window: time.Minute, Global: 100,
		ContactVerification: contactRouteLimit, Profile: contactRouteLimit}}
	contacts := handler.NewClientContactHandler(acceptingContacts{}).WithVerification(acceptingContacts{})
	return NewRouter(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)),
		Handlers{CustomerAuth: &handler.CustomerAuthHandler{}, ClientContacts: contacts},
		Auth{Customer: tokenCustomers{}}, RateLimit{Limiter: ratelimit.NewMemory(nil)})
}

func callContactRoute(router http.Handler, method, path, body, customer, address string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, "/v1/customer/sites/supplier.coti.test"+path, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+customer)
	request.Header.Set("Content-Type", "application/json")
	request.RemoteAddr = address
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

var contactRoutes = []struct{ method, path, body string }{
	{http.MethodPut, "/profile", `{"name":"Juan","contacts":[{"kind":"EMAIL","value":"juan@example.test","primary":true}]}`},
	{http.MethodPost, "/profile/contacts/verification", `{"kind":"EMAIL","value":"juan@example.test"}`},
	{http.MethodPost, "/profile/contacts/verification/confirm", `{"id":"` + uuid.NewString() + `","token":"123456"}`},
}

func TestContactRoutesRefuseOneCustomerPastTheLimitAcrossAddresses(t *testing.T) {
	t.Parallel()
	for _, route := range contactRoutes {
		router := contactLimitRouter()
		customer := uuid.NewString()
		for i := range contactRouteLimit {
			if response := callContactRoute(router, route.method, route.path, route.body, customer, fmt.Sprintf("10.0.0.%d:1111", i+1)); response.Code == http.StatusTooManyRequests {
				t.Fatalf("%s request %d refused inside the limit", route.path, i+1)
			}
		}
		response := callContactRoute(router, route.method, route.path, route.body, customer, "10.0.0.9:1111")
		if response.Code != http.StatusTooManyRequests {
			t.Fatalf("%s from a new address = %d, want 429 for the same customer", route.path, response.Code)
		}
		var body struct {
			Code string `json:"code"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body.Code != string(domain.CodeRateLimited) {
			t.Fatalf("%s body = %s, want the rate-limit error shape", route.path, response.Body)
		}
	}
}

func TestContactRoutesRefuseOneAddressPastTheLimitAcrossCustomers(t *testing.T) {
	t.Parallel()
	for _, route := range contactRoutes {
		router := contactLimitRouter()
		for i := range contactRouteLimit {
			if response := callContactRoute(router, route.method, route.path, route.body, uuid.NewString(), "10.0.0.1:1111"); response.Code == http.StatusTooManyRequests {
				t.Fatalf("%s request %d refused inside the limit", route.path, i+1)
			}
		}
		if response := callContactRoute(router, route.method, route.path, route.body, uuid.NewString(), "10.0.0.1:1111"); response.Code != http.StatusTooManyRequests {
			t.Fatalf("%s from a new customer = %d, want 429 for the same address", route.path, response.Code)
		}
	}
}
