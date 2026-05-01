package bootstrap

import (
	"context"
	"crypto/rsa"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/auth"
	"github.com/usehiveloop/hiveloop/internal/billing"
	"github.com/usehiveloop/hiveloop/internal/billing/paystack"
	"github.com/usehiveloop/hiveloop/internal/billing/subscription"
	"github.com/usehiveloop/hiveloop/internal/cache"
	"github.com/usehiveloop/hiveloop/internal/config"
	"github.com/usehiveloop/hiveloop/internal/counter"
	"github.com/usehiveloop/hiveloop/internal/credentials"
	"github.com/usehiveloop/hiveloop/internal/crypto"
	"github.com/usehiveloop/hiveloop/internal/db"
	"github.com/usehiveloop/hiveloop/internal/hindsight"
	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/mcp/catalog"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/nango"
	"github.com/usehiveloop/hiveloop/internal/registry"
	"github.com/usehiveloop/hiveloop/internal/sandbox"
	"github.com/usehiveloop/hiveloop/internal/sandbox/daytona"
	"github.com/usehiveloop/hiveloop/internal/spider"
	"github.com/usehiveloop/hiveloop/internal/storage"
	"github.com/usehiveloop/hiveloop/internal/streaming"
	"github.com/usehiveloop/hiveloop/internal/turso"

	sentryobs "github.com/usehiveloop/hiveloop/internal/observability/sentry"
)

// Deps holds all shared dependencies initialized during bootstrap.
// Both the API server and the Asynq worker use this struct.
type Deps struct {
	Config          *config.Config
	DB              *gorm.DB
	Redis           *redis.Client
	KMS             *crypto.KeyWrapper
	CacheManager    *cache.Manager
	APIKeyCache     *cache.APIKeyCache
	Counter         *counter.Counter
	NangoClient     *nango.Client
	Registry        *registry.Registry
	ActionsCatalog  *catalog.Catalog
	RSAKey          *rsa.PrivateKey
	SigningKey      []byte
	SandboxEncKey   *crypto.SymmetricKey
	Orchestrator    *sandbox.Orchestrator
	AgentPusher     *sandbox.Pusher
	EventBus        *streaming.EventBus
	Flusher         *streaming.Flusher
	Cleanup         *streaming.Cleanup
	Retainer        *hindsight.Retainer         // nil if Hindsight not configured
	SpiderClient    *spider.Client              // nil if spider not configured
	ToolUsageWriter *middleware.ToolUsageWriter // nil if spider not configured
	BillingRegistry *billing.Registry           // always non-nil; may have zero providers
	Credits         *billing.CreditsService     // credit ledger service
	Subscriptions   *subscription.Service       // wraps registry+credits with the renewal worker
	S3Client        *storage.S3Client           // nil if AWS_S3_BUCKET_NAME not set
}

