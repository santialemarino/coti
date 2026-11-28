package domain

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// ReplyAcceptance is what the backend made of an acceptance read from a customer's reply.
type ReplyAcceptance string

const (
	// ReplyAcceptanceAutomatic accepted the quote on its own.
	ReplyAcceptanceAutomatic ReplyAcceptance = "AUTOMATIC"
	// ReplyAcceptanceUnconfirmed accepted the quote, pending the seller's confirmation.
	ReplyAcceptanceUnconfirmed ReplyAcceptance = "UNCONFIRMED"
	// ReplyAcceptanceSignal left the quote SENT and told the seller the customer seems to accept.
	ReplyAcceptanceSignal ReplyAcceptance = "SIGNAL"
)

// ReplyAcceptanceHold is one reason an acceptance did not go through on its own.
type ReplyAcceptanceHold string

// Hard holds keep the quote SENT; soft ones accept it pending confirmation.
const (
	ReplyAcceptanceHoldNotSent         ReplyAcceptanceHold = "NOT_SENT"
	ReplyAcceptanceHoldPortalRequest   ReplyAcceptanceHold = "PORTAL_REQUEST"
	ReplyAcceptanceHoldOtherLiveQuotes ReplyAcceptanceHold = "OTHER_LIVE_QUOTES"
	ReplyAcceptanceHoldSendNotLive     ReplyAcceptanceHold = "SEND_NOT_LIVE"
	ReplyAcceptanceHoldLowConfidence   ReplyAcceptanceHold = "LOW_CONFIDENCE"

	ReplyAcceptanceHoldBelowAutomatic       ReplyAcceptanceHold = "BELOW_AUTOMATIC_CONFIDENCE"
	ReplyAcceptanceHoldPartialOrAlternative ReplyAcceptanceHold = "PARTIAL_OR_ALTERNATIVE"
	ReplyAcceptanceHoldAlternativesOffered  ReplyAcceptanceHold = "ALTERNATIVES_OFFERED"
	ReplyAcceptanceHoldWording              ReplyAcceptanceHold = "WORDING"
	ReplyAcceptanceHoldAutoAcceptOff        ReplyAcceptanceHold = "AUTO_ACCEPT_OFF"
)

// ReplyAcceptanceFacts is what the backend knows, under the quote's lock, about a reply the model
// read as an acceptance.
type ReplyAcceptanceFacts struct {
	Text              string
	ConfidencePercent int
	// PartialOrAlternative takes part of the quote, or one of its alternatives.
	PartialOrAlternative bool
	// QuoteSent is whether the quote is SENT and unarchived now.
	QuoteSent bool
	// PortalRequest is a batch carrying the customer's «Pedir cambios» from the portal.
	PortalRequest bool
	// OtherLiveQuotes counts the other quotes the sender could have meant.
	OtherLiveQuotes int
	// SendLive is whether the current version is the one the latest successful send carried, and
	// that send has not expired.
	SendLive bool
	// AlternativesOffered is a sent version whose lines carry alternatives for the customer to pick.
	AlternativesOffered bool
	AutoAcceptOn        bool
}

// ReplyAcceptancePolicy holds the thresholds decision 91 leaves configurable.
type ReplyAcceptancePolicy struct {
	AutomaticMinConfidencePercent int
	ReviewMinConfidencePercent    int
	MaxWords                      int
}

// Decide weighs an acceptance: any hard hold keeps the quote SENT with a signal, any soft one
// accepts it pending the seller, and none accepts it on its own.
func (p ReplyAcceptancePolicy) Decide(f ReplyAcceptanceFacts) (ReplyAcceptance, []ReplyAcceptanceHold) {
	hard := make([]ReplyAcceptanceHold, 0)
	if !f.QuoteSent {
		hard = append(hard, ReplyAcceptanceHoldNotSent)
	}
	if f.PortalRequest {
		hard = append(hard, ReplyAcceptanceHoldPortalRequest)
	}
	if f.OtherLiveQuotes > 0 {
		hard = append(hard, ReplyAcceptanceHoldOtherLiveQuotes)
	}
	if !f.SendLive {
		hard = append(hard, ReplyAcceptanceHoldSendNotLive)
	}
	if f.ConfidencePercent < p.ReviewMinConfidencePercent {
		hard = append(hard, ReplyAcceptanceHoldLowConfidence)
	}
	if len(hard) > 0 {
		return ReplyAcceptanceSignal, hard
	}
	soft := make([]ReplyAcceptanceHold, 0)
	if f.ConfidencePercent < p.AutomaticMinConfidencePercent {
		soft = append(soft, ReplyAcceptanceHoldBelowAutomatic)
	}
	if f.PartialOrAlternative {
		soft = append(soft, ReplyAcceptanceHoldPartialOrAlternative)
	}
	if f.AlternativesOffered {
		soft = append(soft, ReplyAcceptanceHoldAlternativesOffered)
	}
	if !IsPlainAcceptance(f.Text, p.MaxWords) {
		soft = append(soft, ReplyAcceptanceHoldWording)
	}
	if !f.AutoAcceptOn {
		soft = append(soft, ReplyAcceptanceHoldAutoAcceptOff)
	}
	if len(soft) > 0 {
		return ReplyAcceptanceUnconfirmed, soft
	}
	return ReplyAcceptanceAutomatic, nil
}

// acceptancePhrases is the fixed wording an automatic acceptance must use, accents removed.
var acceptancePhrases = []string{
	"dale", "listo", "acepto", "aceptamos", "aceptado", "confirmo", "confirmamos", "confirmado",
	"lo compro", "lo compramos", "lo llevo", "lo llevamos", "va", "de acuerdo", "trato hecho",
	"adelante", "mandale", "mandalo", "cerramos", "perfecto", "buenisimo", "me sirve",
}

// negationWords turn an acceptance phrase around («no me sirve»), so a reply carrying one is never
// plain, whatever the model read.
var negationWords = map[string]bool{"no": true, "ni": true, "nunca": true, "tampoco": true}

// IsPlainAcceptance reports whether text is short, at most maxWords words, uses one of the fixed
// acceptance phrases as whole words, and carries no negation.
func IsPlainAcceptance(text string, maxWords int) bool {
	words := acceptanceWords(text)
	if len(words) == 0 || len(words) > maxWords {
		return false
	}
	for _, word := range words {
		if negationWords[word] {
			return false
		}
	}
	joined := " " + strings.Join(words, " ") + " "
	for _, phrase := range acceptancePhrases {
		if strings.Contains(joined, " "+phrase+" ") {
			return true
		}
	}
	return false
}

// acceptanceWords lowercases text, drops accents, and splits it on anything not a letter or digit.
func acceptanceWords(text string) []string {
	var plain strings.Builder
	for _, r := range norm.NFD.String(strings.ToLower(text)) {
		switch {
		case unicode.Is(unicode.Mn, r):
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			plain.WriteRune(r)
		default:
			plain.WriteRune(' ')
		}
	}
	return strings.Fields(plain.String())
}
