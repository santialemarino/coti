//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// mailedInviteToken pulls the raw token out of the invite link the transport received.
func (e *env) mailedInviteToken(t *testing.T) string {
	t.Helper()
	msg, ok := e.mail.last()
	if !ok {
		t.Fatal("no mail was sent, so there is no invite link")
	}
	_, after, found := strings.Cut(msg.TextBody, "https://backoffice.test/reset-password?token=")
	if !found {
		t.Fatalf("the mail carries no invite link:\n%s", msg.TextBody)
	}
	raw, rest, _ := strings.Cut(after, "&")
	if !strings.HasPrefix(rest, "invite=1") {
		t.Fatalf("the link does not mark itself as an invite:\n%s", msg.TextBody)
	}
	token, err := url.QueryUnescape(strings.TrimSpace(raw))
	if err != nil {
		t.Fatalf("unescape token: %v", err)
	}
	return token
}

func (e *env) inviteStatusOf(t *testing.T, token string, userID uuid.UUID) *string {
	t.Helper()
	rec := e.do(t, request{method: http.MethodGet, path: "/v1/users/" + userID.String(), token: token})
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /v1/users/:id: status = %d; body = %s", rec.Code, rec.Body)
	}
	var user struct {
		InviteStatus *string `json:"invite_status"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &user); err != nil {
		t.Fatalf("decode user: %v", err)
	}
	return user.InviteStatus
}

// The whole invite: the admin creates the user without a password, the user chooses one through
// the mailed link, and that both opens the account and proves the address.
func TestInvite_TheLinkSetsThePasswordAndVerifiesTheAddress(t *testing.T) {
	e := newEnv(t, func(c *config.Config) { c.Mail.Provider = config.MailProviderSMTP })
	accountID, branchID := e.seedAccount(t, "Corralón Invita")
	admin := e.seedUser(t, accountID, domain.UserRoleAdmin)
	adminToken := e.tokenFor(t, admin)
	email := "invitada+" + uuid.NewString() + "@corralon.test"

	rec := e.do(t, request{method: http.MethodPost, path: "/v1/users", token: adminToken,
		body: map[string]any{"name": "Invitada", "email": email, "invite": true,
			"role": string(domain.UserRoleSeller), "branch_ids": []uuid.UUID{branchID}}})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /v1/users (invite): status = %d; body = %s", rec.Code, rec.Body)
	}
	var created struct {
		ID           uuid.UUID `json:"id"`
		InviteStatus *string   `json:"invite_status"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created user: %v", err)
	}
	if created.InviteStatus == nil || *created.InviteStatus != "PENDING" {
		t.Fatalf("invite_status = %v, want PENDING", created.InviteStatus)
	}
	invite := e.mailedInviteToken(t)

	if e.login(t, email, "Una-clave-adivinada1") != nil {
		t.Fatal("an invited user logged in before choosing a password")
	}

	// Resending retires the first link: only the newest one opens the account.
	rec = e.do(t, request{method: http.MethodPost,
		path: "/v1/users/" + created.ID.String() + "/invite", token: adminToken})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("POST /v1/users/:id/invite: status = %d; body = %s", rec.Code, rec.Body)
	}
	resent := e.mailedInviteToken(t)
	if resent == invite {
		t.Fatal("the resend mailed the same link")
	}
	rec = e.do(t, request{method: http.MethodPost, path: "/v1/public/auth/reset-password",
		body: map[string]any{"token": invite, "new_password": "Una-clave-larga1"}})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("the retired invite: status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	rec = e.do(t, request{method: http.MethodPost, path: "/v1/public/auth/reset-password",
		body: map[string]any{"token": resent, "new_password": "Una-clave-larga1"}})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("redeem the invite: status = %d; body = %s", rec.Code, rec.Body)
	}
	if e.login(t, email, "Una-clave-larga1") == nil {
		t.Fatal("the chosen password does not log in")
	}
	if status := e.inviteStatusOf(t, adminToken, created.ID); status != nil {
		t.Errorf("invite_status = %q after the password was chosen, want null", *status)
	}
	var verifiedAt *time.Time
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT email_verified_at FROM app_user WHERE id = $1`, created.ID).Scan(&verifiedAt); err != nil {
		t.Fatalf("read the invited user: %v", err)
	}
	if verifiedAt == nil {
		t.Error("redeeming the invite left the address unverified")
	}

	// Nothing is left to resend.
	rec = e.do(t, request{method: http.MethodPost,
		path: "/v1/users/" + created.ID.String() + "/invite", token: adminToken})
	if rec.Code != http.StatusUnprocessableEntity || errorCode(t, rec) != string(domain.CodeInviteNotPending) {
		t.Fatalf("resend after redemption: status = %d, code = %s; want 422 %s",
			rec.Code, errorCode(t, rec), domain.CodeInviteNotPending)
	}
}

// Under the console transport nobody could ever redeem the link, so the API refuses the invite
// instead of creating a user with no way in.
func TestInvite_IsRefusedWhileMailOnlyReachesTheLog(t *testing.T) {
	e := newEnv(t, func(c *config.Config) { c.Mail.Provider = config.MailProviderConsole })
	accountID, branchID := e.seedAccount(t, "Corralón Consola")
	admin := e.seedUser(t, accountID, domain.UserRoleAdmin)
	email := "consola+" + uuid.NewString() + "@corralon.test"

	rec := e.do(t, request{method: http.MethodPost, path: "/v1/users", token: e.tokenFor(t, admin),
		body: map[string]any{"name": "Invitada", "email": email, "invite": true,
			"role": string(domain.UserRoleSeller), "branch_ids": []uuid.UUID{branchID}}})
	if rec.Code != http.StatusServiceUnavailable || errorCode(t, rec) != string(domain.CodeMailNotConfigured) {
		t.Fatalf("POST /v1/users (invite) on console: status = %d, code = %s; want 503 %s",
			rec.Code, errorCode(t, rec), domain.CodeMailNotConfigured)
	}
	var n int
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT count(*) FROM app_user WHERE email = $1`, email).Scan(&n); err != nil {
		t.Fatalf("count users: %v", err)
	}
	if n != 0 {
		t.Error("the refused invite still created the user")
	}
}

