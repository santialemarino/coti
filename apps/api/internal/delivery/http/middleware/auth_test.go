package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// RequireVerifiedEmail is what moves the confirmed-address requirement off the login screen and
// onto the closed routes, so what it lets through is the whole of that decision.

// verifiedEmailProbe mounts the middleware over a tenant and reports whether the handler ran.
func verifiedEmailProbe(t *testing.T, enabled bool, tenant *domain.Tenant) (*httptest.ResponseRecorder, bool) {
	t.Helper()
	handlerRan := false
	r := gin.New()
	r.GET("/closed",
		func(c *gin.Context) {
			if tenant != nil {
				SetTenant(c, *tenant)
			}
		},
		RequireVerifiedEmail(enabled),
		func(*gin.Context) { handlerRan = true },
	)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/closed", nil))
	return rec, handlerRan
}

func verifiedTenant(verified bool) *domain.Tenant {
	return &domain.Tenant{
		AccountID:     uuid.New(),
		UserID:        uuid.New(),
		Role:          domain.UserRoleSeller,
		EmailVerified: verified,
	}
}

func TestRequireVerifiedEmail_LetsAConfirmedAddressThrough(t *testing.T) {
	rec, handlerRan := verifiedEmailProbe(t, true, verifiedTenant(true))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !handlerRan {
		t.Error("a confirmed address was refused")
	}
}

// The code is the part the frontend reads: the status alone cannot tell this apart from a
// seller reaching an admin route, and the two send the caller to different screens.
func TestRequireVerifiedEmail_RefusesAnUnconfirmedAddressWithItsOwnCode(t *testing.T) {
	rec, handlerRan := verifiedEmailProbe(t, true, verifiedTenant(false))

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
	if handlerRan {
		t.Error("handler ran for an unconfirmed address; the middleware must abort first")
	}
	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	if body.Code != string(domain.CodeEmailNotVerified) {
		t.Errorf("code = %q, want %q", body.Code, domain.CodeEmailNotVerified)
	}
}

// Off is the default, and it has to be inert rather than merely lenient: a deployment whose
// transport cannot deliver the link must behave exactly as it did before this existed.
func TestRequireVerifiedEmail_DisabledLetsAnUnconfirmedAddressThrough(t *testing.T) {
	rec, handlerRan := verifiedEmailProbe(t, false, verifiedTenant(false))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !handlerRan {
		t.Error("an unconfirmed address was refused with the requirement off")
	}
}

// Mounted after RequireTenant, so no tenant means the chain was wired wrong. Answering 403
// would tell an anonymous caller that the route exists and what it wants.
func TestRequireVerifiedEmail_NoTenantIsUnauthenticated(t *testing.T) {
	rec, handlerRan := verifiedEmailProbe(t, true, nil)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if handlerRan {
		t.Error("handler ran with no tenant at all")
	}
}
