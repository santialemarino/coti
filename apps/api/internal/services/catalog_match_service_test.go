package services

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// Matching decides; the search offers. These tests stage candidates directly, so every
// confidence below is 1 - distance and can be redone by hand. The search behind them — the two
// halves, the branch filter, the widening — is covered by its own tests.

func testMatchConfig() config.CatalogConfig {
	cfg := testSearchConfig()
	cfg.MatchMinConfidencePercent = 60
	cfg.MatchAmbiguityMarginPercent = 5
	cfg.MatchLexicalConfidencePercent = 75
	return cfg
}

// fakeSearcher answers with the candidates a test staged per line, and records what it was asked.
type fakeSearcher struct {
	perLine [][]domain.CatalogCandidate
	asked   [][]string
	limits  []int
	err     error
}

func (f *fakeSearcher) Search(
	_ context.Context, _ domain.Tenant, in domain.CatalogSearch,
) ([]domain.CatalogSearchResult, error) {
	f.asked = append(f.asked, in.Texts)
	f.limits = append(f.limits, in.Limit)
	if f.err != nil {
		return nil, f.err
	}
	results := make([]domain.CatalogSearchResult, len(f.perLine))
	for i, candidates := range f.perLine {
		results[i] = domain.CatalogSearchResult{Candidates: candidates}
	}
	return results, nil
}

// semantic builds a candidate the vector half found at the given cosine distance.
func semantic(name string, distance float64) domain.CatalogCandidate {
	return domain.CatalogCandidate{ProductID: uuid.New(), CanonicalName: name, Distance: &distance}
}

// lexicalOnly builds a candidate only the full-text half reached — a synonym hit on a product
// carrying no embedding, which has no cosine similarity to read.
func lexicalOnly(name string, score float64) domain.CatalogCandidate {
	return domain.CatalogCandidate{ProductID: uuid.New(), CanonicalName: name, LexicalScore: &score}
}

// matchOne runs one line through the service and returns its decision.
func matchOne(t *testing.T, candidates []domain.CatalogCandidate) domain.LineMatch {
	t.Helper()
	return matchOneWith(t, testMatchConfig(), candidates)
}

func matchOneWith(
	t *testing.T, cfg config.CatalogConfig, candidates []domain.CatalogCandidate,
) domain.LineMatch {
	t.Helper()
	search := &fakeSearcher{perLine: [][]domain.CatalogCandidate{candidates}}
	service := NewCatalogMatchService(search, cfg)
	matches, err := service.Match(context.Background(), testSearchTenant(), []string{"cemento"})
	if err != nil {
		t.Fatalf("Match() = %v, want no error", err)
	}
	if len(matches) != 1 {
		t.Fatalf("matches = %d, want one per line", len(matches))
	}
	return matches[0]
}

// wantDecision asserts the whole decision at once, so a status that is right for the wrong
// confidence still fails.
func wantDecision(
	t *testing.T, got domain.LineMatch, status domain.ItemMatchStatus, confidence string,
	product *uuid.UUID,
) {
	t.Helper()
	if got.MatchStatus != status {
		t.Errorf("match status = %q, want %q", got.MatchStatus, status)
	}
	want := decimal.RequireFromString(confidence)
	if !got.Confidence.Equal(want) {
		t.Errorf("confidence = %s, want %s", got.Confidence, want)
	}
	switch {
	case product == nil && got.ProductID != nil:
		t.Errorf("product = %v, want none", *got.ProductID)
	case product != nil && got.ProductID == nil:
		t.Errorf("product = none, want %v", *product)
	case product != nil && *got.ProductID != *product:
		t.Errorf("product = %v, want %v", *got.ProductID, *product)
	}
}

// A clear leader well above the floor, with nothing near it.
func TestCatalogMatchService_MatchesAClearLeader(t *testing.T) {
	leader := semantic("Cemento Portland 50kg", 0.08)
	runnerUp := semantic("Cal hidratada 25kg", 0.45)

	got := matchOne(t, []domain.CatalogCandidate{leader, runnerUp})

	// 1 - 0.08 = 0.9200, and the margin over 0.5500 is 0.3700, well past the 0.05 asked for.
	wantDecision(t, got, domain.ItemMatchStatusMatched, "0.92", &leader.ProductID)
}

