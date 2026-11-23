package http

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

type gmailRouteIdentity struct{}

func (gmailRouteIdentity) ParseAccessToken(raw string) (domain.AccessClaims, error) {
	return domain.AccessClaims{Role: domain.UserRole(raw)}, nil
}

func (gmailRouteIdentity) ResolveTenant(_ context.Context, claims domain.AccessClaims, branchID uuid.UUID) (domain.Tenant, error) {
	return domain.Tenant{AccountID: uuid.New(), UserID: uuid.New(), BranchID: branchID, Role: claims.Role, EmailVerified: true}, nil
}

func TestDisabledGmailAuthorizationReturnsSetupErrorBehindAdminAuthentication(t *testing.T) {
	t.Parallel()
	router := NewRouter(&config.Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)), Handlers{},
		Auth{Verifier: gmailRouteIdentity{}, Resolver: gmailRouteIdentity{}}, RateLimit{})
	for _, tt := range []struct {
		name   string
		token  string
		status int
	}{
		{name: "anonymous", status: http.StatusUnauthorized},
		{name: "seller", token: "SELLER", status: http.StatusForbidden},
		{name: "admin", token: "ADMIN", status: http.StatusServiceUnavailable},
	} {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/v1/channels/"+uuid.NewString()+"/gmail/authorization", nil)
			request.Header.Set("X-Branch-Id", uuid.NewString())
			if tt.token != "" {
				request.Header.Set("Authorization", "Bearer "+tt.token)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != tt.status {
				t.Fatalf("status = %d, want %d: %s", response.Code, tt.status, response.Body)
			}
			if tt.token == "ADMIN" {
				var body struct {
					Code string `json:"code"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body.Code != "NOT_CONFIGURED" {
					t.Fatalf("body = %s, want NOT_CONFIGURED", response.Body)
				}
			}
		})
	}
}
