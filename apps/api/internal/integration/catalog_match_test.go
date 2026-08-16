//go:build integration

package integration

import (
	"context"
	"math"
	"testing"

	"github.com/google/uuid"
	"github.com/pgvector/pgvector-go"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
	"github.com/santialemarino/coti/apps/api/internal/services"
)

// Matching composed with the real search: a seeded catalog, real SQL, and both halves fused
// before the service scores what came back. The vectors are synthetic, so no provider is
// involved and every confidence below is arithmetic anyone can redo.
//
// Each line is embedded onto an axis of its own and each product onto the axis of the line that
// should find it, at a chosen alignment. A product on its line's axis sits at cosine distance
// 1-alignment from it, so its similarity is exactly the alignment; a product on any other axis
// is orthogonal to the line, which is a distance of 1 and a similarity of zero.

// defaultAxis is where a line lands when the test staged no axis for it.
const defaultAxis = 0

// axisVector builds a unit vector carrying alignment on one axis and the remainder on the next.
func axisVector(axis int, alignment float64) pgvector.Vector {
	values := make([]float32, domain.EmbeddingDimension)
	values[axis] = float32(alignment)
	values[axis+1] = float32(math.Sqrt(1 - alignment*alignment))
	return pgvector.NewVector(values)
}

// axisEmbedder answers each line with the unit vector of the axis the test put it on. Axes are
// two apart, so a vector's remainder never spills onto another line's.
type axisEmbedder struct{ axes map[string]int }

func (e axisEmbedder) Embed(_ context.Context, texts []string) ([]pgvector.Vector, error) {
	vectors := make([]pgvector.Vector, len(texts))
	for i, text := range texts {
		axis, ok := e.axes[text]
		if !ok {
			axis = defaultAxis
		}
		vectors[i] = axisVector(axis, 1)
	}
	return vectors, nil
}

func matchConfig() config.CatalogConfig {
	return config.CatalogConfig{
		DefaultPageSize: 50, MaxPageSize: 200,
		SearchTopK: 10, SearchOverFetchFactor: 4, SearchMaxFetch: 2000,
		SearchProbes: 10, SearchRRFK: 60, EmbeddingBatchSize: 200,
		MatchMinConfidencePercent: 60, MatchAmbiguityMarginPercent: 5,
		MatchLexicalConfidencePercent: 75,
	}
}

// matcher wires the real search to the matching service the way a composition root would.
func (e *env) matcher(
	t *testing.T, cfg config.CatalogConfig, axes map[string]int,
) *services.CatalogMatchService {
	t.Helper()
	search := services.NewCatalogSearchService(e.db, repository.NewProductRepository(),
		axisEmbedder{axes: axes}, cfg)
	return services.NewCatalogMatchService(search, cfg)
}

// seedProduct creates a catalog product and removes it, its synonyms and its availability at the
// end of the test — the seed that made a row is what takes it away.
func (e *env) seedProduct(t *testing.T, accountID uuid.UUID, name, description string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`INSERT INTO product (id, account_id, canonical_name, description) VALUES ($1, $2, $3, $4)`,
		id, accountID, name, description); err != nil {
		t.Fatalf("seed product: %v", err)
	}
	t.Cleanup(func() {
		e.mustCleanup(t, `DELETE FROM branch_product WHERE product_id = $1`, id)
		e.mustCleanup(t, `DELETE FROM product_synonym WHERE product_id = $1`, id)
		e.mustCleanup(t, `DELETE FROM product WHERE id = $1`, id)
	})
	return id
}

func (e *env) stock(t *testing.T, accountID, branchID, productID uuid.UUID) {
	t.Helper()
	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`INSERT INTO branch_product (account_id, branch_id, product_id, is_active)
		 VALUES ($1, $2, $3, TRUE)`, accountID, branchID, productID); err != nil {
		t.Fatalf("seed branch_product: %v", err)
	}
}

// embedOn stores the product's vector on one line's axis, at the similarity the test wants it
// found with.
func (e *env) embedOn(t *testing.T, productID uuid.UUID, axis int, alignment float64) {
	t.Helper()
	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`UPDATE product SET embedding = $1 WHERE id = $2`,
		axisVector(axis, alignment), productID); err != nil {
		t.Fatalf("seed embedding: %v", err)
	}
}

func (e *env) embed(t *testing.T, productID uuid.UUID, alignment float64) {
	t.Helper()
	e.embedOn(t, productID, defaultAxis, alignment)
}

func (e *env) synonym(t *testing.T, accountID, productID uuid.UUID, term string) {
	t.Helper()
	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`INSERT INTO product_synonym (account_id, product_id, term) VALUES ($1, $2, $3)`,
		accountID, productID, term); err != nil {
		t.Fatalf("seed product_synonym: %v", err)
	}
}

