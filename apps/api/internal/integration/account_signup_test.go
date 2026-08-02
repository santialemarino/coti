//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// signupBody builds a registration payload with a unique administrator address.
func signupBody(suffix string) map[string]any {
	return map[string]any{
		"account_name":   "Corralón " + suffix,
		"branch_name":    "Casa Central",
		"admin_name":     "Admin " + suffix,
		"admin_email":    suffix + "@test.local",
		"admin_password": "coti1234",
	}
}

// dropAccountByAdminEmail removes everything a registration created.
func (e *env) dropAccountByAdminEmail(t *testing.T, email string) {
	t.Helper()
	t.Cleanup(func() {
		c := context.Background()
		var accountID uuid.UUID
		if err := e.db.CrossAccount().QueryRow(c,
			`SELECT account_id FROM app_user WHERE lower(email) = lower($1)`, email,
		).Scan(&accountID); err != nil {
			return
		}
		for _, stmt := range []string{
			`DELETE FROM refresh_token WHERE account_id = $1`,
			`DELETE FROM user_branch WHERE account_id = $1`,
			`DELETE FROM app_user WHERE account_id = $1`,
			`DELETE FROM channel WHERE account_id = $1`,
			`DELETE FROM branch WHERE account_id = $1`,
			`DELETE FROM account WHERE id = $1`,
		} {
			_, _ = e.db.CrossAccount().Exec(c, stmt, accountID)
		}
	})
}

