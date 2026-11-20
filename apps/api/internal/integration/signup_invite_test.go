//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
	"github.com/santialemarino/coti/apps/api/internal/services"
)

// closedSignup is the production setting: registration needs an invite.
func closedSignup(c *config.Config) { c.Signup.Enabled = false }

// mintInvite creates an invite the way cmd/invite does and returns its raw token. Call it after
// dropAccountByAdminEmail, so the invite goes before the account it points at.
func (e *env) mintInvite(t *testing.T) (string, uuid.UUID) {
	t.Helper()
	invites := services.NewSignupInviteService(e.db, repository.NewSignupInviteRepository(),
		config.SignupConfig{InviteTTL: 14 * 24 * time.Hour},
		config.WebConfig{BackofficeURL: "https://backoffice.test"}, nil)
	link, invite, err := invites.Create(context.Background(), domain.NewSignupInvite{})
	if err != nil {
		t.Fatalf("mint invite: %v", err)
	}
	t.Cleanup(func() { e.mustCleanup(t, `DELETE FROM signup_invite WHERE id = $1`, invite.ID) })

	parsed, err := url.Parse(link)
	if err != nil {
		t.Fatalf("parse invite link %q: %v", link, err)
	}
	return parsed.Query().Get("invite"), invite.ID
}

func (e *env) inviteUse(t *testing.T, id uuid.UUID) (*time.Time, *uuid.UUID) {
	t.Helper()
	var usedAt *time.Time
	var accountID *uuid.UUID
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT used_at, account_id FROM signup_invite WHERE id = $1`, id,
	).Scan(&usedAt, &accountID); err != nil {
		t.Fatalf("read invite: %v", err)
	}
	return usedAt, accountID
}

func (e *env) signup(t *testing.T, suffix, invite string) *httptest.ResponseRecorder {
	t.Helper()
	body := signupBody(suffix)
	if invite != "" {
		body["invite_token"] = invite
	}
	return e.do(t, request{method: http.MethodPost, path: "/v1/public/accounts", body: body})
}

func TestSignup_ClosedRefusesARegistrationWithoutAnInvite(t *testing.T) {
	e := newEnv(t, closedSignup)
	suffix := uuid.NewString()
	e.dropAccountByAdminEmail(t, suffix+"@test.local")

	res := e.signup(t, suffix, "")
	if res.Code != http.StatusForbidden || errorCode(t, res) != string(domain.CodeSignupClosed) {
		t.Fatalf("signup = %d %s, want 403 SIGNUP_CLOSED", res.Code, res.Body.String())
	}
}

func TestSignup_ClosedRegistersOnceWithAnInvite(t *testing.T) {
	e := newEnv(t, closedSignup)
	first, second := uuid.NewString(), uuid.NewString()
	e.dropAccountByAdminEmail(t, first+"@test.local")
	e.dropAccountByAdminEmail(t, second+"@test.local")
	token, inviteID := e.mintInvite(t)

	res := e.signup(t, first, token)
	if res.Code != http.StatusCreated {
		t.Fatalf("signup with invite = %d %s, want 201", res.Code, res.Body.String())
	}
	var created struct {
		Account struct {
			ID uuid.UUID `json:"id"`
		} `json:"account"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode signup: %v", err)
	}
	usedAt, accountID := e.inviteUse(t, inviteID)
	if usedAt == nil || accountID == nil || *accountID != created.Account.ID {
		t.Fatalf("invite used_at = %v, account_id = %v, want it spent on %s", usedAt, accountID, created.Account.ID)
	}

	again := e.signup(t, second, token)
	if again.Code != http.StatusForbidden || errorCode(t, again) != string(domain.CodeSignupInviteUsed) {
		t.Fatalf("second signup = %d %s, want 403 SIGNUP_INVITE_USED", again.Code, again.Body.String())
	}
}

