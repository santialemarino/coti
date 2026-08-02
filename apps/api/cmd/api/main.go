// Command api is the Coti backend: the composition root reads configuration, opens
// the connection pools, injects dependencies into the layers, and serves HTTP.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/joho/godotenv"

	"github.com/santialemarino/coti/apps/api/internal/config"
	deliveryhttp "github.com/santialemarino/coti/apps/api/internal/delivery/http"
	"github.com/santialemarino/coti/apps/api/internal/delivery/http/handler"
	"github.com/santialemarino/coti/apps/api/internal/repository"
	"github.com/santialemarino/coti/apps/api/internal/services"
)

func main() {
	if err := run(); err != nil {
		slog.Error("startup failed", slog.Any("error", err))
		os.Exit(1)
	}
}

func run() error {
	_ = godotenv.Load() // .env is optional in production.

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	log := newLogger(cfg)
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	db, err := repository.NewDB(ctx, cfg.Database)
	if err != nil {
		return err
	}
	defer db.Close()
	log.Info("database pools ready")

	userRepo := repository.NewUserRepository()
	branchRepo := repository.NewBranchRepository()
	refreshTokenRepo := repository.NewRefreshTokenRepository()
	productPriceRepo := repository.NewProductPriceRepository()

	tokenService := services.NewTokenService(cfg.Auth.JWTSecret, cfg.Auth.AccessTTL, nil)
	authService := services.NewAuthService(db, userRepo, branchRepo, refreshTokenRepo, tokenService, cfg.Auth, nil)
	productPriceImportService := services.NewProductPriceImportService(db, productPriceRepo, nil)
	branchService := services.NewBranchService(db, branchRepo)

	router := deliveryhttp.NewRouter(cfg, log,
		deliveryhttp.Handlers{
			Health: handler.NewHealthHandler(db),
			Auth:   handler.NewAuthHandler(authService),
			Branch: handler.NewBranchHandler(branchService),
			Prices: handler.NewProductPriceHandler(productPriceImportService, cfg.PriceImport.MaxBytes),
		},
		deliveryhttp.Auth{Verifier: tokenService, Resolver: authService},
	)

	server := &http.Server{
		Addr:         ":" + cfg.Server.Port,
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	serverErr := make(chan error, 1)
	go func() {
		log.Info("api listening", slog.String("port", cfg.Server.Port),
			slog.String("environment", string(cfg.Environment)))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		return err
	case <-ctx.Done():
		log.Info("shutting down")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()
	return server.Shutdown(shutdownCtx)
}

// newLogger returns a JSON logger in production and a text one in development, where
// a human reads it directly.
func newLogger(cfg *config.Config) *slog.Logger {
	opts := &slog.HandlerOptions{Level: parseLevel(cfg.LogLevel)}
	if cfg.IsProduction() {
		return slog.New(slog.NewJSONHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewTextHandler(os.Stdout, opts))
}

func parseLevel(raw string) slog.Level {
	switch strings.ToLower(raw) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