func TestSignup_CreatesAccountBranchChannelAndAdmin(t *testing.T) {
	e := newEnv(t)
	suffix := uuid.NewString()
	e.dropAccountByAdminEmail(t, suffix+"@test.local")

	res := e.do(t, request{method: http.MethodPost, path: "/v1/public/accounts", body: signupBody(suffix)})
	if res.Code != http.StatusCreated {
		t.Fatalf("signup = %d, want 201 (body %s)", res.Code, res.Body.String())
	}

	var body struct {
		Account struct {
			ID   uuid.UUID `json:"id"`
			Name string    `json:"name"`
		} `json:"account"`
		Branch struct {
			ID                uuid.UUID `json:"id"`
			DefaultExpiryDays int       `json:"default_expiry_days"`
		} `json:"branch"`
		Tokens struct {
			AccessToken string `json:"access_token"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode signup: %v", err)
	}

	if body.Tokens.AccessToken == "" {
		t.Fatal("signup returned no access token, want a usable session")
	}
	if body.Branch.DefaultExpiryDays != 7 {
		t.Errorf("default_expiry_days = %d, want the configured 7", body.Branch.DefaultExpiryDays)
	}

	// The manual-entry channel is what a counter or phone order hangs off. Without it,
	// rfq.channel_id has no legal value for this branch.
	var channels int
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT count(*) FROM channel
		 WHERE branch_id = $1 AND type = 'MANUAL_ENTRY' AND identifier IS NULL`,
		body.Branch.ID,
	).Scan(&channels); err != nil {
		t.Fatalf("count channels: %v", err)
	}
	if channels != 1 {
		t.Errorf("manual-entry channels = %d, want exactly 1", channels)
	}

	// The session the signup returned must actually reach the account it created.
	me := e.do(t, request{method: http.MethodGet, path: "/v1/me", token: body.Tokens.AccessToken})
	if me.Code != http.StatusOK {
		t.Fatalf("GET /v1/me = %d, want 200 (body %s)", me.Code, me.Body.String())
	}
	var identity struct {
		AccountID uuid.UUID `json:"account_id"`
		Role      string    `json:"role"`
	}
	if err := json.Unmarshal(me.Body.Bytes(), &identity); err != nil {
		t.Fatalf("decode me: %v", err)
	}
	if identity.AccountID != body.Account.ID {
		t.Errorf("me.account_id = %s, want the created account %s", identity.AccountID, body.Account.ID)
	}
	if identity.Role != "ADMIN" {
		t.Errorf("me.role = %q, want ADMIN", identity.Role)
	}
}

func TestSignup_RejectsAnEmailAlreadyRegisteredInAnotherAccount(t *testing.T) {
	e := newEnv(t)
	suffix := uuid.NewString()
	e.dropAccountByAdminEmail(t, suffix+"@test.local")

	if res := e.do(t, request{method: http.MethodPost, path: "/v1/public/accounts", body: signupBody(suffix)}); res.Code != http.StatusCreated {
		t.Fatalf("first signup = %d, want 201", res.Code)
	}

	// Login resolves a user by email alone, so a second account on the same address would
	// make the resulting session ambiguous.
	second := signupBody(suffix)
	second["account_name"] = "Otro Corralón"
	res := e.do(t, request{method: http.MethodPost, path: "/v1/public/accounts", body: second})
	if res.Code != http.StatusConflict {
		t.Fatalf("duplicate signup = %d, want 409 (body %s)", res.Code, res.Body.String())
	}
}

func TestSignup_RejectsAPasswordUnderTheConfiguredFloor(t *testing.T) {
	e := newEnv(t)
	suffix := uuid.NewString()
	e.dropAccountByAdminEmail(t, suffix+"@test.local")

	body := signupBody(suffix)
	body["admin_password"] = "short"
	res := e.do(t, request{method: http.MethodPost, path: "/v1/public/accounts", body: body})
	if res.Code != http.StatusUnprocessableEntity {
		t.Fatalf("short password = %d, want 422 (body %s)", res.Code, res.Body.String())
	}

	var count int
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT count(*) FROM app_user WHERE lower(email) = lower($1)`, suffix+"@test.local",
	).Scan(&count); err != nil {
		t.Fatalf("count users: %v", err)
	}
	if count != 0 {
		t.Errorf("users created = %d, want 0 — a rejected signup must leave nothing behind", count)
	}
}

func TestBranches_CreateOpensItsManualEntryChannel(t *testing.T) {
	e := newEnv(t)
	accountID, _ := e.seedAccount(t, "Branch CRUD")
	admin := e.seedUser(t, accountID, domain.UserRoleAdmin)
	token := e.tokenFor(t, admin)

	res := e.do(t, request{method: http.MethodPost, path: "/v1/branches", token: token, body: map[string]any{"name": "Sucursal Norte"}})
	if res.Code != http.StatusCreated {
		t.Fatalf("create branch = %d, want 201 (body %s)", res.Code, res.Body.String())
	}
	var branch struct {
		ID uuid.UUID `json:"id"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &branch); err != nil {
		t.Fatalf("decode branch: %v", err)
	}

	var channels int
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT count(*) FROM channel
		 WHERE branch_id = $1 AND type = 'MANUAL_ENTRY' AND identifier IS NULL`,
		branch.ID,
	).Scan(&channels); err != nil {
		t.Fatalf("count channels: %v", err)
	}
	if channels != 1 {
		t.Errorf("manual-entry channels = %d, want exactly 1", channels)
	}
}

func TestBranches_RefusesToCloseTheLastActiveBranch(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Last Branch")
	admin := e.seedUser(t, accountID, domain.UserRoleAdmin)
	token := e.tokenFor(t, admin)

	// The seeded account has exactly one branch, so closing it would leave nowhere to operate.
	res := e.do(t, request{method: http.MethodDelete, path: "/v1/branches/" + branchID.String(), token: token})
	if res.Code != http.StatusUnprocessableEntity {
		t.Fatalf("close last branch = %d, want 422 (body %s)", res.Code, res.Body.String())
	}

	// With a second branch open, closing the first is allowed.
	created := e.do(t, request{method: http.MethodPost, path: "/v1/branches", token: token, body: map[string]any{"name": "Segunda"}})
	if created.Code != http.StatusCreated {
		t.Fatalf("create second branch = %d, want 201", created.Code)
	}
	if res := e.do(t, request{method: http.MethodDelete, path: "/v1/branches/" + branchID.String(), token: token}); res.Code != http.StatusNoContent {
		t.Fatalf("close first branch = %d, want 204 (body %s)", res.Code, res.Body.String())
	}
}

func TestBranches_WritesAreAdminOnly(t *testing.T) {
	e := newEnv(t)
	accountID, _ := e.seedAccount(t, "Seller Branch")
	seller := e.seedUser(t, accountID, domain.UserRoleSeller)
	token := e.tokenFor(t, seller)

	res := e.do(t, request{method: http.MethodPost, path: "/v1/branches", token: token, body: map[string]any{"name": "No debería"}})
	if res.Code != http.StatusForbidden {
		t.Fatalf("seller create branch = %d, want 403 (body %s)", res.Code, res.Body.String())
	}
}
