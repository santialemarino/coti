//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"

	"github.com/santialemarino/coti/apps/api/internal/ai"
	"github.com/santialemarino/coti/apps/api/internal/ai/provider"
	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
	"github.com/santialemarino/coti/apps/api/internal/services"
)

// The catalog match evaluation: a real corralón catalog, lines labeled with what a seller would
// accept, and the real embedding model — so it costs money, varies slightly between runs, and is
// opt-in. `pnpm eval:catalog-match` runs it; testdata/catalog_match/README.md says how to read and
// extend it. The suite at large compiles it and skips it.

const catalogMatchEvalDir = "testdata/catalog_match"

// evalOrderSize is how many lines go through matching together, the way an order's lines do: the
// review caps what one order may send, so a whole set in one call would not measure it.
const evalOrderSize = 10

type evalProduct struct {
	Name        string  `json:"name"`
	Description *string `json:"description"`
	Unit        string  `json:"unit"`
}

type evalCase struct {
	Line     string   `json:"line"`
	Statuses []string `json:"statuses"`
	Products []string `json:"products"`
}

// Outcomes of one line, from right to the one that must never happen.
const (
	outcomeCorrect          = "CORRECT"
	outcomeOvercautious     = "OVERCAUTIOUS"
	outcomeOverconfident    = "OVERCONFIDENT"
	outcomeWrongLeader      = "WRONG_LEADER"
	outcomeMissed           = "MISSED"
	outcomeConfidentlyWrong = "CONFIDENTLY_WRONG"
)

type evalCandidate struct {
	Name       string `json:"name"`
	Confidence string `json:"confidence"`
}

type evalResult struct {
	evalCase
	Outcome    string          `json:"outcome"`
	Status     string          `json:"status"`
	Confidence string          `json:"confidence"`
	Candidates []evalCandidate `json:"candidates"`
}

type evalSetReport struct {
	Set      string         `json:"set"`
	Lines    int            `json:"lines"`
	Outcomes map[string]int `json:"outcomes"`
	Accuracy float64        `json:"accuracy"`
	Results  []evalResult   `json:"results"`
}

