//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/secrets"
)

// whatsAppConfig is the shape a messaging channel takes: the provider's references for the number,
// and the credentials to call it with. The number itself is the identifier column.
func whatsAppConfig() map[string]any {
	return map[string]any{
		"phone_number_id":      "1234567890",
		"business_account_id":  "9876543210",
		"access_token":         "EAAG-live-token",
		"webhook_verify_token": "hook-secret",
	}
}

func emailConfig() map[string]any {
	return map[string]any{
		"mailbox": "pedidos@corralon.test", "smtp_host": "smtp.corralon.test",
		"smtp_port": 587, "smtp_username": "pedidos", "smtp_password": "casilla-secreta",
		"smtp_starttls": true,
	}
}

// storedConfig reads channel.config as it sits in the table, which is the only place a credential
// can be inspected: no read path in the product selects the column.
func (e *env) storedConfig(t *testing.T, channelID uuid.UUID) map[string]any {
	t.Helper()
	var raw []byte
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT config FROM channel WHERE id = $1`, channelID).Scan(&raw); err != nil {
		t.Fatalf("read stored config: %v", err)
	}
	if raw == nil {
		return nil
	}
	var stored map[string]any
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatalf("stored config is not an object: %v", err)
	}
	return stored
}

func (e *env) createChannel(
	t *testing.T, token, branch string, body map[string]any,
) (uuid.UUID, map[string]any) {
	t.Helper()
	rec := e.do(t, request{method: http.MethodPost, path: "/v1/channels",
		token: token, branch: branch, body: body})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /v1/channels = %d %s, want 201", rec.Code, rec.Body)
	}
	var created map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created channel: %v", err)
	}
	id, err := uuid.Parse(created["id"].(string))
	if err != nil {
		t.Fatalf("created channel id %v: %v", created["id"], err)
	}
	t.Cleanup(func() { e.mustCleanup(t, `DELETE FROM channel WHERE id = $1`, id) })
	return id, created
}

// The whole point of the ticket: a credential reaches the database sealed, and never comes back out
// of the API. Both halves are asserted on the same write.
func TestChannels_CredentialsAreSealedAndNeverReturned(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Corralón canales")
	admin := e.seedUser(t, accountID, domain.UserRoleAdmin)
	token := e.tokenFor(t, admin)
	sealer, err := secrets.NewAESGCM(testChannelKey)
	if err != nil {
		t.Fatalf("NewAESGCM() = %v, want no error", err)
	}

	for _, test := range []struct {
		name        string
		channelType string
		config      map[string]any
		clear       []string
		sealed      []string
	}{
		{
			name: "whatsapp", channelType: "WHATSAPP", config: whatsAppConfig(),
			clear:  []string{"phone_number_id", "business_account_id"},
			sealed: []string{"access_token", "webhook_verify_token"},
		},
		{
			name: "email", channelType: "EMAIL", config: emailConfig(),
			clear:  []string{"mailbox", "smtp_host", "smtp_username"},
			sealed: []string{"smtp_password"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			channelID, created := e.createChannel(t, token, branchID.String(), map[string]any{
				"type": test.channelType, "identifier": uuid.NewString(), "config": test.config,
			})

			if created["is_configured"] != true {
				t.Errorf("is_configured = %v, want true", created["is_configured"])
			}
			if _, present := created["config"]; present {
				t.Error("the create response carries a config field, want it omitted entirely")
			}
			stored := e.storedConfig(t, channelID)

			for _, field := range test.clear {
				if stored[field] != test.config[field] {
					t.Errorf("stored %s = %#v, want %#v in the clear", field, stored[field],
						test.config[field])
				}
			}
			for _, field := range test.sealed {
				want := test.config[field].(string)
				got, isText := stored[field].(string)
				if !isText {
					t.Fatalf("stored %s = %#v, want a sealed string", field, stored[field])
				}
				if got == want {
					t.Fatalf("stored %s is the plaintext credential", field)
				}
				opened, openErr := sealer.Open(got)
				if openErr != nil {
					t.Fatalf("Open(stored %s) = %v, want no error", field, openErr)
				}
				if opened != want {
					t.Errorf("Open(stored %s) = %q, want %q", field, opened, want)
				}
			}

			// And no route hands the credential back, whichever read a caller reaches for.
			for _, path := range []string{"/v1/channels", "/v1/channels?include_inactive=true"} {
				rec := e.do(t, request{method: http.MethodGet, path: path,
					token: token, branch: branchID.String()})
				if rec.Code != http.StatusOK {
					t.Fatalf("GET %s = %d %s, want 200", path, rec.Code, rec.Body)
				}
				for _, field := range test.sealed {
					credential := test.config[field].(string)
					if strings.Contains(rec.Body.String(), credential) {
						t.Errorf("GET %s returned the %s credential", path, field)
					}
					if strings.Contains(rec.Body.String(), stored[field].(string)) {
						t.Errorf("GET %s returned the sealed %s envelope", path, field)
					}
				}
			}
		})
	}
}

// AC5, against the shape the seed and migration 00003 actually insert: type only, no identifier,
// no config. All four types have to pass, or a fresh database is invalid the day this ships.
func TestChannels_SeedShapedChannelsPassValidation(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Corralón semilla")
	admin := e.seedUser(t, accountID, domain.UserRoleAdmin)
	token := e.tokenFor(t, admin)

	// The branch has no manual-entry channel: seedAccount inserts the branch directly rather than
	// through the service, so all four are creatable here.
	for _, channelType := range []string{"WHATSAPP", "EMAIL", "WEBAPP", "MANUAL_ENTRY"} {
		t.Run(channelType, func(t *testing.T) {
			channelID, created := e.createChannel(t, token, branchID.String(),
				map[string]any{"type": channelType})

			if created["is_configured"] != false {
				t.Errorf("is_configured = %v, want false for a channel with no config",
					created["is_configured"])
			}
			if created["identifier"] != nil {
				t.Errorf("identifier = %v, want null", created["identifier"])
			}
			if stored := e.storedConfig(t, channelID); stored != nil {
				t.Errorf("stored config = %#v, want NULL", stored)
			}
		})
	}
}

func TestChannels_RefusesAConfigThatDoesNotMatchTheType(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Corralón formas")
	admin := e.seedUser(t, accountID, domain.UserRoleAdmin)
	token := e.tokenFor(t, admin)

	for _, test := range []struct {
		name     string
		body     map[string]any
		wantCode string
	}{
		{name: "email shape on a messaging channel", wantCode: "CHANNEL_CONFIG_SHAPE",
			body: map[string]any{"type": "WHATSAPP", "config": emailConfig()}},
		{name: "messaging shape on a mail channel", wantCode: "CHANNEL_CONFIG_SHAPE",
			body: map[string]any{"type": "EMAIL", "config": whatsAppConfig()}},
		{name: "a config on the public link", wantCode: "CHANNEL_CONFIG_SHAPE",
			body: map[string]any{"type": "WEBAPP", "config": whatsAppConfig()}},
		{name: "a field nobody declared", wantCode: "CHANNEL_CONFIG_SHAPE",
			body: map[string]any{"type": "WHATSAPP", "config": map[string]any{
				"phone_number_id": "1", "access_token": "t", "extra": "x"}}},
		{name: "the identifier smuggled into the config", wantCode: "CHANNEL_CONFIG_SHAPE",
			body: map[string]any{"type": "WHATSAPP", "config": map[string]any{
				"phone_number_id": "1", "access_token": "t", "identifier": "+5491100000000"}}},
		{name: "credentials missing", wantCode: "CHANNEL_CONFIG_SHAPE",
			body: map[string]any{"type": "WHATSAPP", "config": map[string]any{
				"phone_number_id": "1"}}},
		{name: "an identifier on the public link", wantCode: "CHANNEL_IDENTIFIER",
			body: map[string]any{"type": "WEBAPP", "identifier": "https://corralon.test"}},
		{name: "an identifier on manual entry", wantCode: "CHANNEL_IDENTIFIER",
			body: map[string]any{"type": "MANUAL_ENTRY", "identifier": "mostrador"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			rec := e.do(t, request{method: http.MethodPost, path: "/v1/channels",
				token: token, branch: branchID.String(), body: test.body})
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("POST /v1/channels = %d %s, want 422", rec.Code, rec.Body)
			}
			if got := errorCode(t, rec); got != test.wantCode {
				t.Errorf("code = %q, want %q (%s)", got, test.wantCode, rec.Body)
			}
			e.assertNoChannels(t, branchID)
		})
	}
}

// assertNoChannels fails when a refused write left a row behind.
func (e *env) assertNoChannels(t *testing.T, branchID uuid.UUID) {
	t.Helper()
	var count int
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT count(*) FROM channel WHERE branch_id = $1`, branchID).Scan(&count); err != nil {
		t.Fatalf("count channels: %v", err)
	}
	if count != 0 {
		t.Errorf("branch holds %d channels, want none: a refused write persisted", count)
	}
}

