package services

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"math"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// Matching decides; the search offers. These tests stage candidates directly and read similarity
// as it comes (a band of 0..100) at a coverage weight of 50, so every confidence below is
// 0.5 × coverage + 0.5 × (1 − distance) and can be redone by hand. The search behind them — the
// two halves, the branch filter, the widening — is covered by its own tests.

func testMatchConfig() config.CatalogConfig {
	cfg := testSearchConfig()
	cfg.MatchMinConfidencePercent = 60
	cfg.MatchAmbiguityMarginPercent = 5
	cfg.MatchCoverageWeightPercent = 50
	cfg.MatchSimilarityFloorPercent = 0
	cfg.MatchSimilarityCeilingPercent = 100
	cfg.MatchHighConfidencePercent = 80
	cfg.MatchReviewFloorPercent = 40
	cfg.MatchReviewMaxLines = 30
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

// semantic builds a candidate with a vector at the given cosine distance from the line.
func semantic(name string, distance float64) domain.CatalogCandidate {
	return domain.CatalogCandidate{ProductID: uuid.New(), CanonicalName: name, Distance: &distance}
}

// unembedded builds a candidate whose product has no vector yet, so only its text can speak.
func unembedded(name string, synonyms ...string) domain.CatalogCandidate {
	score := 0.06
	return domain.CatalogCandidate{ProductID: uuid.New(), CanonicalName: name,
		LexicalScore: &score, Synonyms: synonyms}
}

func learned(name string, distance float64) domain.CatalogCandidate {
	return domain.CatalogCandidate{ProductID: uuid.New(), CanonicalName: name,
		LearnedDistance: &distance}
}

// matchOne runs one line through the service and returns its decision.
func matchOne(t *testing.T, line string, candidates []domain.CatalogCandidate) domain.LineMatch {
	t.Helper()
	return matchOneWith(t, testMatchConfig(), line, candidates)
}

func matchOneWith(
	t *testing.T, cfg config.CatalogConfig, line string, candidates []domain.CatalogCandidate,
) domain.LineMatch {
	t.Helper()
	search := &fakeSearcher{perLine: [][]domain.CatalogCandidate{candidates}}
	service := NewCatalogMatchService(search, cfg)
	matches, err := service.Match(context.Background(), testSearchTenant(), []string{line})
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

	got := matchOne(t, "cemento portland", []domain.CatalogCandidate{leader, runnerUp})

	// The name covers the line: 0.5 × 1 + 0.5 × 0.92 = 0.96. The lime covers none of it:
	// 0.5 × 0.55 = 0.275, far behind.
	wantDecision(t, got, domain.ItemMatchStatusMatched, "0.96", &leader.ProductID)
}

// Two cements a point apart are a choice between products, not a decided line. The leader is
// still carried, so the seller confirms one rather than searching the catalog from scratch.
func TestCatalogMatchService_FlagsTwoNearEqualCandidatesAmbiguous(t *testing.T) {
	leader := semantic("Cemento Portland 50kg", 0.09)
	runnerUp := semantic("Cemento Portland 25kg", 0.10)

	got := matchOne(t, "cemento portland", []domain.CatalogCandidate{leader, runnerUp})

	// 0.955 against 0.95: above the floor, but the margin of 0.005 is under 0.05.
	wantDecision(t, got, domain.ItemMatchStatusAmbiguous, "0.955", &leader.ProductID)
}

// Both names answer the line in full, so the line never said which one it wants. A vector a few
// points closer is not the client choosing, even past the plain margin.
func TestCatalogMatchService_FlagsARivalThatAnswersTheLineAsFullyAmbiguous(t *testing.T) {
	leader := semantic("Cemento Portland 50kg", 0.02)
	runnerUp := semantic("Cemento Portland 25kg", 0.14)

	got := matchOne(t, "cemento portland", []domain.CatalogCandidate{leader, runnerUp})

	// 0.99 against 0.93: a margin of 0.06 clears 0.05 but not the three margins a coverage tie
	// needs to be settled by the vectors.
	wantDecision(t, got, domain.ItemMatchStatusAmbiguous, "0.99", &leader.ProductID)
}

// Past three margins the vectors disagree clearly enough to decide between two full answers.
func TestCatalogMatchService_SettlesACoverageTieOnAClearlyCloserVector(t *testing.T) {
	leader := semantic("Cemento Portland 50kg", 0.02)
	runnerUp := semantic("Cemento Portland 25kg", 0.40)

	got := matchOne(t, "cemento portland", []domain.CatalogCandidate{leader, runnerUp})

	// 0.99 against 0.80: 0.19 is past 0.15.
	wantDecision(t, got, domain.ItemMatchStatusMatched, "0.99", &leader.ProductID)
}

// An embedding blurs the spec that tells two products of a family apart; the text does not. The
// product that carries the size asked for wins even from a farther vector.
func TestCatalogMatchService_DecidesOnTheSpecTheVectorsBlur(t *testing.T) {
	asked := semantic("Vigueta pret 4.50", 0.20)
	closer := semantic("Vigueta pret 4.40", 0.18)

	got := matchOne(t, "vigueta 4.50", []domain.CatalogCandidate{closer, asked})

	// The 4.50 covers the line: 0.5 × 1 + 0.5 × 0.80 = 0.90. The 4.40 misses the figure, which
	// weighs 1.5 against the word's 1 (0.4 covered), and its own 4.40 is a spec nobody asked for:
	// 0.5 × 0.4 + 0.5 × 0.82 − 0.05 = 0.56.
	wantDecision(t, got, domain.ItemMatchStatusMatched, "0.9", &asked.ProductID)
	if got.Candidates[1].ProductID != closer.ProductID ||
		!got.Candidates[1].Confidence.Equal(decimal.RequireFromString("0.56")) {
		t.Errorf("runner-up = %s at %s, want the 4.40 at 0.56",
			got.Candidates[1].CanonicalName, got.Candidates[1].Confidence)
	}
}

// A figure and the unit written after it are one quantity: 4 mm is not 4 litres.
func TestCatalogMatchService_RefusesAFigureInAnotherUnit(t *testing.T) {
	paint := semantic("Pintura asfáltica 4L", 0.10)

	got := matchOne(t, "membrana 4mm", []domain.CatalogCandidate{paint})

	// Neither the word nor the figure is covered, and the 4L is unasked: 0.5 × 0.90 − 0.05.
	wantDecision(t, got, domain.ItemMatchStatusNoMatch, "0.4", nil)
}

// A rival that answers the line as fully but also carries a spec the line never asked for is
// another part, not a second answer: "PVC cupla red 110x100" is a reducer, the line wants 110.
func TestCatalogMatchService_DoesNotLetAnUnaskedSpecContestTheLead(t *testing.T) {
	leader := semantic("PVC Cupla 110", 0.30)
	reducer := semantic("PVC Cupla red 110x100", 0.40)

	got := matchOne(t, "cupla pvc 110", []domain.CatalogCandidate{reducer, leader})

	// 0.85 against 0.8 − 0.05 × ½ = 0.775: within reach and covering the line, but the 100 is
	// unasked while the leader carries nothing extra.
	wantDecision(t, got, domain.ItemMatchStatusMatched, "0.85", &leader.ProductID)

	// The same rival without the extra spec is a second answer, and the line is a choice.
	plain := semantic("PVC Cupla 110 gris", 0.42)
	got = matchOne(t, "cupla pvc 110", []domain.CatalogCandidate{plain, leader})
	wantDecision(t, got, domain.ItemMatchStatusAmbiguous, "0.85", &leader.ProductID)
}

// The figure opening a line is how many the client wants. Read as a spec it would be a size no
// product carries, costing the right product most of its coverage; the packaging after it is only
// weak evidence either way.
func TestCatalogMatchService_DropsTheCountOpeningTheLine(t *testing.T) {
	cement := semantic("Cemento Portland 50kg", 0.10)

	got := matchOne(t, "10 bolsas de cemento", []domain.CatalogCandidate{cement})

	// "10" is the count and goes. "bolsas" stays at its packaging weight of 0.3, which the name
	// does not carry, and "cemento" is covered: coverage 1 / 1.3, so 0.5 × 0.7692 + 0.5 × 0.90,
	// with no figure left on the line to make the 50kg unasked.
	wantDecision(t, got, domain.ItemMatchStatusMatched, "0.8346", &cement.ProductID)
}

// A line typed by ear still meets its product, at the credit a near spelling earns.
func TestCatalogMatchService_ForgivesASpellingTypedByEar(t *testing.T) {
	brick := semantic("Ladrillo comun", 0.50)

	got := matchOne(t, "ladriyo comun", []domain.CatalogCandidate{brick})

	// "ladriyo" meets "ladrillo" on the phonetic key at 0.9, "comun" exactly: coverage 0.95, so
	// 0.5 × 0.95 + 0.5 × 0.50.
	wantDecision(t, got, domain.ItemMatchStatusMatched, "0.725", &brick.ProductID)
}

// The acceptance criterion: a trade term loaded as a synonym resolves to its product. The
// product has no vector yet, which is no evidence either way: its missing half reads as middling
// agreement with its text, 0.5 × 1 + 0.5 × ½.
func TestCatalogMatchService_ResolvesATradeTermThroughASynonym(t *testing.T) {
	membrane := unembedded("Membrana asfáltica 4mm", "telagoma")

	got := matchOne(t, "telagoma", []domain.CatalogCandidate{membrane})

	wantDecision(t, got, domain.ItemMatchStatusMatched, "0.75", &membrane.ProductID)

	// Without the synonym nothing in the product's text is what the line asked for.
	got = matchOne(t, "telagoma", []domain.CatalogCandidate{unembedded("Membrana asfáltica 4mm")})
	wantDecision(t, got, domain.ItemMatchStatusNoMatch, "0", nil)
}

// The margin is read against the closest competitor, not whichever product the search happened
// to put second: here an unrelated product sits between two limes.
func TestCatalogMatchService_ReadsTheMarginAgainstTheClosestRival(t *testing.T) {
	lime := semantic("Cal comun cacique", 0.64)
	unrelated := semantic("Angulo de ajuste", 0.63)
	otherLime := semantic("Holcim cal hidratada", 0.70)

	got := matchOne(t, "cal", []domain.CatalogCandidate{lime, unrelated, otherLime})

	// 0.68 and 0.65 for the limes, 0.185 for the angle: the margin that counts is 0.03.
	wantDecision(t, got, domain.ItemMatchStatusAmbiguous, "0.68", &lime.ProductID)
	if got.Candidates[1].ProductID != otherLime.ProductID {
		t.Errorf("runner-up = %s, want the other lime", got.Candidates[1].CanonicalName)
	}
}

// A line keeps the best topK by confidence, whatever order the search offered them in.
func TestCatalogMatchService_KeepsTheBestTopKByConfidence(t *testing.T) {
	best := semantic("Arena fina", 0.05)
	candidates := []domain.CatalogCandidate{
		semantic("Cemento", 0.60), semantic("Cal", 0.62), semantic("Hierro", 0.64),
		semantic("Ladrillo", 0.66), best,
	}

	got := matchOne(t, "arena", candidates)

	if len(got.Candidates) != testMatchConfig().SearchTopK {
		t.Fatalf("candidates = %d, want the configured top %d", len(got.Candidates),
			testMatchConfig().SearchTopK)
	}
	if got.Candidates[0].ProductID != best.ProductID {
		t.Errorf("leader = %s, want the sand the search offered last",
			got.Candidates[0].CanonicalName)
	}
	if got.Candidates[2].CanonicalName != "Cal" {
		t.Errorf("third = %s, want the lime ahead of the iron and the brick",
			got.Candidates[2].CanonicalName)
	}
}

// The product invariant: a line nothing matched is flagged and kept, never dropped. The score
// it keeps is the best rejected candidate's, which is what separates a near miss from nothing.
func TestCatalogMatchService_FlagsALeaderBelowTheFloorNoMatch(t *testing.T) {
	got := matchOne(t, "polvo de ladrillo", []domain.CatalogCandidate{semantic("Arena fina", 0.10)})

	// No word covered: 0.5 × 0.90.
	wantDecision(t, got, domain.ItemMatchStatusNoMatch, "0.45", nil)
	if len(got.Candidates) != 1 {
		t.Errorf("candidates = %d, want the rejected one kept for the seller", len(got.Candidates))
	}
}

// The branch may simply carry nothing like the line. There is no candidate to score, so the
// confidence is zero rather than the floor's neighbourhood.
func TestCatalogMatchService_FlagsALineWithNoCandidatesNoMatch(t *testing.T) {
	got := matchOne(t, "cemento", nil)

	wantDecision(t, got, domain.ItemMatchStatusNoMatch, "0", nil)
}

// The two thresholds are inclusive, and the boundary is where a calibration change lands.
func TestCatalogMatchService_DecidesAtTheThresholdBoundaries(t *testing.T) {
	cases := []struct {
		name       string
		line       string
		candidates []domain.CatalogCandidate
		status     domain.ItemMatchStatus
		confidence string
	}{
		{
			// 0.5 × 1 + 0.5 × 0.20 = 0.6000, exactly the 60% floor.
			name:       "exactly at the confidence floor",
			line:       "cemento",
			candidates: []domain.CatalogCandidate{semantic("Cemento", 0.80)},
			status:     domain.ItemMatchStatusMatched,
			confidence: "0.6",
		},
		{
			// 0.5 + 0.5 × 0.1998 = 0.5999, one ten-thousandth short of it.
			name:       "one ten-thousandth below the floor",
			line:       "cemento",
			candidates: []domain.CatalogCandidate{semantic("Cemento", 0.8002)},
			status:     domain.ItemMatchStatusNoMatch,
			confidence: "0.5999",
		},
		{
			// 0.80 against a rival covering half the line at 0.25 + 0.5 = 0.75: exactly 0.05.
			name: "exactly at the ambiguity margin",
			line: "cemento portland",
			candidates: []domain.CatalogCandidate{
				semantic("Cemento Portland", 0.40), semantic("Cemento", 0),
			},
			status:     domain.ItemMatchStatusMatched,
			confidence: "0.8",
		},
		{
			// 0.7999 against 0.75: 0.0499.
			name: "one ten-thousandth below the margin",
			line: "cemento portland",
			candidates: []domain.CatalogCandidate{
				semantic("Cemento Portland", 0.4002), semantic("Cemento", 0),
			},
			status:     domain.ItemMatchStatusAmbiguous,
			confidence: "0.7999",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := matchOne(t, tc.line, tc.candidates)
			var product *uuid.UUID
			if tc.status != domain.ItemMatchStatusNoMatch {
				product = &tc.candidates[0].ProductID
			}
			wantDecision(t, got, tc.status, tc.confidence, product)
		})
	}
}