// New initializes all shared dependencies. The caller is responsible for
// closing resources via Deps.Close().
func New(ctx context.Context) (*Deps, error) {

	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}

	if err := sentryobs.Init(cfg, sentryobs.ClientOptions{
		ServiceName: "platform",
		Environment: cfg.Environment,
	}); err != nil {
		return nil, fmt.Errorf("initializing sentry: %w", err)
	}

	database, err := db.New(ctx, cfg.DatabaseDSN())
	if err != nil {
		return nil, fmt.Errorf("connecting to database: %w", err)
	}
	if err := sentryobs.InstallGORMPlugin(database); err != nil {
		return nil, fmt.Errorf("installing sentry gorm plugin: %w", err)
	}
	if err := model.AutoMigrate(database); err != nil {
		return nil, fmt.Errorf("running migrations: %w", err)
	}

	if err := credentials.SeedPlatformOrg(database); err != nil {
		return nil, fmt.Errorf("seeding platform org: %w", err)
	}

	credentialPicker := credentials.NewPicker(database)
	logging.FromContext(ctx).InfoContext(ctx, "database ready")

	var kms *crypto.KeyWrapper
	switch cfg.KMSType {
	case "aead":
		kms, err = crypto.NewAEADWrapper(ctx, cfg.KMSKey, "aead-local")
	case "awskms":
		kms, err = crypto.NewAWSKMSWrapper(ctx, cfg.KMSKey, cfg.AWSRegion)
	default:
		return nil, fmt.Errorf("unsupported KMS_TYPE: %q (supported: aead, awskms)", cfg.KMSType)
	}
	if err != nil {
		return nil, fmt.Errorf("creating %s KMS wrapper: %w", cfg.KMSType, err)
	}
	logging.FromContext(ctx).InfoContext(ctx, "kms wrapper ready", "type", cfg.KMSType)

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
	sentryobs.InstallRedisHook(redisClient)
	if err := redisClient.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("connecting to redis: %w", err)
	}
	logging.FromContext(ctx).InfoContext(ctx, "redis ready",
		"pool_size", redisOpts.PoolSize,
		"min_idle_conns", redisOpts.MinIdleConns,
	)

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
	logging.FromContext(ctx).InfoContext(ctx, "cache manager ready")

	ctr := counter.New(redisClient, database)
	logging.FromContext(ctx).InfoContext(ctx, "request counter ready")

	signingKey := []byte(cfg.JWTSigningKey)

	rsaKey, err := auth.LoadRSAPrivateKey(cfg.AuthRSAPrivateKey)
	if err != nil {
		return nil, fmt.Errorf("loading auth RSA key: %w", err)
	}
	logging.FromContext(ctx).InfoContext(ctx, "embedded auth ready")

	reg := registry.Global()
	logging.FromContext(ctx).InfoContext(ctx, "provider registry ready", "providers", reg.ProviderCount(), "models", reg.ModelCount())

	if cfg.NangoEndpoint == "" || cfg.NangoSecretKey == "" {
		return nil, fmt.Errorf("NANGO_ENDPOINT and NANGO_SECRET_KEY are required")
	}
	nangoClient := nango.NewClient(cfg.NangoEndpoint, cfg.NangoSecretKey)
	if err := nangoClient.FetchProviders(ctx); err != nil {
		return nil, fmt.Errorf("fetching Nango provider catalog: %w", err)
	}
	logging.FromContext(ctx).InfoContext(ctx, "nango client ready", "providers", len(nangoClient.GetProviders()))

	actionsCatalog := catalog.Global()
	logging.FromContext(ctx).InfoContext(ctx, "actions catalog ready", "providers", len(actionsCatalog.ListProviders()))

	var spiderClient *spider.Client
	var toolUsageWriter *middleware.ToolUsageWriter
	if cfg.SpiderAPIKey != "" {
		spiderClient = spider.NewClient(cfg.SpiderBaseURL, cfg.SpiderAPIKey)
		toolUsageWriter = middleware.NewToolUsageWriter(ctx, database, 10000)
		logging.FromContext(ctx).InfoContext(ctx, "spider client ready")
	}

	var sandboxEncKey *crypto.SymmetricKey
	if cfg.SandboxEncryptionKey != "" {
		sandboxEncKey, err = crypto.NewSymmetricKey(cfg.SandboxEncryptionKey)
		if err != nil {
			return nil, fmt.Errorf("invalid SANDBOX_ENCRYPTION_KEY: %w", err)
		}
	}

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
			logging.FromContext(ctx).InfoContext(ctx, "turso provisioner ready")
		}

		orchestrator = sandbox.NewOrchestrator(database, sandboxProvider, tursoProvisioner, sandboxEncKey, cfg)
		agentPusher = sandbox.NewPusher(database, orchestrator, signingKey, cfg, credentialPicker)
		logging.FromContext(ctx).InfoContext(ctx, "sandbox orchestrator ready")
	}

	eventBus := streaming.NewEventBus(redisClient)
	flusher := streaming.NewFlusher(eventBus, database)
	cleanup := streaming.NewCleanup(eventBus)

	var retainer *hindsight.Retainer
	if cfg.HindsightAPIURL != "" {
		hClient := hindsight.NewClient(cfg.HindsightAPIURL)
		retainer = hindsight.NewRetainer(eventBus, database, hClient)
	}

	billingRegistry := billing.NewRegistry()
	credits := billing.NewCreditsService(database)
	if cfg.PaystackSecretKey != "" {
		billingRegistry.Register(paystack.New(paystack.Config{
			SecretKey: cfg.PaystackSecretKey,
		}))
		logging.FromContext(ctx).InfoContext(ctx, "paystack provider registered")
	}
	logging.FromContext(ctx).InfoContext(ctx, "billing ready", "providers", billingRegistry.Names())

	var s3Client *storage.S3Client
	if cfg.S3Bucket != "" {
		s3Client, err = storage.NewS3Client(cfg.S3Bucket, cfg.S3Region, cfg.S3Endpoint, cfg.S3AccessKey, cfg.S3SecretKey)
		if err != nil {
			return nil, fmt.Errorf("creating S3 client: %w", err)
		}
		logging.FromContext(ctx).InfoContext(ctx, "s3 storage ready", "bucket", cfg.S3Bucket)
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
		SigningKey:      signingKey,
		SandboxEncKey:   sandboxEncKey,
		Orchestrator:    orchestrator,
		AgentPusher:     agentPusher,
		EventBus:        eventBus,
		Flusher:         flusher,
		Cleanup:         cleanup,
		Retainer:        retainer,
		SpiderClient:    spiderClient,
		ToolUsageWriter: toolUsageWriter,
		BillingRegistry: billingRegistry,
		Credits:         credits,
		Subscriptions:   subscription.NewService(database, billingRegistry, credits),
		S3Client:        s3Client,
	}, nil
}

// Close releases all resources held by Deps. Sentry is flushed LAST so it
// can capture any errors produced by the other subsystems shutting down.
func (d *Deps) Close(ctx context.Context) {
	d.CacheManager.Memory().Purge()
	if sqlDB, err := d.DB.DB(); err == nil {
		_ = sqlDB.Close()
	}
	_ = d.Redis.Close()
	sentryobs.Close()
	logging.FromContext(ctx).InfoContext(ctx, "deps closed")
}
