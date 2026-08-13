// Command api is the Coti backend: the composition root reads configuration, opens
// the connection pools, injects dependencies into the layers, and serves HTTP.
//
//	@title						Coti API
//	@version					1.0
//	@description				Every /v1 route outside /public needs an access token. The active branch travels in the X-Branch-Id header, not in the token, and is validated per request.
//	@BasePath					/
//	@securityDefinitions.apikey	BearerAuth
//	@in							header
//	@name						Authorization
//	@description				Access token, prefixed with "Bearer ".
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/joho/godotenv"

	"github.com/santialemarino/coti/apps/api/internal/ai"
	"github.com/santialemarino/coti/apps/api/internal/ai/anthropic"
	"github.com/santialemarino/coti/apps/api/internal/ai/openai"
	"github.com/santialemarino/coti/apps/api/internal/config"
	deliveryhttp "github.com/santialemarino/coti/apps/api/internal/delivery/http"
	"github.com/santialemarino/coti/apps/api/internal/delivery/http/handler"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/mail"
	"github.com/santialemarino/coti/apps/api/internal/ratelimit"
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
	userBranchRepo := repository.NewUserBranchRepository()
	productRepo := repository.NewProductRepository()
	productSynonymRepo := repository.NewProductSynonymRepository()
	productAlternativeRepo := repository.NewProductAlternativeRepository()
	branchProductRepo := repository.NewBranchProductRepository()
	productPriceRepo := repository.NewProductPriceRepository()
	catalogImportRepo := repository.NewCatalogImportRepository()
	accountRepo := repository.NewAccountRepository()
	channelRepo := repository.NewChannelRepository()
	authTokenRepo := repository.NewAuthTokenRepository()
	notificationRepo := repository.NewNotificationRepository()
	limiter := ratelimit.NewMemory(nil)
	mailTargetLimiter := handler.NewMailTargetLimiter(limiter, handler.MailTargetLimitOptions{
		Limit:   cfg.RateLimit.MailPerAddress,
		Window:  cfg.RateLimit.Window,
		Enabled: cfg.RateLimit.Enabled,
	})

	mailer, err := newMailer(cfg, log)
	if err != nil {
		return err
	}

	providers, err := newAIProviders(cfg, log)
	if err != nil {
		return err
	}
	providers.describe(log)

	tokenService := services.NewTokenService(cfg.Auth.JWTSecret, cfg.Auth.AccessTTL, nil)
	authService := services.NewAuthService(db, userRepo, branchRepo, refreshTokenRepo, tokenService, cfg.Auth, nil)
	mailService := services.NewMailService(db, mailer, notificationRepo, accountRepo, nil)
	passwordService := services.NewPasswordService(db, userRepo, authTokenRepo, refreshTokenRepo,
		mailService, authService, log, cfg.Auth, cfg.Web, nil)
	verificationService := services.NewVerificationService(db, userRepo, authTokenRepo,
		mailService, log, cfg.Auth, cfg.Web, nil)
	userService := services.NewUserService(db, userRepo, userBranchRepo, branchRepo, cfg.Auth)
	branchService := services.NewBranchService(db, branchRepo, channelRepo, cfg.Branch.DefaultExpiryDays)
	accountService := services.NewAccountService(db, accountRepo, branchRepo, channelRepo,
		userRepo, authService, verificationService, log, cfg.Auth, cfg.Branch)
	productService := services.NewProductService(db, productRepo, productSynonymRepo,
		productAlternativeRepo, cfg.Catalog)
	branchCatalogService := services.NewBranchCatalogService(db, productRepo, branchProductRepo,
		productPriceRepo, nil)
	productPriceImportService := services.NewProductPriceImportService(db, productPriceRepo, nil)
	catalogImportService := services.NewCatalogImportService(db, catalogImportRepo, nil)

	router := deliveryhttp.NewRouter(cfg, log,
		deliveryhttp.Handlers{
			Health:        handler.NewHealthHandler(db),
			Auth:          handler.NewAuthHandler(authService),
			Password:      handler.NewPasswordHandler(passwordService, mailTargetLimiter),
			Verification:  handler.NewVerificationHandler(verificationService, mailTargetLimiter),
			User:          handler.NewUserHandler(userService),
			Branch:        handler.NewBranchHandler(branchService),
			Product:       handler.NewProductHandler(productService),
			BranchCatalog: handler.NewBranchCatalogHandler(branchCatalogService),
			Prices:        handler.NewProductPriceHandler(productPriceImportService, cfg.PriceImport.MaxBytes),
			CatalogImport: handler.NewCatalogImportHandler(catalogImportService, cfg.CatalogImport.MaxBytes),
			Account:       handler.NewAccountHandler(accountService),
		},
		deliveryhttp.Auth{Verifier: tokenService, Resolver: authService},
		deliveryhttp.RateLimit{Limiter: limiter, Identify: identifyForRateLimit(tokenService)},
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

// identifyForRateLimit reads a caller id out of a bearer so two users cannot spend each
// other's allowance. Signature only: the session check is ResolveTenant's job.
func identifyForRateLimit(tokens *services.TokenService) func(string) (string, bool) {
	return func(raw string) (string, bool) {
		claims, err := tokens.ParseAccessToken(raw)
		if err != nil {
			return "", false
		}
		return claims.UserID.String(), true
	}
}

// aiProviders is the set of AI adapters bound at startup. The RFQ engine's services take them
// from here through the domain ports, so none of them ever names a provider.
type aiProviders struct {
	Generator   domain.StructuredGenerator
	Embedder    domain.Embedder
	Transcriber domain.Transcriber
}

// newAIProviders binds each AI port to the provider selected for it, and is the only place those
// choices are made. No provider covers all three capabilities, so each is selected on its own and
// any of them can be left unbound. config.Load rejects a provider with no adapter.
func newAIProviders(cfg *config.Config, log *slog.Logger) (aiProviders, error) {
	providers := aiProviders{
		Generator:   ai.DisabledGenerator{},
		Embedder:    ai.DisabledEmbedder{},
		Transcriber: ai.DisabledTranscriber{},
	}

	switch cfg.AI.LLMProvider {
	case config.AIProviderDisabled:
		log.Warn("no language model is bound: extraction and the change handler will refuse")
	case config.AIProviderAnthropic:
		providers.Generator = anthropic.NewGenerator(cfg.AI, log)
	default:
		return providers, fmt.Errorf("no language model adapter for provider %q", cfg.AI.LLMProvider)
	}

	switch cfg.AI.EmbeddingsProvider {
	case config.AIProviderDisabled:
		log.Warn("no embedding provider is bound: semantic catalog search will refuse")
	case config.AIProviderOpenAI:
		providers.Embedder = openai.NewEmbedder(cfg.AI, log)
	default:
		return providers, fmt.Errorf("no embedding adapter for provider %q", cfg.AI.EmbeddingsProvider)
	}

	switch cfg.AI.TranscriptionProvider {
	case config.AIProviderDisabled:
		log.Warn("no transcription provider is bound: audio ingest will refuse")
	case config.AIProviderOpenAI:
		providers.Transcriber = openai.NewTranscriber(cfg.AI, log)
	default:
		return providers, fmt.Errorf("no transcription adapter for provider %q",
			cfg.AI.TranscriptionProvider)
	}
	return providers, nil
}

// describe records which adapter ended up behind each AI port, so a deployment can be read back
// from its own startup log instead of from the environment it was given.
func (p aiProviders) describe(log *slog.Logger) {
	log.Info("ai providers bound",
		slog.String("language_model", fmt.Sprintf("%T", p.Generator)),
		slog.String("embeddings", fmt.Sprintf("%T", p.Embedder)),
		slog.String("transcription", fmt.Sprintf("%T", p.Transcriber)))
}

// newMailer binds the domain.Mailer port to the transport configuration selected, and is the
// only place a provider is chosen. config.Load rejects a provider with no adapter.
func newMailer(cfg *config.Config, log *slog.Logger) (domain.Mailer, error) {
	switch cfg.Mail.Provider {
	case config.MailProviderConsole:
		log.Warn("outbound mail goes to the log, not to a recipient",
			slog.String("provider", string(cfg.Mail.Provider)))
		return mail.NewConsoleMailer(log, cfg.Mail.FromAddress), nil
	case config.MailProviderSMTP:
		return mail.NewSMTPMailer(cfg.Mail), nil
	default:
		return nil, fmt.Errorf("no mail adapter for provider %q", cfg.Mail.Provider)
	}
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
