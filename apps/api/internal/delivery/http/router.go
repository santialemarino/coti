// Package http wires the routes onto a Gin engine. It owns no business logic — the
// handlers it mounts do that, and the composition root builds them.
package http

import (
	"log/slog"
	"strings"

	"github.com/gin-gonic/gin"
	swaggerfiles "github.com/swaggo/files"
	ginswagger "github.com/swaggo/gin-swagger"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/delivery/http/handler"
	"github.com/santialemarino/coti/apps/api/internal/delivery/http/middleware"
	"github.com/santialemarino/coti/apps/api/internal/storage"

	// The generated spec registers itself on import. It is regenerated from the handler
	// annotations by `pnpm docs:api`, and CI fails if the committed copy is stale.
	_ "github.com/santialemarino/coti/apps/api/docs"
)

// apiPrefix is where every versioned route is mounted.
const apiPrefix = "/v1"

// Handlers carries every handler the router mounts.
type Handlers struct {
	Health        *handler.HealthHandler
	Auth          *handler.AuthHandler
	Password      *handler.PasswordHandler
	Verification  *handler.VerificationHandler
	User          *handler.UserHandler
	Branch        *handler.BranchHandler
	Rfq           *handler.RfqHandler
	Channel       *handler.ChannelHandler
	Product       *handler.ProductHandler
	BranchCatalog *handler.BranchCatalogHandler
	RFQ           *handler.RFQHandler
	RFQAttachment *handler.RFQAttachmentHandler
	Quote         *handler.QuoteHandler
	Account       *handler.AccountHandler
	Prices        *handler.ProductPriceHandler
	CatalogImport *handler.CatalogImportHandler
	Onboarding    *handler.OnboardingHandler
	// File is nil unless the local storage adapter is bound.
	File *handler.FileHandler
}

// Auth carries what the authentication middleware needs to resolve a tenant.
type Auth struct {
	Verifier middleware.AccessVerifier
	Resolver middleware.TenantResolver
}

// RateLimit carries the counter and the identity reader the limiter counts by.
type RateLimit struct {
	Limiter  middleware.Limiter
	Identify func(token string) (string, bool)
}

