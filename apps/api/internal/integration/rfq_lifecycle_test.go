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

// seedManualEntryChannel inserts the MANUAL_ENTRY channel a branch needs to host RFQs.
func seedManualEntryChannel(t *testing.T, e *env, accountID, branchID uuid.UUID) uuid.UUID {
	t.Helper()
	var channelID uuid.UUID
	err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT id FROM channel
		 WHERE account_id = $1 AND branch_id = $2 AND type = 'MANUAL_ENTRY'`,
		accountID, branchID,
	).Scan(&channelID)
	if err == nil {
		return channelID
	}
	// Channel does not exist yet; create it.
	err = e.db.CrossAccount().QueryRow(context.Background(),
		`INSERT INTO channel (id, account_id, branch_id, type)
		 VALUES ($1, $2, $3, 'MANUAL_ENTRY')
		 RETURNING id`,
		uuid.New(), accountID, branchID,
	).Scan(&channelID)
	if err != nil {
		t.Fatalf("seed manual entry channel: %v", err)
	}
	return channelID
}

func TestRfq_CreateAndList(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "RFQ Lifecycle")
	admin := e.seedUser(t, accountID, domain.UserRoleAdmin)
	token := e.tokenFor(t, admin)
	_ = seedManualEntryChannel(t, e, accountID, branchID)

	clientLabel := "Obra San Martín"
	rawText := "100 ladrillos huecos 8x18x33"
	workType := "obra nueva"

	createBody := map[string]any{
		"client_label": clientLabel,
		"raw_text":     rawText,
		"work_type":    workType,
		"items": []map[string]any{
			{
				"requested_description": "Ladrillo hueco 8x18x33",
				"quantity":              "100",
				"unit":                  "un",
			},
			{
				"requested_description": "Ladrillo semihueco 18x18x33",
				"quantity":              "50",
			},
		},
	}
	body, _ := json.Marshal(createBody)

	createRes := e.do(t, request{
		method: http.MethodPost,
		path:   "/v1/rfqs",
		token:  token,
		branch: branchID.String(),
		body:   json.RawMessage(body),
	})
	if createRes.Code != http.StatusCreated {
		t.Fatalf("POST /v1/rfqs = %d, want 201 (body %s)", createRes.Code, createRes.Body.String())
	}

	var created struct {
		Rfq struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"rfq"`
		Quote struct {
			ID            string `json:"id"`
			CurrentStatus string `json:"current_status"`
		} `json:"quote"`
	}
	if err := json.Unmarshal(createRes.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	if created.Rfq.ID == "" {
		t.Fatal("rfq.id must be present")
	}
	if created.Rfq.Status != "GENERATED" {
		t.Errorf("rfq.status = %q, want GENERATED", created.Rfq.Status)
	}
	if created.Quote.ID == "" {
		t.Fatal("quote.id must be present")
	}
	if created.Quote.CurrentStatus != "DRAFT" {
		t.Errorf("quote.current_status = %q, want DRAFT", created.Quote.CurrentStatus)
	}

	// ── GET /v1/rfqs ──────────────────────────────────────────────────────────

	listRes := e.do(t, request{
		method: http.MethodGet,
		path:   "/v1/rfqs",
		token:  token,
		branch: branchID.String(),
	})
	if listRes.Code != http.StatusOK {
		t.Fatalf("GET /v1/rfqs = %d, want 200 (body %s)", listRes.Code, listRes.Body.String())
	}

	var listBody []struct {
		ID        string  `json:"id"`
		Client    *string `json:"client"`
		Channel   string  `json:"channel"`
		Seller    string  `json:"seller"`
		Branch    string  `json:"branch"`
		ItemCount int     `json:"item_count"`
		Total     *string `json:"total"`
		Status    string  `json:"status"`
	}
	if err := json.Unmarshal(listRes.Body.Bytes(), &listBody); err != nil {
		t.Fatalf("decode list response: %v", err)
	}

	if len(listBody) != 1 {
		t.Fatalf("list items = %d, want 1", len(listBody))
	}

	item := listBody[0]
	if item.ID != created.Rfq.ID {
		t.Errorf("list item id = %s, want %s", item.ID, created.Rfq.ID)
	}
	if item.Client == nil || *item.Client != clientLabel {
		t.Errorf("client = %v, want %q", item.Client, clientLabel)
	}
	if item.Channel != "manual_entry" {
		t.Errorf("channel = %q, want manual_entry", item.Channel)
	}
	if item.ItemCount != 2 {
		t.Errorf("item_count = %d, want 2", item.ItemCount)
	}
	if item.Total != nil {
		t.Errorf("total = %v, want nil (no priced version yet)", item.Total)
	}
	// DRAFT is the quote's current_status, which becomes the merged display status.
	if item.Status != "DRAFT" {
		t.Errorf("status = %q, want DRAFT", item.Status)
	}
}

func TestRfq_ListShowsItemsFromMultipleAccounts(t *testing.T) {
	e := newEnv(t)

	// Account A
	accountA, branchA := e.seedAccount(t, "Corralón A")
	userA := e.seedUser(t, accountA, domain.UserRoleSeller)
	tokenA := e.tokenFor(t, userA)
	_ = seedManualEntryChannel(t, e, accountA, branchA)

	// Account B
	accountB, branchB := e.seedAccount(t, "Corralón B")
	userB := e.seedUser(t, accountB, domain.UserRoleSeller)
	tokenB := e.tokenFor(t, userB)
	_ = seedManualEntryChannel(t, e, accountB, branchB)

	// Create one RFQ in each account.
	for _, tc := range []struct {
		token  string
		branch uuid.UUID
		label  string
	}{
		{tokenA, branchA, "Cliente de A"},
		{tokenB, branchB, "Cliente de B"},
	} {
		raw := "50 metros de cable"
		body, _ := json.Marshal(map[string]any{
			"client_label": tc.label,
			"raw_text":     raw,
			"items": []map[string]any{
				{"requested_description": "Cable 2.5mm", "quantity": "50", "unit": "m"},
			},
		})
		res := e.do(t, request{
			method: http.MethodPost,
			path:   "/v1/rfqs",
			token:  tc.token,
			branch: tc.branch.String(),
			body:   json.RawMessage(body),
		})
		if res.Code != http.StatusCreated {
			t.Fatalf("create RFQ for %s = %d, want 201 (body %s)", tc.label, res.Code, res.Body.String())
		}
	}

	// Each account sees only its own RFQ.
	for _, tc := range []struct {
		name   string
		token  string
		branch uuid.UUID
		count  int
	}{
		{"account A", tokenA, branchA, 1},
		{"account B", tokenB, branchB, 1},
	} {
		res := e.do(t, request{
			method: http.MethodGet,
			path:   "/v1/rfqs",
			token:  tc.token,
			branch: tc.branch.String(),
		})
		if res.Code != http.StatusOK {
			t.Fatalf("list for %s = %d, want 200", tc.name, res.Code)
		}

		var body []struct {
			Client *string `json:"client"`
		}
		if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode list for %s: %v", tc.name, err)
		}
		if len(body) != tc.count {
			t.Errorf("%s: items = %d, want %d", tc.name, len(body), tc.count)
		}
	}
}
