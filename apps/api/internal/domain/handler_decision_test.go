package domain

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

func snapshotOf(t *testing.T, quantity string) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(draftOf(quantity))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func draftOf(quantity string) DraftSnapshot {
	return DraftSnapshot{Items: []DraftSnapshotItem{{RequestedDescription: "cemento",
		Quantity: quantity, ApprovedAlternatives: []uuid.UUID{}}}, Discounts: []DraftSnapshotDiscount{}}
}

func run(t *testing.T, outcome HandlerOutcome, before, after string) HandlerDecisionTrace {
	t.Helper()
	return HandlerDecisionTrace{ID: uuid.New(), Outcome: outcome,
		DraftBefore: snapshotOf(t, before), DraftAfter: snapshotOf(t, after)}
}

func TestSettleHandlerDecisions(t *testing.T) {
	t.Parallel()
	rejected := HandlerDecisionRejected

	t.Run("untouched after the AI is approved as is", func(t *testing.T) {
		t.Parallel()
		first := run(t, HandlerOutcomeApplied, "2", "4")
		got := SettleHandlerDecisions([]HandlerDecisionTrace{first}, draftOf("4"))
		if got[first.ID] != HandlerDecisionApprovedAsIs {
			t.Fatalf("decision = %q, want APPROVED_AS_IS", got[first.ID])
		}
	})

	t.Run("a seller edit before sending marks the run edited", func(t *testing.T) {
		t.Parallel()
		first := run(t, HandlerOutcomeApplied, "2", "4")
		got := SettleHandlerDecisions([]HandlerDecisionTrace{first}, draftOf("5"))
		if got[first.ID] != HandlerDecisionEdited {
			t.Fatalf("decision = %q, want EDITED", got[first.ID])
		}
	})

	t.Run("an edit between two runs touches only the earlier one", func(t *testing.T) {
		t.Parallel()
		// The AI set 4, the seller made it 6, the AI then set 8 and the seller sent that.
		first := run(t, HandlerOutcomeApplied, "2", "4")
		second := run(t, HandlerOutcomeApplied, "6", "8")
		got := SettleHandlerDecisions([]HandlerDecisionTrace{first, second}, draftOf("8"))
		if got[first.ID] != HandlerDecisionEdited || got[second.ID] != HandlerDecisionApprovedAsIs {
			t.Fatalf("decisions = %v, want the first EDITED and the second APPROVED_AS_IS", got)
		}
	})

	t.Run("a later AI run is not a seller edit", func(t *testing.T) {
		t.Parallel()
		first := run(t, HandlerOutcomeApplied, "2", "4")
		second := run(t, HandlerOutcomeApplied, "4", "8")
		got := SettleHandlerDecisions([]HandlerDecisionTrace{first, second}, draftOf("8"))
		if got[first.ID] != HandlerDecisionApprovedAsIs || got[second.ID] != HandlerDecisionApprovedAsIs {
			t.Fatalf("decisions = %v, want both APPROVED_AS_IS", got)
		}
	})

	t.Run("a no-match is resolved by hand whatever the seller did", func(t *testing.T) {
		t.Parallel()
		noMatch := run(t, HandlerOutcomeNoMatch, "2", "2")
		got := SettleHandlerDecisions([]HandlerDecisionTrace{noMatch}, draftOf("2"))
		if got[noMatch.ID] != HandlerDecisionManualOverride {
			t.Fatalf("decision = %q, want MANUAL_OVERRIDE", got[noMatch.ID])
		}
	})

	t.Run("a decided run keeps its decision", func(t *testing.T) {
		t.Parallel()
		discarded := run(t, HandlerOutcomeApplied, "2", "4")
		discarded.SellerDecision = &rejected
		got := SettleHandlerDecisions([]HandlerDecisionTrace{discarded}, draftOf("2"))
		if _, ok := got[discarded.ID]; ok {
			t.Fatalf("decisions = %v, want the rejected run left alone", got)
		}
	})

	t.Run("an unreadable snapshot counts as an edit", func(t *testing.T) {
		t.Parallel()
		legacy := HandlerDecisionTrace{ID: uuid.New(), Outcome: HandlerOutcomeApplied}
		got := SettleHandlerDecisions([]HandlerDecisionTrace{legacy}, draftOf("4"))
		if got[legacy.ID] != HandlerDecisionEdited {
			t.Fatalf("decision = %q, want EDITED", got[legacy.ID])
		}
	})
}
