//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

func TestClientContextActivityAndInternalNotes(t *testing.T) {
	e := newEnv(t)
	account, branch := e.seedAccount(t, "Client context")
	otherAccount, _ := e.seedAccount(t, "Foreign client context")
	seller := e.seedUser(t, account, domain.UserRoleSeller)
	otherSeller := e.seedUser(t, account, domain.UserRoleSeller)
	token := e.tokenFor(t, seller)
	ctx := context.Background()
	q := e.db.CrossAccount()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := q.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}

	const taxID = "30712345671"
	hostname, customerToken := e.registerCustomer(t, account, taxID)
	var clientID uuid.UUID
	if err := q.QueryRow(ctx, `SELECT id FROM client WHERE account_id = $1 AND tax_id = $2`, account, taxID).Scan(&clientID); err != nil {
		t.Fatal(err)
	}
	foreignID, otherBranch, channel, otherChannel := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec(`INSERT INTO client (id, account_id, name, notes) VALUES ($1, $2, 'Foreign client', 'Keep this')`, foreignID, otherAccount)
	exec(`INSERT INTO branch (id, account_id, name) VALUES ($1, $2, 'Outside reach')`, otherBranch, account)
	exec(`INSERT INTO user_branch (account_id, user_id, branch_id) VALUES ($1, $2, $3)`, account, seller.ID, branch)
	exec(`INSERT INTO channel (id, account_id, branch_id, type)
		VALUES ($1, $3, $4, 'MANUAL_ENTRY'), ($2, $3, $5, 'MANUAL_ENTRY')`,
		channel, otherChannel, account, branch, otherBranch)
	lastAccepted := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)

	addQuote := func(number int, branchID, channelID, sellerID uuid.UUID, status, total string, eventAt time.Time, archived, unconfirmed bool) uuid.UUID {
		t.Helper()
		rfqID, quoteID, versionID := uuid.New(), uuid.New(), uuid.New()
		exec(`INSERT INTO rfq (id, account_id, branch_id, channel_id, client_id, raw_text)
			VALUES ($1, $2, $3, $4, $5, 'Client context activity')`, rfqID, account, branchID, channelID, clientID)
		exec(`INSERT INTO quote (id, account_id, number, branch_id, client_id, rfq_id, seller_id,
			current_status, archived_at, acceptance_unconfirmed_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8,
				CASE WHEN $9::boolean THEN $11::timestamptz END,
				CASE WHEN $10::boolean THEN $11::timestamptz END)`,
			quoteID, account, number, branchID, clientID, rfqID, sellerID, status, archived, unconfirmed, eventAt)
		exec(`INSERT INTO quote_version (id, account_id, quote_id, version_number, total)
			VALUES ($1, $2, $3, 1, $4::text::numeric)`, versionID, account, quoteID, total)
		exec(`UPDATE quote SET current_version_id = $2 WHERE id = $1`, quoteID, versionID)
		exec(`INSERT INTO quote_status_change (account_id, quote_id, new_status, changed_at)
			VALUES ($1, $2, $3, $4)`, account, quoteID, status, eventAt)
		return rfqID
	}
	addQuote(1, branch, channel, seller.ID, "ACCEPTED", "100.00", lastAccepted.Add(-time.Hour), false, false)
	addQuote(2, branch, channel, seller.ID, "ACCEPTED", "250.50", lastAccepted, true, false)
	addQuote(3, branch, channel, seller.ID, "DRAFT", "60.00", lastAccepted, false, false)
	addQuote(4, branch, channel, seller.ID, "SENT", "90.00", lastAccepted, true, false)
	addQuote(5, branch, channel, otherSeller.ID, "SENT", "400.00", lastAccepted, false, false)
	addQuote(6, otherBranch, otherChannel, seller.ID, "ACCEPTED", "600.00", lastAccepted.Add(time.Hour), false, false)
	addQuote(7, branch, channel, seller.ID, "ACCEPTED", "700.00", lastAccepted.Add(time.Hour), false, true)
	addQuote(9, branch, channel, seller.ID, "QUOTED", "30.00", lastAccepted, true, false)

	profilePath := "/v1/clients/" + clientID.String()
	getProfile := func() dto.ClientProfileResponse {
		t.Helper()
		response := e.do(t, request{method: http.MethodGet, path: profilePath, token: token})
		var profile dto.ClientProfileResponse
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &profile) != nil {
			t.Fatalf("get profile: %d %s", response.Code, response.Body)
		}
		return profile
	}
	profile := getProfile()
	activity := profile.Activity
	if !profile.Registered || activity.OpenQuotes != 1 || activity.AcceptedCount != 2 ||
		activity.AverageTicket == nil || *activity.AverageTicket != "175.25" ||
		activity.LastAcceptedAt == nil || !activity.LastAcceptedAt.Equal(lastAccepted) {
		t.Fatalf("profile activity: registered=%v open=%d accepted=%d last=%v average=%v",
			profile.Registered, activity.OpenQuotes, activity.AcceptedCount, activity.LastAcceptedAt, activity.AverageTicket)
	}

	const note = "Call before delivery\nUse the side entrance"
	writeNotes := func(body any, expected int) {
		t.Helper()
		res := e.do(t, request{method: http.MethodPut, path: profilePath + "/notes", token: token, body: body})
		if res.Code != expected {
			t.Fatalf("update notes: %d %s, want %d", res.Code, res.Body, expected)
		}
	}
	writeNotes(map[string]string{"notes": "  " + note + "  "}, http.StatusOK)
	writeNotes(map[string]string{}, http.StatusUnprocessableEntity)
	writeNotes(map[string]string{"notes": "invalid\ttext"}, http.StatusUnprocessableEntity)
	nextRFQ := addQuote(8, branch, channel, seller.ID, "QUOTED", "80.00", lastAccepted, false, false)
	order := e.do(t, request{method: http.MethodGet, path: "/v1/rfqs/" + nextRFQ.String(), token: token})
	profile = getProfile()
	if order.Code != http.StatusOK || !strings.Contains(order.Body.String(), clientID.String()) ||
		profile.Client.Notes == nil || *profile.Client.Notes != note || profile.Activity.OpenQuotes != 2 {
		t.Fatalf("next order and profile did not retain notes: order=%d profile=%+v", order.Code, profile)
	}
	publicProfile := e.do(t, request{method: http.MethodGet, path: "/v1/customer/sites/" + hostname + "/profile", token: customerToken})
	if publicProfile.Code != http.StatusOK || strings.Contains(publicProfile.Body.String(), "Call before delivery") ||
		strings.Contains(publicProfile.Body.String(), `"notes"`) {
		t.Fatalf("customer profile exposed internal notes or failed: %d %s", publicProfile.Code, publicProfile.Body)
	}
	writeNotes(map[string]string{"notes": "  "}, http.StatusOK)
	if notes := getProfile().Client.Notes; notes != nil {
		t.Fatalf("notes were not cleared: %q", *notes)
	}

	tx, err := e.db.AdminTx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	_, err = repository.NewClientRepository().UpdateNotes(ctx, tx, account, foreignID, nil)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("cross-account notes update = %v, want ErrNotFound", err)
	}
}