// Two cements a point apart are a choice between products, not a decided line. The leader is
// still carried, so the seller confirms one rather than searching the catalog from scratch.
func TestCatalogMatchService_FlagsTwoNearEqualCandidatesAmbiguous(t *testing.T) {
	leader := semantic("Cemento Portland 50kg", 0.09)
	runnerUp := semantic("Cemento Portland 25kg", 0.10)

	got := matchOne(t, []domain.CatalogCandidate{leader, runnerUp})

	// 0.9100 against 0.9000: above the floor, but the margin of 0.0100 is under 0.05.
	wantDecision(t, got, domain.ItemMatchStatusAmbiguous, "0.91", &leader.ProductID)
}

// The product invariant: a line nothing matched is flagged and kept, never dropped. The score
// it keeps is the best rejected candidate's, which is what separates a near miss from nothing.
func TestCatalogMatchService_FlagsALeaderBelowTheFloorNoMatch(t *testing.T) {
	got := matchOne(t, []domain.CatalogCandidate{semantic("Arena fina", 0.55)})

	wantDecision(t, got, domain.ItemMatchStatusNoMatch, "0.45", nil)
	if len(got.Candidates) != 1 {
		t.Errorf("candidates = %d, want the rejected one kept for the seller", len(got.Candidates))
	}
}

// The branch may simply carry nothing like the line. There is no candidate to score, so the
// confidence is zero rather than the floor's neighbourhood.
func TestCatalogMatchService_FlagsALineWithNoCandidatesNoMatch(t *testing.T) {
	got := matchOne(t, nil)

	wantDecision(t, got, domain.ItemMatchStatusNoMatch, "0", nil)
}

// The two thresholds are inclusive, and the boundary is where a calibration change lands.
func TestCatalogMatchService_DecidesAtTheThresholdBoundaries(t *testing.T) {
	cases := []struct {
		name       string
		candidates []domain.CatalogCandidate
		status     domain.ItemMatchStatus
		confidence string
	}{
		{
			// 1 - 0.40 = 0.6000, exactly the 60% floor.
			name:       "exactly at the confidence floor",
			candidates: []domain.CatalogCandidate{semantic("Cemento", 0.40)},
			status:     domain.ItemMatchStatusMatched,
			confidence: "0.6",
		},
		{
			// 1 - 0.4001 = 0.5999, one ten-thousandth short of it.
			name:       "one ten-thousandth below the floor",
			candidates: []domain.CatalogCandidate{semantic("Cemento", 0.4001)},
			status:     domain.ItemMatchStatusNoMatch,
			confidence: "0.5999",
		},
		{
			// 0.9000 - 0.8500 = 0.0500, exactly the 5% margin.
			name:       "exactly at the ambiguity margin",
			candidates: []domain.CatalogCandidate{semantic("A", 0.10), semantic("B", 0.15)},
			status:     domain.ItemMatchStatusMatched,
			confidence: "0.9",
		},
		{
			// 0.9000 - 0.8501 = 0.0499.
			name:       "one ten-thousandth below the margin",
			candidates: []domain.CatalogCandidate{semantic("A", 0.10), semantic("B", 0.1499)},
			status:     domain.ItemMatchStatusAmbiguous,
			confidence: "0.9",
		},
		{
			// Nothing behind it, so the margin rule has nothing to say.
			name:       "a single candidate at the floor",
			candidates: []domain.CatalogCandidate{semantic("Cemento", 0.40)},
			status:     domain.ItemMatchStatusMatched,
			confidence: "0.6",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := matchOne(t, tc.candidates)
			var product *uuid.UUID
			if tc.status != domain.ItemMatchStatusNoMatch {
				product = &tc.candidates[0].ProductID
			}
			wantDecision(t, got, tc.status, tc.confidence, product)
		})
	}
}

// A synonym hit on a product carrying no embedding has no cosine similarity to read. It is
// strong evidence, so it takes the configured confidence — the acceptance criterion that a
// trade term resolves to its product depends on that sitting above the floor.
func TestCatalogMatchService_ScoresALexicalOnlyLeaderFromConfiguration(t *testing.T) {
	leader := lexicalOnly("Membrana asfáltica 4mm", 0.06)

	got := matchOne(t, []domain.CatalogCandidate{leader})

	wantDecision(t, got, domain.ItemMatchStatusMatched, "0.75", &leader.ProductID)
}