func TestChannels_UpdateReplacesTheWholeConfigAndNullRemovesIt(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Corralón actualización")
	admin := e.seedUser(t, accountID, domain.UserRoleAdmin)
	token := e.tokenFor(t, admin)
	identifier := uuid.NewString()
	channelID, _ := e.createChannel(t, token, branchID.String(), map[string]any{
		"type": "WHATSAPP", "identifier": identifier, "config": whatsAppConfig(),
	})
	path := "/v1/channels/" + channelID.String()

	// Sending no config leaves the stored one where it is, so editing the identifier cannot
	// silently discard a credential the API never showed anyone.
	before := e.storedConfig(t, channelID)
	rec := e.do(t, request{method: http.MethodPut, path: path, token: token,
		branch: branchID.String(), body: map[string]any{"identifier": identifier}})
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT = %d %s, want 200", rec.Code, rec.Body)
	}
	after := e.storedConfig(t, channelID)
	if len(after) != len(before) {
		t.Fatalf("stored config = %#v, want the %d fields it had", after, len(before))
	}
	for field, value := range before {
		if after[field] != value {
			t.Errorf("stored %s = %#v, want %#v", field, after[field], value)
		}
	}

	// A config sent replaces the whole thing: the optional fields it omits are gone, not merged.
	rec = e.do(t, request{method: http.MethodPut, path: path, token: token,
		branch: branchID.String(), body: map[string]any{
			"identifier": identifier,
			"config":     map[string]any{"phone_number_id": "5555", "access_token": "second"},
		}})
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT = %d %s, want 200", rec.Code, rec.Body)
	}
	replaced := e.storedConfig(t, channelID)
	if len(replaced) != 2 {
		t.Errorf("stored config = %#v, want only the two fields sent", replaced)
	}
	if replaced["phone_number_id"] != "5555" {
		t.Errorf("stored phone_number_id = %#v, want %q", replaced["phone_number_id"], "5555")
	}

	// An explicit null removes it, which is the only way to say so without a credential to resend.
	rec = e.do(t, request{method: http.MethodPut, path: path, token: token,
		branch: branchID.String(), body: map[string]any{
			"identifier": identifier, "config": nil,
		}})
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT = %d %s, want 200", rec.Code, rec.Body)
	}
	if stored := e.storedConfig(t, channelID); stored != nil {
		t.Errorf("stored config = %#v, want NULL", stored)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["is_configured"] != false {
		t.Errorf("is_configured = %v, want false", body["is_configured"])
	}
}

