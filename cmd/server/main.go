package main

import (
	"context"
	"crypto/rsa"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/awnumar/memguard"
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/llmvault/llmvault/internal/auth"
	"github.com/llmvault/llmvault/internal/cache"
	"github.com/llmvault/llmvault/internal/config"
	"github.com/llmvault/llmvault/internal/counter"
	"github.com/llmvault/llmvault/internal/crypto"
	"github.com/llmvault/llmvault/internal/db"
	"github.com/llmvault/llmvault/internal/handler"
	"github.com/llmvault/llmvault/internal/logging"
	"github.com/llmvault/llmvault/internal/mcp/catalog"
	"github.com/llmvault/llmvault/internal/middleware"
	"github.com/llmvault/llmvault/internal/model"
	"github.com/llmvault/llmvault/internal/nango"
	"github.com/llmvault/llmvault/internal/proxy"
	"github.com/llmvault/llmvault/internal/registry"
)

// @title LLMVault API
// @version 1.0
// @description Proxy bridge for LLM API credentials.
// @host api.dev.llmvault.dev
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
	slog.Info("starting llmvault", "version", version, "commit", commit)

	// 3. Database
	database, err := db.New(cfg.DatabaseDSN())
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	if err := model.AutoMigrate(database); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}
	slog.Info("database ready")

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

	go func() {
		if err := cacheManager.Invalidator().Subscribe(ctx); err != nil {
			slog.Error("invalidation subscriber stopped", "error", err)
		}
	}()

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
	slog.Info("embedded auth ready")

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

	// 13. Handlers
	mcpHandler := handler.NewMCPHandler(database, signingKey, actionsCatalog, nangoClient, ctr)
	credHandler := handler.NewCredentialHandler(database, kms, cacheManager, ctr)
	tokenHandler := handler.NewTokenHandler(database, signingKey, cacheManager, ctr, actionsCatalog, cfg.MCPBaseURL, mcpHandler.ServerCache)
	identityHandler := handler.NewIdentityHandler(database)
	providerHandler := handler.NewProviderHandler(reg)
	connectSessionHandler := handler.NewConnectSessionHandler(database)
	connectAPIHandler := handler.NewConnectAPIHandler(database, kms, reg, nangoClient, actionsCatalog)
	settingsHandler := handler.NewSettingsHandler(database)
	integrationHandler := handler.NewIntegrationHandler(database, nangoClient)
	connectionHandler := handler.NewConnectionHandler(database, nangoClient, actionsCatalog)
	orgHandler := handler.NewOrgHandler(database)
	authHandler := handler.NewAuthHandler(database, rsaKey, signingKey,
		cfg.AuthIssuer, cfg.AuthAudience, cfg.AuthAccessTokenTTL, cfg.AuthRefreshTokenTTL)
	apiKeyHandler := handler.NewAPIKeyHandler(database, apiKeyCache, cacheManager)
	usageHandler := handler.NewUsageHandler(database)
	auditHandler := handler.NewAuditHandler(database)
	generationHandler := handler.NewGenerationHandler(database)
	reportingHandler := handler.NewReportingHandler(database)
	proxyHandler := handler.NewProxyHandler(cacheManager, &proxy.CaptureTransport{Inner: proxy.NewTransport()})

	// 14. Router
	r := chi.NewRouter()

	// Global middleware
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)
	r.Use(middleware.CORS(cfg.CORSOrigins))
	r.Use(middleware.RequestLog(logger))

	// Health checks (no auth)
	r.Get("/healthz", healthz)
	r.Get("/readyz", readyz(database, redisClient))

	// Provider discovery (no auth — used by frontend)
	r.Get("/v1/providers", providerHandler.List)
	r.Get("/v1/providers/{id}", providerHandler.Get)
	r.Get("/v1/providers/{id}/models", providerHandler.Models)

	// Integration catalog discovery (no auth — MCP actions/resources catalog)
	actionsHandler := handler.NewActionsHandler(actionsCatalog)
	r.Get("/v1/catalog/integrations", actionsHandler.ListIntegrations)
	r.Get("/v1/catalog/integrations/{id}", actionsHandler.GetIntegration)
	r.Get("/v1/catalog/integrations/{id}/actions", actionsHandler.ListActions)

	// Embedded auth
	rsaPub := rsaKey.Public().(*rsa.PublicKey)

	// Auth routes (register, login, refresh, logout, me)
	r.Route("/auth", func(r chi.Router) {
		r.Post("/register", authHandler.Register)
		r.Post("/login", authHandler.Login)
		r.Post("/refresh", authHandler.Refresh)
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireAuth(rsaPub, cfg.AuthIssuer, cfg.AuthAudience))
			r.Post("/logout", authHandler.Logout)
			r.Get("/me", authHandler.Me)
		})
	})

	// Org-authenticated routes (JWT or API Key) — credential & token management
	r.Route("/v1", func(r chi.Router) {
		r.Use(middleware.MultiAuth(rsaPub, cfg.AuthIssuer, cfg.AuthAudience, database, apiKeyCache))

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

			// Settings — scope: "all"
			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireAPIKeyScopeOrJWT("all"))
				r.Get("/settings/connect", settingsHandler.GetConnectSettings)
				r.Put("/settings/connect", settingsHandler.UpdateConnectSettings)
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

	// Sandbox-authenticated routes (proxy) — token auth via JWT
	r.Route("/v1/proxy", func(r chi.Router) {
		r.Use(middleware.TokenAuth(signingKey, database))
		r.Use(middleware.IdentityRateLimit(redisClient, database))
		r.Use(middleware.RemainingCheck(ctr))
		r.Use(middleware.Audit(auditWriter, "proxy.request"))
		r.Use(middleware.Generation(generationWriter, database))
		r.Handle("/*", proxyHandler)
	})

	// 15. Server
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 0, // Disabled for streaming responses
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		slog.Info("server starting", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	// 16. MCP Server (separate port)
	mcpRouter := chi.NewRouter()
	mcpRouter.Use(chimw.RequestID)
	mcpRouter.Use(chimw.RealIP)
	mcpRouter.Use(chimw.Recoverer)
	mcpRouter.Use(middleware.RequestLog(logger))

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

	go func() {
		slog.Info("mcp server starting", "port", cfg.MCPPort)
		if err := mcpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("mcp server error", "error", err)
		}
	}()

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
