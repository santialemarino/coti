//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestPublicRFQSiteResolvesOnlyRegisteredActiveBranches(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Public site")
	otherID, otherBranch := e.seedAccount(t, "Other public site")
	hostname := "supplier-" + accountID.String() + ".coti.test"
	if _, err := e.db.CrossAccount().Exec(context.Background(), `UPDATE account SET public_hostname = $2 WHERE id = $1`, accountID, hostname); err != nil {
		t.Fatal(err)
	}
	firstChannel, otherChannel := uuid.New(), uuid.New()
	if _, err := e.db.CrossAccount().Exec(context.Background(), `INSERT INTO channel (id, account_id, branch_id, type) VALUES ($1,$2,$3,'WEBAPP'),($4,$5,$6,'WEBAPP')`, firstChannel, accountID, branchID, otherChannel, otherID, otherBranch); err != nil {
		t.Fatal(err)
	}
	secondBranch, secondChannel := uuid.New(), uuid.New()
	if _, err := e.db.CrossAccount().Exec(context.Background(), `WITH b AS (INSERT INTO branch (id,account_id,name) VALUES ($1,$2,'Second branch') RETURNING id) INSERT INTO channel (id,account_id,branch_id,type) SELECT $3,$2,b.id,'WEBAPP' FROM b`, secondBranch, accountID, secondChannel); err != nil {
		t.Fatal(err)
	}
	path := "/v1/public/rfq-sites/" + hostname
	response := e.do(t, request{method: http.MethodGet, path: path})
	var site struct {
		Branches []struct {
			ChannelID uuid.UUID `json:"channel_id"`
		} `json:"branches"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &site); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || len(site.Branches) != 2 || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("site: %d %s", response.Code, response.Body)
	}
	for _, branch := range site.Branches {
		if branch.ChannelID != firstChannel && branch.ChannelID != secondChannel {
			t.Fatal("site exposed another account's branch")
		}
	}
	if _, err := e.db.CrossAccount().Exec(context.Background(), `UPDATE branch SET is_active=false WHERE account_id=$1 AND id=$2`, accountID, secondBranch); err != nil {
		t.Fatal(err)
	}
	response = e.do(t, request{method: http.MethodGet, path: path})
	if err := json.Unmarshal(response.Body.Bytes(), &site); err != nil {
		t.Fatal(err)
	}
	if len(site.Branches) != 1 || site.Branches[0].ChannelID != firstChannel {
		t.Fatalf("inactive branch exposed: %s", response.Body)
	}
	if response := e.do(t, request{method: http.MethodGet, path: path + ".unknown"}); response.Code != http.StatusNotFound {
		t.Fatalf("unknown hostname: %d", response.Code)
	}
	if _, err := e.db.CrossAccount().Exec(context.Background(), `UPDATE account SET is_active=false WHERE id=$1`, accountID); err != nil {
		t.Fatal(err)
	}
	if response := e.do(t, request{method: http.MethodGet, path: path}); response.Code != http.StatusNotFound {
		t.Fatalf("inactive account: %d", response.Code)
	}
}