func TestCatalogMatchService_LocalMemoryOverridesGenericSearch(t *testing.T) {
	local := learned("Seller choice", 0.19)
	generic := semantic("Generic nearest", 0.01)

	got := matchOne(t, "cemento", []domain.CatalogCandidate{generic, local})

	// A seller-taught phrase leads whatever the text suggests, at 1 − its distance.
	wantDecision(t, got, domain.ItemMatchStatusMatched, "0.81", &local.ProductID)
}

func TestCatalogMatchService_ConflictingLocalMemoriesAreAmbiguous(t *testing.T) {
	first := learned("First seller choice", 0.05)
	second := learned("Second seller choice", 0.18)

	got := matchOne(t, "cemento", []domain.CatalogCandidate{first, second})

	wantDecision(t, got, domain.ItemMatchStatusAmbiguous, "0.95", &first.ProductID)
}

// Cosine distance runs to 2, so a candidate pointing the other way would score below zero and
// the column would refuse a negative that never meant anything.
func TestCatalogMatchService_ClampsAnOppositeVectorToZero(t *testing.T) {
	got := matchOne(t, "cemento", []domain.CatalogCandidate{semantic("Nada que ver", 1.4)})

	wantDecision(t, got, domain.ItemMatchStatusNoMatch, "0", nil)
}

