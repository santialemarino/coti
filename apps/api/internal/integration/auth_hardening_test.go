//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// These three guarantees only exist end to end. The account check lives in a SQL join, the
// middleware is what enforces it per request, and the verification link only means anything
// once the transport has been handed one.

// deactivateAccount flips the flag the way an operator eventually will, through the owner
// pool because no endpoint writes it yet.
func (e *env) deactivateAccount(t *testing.T, accountID uuid.UUID, active bool) {
	t.Helper()
	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`UPDATE account SET is_active = $2 WHERE id = $1`, accountID, active); err != nil {
		t.Fatalf("set account is_active=%v: %v", active, err)
	}
}

func TestDeactivatedAccount_CutsEveryWayIn(t *testing.T) {
	e := newEnv(t)
	accountID, _ := e.seedAccount(t, "Corralón Desactivado")
	user := e.seedUserWithPassword(t, accountID, domain.UserRoleAdmin, seedPassword)

	session := e.login(t, user.Email, seedPassword)
	if session == nil {
		t.Fatal("could not open the session the test needs")
	}
	if me := e.do(t, request{method: http.MethodGet, path: "/v1/me", token: session.AccessToken}); me.Code != http.StatusOK {
		t.Fatalf("GET /v1/me before deactivation = %d, want 200", me.Code)
	}

	e.deactivateAccount(t, accountID, false)

	// The token the user is already holding, which is the AC a login-only check would miss.
	if me := e.do(t, request{method: http.MethodGet, path: "/v1/me", token: session.AccessToken}); me.Code != http.StatusUnauthorized {
		t.Errorf("GET /v1/me with a live token on a deactivated account = %d, want 401", me.Code)
	}
	refreshed := e.do(t, request{method: http.MethodPost, path: "/v1/public/auth/refresh",
		body: map[string]any{"refresh_token": session.RefreshToken}})
	if refreshed.Code != http.StatusUnauthorized {
		t.Errorf("refresh on a deactivated account = %d, want 401", refreshed.Code)
	}
	if e.login(t, user.Email, seedPassword) != nil {
		t.Error("a user of a deactivated account logged in")
	}

	// Reactivating restores access with no other step.
	e.deactivateAccount(t, accountID, true)
	if e.login(t, user.Email, seedPassword) == nil {
		t.Error("reactivating the account did not restore the login")
	}
}

// The response must not say which of the three things was wrong.
func TestDeactivatedAccount_AnswersLikeBadCredentials(t *testing.T) {
	e := newEnv(t)
	accountID, _ := e.seedAccount(t, "Corralón Opaco")
	user := e.seedUserWithPassword(t, accountID, domain.UserRoleAdmin, seedPassword)
	e.deactivateAccount(t, accountID, false)

	deactivated := e.do(t, request{method: http.MethodPost, path: "/v1/public/auth/login",
		body: map[string]any{"email": user.Email, "password": seedPassword}})
	wrongPassword := e.do(t, request{method: http.MethodPost, path: "/v1/public/auth/login",
		body: map[string]any{"email": user.Email, "password": "no-es-la-clave"}})

	if deactivated.Code != wrongPassword.Code {
		t.Fatalf("deactivated account answered %d and a wrong password %d; the pair reveals "+
			"the account's state", deactivated.Code, wrongPassword.Code)
	}
	if deactivated.Body.String() != wrongPassword.Body.String() {
		t.Fatalf("bodies differ: %q vs %q", deactivated.Body.String(), wrongPassword.Body.String())
	}
}

