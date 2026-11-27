//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

func TestAutomaticClientTags(t *testing.T) {
	e := newEnv(t)
	account, branch := e.seedAccount(t, "Automatic tags")
	otherAccount, _ := e.seedAccount(t, "Other tag owner")
	admin := e.seedUser(t, account, domain.UserRoleAdmin)
	seller := e.seedUser(t, account, domain.UserRoleSeller)
	adminToken, sellerToken := e.tokenFor(t, admin), e.tokenFor(t, seller)
	ctx := context.Background()
	q := e.db.CrossAccount()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := q.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	tags := repository.NewTagRepository(config.DefaultClientTagRules())
	for _, accountID := range []uuid.UUID{account, otherAccount} {
		if err := tags.CreateDefaults(ctx, q, accountID); err != nil {
			t.Fatal(err)
		}
	}
	recurrent, abandoner, foreign := uuid.New(), uuid.New(), uuid.New()
	otherBranch, channel, otherChannel := uuid.New(), uuid.New(), uuid.New()
	exec(`INSERT INTO client (id, account_id, name) VALUES ($1, $4, 'Buyer'), ($2, $4, 'Abandoner'), ($3, $5, 'Foreign')`,
		recurrent, abandoner, foreign, account, otherAccount)
	exec(`INSERT INTO branch (id, account_id, name) VALUES ($1, $2, 'Other branch')`, otherBranch, account)
	exec(`INSERT INTO user_branch (account_id, user_id, branch_id) VALUES ($1, $2, $3)`, account, seller.ID, branch)
	exec(`INSERT INTO channel (id, account_id, branch_id, type)
		VALUES ($1, $3, $4, 'MANUAL_ENTRY'), ($2, $3, $5, 'MANUAL_ENTRY')`,
		channel, otherChannel, account, branch, otherBranch)

	addQuote := func(number int, branchID, channelID, clientID uuid.UUID, status string, archived bool, age int) {
		t.Helper()
		rfqID, quoteID := uuid.New(), uuid.New()
		exec(`INSERT INTO rfq (id, account_id, branch_id, channel_id, client_id, raw_text)
			VALUES ($1, $2, $3, $4, $5, 'Client tag activity')`, rfqID, account, branchID, channelID, clientID)
		exec(`INSERT INTO quote (id, account_id, number, branch_id, client_id, rfq_id, current_status, archived_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7,
			        CASE WHEN $8::boolean THEN now() - $9::integer * interval '1 day' END)`,
			quoteID, account, number, branchID, clientID, rfqID, status, archived, age)
		exec(`INSERT INTO quote_status_change (account_id, quote_id, new_status, changed_at)
			VALUES ($1, $2, $3, now() - $4::integer * interval '1 day')`, account, quoteID, status, age)
	}
	addQuote(1, branch, channel, recurrent, "ACCEPTED", true, 0)
	addQuote(2, branch, channel, recurrent, "ACCEPTED", false, 0)
	addQuote(3, otherBranch, otherChannel, recurrent, "ACCEPTED", false, 10)
	addQuote(4, branch, channel, abandoner, "REJECTED", true, 0)
	addQuote(5, branch, channel, abandoner, "SENT", true, 10)

	call := func(method, path, token string, body any, status int) []byte {
		t.Helper()
		res := e.do(t, request{method: method, path: path, token: token, body: body})
		if res.Code != status {
			t.Fatalf("%s %s = %d %s, want %d", method, path, res.Code, res.Body, status)
		}
		return res.Body.Bytes()
	}
	assertTag := func(clientID uuid.UUID, token, name string, excluded bool) uuid.UUID {
		t.Helper()
		path := "/v1/clients/" + clientID.String()
		var profile dto.ClientProfileResponse
		if err := json.Unmarshal(call(http.MethodGet, path, token, nil, http.StatusOK), &profile); err != nil {
			t.Fatal(err)
		}
		items := profile.Tags
		if excluded {
			items = profile.ExcludedTags
		}
		if name == "" && len(items) == 0 {
			return uuid.Nil
		}
		for _, tag := range items {
			if tag.Automatic && tag.Name == name {
				return tag.ID
			}
		}
		t.Fatalf("profile %s: want automatic tag %q (excluded=%t), got %v", clientID, name, excluded, items)
		return uuid.Nil
	}
	rule := func(kind string, enabled bool, minimum, window int) {
		t.Helper()
		call(http.MethodPut, "/v1/tags/rules/"+kind, adminToken,
			map[string]any{"enabled": enabled, "minimum": minimum, "window_days": window}, http.StatusNoContent)
	}

	call(http.MethodGet, "/v1/tags/rules", sellerToken, nil, http.StatusForbidden)
	call(http.MethodPut, "/v1/tags/rules/RECURRING", sellerToken,
		map[string]any{"enabled": false, "minimum": 1, "window_days": 0}, http.StatusForbidden)
	var rules dto.ClientTagRulesResponse
	if err := json.Unmarshal(call(http.MethodGet, "/v1/tags/rules", adminToken, nil, http.StatusOK), &rules); err != nil {
		t.Fatal(err)
	}
	if len(rules.Items) != 2 {
		t.Fatalf("account rules = %v, want two", rules.Items)
	}
	for _, item := range rules.Items {
		if !item.Enabled || item.Minimum != 3 || item.WindowDays != 0 {
			t.Fatalf("default rule = %+v, want enabled, minimum 3 and full history", item)
		}
	}
	recurringTag := assertTag(recurrent, adminToken, "Recurrente", false)
	assertTag(recurrent, sellerToken, "", false)
	assertTag(abandoner, adminToken, "", false)
	addQuote(6, branch, channel, abandoner, "REJECTED", false, 0)
	assertTag(abandoner, adminToken, "Abandonador", false)

	var directory dto.ClientListResponse
	if err := json.Unmarshal(call(http.MethodGet, "/v1/clients", adminToken, nil, http.StatusOK), &directory); err != nil {
		t.Fatal(err)
	}
	if len(directory.Items) != 2 {
		t.Fatalf("directory = %+v", directory.Items)
	}
	for _, client := range directory.Items {
		want := "Abandonador"
		if client.ID == recurrent {
			want = "Recurrente"
		}
		if len(client.Tags) != 1 || client.Tags[0].Name != want || !client.Tags[0].Automatic {
			t.Fatalf("directory client %s tags = %v, want %s", client.ID, client.Tags, want)
		}
	}

	rule("RECURRING", true, 3, 1)
	assertTag(recurrent, adminToken, "", false)
	rule("RECURRING", true, 2, 1)
	assertTag(recurrent, sellerToken, "Recurrente", false)
	rule("ABANDONER", true, 3, 1)
	assertTag(abandoner, adminToken, "", false)
	rule("ABANDONER", true, 3, 0)
	addQuote(7, branch, channel, abandoner, "ACCEPTED", false, 0)
	assertTag(abandoner, adminToken, "", false)

	profilePath := "/v1/clients/" + recurrent.String()
	exclusionPath := profilePath + "/tag-exclusions/" + recurringTag.String()
	call(http.MethodPut, exclusionPath, sellerToken, map[string]bool{"excluded": true}, http.StatusNoContent)
	addQuote(8, branch, channel, recurrent, "ACCEPTED", false, 0)
	assertTag(recurrent, adminToken, "", false)
	assertTag(recurrent, adminToken, "Recurrente", true)
	rule("RECURRING", false, 2, 1)
	assertTag(recurrent, adminToken, "", false)
	assertTag(recurrent, adminToken, "", true)
	rule("RECURRING", true, 2, 1)
	assertTag(recurrent, sellerToken, "Recurrente", true)
	call(http.MethodPut, exclusionPath, sellerToken, map[string]bool{"excluded": false}, http.StatusNoContent)
	assertTag(recurrent, sellerToken, "Recurrente", false)
	rule("RECURRING", false, 2, 1)
	assertTag(recurrent, adminToken, "", false)

	call(http.MethodPut, "/v1/clients/"+foreign.String()+"/tag-exclusions/"+recurringTag.String(),
		adminToken, map[string]bool{"excluded": true}, http.StatusNotFound)
	call(http.MethodPost, "/v1/tags", sellerToken, map[string]string{"name": "rECURRENTE"},
		http.StatusUnprocessableEntity)
	call(http.MethodPut, profilePath+"/tags", sellerToken, map[string]any{"tag_ids": []uuid.UUID{recurringTag}},
		http.StatusUnprocessableEntity)

	var manualTag dto.ClientTagResponse
	if err := json.Unmarshal(call(http.MethodPost, "/v1/tags", sellerToken,
		map[string]string{"name": "Contractor"}, http.StatusCreated), &manualTag); err != nil {
		t.Fatal(err)
	}
	call(http.MethodPut, profilePath+"/tags", sellerToken, map[string]any{"tag_ids": []uuid.UUID{manualTag.ID}}, http.StatusOK)
	var profile dto.ClientProfileResponse
	if err := json.Unmarshal(call(http.MethodGet, profilePath, sellerToken, nil, http.StatusOK), &profile); err != nil {
		t.Fatal(err)
	}
	if len(profile.Tags) != 1 || profile.Tags[0].ID != manualTag.ID || profile.Tags[0].Automatic {
		t.Fatalf("manual assignment = %v, want %v", profile.Tags, manualTag)
	}
}