func TestSignup_RefusesAnExpiredInviteAndLeavesItUnused(t *testing.T) {
	e := newEnv(t, closedSignup)
	suffix := uuid.NewString()
	e.dropAccountByAdminEmail(t, suffix+"@test.local")
	token, inviteID := e.mintInvite(t)
	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`UPDATE signup_invite SET expires_at = now() - interval '1 minute' WHERE id = $1`, inviteID); err != nil {
		t.Fatalf("expire invite: %v", err)
	}

	res := e.signup(t, suffix, token)
	if res.Code != http.StatusForbidden || errorCode(t, res) != string(domain.CodeSignupInviteExpired) {
		t.Fatalf("signup = %d %s, want 403 SIGNUP_INVITE_EXPIRED", res.Code, res.Body.String())
	}
	if usedAt, _ := e.inviteUse(t, inviteID); usedAt != nil {
		t.Fatalf("invite used_at = %v, want a refused registration to leave it unused", usedAt)
	}
}

// Without a usable invite a closed signup must not say whether an address is registered.
func TestSignup_ClosedChecksTheInviteBeforeTheAddress(t *testing.T) {
	e := newEnv(t)
	suffix := uuid.NewString()
	e.dropAccountByAdminEmail(t, suffix+"@test.local")
	if res := e.signup(t, suffix, ""); res.Code != http.StatusCreated {
		t.Fatalf("first signup = %d, want 201", res.Code)
	}

	closed := newEnv(t, closedSignup)
	res := closed.signup(t, suffix, "not-a-real-invite")
	if res.Code != http.StatusForbidden || errorCode(t, res) != string(domain.CodeSignupInviteInvalid) {
		t.Fatalf("signup = %d %s, want 403 SIGNUP_INVITE_INVALID, not EMAIL_TAKEN", res.Code, res.Body.String())
	}
}

func TestSignup_OpenNeedsNoInviteButSpendsOneItIsGiven(t *testing.T) {
	e := newEnv(t)
	plain, invited, bogus := uuid.NewString(), uuid.NewString(), uuid.NewString()
	e.dropAccountByAdminEmail(t, plain+"@test.local")
	e.dropAccountByAdminEmail(t, invited+"@test.local")
	e.dropAccountByAdminEmail(t, bogus+"@test.local")
	token, inviteID := e.mintInvite(t)

	if res := e.signup(t, plain, ""); res.Code != http.StatusCreated {
		t.Fatalf("open signup without invite = %d %s, want 201", res.Code, res.Body.String())
	}
	if res := e.signup(t, invited, token); res.Code != http.StatusCreated {
		t.Fatalf("open signup with invite = %d %s, want 201", res.Code, res.Body.String())
	}
	if usedAt, _ := e.inviteUse(t, inviteID); usedAt == nil {
		t.Fatal("invite left unused, want the open signup to spend it too")
	}
	// A link that was handed out and no longer works says so even while signup is open.
	res := e.signup(t, bogus, "not-a-real-invite")
	if res.Code != http.StatusForbidden || errorCode(t, res) != string(domain.CodeSignupInviteInvalid) {
		t.Fatalf("open signup with unknown invite = %d %s, want 403 SIGNUP_INVITE_INVALID", res.Code, res.Body.String())
	}
}

func TestSignupInvites_CheckRedeemsNothing(t *testing.T) {
	e := newEnv(t, closedSignup)
	suffix := uuid.NewString()
	e.dropAccountByAdminEmail(t, suffix+"@test.local")
	token, inviteID := e.mintInvite(t)
	check := func() *httptest.ResponseRecorder {
		return e.do(t, request{method: http.MethodPost, path: "/v1/public/signup-invites/check",
			body: map[string]any{"token": token}})
	}

	if res := check(); res.Code != http.StatusNoContent {
		t.Fatalf("check = %d %s, want 204", res.Code, res.Body.String())
	}
	if usedAt, _ := e.inviteUse(t, inviteID); usedAt != nil {
		t.Fatal("the check spent the invite")
	}
	if res := e.signup(t, suffix, token); res.Code != http.StatusCreated {
		t.Fatalf("signup = %d %s, want 201", res.Code, res.Body.String())
	}
	if res := check(); res.Code != http.StatusForbidden || errorCode(t, res) != string(domain.CodeSignupInviteUsed) {
		t.Fatalf("check after use = %d %s, want 403 SIGNUP_INVITE_USED", res.Code, res.Body.String())
	}
}
