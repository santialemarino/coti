package domain

import (
	"testing"

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
