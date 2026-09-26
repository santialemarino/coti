package services

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"slices"
	"strings"

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

// coverageTieTolerance is how close two coverages sit to count as the same answer to a line: one
// credit a word earns from a near spelling rather than its exact one.
const coverageTieTolerance = 0.02

// coverageTieReach is how many ambiguity margins below the leader a rival that answers the line
// as fully still contests it. Past that the vectors disagree clearly enough to decide.
const coverageTieReach = 3

// unaskedFigurePenalty is what a candidate loses when none of its figures was asked for: enough to
// order two products that answer a line equally, never enough to beat one that answers it better.
const unaskedFigurePenalty = 0.05

// CatalogMatchService turns the candidates a catalog search offers into a decision per line —
// which product, how confident, and whether the line needs the seller's eye.
type CatalogMatchService struct {
	search catalogSearcher
	// topK is how many candidates a line keeps; pool is how many it scores to choose them.
	topK int
	pool int
	// minConfidence is the floor the leading candidate clears to be a match at all.
	minConfidence decimal.Decimal
	// ambiguityMargin is the lead over the runner-up that makes a line decided, not a choice.
	ambiguityMargin decimal.Decimal
	// coverageWeight is coverage's share of the confidence; calibrated similarity carries the rest.
	coverageWeight float64
	// similarityFloor and similarityCeiling calibrate raw cosine similarity onto 0..1.
	similarityFloor   float64
	similarityCeiling float64
	// reviewer settles flagged lines with trade knowledge; nil leaves every decision to the text.
	reviewer       domain.CatalogMatchReviewer
	reviewFloor    decimal.Decimal
	reviewMaxLines int
	log            *slog.Logger
}

// NewCatalogMatchService builds a CatalogMatchService.
func NewCatalogMatchService(search catalogSearcher, cfg config.CatalogConfig) *CatalogMatchService {
	return &CatalogMatchService{
		search:            search,
		topK:              cfg.SearchTopK,
		pool:              cfg.SearchTopK * cfg.SearchOverFetchFactor,
		minConfidence:     percentAsRatio(cfg.MatchMinConfidencePercent),
		ambiguityMargin:   percentAsRatio(cfg.MatchAmbiguityMarginPercent),
		coverageWeight:    float64(cfg.MatchCoverageWeightPercent) / 100,
		similarityFloor:   float64(cfg.MatchSimilarityFloorPercent) / 100,
		similarityCeiling: float64(cfg.MatchSimilarityCeilingPercent) / 100,
		reviewFloor:       percentAsRatio(cfg.MatchReviewFloorPercent),
		reviewMaxLines:    cfg.MatchReviewMaxLines,
		log:               slog.Default(),
	}
}

// WithReviewer has flagged lines settled by a language model. A review that fails is logged and
// changes nothing.
func (s *CatalogMatchService) WithReviewer(
	reviewer domain.CatalogMatchReviewer, log *slog.Logger,
) *CatalogMatchService {
	s.reviewer = reviewer
	if log != nil {
		s.log = log
	}
	return s
}

// Match resolves every description against the catalog, returning decisions index-aligned with
// descriptions. Every line comes back, including the ones nothing matched.
func (s *CatalogMatchService) Match(
	ctx context.Context, tenant domain.Tenant, descriptions []string,
) ([]domain.LineMatch, error) {
	if len(descriptions) == 0 {
		return nil, nil
	}
	ctx = domain.WithAIAccount(ctx, tenant)
	// One search for the whole set: it embeds every line in a single provider call and reads the
	// catalog in one transaction, neither of which survives being called per line.
	results, err := s.search.Search(ctx, tenant,
		domain.CatalogSearch{Texts: descriptions, Limit: s.pool})
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
		matches[i] = s.decide(lineTokens(descriptions[i]), result.Candidates)
	}
	s.review(ctx, descriptions, matches)
	return matches, nil
}

// decide ranks the candidates and turns the leader and its rivals into a status.
func (s *CatalogMatchService) decide(
	line []matchToken, candidates []domain.CatalogCandidate,
) domain.LineMatch {
	ranked := s.rank(line, candidates)
	scored := make([]domain.ScoredCandidate, len(ranked))
	for i, r := range ranked {
		scored[i] = r.ScoredCandidate
	}
	if len(scored) == 0 {
		return domain.LineMatch{
			MatchStatus: domain.ItemMatchStatusNoMatch,
			Confidence:  decimal.Zero,
			Candidates:  scored,
		}
	}

	leader := ranked[0]
	if leader.Confidence.LessThan(s.minConfidence) {
		return domain.LineMatch{
			MatchStatus: domain.ItemMatchStatusNoMatch,
			Confidence:  leader.Confidence,
			Candidates:  scored,
		}
	}

	status := domain.ItemMatchStatusMatched
	if len(ranked) > 1 && s.contested(ranked) {
		status = domain.ItemMatchStatusAmbiguous
	}
	productID := leader.ProductID
	return domain.LineMatch{
		ProductID:   &productID,
		MatchStatus: status,
		Confidence:  leader.Confidence,
		Candidates:  scored,
	}
}

