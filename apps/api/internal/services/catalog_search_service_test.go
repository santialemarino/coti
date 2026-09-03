package services

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/pgvector/pgvector-go"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

func TestFuse_PrioritizesSellerLearnedCandidates(t *testing.T) {
	t.Parallel()
	genericDistance, learnedDistance := 0.01, 0.19
	candidates := []domain.CatalogCandidate{
		{ProductID: uuid.New(), CanonicalName: "Generic", Distance: &genericDistance},
		{ProductID: uuid.New(), CanonicalName: "Learned", LearnedDistance: &learnedDistance},
	}
	fuse(candidates, 60)
	if candidates[0].CanonicalName != "Learned" {
		t.Fatalf("leader = %q, want seller-learned candidate", candidates[0].CanonicalName)
	}
}

// The search's decisions live in the service — how the two halves are ranked together, how wide
// the fetch grows, and where the trim happens. The SQL behind them is covered by the integration
// tests in internal/repository.

func testSearchTenant() domain.Tenant {
	return domain.Tenant{AccountID: testAccountID, BranchID: testBranchID, Role: domain.UserRoleAdmin}
}

func testSearchConfig() config.CatalogConfig {
	return config.CatalogConfig{
		DefaultPageSize:       50,
		MaxPageSize:           200,
		SearchTopK:            3,
		SearchOverFetchFactor: 2,
		SearchMaxFetch:        200,
		SearchProbes:          7,
		SearchRRFK:            60,
		EmbeddingBatchSize:    100,
	}
}

// fakeEmbedder answers with a vector per text and records what it was asked to embed.
type fakeEmbedder struct {
	calls [][]string
	err   error
	// short makes the answer one vector shorter than the request, the contract break the
	// service has to catch rather than pass on.
	short bool
}

func (f *fakeEmbedder) Embed(_ context.Context, texts []string) ([]pgvector.Vector, error) {
	f.calls = append(f.calls, texts)
	if f.err != nil {
		return nil, f.err
	}
	count := len(texts)
	if f.short {
		count--
	}
	vectors := make([]pgvector.Vector, count)
	for i := range vectors {
		values := make([]float32, domain.EmbeddingDimension)
		values[0] = float32(i + 1)
		vectors[i] = pgvector.NewVector(values)
	}
	return vectors, nil
}

// fakeSearch answers with whatever the test staged for a given fetch width, and records the
// widths it was asked for so the escalation can be asserted.
type fakeSearch struct {
	// byFetch maps a fetch width to how many candidates come back at it. The largest staged
	// width at or below the ask is used, which is how a real index behaves as the limit grows.
	byFetch map[int]int
	// exactByFetch answers by the exact width instead, for the case a wider round comes back
	// with fewer rows than a narrower one did.
	exactByFetch map[int]int
	fetches      []int
	probes       []int
	searched     []string
	err          error
}

func (f *fakeSearch) SetSearchProbes(_ context.Context, _ repository.Querier, probes int) error {
	f.probes = append(f.probes, probes)
	return nil
}

func (f *fakeSearch) SearchCandidates(
	_ context.Context, _ repository.Querier, _, _ uuid.UUID,
	text string, _ pgvector.Vector, fetch int, _ float64,
) ([]domain.CatalogCandidate, error) {
	f.fetches = append(f.fetches, fetch)
	f.searched = append(f.searched, text)
	if f.err != nil {
		return nil, f.err
	}

	if f.exactByFetch != nil {
		return f.candidates(f.exactByFetch[fetch]), nil
	}
	count := 0
	for width, found := range f.byFetch {
		if width <= fetch && found > count {
			count = found
		}
	}
	return f.candidates(count), nil
}

func (f *fakeSearch) candidates(count int) []domain.CatalogCandidate {
	candidates := make([]domain.CatalogCandidate, count)
	for i := range candidates {
		distance := float64(i) / 10
		candidates[i] = domain.CatalogCandidate{
			ProductID:     uuid.New(),
			CanonicalName: fmt.Sprintf("Producto %02d", i),
			Distance:      &distance,
		}
	}
	return candidates
}

