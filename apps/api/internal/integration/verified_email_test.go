//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/storage"
)

// The requirement is charged on use rather than at the door, and that only exists end to end:
// the flag rides the tenant the middleware resolves, the exemptions are the route tree, and
// getting the whole session back after confirming depends on nothing being cached anywhere.

// requiringVerifiedEmail builds an environment with the requirement switched on.
func requiringVerifiedEmail(t *testing.T) *env {
	t.Helper()
	return newEnv(t, func(cfg *config.Config) { cfg.Auth.RequireVerifiedEmail = true })
}

// unconfirmedCaller seeds a user with a password, drops their confirmation, and logs them in.
func (e *env) unconfirmedCaller(t *testing.T, role domain.UserRole) (domain.AppUser, string, string) {
	t.Helper()
	accountID, _ := e.seedAccount(t, "Corralón Sin Confirmar")
	user := e.seedUserWithPassword(t, accountID, role, seedPassword)
	e.unverifyAddress(t, user.ID)

	session := e.login(t, user.Email, seedPassword)
	if session == nil {
		t.Fatal("an unconfirmed address was refused a session, which the requirement no longer does")
	}
	return user, session.AccessToken, session.RefreshToken
}

// Issuing a session is not using the product. Refusing here is what left whoever mistyped their
// address at signup with a session that expired and no screen to correct it from.
func TestVerifiedEmail_AnUnconfirmedAddressStillLogsIn(t *testing.T) {
	e := requiringVerifiedEmail(t)
	_, token, _ := e.unconfirmedCaller(t, domain.UserRoleAdmin)

	if token == "" {
		t.Fatal("login answered 200 with no access token")
	}
}

func TestVerifiedEmail_AClosedRouteAnswers403WithItsOwnCode(t *testing.T) {
	e := requiringVerifiedEmail(t)
	_, token, _ := e.unconfirmedCaller(t, domain.UserRoleAdmin)

	rec := e.do(t, request{method: http.MethodGet, path: "/v1/branches", token: token})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("GET /v1/branches unconfirmed = %d, want 403; body = %s", rec.Code, rec.Body)
	}
	if got := errorCode(t, rec); got != string(domain.CodeEmailNotVerified) {
		t.Fatalf("code = %q, want %q", got, domain.CodeEmailNotVerified)
	}
}

// The three exemptions are the whole escape hatch: without all of them the caller is shut in a
// session that can do nothing and cannot be fixed.
func TestVerifiedEmail_TheExemptRoutesStayOpen(t *testing.T) {
	e := requiringVerifiedEmail(t)
	_, token, refresh := e.unconfirmedCaller(t, domain.UserRoleAdmin)

	if rec := e.do(t, request{method: http.MethodGet, path: "/v1/me",
		token: token}); rec.Code != http.StatusOK {
		t.Errorf("GET /v1/me unconfirmed = %d, want 200; body = %s", rec.Code, rec.Body)
	}

	newAddress := "corregida+" + uuid.NewString() + "@corralon.test"
	if rec := e.do(t, request{method: http.MethodPost, path: "/v1/auth/change-email", token: token,
		body: map[string]any{"new_email": newAddress, "current_password": seedPassword},
	}); rec.Code != http.StatusNoContent {
		t.Errorf("POST /v1/auth/change-email unconfirmed = %d, want 204; body = %s", rec.Code, rec.Body)
	}

	// Last, because it ends the session the other two needed.
	if rec := e.do(t, request{method: http.MethodPost, path: "/v1/auth/logout", token: token,
		body: map[string]any{"refresh_token": refresh}}); rec.Code != http.StatusNoContent {
		t.Errorf("POST /v1/auth/logout unconfirmed = %d, want 204; body = %s", rec.Code, rec.Body)
	}
}

// Without renewal the session dies inside the refresh window and the lock-in comes back through
// another door, so refresh deliberately does not look at the flag.
func TestVerifiedEmail_RefreshWorksForAnUnconfirmedAddress(t *testing.T) {
	e := requiringVerifiedEmail(t)
	_, _, refresh := e.unconfirmedCaller(t, domain.UserRoleAdmin)

	rec := e.do(t, request{method: http.MethodPost, path: "/v1/public/auth/refresh",
		body: map[string]any{"refresh_token": refresh}})
	if rec.Code != http.StatusOK {
		t.Fatalf("refresh unconfirmed = %d, want 200; body = %s", rec.Code, rec.Body)
	}

	var renewed struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &renewed); err != nil {
		t.Fatalf("decode refresh: %v", err)
	}
	// The renewed token is still an unconfirmed one: renewal must not launder the requirement.
	if rec := e.do(t, request{method: http.MethodGet, path: "/v1/branches",
		token: renewed.AccessToken}); rec.Code != http.StatusForbidden {
		t.Fatalf("a renewed token reached a closed route with %d, want 403", rec.Code)
	}
}

