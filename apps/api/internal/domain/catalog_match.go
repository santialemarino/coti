package domain

import (
	"context"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ItemMatchStatus is the catalog-match outcome for a quote line. A line with no acceptable
// candidate is flagged NO_MATCH with a null product, never discarded.
type ItemMatchStatus string

const (
	ItemMatchStatusMatched   ItemMatchStatus = "MATCHED"
	ItemMatchStatusAmbiguous ItemMatchStatus = "AMBIGUOUS"
	ItemMatchStatusNoMatch   ItemMatchStatus = "NO_MATCH"
)

// LineMatch is what one RFQ line resolved to against the catalog: the product it was matched
// against, how confident that is, and the candidates the decision was taken from.
type LineMatch struct {
	// ProductID is the leading candidate's product, and is nil on NO_MATCH.
	ProductID   *uuid.UUID
	MatchStatus ItemMatchStatus
	// Confidence is the leading candidate's similarity, on 0..1 with four decimals so it fits
	// quote_item.confidence_score exactly. A rejected line keeps the score of the candidate it
	// rejected, which is what tells a near miss from a line nothing came close to; only a line
	// offered no candidate at all scores zero.
	Confidence decimal.Decimal
	// Candidates are the search's offers, best first, kept so the seller can pick another and
	// the unmatched-items report can show what was considered.
	Candidates []ScoredCandidate
	// SettledByReview marks a line the language model's review decided. A MATCHED one keeps its
	// candidates on offer, since the choice between them was the model's, not the text's.
	SettledByReview bool
}

// ScoredCandidate is one offer the matcher weighed, carrying the confidence it read the candidate
// at. The search supplies the evidence and the matcher turns it into this figure, so a line can
// show what each candidate was worth rather than only what the leader scored.
type ScoredCandidate struct {
	CatalogCandidate
	Confidence decimal.Decimal
}

// MatchReviewVerdict is what a language model says about one flagged line and the candidates it
// was shown. The model only ever chooses among those candidates; saying none of them fits is a
// valid answer, not an error.
type MatchReviewVerdict string

const (
	// MatchReviewVerdictOne means the line names exactly one of the candidates.
	MatchReviewVerdictOne MatchReviewVerdict = "ONE"
	// MatchReviewVerdictSeveral means several candidates fit and the line does not say which.
	MatchReviewVerdictSeveral MatchReviewVerdict = "SEVERAL"
	// MatchReviewVerdictNone means no candidate is what the line asks for.
	MatchReviewVerdictNone MatchReviewVerdict = "NONE"
)

// MatchReviewLine is one line the matcher could not settle, with the candidates it offered.
type MatchReviewLine struct {
	Description string
	Candidates  []MatchReviewCandidate
}

// MatchReviewCandidate is one catalog item a reviewed line was offered.
type MatchReviewCandidate struct {
	Name        string
	Description *string
	Unit        *string
}

// MatchReviewDecision is the review of one line. Chosen indexes the line's own candidates, best
// first: one entry on ONE, the fitting ones on SEVERAL, none on NONE.
type MatchReviewDecision struct {
	Verdict MatchReviewVerdict
	Chosen  []int
	// Reason is the reviewer's one sentence for the seller on why.
	Reason string
}

// CatalogMatchReviewer asks a language model to settle the lines matching flagged, reading trade
// knowledge the catalog text does not carry. Decisions come back index-aligned with lines.
type CatalogMatchReviewer interface {
	Review(ctx context.Context, lines []MatchReviewLine) ([]MatchReviewDecision, error)
}

// MatchConfidenceLevel is how settled a line's catalog match reads to the seller: the status and
// the score folded into the one answer the review screen shows.
type MatchConfidenceLevel string

const (
	MatchConfidenceHigh   MatchConfidenceLevel = "HIGH"
	MatchConfidenceMedium MatchConfidenceLevel = "MEDIUM"
	MatchConfidenceLow    MatchConfidenceLevel = "LOW"
)

// ConfidenceLevelOf reads a line's level: HIGH only for a MATCHED line at or above high, MEDIUM for
// the rest of MATCHED and AMBIGUOUS, LOW for NO_MATCH, and none for a line nobody scored.
func ConfidenceLevelOf(
	status ItemMatchStatus, score decimal.NullDecimal, high decimal.Decimal,
) *MatchConfidenceLevel {
	if !score.Valid {
		return nil
	}
	level := MatchConfidenceLow
	switch status {
	case ItemMatchStatusMatched:
		level = MatchConfidenceMedium
		if score.Decimal.GreaterThanOrEqual(high) {
			level = MatchConfidenceHigh
		}
	case ItemMatchStatusAmbiguous:
		level = MatchConfidenceMedium
	}
	return &level
}