func TestCatalogSearchService_TrimsToTheRequestedLimit(t *testing.T) {
	search := &fakeSearch{byFetch: map[int]int{1: 20}}
	service := NewCatalogSearchService(&fakeDB{}, search, &fakeEmbedder{}, testSearchConfig())

	results, err := service.Search(context.Background(), testSearchTenant(),
		domain.CatalogSearch{Texts: []string{"cemento"}, Limit: 4})
	if err != nil {
		t.Fatalf("Search() = %v, want no error", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want one per text", len(results))
	}
	if got := len(results[0].Candidates); got != 4 {
		t.Errorf("candidates = %d, want the 4 asked for", got)
	}
	// 4 x the over-fetch factor of 2: the database is asked for more than the caller wants,
	// because the branch filter runs after the vector ordering.
	if len(search.fetches) != 1 || search.fetches[0] != 8 {
		t.Errorf("fetch widths = %v, want a single 8", search.fetches)
	}
}

func TestCatalogSearchService_FallsBackToTheConfiguredTopK(t *testing.T) {
	search := &fakeSearch{byFetch: map[int]int{1: 20}}
	service := NewCatalogSearchService(&fakeDB{}, search, &fakeEmbedder{}, testSearchConfig())

	results, err := service.Search(context.Background(), testSearchTenant(),
		domain.CatalogSearch{Texts: []string{"cemento"}})
	if err != nil {
		t.Fatalf("Search() = %v, want no error", err)
	}
	if got := len(results[0].Candidates); got != 3 {
		t.Errorf("candidates = %d, want the configured top K of 3", got)
	}
}

// The acceptance criterion: asking for a fixed number of results returns that many usable ones.
// The branch filter runs after the approximate ordering, so the first fetch comes back short and
// the service has to widen rather than hand back what it got.
func TestCatalogSearchService_WidensTheFetchUntilTheLimitIsMet(t *testing.T) {
	search := &fakeSearch{byFetch: map[int]int{6: 2, 12: 3, 24: 9}}
	service := NewCatalogSearchService(&fakeDB{}, search, &fakeEmbedder{}, testSearchConfig())

	results, err := service.Search(context.Background(), testSearchTenant(),
		domain.CatalogSearch{Texts: []string{"cemento"}, Limit: 3})
	if err != nil {
		t.Fatalf("Search() = %v, want no error", err)
	}
	if got := len(results[0].Candidates); got != 3 {
		t.Errorf("candidates = %d, want the 3 asked for", got)
	}
	if want := []int{6, 12}; !slices.Equal(search.fetches, want) {
		t.Errorf("fetch widths = %v, want %v", search.fetches, want)
	}
}

// The catalog can simply not hold enough of what the branch carries. Widening then has to stop
// on its own, rather than doubling until the process gives up.
func TestCatalogSearchService_StopsWideningWhenTheCatalogIsExhausted(t *testing.T) {
	search := &fakeSearch{byFetch: map[int]int{6: 1}}
	service := NewCatalogSearchService(&fakeDB{}, search, &fakeEmbedder{}, testSearchConfig())

	results, err := service.Search(context.Background(), testSearchTenant(),
		domain.CatalogSearch{Texts: []string{"cemento"}, Limit: 3})
	if err != nil {
		t.Fatalf("Search() = %v, want no error", err)
	}
	if got := len(results[0].Candidates); got != 1 {
		t.Errorf("candidates = %d, want the only one the branch carries", got)
	}
	// Two rounds: the second returns no more than the first, which is what ends it.
	if want := []int{6, 12}; !slices.Equal(search.fetches, want) {
		t.Errorf("fetch widths = %v, want %v", search.fetches, want)
	}
}

// The first round coming back empty is the very case widening exists for: the nearest vectors
// can all be stock this branch does not carry, and only a wider fetch reaches the ones it does.
func TestCatalogSearchService_WidensPastAnEmptyFirstRound(t *testing.T) {
	search := &fakeSearch{byFetch: map[int]int{12: 3}}
	service := NewCatalogSearchService(&fakeDB{}, search, &fakeEmbedder{}, testSearchConfig())

	results, err := service.Search(context.Background(), testSearchTenant(),
		domain.CatalogSearch{Texts: []string{"cemento"}, Limit: 3})
	if err != nil {
		t.Fatalf("Search() = %v, want no error", err)
	}
	if got := len(results[0].Candidates); got != 3 {
		t.Errorf("candidates = %d, want the 3 the branch carries at a wider fetch", got)
	}
	if want := []int{6, 12}; !slices.Equal(search.fetches, want) {
		t.Errorf("fetch widths = %v, want %v", search.fetches, want)
	}
}

// The fetch cannot double forever: a branch that yields one more row per round would otherwise
// walk the doubling all the way up to the size of the catalog.
func TestCatalogSearchService_StopsWideningAtTheConfiguredCeiling(t *testing.T) {
	cfg := testSearchConfig()
	cfg.SearchMaxFetch = 20
	search := &fakeSearch{byFetch: map[int]int{6: 1, 12: 2, 20: 3}}
	service := NewCatalogSearchService(&fakeDB{}, search, &fakeEmbedder{}, cfg)

	if _, err := service.Search(context.Background(), testSearchTenant(),
		domain.CatalogSearch{Texts: []string{"cemento"}, Limit: 50}); err != nil {
		t.Fatalf("Search() = %v, want no error", err)
	}
	if want := []int{20}; !slices.Equal(search.fetches, want) {
		t.Errorf("fetch widths = %v, want %v: the first fetch is already at the ceiling",
			search.fetches, want)
	}
}

// A round that comes back smaller has dropped candidates an earlier one already had.
func TestCatalogSearchService_KeepsTheWidestRoundNotTheLast(t *testing.T) {
	search := &fakeSearch{exactByFetch: map[int]int{6: 2, 12: 1}}
	service := NewCatalogSearchService(&fakeDB{}, search, &fakeEmbedder{}, testSearchConfig())

	results, err := service.Search(context.Background(), testSearchTenant(),
		domain.CatalogSearch{Texts: []string{"cemento"}, Limit: 3})
	if err != nil {
		t.Fatalf("Search() = %v, want no error", err)
	}
	if got := len(results[0].Candidates); got != 2 {
		t.Errorf("candidates = %d, want the 2 the wider round should not have lost", got)
	}
}

// A candidate both halves found beats one either half found alone, whatever the raw figures
// are — the two share no scale, so only their ranks are comparable.
func TestCatalogSearchService_RanksACandidateBothHalvesFoundFirst(t *testing.T) {
	nearest := 0.05
	further := 0.40
	strongText := 0.90
	both := domain.CatalogCandidate{
		ProductID: uuid.New(), CanonicalName: "Hallado por ambas",
		Distance: &further, LexicalScore: &strongText,
	}
	semanticOnly := domain.CatalogCandidate{
		ProductID: uuid.New(), CanonicalName: "Solo vectorial", Distance: &nearest,
	}
	candidates := []domain.CatalogCandidate{semanticOnly, both}

	fuse(candidates, 60)

	if candidates[0].CanonicalName != "Hallado por ambas" {
		t.Errorf("first candidate = %q, want the one both halves found", candidates[0].CanonicalName)
	}
	// 1/61 + 1/61 against 1/61 alone: the second half is the whole difference.
	if candidates[0].Score <= candidates[1].Score {
		t.Errorf("scores = %v and %v, want the fused one higher",
			candidates[0].Score, candidates[1].Score)
	}
	if candidates[1].Score != 1.0/61 {
		t.Errorf("single-half score = %v, want 1/61", candidates[1].Score)
	}
}

// One provider call for the whole search, and one probes statement for the whole transaction:
// both are per-search decisions, not per-line ones.
func TestCatalogSearchService_EmbedsEveryLineInOneCall(t *testing.T) {
	search := &fakeSearch{byFetch: map[int]int{1: 5}}
	embedder := &fakeEmbedder{}
	service := NewCatalogSearchService(&fakeDB{}, search, embedder, testSearchConfig())

	texts := []string{" cemento ", "cal hidratada", "arena"}
	results, err := service.Search(context.Background(), testSearchTenant(),
		domain.CatalogSearch{Texts: texts})
	if err != nil {
		t.Fatalf("Search() = %v, want no error", err)
	}
	if len(results) != 3 {
		t.Fatalf("results = %d, want one per line", len(results))
	}
	if len(embedder.calls) != 1 || len(embedder.calls[0]) != 3 {
		t.Errorf("embedder calls = %v, want one carrying all three lines", embedder.calls)
	}
	if embedder.calls[0][0] != "cemento" {
		t.Errorf("first embedded text = %q, want it trimmed", embedder.calls[0][0])
	}
	if len(search.probes) != 1 || search.probes[0] != 7 {
		t.Errorf("probes statements = %v, want a single 7", search.probes)
	}
	if results[0].Text != "cemento" {
		t.Errorf("result text = %q, want the trimmed line", results[0].Text)
	}
}

func TestCatalogSearchService_RefusesWhatItCannotAnswer(t *testing.T) {
	cases := []struct {
		name     string
		tenant   domain.Tenant
		in       domain.CatalogSearch
		embedder *fakeEmbedder
		wantErr  error
	}{
		{
			// Without a branch the search would offer products the branch does not sell.
			name:     "no active branch",
			tenant:   domain.Tenant{AccountID: testAccountID, Role: domain.UserRoleAdmin},
			in:       domain.CatalogSearch{Texts: []string{"cemento"}},
			embedder: &fakeEmbedder{},
			wantErr:  domain.ErrInvalidInput,
		},
		{
			name:     "a blank line",
			tenant:   testSearchTenant(),
			in:       domain.CatalogSearch{Texts: []string{"cemento", "   "}},
			embedder: &fakeEmbedder{},
			wantErr:  domain.ErrInvalidInput,
		},
		{
			name:     "the provider is unavailable",
			tenant:   testSearchTenant(),
			in:       domain.CatalogSearch{Texts: []string{"cemento"}},
			embedder: &fakeEmbedder{err: domain.ErrAIUnavailable},
			wantErr:  domain.ErrAIUnavailable,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service := NewCatalogSearchService(&fakeDB{}, &fakeSearch{}, tc.embedder,
				testSearchConfig())
			_, err := service.Search(context.Background(), tc.tenant, tc.in)
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("Search() = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

// An embedder that answers with fewer vectors than lines would otherwise pair a line with the
// next line's vector, which is a wrong match nothing downstream could notice.
func TestCatalogSearchService_RefusesAMisalignedEmbeddingAnswer(t *testing.T) {
	service := NewCatalogSearchService(&fakeDB{}, &fakeSearch{}, &fakeEmbedder{short: true},
		testSearchConfig())

	_, err := service.Search(context.Background(), testSearchTenant(),
		domain.CatalogSearch{Texts: []string{"cemento", "cal"}})
	if err == nil {
		t.Fatal("Search() = nil error, want the short answer refused")
	}
	// Not an outage: the provider answered, the answer was unusable.
	if errors.Is(err, domain.ErrAIUnavailable) {
		t.Errorf("Search() = %v, want a fault of ours rather than a provider outage", err)
	}
}
