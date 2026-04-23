package main

import (
	"context"
	"crypto/rsa"

	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/bootstrap"
	"github.com/usehiveloop/hiveloop/internal/handler"
	"github.com/usehiveloop/hiveloop/internal/middleware"
)

func setupPublicRoutes(
	r chi.Router,
	cfg *bootstrap.Config,
	database *gorm.DB,
	redisClient *redis.Client,
	providerHandler *handler.ProviderHandler,
	inIntegrationHandler *handler.InIntegrationHandler,
	actionsCatalog bootstrap.ActionsCatalog,
	marketplaceHandler *handler.MarketplaceHandler,
	orgInviteHandler *handler.OrgInviteHandler,
	bridgeWebhookHandler *handler.BridgeWebhookHandler,
	nangoWebhookHandler *handler.NangoWebhookHandler,
	incomingWebhookHandler *handler.IncomingWebhookHandler,
	nangoClient bootstrap.NangoClient,
	sandboxEncKey []byte,
) {
	r.Get("/healthz", healthz)
	r.Get("/readyz", readyz(database, redisClient))

	// Provider discovery (no auth)
	r.Get("/v1/providers", providerHandler.List)
	r.Get("/v1/providers/{id}", providerHandler.Get)
	r.Get("/v1/providers/{id}/models", providerHandler.Models)

	// In-integration discovery (no auth)
	r.Get("/v1/in/integrations/available", inIntegrationHandler.ListAvailable)

	// Integration catalog discovery (no auth)
	actionsHandler := handler.NewActionsHandler(actionsCatalog)
	r.Get("/v1/catalog/integrations", actionsHandler.ListIntegrations)
	r.Get("/v1/catalog/integrations/{id}", actionsHandler.GetIntegration)
	r.Get("/v1/catalog/integrations/{id}/actions", actionsHandler.ListActions)
	r.Get("/v1/catalog/integrations/{id}/triggers", actionsHandler.ListTriggers)
	r.Get("/v1/catalog/integrations/{id}/schema-paths", actionsHandler.GetSchemaPaths)

	// Marketplace discovery (no auth, Redis cached)
	r.Get("/v1/marketplace/agents", marketplaceHandler.List)
	r.Get("/v1/marketplace/agents/{slug}", marketplaceHandler.GetBySlug)

	// Org invite preview (public, token-based lookup)
	r.Get("/v1/invites/{token}", orgInviteHandler.Preview)

	// Webhook receivers (HMAC-verified, no auth middleware)
	r.Post("/internal/webhooks/bridge/{sandboxID}", bridgeWebhookHandler.Handle)
	r.Post("/internal/webhooks/nango", nangoWebhookHandler.Handle)
	if cfg.PolarWebhookSecret != "" {
		polarWebhookHandler := handler.NewPolarWebhookHandler(database, cfg.PolarWebhookSecret, cfg.PolarProductFreeID)
		r.Post("/internal/webhooks/polar", polarWebhookHandler.Handle)
	}

	// Sandbox proxy endpoints (bearer-token auth, no middleware)
	if nangoClient != nil && sandboxEncKey != nil {
		gitCredsHandler := handler.NewGitCredentialsHandler(database, sandboxEncKey, nangoClient)
		r.Post("/internal/git-credentials/{agentID}", gitCredsHandler.Handle)

		railwayProxyHandler := handler.NewRailwayProxyHandler(database, sandboxEncKey, nangoClient)
		r.Post("/internal/railway-proxy/{agentID}", railwayProxyHandler.Handle)
	}

	// Direct incoming webhooks for providers requiring manual webhook configuration
	r.Post("/incoming/webhooks/{provider}/{connectionID}", incomingWebhookHandler.Handle)
}

func setupAuthRoutes(
	r chi.Router,
	ctx context.Context,
	cfg *bootstrap.Config,
	rsaPub *rsa.PublicKey,
	authHandler *handler.AuthHandler,
	oauthHandler *handler.OAuthHandler,
) {
	r.Route("/auth", func(r chi.Router) {
		r.Use(middleware.AuthRateLimit(ctx, 10, 20))
		r.Post("/register", authHandler.Register)
		r.Post("/login", authHandler.Login)
		r.Post("/refresh", authHandler.Refresh)
		r.Post("/otp/request", authHandler.OTPRequest)
		r.Post("/otp/verify", authHandler.OTPVerify)
		r.Post("/confirm-email", authHandler.ConfirmEmail)
		r.Post("/resend-confirmation", authHandler.ResendConfirmation)
		r.Post("/forgot-password", authHandler.ForgotPassword)
		r.Post("/reset-password", authHandler.ResetPassword)
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireAuth(rsaPub, cfg.AuthIssuer, cfg.AuthAudience))
			r.Post("/logout", authHandler.Logout)
			r.Get("/me", authHandler.Me)
			r.Post("/change-password", authHandler.ChangePassword)
		})
	})

	r.Route("/oauth", func(r chi.Router) {
		r.Use(middleware.AuthRateLimit(ctx, 10, 20))
		r.Get("/github", oauthHandler.GitHubLogin)
		r.Get("/github/callback", oauthHandler.GitHubCallback)
		r.Get("/google", oauthHandler.GoogleLogin)
		r.Get("/google/callback", oauthHandler.GoogleCallback)
		r.Get("/x", oauthHandler.XLogin)
		r.Get("/x/callback", oauthHandler.XCallback)
		r.Post("/exchange", oauthHandler.Exchange)
	})
}