// contested reports whether the leader has a rival: one within the margin, or one within reach that
// answers the line as fully with no extra spec — then the line never said which of them it wants.
func (s *CatalogMatchService) contested(ranked []rankedCandidate) bool {
	leader, runnerUp := ranked[0], ranked[1]
	// Two seller-taught answers for one phrase are a disagreement only a person settles.
	if leader.LearnedDistance != nil {
		return runnerUp.LearnedDistance != nil
	}
	if leader.Confidence.Sub(runnerUp.Confidence).LessThan(s.ambiguityMargin) {
		return true
	}
	reach := s.ambiguityMargin.Mul(decimal.NewFromInt(coverageTieReach))
	for _, rival := range ranked[1:] {
		if !leader.Confidence.Sub(rival.Confidence).LessThan(reach) {
			break
		}
		answersAsFully := rival.coverage >= leader.coverage-coverageTieTolerance
		if answersAsFully && rival.unasked <= leader.unasked {
			return true
		}
	}
	return false
}

// rankedCandidate carries the coverage and unasked-spec share behind a score, for contested.
type rankedCandidate struct {
	domain.ScoredCandidate
	coverage float64
	unasked  float64
}

// rank orders every candidate by its confidence and keeps the best topK, so the runner-up is the
// closest competitor rather than whichever product the search happened to put second.
func (s *CatalogMatchService) rank(
	line []matchToken, candidates []domain.CatalogCandidate,
) []rankedCandidate {
	ranked := make([]rankedCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		confidence, coverage, unasked := s.confidenceOf(line, candidate)
		ranked = append(ranked, rankedCandidate{
			ScoredCandidate: domain.ScoredCandidate{
				CatalogCandidate: candidate, Confidence: confidence,
			},
			coverage: coverage,
			unasked:  unasked,
		})
	}
	slices.SortStableFunc(ranked, func(a, b rankedCandidate) int {
		// A phrase a seller already resolved outranks anything the catalog text suggests.
		if (a.LearnedDistance != nil) != (b.LearnedDistance != nil) {
			if a.LearnedDistance != nil {
				return -1
			}
			return 1
		}
		if order := b.Confidence.Cmp(a.Confidence); order != 0 {
			return order
		}
		return strings.Compare(a.CanonicalName, b.CanonicalName)
	})
	if len(ranked) > s.topK {
		ranked = ranked[:s.topK]
	}
	return ranked
}

// confidenceOf blends coverage with calibrated similarity, less a little per unasked spec. It
// rounds before any comparison, so the persisted score is exactly the one decided on.
func (s *CatalogMatchService) confidenceOf(
	line []matchToken, candidate domain.CatalogCandidate,
) (decimal.Decimal, float64, float64) {
	document := candidateTokens(candidate)
	coverage := catalogCoverage(line, document)
	// A product with no vector yet, or a zero-length one (read back as NaN), is not evidence of
	// dissimilarity: its missing half reads as middling agreement with what its text says.
	similarity := coverage / 2
	if d := candidate.Distance; d != nil && !math.IsNaN(*d) && !math.IsInf(*d, 0) {
		similarity = calibratedSimilarity(1-*d, s.similarityFloor, s.similarityCeiling)
	}
	confidence := s.coverageWeight*coverage + (1-s.coverageWeight)*similarity
	// Only a line that names a spec says anything about the specs it did not name.
	unasked := 0.0
	if slices.ContainsFunc(line, func(t matchToken) bool { return t.kind == tokenFigure }) {
		unasked = unaskedFigureShare(line, document)
	}
	confidence -= unaskedFigurePenalty * unasked
	if d := candidate.LearnedDistance; d != nil && !math.IsNaN(*d) && !math.IsInf(*d, 0) {
		confidence = math.Max(confidence, 1-*d)
	}
	confidence = math.Min(math.Max(confidence, 0), 1)
	return decimal.NewFromFloat(confidence).Round(confidenceScale), coverage, unasked
}

// candidateTokens is the text a candidate answers a line with: its name, its description and the
// trade terms of its own that the line used.
func candidateTokens(candidate domain.CatalogCandidate) []matchToken {
	parts := append([]string{candidate.CanonicalName}, candidate.Synonyms...)
	if candidate.Description != nil {
		parts = append(parts, *candidate.Description)
	}
	return tokenizeCatalogText(strings.Join(parts, " "))
}

