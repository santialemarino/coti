package services

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

var presetBase = time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)

func presetSend(version uuid.UUID, channel domain.ChannelType, destination string, minutes int,
	status domain.SendTrackingStatus, purpose domain.QuoteDeliveryPurpose) domain.QuoteSend {
	at := presetBase.Add(time.Duration(minutes) * time.Minute)
	return domain.QuoteSend{VersionID: version, ChannelType: channel, Destination: destination,
		TrackingStatus: status, Purpose: purpose, CreatedAt: at, SentAt: &at}
}

func TestDeliveryPreset(t *testing.T) {
	current, previous := uuid.New(), uuid.New()
	sent, failed := domain.SendTrackingStatusSent, domain.SendTrackingStatusFailed
	quote, followup := domain.QuoteDeliveryPurposeQuote, domain.QuoteDeliveryPurposeFollowup
	wa, mail, portal := domain.ChannelTypeWhatsApp, domain.ChannelTypeEmail, domain.ChannelTypeWebApp
	// ListByQuote returns the newest send first.
	history := []domain.QuoteSend{
		presetSend(current, wa, "+5491100000003", 50, sent, followup),
		presetSend(current, mail, "ana@example.com", 40, sent, quote),
		presetSend(current, wa, "+5491100000002", 30, failed, quote),
		presetSend(current, wa, "+5491100000001", 20, sent, quote),
		presetSend(current, portal, "customer-portal", 20, sent, quote),
		presetSend(previous, wa, "+5491100000009", 10, sent, quote),
		presetSend(previous, mail, "old@example.com", 5, sent, quote),
	}

	cases := []struct {
		name       string
		mode       domain.QuoteDeliveryMode
		sends      []domain.QuoteSend
		start      time.Time
		sameClient bool
		want       domain.QuoteDeliveryPreset
	}{
		{
			name: "a resend reuses every successful destination of the cycle, never reminders or failures",
			mode: domain.QuoteDeliveryModeResend, sends: history, sameClient: true,
			want: domain.QuoteDeliveryPreset{Source: domain.QuoteDeliveryPresetVersion,
				Phones: []string{"+5491100000001"}, Emails: []string{"ana@example.com"},
				UsedPhones: []string{"+5491100000001"}, UsedEmails: []string{"ana@example.com"}},
		},
		{
			name: "a reminder preselects what the quote reached",
			mode: domain.QuoteDeliveryModeFollowup, sends: history, sameClient: true,
			want: domain.QuoteDeliveryPreset{Source: domain.QuoteDeliveryPresetVersion,
				Phones: []string{"+5491100000001"}, Emails: []string{"ana@example.com"},
				UsedPhones: []string{"+5491100000001"}, UsedEmails: []string{"ana@example.com"}},
		},
		{
			name: "otro medio preselects nothing and reports what is used",
			mode: domain.QuoteDeliveryModeOtherMedium, sends: history, sameClient: true,
			want: domain.QuoteDeliveryPreset{Source: domain.QuoteDeliveryPresetNone,
				UsedPhones: []string{"+5491100000001"}, UsedEmails: []string{"ana@example.com"}},
		},
		{
			name: "a reactivated cycle forgets what went out before it for otro medio",
			mode: domain.QuoteDeliveryModeOtherMedium, sends: history, sameClient: true,
			start: presetBase.Add(35 * time.Minute),
			want: domain.QuoteDeliveryPreset{Source: domain.QuoteDeliveryPresetNone,
				UsedPhones: []string{}, UsedEmails: []string{"ana@example.com"}},
		},
		{
			name: "a first send of a reactivated version reuses that version's destinations",
			mode: domain.QuoteDeliveryModeFirst, sends: history, sameClient: true,
			start: presetBase.Add(60 * time.Minute),
			want: domain.QuoteDeliveryPreset{Source: domain.QuoteDeliveryPresetVersion,
				Phones: []string{"+5491100000001"}, Emails: []string{"ana@example.com"},
				UsedPhones: []string{}, UsedEmails: []string{}},
		},
		{
			name: "a first send of a new version reuses the last sent version",
			mode: domain.QuoteDeliveryModeFirst, sends: history[5:], sameClient: true,
			want: domain.QuoteDeliveryPreset{Source: domain.QuoteDeliveryPresetPreviousVersion,
				Phones: []string{"+5491100000009"}, Emails: []string{"old@example.com"},
				UsedPhones: []string{}, UsedEmails: []string{}},
		},
		{
			name: "another client in the dialog never inherits the quote's destinations",
			mode: domain.QuoteDeliveryModeFirst, sends: history[5:], sameClient: false,
			want: domain.QuoteDeliveryPreset{Source: domain.QuoteDeliveryPresetNone,
				UsedPhones: []string{}, UsedEmails: []string{}},
		},
		{
			name: "a quote never sent has no preset",
			mode: domain.QuoteDeliveryModeFirst, sameClient: true,
			want: domain.QuoteDeliveryPreset{Source: domain.QuoteDeliveryPresetNone,
				UsedPhones: []string{}, UsedEmails: []string{}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := deliveryPreset(tc.mode, tc.sends, current, tc.start, tc.sameClient)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

type statusHistory struct {
	quoteDeliveryQuoteRepository
	changes []domain.QuoteStatusChange
}

func (s statusHistory) ListStatusChanges(context.Context, repository.Querier, uuid.UUID, uuid.UUID, uuid.UUID) ([]domain.QuoteStatusChange, error) {
	return s.changes, nil
}

// A reactivated quote re-enters QUOTED with the same version: its send cycle starts there.
func TestQuoteDeliveryService_CycleStartIsTheLastEntryIntoQuoted(t *testing.T) {
	change := func(status domain.QuoteStatus, minutes int) domain.QuoteStatusChange {
		return domain.QuoteStatusChange{NewStatus: status, ChangedAt: presetBase.Add(time.Duration(minutes) * time.Minute)}
	}
	service := &QuoteDeliveryService{quotes: statusHistory{changes: []domain.QuoteStatusChange{
		change(domain.QuoteStatusQuoted, 10), change(domain.QuoteStatusSent, 20),
		change(domain.QuoteStatusAccepted, 30), change(domain.QuoteStatusQuoted, 40), change(domain.QuoteStatusSent, 50),
	}}}
	start, err := service.cycleStart(context.Background(), nil, domain.Tenant{}, uuid.New())
	if err != nil || !start.Equal(presetBase.Add(40*time.Minute)) {
		t.Fatalf("cycle start = %v, %v; want the reactivation at +40m", start, err)
	}
}

func TestNormalizeQuoteDeliveryInputMode(t *testing.T) {
	message := "Seguimos a disposición."
	base := domain.QuoteDeliveryInput{IdempotencyKey: uuid.New(), Phones: []string{"+5491100000001"}}
	cases := []struct {
		name    string
		mutate  func(*domain.QuoteDeliveryInput)
		want    domain.QuoteDeliveryMode
		invalid bool
	}{
		{name: "a legacy reminder becomes a FOLLOWUP", mutate: func(in *domain.QuoteDeliveryInput) {
			in.FollowupMessage = &message
		}, want: domain.QuoteDeliveryModeFollowup},
		{name: "a legacy other-channels flag becomes OTHER_MEDIUM", mutate: func(in *domain.QuoteDeliveryInput) {
			in.OtherChannelsOnly = true
		}, want: domain.QuoteDeliveryModeOtherMedium},
		{name: "no mode is left for the status to decide", mutate: func(*domain.QuoteDeliveryInput) {}},
		{name: "an unknown mode is refused", mutate: func(in *domain.QuoteDeliveryInput) {
			in.Mode = "AGAIN"
		}, invalid: true},
		{name: "a FOLLOWUP without a message is refused", mutate: func(in *domain.QuoteDeliveryInput) {
			in.Mode = domain.QuoteDeliveryModeFollowup
		}, invalid: true},
		{name: "a message outside a FOLLOWUP is refused", mutate: func(in *domain.QuoteDeliveryInput) {
			in.Mode, in.FollowupMessage = domain.QuoteDeliveryModeResend, &message
		}, invalid: true},
		{name: "the other-channels flag on another mode is refused", mutate: func(in *domain.QuoteDeliveryInput) {
			in.Mode, in.OtherChannelsOnly = domain.QuoteDeliveryModeResend, true
		}, invalid: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := base
			tc.mutate(&in)
			got, err := normalizeQuoteDeliveryInput(in)
			if tc.invalid {
				if !errors.Is(err, domain.ErrInvalidInput) {
					t.Fatalf("err = %v, want invalid input", err)
				}
				return
			}
			if err != nil || got.Mode != tc.want {
				t.Fatalf("mode = %q, err = %v, want %q", got.Mode, err, tc.want)
			}
			if got.OtherChannelsOnly != (tc.want == domain.QuoteDeliveryModeOtherMedium) {
				t.Fatalf("other channels flag = %v for mode %q", got.OtherChannelsOnly, got.Mode)
			}
		})
	}
}
