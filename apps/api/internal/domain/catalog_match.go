package domain

import (
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
	Candidates []CatalogCandidate
}
