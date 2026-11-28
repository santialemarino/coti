package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestRfqDetail_AcceptedByReply(t *testing.T) {
	current, earlier, batch := uuid.New(), uuid.New(), uuid.New()
	accepted := &Quote{CurrentStatus: QuoteStatusAccepted, CurrentVersionID: &current}
	cases := []struct {
		name   string
		quote  *Quote
		action ClientAction
		want   bool
	}{
		{"a reply's acceptance of the current version", accepted,
			ClientAction{Type: ClientActionAccept, VersionID: current, MessageBatchID: &batch}, true},
		{"the customer's own click", accepted,
			ClientAction{Type: ClientActionAccept, VersionID: current}, false},
		{"a reply's acceptance of an earlier version", accepted,
			ClientAction{Type: ClientActionAccept, VersionID: earlier, MessageBatchID: &batch}, false},
		{"a quote no longer accepted",
			&Quote{CurrentStatus: QuoteStatusSent, CurrentVersionID: &current},
			ClientAction{Type: ClientActionAccept, VersionID: current, MessageBatchID: &batch}, false},
		{"a rejection read from a reply", accepted,
			ClientAction{Type: ClientActionReject, VersionID: current, MessageBatchID: &batch}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			detail := RfqDetail{Quote: tc.quote, ClientActions: []ClientAction{tc.action}}
			if got := detail.AcceptedByReply(); got != tc.want {
				t.Errorf("AcceptedByReply() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRfqDetail_AcceptedByReply_IgnoresAReopenedCycle(t *testing.T) {
	current, batch := uuid.New(), uuid.New()
	accepted, quoted := QuoteStatusAccepted, QuoteStatusQuoted
	start := time.Now()
	reply := ClientAction{Type: ClientActionAccept, VersionID: current, MessageBatchID: &batch,
		CreatedAt: start}
	reopened := QuoteStatusChange{PreviousStatus: &accepted, NewStatus: quoted,
		ChangedAt: start.Add(time.Minute)}
	click := ClientAction{Type: ClientActionAccept, VersionID: current, CreatedAt: start.Add(time.Hour)}
	quote := &Quote{CurrentStatus: QuoteStatusAccepted, CurrentVersionID: &current}
	for name, detail := range map[string]RfqDetail{
		"the seller accepted after a reactivation": {Quote: quote, ClientActions: []ClientAction{reply},
			QuoteStatusChanges: []QuoteStatusChange{reopened}},
		"the customer clicked in the new cycle": {Quote: quote, ClientActions: []ClientAction{reply, click},
			QuoteStatusChanges: []QuoteStatusChange{reopened}},
	} {
		if detail.AcceptedByReply() {
			t.Errorf("%s: AcceptedByReply() = true, want the old reply acceptance ignored", name)
		}
	}
}
