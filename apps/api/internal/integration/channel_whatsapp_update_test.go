//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/secrets"
)

func TestChannels_WhatsAppPartialUpdatePreservesCredentialsAndTenantScope(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "WhatsApp settings")
	admin := e.seedUser(t, accountID, domain.UserRoleAdmin)
	seller := e.seedUser(t, accountID, domain.UserRoleSeller)
	otherAccountID, otherBranchID := e.seedAccount(t, "Other WhatsApp settings")
	otherAdmin := e.seedUser(t, otherAccountID, domain.UserRoleAdmin)
	token := e.tokenFor(t, admin)
	configuration := whatsAppConfig()
	configuration["phone_number_id"] = "123456789012345"
	channelID, _ := e.createChannel(t, token, branchID.String(), map[string]any{
		"type": "WHATSAPP", "identifier": "+5491155550101", "config": configuration,
	})
	before := e.storedConfig(t, channelID)
	path := "/v1/channels/" + channelID.String() + "/whatsapp"
	rec := e.do(t, request{method: http.MethodPatch, path: path, token: token,
		branch: branchID.String(), body: map[string]any{"access_token": "rotated-access-token"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("rotate token = %d %s", rec.Code, rec.Body)
	}
	var response map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if _, present := response["config"]; present || response["identifier"] != "+5491155550101" {
		t.Fatal("rotation returned configuration or changed the number")
	}
	after := e.storedConfig(t, channelID)
	for _, field := range []string{"phone_number_id", "business_account_id", "webhook_verify_token"} {
		if after[field] != before[field] {
			t.Fatalf("rotation changed omitted %s", field)
		}
	}
	sealer, err := secrets.NewAESGCM(testChannelKey)
	if err != nil {
		t.Fatal(err)
	}
	opened, err := sealer.Open(after["access_token"].(string))
	if err != nil || opened != "rotated-access-token" || strings.Contains(rec.Body.String(), opened) {
		t.Fatal("rotation did not seal the token or returned its plaintext")
	}
	for _, test := range []struct {
		name   string
		token  string
		branch string
		body   map[string]any
		status int
	}{
		{"anonymous", "", branchID.String(), map[string]any{"access_token": "denied"}, http.StatusUnauthorized},
		{"seller", e.tokenFor(t, seller), branchID.String(), map[string]any{"access_token": "denied"}, http.StatusForbidden},
		{"other account", e.tokenFor(t, otherAdmin), otherBranchID.String(), map[string]any{"access_token": "denied"}, http.StatusNotFound},
		{"other branch", token, otherBranchID.String(), map[string]any{"access_token": "denied"}, http.StatusForbidden},
		{"blank token", token, branchID.String(), map[string]any{"access_token": ""}, http.StatusUnprocessableEntity},
		{"oversized token", token, branchID.String(), map[string]any{"access_token": strings.Repeat("ñ", 2049)}, http.StatusUnprocessableEntity},
		{"incomplete number change", token, branchID.String(), map[string]any{"identifier": "+541143210000"}, http.StatusUnprocessableEntity},
	} {
		t.Run(test.name, func(t *testing.T) {
			rec := e.do(t, request{method: http.MethodPatch, path: path, token: test.token,
				branch: test.branch, body: test.body})
			if rec.Code != test.status {
				t.Fatalf("status = %d %s, want %d", rec.Code, rec.Body, test.status)
			}
			unchanged := e.storedConfig(t, channelID)
			if unchanged["access_token"] != after["access_token"] || unchanged["webhook_verify_token"] != after["webhook_verify_token"] {
				t.Fatal("refused update changed stored credentials")
			}
		})
	}
	rec = e.do(t, request{method: http.MethodPatch, path: path, token: token,
		branch: branchID.String(), body: map[string]any{"identifier": "+541143210000",
			"phone_number_id": "222222222222222", "business_account_id": "333333333333333", "access_token": "real-line-token"}})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "+541143210000") {
		t.Fatalf("switch to real landline = %d %s", rec.Code, rec.Body)
	}
	if e.storedConfig(t, channelID)["webhook_verify_token"] != before["webhook_verify_token"] {
		t.Fatal("switching the sending number discarded its retained webhook verifier")
	}
}