func TestCatalogMatchEvaluation(t *testing.T) {
	if os.Getenv("CATALOG_MATCH_EVAL") != "1" {
		t.Skip("set CATALOG_MATCH_EVAL=1, or run pnpm eval:catalog-match, to evaluate against " +
			"a live model")
	}
	e := newEnv(t)
	// The calibration under test is the one the API would run with, keys included.
	_ = godotenv.Load("../../.env")
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.AI.EmbeddingsProvider == config.AIProviderDisabled {
		t.Fatal("AI_EMBEDDINGS_PROVIDER is disabled: the evaluation measures the real " +
			"embedding model")
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	providers, err := provider.Bind(cfg.AI, log)
	if err != nil {
		t.Fatalf("bind providers: %v", err)
	}

	var catalog []evalProduct
	readEvalFile(t, "catalog.json", &catalog)
	accountID, branchID := e.seedAccount(t, "Catalog match evaluation")
	e.seedEvalCatalog(t, accountID, branchID, catalog)
	ctx := context.Background()
	embeddings := services.NewCatalogEmbeddingService(e.db, repository.NewAccountRepository(),
		repository.NewProductRepository(), providers.Embedder, cfg.Catalog)
	if _, err := embeddings.Backfill(ctx, domain.Tenant{AccountID: accountID}, true); err != nil {
		t.Fatalf("embed the catalog: %v", err)
	}

	search := services.NewCatalogSearchService(e.db, repository.NewProductRepository(),
		providers.Embedder, cfg.Catalog)
	matcher := services.NewCatalogMatchService(search, cfg.Catalog)
	reviewed := os.Getenv("CATALOG_MATCH_EVAL_REVIEW") == "1"
	if reviewed {
		if cfg.AI.LLMProvider == config.AIProviderDisabled {
			t.Fatal("CATALOG_MATCH_EVAL_REVIEW=1 needs AI_LLM_PROVIDER: the review is the " +
				"language model")
		}
		matcher.WithReviewer(ai.NewCatalogMatchReviewer(providers.Generator), log)
	}
	tenant := domain.Tenant{AccountID: accountID, BranchID: branchID, Role: domain.UserRoleAdmin}
	minAccuracy := evalMinAccuracy(t)

	var reports []evalSetReport
	for _, set := range []string{"tuning", "holdout"} {
		var cases []evalCase
		readEvalFile(t, set+".json", &cases)
		report := evaluateSet(t, ctx, matcher, tenant, set, cases)
		reports = append(reports, report)

		t.Logf("%s: %d/%d correct (%.1f%%) %v", set, report.Outcomes[outcomeCorrect], report.Lines,
			100*report.Accuracy, report.Outcomes)
		for _, result := range report.Results {
			if result.Outcome != outcomeCorrect {
				t.Logf("  %-17s %-36q got %-9s %s, leading %v", result.Outcome, result.Line,
					result.Status, result.Confidence, leaderOf(result))
			}
		}
		// A wrong product marked decided is the one error a seller would not catch.
		if n := report.Outcomes[outcomeConfidentlyWrong]; n > 0 {
			t.Errorf("%s: %d lines matched to a wrong product with confidence, want none", set, n)
		}
		if report.Accuracy < minAccuracy {
			t.Errorf("%s: accuracy %.3f, want at least %.3f", set, report.Accuracy, minAccuracy)
		}
	}
	writeEvalReport(t, cfg.Catalog, reviewed, reports)
}

// evaluateSet matches a set in order-sized batches and grades every line against its labels.
func evaluateSet(
	t *testing.T, ctx context.Context, matcher *services.CatalogMatchService,
	tenant domain.Tenant, set string, cases []evalCase,
) evalSetReport {
	t.Helper()
	report := evalSetReport{Set: set, Lines: len(cases), Outcomes: map[string]int{}}
	for start := 0; start < len(cases); start += evalOrderSize {
		batch := cases[start:min(start+evalOrderSize, len(cases))]
		lines := make([]string, len(batch))
		for i, c := range batch {
			lines[i] = c.Line
		}
		matches, err := matcher.Match(ctx, tenant, lines)
		if err != nil {
			t.Fatalf("%s: match lines %d–%d: %v", set, start+1, start+len(batch), err)
		}
		for i, c := range batch {
			result := gradeMatch(c, matches[i])
			report.Outcomes[result.Outcome]++
			report.Results = append(report.Results, result)
		}
	}
	report.Accuracy = float64(report.Outcomes[outcomeCorrect]) / float64(report.Lines)
	return report
}

// gradeMatch reads one decision against the line's labels. A line is correct when its status is
// one the labels allow and, unless nothing was matched or nothing in the catalog is right, it is
// led by a product the labels accept.
func gradeMatch(c evalCase, match domain.LineMatch) evalResult {
	result := evalResult{evalCase: c, Status: string(match.MatchStatus),
		Confidence: match.Confidence.String()}
	for i, candidate := range match.Candidates {
		if i == 3 {
			break
		}
		result.Candidates = append(result.Candidates, evalCandidate{
			Name: candidate.CanonicalName, Confidence: candidate.Confidence.String()})
	}
	acceptable := func(name string) bool { return slices.Contains(c.Products, name) }
	allowed := slices.Contains(c.Statuses, result.Status)
	leads := len(match.Candidates) > 0 && acceptable(match.Candidates[0].CanonicalName)
	offered := len(c.Products) == 0 || slices.ContainsFunc(match.Candidates,
		func(candidate domain.ScoredCandidate) bool { return acceptable(candidate.CanonicalName) })

	switch {
	case match.MatchStatus == domain.ItemMatchStatusMatched && !leads:
		result.Outcome = outcomeConfidentlyWrong
	case match.MatchStatus == domain.ItemMatchStatusMatched && !allowed:
		result.Outcome = outcomeOverconfident
	case allowed && (match.MatchStatus == domain.ItemMatchStatusNoMatch || len(c.Products) == 0 ||
		leads):
		result.Outcome = outcomeCorrect
	case !offered:
		result.Outcome = outcomeMissed
	case !allowed:
		result.Outcome = outcomeOvercautious
	default:
		result.Outcome = outcomeWrongLeader
	}
	return result
}

// seedEvalCatalog loads the snapshot into the evaluation's own account in one statement, stocked
// at its branch. The account's cleanup takes the products with it.
func (e *env) seedEvalCatalog(
	t *testing.T, accountID, branchID uuid.UUID, catalog []evalProduct,
) {
	t.Helper()
	names := make([]string, len(catalog))
	descriptions := make([]*string, len(catalog))
	units := make([]string, len(catalog))
	for i, product := range catalog {
		names[i], descriptions[i], units[i] = product.Name, product.Description, product.Unit
	}
	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`WITH seeded AS (
		   INSERT INTO product (account_id, canonical_name, description, unit)
		   SELECT $1, name, description, unit
		   FROM unnest($3::text[], $4::text[], $5::text[]) AS u(name, description, unit)
		   RETURNING id
		 )
		 INSERT INTO branch_product (account_id, branch_id, product_id, is_active)
		 SELECT $1, $2, id, TRUE FROM seeded`,
		accountID, branchID, names, descriptions, units); err != nil {
		t.Fatalf("seed the catalog: %v", err)
	}
}

