package bootstrap

import (
	"context"
	"crypto/rsa"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/auth"
	"github.com/ziraloop/ziraloop/internal/cache"
	"github.com/ziraloop/ziraloop/internal/config"
	"github.com/ziraloop/ziraloop/internal/counter"
	"github.com/ziraloop/ziraloop/internal/crypto"
	"github.com/ziraloop/ziraloop/internal/db"
	"github.com/ziraloop/ziraloop/internal/hindsight"
	"github.com/ziraloop/ziraloop/internal/mcp/catalog"
	"github.com/ziraloop/ziraloop/internal/model"
	"github.com/ziraloop/ziraloop/internal/nango"
	"github.com/ziraloop/ziraloop/internal/registry"
	"github.com/ziraloop/ziraloop/internal/sandbox"
	"github.com/ziraloop/ziraloop/internal/sandbox/daytona"
	"github.com/ziraloop/ziraloop/internal/streaming"
	"github.com/ziraloop/ziraloop/internal/turso"
)

// Deps holds all shared dependencies initialized during bootstrap.
// Both the API server and the Asynq worker use this struct.
type Deps struct {
	Config         *config.Config
	DB             *gorm.DB
	Redis          *redis.Client
	KMS            *crypto.KeyWrapper
	CacheManager   *cache.Manager
	APIKeyCache    *cache.APIKeyCache
	Counter        *counter.Counter
	NangoClient    *nango.Client
	Registry       *registry.Registry
	ActionsCatalog *catalog.Catalog
	RSAKey         *rsa.PrivateKey
	SigningKey      []byte
	SandboxEncKey  *crypto.SymmetricKey
	Orchestrator   *sandbox.Orchestrator
	AgentPusher    *sandbox.Pusher
	EventBus       *streaming.EventBus
	Flusher        *streaming.Flusher
	Cleanup        *streaming.Cleanup
	Retainer       *hindsight.Retainer // nil if Hindsight not configured
	HindsightMCPURL func(uuid.UUID) string // nil if Hindsight not configured
}

