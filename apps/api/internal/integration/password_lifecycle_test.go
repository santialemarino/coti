//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// These guarantees span layers: the epoch bump lives in SQL, the middleware enforces it, and
// the recovery link only exists because the transport was handed one.

const (
	seedPassword = "Clave-original-1"
	nextPassword = "Clave-nueva-2026"
)

var errUndeliverable = errors.New("provider rejected the message")

// seedUserWithPassword seeds a user whose stored hash actually verifies, which the shared
// helper's placeholder does not.
func (e *env) seedUserWithPassword(t *testing.T, accountID uuid.UUID, role domain.UserRole, password string) domain.AppUser {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash seed password: %v", err)
	}
	id := uuid.New()
	user := domain.AppUser{
		ID: id, AccountID: accountID, Name: "Titular", Email: id.String() + "@test.local",
		PasswordHash: string(hash), Role: role, IsActive: true, SessionEpoch: 1,
	}
	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`INSERT INTO app_user (id, account_id, name, email, password_hash, role, session_epoch)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		user.ID, user.AccountID, user.Name, user.Email, user.PasswordHash, user.Role,
		user.SessionEpoch); err != nil {
		t.Fatalf("seed user with password: %v", err)
	}
	return user
}

// mailedResetToken pulls the raw token out of the link the transport received.
func (e *env) mailedResetToken(t *testing.T) string {
	t.Helper()
	msg, ok := e.mail.last()
	if !ok {
		t.Fatal("no mail was sent, so there is no recovery link")
	}
	_, after, found := strings.Cut(msg.TextBody, "https://backoffice.test/reset-password?token=")
	if !found {
		t.Fatalf("the mail carries no reset link:\n%s", msg.TextBody)
	}
	raw, _, _ := strings.Cut(after, "\n")
	token, err := url.QueryUnescape(strings.TrimSpace(raw))
	if err != nil {
		t.Fatalf("unescape token: %v", err)
	}
	return token
}

func (e *env) login(t *testing.T, email, password string) *struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
} {
	t.Helper()
	res := e.do(t, request{method: http.MethodPost, path: "/v1/public/auth/login",
		body: map[string]any{"email": email, "password": password}})
	if res.Code != http.StatusOK {
		return nil
	}
	var body struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode login: %v", err)
	}
	return &body
}

func TestChangePassword_EndsOtherSessionsAndKeepsTheCallersOwn(t *testing.T) {
	e := newEnv(t)
	accountID, _ := e.seedAccount(t, "Corralón Cambio")
	user := e.seedUserWithPassword(t, accountID, domain.UserRoleAdmin, seedPassword)

	// Two live sessions: the one that changes the password, and one on another device.
	changing := e.login(t, user.Email, seedPassword)
	other := e.login(t, user.Email, seedPassword)
	if changing == nil || other == nil {
		t.Fatal("could not open the two sessions the test needs")
	}

	res := e.do(t, request{method: http.MethodPost, path: "/v1/auth/change-password",
		token: changing.AccessToken,
		body:  map[string]any{"current_password": seedPassword, "new_password": nextPassword}})
	if res.Code != http.StatusOK {
		t.Fatalf("change-password = %d, want 200 (body %s)", res.Code, res.Body.String())
	}
	var issued struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &issued); err != nil {
		t.Fatalf("decode change-password: %v", err)
	}

	// The pair the change handed back has to work, or the caller logged themselves out.
	if me := e.do(t, request{method: http.MethodGet, path: "/v1/me", token: issued.AccessToken}); me.Code != http.StatusOK {
		t.Fatalf("GET /v1/me with the freshly issued token = %d, want 200 (body %s)", me.Code, me.Body.String())
	}

	// The other device's access token is dead on the epoch bump...
	if me := e.do(t, request{method: http.MethodGet, path: "/v1/me", token: other.AccessToken}); me.Code != http.StatusUnauthorized {
		t.Errorf("GET /v1/me with the other session's access token = %d, want 401", me.Code)
	}
	// ...and its refresh token must not resurrect it, which the epoch alone does not prevent.
	refreshed := e.do(t, request{method: http.MethodPost, path: "/v1/public/auth/refresh",
		body: map[string]any{"refresh_token": other.RefreshToken}})
	if refreshed.Code != http.StatusUnauthorized {
		t.Errorf("refreshing the other session = %d, want 401: the session survived the password change",
			refreshed.Code)
	}

	if e.login(t, user.Email, seedPassword) != nil {
		t.Error("the old password still logs in")
	}
	if e.login(t, user.Email, nextPassword) == nil {
		t.Error("the new password does not log in")
	}
}

func TestChangePassword_WrongCurrentPasswordIsRejected(t *testing.T) {
	e := newEnv(t)
	accountID, _ := e.seedAccount(t, "Corralón Actual")
	user := e.seedUserWithPassword(t, accountID, domain.UserRoleSeller, seedPassword)
	session := e.login(t, user.Email, seedPassword)

	res := e.do(t, request{method: http.MethodPost, path: "/v1/auth/change-password",
		token: session.AccessToken,
		body:  map[string]any{"current_password": "no-es-la-actual", "new_password": nextPassword}})
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("change-password with the wrong current password = %d, want 401 (body %s)",
			res.Code, res.Body.String())
	}
	if e.login(t, user.Email, seedPassword) == nil {
		t.Error("the original password stopped working after a rejected change")
	}
}

func TestForgotPassword_LinkRecoversTheAccountAndWorksOnlyOnce(t *testing.T) {
	e := newEnv(t)
	accountID, _ := e.seedAccount(t, "Corralón Recupero")
	user := e.seedUserWithPassword(t, accountID, domain.UserRoleAdmin, seedPassword)
	stale := e.login(t, user.Email, seedPassword)

	res := e.do(t, request{method: http.MethodPost, path: "/v1/public/auth/forgot-password",
		body: map[string]any{"email": strings.ToUpper(user.Email)}})
	if res.Code != http.StatusAccepted {
		t.Fatalf("forgot-password = %d, want 202 (body %s)", res.Code, res.Body.String())
	}
	token := e.mailedResetToken(t)

	// Only the hash is stored, so the mailed value cannot be read back out of the table.
	var stored int
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT count(*) FROM auth_token WHERE token_hash = $1`, token).Scan(&stored); err != nil {
		t.Fatalf("count raw-token rows: %v", err)
	}
	if stored != 0 {
		t.Error("the raw token is stored in auth_token; only its SHA-256 may be")
	}

	reset := e.do(t, request{method: http.MethodPost, path: "/v1/public/auth/reset-password",
		body: map[string]any{"token": token, "new_password": nextPassword}})
	if reset.Code != http.StatusNoContent {
		t.Fatalf("reset-password = %d, want 204 (body %s)", reset.Code, reset.Body.String())
	}

	if e.login(t, user.Email, nextPassword) == nil {
		t.Error("the recovered password does not log in")
	}
	if e.login(t, user.Email, seedPassword) != nil {
		t.Error("the old password still logs in after a recovery")
	}
	if me := e.do(t, request{method: http.MethodGet, path: "/v1/me", token: stale.AccessToken}); me.Code != http.StatusUnauthorized {
		t.Errorf("the session open before the recovery = %d, want 401", me.Code)
	}

	replay := e.do(t, request{method: http.MethodPost, path: "/v1/public/auth/reset-password",
		body: map[string]any{"token": token, "new_password": "Otra-clave-mas-3"}})
	if replay.Code != http.StatusUnauthorized {
		t.Fatalf("replaying the link = %d, want 401: it is single use", replay.Code)
	}
}