// review hands the lines the text left flagged to the reviewer and applies its verdicts. A line
// whose best offer is under the review floor, or led by a seller-taught answer, is not sent.
func (s *CatalogMatchService) review(
	ctx context.Context, descriptions []string, matches []domain.LineMatch,
) {
	if s.reviewer == nil || s.reviewMaxLines == 0 {
		return
	}
	var pending []int
	for i, match := range matches {
		if len(pending) == s.reviewMaxLines {
			break
		}
		if match.MatchStatus == domain.ItemMatchStatusMatched || len(match.Candidates) == 0 ||
			match.Candidates[0].Confidence.LessThan(s.reviewFloor) ||
			match.Candidates[0].LearnedDistance != nil {
			continue
		}
		pending = append(pending, i)
	}
	if len(pending) == 0 {
		return
	}
	lines := make([]domain.MatchReviewLine, len(pending))
	for i, index := range pending {
		lines[i] = reviewLineOf(descriptions[index], matches[index])
	}
	decisions, err := s.reviewer.Review(ctx, lines)
	if err != nil {
		s.log.WarnContext(ctx, "catalog match review did not run; the flagged lines stand",
			slog.Any("error", err), slog.Int("lines", len(lines)))
		return
	}
	if len(decisions) != len(lines) {
		s.log.ErrorContext(ctx, "catalog match review returned a different number of decisions",
			slog.Int("decisions", len(decisions)), slog.Int("lines", len(lines)))
		return
	}
	for i, index := range pending {
		matches[index] = s.applyReview(matches[index], decisions[i])
		// The reason is not stored anywhere yet, so the log is the trace of why a line moved. It
		// names the line by position: the client's words stay out of the log.
		if decisions[i].Verdict != "" {
			s.log.InfoContext(ctx, "catalog match reviewed", slog.Int("line", index),
				slog.String("verdict", string(decisions[i].Verdict)),
				slog.String("status", string(matches[index].MatchStatus)),
				slog.String("reason", decisions[i].Reason))
		}
	}
}

// applyReview settles a flagged line on a verdict that holds up. A pick under the match floor leads
// the line but leaves it AMBIGUOUS, so the model's knowledge alone never marks a line decided.
func (s *CatalogMatchService) applyReview(
	match domain.LineMatch, decision domain.MatchReviewDecision,
) domain.LineMatch {
	chosen, ok := validChoices(decision.Chosen, len(match.Candidates))
	if !ok {
		return match
	}
	switch decision.Verdict {
	case domain.MatchReviewVerdictNone:
		if len(chosen) > 0 {
			return match
		}
		match.MatchStatus = domain.ItemMatchStatusNoMatch
		match.ProductID = nil
		return match
	case domain.MatchReviewVerdictOne:
		if len(chosen) != 1 || match.Candidates[chosen[0]].Confidence.LessThan(s.reviewFloor) {
			return match
		}
		// The model's knowledge picks the product; only the catalog text clears it as decided.
		if match.Candidates[chosen[0]].Confidence.LessThan(s.minConfidence) {
			return settleOn(match, chosen, domain.ItemMatchStatusAmbiguous)
		}
		return settleOn(match, chosen, domain.ItemMatchStatusMatched)
	case domain.MatchReviewVerdictSeveral:
		if len(chosen) == 0 || match.Candidates[chosen[0]].Confidence.LessThan(s.reviewFloor) {
			return match
		}
		return settleOn(match, chosen, domain.ItemMatchStatusAmbiguous)
	}
	return match
}

// settleOn moves the chosen candidates to the front in the reviewer's order and leads the line
// with the first of them, keeping the confidence the text gave it.
func settleOn(
	match domain.LineMatch, chosen []int, status domain.ItemMatchStatus,
) domain.LineMatch {
	reordered := make([]domain.ScoredCandidate, 0, len(match.Candidates))
	taken := make([]bool, len(match.Candidates))
	for _, index := range chosen {
		reordered = append(reordered, match.Candidates[index])
		taken[index] = true
	}
	for i, candidate := range match.Candidates {
		if !taken[i] {
			reordered = append(reordered, candidate)
		}
	}
	productID := reordered[0].ProductID
	match.Candidates = reordered
	match.ProductID = &productID
	match.MatchStatus = status
	match.Confidence = reordered[0].Confidence
	match.SettledByReview = true
	return match
}

// validChoices refuses a verdict that points outside the line's candidates or names one twice.
func validChoices(chosen []int, candidates int) ([]int, bool) {
	seen := make(map[int]bool, len(chosen))
	for _, index := range chosen {
		if index < 0 || index >= candidates || seen[index] {
			return nil, false
		}
		seen[index] = true
	}
	return chosen, true
}

// reviewLineOf is what the reviewer sees of one line: the client's words and every candidate the
// matcher kept for it.
func reviewLineOf(description string, match domain.LineMatch) domain.MatchReviewLine {
	candidates := make([]domain.MatchReviewCandidate, len(match.Candidates))
	for i, candidate := range match.Candidates {
		candidates[i] = domain.MatchReviewCandidate{
			Name: candidate.CanonicalName, Description: candidate.Description, Unit: candidate.Unit,
		}
	}
	return domain.MatchReviewLine{Description: description, Candidates: candidates}
}

// percentAsRatio turns a configured whole percentage into the 0..1 scale the scores live on.
func percentAsRatio(value int) decimal.Decimal {
	return decimal.NewFromInt(int64(value)).Div(decimal.NewFromInt(100))
}
