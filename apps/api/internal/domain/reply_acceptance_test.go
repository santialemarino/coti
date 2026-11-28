package domain

import (
	"slices"
	"testing"
)

func plainFacts() ReplyAcceptanceFacts {
	return ReplyAcceptanceFacts{Text: "Dale, lo llevo", ConfidencePercent: 95, QuoteSent: true,
		SendLive: true, AutoAcceptOn: true}
}

func TestReplyAcceptancePolicy_Decide(t *testing.T) {
	t.Parallel()
	policy := ReplyAcceptancePolicy{AutomaticMinConfidencePercent: 90, ReviewMinConfidencePercent: 60,
		MaxWords: 8}
	cases := []struct {
		name  string
		edit  func(*ReplyAcceptanceFacts)
		want  ReplyAcceptance
		holds []ReplyAcceptanceHold
	}{
		{"plain and sure", func(*ReplyAcceptanceFacts) {}, ReplyAcceptanceAutomatic, nil},
		{"at the automatic threshold", func(f *ReplyAcceptanceFacts) { f.ConfidencePercent = 90 },
			ReplyAcceptanceAutomatic, nil},
		{"below the automatic threshold", func(f *ReplyAcceptanceFacts) { f.ConfidencePercent = 89 },
			ReplyAcceptanceUnconfirmed, []ReplyAcceptanceHold{ReplyAcceptanceHoldBelowAutomatic}},
		{"at the review threshold", func(f *ReplyAcceptanceFacts) { f.ConfidencePercent = 60 },
			ReplyAcceptanceUnconfirmed, []ReplyAcceptanceHold{ReplyAcceptanceHoldBelowAutomatic}},
		{"below review", func(f *ReplyAcceptanceFacts) { f.ConfidencePercent = 59 },
			ReplyAcceptanceSignal, []ReplyAcceptanceHold{ReplyAcceptanceHoldLowConfidence}},
		{"partial", func(f *ReplyAcceptanceFacts) { f.PartialOrAlternative = true },
			ReplyAcceptanceUnconfirmed, []ReplyAcceptanceHold{ReplyAcceptanceHoldPartialOrAlternative}},
		{"alternatives offered", func(f *ReplyAcceptanceFacts) { f.AlternativesOffered = true },
			ReplyAcceptanceUnconfirmed, []ReplyAcceptanceHold{ReplyAcceptanceHoldAlternativesOffered}},
		{"unlisted wording", func(f *ReplyAcceptanceFacts) { f.Text = "me parece bien todo" },
			ReplyAcceptanceUnconfirmed, []ReplyAcceptanceHold{ReplyAcceptanceHoldWording}},
		{"switch off", func(f *ReplyAcceptanceFacts) { f.AutoAcceptOn = false },
			ReplyAcceptanceUnconfirmed, []ReplyAcceptanceHold{ReplyAcceptanceHoldAutoAcceptOff}},
		{"another live quote", func(f *ReplyAcceptanceFacts) { f.OtherLiveQuotes = 1 },
			ReplyAcceptanceSignal, []ReplyAcceptanceHold{ReplyAcceptanceHoldOtherLiveQuotes}},
		{"send not live", func(f *ReplyAcceptanceFacts) { f.SendLive = false },
			ReplyAcceptanceSignal, []ReplyAcceptanceHold{ReplyAcceptanceHoldSendNotLive}},
		{"portal request", func(f *ReplyAcceptanceFacts) { f.PortalRequest = true },
			ReplyAcceptanceSignal, []ReplyAcceptanceHold{ReplyAcceptanceHoldPortalRequest}},
		{"not sent", func(f *ReplyAcceptanceFacts) { f.QuoteSent = false },
			ReplyAcceptanceSignal, []ReplyAcceptanceHold{ReplyAcceptanceHoldNotSent}},
		{"a hard hold wins over soft ones", func(f *ReplyAcceptanceFacts) {
			f.OtherLiveQuotes, f.AutoAcceptOn = 2, false
		}, ReplyAcceptanceSignal, []ReplyAcceptanceHold{ReplyAcceptanceHoldOtherLiveQuotes}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			facts := plainFacts()
			tc.edit(&facts)
			got, holds := policy.Decide(facts)
			if got != tc.want || !slices.Equal(holds, tc.holds) {
				t.Errorf("Decide() = %s %v, want %s %v", got, holds, tc.want, tc.holds)
			}
		})
	}
}

func TestIsPlainAcceptance(t *testing.T) {
	t.Parallel()
	for text, want := range map[string]bool{
		"dale":                          true,
		"Listo, lo compro!":             true,
		"Vamos para adelante, me gusta": true,
		"CONFIRMÓ":                      true,
		"confirmo 👍":                    true,
		"Buenísimo":                     true,
		"me parece bien":                false,
		"no me sirve":                   false,
		"no acepto":                     false,
		"todavía no confirmo":           false,
		"vale la pena pensarlo":         false,
		"dale, pero antes decime si tienen el flete incluido": false,
		"": false,
	} {
		if got := IsPlainAcceptance(text, 8); got != want {
			t.Errorf("IsPlainAcceptance(%q) = %v, want %v", text, got, want)
		}
	}
}