func nameOf(candidates []domain.CatalogCandidate) []string {
	names := make([]string, len(candidates))
	for i, c := range candidates {
		names[i] = c.CanonicalName
	}
	return names
}

func (e *env) matchOne(
	t *testing.T, cfg config.CatalogConfig, accountID, branchID uuid.UUID, text string,
) domain.LineMatch {
	t.Helper()
	tenant := domain.Tenant{AccountID: accountID, BranchID: branchID, Role: domain.UserRoleAdmin}
	matches, err := e.matcher(t, cfg, nil).Match(context.Background(), tenant, []string{text})
	if err != nil {
		t.Fatalf("Match(%q) = %v, want no error", text, err)
	}
	if len(matches) != 1 {
		t.Fatalf("matches = %d, want one per line", len(matches))
	}
	return matches[0]
}

// The acceptance criterion, end to end: a trade term loaded as a synonym resolves to its
// product. The product carries no embedding, so only the lexical half can reach it and the
// configured lexical confidence is what has to carry it over the floor.
func TestCatalogMatch_ResolvesATradeTermThroughASynonym(t *testing.T) {
	e := newEnv(t)
	account, branch := e.seedAccount(t, "Corralon Sinonimos")

	product := e.seedProduct(t, account, "Membrana asfáltica 4mm", "rollo de 10m")
	e.stock(t, account, branch, product)
	e.synonym(t, account, product, "telagoma")

	got := e.matchOne(t, matchConfig(), account, branch, "telagoma")

	if got.MatchStatus != domain.ItemMatchStatusMatched {
		t.Errorf("match status = %q, want MATCHED", got.MatchStatus)
	}
	if got.ProductID == nil || *got.ProductID != product {
		t.Errorf("product = %v, want the membrane %v", got.ProductID, product)
	}
	if want := decimal.RequireFromString("0.75"); !got.Confidence.Equal(want) {
		t.Errorf("confidence = %s, want the configured lexical %s", got.Confidence, want)
	}
}

// A clear leader with nothing near it, scored off a distance the fixture fixes exactly.
func TestCatalogMatch_MatchesAClearLeaderAtTheDistanceItWasSeededWith(t *testing.T) {
	e := newEnv(t)
	account, branch := e.seedAccount(t, "Corralon Match")

	cement := e.seedProduct(t, account, "Cemento Portland 50kg", "bolsa")
	paint := e.seedProduct(t, account, "Pintura látex 20L", "balde")
	e.embed(t, cement, 0.95)
	e.embed(t, paint, 0.20)
	e.stock(t, account, branch, cement)
	e.stock(t, account, branch, paint)

	got := e.matchOne(t, matchConfig(), account, branch, "cemento portland")

	if got.MatchStatus != domain.ItemMatchStatusMatched {
		t.Errorf("match status = %q, want MATCHED", got.MatchStatus)
	}
	if got.ProductID == nil || *got.ProductID != cement {
		t.Errorf("product = %v, want the cement %v", got.ProductID, cement)
	}
	// Seeded at an alignment of 0.95, so the cosine distance is 0.05 and the similarity 0.9500.
	if want := decimal.RequireFromString("0.95"); !got.Confidence.Equal(want) {
		t.Errorf("confidence = %s, want %s", got.Confidence, want)
	}
}

// Two products a hundredth apart are a choice for the seller, not a decided line. Which of the
// two the fusion puts first is the search's business; that neither is taken as settled is this
// service's.
func TestCatalogMatch_FlagsTwoNearIdenticalProductsAmbiguous(t *testing.T) {
	e := newEnv(t)
	account, branch := e.seedAccount(t, "Corralon Ambiguo")

	fifty := e.seedProduct(t, account, "Cemento Portland 50kg", "bolsa")
	twentyFive := e.seedProduct(t, account, "Cemento Portland 25kg", "bolsa")
	e.embed(t, fifty, 0.95)
	e.embed(t, twentyFive, 0.94)
	e.stock(t, account, branch, fifty)
	e.stock(t, account, branch, twentyFive)

	got := e.matchOne(t, matchConfig(), account, branch, "cemento portland")

	if got.MatchStatus != domain.ItemMatchStatusAmbiguous {
		t.Errorf("match status = %q, want AMBIGUOUS", got.MatchStatus)
	}
	if got.ProductID == nil || (*got.ProductID != fifty && *got.ProductID != twentyFive) {
		t.Errorf("product = %v, want one of the two cements", got.ProductID)
	}
	if len(got.Candidates) != 2 {
		t.Errorf("candidates = %d, want both cements kept for the seller", len(got.Candidates))
	}
}

