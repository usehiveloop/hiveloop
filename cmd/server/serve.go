package main

import (
	"context"
	"crypto/rsa"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/usehiveloop/hiveloop/internal/bootstrap"
	"github.com/usehiveloop/hiveloop/internal/email"
	"github.com/usehiveloop/hiveloop/internal/enqueue"
	"github.com/usehiveloop/hiveloop/internal/goroutine"
	"github.com/usehiveloop/hiveloop/internal/handler"
	"github.com/usehiveloop/hiveloop/internal/hindsight"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	posthogobs "github.com/usehiveloop/hiveloop/internal/observability/posthog"
	"github.com/usehiveloop/hiveloop/internal/proxy"
	ragscheduler "github.com/usehiveloop/hiveloop/internal/rag/scheduler"
	"github.com/usehiveloop/hiveloop/internal/storage"
	"github.com/usehiveloop/hiveloop/internal/subscriptions"
)

func runServe(ctx context.Context, deps *bootstrap.Deps, enqueuer enqueue.TaskEnqueuer) error {
	cfg := deps.Config
	database := deps.DB
	redisClient := deps.Redis
	cacheManager := deps.CacheManager
	apiKeyCache := deps.APIKeyCache
	ctr := deps.Counter
	signingKey := deps.SigningKey
	rsaKey := deps.RSAKey
	reg := deps.Registry
	nangoClient := deps.NangoClient
	actionsCatalog := deps.ActionsCatalog
	sandboxEncKey := deps.SandboxEncKey
	orchestrator := deps.Orchestrator
	agentPusher := deps.AgentPusher
	eventBus := deps.EventBus

	logger := slog.Default()

	// Start cache invalidation subscriber (per-instance, real-time pub/sub)
	goroutine.Go(ctx, func(ctx context.Context) {
		if err := cacheManager.Invalidator().Subscribe(ctx); err != nil {
			slog.Error("invalidation subscriber stopped", "error", err)
		}
	})

	// Audit writer (buffered, non-blocking)
	auditWriter := middleware.NewAuditWriter(database, 10000)

	// Generation writer (buffered, non-blocking)
	generationWriter := middleware.NewGenerationWriter(database, reg, 10000)

	mcpHandler := handler.NewMCPHandler(database, signingKey, actionsCatalog, nangoClient, ctr)
	if cfg.HindsightAPIURL != "" {
		mcpHandler.SetMemoryTools(hindsight.NewMemoryToolsFunc(hindsight.NewClient(cfg.HindsightAPIURL)))
	}
	mcpHandler.SetSubscriptionTools(subscriptions.RegisterTools(subscriptions.NewService(database, actionsCatalog)))
	credHandler := handler.NewCredentialHandler(database, deps.KMS, cacheManager, ctr)
	tokenHandler := handler.NewTokenHandler(database, signingKey, cacheManager, ctr, actionsCatalog, cfg.MCPBaseURL, mcpHandler.ServerCache)
	providerHandler := handler.NewProviderHandler(reg)
	customDomainHandler := handler.NewCustomDomainHandler(database, cfg)
	inIntegrationHandler := handler.NewInIntegrationHandler(database, nangoClient, actionsCatalog)
	inConnectionHandler := handler.NewInConnectionHandler(database, nangoClient, actionsCatalog)
	orgHandler := handler.NewOrgHandler(database)
	plansHandler := handler.NewPlansHandler(database)
	routerHandler := handler.NewRouterHandler(database, actionsCatalog)
	var emailSender email.Sender = &email.LogSender{}
	if enqueuer != nil {
		emailSender = email.NewAsynqSender(enqueuer)
	}
	orgInviteHandler := handler.NewOrgInviteHandler(database, emailSender, cfg.FrontendURL)
	authHandler := handler.NewAuthHandler(database, rsaKey, signingKey,
		cfg.AuthIssuer, cfg.AuthAudience, cfg.AuthAccessTokenTTL, cfg.AuthRefreshTokenTTL,
		emailSender, cfg.FrontendURL, cfg.AutoConfirmEmail, deps.Credits)
	if cfg.PlatformAdminEmails != "" {
		authHandler.SetPlatformAdminEmails(strings.Split(cfg.PlatformAdminEmails, ","))
	}
	if cfg.AdminAPIEnabled && cfg.PlatformAdminEmails != "" {
		authHandler.SetAdminMode(strings.Split(cfg.PlatformAdminEmails, ","))
	}
	authHandler.StartCleanup(ctx)
	oauthHandler := handler.NewOAuthHandler(database, rsaKey, signingKey,
		cfg.AuthIssuer, cfg.AuthAudience, cfg.AuthAccessTokenTTL, cfg.AuthRefreshTokenTTL,
		cfg.FrontendURL,
		cfg.OAuthGitHubClientID, cfg.OAuthGitHubClientSecret,
		cfg.OAuthGoogleClientID, cfg.OAuthGoogleClientSecret,
		cfg.OAuthXClientID, cfg.OAuthXClientSecret,
		deps.Credits)
	apiKeyHandler := handler.NewAPIKeyHandler(database, apiKeyCache, cacheManager)
	usageHandler := handler.NewUsageHandler(database)
	auditHandler := handler.NewAuditHandler(database)
	generationHandler := handler.NewGenerationHandler(database)
	reportingHandler := handler.NewReportingHandler(database)
	proxyHandler := handler.NewProxyHandler(cacheManager, &proxy.CaptureTransport{Inner: proxy.NewTransport()})

	var conversationHandler *handler.ConversationHandler
	if orchestrator != nil && agentPusher != nil {
		conversationHandler = handler.NewConversationHandler(database, orchestrator, agentPusher, eventBus)
		if enqueuer != nil {
			conversationHandler.SetEnqueuer(enqueuer)
		}
		conversationHandler.SetCredits(deps.Credits)
	}

	bridgeWebhookHandler := handler.NewBridgeWebhookHandler(database, sandboxEncKey, eventBus, enqueuer)
	nangoWebhookHandler := handler.NewNangoWebhookHandler(database, cfg.NangoSecretKey, sandboxEncKey, enqueuer)
	incomingWebhookHandler := handler.NewIncomingWebhookHandler(database, enqueuer)
	httpTriggerHandler := handler.NewHTTPTriggerHandler(database, enqueuer)

	var templateBuilder handler.TemplateBuildable
	if orchestrator != nil {
		templateBuilder = orchestrator
	}
	sandboxTemplateHandler := handler.NewSandboxTemplateHandler(database, templateBuilder, enqueuer)
	skillHandler := handler.NewSkillHandler(database, enqueuer)
	subagentHandler := handler.NewSubagentHandler(database)

	agentHandler := handler.NewAgentHandler(database, reg, sandboxEncKey, enqueuer)
	agentHandler.SetCatalog(actionsCatalog)
	marketplaceHandler := handler.NewMarketplaceHandler(database, redisClient)

	var driveHandler *handler.DriveHandler
	if deps.S3Client != nil {
		driveHandler = handler.NewDriveHandler(database, deps.S3Client)
	}

	var uploadsHandler *handler.UploadsHandler
	if cfg.PublicAssetsBucket != "" {
		presigner, err := storage.NewS3Presigner(storage.PublicAssetsConfig{
			Bucket:       cfg.PublicAssetsBucket,
			Region:       cfg.PublicAssetsRegion,
			Endpoint:     cfg.PublicAssetsEndpoint,
			AccessKey:    cfg.PublicAssetsAccessKey,
			SecretKey:    cfg.PublicAssetsSecretKey,
			PublicBase:   cfg.PublicAssetsBaseURL,
			SignTTL:      cfg.PublicAssetsSignTTL,
			UsePublicACL: cfg.PublicAssetsUseACL,
		})
		if err != nil {
			slog.Error("public assets presigner init failed; /v1/uploads/sign disabled", "error", err)
		} else {
			uploadsHandler = handler.NewUploadsHandler(database, presigner)
			slog.Info("public assets uploads ready", "bucket", cfg.PublicAssetsBucket)
		}
	}

	billingHandler := handler.NewBillingHandler(database, deps.BillingRegistry, deps.Credits)
	billingWebhookHandler := handler.NewBillingWebhookHandler(database, deps.BillingRegistry, deps.Credits)

	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(posthogobs.Recoverer(deps.PostHog))
	r.Use(middleware.SecurityHeaders())
	r.Use(middleware.CORS(cfg.CORSOrigins))
	r.Use(middleware.RequestLog(logger))

	rsaPub := rsaKey.Public().(*rsa.PublicKey)

	setupPublicRoutes(r, cfg, database, redisClient, providerHandler, inIntegrationHandler, actionsCatalog, marketplaceHandler, orgInviteHandler, plansHandler, bridgeWebhookHandler, nangoWebhookHandler, incomingWebhookHandler, billingWebhookHandler, nangoClient, sandboxEncKey)

	// HTTP triggers: unauthenticated endpoint, trigger UUID acts as bearer token.
	r.Post("/incoming/triggers/{triggerID}", httpTriggerHandler.Handle)
	setupAuthRoutes(r, ctx, cfg, rsaPub, authHandler, oauthHandler)
	ragSourceHandler := handler.NewRAGSourceHandler(database, enqueuer, ragscheduler.HasPermSyncCapability)
	setupV1Routes(r, cfg, rsaPub, database, apiKeyCache, enqueuer, orgHandler, orgInviteHandler, usageHandler, auditHandler, reportingHandler, generationHandler, apiKeyHandler, billingHandler, credHandler, tokenHandler, sandboxTemplateHandler, skillHandler, subagentHandler, agentHandler, marketplaceHandler, conversationHandler, routerHandler, customDomainHandler, ragSourceHandler, uploadsHandler, orchestrator, auditWriter)

	var platformAdminEmails []string
	if cfg.PlatformAdminEmails != "" {
		platformAdminEmails = strings.Split(cfg.PlatformAdminEmails, ",")
	}
	setupConnectRoutes(r, cfg, rsaPub, database, platformAdminEmails, inIntegrationHandler, inConnectionHandler)
	setupAdminRoutes(r, cfg, deps, rsaPub, database, platformAdminEmails, enqueuer, marketplaceHandler)
	setupProxyAndAuxRoutes(r, cfg, deps, signingKey, database, proxyHandler, driveHandler, sandboxEncKey, auditWriter, generationWriter, ctr, enqueuer)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 0,
		IdleTimeout:  120 * time.Second,
		ErrorLog:     posthogobs.NewStdlogBridge("api_server"),
	}

	goroutine.Go(ctx, func(context.Context) {
		slog.Info("server starting", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
		}
	})

	mcpSrv := setupMCPServer(ctx, cfg, deps, signingKey, database, mcpHandler)

	<-ctx.Done()
	slog.Info("shutting down")

	// Shutdown intentionally decouples from the (already-cancelled) parent ctx
	// but inherits its values so observability tags propagate. context.WithoutCancel
	// strips cancellation while preserving values; the WithTimeout below bounds
	// how long shutdown can take.
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("server shutdown error", "error", err)
	}
	if err := mcpSrv.Shutdown(shutdownCtx); err != nil {
		slog.Error("mcp server shutdown error", "error", err)
	}

	auditWriter.Shutdown(shutdownCtx)
	generationWriter.Shutdown(shutdownCtx)
	if deps.ToolUsageWriter != nil {
		deps.ToolUsageWriter.Shutdown(shutdownCtx)
	}

	slog.Info("serve shutdown complete")
	return nil
}