// A product stored with a zero-length vector comes back from the database at NaN, and the
// decimal package refuses to build one — so the line would take the whole request down with it.
// It reads as a product with no vector: its text, and half of it for the missing vector.
func TestCatalogMatchService_ScoresAnUnusableDistanceAsNoVector(t *testing.T) {
	for _, tc := range []struct {
		name     string
		distance float64
	}{
		{"a zero-length vector", math.NaN()},
		{"an infinite distance", math.Inf(1)},
		{"a negative infinite distance", math.Inf(-1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			broken := semantic("Vector roto", tc.distance)
			got := matchOne(t, "cemento", []domain.CatalogCandidate{broken})
			wantDecision(t, got, domain.ItemMatchStatusNoMatch, "0", nil)

			cement := semantic("Cemento", tc.distance)
			got = matchOne(t, "cemento", []domain.CatalogCandidate{cement})
			wantDecision(t, got, domain.ItemMatchStatusMatched, "0.75", &cement.ProductID)
		})
	}
}

// The threshold is calibration, not code: the same candidates decide differently once the
// floor moves, which is the acceptance criterion about changing it by configuration.
func TestCatalogMatchService_FollowsTheConfiguredThreshold(t *testing.T) {
	candidates := []domain.CatalogCandidate{semantic("Arena fina", 0.10)}

	strict := matchOneWith(t, testMatchConfig(), "polvo de ladrillo", candidates)
	wantDecision(t, strict, domain.ItemMatchStatusNoMatch, "0.45", nil)

	lowered := testMatchConfig()
	lowered.MatchMinConfidencePercent = 40
	relaxed := matchOneWith(t, lowered, "polvo de ladrillo", candidates)
	wantDecision(t, relaxed, domain.ItemMatchStatusMatched, "0.45", &candidates[0].ProductID)
}

