// Command catalog-embed vectorizes one account's catalog so the semantic half of the search
// has something to compare against.
//
// It is a command rather than a route because a catalog is thousands of texts and each provider
// call is bounded per attempt rather than per chain, which no HTTP response budget survives.
// Run it after a catalog load, then create the vector index with `pnpm db:vector-index`.
//
//	go run ./cmd/catalog-embed --account <uuid> [--refresh-all]
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/google/uuid"
	"github.com/joho/godotenv"

	"github.com/santialemarino/coti/apps/api/internal/ai/provider"
	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
	"github.com/santialemarino/coti/apps/api/internal/services"
)

func main() {
	if err := run(); err != nil {
		slog.Error("catalog embedding failed", slog.Any("error", err))
		os.Exit(1)
	}
}

func run() error {
	account := flag.String("account", "", "id of the account whose catalog is embedded")
	refreshAll := flag.Bool("refresh-all", false,
		"re-embed every product, not only the ones missing a vector or edited since")
	flag.Parse()

	accountID, err := uuid.Parse(*account)
	if err != nil {
		return fmt.Errorf("--account must be an account id: %w", err)
	}

	_ = godotenv.Load()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Refused here so the message names the setting at fault. Left to the stand-in it would
	// surface as an unavailable provider, which is not what is wrong.
	if cfg.AI.EmbeddingsProvider == config.AIProviderDisabled {
		return fmt.Errorf("AI_EMBEDDINGS_PROVIDER is %q, so there is nothing to embed with",
			config.AIProviderDisabled)
	}
	providers, err := provider.Bind(cfg.AI, log)
	if err != nil {
		return err
	}
	// Which model produced these vectors is the one fact a later --refresh-all decision needs.
	providers.Describe(log)

	db, err := repository.NewDB(ctx, cfg.Database)
	if err != nil {
		return err
	}
	defer db.Close()

	embeddings := services.NewCatalogEmbeddingService(db, repository.NewAccountRepository(),
		repository.NewProductRepository(), providers.Embedder, cfg.Catalog)

	log.Info("embedding catalog", slog.String("account", accountID.String()),
		slog.Bool("refresh_all", *refreshAll))
	report, err := embeddings.Backfill(ctx, domain.Tenant{AccountID: accountID}, *refreshAll)
	if err != nil {
		// A run that stopped halfway still stored the pages before it, and the next one resumes
		// from there — so the count is worth having, but not under a message saying it finished.
		log.Error("stopped before the catalog was embedded",
			slog.Int("products_stored", report.Embedded), slog.Int("rounds", report.Rounds))
		return err
	}
	log.Info("catalog embedded", slog.Int("products", report.Embedded),
		slog.Int("rounds", report.Rounds))
	return nil
}