// The product invariant, end to end: a line the branch cannot serve keeps no product, keeps its
// place, and is flagged rather than dropped — even when another branch stocks a perfect match.
func TestCatalogMatch_FlagsALineTheBranchCannotServeNoMatch(t *testing.T) {
	e := newEnv(t)
	account, centro := e.seedAccount(t, "Corralon Sucursal")
	norte := uuid.New()
	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`INSERT INTO branch (id, account_id, name) VALUES ($1, $2, 'Norte')`,
		norte, account); err != nil {
		t.Fatalf("seed branch: %v", err)
	}

	cement := e.seedProduct(t, account, "Cemento Portland 50kg", "bolsa")
	e.embed(t, cement, 1)
	e.stock(t, account, norte, cement)

	got := e.matchOne(t, matchConfig(), account, centro, "cemento portland")

	if got.MatchStatus != domain.ItemMatchStatusNoMatch {
		t.Errorf("match status = %q, want NO_MATCH", got.MatchStatus)
	}
	if got.ProductID != nil {
		t.Errorf("product = %v, want none: the branch does not sell it", *got.ProductID)
	}
	if !got.Confidence.IsZero() {
		t.Errorf("confidence = %s, want zero: nothing was offered", got.Confidence)
	}
	// The branch filter runs in the search's SQL, so the line is offered nothing at all rather
	// than offered the cement and then talked out of it.
	if len(got.Candidates) != 0 {
		t.Errorf("candidates = %v, want none reaching the service", nameOf(got.Candidates))
	}
}

// The threshold is calibration rather than code: the same catalog and the same line decide
// differently once the floor moves.
func TestCatalogMatch_FollowsTheConfiguredThreshold(t *testing.T) {
	e := newEnv(t)
	account, branch := e.seedAccount(t, "Corralon Umbral")

	sand := e.seedProduct(t, account, "Arena fina", "metro cúbico")
	e.embed(t, sand, 0.45)
	e.stock(t, account, branch, sand)

	// Asked with a term the lexical half does not carry, so the vector half alone decides and
	// the similarity is the seeded 0.45.
	if got := e.matchOne(t, matchConfig(), account, branch, "polvo de ladrillo"); got.MatchStatus !=
		domain.ItemMatchStatusNoMatch {
		t.Errorf("match status at a floor of 60%% = %q, want NO_MATCH", got.MatchStatus)
	}

	relaxed := matchConfig()
	relaxed.MatchMinConfidencePercent = 40
	got := e.matchOne(t, relaxed, account, branch, "polvo de ladrillo")
	if got.MatchStatus != domain.ItemMatchStatusMatched {
		t.Errorf("match status at a floor of 40%% = %q, want MATCHED", got.MatchStatus)
	}
	if want := decimal.RequireFromString("0.45"); !got.Confidence.Equal(want) {
		t.Errorf("confidence = %s, want %s", got.Confidence, want)
	}
}

// Several lines resolve in one pass, and each keeps its place: the unmatched one in the middle
// must not shift the ones around it onto the wrong products.
func TestCatalogMatch_KeepsEveryLineInItsPlace(t *testing.T) {
	e := newEnv(t)
	account, branch := e.seedAccount(t, "Corralon Lineas")

	const cementLine, sandLine, doorLine = "cemento portland 50", "arena fina", "puerta placa de roble"
	axes := map[string]int{cementLine: 0, sandLine: 2, doorLine: 4}

	cement := e.seedProduct(t, account, "Cemento Portland 50kg", "bolsa")
	sand := e.seedProduct(t, account, "Arena fina", "metro cúbico")
	e.embedOn(t, cement, axes[cementLine], 0.98)
	e.embedOn(t, sand, axes[sandLine], 0.97)
	e.stock(t, account, branch, cement)
	e.stock(t, account, branch, sand)

	tenant := domain.Tenant{AccountID: account, BranchID: branch, Role: domain.UserRoleAdmin}
	matches, err := e.matcher(t, matchConfig(), axes).Match(context.Background(), tenant,
		[]string{cementLine, doorLine, sandLine})
	if err != nil {
		t.Fatalf("Match() = %v, want no error", err)
	}
	if len(matches) != 3 {
		t.Fatalf("matches = %d, want one per line", len(matches))
	}
	// Each product is orthogonal to the other lines, so the door line is offered both of them at
	// a similarity of zero and keeps neither.
	if matches[0].MatchStatus != domain.ItemMatchStatusMatched ||
		matches[0].ProductID == nil || *matches[0].ProductID != cement {
		t.Errorf("line 1 = %q on %v, want MATCHED on the cement", matches[0].MatchStatus,
			matches[0].ProductID)
	}
	if matches[1].MatchStatus != domain.ItemMatchStatusNoMatch || matches[1].ProductID != nil {
		t.Errorf("line 2 = %q on %v, want NO_MATCH on nothing", matches[1].MatchStatus,
			matches[1].ProductID)
	}
	if matches[2].MatchStatus != domain.ItemMatchStatusMatched ||
		matches[2].ProductID == nil || *matches[2].ProductID != sand {
		t.Errorf("line 3 = %q on %v, want MATCHED on the sand", matches[2].MatchStatus,
			matches[2].ProductID)
	}
}
