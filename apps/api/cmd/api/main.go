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
	aiprovider "github.com/santialemarino/coti/apps/api/internal/ai/provider"
	"github.com/santialemarino/coti/apps/api/internal/config"
	deliveryhttp "github.com/santialemarino/coti/apps/api/internal/delivery/http"
	"github.com/santialemarino/coti/apps/api/internal/delivery/http/handler"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/mail"
	"github.com/santialemarino/coti/apps/api/internal/ratelimit"
	"github.com/santialemarino/coti/apps/api/internal/repository"
	"github.com/santialemarino/coti/apps/api/internal/secrets"
	"github.com/santialemarino/coti/apps/api/internal/services"
	storageprovider "github.com/santialemarino/coti/apps/api/internal/storage/provider"
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
	rfqRepo := repository.NewRFQRepository()
	rfqAttachmentRepo := repository.NewRFQAttachmentRepository()
	quoteRepo := repository.NewQuoteRepository()
	accountRepo := repository.NewAccountRepository()
	onboardingRepo := repository.NewOnboardingRepository()
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

	providers, err := aiprovider.Bind(cfg.AI, log)
	if err != nil {
		return err
	}
	providers.Describe(log)

	objectStorage, err := storageprovider.Bind(cfg.Storage, log)
	if err != nil {
		return err
	}
	objectStorage.Describe(log)

	channelSealer, err := secrets.NewAESGCM(cfg.Channel.EncryptionKey)
	if err != nil {
		return err
	}
	if !channelSealer.Enabled() {
		log.Warn("channel credentials cannot be stored: CHANNEL_CONFIG_ENCRYPTION_KEY is unset")
	}

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
		userRepo, onboardingRepo, authService, verificationService, log, cfg.Auth, cfg.Branch)
	onboardingService := services.NewOnboardingService(db, onboardingRepo)
	productService := services.NewProductService(db, productRepo, productSynonymRepo,
		productAlternativeRepo, cfg.Catalog)
	branchCatalogService := services.NewBranchCatalogService(db, productRepo, branchProductRepo,
		productPriceRepo, nil)
	productPriceImportService := services.NewProductPriceImportService(db, productPriceRepo, nil)
	catalogImportService := services.NewCatalogImportService(db, catalogImportRepo, nil)
	channelService := services.NewChannelService(db, channelRepo, channelSealer)
	catalogSearchService := services.NewCatalogSearchService(db, productRepo, providers.Embedder,
		cfg.Catalog)
	catalogMatchService := services.NewCatalogMatchService(catalogSearchService, cfg.Catalog)
	rfqExtractor := ai.NewRFQExtractor(providers.Generator, cfg.RFQ.MaxItems)
	rfqService := services.NewRFQService(db, rfqRepo, quoteRepo, channelRepo, rfqExtractor,
		catalogMatchService, log, cfg.RFQ)
	quoteService := services.NewQuoteService(db, quoteRepo, productPriceRepo, log)
	rfqAttachmentService := services.NewRFQAttachmentService(db, rfqAttachmentRepo,
		objectStorage.Storage, cfg.Storage, nil)

	router := deliveryhttp.NewRouter(cfg, log,
		deliveryhttp.Handlers{
			Health:        handler.NewHealthHandler(db),
			Auth:          handler.NewAuthHandler(authService),
			Password:      handler.NewPasswordHandler(passwordService, mailTargetLimiter),
			Verification:  handler.NewVerificationHandler(verificationService, mailTargetLimiter),
			User:          handler.NewUserHandler(userService),
			Branch:        handler.NewBranchHandler(branchService),
			Rfq:           handler.NewRfqHandler(rfqService),
			Channel:       handler.NewChannelHandler(channelService),
			Product:       handler.NewProductHandler(productService),
			BranchCatalog: handler.NewBranchCatalogHandler(branchCatalogService),
			RFQ:           handler.NewRFQHandler(rfqService),
			RFQAttachment: handler.NewRFQAttachmentHandler(rfqAttachmentService, cfg.Storage.MaxFileSize),
			Quote:         handler.NewQuoteHandler(quoteService),
			Prices:        handler.NewProductPriceHandler(productPriceImportService, cfg.PriceImport.MaxBytes),
			CatalogImport: handler.NewCatalogImportHandler(catalogImportService, cfg.CatalogImport.MaxBytes),
			Account:       handler.NewAccountHandler(accountService),
			Onboarding:    handler.NewOnboardingHandler(onboardingService),
			File:          fileHandler(objectStorage),
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

func identifyForRateLimit(tokens *services.TokenService) func(string) (string, bool) {
	return func(raw string) (string, bool) {
		claims, err := tokens.ParseAccessToken(raw)
		if err != nil {
			return "", false
		}
		return claims.UserID.String(), true
	}
}

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

func fileHandler(set storageprovider.Set) *handler.FileHandler {
	if set.Local == nil {
		return nil
	}
	return handler.NewFileHandler(set.Local)
}