// The calibration band is configuration too: the same distance reads higher once the band
// admits that the model rarely brings a correct pair past a cosine of 0.90.
func TestCatalogMatchService_FollowsTheConfiguredCalibration(t *testing.T) {
	candidates := []domain.CatalogCandidate{semantic("Arena fina", 0.325)}

	calibrated := testMatchConfig()
	calibrated.MatchSimilarityFloorPercent = 25
	calibrated.MatchSimilarityCeilingPercent = 90
	got := matchOneWith(t, calibrated, "polvo de ladrillo", candidates)

	// A cosine of 0.675 sits at (0.675 − 0.25) / 0.65 = 0.6538 of the band, halved.
	wantDecision(t, got, domain.ItemMatchStatusNoMatch, "0.3269", nil)
}

// Every line is resolved in one search, because the search embeds the whole set in a single
// provider call and reads the catalog in one transaction. One call per line would undo both.
func TestCatalogMatchService_ResolvesEveryLineInOneSearch(t *testing.T) {
	first := semantic("Cemento Portland 50kg", 0.05)
	third := semantic("Arena fina", 0.02)
	search := &fakeSearcher{perLine: [][]domain.CatalogCandidate{{first}, nil, {third}}}
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
	// The pool it scores is wider than what a line keeps: the search's order is not the decision's.
	cfg := testMatchConfig()
	if want := cfg.SearchTopK * cfg.SearchOverFetchFactor; search.limits[0] != want {
		t.Errorf("search limit = %d, want the pool of %d", search.limits[0], want)
	}
	if len(matches) != 3 {
		t.Fatalf("matches = %d, want one per line", len(matches))
	}
	// Index alignment is the whole contract: line two is the unmatched one, and it stays in place.
	wantDecision(t, matches[0], domain.ItemMatchStatusMatched, "0.975", &first.ProductID)
	wantDecision(t, matches[1], domain.ItemMatchStatusNoMatch, "0", nil)
	wantDecision(t, matches[2], domain.ItemMatchStatusMatched, "0.99", &third.ProductID)
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
	// Not an outage: the search answered, the answer was unusable. A fault of ours surfacing as a
	// provider outage would invite a retry that cannot succeed.
	if errors.Is(err, domain.ErrAIUnavailable) {
		t.Errorf("Match() = %v, want a fault of ours rather than a provider outage", err)
	}
}