func TestChannels_ClosingIsSoftAndSparesTheManualEntryRoute(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Corralón cierre")
	admin := e.seedUser(t, accountID, domain.UserRoleAdmin)
	token := e.tokenFor(t, admin)
	whatsAppID, _ := e.createChannel(t, token, branchID.String(), map[string]any{
		"type": "WHATSAPP", "identifier": uuid.NewString(),
	})
	manualID, _ := e.createChannel(t, token, branchID.String(),
		map[string]any{"type": "MANUAL_ENTRY"})

	rec := e.do(t, request{method: http.MethodDelete,
		path: "/v1/channels/" + manualID.String(), token: token, branch: branchID.String()})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("DELETE the manual-entry channel = %d %s, want 422", rec.Code, rec.Body)
	}
	if got := errorCode(t, rec); got != "MANUAL_ENTRY_CHANNEL" {
		t.Errorf("code = %q, want MANUAL_ENTRY_CHANNEL", got)
	}

	// The same guard has to hold on the flag, or PUT is a way around DELETE.
	rec = e.do(t, request{method: http.MethodPut, path: "/v1/channels/" + manualID.String(),
		token: token, branch: branchID.String(), body: map[string]any{"is_active": false}})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("PUT is_active=false on the manual-entry channel = %d %s, want 422",
			rec.Code, rec.Body)
	}
	if got := errorCode(t, rec); got != "MANUAL_ENTRY_CHANNEL" {
		t.Errorf("code = %q, want MANUAL_ENTRY_CHANNEL", got)
	}

	rec = e.do(t, request{method: http.MethodDelete,
		path: "/v1/channels/" + whatsAppID.String(), token: token, branch: branchID.String()})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE = %d %s, want 204", rec.Code, rec.Body)
	}

	// Closed, not gone: the row survives so the orders that arrived through it stay explainable,
	// and only the administrative list still shows it.
	var isActive bool
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT is_active FROM channel WHERE id = $1`, whatsAppID).Scan(&isActive); err != nil {
		t.Fatalf("read closed channel: %v", err)
	}
	if isActive {
		t.Error("is_active = true after DELETE, want the row closed")
	}
	if ids := e.listChannelIDs(t, token, branchID, false); len(ids) != 1 || ids[0] != manualID {
		t.Errorf("active list = %v, want only the manual-entry channel", ids)
	}
	if ids := e.listChannelIDs(t, token, branchID, true); len(ids) != 2 {
		t.Errorf("administrative list = %v, want both channels", ids)
	}
}

func (e *env) listChannelIDs(
	t *testing.T, token string, branchID uuid.UUID, includeInactive bool,
) []uuid.UUID {
	t.Helper()
	path := "/v1/channels"
	if includeInactive {
		path += "?include_inactive=true"
	}
	rec := e.do(t, request{method: http.MethodGet, path: path, token: token,
		branch: branchID.String()})
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s = %d %s, want 200", path, rec.Code, rec.Body)
	}
	var body struct {
		Items []struct {
			ID uuid.UUID `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode channel list: %v", err)
	}
	ids := make([]uuid.UUID, 0, len(body.Items))
	for _, item := range body.Items {
		ids = append(ids, item.ID)
	}
	return ids
}