// The screen decides whether an unconfirmed address blocks it, and whether to offer an invite,
// from what /me reports — never from a guess about the installation.
func TestMe_ReportsTheSessionPolicy(t *testing.T) {
	cases := []struct {
		name     string
		provider config.MailProvider
		require  bool
	}{
		{"console mail, nothing required", config.MailProviderConsole, false},
		{"real mail, verification required", config.MailProviderSMTP, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t, func(c *config.Config) {
				c.Mail.Provider = tc.provider
				c.Auth.RequireVerifiedEmail = tc.require
			})
			accountID, _ := e.seedAccount(t, "Corralón Política")
			admin := e.seedUser(t, accountID, domain.UserRoleAdmin)

			rec := e.do(t, request{method: http.MethodGet, path: "/v1/me", token: e.tokenFor(t, admin)})
			if rec.Code != http.StatusOK {
				t.Fatalf("GET /v1/me: status = %d; body = %s", rec.Code, rec.Body)
			}
			var me struct {
				Required     bool `json:"email_verification_required"`
				MailDelivery bool `json:"mail_delivery"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &me); err != nil {
				t.Fatalf("decode /v1/me: %v", err)
			}
			if me.Required != tc.require {
				t.Errorf("email_verification_required = %v, want %v", me.Required, tc.require)
			}
			if want := tc.provider != config.MailProviderConsole; me.MailDelivery != want {
				t.Errorf("mail_delivery = %v, want %v", me.MailDelivery, want)
			}
		})
	}
}
