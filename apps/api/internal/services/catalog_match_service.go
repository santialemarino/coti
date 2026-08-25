package services

import (
	"context"
	"fmt"
	"math"

	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// catalogSearcher is the search surface matching needs. Defined here, in the consumer, so a
// test can decide statuses from staged candidates with neither a database nor a provider.
type catalogSearcher interface {
	Search(ctx context.Context, tenant domain.Tenant, in domain.CatalogSearch) (
		[]domain.CatalogSearchResult, error)
}

// confidenceScale is quote_item.confidence_score's own: NUMERIC(5,4).
const confidenceScale int32 = 4

// CatalogMatchService turns the candidates a catalog search offers into a decision per line —
// which product, how confident, and whether the line needs the seller's eye.
type CatalogMatchService struct {
	search catalogSearcher
	// minConfidence is the floor the leading candidate clears to be a match at all.
	minConfidence decimal.Decimal
	// ambiguityMargin is how far the leading candidate sits above the runner-up before the line
	// counts as decided rather than as a choice between two products.
	ambiguityMargin decimal.Decimal
	// lexicalConfidence is the confidence floor for a candidate carrying lexical evidence.
	lexicalConfidence decimal.Decimal
}

// NewCatalogMatchService builds a CatalogMatchService.
func NewCatalogMatchService(search catalogSearcher, cfg config.CatalogConfig) *CatalogMatchService {
	return &CatalogMatchService{
		search:            search,
		minConfidence:     percentAsRatio(cfg.MatchMinConfidencePercent),
		ambiguityMargin:   percentAsRatio(cfg.MatchAmbiguityMarginPercent),
		lexicalConfidence: percentAsRatio(cfg.MatchLexicalConfidencePercent),
	}
}

// Match resolves every description against the catalog, returning decisions index-aligned with
// descriptions. Every line comes back, including the ones nothing matched.
func (s *CatalogMatchService) Match(
	ctx context.Context, tenant domain.Tenant, descriptions []string,
) ([]domain.LineMatch, error) {
	if len(descriptions) == 0 {
		return nil, nil
	}
	// One search for the whole set: it embeds every line in a single provider call and reads the
	// catalog in one transaction, neither of which survives being called per line.
	results, err := s.search.Search(ctx, tenant, domain.CatalogSearch{Texts: descriptions})
	if err != nil {
		return nil, err
	}
	// Pairing a line with another line's candidates is a wrong match nothing downstream could
	// notice, so a broken alignment is refused rather than indexed into.
	if len(results) != len(descriptions) {
		return nil, fmt.Errorf("catalog search returned %d results for %d lines",
			len(results), len(descriptions))
	}

	matches := make([]domain.LineMatch, len(results))
	for i, result := range results {
		matches[i] = s.decide(result.Candidates)
	}
	return matches, nil
}

// decide reads the leading candidate and the one behind it, and turns the pair into a status.
func (s *CatalogMatchService) decide(candidates []domain.CatalogCandidate) domain.LineMatch {
	scored := s.scoreAll(candidates)
	if len(scored) == 0 {
		return domain.LineMatch{
			MatchStatus: domain.ItemMatchStatusNoMatch,
			Confidence:  decimal.Zero,
			Candidates:  scored,
		}
	}

	leader := scored[0]
	if leader.Confidence.LessThan(s.minConfidence) {
		return domain.LineMatch{
			MatchStatus: domain.ItemMatchStatusNoMatch,
			Confidence:  leader.Confidence,
			Candidates:  scored,
		}
	}

	status := domain.ItemMatchStatusMatched
	if len(scored) > 1 {
		// Negative when the two halves disagree about which product this is — the runner-up is
		// the closer vector and the leader only won on the fused rank. That is an ambiguous line.
		margin := leader.Confidence.Sub(scored[1].Confidence)
		if margin.LessThan(s.ambiguityMargin) {
			status = domain.ItemMatchStatusAmbiguous
		}
	}
	productID := leader.ProductID
	return domain.LineMatch{
		ProductID:   &productID,
		MatchStatus: status,
		Confidence:  leader.Confidence,
		Candidates:  scored,
	}
}

// scoreAll reads every candidate at its confidence, keeping the search's order. Scoring the whole
// set rather than the leading pair is what lets a flagged line show what each offer was worth.
func (s *CatalogMatchService) scoreAll(candidates []domain.CatalogCandidate) []domain.ScoredCandidate {
	if len(candidates) == 0 {
		return nil
	}
	scored := make([]domain.ScoredCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		scored = append(scored, domain.ScoredCandidate{
			CatalogCandidate: candidate,
			Confidence:       s.confidenceOf(candidate),
		})
	}
	return scored
}

// confidenceOf scores one candidate on 0..1. Rounding here rather than on the way to the
// database is what makes the persisted number the one the decision was taken on.
func (s *CatalogMatchService) confidenceOf(candidate domain.CatalogCandidate) decimal.Decimal {
	if candidate.Distance == nil {
		if candidate.LexicalScore != nil {
			return s.lexicalConfidence.Round(confidenceScale)
		}
		return decimal.Zero
	}
	// A product stored with a zero-length vector comes back at NaN, which the decimal package
	// refuses outright. Lexical evidence remains valid even when its vector is unusable.
	if math.IsNaN(*candidate.Distance) || math.IsInf(*candidate.Distance, 0) {
		if candidate.LexicalScore != nil {
			return s.lexicalConfidence.Round(confidenceScale)
		}
		return decimal.Zero
	}
	similarity := decimal.NewFromInt(1).Sub(decimal.NewFromFloat(*candidate.Distance))
	similarity = decimal.Min(decimal.Max(similarity, decimal.Zero), decimal.NewFromInt(1))
	if candidate.LexicalScore != nil {
		similarity = decimal.Max(similarity, s.lexicalConfidence)
	}
	return similarity.Round(confidenceScale)
}

// percentAsRatio turns a configured whole percentage into the 0..1 scale the scores live on.
func percentAsRatio(value int) decimal.Decimal {
	return decimal.NewFromInt(int64(value)).Div(decimal.NewFromInt(100))
}
