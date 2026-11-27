//go:build integration

package integration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"

	"github.com/santialemarino/coti/apps/api/internal/ai/provider"
	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
	"github.com/santialemarino/coti/apps/api/internal/services"
)

type demoBenchmarkCase struct {
	ID             string   `json:"id"`
	Query          string   `json:"query"`
	Category       string   `json:"category"`
	ExpectedCodes  []string `json:"expected_codes"`
	ExpectedStatus string   `json:"expected_status"`
	Reason         string   `json:"reason"`
}

type demoBenchmarkResult struct {
	ID            string     `json:"id"`
	Pass          string     `json:"pass"`
	Status        string     `json:"status"`
	ProductID     *uuid.UUID `json:"product_id"`
	Rank          int        `json:"rank"`
	Correct       bool       `json:"correct"`
	Positive      bool       `json:"positive"`
	Negative      bool       `json:"negative"`
	FalsePositive bool       `json:"false_positive"`
	Error         bool       `json:"error"`
	RetrievalMS   float64    `json:"retrieval_ms"`
	TotalMS       float64    `json:"total_ms"`
}

type observedDemoSearch struct {
	base    *services.CatalogSearchService
	results []domain.CatalogSearchResult
	elapsed time.Duration
}

func (s *observedDemoSearch) Search(ctx context.Context, tenant domain.Tenant, in domain.CatalogSearch) ([]domain.CatalogSearchResult, error) {
	start := time.Now()
	out, err := s.base.Search(ctx, tenant, in)
	s.elapsed = time.Since(start)
	s.results = out
	return out, err
}

func TestDemoCatalogBenchmark(t *testing.T) {
	if os.Getenv("COTI_DEMO_BENCHMARK") != "1" {
		t.Skip("opt-in live provider benchmark; run pnpm demo:catalog --apply --benchmark")
	}
	_ = godotenv.Load("../../.env")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Environment != config.EnvironmentDevelopment {
		t.Fatal("demo benchmark is development-only")
	}
	e := newEnv(t)
	ctx := context.Background()
	tenant := domain.Tenant{AccountID: uuid.MustParse("a0000000-0000-4000-8000-000000000001"), BranchID: uuid.MustParse("b0000000-0000-4000-8000-000000000001"), Role: domain.UserRoleAdmin}
	var dataset struct {
		Version string              `json:"version"`
		Split   string              `json:"split"`
		Cases   []demoBenchmarkCase `json:"cases"`
	}
	readEvalFile(t, "demo.json", &dataset)
	codes := map[string]uuid.UUID{}
	coverage, total := 0, 0
	var signature []string
	err = e.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		rows, err := q.Query(ctx, `SELECT p.id,p.code,p.canonical_name,COALESCE(p.description,''),COALESCE(p.unit,''),
   COALESCE(p.embedding IS NOT NULL AND vector_dims(p.embedding)=1536 AND p.embedding_updated_at >= p.updated_at, FALSE)
   FROM product p JOIN branch_product bp ON bp.account_id=$1 AND bp.product_id=p.id AND bp.branch_id=$2 AND bp.is_active
   WHERE p.account_id=$1 AND p.is_active ORDER BY p.id`, tenant.AccountID, tenant.BranchID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id uuid.UUID
			var code *string
			var name, description, unit string
			var embedded bool
			if err := rows.Scan(&id, &code, &name, &description, &unit, &embedded); err != nil {
				return err
			}
			total++
			if embedded {
				coverage++
			}
			if code != nil {
				codes[*code] = id
			}
			signature = append(signature, id.String()+name+description+unit)
		}
		return rows.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	if total == 0 || coverage != total {
		t.Fatalf("demo catalog embedding coverage %d/%d; prepare embeddings first", coverage, total)
	}
	for _, item := range dataset.Cases {
		for _, code := range item.ExpectedCodes {
			if _, ok := codes[code]; !ok {
				t.Fatalf("case %s references absent code %s", item.ID, code)
			}
		}
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	providers, err := provider.Bind(cfg.AI, log, nil)
	if err != nil {
		t.Fatal(err)
	}
	observed := &observedDemoSearch{base: services.NewCatalogSearchService(e.db, repository.NewProductRepository(), providers.Embedder, cfg.Catalog)}
	matcher := services.NewCatalogMatchService(observed, cfg.Catalog)
	var results []demoBenchmarkResult
	for _, pass := range []string{"first-pass", "repeat-pass"} {
		for _, item := range dataset.Cases {
			result := demoBenchmarkResult{ID: item.ID, Pass: pass, Positive: len(item.ExpectedCodes) > 0, Negative: item.ExpectedStatus == "NO_MATCH"}
			start := time.Now()
			matches, matchErr := matcher.Match(ctx, tenant, []string{item.Query})
			result.TotalMS = float64(time.Since(start).Microseconds()) / 1000
			result.RetrievalMS = float64(observed.elapsed.Microseconds()) / 1000
			if matchErr != nil || len(matches) != 1 {
				result.Error = true
				results = append(results, result)
				continue
			}
			match := matches[0]
			result.Status = string(match.MatchStatus)
			result.ProductID = match.ProductID
			expected := map[uuid.UUID]bool{}
			for _, code := range item.ExpectedCodes {
				expected[codes[code]] = true
			}
			if len(observed.results) == 1 {
				for i, candidate := range observed.results[0].Candidates {
					if expected[candidate.ProductID] {
						result.Rank = i + 1
						break
					}
				}
			}
			result.Correct = result.Status == item.ExpectedStatus && (len(expected) == 0 || (match.ProductID != nil && expected[*match.ProductID]))
			result.FalsePositive = result.Negative && match.MatchStatus == domain.ItemMatchStatusMatched
			results = append(results, result)
		}
	}
	raw, _ := json.Marshal(signature)
	fingerprint := sha256.Sum256(raw)
	report := map[string]any{"expected_code_ids": codes, "dataset_version": dataset.Version, "dataset_split": dataset.Split, "dataset_cases": len(dataset.Cases), "catalog_hash": hex.EncodeToString(fingerprint[:]), "catalog_products": total, "embedded_products": coverage, "embedding_model": cfg.AI.Embeddings().Model, "embedding_dimension": domain.EmbeddingDimension, "commit": os.Getenv("COTI_BENCHMARK_COMMIT"), "working_tree": "local sprint implementation; see git diff", "environment": "local PostgreSQL with real embedding provider", "ran_at": time.Now().UTC().Format(time.RFC3339), "top_k": cfg.Catalog.SearchTopK, "cases": dataset.Cases, "results": results, "first_pass": summarizeDemoBenchmark(results, "first-pass", cfg.Catalog.SearchTopK), "repeat_pass": summarizeDemoBenchmark(results, "repeat-pass", cfg.Catalog.SearchTopK), "limitations": []string{"First/repeat passes do not flush OS, PostgreSQL or provider caches; they are not controlled cold/warm measurements.", "Hybrid retrieval and final decision are measured separately on the same real search call.", "Language-model review is disabled; the benchmark evaluates retrieval and deterministic matching only.", "No quality acceptance threshold or comparable baseline is specified. Multi-item extraction is covered by the RFQ integration suite."}}
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join("..", "..", "..", "..", ".artifacts", "demo-catalog-benchmark", "report.json")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, encoded, 0644); err != nil {
		t.Fatal(err)
	}
	t.Logf("report: %s; first pass: %+v", path, report["first_pass"])
	for _, result := range results {
		if result.Error {
			t.Error("provider/search errors occurred; inspect report")
		}
	}
}