func readEvalFile(t *testing.T, name string, into any) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(catalogMatchEvalDir, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	if err := json.Unmarshal(raw, into); err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}
}

// evalMinAccuracy is the share of lines each set has to get right. The default sits about five
// lines under the measured calibration, more than the embedding model moves between runs.
func evalMinAccuracy(t *testing.T) float64 {
	t.Helper()
	raw := os.Getenv("CATALOG_MATCH_EVAL_MIN_ACCURACY")
	if raw == "" {
		return 0.88
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || value < 0 || value > 1 {
		t.Fatalf("CATALOG_MATCH_EVAL_MIN_ACCURACY = %q, want a share between 0 and 1", raw)
	}
	return value
}

// writeEvalReport stores the run where CATALOG_MATCH_EVAL_REPORT points, with the calibration it
// ran on, so two runs can be compared line by line.
func writeEvalReport(
	t *testing.T, catalog config.CatalogConfig, reviewed bool, reports []evalSetReport,
) {
	t.Helper()
	path := os.Getenv("CATALOG_MATCH_EVAL_REPORT")
	if path == "" {
		return
	}
	payload, err := json.MarshalIndent(map[string]any{
		"ran_at":   time.Now().UTC().Format(time.RFC3339),
		"reviewed": reviewed,
		"calibration": map[string]int{
			"min_confidence_percent":     catalog.MatchMinConfidencePercent,
			"ambiguity_margin_percent":   catalog.MatchAmbiguityMarginPercent,
			"coverage_weight_percent":    catalog.MatchCoverageWeightPercent,
			"similarity_floor_percent":   catalog.MatchSimilarityFloorPercent,
			"similarity_ceiling_percent": catalog.MatchSimilarityCeilingPercent,
			"review_floor_percent":       catalog.MatchReviewFloorPercent,
		},
		"sets": reports,
	}, "", "  ")
	if err != nil {
		t.Fatalf("encode the report: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create the report directory: %v", err)
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatalf("write the report: %v", err)
	}
	t.Logf("report written to %s", path)
}

func leaderOf(result evalResult) string {
	if len(result.Candidates) == 0 {
		return "nothing"
	}
	return fmt.Sprintf("%s at %s", result.Candidates[0].Name, result.Candidates[0].Confidence)
}