func TestForgotPassword_RequestingANewLinkRetiresThePrevious(t *testing.T) {
	e := newEnv(t)
	accountID, _ := e.seedAccount(t, "Corralón Segundo Enlace")
	user := e.seedUserWithPassword(t, accountID, domain.UserRoleAdmin, seedPassword)

	e.do(t, request{method: http.MethodPost, path: "/v1/public/auth/forgot-password",
		body: map[string]any{"email": user.Email}})
	first := e.mailedResetToken(t)
	e.do(t, request{method: http.MethodPost, path: "/v1/public/auth/forgot-password",
		body: map[string]any{"email": user.Email}})
	second := e.mailedResetToken(t)

	if first == second {
		t.Fatal("the second request mailed the same token, so nothing was rotated")
	}
	res := e.do(t, request{method: http.MethodPost, path: "/v1/public/auth/reset-password",
		body: map[string]any{"token": first, "new_password": nextPassword}})
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("redeeming the superseded link = %d, want 401", res.Code)
	}
	// Distinct from a refused credential, which answers the same status: the reset screen says
	// "pedí un enlace nuevo" and the login screen says nothing of the sort.
	if got := errorCode(t, res); got != string(domain.CodeInvalidLink) {
		t.Errorf("redeeming the superseded link: code = %q, want %q", got, domain.CodeInvalidLink)
	}
	if e.login(t, user.Email, seedPassword) == nil {
		t.Error("the superseded link changed the password anyway")
	}
}

// The whole point of the uniform answer: an unregistered address must be indistinguishable
// from a registered one, in the status code and in what the system does.
func TestForgotPassword_UnregisteredAddressAnswersTheSame(t *testing.T) {
	e := newEnv(t)
	accountID, _ := e.seedAccount(t, "Corralón Enumeración")
	known := e.seedUserWithPassword(t, accountID, domain.UserRoleAdmin, seedPassword)

	registered := e.do(t, request{method: http.MethodPost, path: "/v1/public/auth/forgot-password",
		body: map[string]any{"email": known.Email}})
	unknown := e.do(t, request{method: http.MethodPost, path: "/v1/public/auth/forgot-password",
		body: map[string]any{"email": uuid.NewString() + "@test.local"}})

	if registered.Code != unknown.Code {
		t.Fatalf("registered address answered %d and unknown one %d; the pair reveals which exist",
			registered.Code, unknown.Code)
	}
	if registered.Body.String() != unknown.Body.String() {
		t.Fatalf("bodies differ: %q vs %q", registered.Body.String(), unknown.Body.String())
	}
}

