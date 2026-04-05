package main

import (
	"context"
	"crypto/rsa"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/awnumar/memguard"
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/auth"
	"github.com/ziraloop/ziraloop/internal/cache"
	"github.com/google/uuid"
	"github.com/ziraloop/ziraloop/internal/goroutine"
	"github.com/ziraloop/ziraloop/internal/hindsight"
	"github.com/ziraloop/ziraloop/internal/config"
	"github.com/ziraloop/ziraloop/internal/counter"
	"github.com/ziraloop/ziraloop/internal/crypto"
	"github.com/ziraloop/ziraloop/internal/db"
	"github.com/ziraloop/ziraloop/internal/handler"
	"github.com/ziraloop/ziraloop/internal/logging"
	"github.com/ziraloop/ziraloop/internal/mcp/catalog"
	"github.com/ziraloop/ziraloop/internal/middleware"
	"github.com/ziraloop/ziraloop/internal/email"
	"github.com/ziraloop/ziraloop/internal/model"
	"github.com/ziraloop/ziraloop/internal/nango"
	"github.com/ziraloop/ziraloop/internal/proxy"
	"github.com/ziraloop/ziraloop/internal/registry"
	"github.com/ziraloop/ziraloop/internal/forge"
	"github.com/ziraloop/ziraloop/internal/sandbox"
	"github.com/ziraloop/ziraloop/internal/sandbox/daytona"
	"github.com/ziraloop/ziraloop/internal/streaming"
	systemagents "github.com/ziraloop/ziraloop/internal/system-agents"
	"github.com/ziraloop/ziraloop/internal/turso"
)

// @title ZiraLoop API
// @version 1.0
// @description Proxy bridge for LLM API credentials.
// @host api.dev.ziraloop.com
// @BasePath /
// @schemes https
//
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Bearer token (JWT or API key).

// Set via -ldflags at build time.
var (
	version = "dev"
	commit  = "unknown"
)

