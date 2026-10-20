package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

type customerVerifierFake struct {
	hostname string
	secret   string
	identity domain.CustomerPrincipal
}

func (f customerVerifierFake) Authenticate(_ context.Context, hostname, raw string) (*domain.CustomerPrincipal, error) {
	if hostname != f.hostname || raw != f.secret {
		return nil, domain.ErrUnauthenticated
	}
	return &f.identity, nil
}

func TestRequireCustomerSeparatesCustomerAndStaffScopes(t *testing.T) {
	verifier := customerVerifierFake{hostname: "supplier.coti.test", secret: "customer-session", identity: domain.CustomerPrincipal{CustomerID: uuid.New(), AccountID: uuid.New()}}
	router := gin.New()
	router.GET("/sites/:hostname/me", RequireCustomer(verifier), func(c *gin.Context) {
		principal, ok := CustomerFrom(c)
		if !ok || principal != verifier.identity {
			t.Fatal("missing customer principal")
		}
		if _, ok := TenantFrom(c); ok {
			t.Fatal("customer authentication established staff permissions")
		}
		c.Status(http.StatusNoContent)
	})
	for _, tc := range []struct {
		name   string
		host   string
		token  string
		status int
	}{
		{"valid", verifier.hostname, verifier.secret, http.StatusNoContent},
		{"missing", verifier.hostname, "", http.StatusUnauthorized},
		{"staff JWT", verifier.hostname, "staff.jwt.token", http.StatusUnauthorized},
		{"other supplier", "other.coti.test", verifier.secret, http.StatusUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/sites/"+tc.host+"/me", nil)
			if tc.token != "" {
				req.Header.Set("Authorization", "Bearer "+tc.token)
			}
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, req)
			if recorder.Code != tc.status {
				t.Fatalf("status %d, expected %d", recorder.Code, tc.status)
			}
		})
	}
}