func summarizeDemoBenchmark(results []demoBenchmarkResult, pass string, k int) map[string]any {
	count, positive, negative, correct, top1, recall, falsePositive, review, failures := 0, 0, 0, 0, 0, 0, 0, 0, 0
	reciprocal := 0.0
	var latencies, retrieval []float64
	for _, result := range results {
		if result.Pass != pass {
			continue
		}
		count++
		if result.Positive {
			positive++
		}
		if result.Negative {
			negative++
		}
		if result.Error {
			failures++
			continue
		}
		latencies = append(latencies, result.TotalMS)
		retrieval = append(retrieval, result.RetrievalMS)
		if result.Correct {
			correct++
		}
		if result.Positive && result.Rank == 1 {
			top1++
		}
		if result.Positive && result.Rank > 0 {
			reciprocal += 1 / float64(result.Rank)
			if result.Rank <= k {
				recall++
			}
		}
		if result.FalsePositive {
			falsePositive++
		}
		if result.Status != "MATCHED" {
			review++
		}
	}
	ratio := func(n, d int) any {
		if d == 0 {
			return nil
		}
		return float64(n) / float64(d)
	}
	var mrr any
	if positive > 0 {
		mrr = reciprocal / float64(positive)
	}
	return map[string]any{"cases": count, "positive_cases": positive, "negative_cases": negative, "correct_decisions": correct, "decision_accuracy": ratio(correct, count), "retrieval_top1_accuracy": ratio(top1, positive), "recall_at_k": ratio(recall, positive), "mrr": mrr, "false_positives": falsePositive, "false_positive_rate": ratio(falsePositive, negative), "review_cases": review, "review_rate": ratio(review, count), "errors": failures, "total_p50_ms": demoPercentile(latencies, .5), "total_p95_ms": demoPercentile(latencies, .95), "retrieval_p50_ms": demoPercentile(retrieval, .5), "retrieval_p95_ms": demoPercentile(retrieval, .95)}
}

func demoPercentile(values []float64, quantile float64) any {
	if len(values) == 0 {
		return nil
	}
	sorted := slices.Clone(values)
	slices.Sort(sorted)
	return sorted[int(math.Ceil(float64(len(sorted))*quantile))-1]
}

func TestDemoBenchmarkMetrics(t *testing.T) {
	results := []demoBenchmarkResult{{Pass: "test", Positive: true, Rank: 1, Correct: true, TotalMS: 10}, {Pass: "test", Positive: true, Rank: 2, TotalMS: 20, Status: "AMBIGUOUS"}, {Pass: "test", Negative: true, FalsePositive: true, TotalMS: 30, Status: "MATCHED"}}
	got := summarizeDemoBenchmark(results, "test", 2)
	if got["recall_at_k"] != 1.0 || got["retrieval_top1_accuracy"] != 0.5 || got["mrr"] != 0.75 || got["false_positives"] != 1 || got["total_p95_ms"] != float64(30) {
		t.Fatalf("metrics=%+v", got)
	}
}
