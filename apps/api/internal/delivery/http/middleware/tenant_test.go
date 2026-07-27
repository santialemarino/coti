package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// RequireTenant is the gate in front of every authenticated route, so its two
// outcomes are worth pinning down: a request without a resolved account never
// reaches the handler, and one with an account arrives carrying it.

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

func TestRequireTenant_RejectsRequestWithoutTenant(t *testing.T) {
	handlerRan := false
	r := gin.New()
	r.GET("/guarded", RequireTenant(), func(*gin.Context) { handlerRan = true })

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/guarded", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if handlerRan {
		t.Error("handler ran without a tenant; RequireTenant must abort first")
	}
}

func TestRequireTenant_PassesTenantThrough(t *testing.T) {
	want := domain.Tenant{
		AccountID: uuid.New(),
		UserID:    uuid.New(),
		Role:      domain.UserRoleSeller,
		BranchID:  uuid.New(),
	}

	var got domain.Tenant
	var found bool
	r := gin.New()
	r.GET("/guarded",
		func(c *gin.Context) { SetTenant(c, want) },
		RequireTenant(),
		func(c *gin.Context) { got, found = TenantFrom(c) },
	)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/guarded", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !found {
		t.Fatal("TenantFrom() found = false, want true")
	}
	if got != want {
		t.Errorf("TenantFrom() = %+v, want %+v", got, want)
	}
}

// A caller must be able to tell "no tenant" from "zero-value tenant": treating the
// second as usable would run an unscoped query, which reads nothing under row level
// security and looks like an empty result rather than a wiring mistake.
func TestTenantFrom_ReportsAbsence(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	if _, found := TenantFrom(c); found {
		t.Error("TenantFrom() found = true on a bare context, want false")
	}
}

func TestTenantHelpers(t *testing.T) {
	cases := []struct {
		name        string
		tenant      domain.Tenant
		wantAdmin   bool
		wantsBranch bool
	}{
		{"admin without branch", domain.Tenant{Role: domain.UserRoleAdmin}, true, false},
		{"seller with branch", domain.Tenant{Role: domain.UserRoleSeller, BranchID: uuid.New()}, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.tenant.IsAdmin(); got != tc.wantAdmin {
				t.Errorf("IsAdmin() = %v, want %v", got, tc.wantAdmin)
			}
			if got := tc.tenant.HasBranch(); got != tc.wantsBranch {
				t.Errorf("HasBranch() = %v, want %v", got, tc.wantsBranch)
			}
		})
	}
}
