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
	Environment string `env:"HIVY_ENVIRONMENT" envDefault:"development"` // "development" or "production"

	// Server
	Port      int    `env:"HIVY_PORT,required"`
	LogLevel  string `env:"HIVY_LOG_LEVEL,required"`
	LogFormat string `env:"HIVY_LOG_FORMAT,required"`

	// Postgres
	DBHost     string `env:"HIVY_DB_HOST,required"`
	DBPort     int    `env:"HIVY_DB_PORT" envDefault:"5432"`
	DBUser     string `env:"HIVY_DB_USER,required"`
	DBPassword string `env:"HIVY_DB_PASSWORD,required"`
	DBName     string `env:"HIVY_DB_NAME,required"`
	DBSSLMode  string `env:"HIVY_DB_SSLMODE" envDefault:"disable"`

	// KMS (key wrapping for credential encryption)
	KMSType   string `env:"HIVY_KMS_TYPE,required"` // "aead" or "awskms"
	KMSKey    string `env:"HIVY_KMS_KEY"`           // base64-encoded 32-byte key (aead) or AWS KMS key ID/ARN (awskms)
	AWSRegion string `env:"HIVY_AWS_REGION"`        // AWS region for awskms (default: us-east-1)

	// Redis
	RedisURL      string        `env:"HIVY_REDIS_URL"`  // Full URL (e.g. rediss://...), enables TLS automatically
	RedisAddr     string        `env:"HIVY_REDIS_ADDR"` // Fallback: host:port (ignored when HIVY_REDIS_URL is set)
	RedisPassword string        `env:"HIVY_REDIS_PASSWORD"`
	RedisDB       int           `env:"HIVY_REDIS_DB"`
	RedisCacheTTL time.Duration `env:"HIVY_REDIS_CACHE_TTL,required"`

	// L1 Cache (in-memory)
	MemCacheTTL     time.Duration `env:"HIVY_MEM_CACHE_TTL,required"`
	MemCacheMaxSize int           `env:"HIVY_MEM_CACHE_MAX_SIZE,required"`

	// JWT (for sandbox proxy tokens)
	JWTSigningKey string `env:"HIVY_JWT_SIGNING_KEY,required"`

	// Auth (RSA key for JWT signing)
	AuthRSAPrivateKey   string        `env:"HIVY_AUTH_RSA_PRIVATE_KEY,required"` // base64-encoded PEM
	AuthIssuer          string        `env:"HIVY_AUTH_ISSUER" envDefault:"hivy"`
	AuthAudience        string        `env:"HIVY_AUTH_AUDIENCE" envDefault:"https://api.usehivy.com"`
	AuthAccessTokenTTL  time.Duration `env:"HIVY_AUTH_ACCESS_TOKEN_TTL" envDefault:"15m"`
	AuthRefreshTokenTTL time.Duration `env:"HIVY_AUTH_REFRESH_TOKEN_TTL" envDefault:"720h"` // 30 days

	// Frontend (for building email links and OAuth redirects)
	FrontendURL string `env:"HIVY_FRONTEND_URL,required"`

	// Auth: auto-confirm email on registration (useful for self-hosted deployments)
	AutoConfirmEmail bool `env:"HIVY_AUTO_CONFIRM_EMAIL" envDefault:"false"`

	// Kibamail (transactional email). When KibamailAPIKey is empty the
	// worker falls back to LogSender (emails appear in logs only).
	// FromAddress must be a bare email — Kibamail's API rejects RFC
	// "Name <email>" syntax with a 422 INVALID_FIELD_VALUE on `from`.
	// FromName is sent as the separate `fromName` top-level field so
	// recipients see "Display Name <email>" in their inbox.
	KibamailAPIKey      string `env:"HIVY_KIBAMAIL_API_KEY"`
	KibamailFromAddress string `env:"HIVY_KIBAMAIL_FROM_ADDRESS" envDefault:"betty@notifications.usehivy.com"`
	KibamailFromName    string `env:"HIVY_KIBAMAIL_FROM_NAME" envDefault:"Betty from Hivy"`

	// OAuth (social login)
	OAuthGitHubClientID     string `env:"HIVY_OAUTH_GITHUB_CLIENT_ID"`
	OAuthGitHubClientSecret string `env:"HIVY_OAUTH_GITHUB_CLIENT_SECRET"`
	OAuthGoogleClientID     string `env:"HIVY_OAUTH_GOOGLE_CLIENT_ID"`
	OAuthGoogleClientSecret string `env:"HIVY_OAUTH_GOOGLE_CLIENT_SECRET"`
	OAuthXClientID          string `env:"HIVY_OAUTH_X_CLIENT_ID"`
	OAuthXClientSecret      string `env:"HIVY_OAUTH_X_CLIENT_SECRET"`

	// CORS
	CORSOrigins []string `env:"HIVY_CORS_ORIGINS" envSeparator:","`

	// Nango (OAuth integration proxy)
	NangoEndpoint  string `env:"HIVY_NANGO_ENDPOINT"`   // e.g. http://localhost:3004
	NangoSecretKey string `env:"HIVY_NANGO_SECRET_KEY"` // Nango secret key for API auth

	// Slack app-level token used by employee sandboxes for Socket Mode.
	SlackAppToken string `env:"HIVY_SLACK_APP_TOKEN"`

	// GitHub API token used by the skill hydrator. Optional — raises the
	// anonymous rate limit from 60 req/hr to 5000 req/hr per token.
	GitHubToken string `env:"HIVY_GITHUB_TOKEN"`

	// MCP Server
	MCPPort    int    `env:"HIVY_MCP_PORT" envDefault:"8081"`
	MCPBaseURL string `env:"HIVY_MCP_BASE_URL" envDefault:"http://localhost:8081"`

	// Sandbox provider (global — one provider for the whole platform)
	SandboxEncryptionKey              string `env:"HIVY_SANDBOX_ENCRYPTION_KEY"`                   // base64-encoded 32-byte key for encrypting sandbox secrets (Bridge API keys)
	SandboxProviderID                 string `env:"HIVY_SANDBOX_PROVIDER_ID" envDefault:"daytona"` // startup-time sandbox provider adapter
	SandboxDockerHost                 string `env:"HIVY_SANDBOX_DOCKER_HOST"`
	SandboxDockerPublicHost           string `env:"HIVY_SANDBOX_DOCKER_PUBLIC_HOST"`
	SandboxDockerContainerLabelPrefix string `env:"HIVY_SANDBOX_DOCKER_CONTAINER_LABEL_PREFIX" envDefault:"hivy"`

	// Daytona sandbox provider.
	DaytonaAPIURL string `env:"HIVY_DAYTONA_API_URL"`
	DaytonaAPIKey string `env:"HIVY_DAYTONA_API_KEY"`
	DaytonaTarget string `env:"HIVY_DAYTONA_TARGET"`

	// Cloud agent sandbox runtime.
	CloudAgentsSandboxBaseImagePrefix string `env:"HIVY_CLOUD_AGENTS_SANDBOX_BASE_IMAGE_PREFIX" envDefault:"hivy-bridge-1-0-0-small-v1"` // provider template/image for cloud agent sandboxes
	CloudAgentsSandboxRuntimeVersion  string `env:"HIVY_CLOUD_AGENTS_SANDBOX_RUNTIME_VERSION,required"`                                  // usehivy/hivy release tag installed into user templates (e.g. v1.0.0)
	CloudAgentsSandboxHost            string `env:"HIVY_CLOUD_AGENTS_SANDBOX_HOST"`                                                      // public control-plane host reachable from cloud agent sandboxes
	APIWebhookBaseURL                 string `env:"HIVY_API_WEBHOOK_BASE_URL" envDefault:"https://api.usehivy.com"`                      // public API base URL for provider webhook callbacks
	ProxyHost                         string `env:"HIVY_PROXY_HOST" envDefault:"proxy.usehivy.com"`                                      // LLM proxy hostname (proxy.usehivy.com)

	// Employee sandbox runtime — ghcr.io/usehivy/employee-sandbox image.
	EmployeeSandboxBaseImagePrefix string `env:"HIVY_EMPLOYEE_SANDBOX_BASE_IMAGE_PREFIX" envDefault:"hivy-employee-sandbox-0-0-3-small-v1"`

	// Hindsight (agent memory)
	HindsightAPIURL string `env:"HIVY_HINDSIGHT_API_URL"` // e.g. http://hindsight.railway.internal:8888 — empty = memory disabled

	// Platform admin (comma-separated email allowlist)
	PlatformAdminEmails string `env:"HIVY_PLATFORM_ADMIN_EMAILS"`

	// Custom preview domains
	PreviewCNAMETarget   string `env:"HIVY_PREVIEW_CNAME_TARGET" envDefault:"preview-proxy.usehivy.com"`
	InternalDomainSecret string `env:"HIVY_INTERNAL_DOMAIN_SECRET"` // shared secret for Gatekeeper + acme-dns proxy + Caddy admin proxy
	AcmeDNSAPIURL        string `env:"HIVY_ACME_DNS_API_URL"`       // acme-dns registration API (e.g. https://acme-dns-api.daytona.usehivy.com)
	CaddyAdminURL        string `env:"HIVY_CADDY_ADMIN_URL"`        // Caddy admin API proxy (e.g. https://caddy-admin.daytona.usehivy.com)

	// Spider (web crawling/search via spider.cloud)
	SpiderAPIKey  string `env:"HIVY_SPIDER_CLOUD_API_KEY"`                                  // empty = spider disabled
	SpiderBaseURL string `env:"HIVY_SPIDER_BASE_URL" envDefault:"https://api.spider.cloud"` // Spider.cloud API endpoint

	// S3 (agent drive storage — empty HIVY_AWS_S3_BUCKET_NAME disables the drive)
	S3Bucket                     string `env:"HIVY_AWS_S3_BUCKET_NAME"`
	S3Region                     string `env:"HIVY_AWS_DEFAULT_REGION" envDefault:"us-east-1"`
	S3Endpoint                   string `env:"HIVY_AWS_ENDPOINT_URL"` // for MinIO / R2 / local dev
	S3AccessKey                  string `env:"HIVY_AWS_ACCESS_KEY_ID"`
	S3SecretKey                  string `env:"HIVY_AWS_SECRET_ACCESS_KEY"`
	EmployeeSQLiteBackupMaxBytes int64  `env:"HIVY_EMPLOYEE_SQLITE_BACKUP_MAX_BYTES" envDefault:"5368709120"`

	// Public assets (avatars, org logos, generic public uploads). Empty
	// HIVY_PUBLIC_ASSETS_S3_BUCKET disables the /v1/uploads/sign endpoint.
	PublicAssetsBucket    string        `env:"HIVY_PUBLIC_ASSETS_S3_BUCKET"`
	PublicAssetsRegion    string        `env:"HIVY_PUBLIC_ASSETS_S3_REGION" envDefault:"auto"`
	PublicAssetsEndpoint  string        `env:"HIVY_PUBLIC_ASSETS_S3_ENDPOINT"`
	PublicAssetsAccessKey string        `env:"HIVY_PUBLIC_ASSETS_ACCESS_KEY_ID"`
	PublicAssetsSecretKey string        `env:"HIVY_PUBLIC_ASSETS_SECRET_ACCESS_KEY"`
	PublicAssetsBaseURL   string        `env:"HIVY_PUBLIC_ASSETS_BASE_URL"`
	PublicAssetsSignTTL   time.Duration `env:"HIVY_PUBLIC_ASSETS_SIGN_TTL" envDefault:"15m"`
	PublicAssetsUseACL    bool          `env:"HIVY_PUBLIC_ASSETS_USE_ACL" envDefault:"false"`

	// Sandbox defaults
	CloudAgentSandboxGracePeriodMins int           `env:"HIVY_CLOUD_AGENT_SANDBOX_GRACE_PERIOD_MINS" envDefault:"5"`
	SandboxResourceCheckInterval     time.Duration `env:"HIVY_SANDBOX_RESOURCE_CHECK_INTERVAL" envDefault:"30m"`

	// Asynq worker
	WorkerHealthPort     int           `env:"HIVY_WORKER_HEALTH_PORT" envDefault:"8090"`
	AsynqConcurrency     int           `env:"HIVY_ASYNQ_CONCURRENCY" envDefault:"30"`
	AsynqShutdownTimeout time.Duration `env:"HIVY_ASYNQ_SHUTDOWN_TIMEOUT" envDefault:"120s"`

	// Sentry error tracking + distributed tracing (empty HIVY_SENTRY_DSN disables
	// capture). When enabled, the SDK is wired into chi (HTTP transactions),
	// asynq (per-task transactions), GORM (db.sql spans), go-redis (db.redis
	// spans), outbound HTTP transports, and slog (Error+ records become
	// Sentry events). See internal/observability/sentry.
	SentryDSN                string  `env:"HIVY_SENTRY_DSN"`
	SentryEnabled            bool    `env:"HIVY_SENTRY_ENABLED" envDefault:"false"`
	SentryRelease            string  `env:"HIVY_SENTRY_RELEASE"`
	SentryTracesSampleRate   float64 `env:"HIVY_SENTRY_TRACES_SAMPLE_RATE" envDefault:"0.1"`
	SentryProfilesSampleRate float64 `env:"HIVY_SENTRY_PROFILES_SAMPLE_RATE" envDefault:"0.0"`
	EmployeeSandboxSentryDSN string  `env:"HIVY_EMPLOYEE_SANDBOX_SENTRY_DSN"`
	AgentSandboxSentryDSN    string  `env:"HIVY_AGENT_SANDBOX_SENTRY_DSN"`

	// Qdrant (vector store, gRPC). Empty QdrantHost disables RAG.
	QdrantHost       string `env:"HIVY_QDRANT_HOST"`
	QdrantPort       int    `env:"HIVY_QDRANT_PORT" envDefault:"6334"`
	QdrantUseTLS     bool   `env:"HIVY_QDRANT_USE_TLS" envDefault:"false"`
	QdrantAPIKey     string `env:"HIVY_QDRANT_API_KEY"`
	QdrantCollection string `env:"HIVY_QDRANT_COLLECTION" envDefault:"rag_chunks_3072"`

	// Embedder (OpenAI-compatible).
	LLMAPIURL       string `env:"HIVY_LLM_API_URL"`
	LLMAPIKey       string `env:"HIVY_LLM_API_KEY"`
	LLMModel        string `env:"HIVY_LLM_MODEL"`
	LLMEmbeddingDim uint32 `env:"HIVY_LLM_EMBEDDING_DIM" envDefault:"3072"`

	// Reranker (Cohere-compatible via OpenRouter).
	RerankerBaseURL string `env:"HIVY_RERANKER_BASE_URL"`
	RerankerAPIKey  string `env:"HIVY_RERANKER_API_KEY"`
	RerankerModel   string `env:"HIVY_RERANKER_MODEL"`

	RagBatchSize int `env:"HIVY_RAG_BATCH_SIZE" envDefault:"100"`

	// Paystack (billing provider). Empty PaystackSecretKey disables the
	// provider — the billing registry simply won't include it and checkout
	// for NGN plans will fail fast with ErrUnknownProvider. Plan codes
	// (PLN_xxx) live on the plans table (provider_plan_id column) — run
	// `make setup-paystack` to seed them from Paystack's API.
	PaystackSecretKey string `env:"HIVY_PAYSTACK_SECRET_KEY"`
}

func Load() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	if cfg.KMSType != "aead" && cfg.KMSType != "awskms" {
		return nil, fmt.Errorf("HIVY_KMS_TYPE must be 'aead' or 'awskms' (got %q)", cfg.KMSType)
	}

	if cfg.RedisURL == "" && cfg.RedisAddr == "" {
		return nil, fmt.Errorf("either HIVY_REDIS_URL or HIVY_REDIS_ADDR must be set")
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
