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
	"github.com/usehiveloop/hiveloop/internal/credentials"
	"github.com/usehiveloop/hiveloop/internal/email"
	"github.com/usehiveloop/hiveloop/internal/employeeruntime"
	"github.com/usehiveloop/hiveloop/internal/enqueue"
	"github.com/usehiveloop/hiveloop/internal/goroutine"
	"github.com/usehiveloop/hiveloop/internal/handler"
	"github.com/usehiveloop/hiveloop/internal/hindsight"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	sentryobs "github.com/usehiveloop/hiveloop/internal/observability/sentry"
	"github.com/usehiveloop/hiveloop/internal/proxy"
	"github.com/usehiveloop/hiveloop/internal/spider"
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

	goroutine.Go(ctx, func(ctx context.Context) {
		if err := cacheManager.Invalidator().Subscribe(ctx); err != nil {
			slog.Error("invalidation subscriber stopped", "error", err)
		}
	})

	auditWriter := middleware.NewAuditWriter(ctx, database, 10000)

	generationWriter := middleware.NewGenerationWriter(ctx, database, reg, 10000)

	mcpHandler := handler.NewMCPHandler(database, signingKey, actionsCatalog, nangoClient, ctr)
	var hindsightClient *hindsight.Client
	if cfg.HindsightAPIURL != "" {
		hindsightClient = hindsight.NewClient(cfg.HindsightAPIURL)
		mcpHandler.SetMemoryTools(hindsight.NewMemoryToolsFunc(hindsightClient, hindsightMemoryRefresh(enqueuer)))
	}
	mcpHandler.SetSubscriptionTools(subscriptions.RegisterTools(subscriptions.NewService(database, actionsCatalog)))
	if deps.SpiderClient != nil {
		mcpHandler.SetWebTools(spider.NewWebToolsFunc(deps.SpiderClient))
	}
	credHandler := handler.NewCredentialHandler(database, deps.KMS, cacheManager, ctr)
	tokenHandler := handler.NewTokenHandler(database, signingKey, cacheManager, ctr, actionsCatalog, cfg.MCPBaseURL, mcpHandler.ServerCache)
	providerHandler := handler.NewProviderHandler(reg, database)
	customDomainHandler := handler.NewCustomDomainHandler(database, cfg)
	inIntegrationHandler := handler.NewInIntegrationHandler(database, nangoClient, actionsCatalog)
	inConnectionHandler := handler.NewInConnectionHandler(database, nangoClient, actionsCatalog)
	orgHandler := handler.NewOrgHandler(database)
	plansHandler := handler.NewPlansHandler(database)
	routerHandler := handler.NewRouterHandler(database, actionsCatalog)
	var emailSender email.Sender = &email.LogSender{}
	if enqueuer != nil && cfg.KibamailAPIKey != "" {
		emailSender = email.NewAsynqSender(enqueuer)
	}
	orgInviteHandler := handler.NewOrgInviteHandler(database, emailSender, cfg.FrontendURL)
	teamHandler := handler.NewTeamHandler(database)
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
	proxyHandler := handler.NewProxyHandler(cacheManager, &proxy.CaptureTransport{Inner: sentryobs.WrapTransport(proxy.NewTransport())})

	var conversationHandler *handler.ConversationHandler
	if orchestrator != nil && agentPusher != nil {
		conversationHandler = handler.NewConversationHandler(database, orchestrator, agentPusher, eventBus)
		if enqueuer != nil {
			conversationHandler.SetEnqueuer(enqueuer)
		}
		conversationHandler.SetCredits(deps.Credits)
	}

	bridgeWebhookHandler := handler.NewBridgeWebhookHandler(database, sandboxEncKey, eventBus, enqueuer)
	employeeEventWriter := handler.NewEmployeeEventWriter(ctx, database, 20000)
	employeeOutboundWebhookHandler := handler.NewEmployeeOutboundWebhookHandler(database, sandboxEncKey, enqueuer, employeeEventWriter)
	nangoWebhookHandler := handler.NewNangoWebhookHandler(database, cfg.NangoSecretKey, sandboxEncKey, enqueuer)

	var cloudAgentHandler *handler.CloudAgentHandler
	if orchestrator != nil && agentPusher != nil {
		cloudAgentHandler = handler.NewCloudAgentHandler(database, sandboxEncKey, orchestrator, agentPusher)
	}
	incomingWebhookHandler := handler.NewIncomingWebhookHandler(database, enqueuer)
	httpTriggerHandler := handler.NewHTTPTriggerHandler(database, enqueuer)

	var templateBuilder handler.TemplateBuildable
	if orchestrator != nil {
		templateBuilder = orchestrator
	}
	sandboxTemplateHandler := handler.NewSandboxTemplateHandler(database, templateBuilder, enqueuer)
	skillHandler := handler.NewSkillHandler(database, enqueuer)

	agentHandler := handler.NewAgentHandler(database, reg, sandboxEncKey, enqueuer)
	agentHandler.SetCatalog(actionsCatalog)
	var chatHandler *handler.ChatHandler
	if orchestrator != nil {
		chatHandler = handler.NewChatHandler(database, orchestrator, sandboxEncKey, signingKey)
	}

	var employeeHandler *handler.EmployeeHandler
	if orchestrator != nil {
		employeeHandler = handler.NewEmployeeHandler(database, orchestrator, employeeruntime.CompileDeps{
			DB:         database,
			Picker:     credentials.NewPickerWithRegistry(database, reg),
			KMS:        deps.KMS,
			EncKey:     sandboxEncKey,
			SigningKey: signingKey,
			Cfg:        cfg,
			Hindsight:  hindsightClient,
		}, agentHandler)
	}
	agentProfileHandler := handler.NewAgentProfileHandler(database, deps.KMS, sandboxEncKey, nangoClient)
	agentProfileHandler.SetWebhookBaseURL(cfg.APIWebhookBaseURL)
	agentProfileHandler.SetRAGEnqueuer(enqueuer)
	githubEmployeeWebhookHandler := handler.NewGitHubEmployeeWebhookHandler(database, deps.KMS)
	marketplaceHandler := handler.NewMarketplaceHandler(database, redisClient)

	var driveHandler *handler.DriveHandler
	if deps.S3Client != nil {
		driveHandler = handler.NewDriveHandler(database, deps.S3Client)
	}
	var sqliteBackupHandler *handler.EmployeeSQLiteBackupHandler
	if deps.S3Client != nil && sandboxEncKey != nil {
		sqliteBackupHandler = handler.NewEmployeeSQLiteBackupHandler(
			database,
			deps.S3Client,
			sandboxEncKey,
			cfg.EmployeeSQLiteBackupMaxBytes,
		)
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
			if sandboxEncKey != nil {
				uploadsHandler.WithStreamer(presigner, sandboxEncKey)
			}
			slog.Info("public assets uploads ready", "bucket", cfg.PublicAssetsBucket)
		}
	}

	billingHandler := handler.NewBillingHandler(database, deps.BillingRegistry, deps.Credits)
	subscriptionHandler := handler.NewSubscriptionHandler(database, deps.BillingRegistry, deps.Credits)

	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(sentryobs.Middleware())
	r.Use(sentryobs.Recoverer())
	r.Use(sentryobs.Capture5xxResponses())
	r.Use(middleware.SecurityHeaders())
	r.Use(middleware.CORS(cfg.CORSOrigins))
	r.Use(middleware.RequestLog(logger))

	rsaPub := rsaKey.Public().(*rsa.PublicKey)

	setupPublicRoutes(r, cfg, database, redisClient, providerHandler, inIntegrationHandler, actionsCatalog, marketplaceHandler, orgInviteHandler, plansHandler, bridgeWebhookHandler, employeeOutboundWebhookHandler, nangoWebhookHandler, githubEmployeeWebhookHandler, incomingWebhookHandler, nangoClient, sandboxEncKey, uploadsHandler, sqliteBackupHandler, cloudAgentHandler)

	if chatHandler != nil {
		r.Get("/v1/chats/{id}/stream", chatHandler.Stream)
	}

	r.Post("/incoming/triggers/{triggerID}", httpTriggerHandler.Handle)
	setupAuthRoutes(r, ctx, cfg, rsaPub, authHandler, oauthHandler)
	ragSourceHandler, ragSearchHandler, err := setupRAGRuntime(cfg, database, enqueuer, mcpHandler)
	if err != nil {
		return err
	}
	systemTaskHandler := buildSystemTaskHandler(database, deps, redisClient)
	setupV1Routes(r, cfg, rsaPub, database, apiKeyCache, enqueuer, orgHandler, orgInviteHandler, teamHandler, usageHandler, auditHandler, reportingHandler, generationHandler, apiKeyHandler, billingHandler, subscriptionHandler, credHandler, tokenHandler, sandboxTemplateHandler, skillHandler, agentHandler, agentProfileHandler, marketplaceHandler, conversationHandler, routerHandler, customDomainHandler, ragSourceHandler, ragSearchHandler, uploadsHandler, systemTaskHandler, employeeHandler, chatHandler, orchestrator, auditWriter)

	var platformAdminEmails []string
	if cfg.PlatformAdminEmails != "" {
		platformAdminEmails = strings.Split(cfg.PlatformAdminEmails, ",")
	}
	setupConnectRoutes(r, cfg, rsaPub, database, platformAdminEmails, inIntegrationHandler, inConnectionHandler)
	setupAdminRoutes(r, cfg, deps, rsaPub, database, platformAdminEmails, enqueuer, marketplaceHandler)
	setupProxyAndAuxRoutes(r, cfg, deps, signingKey, database, proxyHandler, driveHandler, sandboxEncKey, auditWriter, generationWriter, ctr)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 0,
		IdleTimeout:  120 * time.Second,
		ErrorLog:     sentryobs.NewStdlogBridge("api_server"),
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
	employeeEventWriter.Shutdown(shutdownCtx)
	if deps.ToolUsageWriter != nil {
		deps.ToolUsageWriter.Shutdown(shutdownCtx)
	}

	slog.Info("serve shutdown complete")
	return nil
}
