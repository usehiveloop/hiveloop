package main

import (
	"context"
	"crypto/rsa"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/mcp/catalog"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/nango"
)

func setupPublicRoutes(
	r chi.Router,
	cfg *config.Config,
	database *gorm.DB,
	redisClient *redis.Client,
	providerHandler *handler.ProviderHandler,
	inIntegrationHandler *handler.InIntegrationHandler,
	actionsCatalog *catalog.Catalog,
	orgInviteHandler *handler.OrgInviteHandler,
	plansHandler *handler.PlansHandler,
	bridgeWebhookHandler *handler.BridgeWebhookHandler,
	employeeOutboundWebhookHandler *handler.EmployeeOutboundWebhookHandler,
	nangoWebhookHandler *handler.NangoWebhookHandler,
	incomingWebhookHandler *handler.IncomingWebhookHandler,
	nangoClient *nango.Client,
	sandboxEncKey *crypto.SymmetricKey,
	uploadsHandler *handler.UploadsHandler,
	sqliteBackupHandler *handler.EmployeeSQLiteBackupHandler,
	specialistTaskHandler *handler.SpecialistTaskHandler,
) {
	r.Get("/healthz", healthz)
	r.Get("/readyz", readyz(database, redisClient))

	// Provider discovery (no auth)
	r.Get("/v1/providers", providerHandler.List)
	r.Get("/v1/providers/{id}", providerHandler.Get)
	r.Get("/v1/providers/{id}/models", providerHandler.Models)
	r.Get("/v1/models", providerHandler.AllModels)

	// In-integration discovery (no auth)
	r.Get("/v1/in/integrations/available", inIntegrationHandler.ListAvailable)

	// Integration catalog discovery (no auth)
	actionsHandler := handler.NewActionsHandler(actionsCatalog)
	r.Get("/v1/catalog/integrations", actionsHandler.ListIntegrations)
	r.Get("/v1/catalog/integrations/{id}", actionsHandler.GetIntegration)
	r.Get("/v1/catalog/integrations/{id}/actions", actionsHandler.ListActions)
	r.Get("/v1/catalog/integrations/{id}/triggers", actionsHandler.ListTriggers)
	r.Get("/v1/catalog/integrations/{id}/schema-paths", actionsHandler.GetSchemaPaths)

	// Org invite preview (public, token-based lookup)
	r.Get("/v1/invites/{token}", orgInviteHandler.Preview)

	// Billing plans catalog (no auth)
	r.Get("/v1/plans", plansHandler.List)

	// Webhook receivers (HMAC-verified, no auth middleware)
	r.Post("/internal/webhooks/bridge/{sandboxID}", bridgeWebhookHandler.Handle)
	r.Post("/internal/webhooks/employee/{sandboxID}", employeeOutboundWebhookHandler.Handle)
	r.Post("/internal/webhooks/employee/{sandboxID}/batch", employeeOutboundWebhookHandler.HandleBatch)
	r.Post("/internal/webhooks/nango", nangoWebhookHandler.Handle)

	// Sandbox proxy endpoints (bearer-token auth, no middleware)
	if nangoClient != nil && sandboxEncKey != nil {
		gitCredsHandler := handler.NewGitCredentialsHandler(database, sandboxEncKey, nangoClient)
		r.Post("/internal/git-credentials/{employeeID}", gitCredsHandler.Handle)

		railwayProxyHandler := handler.NewRailwayProxyHandler(database, sandboxEncKey, nangoClient)
		r.Post("/internal/railway-proxy/{employeeID}", railwayProxyHandler.Handle)

		bugsinkProxyHandler := handler.NewBugsinkProxyHandler(database, sandboxEncKey, nangoClient)
		r.Handle("/internal/bugsink-proxy/{employeeID}/*", http.HandlerFunc(bugsinkProxyHandler.Handle))

		linearProxyHandler := handler.NewLinearProxyHandler(database, sandboxEncKey, nangoClient)
		r.Post("/internal/linear-proxy/{employeeID}", linearProxyHandler.Handle)

		notionProxyHandler := handler.NewNotionProxyHandler(database, sandboxEncKey, nangoClient)
		r.Handle("/internal/notion-proxy/{employeeID}/*", http.HandlerFunc(notionProxyHandler.Handle))
	}

	// Direct incoming webhooks for providers requiring manual webhook configuration
	r.Post("/incoming/webhooks/{provider}/{connectionID}", incomingWebhookHandler.Handle)

	// Conversation-scoped streaming asset uploads from inside the sandbox.
	// Bearer auth = the sandbox's bridge API key (matches existing
	// sandbox-drive / git-credentials / railway-proxy endpoints).
	if uploadsHandler != nil {
		r.Put("/internal/conversations/{conversationID}/assets/*", uploadsHandler.StreamConversationAsset)
		r.Post("/internal/conversations/{conversationID}/assets/move", uploadsHandler.MoveConversationAsset)
		r.Delete("/internal/conversations/{conversationID}/assets/*", uploadsHandler.DeleteConversationAsset)

		r.Put("/internal/employees/{employeeID}/assets/*", uploadsHandler.StreamEmployeeAsset)
		r.Post("/internal/employees/{employeeID}/assets/move", uploadsHandler.MoveEmployeeAsset)
		r.Delete("/internal/employees/{employeeID}/assets/*", uploadsHandler.DeleteEmployeeAsset)
	}

	if sqliteBackupHandler != nil {
		r.Put("/internal/employees/{employeeID}/sqlite-backup", sqliteBackupHandler.Upload)
		r.Post("/internal/employees/{employeeID}/sqlite-backup/presign", sqliteBackupHandler.Presign)
		r.Post("/internal/employees/{employeeID}/sqlite-backup/confirm", sqliteBackupHandler.Confirm)
	}

	if specialistTaskHandler != nil {
		r.Get("/internal/employees/{employeeID}/specialists/", specialistTaskHandler.ListSpecialistRuntimes)
		r.Get("/internal/employees/{employeeID}/specialists/{specialistSlug}/tasks", specialistTaskHandler.ListTasks)
		r.Get("/internal/employees/{employeeID}/specialists/{specialistSlug}/tasks/{taskID}", specialistTaskHandler.GetTask)
		r.Post("/internal/employees/{employeeID}/specialists/{specialistSlug}/tasks", specialistTaskHandler.CreateTask)
		r.Post("/internal/employees/{employeeID}/specialists/{specialistSlug}/tasks/{taskID}/message", specialistTaskHandler.SendTaskMessage)
		r.Post("/internal/employees/{employeeID}/specialists/{specialistSlug}/tasks/{taskID}", specialistTaskHandler.TerminateTask)
	}
}

func setupAuthRoutes(
	r chi.Router,
	ctx context.Context,
	cfg *config.Config,
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
