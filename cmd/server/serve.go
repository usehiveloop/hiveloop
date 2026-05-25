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

	"github.com/usehivy/hivy/internal/bootstrap"
	"github.com/usehivy/hivy/internal/credentials"
	"github.com/usehivy/hivy/internal/email"
	"github.com/usehivy/hivy/internal/employeeruntime"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/gateway"
	"github.com/usehivy/hivy/internal/goroutine"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/hindsight"
	"github.com/usehivy/hivy/internal/middleware"
	sentryobs "github.com/usehivy/hivy/internal/observability/sentry"
	"github.com/usehivy/hivy/internal/proxy"
	"github.com/usehivy/hivy/internal/specialisttasks"
	"github.com/usehivy/hivy/internal/spider"
	"github.com/usehivy/hivy/internal/storage"
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
	if deps.SpiderClient != nil {
		mcpHandler.SetWebTools(spider.NewWebToolsFunc(deps.SpiderClient))
	}
	runtimeCompileDeps := employeeruntime.CompileDeps{
		DB:          database,
		Picker:      credentials.NewPickerWithRegistry(database, reg),
		KMS:         deps.KMS,
		EncKey:      sandboxEncKey,
		SigningKey:  signingKey,
		Cfg:         cfg,
		Nango:       nangoClient,
		Hindsight:   hindsightClient,
		Specialists: deps.Specialists,
	}
	if orchestrator != nil {
		specialistService := specialisttasks.NewService(database, orchestrator, runtimeCompileDeps, deps.Specialists)
		mcpHandler.SetSpecialistTools(specialisttasks.NewToolsFunc(specialistService))
	}
	credHandler := handler.NewCredentialHandler(database, deps.KMS, cacheManager, ctr)
	tokenHandler := handler.NewTokenHandler(database, signingKey, cacheManager, ctr, actionsCatalog, cfg.MCPBaseURL, mcpHandler.ServerCache)
	providerHandler := handler.NewProviderHandler(reg, database)
	customDomainHandler := handler.NewCustomDomainHandler(database, cfg)
	integrationHandler := handler.NewIntegrationHandler(database, nangoClient, actionsCatalog)
	connectionHandler := handler.NewConnectionHandler(database, nangoClient, actionsCatalog)
	orgHandler := handler.NewOrgHandler(database)
	plansHandler := handler.NewPlansHandler(database)
	var emailSender email.Sender = &email.LogSender{}
	if enqueuer != nil && cfg.KibamailAPIKey != "" {
		emailSender = email.NewAsynqSender(enqueuer)
	}
	orgInviteHandler := handler.NewOrgInviteHandler(database, emailSender, cfg.FrontendURL)
	authHandler := handler.NewAuthHandler(database, rsaKey, signingKey,
		cfg.AuthIssuer, cfg.AuthAudience, cfg.AuthAccessTokenTTL, cfg.AuthRefreshTokenTTL,
		emailSender, cfg.FrontendURL, cfg.AutoConfirmEmail, deps.Credits)
	if cfg.PlatformAdminEmails != "" {
		authHandler.SetPlatformAdminEmails(strings.Split(cfg.PlatformAdminEmails, ","))
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

	employeeEventWriter := handler.NewEmployeeEventWriter(ctx, database, 20000)
	employeeOutboundWebhookHandler := handler.NewEmployeeOutboundWebhookHandler(database, sandboxEncKey, enqueuer, employeeEventWriter)
	var gatewayHTTPHandler *handler.GatewayHTTPHandler
	if orchestrator != nil {
		gatewayRuntime := gateway.NewOrchestratedRuntimeMessenger(database, orchestrator)
		gatewayService := gateway.NewService(database, gatewayRuntime, gateway.NewFakeSlackAdapter(), gateway.NewHTTPAdapter(nil))
		employeeOutboundWebhookHandler.SetGatewayService(gatewayService)
		gatewayHTTPHandler = handler.NewGatewayHTTPHandler(gatewayService)
	}
	nangoWebhookHandler := handler.NewNangoWebhookHandler(database, cfg.NangoSecretKey, sandboxEncKey, enqueuer)

	incomingWebhookHandler := handler.NewIncomingWebhookHandler(database, enqueuer)
	httpTriggerHandler := handler.NewHTTPTriggerHandler(database, enqueuer)

	var templateBuilder handler.TemplateBuildable
	if orchestrator != nil {
		templateBuilder = orchestrator
	}
	sandboxTemplateHandler := handler.NewSandboxTemplateHandler(database, templateBuilder, enqueuer)
	skillHandler := handler.NewSkillHandler(database, enqueuer)

	var employeeHandler *handler.EmployeeHandler
	if orchestrator != nil {
		employeeHandler = handler.NewEmployeeHandler(database, orchestrator, runtimeCompileDeps, reg, deps.Specialists)
		if deps.S3Client != nil {
			employeeHandler.SetEnqueuer(enqueuer)
		}
		orgHandler.SetEmployeeSyncer(employeeHandler)
	}
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
	dashboardHandler := handler.NewDashboardHandler(database, deps.Credits)
	slackChannelHandler := handler.NewSlackChannelHandler(database, nangoClient)

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

	setupPublicRoutes(r, cfg, database, redisClient, providerHandler, integrationHandler, actionsCatalog, orgInviteHandler, plansHandler, employeeOutboundWebhookHandler, nangoWebhookHandler, incomingWebhookHandler, gatewayHTTPHandler, nangoClient, sandboxEncKey, uploadsHandler, sqliteBackupHandler)

	r.Post("/incoming/triggers/{triggerID}", httpTriggerHandler.Handle)
	setupAuthRoutes(r, ctx, cfg, rsaPub, authHandler, oauthHandler)
	ragSourceHandler, ragSearchHandler, err := setupRAGRuntime(cfg, database, enqueuer, mcpHandler)
	if err != nil {
		return err
	}
	systemTaskHandler := buildSystemTaskHandler(database, deps, redisClient)
	setupV1Routes(r, cfg, rsaPub, database, apiKeyCache, enqueuer, orgHandler, orgInviteHandler, usageHandler, auditHandler, reportingHandler, generationHandler, apiKeyHandler, billingHandler, subscriptionHandler, dashboardHandler, slackChannelHandler, credHandler, tokenHandler, sandboxTemplateHandler, skillHandler, customDomainHandler, ragSourceHandler, ragSearchHandler, uploadsHandler, systemTaskHandler, employeeHandler, orchestrator, auditWriter)

	var platformAdminEmails []string
	if cfg.PlatformAdminEmails != "" {
		platformAdminEmails = strings.Split(cfg.PlatformAdminEmails, ",")
	}
	setupConnectRoutes(r, cfg, rsaPub, database, platformAdminEmails, integrationHandler, connectionHandler)
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
