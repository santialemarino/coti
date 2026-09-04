package services

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/pgvector/pgvector-go"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// catalogSearchRepository is the search surface the service needs. Defined here, in the
// consumer, so a test can fake it without a database.
type catalogSearchRepository interface {
	SetSearchProbes(ctx context.Context, q repository.Querier, probes int) error
	SearchCandidates(ctx context.Context, q repository.Querier, accountID, branchID uuid.UUID,
		text string, embedding pgvector.Vector, fetch int,
		correctionMaxDistance float64) ([]domain.CatalogCandidate, error)
}

// CatalogSearchService resolves RFQ line text against the account's catalog, combining the
// lexical and semantic halves and returning candidates the active branch actually carries.
//
// It offers candidates and nothing more: which of them counts as a match, and which line ends
// up flagged, is the matching service's decision.
type CatalogSearchService struct {
	db       tenantTxRunner
	products catalogSearchRepository
	embedder domain.Embedder
	cfg      config.CatalogConfig
}

// NewCatalogSearchService builds a CatalogSearchService.
func NewCatalogSearchService(
	db tenantTxRunner, products catalogSearchRepository, embedder domain.Embedder,
	cfg config.CatalogConfig,
) *CatalogSearchService {
	return &CatalogSearchService{db: db, products: products, embedder: embedder, cfg: cfg}
}

// Search resolves every line of in against the catalog, returning results index-aligned with
// in.Texts. Each result holds at most in.Limit candidates, best first.
func (s *CatalogSearchService) Search(
	ctx context.Context, tenant domain.Tenant, in domain.CatalogSearch,
) ([]domain.CatalogSearchResult, error) {
	// The branch filter is the whole point: without one the search would offer products the
	// branch does not sell, which is worse than refusing.
	if !tenant.HasBranch() {
		return nil, fmt.Errorf("%w: a catalog search needs an active branch", domain.ErrInvalidInput)
	}
	if len(in.Texts) == 0 {
		return nil, nil
	}
	texts := make([]string, len(in.Texts))
	for i, text := range in.Texts {
		texts[i] = strings.TrimSpace(text)
		if texts[i] == "" {
			return nil, fmt.Errorf("%w: search text %d is empty", domain.ErrInvalidInput, i)
		}
	}
	limit := in.Limit
	if limit < 1 {
		limit = s.cfg.SearchTopK
	}
	// The same ceiling the catalog listing uses: no caller asks the database for the whole
	// catalog in one answer, and here the over-fetch would multiply it again.
	limit = min(limit, s.cfg.MaxPageSize)

	// Outside the transaction on purpose: a provider call is slow and can fail on its own
	// timeline, and no transaction is held open across it.
	vectors, err := s.embedder.Embed(ctx, texts)
	if err != nil {
		return nil, err
	}
	if len(vectors) != len(texts) {
		return nil, fmt.Errorf("embedder returned %d vectors for %d texts", len(vectors), len(texts))
	}

	results := make([]domain.CatalogSearchResult, len(texts))
	err = s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		if probeErr := s.products.SetSearchProbes(ctx, q, s.cfg.SearchProbes); probeErr != nil {
			return probeErr
		}
		// One query per line, not one per candidate: each line carries its own vector and so
		// orders the catalog differently, which no single statement expresses.
		for i, text := range texts {
			candidates, searchErr := s.candidatesFor(ctx, q, tenant, text, vectors[i], limit)
			if searchErr != nil {
				return searchErr
			}
			results[i] = domain.CatalogSearchResult{Text: text, Candidates: candidates}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return results, nil
}

// candidatesFor resolves one line, widening the fetch until the branch filter stops costing
// results. An approximate vector scan orders before that filter runs, so asking for exactly
// limit rows comes back short of it whenever the branch does not carry the nearest products.
func (s *CatalogSearchService) candidatesFor(
	ctx context.Context, q repository.Querier, tenant domain.Tenant,
	text string, vector pgvector.Vector, limit int,
) ([]domain.CatalogCandidate, error) {
	fetch := min(limit*s.cfg.SearchOverFetchFactor, s.cfg.SearchMaxFetch)
	var candidates []domain.CatalogCandidate
	// Below zero so the first round always counts as progress: coming back empty is the very
	// case widening exists for, since the nearest vectors can all be stock the branch lacks.
	previous := -1
	for {
		found, err := s.products.SearchCandidates(ctx, q, tenant.AccountID, tenant.BranchID,
			text, vector, fetch, 1-float64(s.cfg.CorrectionSimilarityPercent)/100)
		if err != nil {
			return nil, err
		}
		// The widest round wins rather than the last: a shrinking one has dropped candidates an
		// earlier round already had.
		if len(found) > len(candidates) {
			candidates = found
		}
		// Widening stopped paying: the index has no more of this account's catalog to give, so
		// another round would repeat the same rows.
		if len(found) >= limit || len(found) <= previous || fetch >= s.cfg.SearchMaxFetch {
			break
		}
		previous = len(found)
		fetch = min(fetch*2, s.cfg.SearchMaxFetch)
	}

	fuse(candidates, s.cfg.SearchRRFK)
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	return candidates, nil
}

// fuse scores each candidate by reciprocal rank fusion — every half it appears in contributes
// 1/(k + its rank) — and orders them best first. Ranks are what make the two halves comparable
// at all: a cosine distance and a text rank share no scale.
func fuse(candidates []domain.CatalogCandidate, k int) {
	semantic := rankBy(candidates, func(c domain.CatalogCandidate) (float64, bool) {
		if c.Distance == nil {
			return 0, false
		}
		return *c.Distance, true
	})
	lexical := rankBy(candidates, func(c domain.CatalogCandidate) (float64, bool) {
		if c.LexicalScore == nil {
			return 0, false
		}
		return -*c.LexicalScore, true
	})

	for i := range candidates {
		candidates[i].Score = contribution(semantic[i], k) + contribution(lexical[i], k)
	}
	slices.SortStableFunc(candidates, func(a, b domain.CatalogCandidate) int {
		if a.LearnedDistance != nil && b.LearnedDistance == nil {
			return -1
		}
		if a.LearnedDistance == nil && b.LearnedDistance != nil {
			return 1
		}
		if a.LearnedDistance != nil && b.LearnedDistance != nil {
			if order := cmp.Compare(*a.LearnedDistance, *b.LearnedDistance); order != 0 {
				return order
			}
		}
		// b before a: the higher fused score is the better candidate.
		if order := cmp.Compare(b.Score, a.Score); order != 0 {
			return order
		}
		return strings.Compare(a.CanonicalName, b.CanonicalName)
	})
}

// rankBy returns each candidate's 1-based position when ordered ascending by key, or zero for
// the ones the key does not apply to.
func rankBy(
	candidates []domain.CatalogCandidate, key func(domain.CatalogCandidate) (float64, bool),
) []int {
	type scored struct {
		index int
		value float64
	}
	var ordered []scored
	for i, c := range candidates {
		if value, ok := key(c); ok {
			ordered = append(ordered, scored{index: i, value: value})
		}
	}
	slices.SortStableFunc(ordered, func(a, b scored) int { return cmp.Compare(a.value, b.value) })

	ranks := make([]int, len(candidates))
	for position, s := range ordered {
		ranks[s.index] = position + 1
	}
	return ranks
}

// contribution is one half's share of the fused score. A rank of zero means the half never
// found the candidate, which is worth nothing rather than worth the top position.
func contribution(rank, k int) float64 {
	if rank == 0 {
		return 0
	}
	return 1 / float64(k+rank)
}