// New initializes all shared dependencies. The caller is responsible for
// closing resources via Deps.Close().
func New(ctx context.Context) (*Deps, error) {
	// 1. Config
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}

	// 2. Logging
	// Note: logging is NOT initialized here — the caller must do it before
	// calling New() so that any errors during bootstrap are properly formatted.

	// 3. Database
	database, err := db.New(cfg.DatabaseDSN())
	if err != nil {
		return nil, fmt.Errorf("connecting to database: %w", err)
	}
	if err := model.AutoMigrate(database); err != nil {
		return nil, fmt.Errorf("running migrations: %w", err)
	}
	slog.Info("database ready")

	// 4. KMS wrapper
	var kms *crypto.KeyWrapper
	switch cfg.KMSType {
	case "aead":
		kms, err = crypto.NewAEADWrapper(cfg.KMSKey, "aead-local")
	case "awskms":
		kms, err = crypto.NewAWSKMSWrapper(cfg.KMSKey, cfg.AWSRegion)
	case "vault":
		vaultCfg := cfg.VaultConfig()
		if vaultCfg == nil {
			return nil, fmt.Errorf("vault configuration is nil")
		}
		kms, err = crypto.NewVaultTransitWrapper(*vaultCfg)
	default:
		return nil, fmt.Errorf("unsupported KMS_TYPE: %q (supported: aead, awskms, vault)", cfg.KMSType)
	}
	if err != nil {
		return nil, fmt.Errorf("creating %s KMS wrapper: %w", cfg.KMSType, err)
	}
	slog.Info("kms wrapper ready", "type", cfg.KMSType)

	// 5. Redis
	var redisOpts *redis.Options
	if cfg.RedisURL != "" {
		redisOpts, err = redis.ParseURL(cfg.RedisURL)
		if err != nil {
			return nil, fmt.Errorf("parsing REDIS_URL: %w", err)
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
		return nil, fmt.Errorf("connecting to redis: %w", err)
	}
	slog.Info("redis ready")

	// 6. Cache manager
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

	// 7. Request-cap counter
	ctr := counter.New(redisClient, database)
	slog.Info("request counter ready")

	// 8. Signing key
	signingKey := []byte(cfg.JWTSigningKey)

	// 9. RSA key for embedded auth
	rsaKey, err := auth.LoadRSAPrivateKey(cfg.AuthRSAPrivateKey)
	if err != nil {
		return nil, fmt.Errorf("loading auth RSA key: %w", err)
	}
	slog.Info("embedded auth ready")

	// 10. Provider registry
	reg := registry.Global()
	slog.Info("provider registry ready", "providers", reg.ProviderCount(), "models", reg.ModelCount())

	// 11. Nango client
	if cfg.NangoEndpoint == "" || cfg.NangoSecretKey == "" {
		return nil, fmt.Errorf("NANGO_ENDPOINT and NANGO_SECRET_KEY are required")
	}
	nangoClient := nango.NewClient(cfg.NangoEndpoint, cfg.NangoSecretKey)
	if err := nangoClient.FetchProviders(context.Background()); err != nil {
		return nil, fmt.Errorf("fetching Nango provider catalog: %w", err)
	}
	slog.Info("nango client ready", "providers", len(nangoClient.GetProviders()))

	// 12. Actions catalog
	actionsCatalog := catalog.Global()
	slog.Info("actions catalog ready", "providers", len(actionsCatalog.ListProviders()))

	// 13. Sandbox encryption key
	var sandboxEncKey *crypto.SymmetricKey
	if cfg.SandboxEncryptionKey != "" {
		sandboxEncKey, err = crypto.NewSymmetricKey(cfg.SandboxEncryptionKey)
		if err != nil {
			return nil, fmt.Errorf("invalid SANDBOX_ENCRYPTION_KEY: %w", err)
		}
	}

	// 14. Sandbox orchestrator (optional)
	var orchestrator *sandbox.Orchestrator
	var agentPusher *sandbox.Pusher
	var hindsightMCPURL func(uuid.UUID) string

	if cfg.SandboxProviderKey != "" && sandboxEncKey != nil {
		sandboxProvider, err := daytona.NewDriver(daytona.Config{
			APIURL: cfg.SandboxProviderURL,
			APIKey: cfg.SandboxProviderKey,
			Target: cfg.SandboxTarget,
		})
		if err != nil {
			return nil, fmt.Errorf("creating sandbox provider: %w", err)
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

		if cfg.HindsightAPIURL != "" {
			mcpBase := cfg.MCPBaseURL
			hindsightMCPURL = func(agentID uuid.UUID) string {
				return mcpBase + "/memory/" + agentID.String()
			}
		}

		agentPusher = sandbox.NewPusher(database, orchestrator, signingKey, cfg, hindsightMCPURL)
		slog.Info("sandbox orchestrator ready")
	}

	// 15. Event streaming
	eventBus := streaming.NewEventBus(redisClient)
	flusher := streaming.NewFlusher(eventBus, database)
	cleanup := streaming.NewCleanup(eventBus)

	// 16. Hindsight retainer (optional)
	var retainer *hindsight.Retainer
	if cfg.HindsightAPIURL != "" {
		hClient := hindsight.NewClient(cfg.HindsightAPIURL)
		retainer = hindsight.NewRetainer(eventBus, database, hClient)
	}

	return &Deps{
		Config:          cfg,
		DB:              database,
		Redis:           redisClient,
		KMS:             kms,
		CacheManager:    cacheManager,
		APIKeyCache:     apiKeyCache,
		Counter:         ctr,
		NangoClient:     nangoClient,
		Registry:        reg,
		ActionsCatalog:  actionsCatalog,
		RSAKey:          rsaKey,
		SigningKey:       signingKey,
		SandboxEncKey:   sandboxEncKey,
		Orchestrator:    orchestrator,
		AgentPusher:     agentPusher,
		EventBus:        eventBus,
		Flusher:         flusher,
		Cleanup:         cleanup,
		Retainer:        retainer,
		HindsightMCPURL: hindsightMCPURL,
	}, nil
}

// Close releases all resources held by Deps.
func (d *Deps) Close() {
	d.CacheManager.Memory().Purge()
	if sqlDB, err := d.DB.DB(); err == nil {
		_ = sqlDB.Close()
	}
	_ = d.Redis.Close()
	slog.Info("deps closed")
}