func main() {
	// Secure memory: catch interrupts for memguard cleanup, disable core dumps.
	memguard.CatchInterrupt()
	disableCoreDumps()

	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run() error {
	// 1. Config
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// 2. Logging — must be initialized before anything else logs
	logging.Init(cfg.LogLevel, cfg.LogFormat)
	logger := slog.Default()
	slog.Info("starting ziraloop", "version", version, "commit", commit)

	// 3. Database
	database, err := db.New(cfg.DatabaseDSN())
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	if err := model.AutoMigrate(database); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}
	slog.Info("database ready")

	// Seed system agents (non-blocking)
	goroutine.Go(func() {
		if err := systemagents.Seed(database); err != nil {
			slog.Error("failed to seed system agents", "error", err)
		} else {
			slog.Info("system agents seeded")
		}
	})

	// 4. KMS wrapper (envelope encryption for DEKs)
	var kms *crypto.KeyWrapper
	switch cfg.KMSType {
	case "aead":
		kms, err = crypto.NewAEADWrapper(cfg.KMSKey, "aead-local")
		if err != nil {
			return fmt.Errorf("creating AEAD KMS wrapper: %w", err)
		}
	case "awskms":
		kms, err = crypto.NewAWSKMSWrapper(cfg.KMSKey, cfg.AWSRegion)
		if err != nil {
			return fmt.Errorf("creating AWS KMS wrapper: %w", err)
		}
	case "vault":
		vaultCfg := cfg.VaultConfig()
		if vaultCfg == nil {
			return fmt.Errorf("vault configuration is nil")
		}
		kms, err = crypto.NewVaultTransitWrapper(*vaultCfg)
		if err != nil {
			return fmt.Errorf("creating Vault Transit wrapper: %w", err)
		}
	default:
		return fmt.Errorf("unsupported KMS_TYPE: %q (supported: aead, awskms, vault)", cfg.KMSType)
	}
	slog.Info("kms wrapper ready", "type", cfg.KMSType)

	// 5. Redis
	var redisOpts *redis.Options
	if cfg.RedisURL != "" {
		var parseErr error
		redisOpts, parseErr = redis.ParseURL(cfg.RedisURL)
		if parseErr != nil {
			return fmt.Errorf("parsing REDIS_URL: %w", parseErr)
		}
	} else {
		redisOpts = &redis.Options{
			Addr:     cfg.RedisAddr,
			Password: cfg.RedisPassword,
			DB:       cfg.RedisDB,
		}
	}
	redisClient := redis.NewClient(redisOpts)
	if err := redisClient.Ping(context.Background()).Err(); err != nil {
		return fmt.Errorf("connecting to redis: %w", err)
	}
	defer redisClient.Close()
	slog.Info("redis ready")

	// 6. Cache manager (L1 memory → L2 Redis → L3 Postgres+KMS)
	apiKeyCache := cache.NewAPIKeyCache(5000, 5*time.Minute)
	cacheCfg := cache.Config{
		MemMaxSize: cfg.MemCacheMaxSize,
		MemTTL:     cfg.MemCacheTTL,
		RedisTTL:   cfg.RedisCacheTTL,
		DEKMaxSize: 1000,
		DEKTTL:     30 * time.Minute,
		HardExpiry: 15 * time.Minute,
	}
	cacheManager := cache.Build(cacheCfg, redisClient, kms, database, apiKeyCache)
	slog.Info("cache manager ready")

	// 7. Start cache invalidation subscriber (cross-instance pub/sub)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	goroutine.Go(func() {
		if err := cacheManager.Invalidator().Subscribe(ctx); err != nil {
			slog.Error("invalidation subscriber stopped", "error", err)
		}
	})

	// 8. Request-cap counter (Redis + Postgres lazy refill)
	ctr := counter.New(redisClient, database)
	slog.Info("request counter ready")

	// 9. Audit writer (buffered, non-blocking)
	auditWriter := middleware.NewAuditWriter(database, 10000)

	// 10. Signing key
	signingKey := []byte(cfg.JWTSigningKey)

	// 10b. Embedded auth (RSA key for JWT signing — required)
	rsaKey, err := auth.LoadRSAPrivateKey(cfg.AuthRSAPrivateKey)
	if err != nil {
		return fmt.Errorf("loading auth RSA key: %w", err)
	}
	slog.Info("embedded auth ready") // phase 6 sandbox orchestrator

	// 11. Provider registry (embedded at build time)
	reg := registry.Global()
	slog.Info("provider registry ready", "providers", reg.ProviderCount(), "models", reg.ModelCount())

	// 11b. Generation writer (buffered, non-blocking — observability for proxy requests)
	generationWriter := middleware.NewGenerationWriter(database, reg, 10000)

	// 12. Nango client (REQUIRED — OAuth integration proxy)
	if cfg.NangoEndpoint == "" || cfg.NangoSecretKey == "" {
		return fmt.Errorf("NANGO_ENDPOINT and NANGO_SECRET_KEY are required")
	}
	nangoClient := nango.NewClient(cfg.NangoEndpoint, cfg.NangoSecretKey)
	if err := nangoClient.FetchProviders(context.Background()); err != nil {
		return fmt.Errorf("fetching Nango provider catalog: %w", err)
	}
	slog.Info("nango client ready", "providers", len(nangoClient.GetProviders()))

	// 12c. Actions catalog (embedded at build time)
	actionsCatalog := catalog.Global()
	slog.Info("actions catalog ready", "providers", len(actionsCatalog.ListProviders()))

	// 13. Encryption key for sandbox secrets (env vars, bridge API keys)
	var sandboxEncKey *crypto.SymmetricKey
	if cfg.SandboxEncryptionKey != "" {
		var err error
		sandboxEncKey, err = crypto.NewSymmetricKey(cfg.SandboxEncryptionKey)
		if err != nil {
			return fmt.Errorf("invalid SANDBOX_ENCRYPTION_KEY: %w", err)
		}
	}

	// 13a. Handlers
	mcpHandler := handler.NewMCPHandler(database, signingKey, actionsCatalog, nangoClient, ctr)
	credHandler := handler.NewCredentialHandler(database, kms, cacheManager, ctr)
	tokenHandler := handler.NewTokenHandler(database, signingKey, cacheManager, ctr, actionsCatalog, cfg.MCPBaseURL, mcpHandler.ServerCache)
	identityHandler := handler.NewIdentityHandler(database, sandboxEncKey)
	providerHandler := handler.NewProviderHandler(reg)
	connectSessionHandler := handler.NewConnectSessionHandler(database)
	connectAPIHandler := handler.NewConnectAPIHandler(database, kms, reg, nangoClient, actionsCatalog)
	settingsHandler := handler.NewSettingsHandler(database, sandboxEncKey)
	customDomainHandler := handler.NewCustomDomainHandler(database, cfg)
	integrationHandler := handler.NewIntegrationHandler(database, nangoClient, actionsCatalog)
	connectionHandler := handler.NewConnectionHandler(database, nangoClient, actionsCatalog)
	inIntegrationHandler := handler.NewInIntegrationHandler(database, nangoClient, actionsCatalog)
	inConnectionHandler := handler.NewInConnectionHandler(database, nangoClient, actionsCatalog)
	orgHandler := handler.NewOrgHandler(database)
	emailSender := &email.LogSender{}
	authHandler := handler.NewAuthHandler(database, rsaKey, signingKey,
		cfg.AuthIssuer, cfg.AuthAudience, cfg.AuthAccessTokenTTL, cfg.AuthRefreshTokenTTL,
		emailSender, cfg.FrontendURL, cfg.AutoConfirmEmail)
	if cfg.AdminAPIEnabled && cfg.PlatformAdminEmails != "" {
		authHandler.SetAdminMode(strings.Split(cfg.PlatformAdminEmails, ","))
	}
	authHandler.StartCleanup(ctx)
	oauthHandler := handler.NewOAuthHandler(database, rsaKey, signingKey,
		cfg.AuthIssuer, cfg.AuthAudience, cfg.AuthAccessTokenTTL, cfg.AuthRefreshTokenTTL,
		cfg.FrontendURL,
		cfg.OAuthGitHubClientID, cfg.OAuthGitHubClientSecret,
		cfg.OAuthGoogleClientID, cfg.OAuthGoogleClientSecret,
		cfg.OAuthXClientID, cfg.OAuthXClientSecret)
	apiKeyHandler := handler.NewAPIKeyHandler(database, apiKeyCache, cacheManager)
	usageHandler := handler.NewUsageHandler(database)
	auditHandler := handler.NewAuditHandler(database)
	generationHandler := handler.NewGenerationHandler(database)
	reportingHandler := handler.NewReportingHandler(database)
	proxyHandler := handler.NewProxyHandler(cacheManager, &proxy.CaptureTransport{Inner: proxy.NewTransport()})
	// 13b. Sandbox orchestrator (optional — only if sandbox provider is configured)
	var orchestrator *sandbox.Orchestrator
	var agentPusher *sandbox.Pusher
	slog.Info("sandbox config check", "provider_key_set", cfg.SandboxProviderKey != "", "encryption_key_set", cfg.SandboxEncryptionKey != "")
	if cfg.SandboxProviderKey != "" && sandboxEncKey != nil {
		sandboxProvider, err := daytona.NewDriver(daytona.Config{
			APIURL: cfg.SandboxProviderURL,
			APIKey: cfg.SandboxProviderKey,
			Target: cfg.SandboxTarget,
		})
		if err != nil {
			return fmt.Errorf("creating sandbox provider: %w", err)
		}
		var tursoProvisioner *turso.Provisioner
		if cfg.TursoAPIToken != "" && cfg.TursoOrgSlug != "" {
			tursoClient := turso.NewClient(cfg.TursoAPIToken, cfg.TursoOrgSlug)
			tursoProvisioner = turso.NewProvisioner(tursoClient, cfg.TursoGroup, database)
			slog.Info("turso provisioner ready")
		} else {
			slog.Info("turso not configured, sandboxes will run without libsql storage")
		}
		orchestrator = sandbox.NewOrchestrator(database, sandboxProvider, tursoProvisioner, sandboxEncKey, cfg)

		// Hindsight MCP URL closure (if configured — agents get memory tools via ZiraLoop MCP server)
		var hindsightMCPURL func(uuid.UUID) string
		if cfg.HindsightAPIURL != "" {
			mcpBase := cfg.MCPBaseURL
			hindsightMCPURL = func(agentID uuid.UUID) string {
				return mcpBase + "/memory/" + agentID.String()
			}
		}

		agentPusher = sandbox.NewPusher(database, orchestrator, signingKey, cfg, hindsightMCPURL)
		goroutine.Go(func() { orchestrator.StartHealthChecker(ctx) })
		goroutine.Go(func() { orchestrator.StartResourceChecker(ctx) })
		slog.Info("sandbox orchestrator ready")
	}
	// Event streaming via Redis Streams
	eventBus := streaming.NewEventBus(redisClient)
	flusher := streaming.NewFlusher(eventBus, database)
	goroutine.Go(func() { flusher.Run(ctx) })
	cleanup := streaming.NewCleanup(eventBus)
	goroutine.Go(func() { cleanup.Run(ctx) })

	// Hindsight memory retainer (Redis Stream consumer — started after eventBus)
	if cfg.HindsightAPIURL != "" {
		hClient := hindsight.NewClient(cfg.HindsightAPIURL)
		retainer := hindsight.NewRetainer(eventBus, database, hClient)
		goroutine.Go(func() { retainer.Run(ctx) })
		slog.Info("hindsight memory retainer started", "url", cfg.HindsightAPIURL)
	}

	var conversationHandler *handler.ConversationHandler
	var forgeHandler *handler.ForgeHandler
	var systemConvHandler *handler.SystemConversationHandler
	forgeMCPHandler := forge.NewForgeMCPHandler(database)
	if orchestrator != nil && agentPusher != nil {
		conversationHandler = handler.NewConversationHandler(database, orchestrator, agentPusher, eventBus)
		systemConvHandler = handler.NewSystemConversationHandler(database, orchestrator, agentPusher, eventBus, signingKey, cfg)
		forgeCtrl := forge.NewForgeController(database, orchestrator, agentPusher, signingKey, cfg, eventBus, catalog.Global())
		forgeHandler = handler.NewForgeHandler(database, forgeCtrl, eventBus)
		goroutine.Go(func() { forgeCtrl.ResumeStaleRuns(ctx) })
		slog.Info("forge controller ready")
	}

	bridgeWebhookHandler := handler.NewBridgeWebhookHandler(database, sandboxEncKey, eventBus)
	nangoWebhookHandler := handler.NewNangoWebhookHandler(database, cfg.NangoSecretKey, sandboxEncKey)

	var templateBuilder handler.TemplateBuildable
	if orchestrator != nil {
		templateBuilder = orchestrator
	}
	sandboxTemplateHandler := handler.NewSandboxTemplateHandler(database, templateBuilder)

	var pusherForHandler handler.AgentPusher
	if agentPusher != nil {
		pusherForHandler = agentPusher
	}
	agentHandler := handler.NewAgentHandler(database, reg, pusherForHandler, sandboxEncKey)

	// 14. Router
	r := chi.NewRouter()

	// Global middleware
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)
	r.Use(middleware.SecurityHeaders())
	r.Use(middleware.CORS(cfg.CORSOrigins))
	r.Use(middleware.RequestLog(logger))

	// Health checks (no auth)
	r.Get("/healthz", healthz)
	r.Get("/readyz", readyz(database, redisClient))

	// Provider discovery (no auth — used by frontend)
	r.Get("/v1/providers", providerHandler.List)
	r.Get("/v1/providers/{id}", providerHandler.Get)
	r.Get("/v1/providers/{id}/models", providerHandler.Models)

	// In-integration discovery (no auth — used by frontend)
	r.Get("/v1/in/integrations/available", inIntegrationHandler.ListAvailable)

	// Integration catalog discovery (no auth — MCP actions/resources catalog)
	actionsHandler := handler.NewActionsHandler(actionsCatalog)
	r.Get("/v1/catalog/integrations", actionsHandler.ListIntegrations)
	r.Get("/v1/catalog/integrations/{id}", actionsHandler.GetIntegration)
	r.Get("/v1/catalog/integrations/{id}/actions", actionsHandler.ListActions)

	// Bridge webhook receiver (no auth middleware — uses HMAC signature verification)
	r.Post("/internal/webhooks/bridge/{sandboxID}", bridgeWebhookHandler.Handle)

	// Nango webhook receiver (no auth middleware — uses HMAC signature verification)
	r.Post("/internal/webhooks/nango", nangoWebhookHandler.Handle)

	// Embedded auth
	rsaPub := rsaKey.Public().(*rsa.PublicKey)

	// Auth routes (register, login, refresh, logout, me, email confirmation, password reset)
	r.Route("/auth", func(r chi.Router) {
		r.Use(middleware.AuthRateLimit(ctx, 10, 20)) // 10 rps per IP, burst 20
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

	// OAuth social login (GitHub, Google, X)
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

	// Org-authenticated routes (JWT or API Key) — credential & token management
	r.Route("/v1", func(r chi.Router) {
		r.Use(middleware.MultiAuth(rsaPub, cfg.AuthIssuer, cfg.AuthAudience, database, apiKeyCache))
		r.Use(middleware.RequireEmailConfirmed(database))

		// Org management (JWT-only, no org context needed for creation)
		r.Post("/orgs", orgHandler.Create)

		// Org-scoped routes (require resolved org context)
		r.Group(func(r chi.Router) {
			r.Use(middleware.ResolveOrgFlexible(database))
			r.Use(middleware.RateLimit())
			r.Use(middleware.Audit(auditWriter))

			r.Get("/orgs/current", orgHandler.Current)
			r.Get("/usage", usageHandler.Get)
			r.Get("/audit", auditHandler.List)
			r.Get("/reporting", reportingHandler.Get)
			r.Get("/generations", generationHandler.List)
			r.Get("/generations/{id}", generationHandler.Get)

			// API key CRUD (any auth, no scope required)
			r.Post("/api-keys", apiKeyHandler.Create)
			r.Get("/api-keys", apiKeyHandler.List)
			r.Delete("/api-keys/{id}", apiKeyHandler.Revoke)

			// Credential operations — scope: "credentials"
			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireAPIKeyScopeOrJWT("credentials"))
				r.Post("/credentials", credHandler.Create)
				r.Get("/credentials", credHandler.List)
				r.Get("/credentials/{id}", credHandler.Get)
				r.Delete("/credentials/{id}", credHandler.Revoke)
			})

			// Token operations — scope: "tokens"
			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireAPIKeyScopeOrJWT("tokens"))
				r.Get("/tokens", tokenHandler.List)
				r.Post("/tokens", tokenHandler.Mint)
				r.Delete("/tokens/{jti}", tokenHandler.Revoke)
			})

			// Identity operations — scope: "all"
			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireAPIKeyScopeOrJWT("all"))
				r.Post("/identities", identityHandler.Create)
				r.Get("/identities", identityHandler.List)
				r.Get("/identities/{id}", identityHandler.Get)
				r.Put("/identities/{id}", identityHandler.Update)
				r.Delete("/identities/{id}", identityHandler.Delete)
			})

			// Connect operations — scope: "connect"
			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireAPIKeyScopeOrJWT("connect"))
				r.Post("/connect/sessions", connectSessionHandler.Create)
				r.Get("/connect/sessions", connectSessionHandler.List)
				r.Get("/connect/sessions/{id}", connectSessionHandler.Get)
				r.Delete("/connect/sessions/{id}", connectSessionHandler.Delete)
			})

			// Integration operations — scope: "integrations"
			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireAPIKeyScopeOrJWT("integrations"))
				r.Get("/integrations/providers", integrationHandler.ListProviders)
				r.Post("/integrations", integrationHandler.Create)
				r.Get("/integrations", integrationHandler.List)
				r.Get("/integrations/{id}", integrationHandler.Get)
				r.Put("/integrations/{id}", integrationHandler.Update)
				r.Delete("/integrations/{id}", integrationHandler.Delete)
				r.Post("/integrations/{id}/connections", connectionHandler.Create)
				r.Get("/integrations/{id}/connections", connectionHandler.List)
			})

			// Connection operations — scope: "integrations"
			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireAPIKeyScopeOrJWT("integrations"))
				r.Get("/connections/available-scopes", connectionHandler.AvailableScopes)
				r.Get("/connections/{id}", connectionHandler.Get)
				r.HandleFunc("/connections/{id}/proxy/*", connectionHandler.Proxy)
				r.Delete("/connections/{id}", connectionHandler.Revoke)
			})

			// Agent operations — scope: "agents"
			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireAPIKeyScopeOrJWT("agents"))
				r.Route("/sandbox-templates", func(r chi.Router) {
					r.Post("/", sandboxTemplateHandler.Create)
					r.Get("/", sandboxTemplateHandler.List)
					r.Get("/{id}", sandboxTemplateHandler.Get)
					r.Put("/{id}", sandboxTemplateHandler.Update)
					r.Delete("/{id}", sandboxTemplateHandler.Delete)
				})
				r.Route("/agents", func(r chi.Router) {
					r.Post("/", agentHandler.Create)
					r.Get("/", agentHandler.List)
					r.Get("/{id}", agentHandler.Get)
					r.Put("/{id}", agentHandler.Update)
					r.Delete("/{id}", agentHandler.Delete)
					r.Get("/{id}/setup", agentHandler.GetSetup)
					r.Put("/{id}/setup", agentHandler.UpdateSetup)
					// Conversations under agent
					if conversationHandler != nil {
						r.Post("/{agentID}/conversations", conversationHandler.Create)
						r.Get("/{agentID}/conversations", conversationHandler.List)
					}
					// Forge under agent
					if forgeHandler != nil {
						r.Post("/{agentID}/forge", forgeHandler.Start)
						r.Get("/{agentID}/forge", forgeHandler.ListRuns)
					}
				})
				// Forge run operations (top-level by run ID)
				if forgeHandler != nil {
					r.Route("/forge-runs/{runID}", func(r chi.Router) {
						r.Get("/", forgeHandler.GetRun)
						r.Get("/stream", forgeHandler.Stream)
						r.Get("/events", forgeHandler.ListEvents)
						r.Post("/cancel", forgeHandler.Cancel)
						r.Post("/apply", forgeHandler.Apply)
						r.Get("/iterations/{iterationID}/evals", forgeHandler.ListEvals)
					})
				}
				// Conversation operations (top-level by conversation ID)
				if conversationHandler != nil {
					r.Route("/conversations/{convID}", func(r chi.Router) {
						r.Get("/", conversationHandler.Get)
						r.Delete("/", conversationHandler.End)
						r.Post("/messages", conversationHandler.SendMessage)
						r.Get("/stream", conversationHandler.Stream)
						r.Post("/abort", conversationHandler.Abort)
						r.Get("/approvals", conversationHandler.ListApprovals)
						r.Post("/approvals/{requestID}", conversationHandler.ResolveApproval)
						r.Get("/events", conversationHandler.ListEvents)
					})
				}
				// System agent conversations
				if systemConvHandler != nil {
					r.Post("/system-agents/{type}/conversations", systemConvHandler.Create)
				}
				// Sandbox management
				r.Route("/sandboxes", func(r chi.Router) {
					sandboxHandler := handler.NewSandboxHandler(database, orchestrator)
					r.Get("/", sandboxHandler.List)
					r.Get("/{id}", sandboxHandler.Get)
					if orchestrator != nil {
						r.Post("/{id}/stop", sandboxHandler.Stop)
						r.Post("/{id}/exec", sandboxHandler.Exec)
						r.Delete("/{id}", sandboxHandler.Delete)
					}
				})
			})

			// Settings — scope: "all"
			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireAPIKeyScopeOrJWT("all"))
				r.Get("/settings/connect", settingsHandler.GetConnectSettings)
				r.Put("/settings/connect", settingsHandler.UpdateConnectSettings)
				r.Get("/settings/webhooks", settingsHandler.GetWebhookSettings)
				r.Put("/settings/webhooks", settingsHandler.UpdateWebhookSettings)
				r.Post("/settings/webhooks/rotate-secret", settingsHandler.RotateWebhookSecret)
				r.Delete("/settings/webhooks", settingsHandler.DeleteWebhookSettings)
				r.Post("/custom-domains", customDomainHandler.Create)
				r.Get("/custom-domains", customDomainHandler.List)
				r.Post("/custom-domains/{id}/verify", customDomainHandler.Verify)
				r.Delete("/custom-domains/{id}", customDomainHandler.Delete)
			})
		})
	})

	// Connect API (session-authenticated — used by Connect widget iframe)
	r.Route("/v1/widget", func(r chi.Router) {
		r.Use(middleware.ConnectSessionAuth(database))
		r.Use(middleware.ConnectSecurityHeaders())
		r.Use(middleware.ConnectCORS())

		r.Get("/session", connectAPIHandler.SessionInfo)
		r.Get("/providers", connectAPIHandler.ListProviders)
		r.Route("/integrations", func(r chi.Router) {
			r.Get("/providers", integrationHandler.ListProviders)
			r.Get("/", connectAPIHandler.ListIntegrations)
			r.Post("/{id}/connect-session", connectAPIHandler.CreateIntegrationConnectSession)
			r.Get("/{id}/resources/{type}/available", connectAPIHandler.ListAvailableResources)
			r.Post("/{id}/connections", connectAPIHandler.CreateIntegrationConnection)
			r.Patch("/{id}/connections/{connectionId}", connectAPIHandler.PatchIntegrationConnection)
			r.Delete("/{id}/connections/{connectionId}", connectAPIHandler.DeleteIntegrationConnection)
		})
		r.Get("/connections", connectAPIHandler.ListConnections)
		r.Post("/connections", connectAPIHandler.CreateConnection)
		r.Delete("/connections/{id}", connectAPIHandler.DeleteConnection)
		r.Post("/connections/{id}/verify", connectAPIHandler.VerifyConnection)
	})

	// In-integrations & in-connections (app-owned, user-scoped)
	var platformAdminEmails []string
	if cfg.PlatformAdminEmails != "" {
		platformAdminEmails = strings.Split(cfg.PlatformAdminEmails, ",")
	}
	r.Route("/v1/in", func(r chi.Router) {
		r.Use(middleware.RequireAuth(rsaPub, cfg.AuthIssuer, cfg.AuthAudience))
		r.Use(middleware.RequireEmailConfirmed(database))
		r.Use(middleware.ResolveUser(database))

		// Admin CRUD for in-integrations
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequirePlatformAdmin(platformAdminEmails))
			r.Post("/integrations", inIntegrationHandler.Create)
			r.Get("/integrations", inIntegrationHandler.List)
			r.Get("/integrations/{id}", inIntegrationHandler.Get)
			r.Put("/integrations/{id}", inIntegrationHandler.Update)
			r.Delete("/integrations/{id}", inIntegrationHandler.Delete)
		})

		// User connections (any authenticated user, org-scoped)
		r.Group(func(r chi.Router) {
			r.Use(middleware.ResolveOrgFlexible(database))
			r.Post("/integrations/{id}/connect-session", inConnectionHandler.CreateConnectSession)
			r.Post("/integrations/{id}/connections", inConnectionHandler.Create)
			r.Get("/connections", inConnectionHandler.List)
			r.Get("/connections/{id}", inConnectionHandler.Get)
			r.Delete("/connections/{id}", inConnectionHandler.Revoke)
		})
	})

	// Admin API (disabled by default — only mounted when ADMIN_API_ENABLED=true)
	if cfg.AdminAPIEnabled {
		adminHandler := handler.NewAdminHandler(database, orchestrator, nangoClient, actionsCatalog)
		r.Route("/admin/v1", func(r chi.Router) {
			r.Use(middleware.RequireAuth(rsaPub, cfg.AuthIssuer, cfg.AuthAudience))
			r.Use(middleware.RequireEmailConfirmed(database))
			r.Use(middleware.ResolveUser(database))
			r.Use(middleware.RequirePlatformAdmin(platformAdminEmails))
			r.Use(middleware.AdminAudit(database))

			// Platform stats
			r.Get("/stats", adminHandler.Stats)

			// Users
			r.Get("/users", adminHandler.ListUsers)
			r.Get("/users/{id}", adminHandler.GetUser)
			r.Put("/users/{id}", adminHandler.UpdateUser)
			r.Post("/users/{id}/ban", adminHandler.BanUser)
			r.Post("/users/{id}/unban", adminHandler.UnbanUser)
			r.Post("/users/{id}/confirm-email", adminHandler.ConfirmUserEmail)
			r.Delete("/users/{id}", adminHandler.DeleteUser)

			// Organizations
			r.Get("/orgs", adminHandler.ListOrgs)
			r.Get("/orgs/{id}", adminHandler.GetOrg)
			r.Put("/orgs/{id}", adminHandler.UpdateOrgFull)
			r.Post("/orgs/{id}/deactivate", adminHandler.DeactivateOrg)
			r.Post("/orgs/{id}/activate", adminHandler.ActivateOrg)
			r.Get("/orgs/{id}/members", adminHandler.ListOrgMembers)
			r.Delete("/orgs/{id}", adminHandler.DeleteOrg)

			// Credentials
			r.Get("/credentials", adminHandler.ListCredentials)
			r.Get("/credentials/{id}", adminHandler.GetCredential)
			r.Put("/credentials/{id}", adminHandler.UpdateCredential)
			r.Post("/credentials/{id}/revoke", adminHandler.RevokeCredential)

			// API Keys
			r.Get("/api-keys", adminHandler.ListAPIKeys)
			r.Post("/api-keys/{id}/revoke", adminHandler.RevokeAPIKey)

			// Tokens
			r.Get("/tokens", adminHandler.ListTokens)
			r.Post("/tokens/{id}/revoke", adminHandler.RevokeToken)

			// Identities
			r.Get("/identities", adminHandler.ListIdentities)
			r.Get("/identities/{id}", adminHandler.GetIdentity)
			r.Put("/identities/{id}", adminHandler.UpdateIdentity)
			r.Delete("/identities/{id}", adminHandler.DeleteIdentity)

			// Agents
			r.Get("/agents", adminHandler.ListAgents)
			r.Get("/agents/{id}", adminHandler.GetAgent)
			r.Put("/agents/{id}", adminHandler.UpdateAgent)
			r.Post("/agents/{id}/archive", adminHandler.ArchiveAgent)
			r.Delete("/agents/{id}", adminHandler.DeleteAgent)

			// Sandboxes
			r.Get("/sandboxes", adminHandler.ListSandboxes)
			r.Get("/sandboxes/{id}", adminHandler.GetSandbox)
			r.Post("/sandboxes/{id}/stop", adminHandler.StopSandbox)
			r.Delete("/sandboxes/{id}", adminHandler.DeleteSandbox)
			r.Post("/sandboxes/cleanup", adminHandler.CleanupSandboxes)

			// Sandbox Templates
			r.Get("/sandbox-templates", adminHandler.ListSandboxTemplates)
			r.Put("/sandbox-templates/{id}", adminHandler.UpdateSandboxTemplate)
			r.Delete("/sandbox-templates/{id}", adminHandler.DeleteSandboxTemplate)

			// Conversations
			r.Get("/conversations", adminHandler.ListConversations)
			r.Get("/conversations/{id}", adminHandler.GetConversation)
			r.Delete("/conversations/{id}", adminHandler.EndConversation)

			// Forge Runs
			r.Get("/forge-runs", adminHandler.ListForgeRuns)
			r.Get("/forge-runs/{id}", adminHandler.GetForgeRun)
			r.Post("/forge-runs/{id}/cancel", adminHandler.CancelForgeRun)

			// Generations
			r.Get("/generations", adminHandler.ListGenerations)
			r.Get("/generations/stats", adminHandler.GenerationStats)

			// Integrations & Connections
			r.Get("/integrations", adminHandler.ListIntegrations)
			r.Get("/connections", adminHandler.ListConnections)
			r.Post("/connections/{id}/revoke", adminHandler.RevokeConnection)
			r.Get("/in-integration-providers", adminHandler.ListInIntegrationProviders)
			r.Post("/in-integrations", adminHandler.CreateInIntegration)
			r.Get("/in-integrations", adminHandler.ListInIntegrations)
			r.Get("/in-integrations/{id}", adminHandler.GetInIntegration)
			r.Put("/in-integrations/{id}", adminHandler.UpdateInIntegration)
			r.Delete("/in-integrations/{id}", adminHandler.DeleteInIntegration)
			r.Get("/in-connections", adminHandler.ListInConnections)

			// Connect Sessions
			r.Get("/connect-sessions", adminHandler.ListConnectSessions)
			r.Delete("/connect-sessions/{id}", adminHandler.DeleteConnectSession)

			// Custom Domains
			r.Get("/custom-domains", adminHandler.ListCustomDomains)
			r.Delete("/custom-domains/{id}", adminHandler.DeleteCustomDomain)

			// Audit & Usage
			r.Get("/audit", adminHandler.ListAudit)
			r.Get("/usage", adminHandler.ListUsage)
			r.Get("/admin-audit", adminHandler.ListAdminAudit)

			// Workspace Storage
			r.Get("/workspace-storage", adminHandler.ListWorkspaceStorage)
			r.Delete("/workspace-storage/{id}", adminHandler.DeleteWorkspaceStorage)
		})
		slog.Info("admin API enabled", "path", "/admin/v1")
	}

	// Sandbox-authenticated routes (proxy) — token auth via JWT
	r.Route("/v1/proxy", func(r chi.Router) {
		r.Use(middleware.TokenAuth(signingKey, database))
		r.Use(middleware.IdentityRateLimit(redisClient, database))
		r.Use(middleware.RemainingCheck(ctr))
		r.Use(middleware.Audit(auditWriter, "proxy.request"))
		r.Use(middleware.Generation(generationWriter, database))
		r.Handle("/*", proxyHandler)
	})

	// 15. Token cleanup (expired email verifications & password resets)
	goroutine.Go(func() {
		ticker := time.NewTicker(6 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				cutoff := time.Now().Add(-7 * 24 * time.Hour)
				database.Where("expires_at < ? OR used_at < ?", cutoff, cutoff).Delete(&model.EmailVerification{})
				database.Where("expires_at < ? OR used_at < ?", cutoff, cutoff).Delete(&model.PasswordReset{})
				database.Where("expires_at < ? OR used_at < ?", cutoff, cutoff).Delete(&model.OAuthExchangeToken{})
				slog.Debug("cleaned up expired verification/reset tokens")
			}
		}
	})

	// 16. Server
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 0, // Disabled for streaming responses
		IdleTimeout:  120 * time.Second,
	}

	goroutine.Go(func() {
		slog.Info("server starting", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	})

	// 16. MCP Server (separate port)
	mcpRouter := chi.NewRouter()
	mcpRouter.Use(chimw.RequestID)
	mcpRouter.Use(chimw.RealIP)
	mcpRouter.Use(chimw.Recoverer)
	mcpRouter.Use(middleware.RequestLog(logger))

	// Hindsight memory MCP tools (if configured)
	if cfg.HindsightAPIURL != "" {
		memoryHandler := hindsight.NewMemoryMCPHandler(database, hindsight.NewClient(cfg.HindsightAPIURL))
		mcpRouter.Route("/memory/{agentID}", func(r chi.Router) {
			r.Use(middleware.TokenAuth(signingKey, database))
			r.Use(memoryHandler.ValidateAgentToken)
			r.Handle("/*", memoryHandler.StreamableHTTPHandler())
			r.Handle("/", memoryHandler.StreamableHTTPHandler())
		})
		memoryHandler.StartCleanup(ctx, 5*time.Minute)
		slog.Info("hindsight memory MCP tools registered on /memory/{agentID}")
	}

	// Forge MCP server (mock tools for eval execution)
	mcpRouter.Route("/forge/{forgeRunID}", func(r chi.Router) {
		r.Use(middleware.TokenAuth(signingKey, database))
		r.Handle("/*", forgeMCPHandler.StreamableHTTPHandler())
		r.Handle("/", forgeMCPHandler.StreamableHTTPHandler())
	})
	slog.Info("forge MCP tools registered on /forge/{forgeRunID}")

	// Streamable HTTP transport (primary)
	mcpRouter.Route("/{jti}", func(r chi.Router) {
		r.Use(middleware.TokenAuth(signingKey, database))
		r.Use(mcpHandler.ValidateJTIMatch)
		r.Use(mcpHandler.ValidateHasScopes)
		r.Handle("/*", mcpHandler.StreamableHTTPHandler())
	})

	// SSE transport (legacy compatibility)
	mcpRouter.Route("/sse/{jti}", func(r chi.Router) {
		r.Use(middleware.TokenAuth(signingKey, database))
		r.Use(mcpHandler.ValidateJTIMatch)
		r.Use(mcpHandler.ValidateHasScopes)
		r.Handle("/*", mcpHandler.SSEHandler())
	})

	mcpSrv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.MCPPort),
		Handler:      mcpRouter,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 0, // streaming
		IdleTimeout:  120 * time.Second,
	}

	mcpHandler.ServerCache.StartCleanup(ctx, 5*time.Minute)

	goroutine.Go(func() {
		slog.Info("mcp server starting", "port", cfg.MCPPort)
		if err := mcpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("mcp server error", "error", err)
		}
	})

	// 17. Wait for shutdown signal
	<-ctx.Done()
	slog.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Drain HTTP connections (both servers)
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("server shutdown error", "error", err)
	}
	if err := mcpSrv.Shutdown(shutdownCtx); err != nil {
		slog.Error("mcp server shutdown error", "error", err)
	}

	// Flush audit + generation buffers
	auditWriter.Shutdown(shutdownCtx)
	generationWriter.Shutdown(shutdownCtx)

	// Purge L1 cache (zeros memguard enclaves)
	cacheManager.Memory().Purge()

	// Close database connection pool
	if sqlDB, err := database.DB(); err == nil {
		_ = sqlDB.Close()
	}

	slog.Info("shutdown complete")
	return nil
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func readyz(database *gorm.DB, rc *redis.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Check Postgres
		sqlDB, err := database.DB()
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"error","detail":"db connection failed"}`))
			return
		}
		if err := sqlDB.Ping(); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"error","detail":"db ping failed"}`))
			return
		}

		// Check Redis
		if err := rc.Ping(r.Context()).Err(); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"error","detail":"redis ping failed"}`))
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}
}

func disableCoreDumps() {
	// Set RLIMIT_CORE to 0 to prevent core dumps that could leak secrets.
	var rLimit syscall.Rlimit
	rLimit.Cur = 0
	rLimit.Max = 0
	_ = syscall.Setrlimit(syscall.RLIMIT_CORE, &rLimit)
}
