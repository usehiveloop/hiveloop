package config

import (
	"fmt"
	"net/url"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/hibiken/asynq"
)

type Config struct {
	// Environment
	Environment string `env:"ENVIRONMENT" envDefault:"development"` // "development" or "production"

	// Server
	Port      int    `env:"PORT,required"`
	LogLevel  string `env:"LOG_LEVEL,required"`
	LogFormat string `env:"LOG_FORMAT,required"`

	// Postgres
	DBHost     string `env:"DB_HOST,required"`
	DBPort     int    `env:"DB_PORT" envDefault:"5432"`
	DBUser     string `env:"DB_USER,required"`
	DBPassword string `env:"DB_PASSWORD,required"`
	DBName     string `env:"DB_NAME,required"`
	DBSSLMode  string `env:"DB_SSLMODE" envDefault:"disable"`

	// KMS (key wrapping for credential encryption)
	KMSType   string `env:"KMS_TYPE,required"` // "aead" or "awskms"
	KMSKey    string `env:"KMS_KEY"`           // base64-encoded 32-byte key (aead) or AWS KMS key ID/ARN (awskms)
	AWSRegion string `env:"AWS_REGION"`        // AWS region for awskms (default: us-east-1)

	// Redis
	RedisURL      string        `env:"REDIS_URL"`              // Full URL (e.g. rediss://...), enables TLS automatically
	RedisAddr     string        `env:"REDIS_ADDR"`             // Fallback: host:port (ignored when REDIS_URL is set)
	RedisPassword string        `env:"REDIS_PASSWORD"`
	RedisDB       int           `env:"REDIS_DB"`
	RedisCacheTTL time.Duration `env:"REDIS_CACHE_TTL,required"`

	// L1 Cache (in-memory)
	MemCacheTTL     time.Duration `env:"MEM_CACHE_TTL,required"`
	MemCacheMaxSize int           `env:"MEM_CACHE_MAX_SIZE,required"`

	// JWT (for sandbox proxy tokens)
	JWTSigningKey string `env:"JWT_SIGNING_KEY,required"`

	// Auth (RSA key for JWT signing)
	AuthRSAPrivateKey   string        `env:"AUTH_RSA_PRIVATE_KEY,required"` // base64-encoded PEM
	AuthIssuer          string        `env:"AUTH_ISSUER" envDefault:"hiveloop"`
	AuthAudience        string        `env:"AUTH_AUDIENCE" envDefault:"https://api.usehiveloop.com"`
	AuthAccessTokenTTL  time.Duration `env:"AUTH_ACCESS_TOKEN_TTL" envDefault:"15m"`
	AuthRefreshTokenTTL time.Duration `env:"AUTH_REFRESH_TOKEN_TTL" envDefault:"720h"` // 30 days

	// Frontend (for building email links and OAuth redirects)
	FrontendURL string `env:"FRONTEND_URL,required"`

	// Auth: auto-confirm email on registration (useful for self-hosted deployments)
	AutoConfirmEmail bool `env:"AUTO_CONFIRM_EMAIL" envDefault:"false"`

	// Kibamail (transactional email). When KibamailAPIKey is empty the
	// worker falls back to LogSender (emails appear in logs only).
	// FromAddress must be a bare email — Kibamail's API rejects RFC
	// "Name <email>" syntax with a 422 INVALID_FIELD_VALUE on `from`.
	// FromName is sent as the separate `fromName` top-level field so
	// recipients see "Display Name <email>" in their inbox.
	KibamailAPIKey      string `env:"KIBAMAIL_API_KEY"`
	KibamailFromAddress string `env:"KIBAMAIL_FROM_ADDRESS" envDefault:"betty@notifications.usehiveloop.com"`
	KibamailFromName    string `env:"KIBAMAIL_FROM_NAME" envDefault:"Betty from Hiveloop"`

	// OAuth (social login)
	OAuthGitHubClientID     string `env:"OAUTH_GITHUB_CLIENT_ID"`
	OAuthGitHubClientSecret string `env:"OAUTH_GITHUB_CLIENT_SECRET"`
	OAuthGoogleClientID     string `env:"OAUTH_GOOGLE_CLIENT_ID"`
	OAuthGoogleClientSecret string `env:"OAUTH_GOOGLE_CLIENT_SECRET"`
	OAuthXClientID          string `env:"OAUTH_X_CLIENT_ID"`
	OAuthXClientSecret      string `env:"OAUTH_X_CLIENT_SECRET"`

	// CORS
	CORSOrigins []string `env:"CORS_ORIGINS" envSeparator:","`

	// Nango (OAuth integration proxy)
	NangoEndpoint  string `env:"NANGO_ENDPOINT"`    // e.g. http://localhost:3004
	NangoSecretKey string `env:"NANGO_SECRET_KEY"`  // Nango secret key for API auth

	// GitHub API token used by the skill hydrator. Optional — raises the
	// anonymous rate limit from 60 req/hr to 5000 req/hr per token.
	GitHubToken string `env:"GITHUB_TOKEN"`

	// MCP Server
	MCPPort    int    `env:"MCP_PORT" envDefault:"8081"`
	MCPBaseURL string `env:"MCP_BASE_URL" envDefault:"http://localhost:8081"`

	// Turso (per-workspace libsql database provisioning)
	TursoAPIToken string `env:"TURSO_API_TOKEN"`
	TursoOrgSlug  string `env:"TURSO_ORG_SLUG"`
	TursoGroup    string `env:"TURSO_GROUP" envDefault:"default"`

	// Sandbox provider (global — one provider for the whole platform)
	SandboxEncryptionKey string `env:"SANDBOX_ENCRYPTION_KEY"` // base64-encoded 32-byte key for encrypting sandbox secrets (Bridge API keys)
	SandboxProviderID    string `env:"SANDBOX_PROVIDER_ID" envDefault:"daytona"` // "daytona"
	SandboxProviderURL string `env:"SANDBOX_PROVIDER_URL"`                     // e.g. https://app.daytona.io/api
	SandboxProviderKey string `env:"SANDBOX_PROVIDER_KEY"`                     // API key for the sandbox provider
	SandboxTarget      string `env:"SANDBOX_TARGET"`                           // provider-specific target/region

	// Bridge (agent runtime in sandboxes)
	BridgeBaseImagePrefix          string `env:"BRIDGE_BASE_IMAGE_PREFIX" envDefault:"hiveloop-bridge-0-10-0-small-v2"`                    // snapshot for shared/pool sandboxes
	BridgeBaseDedicatedImagePrefix string `env:"BRIDGE_BASE_DEDICATED_IMAGE_PREFIX" envDefault:"hiveloop-bridge-0-10-0-small-v2"` // snapshot for dedicated agent sandboxes
	BridgeHost            string `env:"BRIDGE_HOST"`                                                  // our external hostname for webhook URLs
	ProxyHost             string `env:"PROXY_HOST" envDefault:"proxy.hiveloop.com"`                   // LLM proxy hostname (proxy.hiveloop.com)

	// Hindsight (agent memory)
	HindsightAPIURL string `env:"HINDSIGHT_API_URL"` // e.g. http://hindsight.railway.internal:8888 — empty = memory disabled

	// Platform admin (comma-separated email allowlist)
	PlatformAdminEmails string `env:"PLATFORM_ADMIN_EMAILS"`

	// Custom preview domains
	PreviewCNAMETarget   string `env:"PREVIEW_CNAME_TARGET" envDefault:"preview-proxy.hiveloop.com"`
	InternalDomainSecret string `env:"INTERNAL_DOMAIN_SECRET"`  // shared secret for Gatekeeper + acme-dns proxy + Caddy admin proxy
	AcmeDNSAPIURL        string `env:"ACME_DNS_API_URL"`        // acme-dns registration API (e.g. https://acme-dns-api.daytona.hiveloop.com)
	CaddyAdminURL        string `env:"CADDY_ADMIN_URL"`         // Caddy admin API proxy (e.g. https://caddy-admin.daytona.hiveloop.com)

	// Spider (web crawling/search via spider.cloud)
	SpiderAPIKey  string `env:"SPIDER_CLOUD_API_KEY"`                                  // empty = spider disabled
	SpiderBaseURL string `env:"SPIDER_BASE_URL" envDefault:"https://api.spider.cloud"` // Spider.cloud API endpoint

	// S3 (agent drive storage — empty AWS_S3_BUCKET_NAME disables the drive)
	S3Bucket    string `env:"AWS_S3_BUCKET_NAME"`
	S3Region    string `env:"AWS_DEFAULT_REGION" envDefault:"us-east-1"`
	S3Endpoint  string `env:"AWS_ENDPOINT_URL"` // for MinIO / R2 / local dev
	S3AccessKey string `env:"AWS_ACCESS_KEY_ID"`
	S3SecretKey string `env:"AWS_SECRET_ACCESS_KEY"`

	// Public assets (avatars, org logos, generic public uploads). Empty
	// PUBLIC_ASSETS_S3_BUCKET disables the /v1/uploads/sign endpoint.
	PublicAssetsBucket    string        `env:"PUBLIC_ASSETS_S3_BUCKET"`
	PublicAssetsRegion    string        `env:"PUBLIC_ASSETS_S3_REGION" envDefault:"auto"`
	PublicAssetsEndpoint  string        `env:"PUBLIC_ASSETS_S3_ENDPOINT"`
	PublicAssetsAccessKey string        `env:"PUBLIC_ASSETS_ACCESS_KEY_ID"`
	PublicAssetsSecretKey string        `env:"PUBLIC_ASSETS_SECRET_ACCESS_KEY"`
	PublicAssetsBaseURL   string        `env:"PUBLIC_ASSETS_BASE_URL"`
	PublicAssetsSignTTL   time.Duration `env:"PUBLIC_ASSETS_SIGN_TTL" envDefault:"15m"`
	PublicAssetsUseACL    bool          `env:"PUBLIC_ASSETS_USE_ACL" envDefault:"false"`

	// Admin API (disabled by default — deploy a separate private instance with ADMIN_API_ENABLED=true)
	AdminAPIEnabled bool `env:"ADMIN_API_ENABLED" envDefault:"false"`

	// Sandbox defaults
	DedicatedSandboxGracePeriodMins int           `env:"DEDICATED_SANDBOX_GRACE_PERIOD_MINS" envDefault:"5"`
	SandboxResourceCheckInterval    time.Duration `env:"SANDBOX_RESOURCE_CHECK_INTERVAL" envDefault:"30m"`

	// Asynq worker
	WorkerHealthPort      int           `env:"WORKER_HEALTH_PORT" envDefault:"8090"`
	AsynqConcurrency      int           `env:"ASYNQ_CONCURRENCY" envDefault:"30"`
	AsynqShutdownTimeout  time.Duration `env:"ASYNQ_SHUTDOWN_TIMEOUT" envDefault:"120s"`

	// PostHog error tracking (empty POSTHOG_API_KEY disables capture)
	PostHogAPIKey   string `env:"POSTHOG_API_KEY"`
	PostHogEndpoint string `env:"POSTHOG_ENDPOINT" envDefault:"https://us.i.posthog.com"`
	PostHogEnabled  bool   `env:"POSTHOG_ENABLED" envDefault:"false"`

	// Qdrant (vector store, gRPC). Empty QdrantHost disables RAG.
	QdrantHost       string `env:"QDRANT_HOST"`
	QdrantPort       int    `env:"QDRANT_PORT" envDefault:"6334"`
	QdrantUseTLS     bool   `env:"QDRANT_USE_TLS" envDefault:"false"`
	QdrantAPIKey     string `env:"QDRANT_API_KEY"`
	QdrantCollection string `env:"QDRANT_COLLECTION" envDefault:"rag_chunks_3072"`

	// Embedder (OpenAI-compatible).
	LLMAPIURL       string `env:"LLM_API_URL"`
	LLMAPIKey       string `env:"LLM_API_KEY"`
	LLMModel        string `env:"LLM_MODEL"`
	LLMEmbeddingDim uint32 `env:"LLM_EMBEDDING_DIM" envDefault:"3072"`

	// Reranker (Cohere-compatible via OpenRouter).
	RerankerBaseURL string `env:"RERANKER_BASE_URL"`
	RerankerAPIKey  string `env:"RERANKER_API_KEY"`
	RerankerModel   string `env:"RERANKER_MODEL"`

	RagBatchSize int `env:"RAG_BATCH_SIZE" envDefault:"100"`

	// Paystack (billing provider). Empty PaystackSecretKey disables the
	// provider — the billing registry simply won't include it and checkout
	// for NGN plans will fail fast with ErrUnknownProvider. Plan codes
	// (PLN_xxx) live on the plans table (provider_plan_id column) — run
	// `make setup-paystack` to seed them from Paystack's API.
	PaystackSecretKey string `env:"PAYSTACK_SECRET_KEY"`
}

func Load() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	if cfg.KMSType != "aead" && cfg.KMSType != "awskms" {
		return nil, fmt.Errorf("KMS_TYPE must be 'aead' or 'awskms' (got %q)", cfg.KMSType)
	}

	if cfg.RedisURL == "" && cfg.RedisAddr == "" {
		return nil, fmt.Errorf("either REDIS_URL or REDIS_ADDR must be set")
	}

	return cfg, nil
}

// DatabaseDSN constructs a Postgres connection string from individual fields.
// The password is URL-encoded to handle special characters safely.
func (c *Config) DatabaseDSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		url.QueryEscape(c.DBUser),
		url.QueryEscape(c.DBPassword),
		c.DBHost,
		c.DBPort,
		c.DBName,
		c.DBSSLMode,
	)
}

// AsynqRedisOpt returns an asynq.RedisConnOpt from the Redis config fields.
func (c *Config) AsynqRedisOpt() asynq.RedisConnOpt {
	if c.RedisURL != "" {
		opt, err := asynq.ParseRedisURI(c.RedisURL)
		if err == nil {
			return opt
		}
	}
	return asynq.RedisClientOpt{
		Addr:     c.RedisAddr,
		Password: c.RedisPassword,
		DB:       c.RedisDB,
	}
}