// The score is written to NUMERIC(5,4), so a value it cannot hold exactly would be rounded by
// the database into something that no longer explains the decision it was taken on.
func TestCatalogMatchService_ScoresFitTheConfidenceColumn(t *testing.T) {
	got := matchOne(t, "cemento", []domain.CatalogCandidate{semantic("Cemento", 0.123456789)})

	if got.Confidence.Exponent() < -4 {
		t.Errorf("confidence = %s, want at most four decimals", got.Confidence)
	}
	// 0.5 + 0.5 × 0.876543211 = 0.9382716055, rounded to 0.9383.
	if want := decimal.RequireFromString("0.9383"); !got.Confidence.Equal(want) {
		t.Errorf("confidence = %s, want %s", got.Confidence, want)
	}
}

// fakeReviewer answers each review with staged decisions, and records what it was shown.
type fakeReviewer struct {
	decide reviewAnswer
	err    error
	asked  [][]domain.MatchReviewLine
}

func (f *fakeReviewer) Review(
	_ context.Context, lines []domain.MatchReviewLine,
) ([]domain.MatchReviewDecision, error) {
	f.asked = append(f.asked, lines)
	if f.err != nil {
		return nil, f.err
	}
	return f.decide(lines), nil
}

// reviewedOrder stages one order of four lines: one decided, two flagged, and one whose best
// candidate is not worth the model's time.
type reviewedOrder struct {
	lines                   []string
	candidates              [][]domain.CatalogCandidate
	standard, thin, garbage domain.CatalogCandidate
	fifty, twentyFive       domain.CatalogCandidate
}