func TestAdminResetPassword_MailsTheUserAndRecordsTheDelivery(t *testing.T) {
	e := newEnv(t)
	accountID, _ := e.seedAccount(t, "Corralón Reseteo")
	admin := e.seedUserWithPassword(t, accountID, domain.UserRoleAdmin, seedPassword)
	target := e.seedUserWithPassword(t, accountID, domain.UserRoleSeller, seedPassword)

	res := e.do(t, request{method: http.MethodPost,
		path: "/v1/users/" + target.ID.String() + "/password-reset", token: e.tokenFor(t, admin)})
	if res.Code != http.StatusNoContent {
		t.Fatalf("admin reset = %d, want 204 (body %s)", res.Code, res.Body.String())
	}

	msg, ok := e.mail.last()
	if !ok || msg.To != target.Email {
		t.Fatalf("mail went to %q, want the target user at %q", msg.To, target.Email)
	}

	// Every send leaves its record, with the timestamp the ticket asks for.
	var status string
	var sentAt *string
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT status, sent_at::text FROM notification
		 WHERE user_id = $1 AND event = 'PASSWORD_RESET' AND medium = 'EMAIL'`,
		target.ID).Scan(&status, &sentAt); err != nil {
		t.Fatalf("read the notification row: %v", err)
	}
	if status != string(domain.NotificationStatusSent) || sentAt == nil {
		t.Fatalf("notification recorded as %q with sent_at %v, want SENT with a timestamp", status, sentAt)
	}

	// The administrator never sees a password: the link is what changes it.
	token := e.mailedResetToken(t)
	reset := e.do(t, request{method: http.MethodPost, path: "/v1/public/auth/reset-password",
		body: map[string]any{"token": token, "new_password": nextPassword}})
	if reset.Code != http.StatusNoContent {
		t.Fatalf("redeeming the administrator's link = %d, want 204", reset.Code)
	}
	if e.login(t, target.Email, nextPassword) == nil {
		t.Error("the user cannot log in with the password they set from the link")
	}
}

// The reset reaches the administrator's own account and no further. RequireAdmin governs who
// may call it; the account-scoped lookup governs whom they may call it on.
func TestAdminResetPassword_StopsAtTheAccountBoundaryAndAtTheRole(t *testing.T) {
	e := newEnv(t)
	accountA, _ := e.seedAccount(t, "Corralón A reseteo")
	accountB, _ := e.seedAccount(t, "Corralón B reseteo")
	adminA := e.seedUserWithPassword(t, accountA, domain.UserRoleAdmin, seedPassword)
	sellerA := e.seedUserWithPassword(t, accountA, domain.UserRoleSeller, seedPassword)
	userB := e.seedUserWithPassword(t, accountB, domain.UserRoleAdmin, seedPassword)

	foreign := e.do(t, request{method: http.MethodPost,
		path: "/v1/users/" + userB.ID.String() + "/password-reset", token: e.tokenFor(t, adminA)})
	if foreign.Code != http.StatusNotFound {
		t.Errorf("resetting a user of another account = %d, want 404", foreign.Code)
	}

	bySeller := e.do(t, request{method: http.MethodPost,
		path: "/v1/users/" + adminA.ID.String() + "/password-reset", token: e.tokenFor(t, sellerA)})
	if bySeller.Code != http.StatusForbidden {
		t.Errorf("a seller triggering a reset = %d, want 403", bySeller.Code)
	}

	if _, sent := e.mail.last(); sent {
		t.Fatal("a recovery link was mailed by a rejected request")
	}
}

// A bounce is recorded and the operation that caused it still succeeds, which is what keeps a
// provider outage from turning into a failed request.
func TestOutboundMail_DeliveryFailureIsRecordedAndDoesNotFailTheRequest(t *testing.T) {
	e := newEnv(t)
	accountID, _ := e.seedAccount(t, "Corralón Rebote")
	admin := e.seedUserWithPassword(t, accountID, domain.UserRoleAdmin, seedPassword)
	target := e.seedUserWithPassword(t, accountID, domain.UserRoleSeller, seedPassword)
	e.mail.err = errUndeliverable

	res := e.do(t, request{method: http.MethodPost,
		path: "/v1/users/" + target.ID.String() + "/password-reset", token: e.tokenFor(t, admin)})
	if res.Code != http.StatusNoContent {
		t.Fatalf("admin reset with a bouncing transport = %d, want 204 (body %s)",
			res.Code, res.Body.String())
	}

	var status string
	var sentAt *string
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT status, sent_at::text FROM notification WHERE user_id = $1`, target.ID,
	).Scan(&status, &sentAt); err != nil {
		t.Fatalf("read the notification row: %v", err)
	}
	if status != string(domain.NotificationStatusFailed) {
		t.Errorf("notification recorded as %q, want FAILED", status)
	}
	if sentAt != nil {
		t.Errorf("sent_at = %v on a message that never went out, want null", *sentAt)
	}
}