/*
 * The AC that costs nothing only because the flag is read per request: confirming writes one
 * column and the very next call is allowed through, with no epoch bump, no re-login and nothing
 * to invalidate.
 */
func TestVerifiedEmail_ConfirmingRestoresAccessWithoutLoggingInAgain(t *testing.T) {
	e := requiringVerifiedEmail(t)
	user, token, _ := e.unconfirmedCaller(t, domain.UserRoleAdmin)

	if rec := e.do(t, request{method: http.MethodGet, path: "/v1/branches",
		token: token}); rec.Code != http.StatusForbidden {
		t.Fatalf("the closed route answered %d before confirming, want 403", rec.Code)
	}

	// Through the real route, with the token the mail carried.
	e.confirmAddress(t, user)

	// The same bearer, unchanged.
	if rec := e.do(t, request{method: http.MethodGet, path: "/v1/branches",
		token: token}); rec.Code != http.StatusOK {
		t.Fatalf("the same token after confirming = %d, want 200; body = %s", rec.Code, rec.Body)
	}
}

// Whatever else this changes, it must be invisible to everyone who confirmed.
func TestVerifiedEmail_AConfirmedCallerNoticesNothing(t *testing.T) {
	e := requiringVerifiedEmail(t)
	accountID, _ := e.seedAccount(t, "Corralón Confirmado")
	user := e.seedUserWithPassword(t, accountID, domain.UserRoleAdmin, seedPassword)

	session := e.login(t, user.Email, seedPassword)
	if session == nil {
		t.Fatal("a confirmed user could not log in with the requirement on")
	}
	for _, path := range []string{"/v1/me", "/v1/branches", "/v1/products"} {
		if rec := e.do(t, request{method: http.MethodGet, path: path,
			token: session.AccessToken}); rec.Code != http.StatusOK {
			t.Errorf("GET %s confirmed = %d, want 200; body = %s", path, rec.Code, rec.Body)
		}
	}

	/*
	 * And the whole authenticated surface, read off the router: what each route answers is its own
	 * business — a missing row is a 404, an empty body a 400 — but none of them may answer this
	 * refusal to somebody who confirmed.
	 */
	for _, route := range authenticatedRoutes(e) {
		rec := e.do(t, request{method: route.Method, path: placeholderFreePath(route.Path),
			token: session.AccessToken})
		if rec.Code == http.StatusForbidden &&
			errorCode(t, rec) == string(domain.CodeEmailNotVerified) {
			t.Errorf("%s %s refused a confirmed caller for an unconfirmed address",
				route.Method, route.Path)
		}
	}
}

/*
 * The default, and it has to be inert rather than merely lenient: the config refuses to switch
 * the requirement on while the transport is the console one, so an environment that cannot
 * deliver a link has to behave exactly as it did before any of this existed.
 */
func TestVerifiedEmail_WithTheRequirementOffNothingIsClosed(t *testing.T) {
	e := newEnv(t)
	accountID, _ := e.seedAccount(t, "Corralón Sin Exigencia")
	user := e.seedUserWithPassword(t, accountID, domain.UserRoleAdmin, seedPassword)
	e.unverifyAddress(t, user.ID)

	session := e.login(t, user.Email, seedPassword)
	if session == nil {
		t.Fatal("could not open the session the test needs")
	}
	if rec := e.do(t, request{method: http.MethodGet, path: "/v1/branches",
		token: session.AccessToken}); rec.Code != http.StatusOK {
		t.Fatalf("a closed route with the requirement off = %d, want 200; body = %s", rec.Code, rec.Body)
	}
}

/*
 * The gate sits ahead of RequireAdmin, so an unconfirmed seller on an admin route is told the
 * actionable thing rather than that they are the wrong role. Both answer 403; only the code says
 * which, and "ask an admin" would be the wrong screen for someone who has to confirm an address.
 */
func TestVerifiedEmail_TheGateAnswersBeforeTheRoleCheck(t *testing.T) {
	e := requiringVerifiedEmail(t)
	_, token, _ := e.unconfirmedCaller(t, domain.UserRoleSeller)

	rec := e.do(t, request{method: http.MethodGet, path: "/v1/users", token: token})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("GET /v1/users as an unconfirmed seller = %d, want 403; body = %s", rec.Code, rec.Body)
	}
	if got := errorCode(t, rec); got != string(domain.CodeEmailNotVerified) {
		t.Fatalf("code = %q, want %q: the role check answered first", got, domain.CodeEmailNotVerified)
	}
}

/*
 * Read off the router rather than listed by hand, so a route added to the wrong group later fails
 * here instead of quietly staying reachable. It only guards that direction — the three exemptions
 * are skipped, and TheExemptRoutesStayOpen is what would catch one of them being closed.
 */