// NewRouter builds the engine with the global middleware and mounts every route.
func NewRouter(cfg *config.Config, log *slog.Logger, h Handlers, auth Auth, rl RateLimit) *gin.Engine {
	if cfg.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(middleware.RequestID(), middleware.Logger(log), middleware.Recovery(log))

	r.GET("/health", h.Health.Live)
	r.GET("/ready", h.Health.Ready)

	if !cfg.IsProduction() {
		r.GET("/swagger/*any", ginswagger.WrapHandler(swaggerfiles.Handler))
	}

	limit := func(scope string, max int) gin.HandlerFunc {
		return middleware.RateLimit(rl.Limiter, middleware.RateLimitOptions{
			Scope:            scope,
			Limit:            max,
			Window:           cfg.RateLimit.Window,
			Enabled:          cfg.RateLimit.Enabled,
			TrustedProxyHops: cfg.RateLimit.TrustedProxyHops,
			TrustedProxies:   cfg.RateLimit.TrustedProxies,
			Identify:         rl.Identify,
		})
	}

	v1 := r.Group(apiPrefix,
		limit("global", cfg.RateLimit.Global),
		middleware.Authenticate(auth.Verifier, auth.Resolver))

	if h.File != nil {
		v1.GET(strings.TrimPrefix(storage.LinkPath, apiPrefix)+"/*key", h.File.Get)
	}

	public := v1.Group("/public")
	mail := limit("mail", cfg.RateLimit.Mail)

	public.POST("/auth/login", limit("login", cfg.RateLimit.Credentials), h.Auth.Login)
	public.POST("/auth/refresh", h.Auth.Refresh)

	public.POST("/auth/forgot-password", mail, h.Password.Forgot)
	public.POST("/auth/reset-password", limit("reset", cfg.RateLimit.Credentials), h.Password.Reset)

	public.POST("/auth/verify-email", limit("verify", cfg.RateLimit.Credentials), h.Verification.Confirm)
	public.POST("/auth/resend-verification", mail, h.Verification.Resend)

	public.POST("/accounts", limit("signup", cfg.RateLimit.Signup), h.Account.Register)

	authed := v1.Group("", middleware.RequireTenant())

	// The three an unconfirmed address does not close, because they are the only way out of
	// that state: closing them would trap whoever mistyped theirs at signup.
	authed.POST("/auth/logout", h.Auth.Logout)
	// The frontend reads its own identity here instead of decoding the access token.
	authed.GET("/me", h.User.Me)
	// On the mail allowance: it sends to an address the caller names.
	authed.POST("/auth/change-email", mail, h.Verification.ChangeEmail)

	// Using the product needs a confirmed address. Everything below is closed until then.
	verified := authed.Group("", middleware.RequireVerifiedEmail(cfg.Auth.RequireVerifiedEmail))
	verified.POST("/auth/change-password", h.Password.Change)

	ai := limit("ai", cfg.RateLimit.AI)
	verified.POST("/rfqs/text-drafts", ai, h.RFQ.CreateTextDraft)
	rfqs := verified.Group("/rfqs")
	rfqs.GET("/:rfqId", h.Rfq.Get)
	rfqs.GET("/:rfqId/attachments", h.RFQAttachment.List)
	rfqs.POST("/:rfqId/attachments", h.RFQAttachment.Upload)

	// Reading is not admin-only: a text draft has to name the channel its order arrived through,
	// so any seller needs the list. Configuring one, credentials included, is admin-only.
	channels := verified.Group("/channels")
	channels.GET("", h.Channel.List)
	channelAdmin := channels.Group("", middleware.RequireAdmin())
	channelAdmin.POST("", h.Channel.Create)
	channelAdmin.PUT("/:channelId", h.Channel.Update)
	channelAdmin.DELETE("/:channelId", h.Channel.Delete)

	// Accepting the materials is what prices the quote, so the route names the seller's action
	// rather than the calculation behind it. It reaches no provider, so it needs no allowance of
	// its own beyond the global one.
	quotes := verified.Group("/quotes")
	quotes.POST("/:quoteId/accept-materials", h.Quote.AcceptMaterials)
	quotes.POST("/:quoteId/transition", h.Quote.Transition)
	quotes.POST("/:quoteId/archive", h.Quote.Archive)
	quotes.POST("/:quoteId/unarchive", h.Quote.Unarchive)
	quotes.POST("/:quoteId/items", h.Rfq.AddItem)
	quotes.PATCH("/:quoteId/items/:itemId", h.Rfq.UpdateItem)
	quotes.DELETE("/:quoteId/items/:itemId", h.Rfq.DeleteItem)

	if !cfg.IsProduction() {
		verified.POST("/dev/whatsapp/messages", ai, h.RFQ.CreateWhatsAppMockDraft)
	}

	account := verified.Group("/account")
	account.GET("", h.Account.Get)
	account.PUT("", middleware.RequireAdmin(), h.Account.Update)

	onboarding := verified.Group("/onboarding", middleware.RequireAdmin())
	onboarding.GET("", h.Onboarding.Get)
	onboarding.PUT("", h.Onboarding.SaveProgress)
	onboarding.POST("/complete", h.Onboarding.Complete)
	onboarding.POST("/dismiss", h.Onboarding.Dismiss)
	onboarding.POST("/resume", h.Onboarding.Resume)

	// The branch switcher needs the list before it can send X-Branch-Id, so reading is not
	// admin-only: the repository already narrows a seller to their assignments. Writing is.
	branches := verified.Group("/branches")
	branches.GET("", h.Branch.List)
	branchAdmin := branches.Group("", middleware.RequireAdmin())
	branchAdmin.POST("", h.Branch.Create)
	branchAdmin.PUT("/:branchId", h.Branch.Update)
	branchAdmin.DELETE("/:branchId", h.Branch.Delete)

	// Manual RFQ intake.
	authed.GET("/rfqs", h.Rfq.List)
	authed.POST("/rfqs", h.Rfq.Create)

	// User administration is the one admin-only group. RequireAdmin runs after RequireTenant,
	// which is what put the role on the context.
	admin := verified.Group("", middleware.RequireAdmin())
	admin.GET("/product-prices/export", h.Prices.Export)
	admin.POST("/product-prices/import/preview", h.Prices.PreviewImport)
	admin.POST("/product-prices/import/confirm", h.Prices.ConfirmImport)
	admin.GET("/products/export", h.CatalogImport.Export)
	admin.POST("/products/import/preview", h.CatalogImport.Preview)
	admin.POST("/products/import/confirm", h.CatalogImport.Confirm)

	users := verified.Group("/users", middleware.RequireAdmin())
	users.GET("", h.User.List)
	users.POST("", h.User.Create)
	users.GET("/:userId", h.User.Get)
	users.PUT("/:userId", h.User.Update)
	users.DELETE("/:userId", h.User.Delete)
	users.POST("/:userId/password-reset", mail, h.Password.AdminReset)

	// The catalog itself is account-scoped, so those routes need no active branch. The
	// per-branch ones below take it from the X-Branch-Id header the middleware validated.
	products := verified.Group("/products")
	products.GET("", h.Product.List)
	products.POST("", h.Product.Create)
	products.GET("/:productId", h.Product.Get)
	products.PUT("/:productId", h.Product.Update)
	products.DELETE("/:productId", h.Product.Delete)
	products.GET("/:productId/synonyms", h.Product.ListSynonyms)
	products.POST("/:productId/synonyms", h.Product.AddSynonym)
	products.DELETE("/:productId/synonyms/:synonymId", h.Product.RemoveSynonym)
	products.GET("/:productId/alternatives", h.Product.ListAlternatives)
	products.POST("/:productId/alternatives", h.Product.AddAlternative)
	products.DELETE("/:productId/alternatives/:alternativeId", h.Product.RemoveAlternative)
	products.GET("/:productId/availability", h.BranchCatalog.ListAvailability)
	products.PUT("/:productId/availability", h.BranchCatalog.SetAvailability)
	products.GET("/:productId/prices", h.BranchCatalog.ListPrices)
	products.POST("/:productId/prices", h.BranchCatalog.SetPrice)

	return r
}