func newReviewedOrder() reviewedOrder {
	o := reviewedOrder{
		// 0.5 × ½ + 0.5 × 0.60 = 0.55 and 0.5 × ½ + 0.5 × 0.55 = 0.525: under the floor, over the
		// review's; the brick covers none of the line and scores 0.5 × 0.30 = 0.15.
		standard: semantic("Durlock placa 12.5", 0.40),
		thin:     semantic("Durlock placa 9", 0.45),
		garbage:  semantic("Ladrillo", 0.70),
		// 0.975 and 0.97: both cover "cemento", a margin of 0.005.
		fifty:      semantic("Cemento Portland 50kg", 0.05),
		twentyFive: semantic("Cemento Portland 25kg", 0.06),
	}
	o.lines = []string{"cemento portland 50kg", "placas de yeso", "cemento", "zzz"}
	o.candidates = [][]domain.CatalogCandidate{
		{semantic("Cemento Portland 50kg", 0.02)},
		{o.standard, o.thin, o.garbage},
		{o.fifty, o.twentyFive},
		{semantic("Nada", 0.90)},
	}
	return o
}

func (o reviewedOrder) match(
	t *testing.T, cfg config.CatalogConfig, reviewer *fakeReviewer,
) []domain.LineMatch {
	t.Helper()
	service := NewCatalogMatchService(&fakeSearcher{perLine: o.candidates}, cfg).
		WithReviewer(reviewer, slog.New(slog.NewTextHandler(io.Discard, nil)))
	matches, err := service.Match(context.Background(), testSearchTenant(), o.lines)
	if err != nil {
		t.Fatalf("Match() = %v, want a review never to fail the order", err)
	}
	return matches
}

// reviewAnswer is what a staged reviewer answers a batch with.
type reviewAnswer func([]domain.MatchReviewLine) []domain.MatchReviewDecision

// decideAll answers every reviewed line the same way.
func decideAll(decision domain.MatchReviewDecision) reviewAnswer {
	return func(lines []domain.MatchReviewLine) []domain.MatchReviewDecision {
		out := make([]domain.MatchReviewDecision, len(lines))
		for i := range out {
			out[i] = decision
		}
		return out
	}
}

// decideOne answers a batch with a single decision, however many lines it carried.
func decideOne(decision domain.MatchReviewDecision) reviewAnswer {
	return func([]domain.MatchReviewLine) []domain.MatchReviewDecision {
		return []domain.MatchReviewDecision{decision}
	}
}

// byLine answers the plasterboard line and the cement line with one decision each.
func byLine(plaster, cement domain.MatchReviewDecision) reviewAnswer {
	return func(lines []domain.MatchReviewLine) []domain.MatchReviewDecision {
		out := make([]domain.MatchReviewDecision, len(lines))
		for i, line := range lines {
			switch line.Description {
			case "placas de yeso":
				out[i] = plaster
			case "cemento":
				out[i] = cement
			}
		}
		return out
	}
}

// Only what the text could not settle goes to review, in one call: a decided line is not the
// model's to reopen, and a line whose best offer is noise is not worth its time.
func TestCatalogMatchService_ReviewsOnlyTheLinesTheTextCouldNotSettle(t *testing.T) {
	o := newReviewedOrder()
	reviewer := &fakeReviewer{decide: decideAll(domain.MatchReviewDecision{})}

	o.match(t, testMatchConfig(), reviewer)

	if len(reviewer.asked) != 1 {
		t.Fatalf("review calls = %d, want one for the order", len(reviewer.asked))
	}
	shown := reviewer.asked[0]
	if len(shown) != 2 || shown[0].Description != "placas de yeso" ||
		shown[1].Description != "cemento" {
		t.Fatalf("reviewed lines = %+v, want the plasterboard and the cement lines", shown)
	}
	// The candidates travel in the matcher's order, which is what the verdict's indexes point at.
	if got := shown[0].Candidates; len(got) != 3 || got[0].Name != "Durlock placa 12.5" ||
		got[2].Name != "Ladrillo" {
		t.Errorf("plasterboard candidates = %+v, want the matcher's three in its order", got)
	}
}