func TestSignup_MailsAVerificationLinkThatConfirmsTheAddress(t *testing.T) {
	e := newEnv(t)
	suffix := uuid.NewString()
	email := suffix + "@test.local"
	e.dropAccountByAdminEmail(t, email)

	res := e.do(t, request{method: http.MethodPost, path: "/v1/public/accounts", body: signupBody(suffix)})
	if res.Code != http.StatusCreated {
		t.Fatalf("signup = %d, want 201 (body %s)", res.Code, res.Body.String())
	}
	var body struct {
		Tokens struct {
			User struct {
				ID uuid.UUID `json:"id"`
			} `json:"user"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode signup: %v", err)
	}
	adminID := body.Tokens.User.ID

	// The send is recorded, which is the first AC.
	var event, status string
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT event, status FROM notification WHERE user_id = $1`, adminID,
	).Scan(&event, &status); err != nil {
		t.Fatalf("read the notification row: %v", err)
	}
	if event != string(domain.NotificationEventEmailVerification) || status != string(domain.NotificationStatusSent) {
		t.Fatalf("recorded %q/%q, want EMAIL_VERIFICATION/SENT", event, status)
	}

	token := e.mailedToken(t, "/verify-email")
	confirmed := e.do(t, request{method: http.MethodPost, path: "/v1/public/auth/verify-email",
		body: map[string]any{"token": token}})
	if confirmed.Code != http.StatusNoContent {
		t.Fatalf("verify-email = %d, want 204 (body %s)", confirmed.Code, confirmed.Body.String())
	}

	var verifiedAt *string
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT email_verified_at::text FROM app_user WHERE id = $1`, adminID,
	).Scan(&verifiedAt); err != nil {
		t.Fatalf("read email_verified_at: %v", err)
	}
	if verifiedAt == nil {
		t.Fatal("confirming the link did not stamp the user as verified")
	}

	// Single use: the same link a second time is a rejection.
	replay := e.do(t, request{method: http.MethodPost, path: "/v1/public/auth/verify-email",
		body: map[string]any{"token": token}})
	if replay.Code != http.StatusUnauthorized {
		t.Fatalf("replaying the link = %d, want 401", replay.Code)
	}
}

func TestResendVerification_RetiresThePreviousLinkAndNeverEnumerates(t *testing.T) {
	e := newEnv(t)
	suffix := uuid.NewString()
	email := suffix + "@test.local"
	e.dropAccountByAdminEmail(t, email)
	e.do(t, request{method: http.MethodPost, path: "/v1/public/accounts", body: signupBody(suffix)})
	first := e.mailedToken(t, "/verify-email")

	resent := e.do(t, request{method: http.MethodPost, path: "/v1/public/auth/resend-verification",
		body: map[string]any{"email": email}})
	if resent.Code != http.StatusAccepted {
		t.Fatalf("resend = %d, want 202 (body %s)", resent.Code, resent.Body.String())
	}
	second := e.mailedToken(t, "/verify-email")
	if first == second {
		t.Fatal("the resend mailed the same token, so nothing was rotated")
	}

	superseded := e.do(t, request{method: http.MethodPost, path: "/v1/public/auth/verify-email",
		body: map[string]any{"token": first}})
	if superseded.Code != http.StatusUnauthorized {
		t.Fatalf("redeeming the superseded link = %d, want 401", superseded.Code)
	}

	unknown := e.do(t, request{method: http.MethodPost, path: "/v1/public/auth/resend-verification",
		body: map[string]any{"email": uuid.NewString() + "@test.local"}})
	if unknown.Code != resent.Code || unknown.Body.String() != resent.Body.String() {
		t.Fatalf("an unregistered address answered %d/%q against %d/%q; the pair reveals which "+
			"addresses exist", unknown.Code, unknown.Body.String(), resent.Code, resent.Body.String())
	}
}

// mailedToken pulls the raw token out of the last link the transport received.
func (e *env) mailedToken(t *testing.T, route string) string {
	t.Helper()
	msg, ok := e.mail.last()
	if !ok {
		t.Fatal("no mail was sent, so there is no link")
	}
	_, after, found := strings.Cut(msg.TextBody, "https://backoffice.test"+route+"?token=")
	if !found {
		t.Fatalf("the mail carries no %s link:\n%s", route, msg.TextBody)
	}
	raw, _, _ := strings.Cut(after, "\n")
	return strings.TrimSpace(raw)
}

// newLimitedEnv builds an environment whose allowances actually bite, since the suite at large
// runs with them off.
func newLimitedEnv(t *testing.T, credentials int) *env {
	t.Helper()
	return newEnv(t, func(cfg *config.Config) {
		cfg.RateLimit = config.RateLimitConfig{
			Enabled:     true,
			Window:      time.Minute,
			Global:      1000,
			Credentials: credentials,
			Signup:      credentials,
			Mail:        credentials,
		}
	})
}

func TestRateLimit_RefusesPastTheRouteAllowanceEndToEnd(t *testing.T) {
	e := newLimitedEnv(t, 2)
	body := map[string]any{"email": "nadie@test.local", "password": "no-importa-1"}

	for i := 1; i <= 2; i++ {
		res := e.do(t, request{method: http.MethodPost, path: "/v1/public/auth/login", body: body})
		if res.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d = %d, want 401 (inside the allowance)", i, res.Code)
		}
	}

	refused := e.do(t, request{method: http.MethodPost, path: "/v1/public/auth/login", body: body})
	if refused.Code != http.StatusTooManyRequests {
		t.Fatalf("attempt past the allowance = %d, want 429", refused.Code)
	}
	if refused.Header().Get("Retry-After") == "" {
		t.Error("the 429 carries no Retry-After header")
	}
	var payload struct {
		Error             string `json:"error"`
		RetryAfterSeconds int    `json:"retry_after_seconds"`
	}
	if err := json.Unmarshal(refused.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode the 429 body: %v", err)
	}
	if payload.RetryAfterSeconds <= 0 {
		t.Errorf("retry_after_seconds = %d, want a positive value", payload.RetryAfterSeconds)
	}
	// Naming the bucket would tell a caller probing the API how the allowances are laid out.
	if strings.Contains(strings.ToLower(payload.Error), "credential") {
		t.Errorf("the 429 body names the limit that was hit: %q", payload.Error)
	}
}

// The per-address cap is only observable end to end: the answer has to stay identical while
// the transport stops being handed anything.
func TestMailTargetLimit_StopsSendingWithoutChangingTheAnswer(t *testing.T) {
	e := newEnv(t, func(cfg *config.Config) {
		// The caller allowances are deliberately out of reach, so the per-address one is the
		// only thing that can refuse.
		cfg.RateLimit = config.RateLimitConfig{
			Enabled: true, Window: time.Minute, Global: 1000,
			Credentials: 1000, Signup: 1000, Mail: 1000, MailPerAddress: 2,
		}
	})
	accountID, _ := e.seedAccount(t, "Corralón Buzón")
	user := e.seedUserWithPassword(t, accountID, domain.UserRoleAdmin, seedPassword)
	other := e.seedUserWithPassword(t, accountID, domain.UserRoleSeller, seedPassword)

	inside := e.do(t, request{method: http.MethodPost, path: "/v1/public/auth/forgot-password",
		body: map[string]any{"email": user.Email}})
	if inside.Code != http.StatusAccepted {
		t.Fatalf("the first request = %d, want 202 (body %s)", inside.Code, inside.Body.String())
	}
	e.do(t, request{method: http.MethodPost, path: "/v1/public/auth/forgot-password",
		body: map[string]any{"email": user.Email}})
	sent := e.mail.count()
	if sent != 2 {
		t.Fatalf("the transport was handed %d messages inside the cap, want 2", sent)
	}

	past := e.do(t, request{method: http.MethodPost, path: "/v1/public/auth/forgot-password",
		body: map[string]any{"email": user.Email}})
	if past.Code != inside.Code || past.Body.String() != inside.Body.String() {
		t.Fatalf("past the cap answered %d/%q against %d/%q; the pair says the address exists and "+
			"that someone keeps asking about it", past.Code, past.Body.String(),
			inside.Code, inside.Body.String())
	}
	if e.mail.count() != sent {
		t.Fatal("the request past the per-address cap still sent a message")
	}

	// The other route shares the mailbox's bucket, and the address is normalised before it is
	// counted, so a different spelling is not a fresh allowance.
	resent := e.do(t, request{method: http.MethodPost, path: "/v1/public/auth/resend-verification",
		body: map[string]any{"email": strings.ToUpper(user.Email)}})
	if resent.Code != http.StatusAccepted {
		t.Fatalf("resend past the cap = %d, want 202", resent.Code)
	}
	if e.mail.count() != sent {
		t.Fatal("a second route refilled the same mailbox past its cap")
	}

	// A different mailbox has its own bucket, so one flooded address does not silence the route.
	if untouched := e.do(t, request{method: http.MethodPost, path: "/v1/public/auth/forgot-password",
		body: map[string]any{"email": other.Email}}); untouched.Code != http.StatusAccepted {
		t.Fatalf("another address = %d, want 202", untouched.Code)
	}
	if e.mail.count() != sent+1 {
		t.Fatal("another address got no message while the first one was capped")
	}
}

// With the requirement on, an unverified user is refused and told why — the one rejection
// here that is not opaque, because it is only reachable once the password matched.
func TestLogin_RequiringAVerifiedAddressAnswers403(t *testing.T) {
	e := newEnv(t, func(cfg *config.Config) { cfg.Auth.RequireVerifiedEmail = true })

	accountID, _ := e.seedAccount(t, "Corralón Sin Verificar")
	user := e.seedUserWithPassword(t, accountID, domain.UserRoleAdmin, seedPassword)

	unverified := e.do(t, request{method: http.MethodPost, path: "/v1/public/auth/login",
		body: map[string]any{"email": user.Email, "password": seedPassword}})
	if unverified.Code != http.StatusForbidden {
		t.Fatalf("login unverified with the requirement on = %d, want 403 (body %s)",
			unverified.Code, unverified.Body.String())
	}

	// A wrong password still answers opaquely, so the 403 is not an enumeration oracle.
	wrong := e.do(t, request{method: http.MethodPost, path: "/v1/public/auth/login",
		body: map[string]any{"email": user.Email, "password": "no-es-la-clave"}})
	if wrong.Code != http.StatusUnauthorized {
		t.Fatalf("a wrong password with the requirement on = %d, want 401", wrong.Code)
	}

	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`UPDATE app_user SET email_verified_at = now() WHERE id = $1`, user.ID); err != nil {
		t.Fatalf("stamp the user verified: %v", err)
	}
	if e.login(t, user.Email, seedPassword) == nil {
		t.Error("a verified user could not log in with the requirement on")
	}
}