func TestVerifiedEmail_EveryAuthenticatedRouteIsClosedExceptTheThree(t *testing.T) {
	e := requiringVerifiedEmail(t)
	_, token, _ := e.unconfirmedCaller(t, domain.UserRoleAdmin)

	exempt := map[string]bool{
		"GET /v1/me":                 true,
		"POST /v1/auth/logout":       true,
		"POST /v1/auth/change-email": true,
	}

	for _, route := range authenticatedRoutes(e) {
		name := route.Method + " " + route.Path
		if exempt[name] {
			continue
		}

		t.Run(name, func(t *testing.T) {
			// A placeholder needs *some* value; the middleware refuses before the handler parses it.
			path := placeholderFreePath(route.Path)
			rec := e.do(t, request{method: route.Method, path: path, token: token})
			if rec.Code != http.StatusForbidden {
				t.Fatalf("%s unconfirmed = %d, want 403: the route is not behind the requirement",
					name, rec.Code)
			}
			if got := errorCode(t, rec); got != string(domain.CodeEmailNotVerified) {
				t.Errorf("%s refused with code %q, want %q", name, got, domain.CodeEmailNotVerified)
			}
		})
	}
}

/*
 * authenticatedRoutes is every route behind RequireTenant, read off the router rather than listed
 * by hand: a route added to the wrong group later is then caught here instead of quietly changing
 * what an unconfirmed caller reaches.
 */
func authenticatedRoutes(e *env) []gin.RouteInfo {
	var routes []gin.RouteInfo
	for _, route := range e.router.Routes() {
		if !strings.HasPrefix(route.Path, "/v1/") || strings.HasPrefix(route.Path, "/v1/public/") {
			continue
		}
		// The signed link carries its whole authorization in the URL and resolves no tenant.
		if strings.HasPrefix(route.Path, storage.LinkPath) {
			continue
		}
		routes = append(routes, route)
	}
	return routes
}

// placeholderFreePath fills Gin's :params and *wildcards so the request routes to its handler.
func placeholderFreePath(pattern string) string {
	segments := strings.Split(pattern, "/")
	for i, segment := range segments {
		if strings.HasPrefix(segment, ":") || strings.HasPrefix(segment, "*") {
			segments[i] = uuid.NewString()
		}
	}
	return strings.Join(segments, "/")
}

/*
 * The change is what re-imposes the requirement, so its own effect on the flag is the load-bearing
 * half: a change that kept the stamp would leave an account reading as confirmed while pointing at
 * a mailbox nobody proved.
 */