// Reading the list is every seller's business — a text draft has to name its channel. Configuring
// one is not.
func TestChannels_SellerReadsTheListAndWritesNothing(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Corralón vendedor")
	seller := e.seedUser(t, accountID, domain.UserRoleSeller)
	e.assignBranch(t, accountID, seller, branchID)
	token := e.tokenFor(t, seller)
	channelID := e.seedIntakeChannel(t, accountID, branchID)

	if ids := e.listChannelIDs(t, token, branchID, false); len(ids) != 1 || ids[0] != channelID {
		t.Errorf("GET /v1/channels = %v, want the branch's channel %v", ids, channelID)
	}

	for _, test := range []struct {
		method string
		path   string
		body   any
	}{
		{http.MethodPost, "/v1/channels", map[string]any{"type": "WEBAPP"}},
		{http.MethodPut, "/v1/channels/" + channelID.String(), map[string]any{}},
		{http.MethodDelete, "/v1/channels/" + channelID.String(), nil},
	} {
		t.Run(test.method, func(t *testing.T) {
			rec := e.do(t, request{method: test.method, path: test.path, token: token,
				branch: branchID.String(), body: test.body})
			if rec.Code != http.StatusForbidden {
				t.Fatalf("%s %s = %d %s, want 403", test.method, test.path, rec.Code, rec.Body)
			}
		})
	}

	// The administrative read is refused too, and by the service rather than the router: it hangs
	// off the same route a seller may reach.
	rec := e.do(t, request{method: http.MethodGet, path: "/v1/channels?include_inactive=true",
		token: token, branch: branchID.String()})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("GET /v1/channels?include_inactive=true = %d %s, want 403", rec.Code, rec.Body)
	}
}

// With no key configured the product still runs: what it refuses is storing a credential, and it
// says so with a 503 rather than pretending the request was malformed.
func TestChannels_WithNoEncryptionKeyRefusesCredentialsOnly(t *testing.T) {
	e := newEnv(t, func(cfg *config.Config) {
		cfg.Channel = config.ChannelConfig{}
	})
	accountID, branchID := e.seedAccount(t, "Corralón sin clave")
	admin := e.seedUser(t, accountID, domain.UserRoleAdmin)
	token := e.tokenFor(t, admin)

	rec := e.do(t, request{method: http.MethodPost, path: "/v1/channels", token: token,
		branch: branchID.String(), body: map[string]any{
			"type": "WHATSAPP", "identifier": uuid.NewString(), "config": whatsAppConfig(),
		}})
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("POST with a credential and no key = %d %s, want 503", rec.Code, rec.Body)
	}
	if got := errorCode(t, rec); got != "NOT_CONFIGURED" {
		t.Errorf("code = %q, want NOT_CONFIGURED", got)
	}
	if strings.Contains(rec.Body.String(), "CHANNEL_CONFIG_ENCRYPTION_KEY") {
		t.Error("the response names the missing setting, want that left to the log")
	}
	e.assertNoChannels(t, branchID)

	// A channel that needs no credential is unaffected, so the branch can still be set up.
	e.createChannel(t, token, branchID.String(), map[string]any{"type": "WEBAPP"})
}
