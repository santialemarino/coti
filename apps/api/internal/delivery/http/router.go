// Package http wires the routes onto a Gin engine. It owns no business logic — the
// handlers it mounts do that, and the composition root builds them.
package http

import (
	"log/slog"

	"github.com/gin-gonic/gin"
	swaggerfiles "github.com/swaggo/files"
	ginswagger "github.com/swaggo/gin-swagger"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/delivery/http/handler"
	"github.com/santialemarino/coti/apps/api/internal/delivery/http/middleware"

	// The generated spec registers itself on import. It is regenerated from the handler
	// annotations by `pnpm docs:api`, and CI fails if the committed copy is stale.
	_ "github.com/santialemarino/coti/apps/api/docs"
)

// Handlers carries every handler the router mounts, so adding a feature is one field
// here instead of a new router parameter.
type Handlers struct {
	Health        *handler.HealthHandler
	Auth          *handler.AuthHandler
	Product       *handler.ProductHandler
	BranchCatalog *handler.BranchCatalogHandler
}

// Auth carries what the authentication middleware needs to resolve a tenant.
type Auth struct {
	Verifier middleware.AccessVerifier
	Resolver middleware.TenantResolver
}

// NewRouter builds the engine with the global middleware and mounts every route.
func NewRouter(cfg *config.Config, log *slog.Logger, h Handlers, auth Auth) *gin.Engine {
	if cfg.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(middleware.RequestID(), middleware.Logger(log), middleware.Recovery(log))

	// Probes stay outside /v1 and outside auth: an orchestrator has no credentials.
	r.GET("/health", h.Health.Live)
	r.GET("/ready", h.Health.Ready)

	// The spec describes an internal API, so it is not published in production. Serving it
	// there would hand an unauthenticated reader the whole surface for no benefit — the
	// consumers are this repo's own web apps and whoever is writing them.
	if !cfg.IsProduction() {
		r.GET("/swagger/*any", ginswagger.WrapHandler(swaggerfiles.Handler))
	}

	// Authenticate runs for every /v1 route and resolves a tenant when a valid token is
	// present. It does not reject anonymous requests — RequireTenant does — so a public
	// route can still see who the caller is when they happen to be logged in.
	v1 := r.Group("/v1", middleware.Authenticate(auth.Verifier, auth.Resolver))

	// Works without a session. Each route resolves its own scope before touching
	// tenant-scoped data.
	public := v1.Group("/public")
	public.POST("/auth/login", h.Auth.Login)
	public.POST("/auth/refresh", h.Auth.Refresh)

	// Everything else needs a resolved tenant: a request-scoped query without an account
	// reads nothing under row level security.
	authed := v1.Group("", middleware.RequireTenant())
	authed.POST("/auth/logout", h.Auth.Logout)

	// The catalog itself is account-scoped, so those routes need no active branch. The
	// per-branch ones below take it from the X-Branch-Id header the middleware validated.
	products := authed.Group("/products")
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