func TestChangeEmail_DropsTheConfirmationAndMailsTheNewAddress(t *testing.T) {
	e := requiringVerifiedEmail(t)
	accountID, _ := e.seedAccount(t, "Corralón Corrige")
	user := e.seedUserWithPassword(t, accountID, domain.UserRoleAdmin, seedPassword)
	session := e.login(t, user.Email, seedPassword)
	if session == nil {
		t.Fatal("could not open the session the test needs")
	}

	newAddress := "corregida+" + uuid.NewString() + "@corralon.test"
	before := e.mail.count()
	rec := e.do(t, request{method: http.MethodPost, path: "/v1/auth/change-email",
		token: session.AccessToken,
		body:  map[string]any{"new_email": newAddress, "current_password": seedPassword}})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("POST /v1/auth/change-email = %d, want 204; body = %s", rec.Code, rec.Body)
	}

	var storedAddress string
	var verifiedAt *string
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT email, email_verified_at::text FROM app_user WHERE id = $1`,
		user.ID).Scan(&storedAddress, &verifiedAt); err != nil {
		t.Fatalf("read the user back: %v", err)
	}
	if storedAddress != newAddress {
		t.Errorf("stored address = %q, want %q", storedAddress, newAddress)
	}
	if verifiedAt != nil {
		t.Error("the confirmation survived the change, so the account reads as verified for a mailbox nobody proved")
	}
	// Two messages: the link to the new address and the warning to the one it left.
	if got := e.mail.count() - before; got != 2 {
		t.Fatalf("sent %d messages, want 2", got)
	}
	link, ok := e.mail.last()
	if !ok || link.To != newAddress {
		t.Fatalf("the confirmation link went to %q, want %q", link.To, newAddress)
	}

	// And the closed routes are shut again until the new address is confirmed.
	if rec := e.do(t, request{method: http.MethodGet, path: "/v1/branches",
		token: session.AccessToken}); rec.Code != http.StatusForbidden {
		t.Fatalf("a closed route after the change = %d, want 403", rec.Code)
	}
}

func TestChangeEmail_RefusesAWrongPasswordAndAnAddressInUse(t *testing.T) {
	e := requiringVerifiedEmail(t)
	accountID, _ := e.seedAccount(t, "Corralón Rechaza")
	user := e.seedUserWithPassword(t, accountID, domain.UserRoleAdmin, seedPassword)
	other := e.seedUser(t, accountID, domain.UserRoleSeller)
	session := e.login(t, user.Email, seedPassword)
	if session == nil {
		t.Fatal("could not open the session the test needs")
	}

	cases := []struct {
		name     string
		body     map[string]any
		wantCode int
	}{
		{"a wrong password", map[string]any{
			"new_email":        "otra+" + uuid.NewString() + "@corralon.test",
			"current_password": "no-es-la-clave"}, http.StatusUnauthorized},
		{"an address another user holds", map[string]any{
			"new_email": other.Email, "current_password": seedPassword}, http.StatusConflict},
		{"the caller's own address", map[string]any{
			"new_email": strings.ToUpper(user.Email), "current_password": seedPassword},
			http.StatusConflict},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := e.mail.count()
			rec := e.do(t, request{method: http.MethodPost, path: "/v1/auth/change-email",
				token: session.AccessToken, body: tc.body})
			if rec.Code != tc.wantCode {
				t.Fatalf("change-email with %s = %d, want %d; body = %s",
					tc.name, rec.Code, tc.wantCode, rec.Body)
			}
			if e.mail.count() != before {
				t.Error("a refused change still mailed somebody")
			}
			var stored string
			if err := e.db.CrossAccount().QueryRow(context.Background(),
				`SELECT email FROM app_user WHERE id = $1`, user.ID).Scan(&stored); err != nil {
				t.Fatalf("read the user back: %v", err)
			}
			if stored != user.Email {
				t.Errorf("the address moved to %q on a refused change", stored)
			}
		})
	}
}

/*
 * A recovery link already delivered to the old mailbox outlives the address, and after the change
 * that mailbox belongs to somebody else — so redeeming it would be a way back into an account its
 * owner just moved away from them.
 */
func TestChangeEmail_RetiresARecoveryLinkSentToTheOldAddress(t *testing.T) {
	e := requiringVerifiedEmail(t)
	accountID, _ := e.seedAccount(t, "Corralón Retira")
	user := e.seedUserWithPassword(t, accountID, domain.UserRoleAdmin, seedPassword)
	session := e.login(t, user.Email, seedPassword)
	if session == nil {
		t.Fatal("could not open the session the test needs")
	}

	// A recovery link, mailed to the address the account is about to leave.
	if rec := e.do(t, request{method: http.MethodPost, path: "/v1/public/auth/forgot-password",
		body: map[string]any{"email": user.Email}}); rec.Code != http.StatusAccepted {
		t.Fatalf("forgot-password = %d, want 202", rec.Code)
	}
	staleToken := e.mailedResetToken(t)

	if rec := e.do(t, request{method: http.MethodPost, path: "/v1/auth/change-email",
		token: session.AccessToken,
		body: map[string]any{"new_email": "corregida+" + uuid.NewString() + "@corralon.test",
			"current_password": seedPassword}}); rec.Code != http.StatusNoContent {
		t.Fatalf("change-email = %d, want 204; body = %s", rec.Code, rec.Body)
	}

	rec := e.do(t, request{method: http.MethodPost, path: "/v1/public/auth/reset-password",
		body: map[string]any{"token": staleToken, "new_password": "Otra-clave-nueva-9"}})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("redeeming a link mailed to the old address = %d, want 401; body = %s",
			rec.Code, rec.Body)
	}
}

// confirmAddress mails the user a link and redeems it through the public route, the way a person
// clicking the mail does.
func (e *env) confirmAddress(t *testing.T, user domain.AppUser) {
	t.Helper()
	if rec := e.do(t, request{method: http.MethodPost, path: "/v1/public/auth/resend-verification",
		body: map[string]any{"email": user.Email}}); rec.Code != http.StatusAccepted {
		t.Fatalf("resend-verification = %d, want 202; body = %s", rec.Code, rec.Body)
	}

	msg, ok := e.mail.last()
	if !ok {
		t.Fatal("no mail was sent, so there is no confirmation link")
	}
	_, after, found := strings.Cut(msg.TextBody, "https://backoffice.test/verify-email?token=")
	if !found {
		t.Fatalf("the mail carries no verification link:\n%s", msg.TextBody)
	}
	raw, _, _ := strings.Cut(after, "\n")
	token, err := url.QueryUnescape(strings.TrimSpace(raw))
	if err != nil {
		t.Fatalf("unescape the verification token: %v", err)
	}

	rec := e.do(t, request{method: http.MethodPost, path: "/v1/public/auth/verify-email",
		body: map[string]any{"token": token}})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("verify-email = %d, want 204; body = %s", rec.Code, rec.Body)
	}
}