// ONE on a candidate the text backs past the floor settles the line on it.
func TestCatalogMatchService_ReviewSettlesALineOnTheProductItNames(t *testing.T) {
	o := newReviewedOrder()
	reviewer := &fakeReviewer{decide: byLine(domain.MatchReviewDecision{},
		domain.MatchReviewDecision{Verdict: domain.MatchReviewVerdictOne, Chosen: []int{1}})}

	matches := o.match(t, testMatchConfig(), reviewer)

	wantDecision(t, matches[2], domain.ItemMatchStatusMatched, "0.97", &o.twentyFive.ProductID)
	if matches[2].Candidates[1].ProductID != o.fifty.ProductID {
		t.Errorf("runner-up = %s, want the other cement behind the chosen one",
			matches[2].Candidates[1].CanonicalName)
	}
	// The choice between the cements was the model's, so the other one stays on offer.
	if !matches[2].SettledByReview {
		t.Error("settled by review = false, want the line marked as the model's decision")
	}
	if matches[0].SettledByReview {
		t.Error("the line the text decided is marked as settled by review")
	}
}

// Knowledge alone never marks a line decided: ONE on a candidate under the match floor leads the
// line, and the line stays for the seller to confirm.
func TestCatalogMatchService_ReviewLeadsButDoesNotDecideUnderTheFloor(t *testing.T) {
	o := newReviewedOrder()
	reviewer := &fakeReviewer{decide: byLine(
		domain.MatchReviewDecision{Verdict: domain.MatchReviewVerdictOne, Chosen: []int{1}},
		domain.MatchReviewDecision{})}

	matches := o.match(t, testMatchConfig(), reviewer)

	wantDecision(t, matches[1], domain.ItemMatchStatusAmbiguous, "0.525", &o.thin.ProductID)
}

// A pick the catalog text barely backs is not taken at all.
func TestCatalogMatchService_ReviewIgnoresAPickUnderItsFloor(t *testing.T) {
	o := newReviewedOrder()
	reviewer := &fakeReviewer{decide: byLine(
		domain.MatchReviewDecision{Verdict: domain.MatchReviewVerdictOne, Chosen: []int{2}},
		domain.MatchReviewDecision{})}

	matches := o.match(t, testMatchConfig(), reviewer)

	wantDecision(t, matches[1], domain.ItemMatchStatusNoMatch, "0.55", nil)
}

// SEVERAL puts the fitting candidates first, in the model's order, and leaves the choice open.
func TestCatalogMatchService_ReviewOrdersTheCandidatesThatFit(t *testing.T) {
	o := newReviewedOrder()
	reviewer := &fakeReviewer{decide: byLine(
		domain.MatchReviewDecision{Verdict: domain.MatchReviewVerdictSeveral, Chosen: []int{1, 0}},
		domain.MatchReviewDecision{})}

	matches := o.match(t, testMatchConfig(), reviewer)

	wantDecision(t, matches[1], domain.ItemMatchStatusAmbiguous, "0.525", &o.thin.ProductID)
	if matches[1].Candidates[1].ProductID != o.standard.ProductID ||
		matches[1].Candidates[2].ProductID != o.garbage.ProductID {
		t.Errorf("candidates = %v, want the 9mm, the 12.5mm, then the rest",
			[]string{matches[1].Candidates[1].CanonicalName,
				matches[1].Candidates[2].CanonicalName})
	}
}

// NONE only ever makes a line more cautious: the product goes, the candidates and the score stay.
func TestCatalogMatchService_ReviewCanClearALine(t *testing.T) {
	o := newReviewedOrder()
	reviewer := &fakeReviewer{decide: byLine(domain.MatchReviewDecision{},
		domain.MatchReviewDecision{Verdict: domain.MatchReviewVerdictNone})}

	matches := o.match(t, testMatchConfig(), reviewer)

	wantDecision(t, matches[2], domain.ItemMatchStatusNoMatch, "0.975", nil)
	if len(matches[2].Candidates) != 2 {
		t.Errorf("candidates = %d, want both cements kept for the seller",
			len(matches[2].Candidates))
	}
}

