package main

import (
	"context"
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

	"github.com/useportal/proxy-bridge/internal/cache"
	"github.com/useportal/proxy-bridge/internal/config"
	"github.com/useportal/proxy-bridge/internal/counter"
	"github.com/useportal/proxy-bridge/internal/crypto"
	"github.com/useportal/proxy-bridge/internal/db"
	"github.com/useportal/proxy-bridge/internal/handler"
	"github.com/useportal/proxy-bridge/internal/logging"
	"github.com/useportal/proxy-bridge/internal/middleware"
	"github.com/useportal/proxy-bridge/internal/model"
	"github.com/useportal/proxy-bridge/internal/proxy"
	"github.com/useportal/proxy-bridge/internal/registry"
)

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
	slog.Info("starting proxy-bridge", "version", version, "commit", commit)

	// 3. Database
	database, err := db.New(cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	if err := model.AutoMigrate(database); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}
	slog.Info("database ready")

	// 4. Vault Transit
	vault, err := crypto.NewVaultTransit(cfg.VaultAddr, cfg.VaultToken, cfg.VaultKeyName)
	if err != nil {
		return fmt.Errorf("connecting to vault: %w", err)
	}
	slog.Info("vault transit ready")

	// 5. Redis
	redisClient := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})
	if err := redisClient.Ping(context.Background()).Err(); err != nil {
		return fmt.Errorf("connecting to redis: %w", err)
	}
	defer redisClient.Close()
	slog.Info("redis ready")

	// 6. Cache manager (L1 memory → L2 Redis → L3 Postgres+Vault)
	cacheCfg := cache.Config{
		MemMaxSize: cfg.MemCacheMaxSize,
		MemTTL:     cfg.MemCacheTTL,
		RedisTTL:   cfg.RedisCacheTTL,
		DEKMaxSize: 1000,
		DEKTTL:     30 * time.Minute,
		HardExpiry: 15 * time.Minute,
	}
	cacheManager := cache.Build(cacheCfg, redisClient, vault, database)
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

	// 11. Provider registry (embedded at build time)
	reg := registry.Global()
	slog.Info("provider registry ready", "providers", reg.ProviderCount(), "models", reg.ModelCount())

	// 12. Handlers
	credHandler := handler.NewCredentialHandler(database, vault, cacheManager, ctr)
	tokenHandler := handler.NewTokenHandler(database, signingKey, cacheManager, ctr)
	identityHandler := handler.NewIdentityHandler(database)
	providerHandler := handler.NewProviderHandler(reg)
	connectSessionHandler := handler.NewConnectSessionHandler(database, reg)
	connectAPIHandler := handler.NewConnectAPIHandler(database, vault, reg)
	settingsHandler := handler.NewSettingsHandler(database)
	proxyHandler := handler.NewProxyHandler(cacheManager, proxy.NewTransport())

	// 11. Router
	r := chi.NewRouter()

	// Global middleware
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)
	r.Use(middleware.RequestLog(logger))

	// Health checks (no auth)
	r.Get("/healthz", healthz)
	r.Get("/readyz", readyz(database, redisClient))

	// Provider discovery (no auth — used by frontend)
	r.Get("/v1/providers", providerHandler.List)
	r.Get("/v1/providers/{id}", providerHandler.Get)
	r.Get("/v1/providers/{id}/models", providerHandler.Models)

	// Org-authenticated routes (ZITADEL) — credential & token management
	if cfg.ZitadelClientID != "" && cfg.ZitadelClientSecret != "" {
		zitadelMW, err := middleware.NewZitadelAuth(ctx, cfg.ZitadelDomain, cfg.ZitadelClientID, cfg.ZitadelClientSecret)
		if err != nil {
			return fmt.Errorf("initializing zitadel auth: %w", err)
		}
		slog.Info("zitadel auth ready")

		r.Route("/v1", func(r chi.Router) {
			r.Use(zitadelMW.RequireAuthorization())
			r.Use(middleware.ResolveOrg(zitadelMW, database))
			r.Use(middleware.RateLimit())
			r.Use(middleware.Audit(auditWriter))

			r.Post("/credentials", credHandler.Create)
			r.Get("/credentials", credHandler.List)
			r.Delete("/credentials/{id}", credHandler.Revoke)
			r.Post("/tokens", tokenHandler.Mint)
			r.Delete("/tokens/{jti}", tokenHandler.Revoke)
			r.Post("/identities", identityHandler.Create)
			r.Get("/identities", identityHandler.List)
			r.Get("/identities/{id}", identityHandler.Get)
			r.Put("/identities/{id}", identityHandler.Update)
			r.Delete("/identities/{id}", identityHandler.Delete)
			r.Post("/connect/sessions", connectSessionHandler.Create)
			r.Get("/settings/connect", settingsHandler.GetConnectSettings)
			r.Put("/settings/connect", settingsHandler.UpdateConnectSettings)
		})
	} else {
		slog.Warn("ZITADEL credentials not configured — management API disabled")
	}

	// Connect API (session-authenticated — used by Connect widget iframe)
	r.Route("/connect/api", func(r chi.Router) {
		r.Use(middleware.ConnectSessionAuth(database))
		r.Use(middleware.ConnectSecurityHeaders())
		r.Use(middleware.ConnectCORS())

		r.Get("/session", connectAPIHandler.SessionInfo)
		r.Get("/providers", connectAPIHandler.ListProviders)
		r.Get("/connections", connectAPIHandler.ListConnections)
		r.Post("/connections", connectAPIHandler.CreateConnection)
		r.Delete("/connections/{id}", connectAPIHandler.DeleteConnection)
		r.Post("/connections/{id}/verify", connectAPIHandler.VerifyConnection)
	})

	// Sandbox-authenticated routes (proxy) — token auth via JWT
	r.Route("/v1/proxy/{credentialID}", func(r chi.Router) {
		r.Use(middleware.TokenAuth(signingKey, database))
		r.Use(middleware.IdentityRateLimit(redisClient, database))
		r.Use(middleware.RemainingCheck(ctr))
		r.Use(middleware.Audit(auditWriter))
		r.Handle("/*", proxyHandler)
	})

	// 12. Server
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

	// 13. Wait for shutdown signal
	<-ctx.Done()
	slog.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Drain HTTP connections
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("server shutdown error", "error", err)
	}

	// Flush audit buffer
	auditWriter.Shutdown(shutdownCtx)

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
