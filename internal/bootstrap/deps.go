package bootstrap

import (
	"context"
	"crypto/rsa"
	"fmt"
	"log/slog"
	"time"

	ph "github.com/posthog/posthog-go"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/auth"
	"github.com/usehiveloop/hiveloop/internal/cache"
	"github.com/usehiveloop/hiveloop/internal/config"
	"github.com/usehiveloop/hiveloop/internal/counter"
	"github.com/usehiveloop/hiveloop/internal/crypto"
	"github.com/usehiveloop/hiveloop/internal/db"
	"github.com/usehiveloop/hiveloop/internal/hindsight"
	"github.com/usehiveloop/hiveloop/internal/mcp/catalog"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/nango"
	"github.com/usehiveloop/hiveloop/internal/rag"
	"github.com/usehiveloop/hiveloop/internal/registry"
	"github.com/usehiveloop/hiveloop/internal/sandbox"
	"github.com/usehiveloop/hiveloop/internal/sandbox/daytona"
	"github.com/usehiveloop/hiveloop/internal/spider"
	"github.com/usehiveloop/hiveloop/internal/storage"
	"github.com/usehiveloop/hiveloop/internal/streaming"
	"github.com/usehiveloop/hiveloop/internal/turso"

	polargo "github.com/polarsource/polar-go"

	posthogobs "github.com/usehiveloop/hiveloop/internal/observability/posthog"
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
	Retainer        *hindsight.Retainer         // nil if Hindsight not configured
	SpiderClient    *spider.Client             // nil if spider not configured
	ToolUsageWriter *middleware.ToolUsageWriter // nil if spider not configured
	PolarClient     *polargo.Polar             // nil if POLAR_ACCESS_TOKEN not set
	S3Client        *storage.S3Client          // nil if AWS_S3_BUCKET_NAME not set
	PostHog         ph.Client                  // nil if PostHog disabled
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
	// RAG schema migrations — wired here (and not inside
	// model.AutoMigrate) because internal/rag/model imports
	// internal/model for the JSON type, which would close an import
	// cycle. See plans/onyx-port.md, Tranche 1F.
	if err := rag.AutoMigrate(database); err != nil {
		return nil, fmt.Errorf("running RAG migrations: %w", err)
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
	// Explicit pool sizing: the SSE fan-out holds one blocked XREAD
	// connection per active conversation per pod. Default (10 * GOMAXPROCS)
	// starves under multi-tenant load. Callers can override via
	// REDIS_POOL_SIZE env var through config.
	if redisOpts.PoolSize == 0 {
		redisOpts.PoolSize = 500
	}
	if redisOpts.MinIdleConns == 0 {
		redisOpts.MinIdleConns = 20
	}
	if redisOpts.PoolTimeout == 0 {
		redisOpts.PoolTimeout = 4 * time.Second
	}
	redisClient := redis.NewClient(redisOpts)
	if err := redisClient.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("connecting to redis: %w", err)
	}
	slog.Info("redis ready",
		"pool_size", redisOpts.PoolSize,
		"min_idle_conns", redisOpts.MinIdleConns,
	)

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

	// 12b. Spider client (optional)
	var spiderClient *spider.Client
	var toolUsageWriter *middleware.ToolUsageWriter
	if cfg.SpiderAPIKey != "" {
		spiderClient = spider.NewClient(cfg.SpiderBaseURL, cfg.SpiderAPIKey)
		toolUsageWriter = middleware.NewToolUsageWriter(database, 10000)
		slog.Info("spider client ready")
	}

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
		agentPusher = sandbox.NewPusher(database, orchestrator, signingKey, cfg)
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

	// 17. Polar billing client (optional)
	var polarClient *polargo.Polar
	if cfg.PolarAccessToken != "" {
		server := polargo.ServerSandbox
		if cfg.PolarServer == "production" {
			server = polargo.ServerProduction
		}
		polarClient = polargo.New(
			polargo.WithSecurity(cfg.PolarAccessToken),
			polargo.WithServer(server),
		)
		slog.Info("polar billing client initialized", "server", cfg.PolarServer)
	}

	// 18. S3 storage (agent drive — optional)
	var s3Client *storage.S3Client
	if cfg.S3Bucket != "" {
		s3Client, err = storage.NewS3Client(cfg.S3Bucket, cfg.S3Region, cfg.S3Endpoint, cfg.S3AccessKey, cfg.S3SecretKey)
		if err != nil {
			return nil, fmt.Errorf("creating S3 client: %w", err)
		}
		slog.Info("s3 storage ready", "bucket", cfg.S3Bucket)
	}

	// 19. PostHog error tracking (optional — disabled when POSTHOG_ENABLED=false
	// or POSTHOG_API_KEY is empty). NewClient returns (nil, nil) in that case.
	// The caller (main.go) is responsible for wrapping the slog handler once
	// the service name is known (api vs worker vs proxy).
	postHogClient, err := posthogobs.NewClient(cfg, posthogobs.ClientOptions{
		ServiceName: "platform",
		Environment: cfg.Environment,
	})
	if err != nil {
		return nil, fmt.Errorf("initializing posthog client: %w", err)
	}
	if postHogClient != nil {
		slog.Info("posthog error tracking ready", "endpoint", cfg.PostHogEndpoint)
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
		SpiderClient:    spiderClient,
		ToolUsageWriter: toolUsageWriter,
		PolarClient:     polarClient,
		S3Client:        s3Client,
		PostHog:         postHogClient,
	}, nil
}

// Close releases all resources held by Deps. PostHog is closed LAST so it can
// capture any errors produced by the other subsystems shutting down.
func (d *Deps) Close() {
	d.CacheManager.Memory().Purge()
	if sqlDB, err := d.DB.DB(); err == nil {
		_ = sqlDB.Close()
	}
	_ = d.Redis.Close()
	posthogobs.Close(d.PostHog)
	slog.Info("deps closed")
}