// A verdict that does not hold together leaves the line exactly as the text had it.
func TestCatalogMatchService_ReviewIgnoresAVerdictThatDoesNotHoldUp(t *testing.T) {
	for _, tc := range []struct {
		name     string
		decision domain.MatchReviewDecision
	}{
		{"no verdict", domain.MatchReviewDecision{}},
		{"NONE naming a product", domain.MatchReviewDecision{
			Verdict: domain.MatchReviewVerdictNone, Chosen: []int{0}}},
		{"ONE naming two", domain.MatchReviewDecision{
			Verdict: domain.MatchReviewVerdictOne, Chosen: []int{0, 1}}},
		{"a candidate past the end", domain.MatchReviewDecision{
			Verdict: domain.MatchReviewVerdictOne, Chosen: []int{2}}},
		{"a candidate twice", domain.MatchReviewDecision{
			Verdict: domain.MatchReviewVerdictSeveral, Chosen: []int{1, 1}}},
		{"SEVERAL naming none", domain.MatchReviewDecision{
			Verdict: domain.MatchReviewVerdictSeveral}},
		{"a verdict it does not know", domain.MatchReviewDecision{
			Verdict: "MAYBE", Chosen: []int{1}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := newReviewedOrder()
			reviewer := &fakeReviewer{decide: byLine(domain.MatchReviewDecision{}, tc.decision)}

			matches := o.match(t, testMatchConfig(), reviewer)

			wantDecision(t, matches[2], domain.ItemMatchStatusAmbiguous, "0.975",
				&o.fifty.ProductID)
		})
	}
}

// A review that fails, or answers for a different number of lines, changes nothing and fails
// nothing: the text's own decisions stand.
func TestCatalogMatchService_KeepsTheTextsDecisionsWhenTheReviewFails(t *testing.T) {
	for _, tc := range []struct {
		name     string
		reviewer *fakeReviewer
	}{
		{"an unavailable model", &fakeReviewer{err: domain.ErrAIUnavailable}},
		{"a short answer", &fakeReviewer{decide: decideOne(domain.MatchReviewDecision{
			Verdict: domain.MatchReviewVerdictNone})}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := newReviewedOrder()

			matches := o.match(t, testMatchConfig(), tc.reviewer)

			wantDecision(t, matches[1], domain.ItemMatchStatusNoMatch, "0.55", nil)
			wantDecision(t, matches[2], domain.ItemMatchStatusAmbiguous, "0.975",
				&o.fifty.ProductID)
		})
	}
}

// The cap bounds what one order may cost, and zero turns the review off.
func TestCatalogMatchService_ReviewsNoMoreLinesThanTheCap(t *testing.T) {
	o := newReviewedOrder()
	capped := testMatchConfig()
	capped.MatchReviewMaxLines = 1
	reviewer := &fakeReviewer{decide: decideAll(domain.MatchReviewDecision{})}

	o.match(t, capped, reviewer)

	if len(reviewer.asked) != 1 || len(reviewer.asked[0]) != 1 {
		t.Fatalf("reviewed = %v, want one line", reviewer.asked)
	}

	off := testMatchConfig()
	off.MatchReviewMaxLines = 0
	silent := &fakeReviewer{decide: decideAll(domain.MatchReviewDecision{})}
	o.match(t, off, silent)
	if len(silent.asked) != 0 {
		t.Errorf("review calls = %d, want none with the review off", len(silent.asked))
	}
}

// Two seller-taught answers for one phrase are the seller's to settle, not the model's.
func TestCatalogMatchService_DoesNotReviewAConflictBetweenSellerAnswers(t *testing.T) {
	search := &fakeSearcher{perLine: [][]domain.CatalogCandidate{
		{learned("First seller choice", 0.05), learned("Second seller choice", 0.18)},
	}}
	reviewer := &fakeReviewer{decide: decideAll(domain.MatchReviewDecision{})}
	service := NewCatalogMatchService(search, testMatchConfig()).
		WithReviewer(reviewer, slog.New(slog.NewTextHandler(io.Discard, nil)))

	_, err := service.Match(context.Background(), testSearchTenant(), []string{"cemento"})
	if err != nil {
		t.Fatalf("Match() = %v", err)
	}
	if len(reviewer.asked) != 0 {
		t.Errorf("review calls = %d, want none for a seller conflict", len(reviewer.asked))
	}
}