// Two products sharing a trade term both come back on the lexical half alone, at the same
// configured worth — a margin of exactly zero, and a line the seller has to settle.
func TestCatalogMatchService_FlagsTwoLexicalOnlyCandidatesAmbiguous(t *testing.T) {
	leader := lexicalOnly("Membrana asfáltica 4mm", 0.06)
	runnerUp := lexicalOnly("Membrana asfáltica 3mm", 0.06)

	got := matchOne(t, []domain.CatalogCandidate{leader, runnerUp})

	wantDecision(t, got, domain.ItemMatchStatusAmbiguous, "0.75", &leader.ProductID)
}

// The runner-up can be the lexical-only one instead, and the margin is still read on one scale.
func TestCatalogMatchService_ComparesALexicalOnlyRunnerUpOnTheSameScale(t *testing.T) {
	leader := lexicalOnly("Membrana asfáltica 4mm", 0.06)
	runnerUp := semantic("Membrana líquida 20L", 0.28)

	got := matchOne(t, []domain.CatalogCandidate{leader, runnerUp})

	// 0.7500 against 0.7200: the margin of 0.0300 is under 0.05.
	wantDecision(t, got, domain.ItemMatchStatusAmbiguous, "0.75", &leader.ProductID)
}

// The fused rank can put a candidate both halves found ahead of a nearer vector only one half
// saw. The margin then goes negative, which is the halves disagreeing — an ambiguous line, and
// not something the formula needs a special case for.
func TestCatalogMatchService_FlagsADisagreementBetweenTheHalvesAmbiguous(t *testing.T) {
	leader := semantic("Cemento de albañilería", 0.30)
	runnerUp := semantic("Cemento Portland 50kg", 0.05)

	got := matchOne(t, []domain.CatalogCandidate{leader, runnerUp})

	// 0.7000 - 0.9500 = -0.2500.
	wantDecision(t, got, domain.ItemMatchStatusAmbiguous, "0.7", &leader.ProductID)
}

// Cosine distance runs to 2, so a candidate pointing the other way would score below zero and
// the column would refuse a negative that never meant anything.
func TestCatalogMatchService_ClampsAnOppositeVectorToZero(t *testing.T) {
	got := matchOne(t, []domain.CatalogCandidate{semantic("Nada que ver", 1.4)})

	wantDecision(t, got, domain.ItemMatchStatusNoMatch, "0", nil)
}

// A product stored with a zero-length vector comes back from the database at NaN, and the
// decimal package refuses to build one — so the line would take the whole request down with it.
func TestCatalogMatchService_ScoresAnUnusableDistanceAsNoEvidence(t *testing.T) {
	for _, tc := range []struct {
		name     string
		distance float64
	}{
		{"a zero-length vector", math.NaN()},
		{"an infinite distance", math.Inf(1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := matchOne(t, []domain.CatalogCandidate{semantic("Vector roto", tc.distance)})

			wantDecision(t, got, domain.ItemMatchStatusNoMatch, "0", nil)
		})
	}
}

// The runner-up carrying the unusable vector must not take the request down either, and it
// scores as no evidence rather than as a rival for the line.
func TestCatalogMatchService_ScoresAnUnusableRunnerUpAsNoEvidence(t *testing.T) {
	leader := semantic("Cemento Portland 50kg", 0.08)

	got := matchOne(t, []domain.CatalogCandidate{leader, semantic("Vector roto", math.NaN())})

	wantDecision(t, got, domain.ItemMatchStatusMatched, "0.92", &leader.ProductID)
}

// The threshold is calibration, not code: the same candidates decide differently once the
// floor moves, which is the acceptance criterion about changing it by configuration.
func TestCatalogMatchService_FollowsTheConfiguredThreshold(t *testing.T) {
	candidates := []domain.CatalogCandidate{semantic("Arena fina", 0.55)}

	got := matchOneWith(t, testMatchConfig(), candidates)
	if got.MatchStatus != domain.ItemMatchStatusNoMatch {
		t.Fatalf("match status at the default floor = %q, want NO_MATCH", got.MatchStatus)
	}

	lowered := testMatchConfig()
	lowered.MatchMinConfidencePercent = 40
	if got := matchOneWith(t, lowered, candidates); got.MatchStatus != domain.ItemMatchStatusMatched {
		t.Errorf("match status at a floor of 40%% = %q, want MATCHED", got.MatchStatus)
	}
}

