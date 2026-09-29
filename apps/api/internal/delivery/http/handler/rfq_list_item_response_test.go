package handler

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// The list row carries when the follow-up flag was raised, so the queue can word how long a quote
// has been quiet without a detail read per row.
func TestToListItemResponse_CarriesTheFollowupDate(t *testing.T) {
	flaggedAt := time.Date(2026, 9, 27, 9, 0, 0, 0, time.UTC)
	item := domain.RfqListItem{ID: uuid.New(), NeedsFollowup: true, FollowupFlaggedAt: &flaggedAt}

	body, err := json.Marshal(toListItemResponse(item))
	if err != nil {
		t.Fatalf("marshal list row: %v", err)
	}
	var wire map[string]any
	if err := json.Unmarshal(body, &wire); err != nil {
		t.Fatalf("unmarshal list row: %v", err)
	}
	if got := wire["followup_flagged_at"]; got != "2026-09-27T09:00:00Z" {
		t.Errorf("followup_flagged_at = %v, want 2026-09-27T09:00:00Z", got)
	}
	if got := wire["needs_followup"]; got != true {
		t.Errorf("needs_followup = %v, want true", got)
	}
}

func TestToListItemResponse_UnflaggedRowSendsNull(t *testing.T) {
	body, err := json.Marshal(toListItemResponse(domain.RfqListItem{ID: uuid.New()}))
	if err != nil {
		t.Fatalf("marshal list row: %v", err)
	}
	var wire map[string]any
	if err := json.Unmarshal(body, &wire); err != nil {
		t.Fatalf("unmarshal list row: %v", err)
	}
	value, present := wire["followup_flagged_at"]
	if !present || value != nil {
		t.Errorf("followup_flagged_at = %v (present %v), want an explicit null", value, present)
	}
}