// Every line is resolved in one search, because the search embeds the whole set in a single
// provider call and reads the catalog in one transaction. One call per line would undo both.
func TestCatalogMatchService_ResolvesEveryLineInOneSearch(t *testing.T) {
	first := semantic("Cemento Portland 50kg", 0.05)
	third := semantic("Arena fina", 0.02)
	search := &fakeSearcher{perLine: [][]domain.CatalogCandidate{
		{first},
		nil,
		{third},
	}}
	service := NewCatalogMatchService(search, testMatchConfig())

	texts := []string{"cemento", "algo que no vendemos", "arena"}
	matches, err := service.Match(context.Background(), testSearchTenant(), texts)
	if err != nil {
		t.Fatalf("Match() = %v, want no error", err)
	}
	if len(search.asked) != 1 {
		t.Fatalf("searches = %d, want one for the whole set", len(search.asked))
	}
	if len(search.asked[0]) != 3 {
		t.Errorf("searched texts = %v, want all three lines", search.asked[0])
	}
	// Zero takes the configured top K, so the candidate count stays one setting rather than two.
	if search.limits[0] != 0 {
		t.Errorf("search limit = %d, want the configured default", search.limits[0])
	}
	if len(matches) != 3 {
		t.Fatalf("matches = %d, want one per line", len(matches))
	}
	// Index alignment is the whole contract: line two is the unmatched one, and it stays in place.
	wantDecision(t, matches[0], domain.ItemMatchStatusMatched, "0.95", &first.ProductID)
	wantDecision(t, matches[1], domain.ItemMatchStatusNoMatch, "0", nil)
	wantDecision(t, matches[2], domain.ItemMatchStatusMatched, "0.98", &third.ProductID)
}

func TestCatalogMatchService_MatchesNothingWhenAskedForNothing(t *testing.T) {
	search := &fakeSearcher{}
	service := NewCatalogMatchService(search, testMatchConfig())

	matches, err := service.Match(context.Background(), testSearchTenant(), nil)
	if err != nil {
		t.Fatalf("Match() = %v, want no error", err)
	}
	if matches != nil {
		t.Errorf("matches = %v, want none", matches)
	}
	if len(search.asked) != 0 {
		t.Errorf("searches = %d, want none for an empty set", len(search.asked))
	}
}

// A search that cannot answer is not a line that did not match: an outage would otherwise be
// persisted as a catalog with nothing in it.
func TestCatalogMatchService_PassesUpASearchFailure(t *testing.T) {
	search := &fakeSearcher{err: domain.ErrAIUnavailable}
	service := NewCatalogMatchService(search, testMatchConfig())

	_, err := service.Match(context.Background(), testSearchTenant(), []string{"cemento"})
	if !errors.Is(err, domain.ErrAIUnavailable) {
		t.Errorf("Match() = %v, want %v", err, domain.ErrAIUnavailable)
	}
}

// Fewer results than lines would pair a line with the next line's candidates, which is a wrong
// match nothing downstream could notice.
func TestCatalogMatchService_RefusesAMisalignedSearchAnswer(t *testing.T) {
	search := &fakeSearcher{perLine: [][]domain.CatalogCandidate{{semantic("Cemento", 0.05)}}}
	service := NewCatalogMatchService(search, testMatchConfig())

	_, err := service.Match(context.Background(), testSearchTenant(), []string{"cemento", "cal"})
	if err == nil {
		t.Fatal("Match() = nil error, want the short answer refused")
	}
}

// The score is written to NUMERIC(5,4), so a value it cannot hold exactly would be rounded by
// the database into something that no longer explains the decision it was taken on.
func TestCatalogMatchService_ScoresFitTheConfidenceColumn(t *testing.T) {
	got := matchOne(t, []domain.CatalogCandidate{semantic("Cemento", 0.123456789)})

	if got.Confidence.Exponent() < -4 {
		t.Errorf("confidence = %s, want at most four decimals", got.Confidence)
	}
	if got.Confidence.GreaterThan(decimal.NewFromInt(1)) || got.Confidence.IsNegative() {
		t.Errorf("confidence = %s, want it on 0..1", got.Confidence)
	}
	// 1 - 0.123456789 = 0.876543211, rounded to 0.8765.
	if want := decimal.RequireFromString("0.8765"); !got.Confidence.Equal(want) {
		t.Errorf("confidence = %s, want %s", got.Confidence, want)
	}
}
