package config

import (
	"fmt"
	"net/url"
	"time"

	"github.com/caarlos0/env/v11"

	"github.com/llmvault/llmvault/internal/crypto"
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
	KMSType   string `env:"KMS_TYPE,required"` // "aead", "awskms", or "vault"
	KMSKey    string `env:"KMS_KEY"`           // base64-encoded 32-byte key (aead) or AWS KMS key ID/ARN (awskms) or Vault key name (vault)
	AWSRegion string `env:"AWS_REGION"`        // AWS region for awskms (default: us-east-1)

	// HashiCorp Vault (for KMS_TYPE=vault)
	VaultAddress   string `env:"VAULT_ADDRESS"`   // Vault server URL (e.g., http://localhost:8200)
	VaultToken     string `env:"VAULT_TOKEN"`     // Vault authentication token
	VaultNamespace string `env:"VAULT_NAMESPACE"` // Optional Vault Enterprise namespace
	VaultMountPath string `env:"VAULT_MOUNT_PATH"` // Transit engine mount path (default: transit)
	VaultCACert    string `env:"VAULT_CA_CERT"`   // Path to CA certificate (optional, for TLS)
	VaultClientCert string `env:"VAULT_CLIENT_CERT"` // Path to client certificate (optional, for TLS)
	VaultClientKey string `env:"VAULT_CLIENT_KEY"`   // Path to client key (optional, for TLS)

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
	AuthIssuer          string        `env:"AUTH_ISSUER" envDefault:"llmvault"`
	AuthAudience        string        `env:"AUTH_AUDIENCE" envDefault:"https://api.llmvault.dev"`
	AuthAccessTokenTTL  time.Duration `env:"AUTH_ACCESS_TOKEN_TTL" envDefault:"15m"`
	AuthRefreshTokenTTL time.Duration `env:"AUTH_REFRESH_TOKEN_TTL" envDefault:"720h"` // 30 days

	// Frontend (for building email links)
	FrontendURL string `env:"FRONTEND_URL" envDefault:"http://localhost:3000"`

	// Auth: auto-confirm email on registration (useful for self-hosted deployments)
	AutoConfirmEmail bool `env:"AUTO_CONFIRM_EMAIL" envDefault:"false"`

	// CORS
	CORSOrigins []string `env:"CORS_ORIGINS" envSeparator:","`

	// Nango (OAuth integration proxy)
	NangoEndpoint  string `env:"NANGO_ENDPOINT"`    // e.g. http://localhost:3004
	NangoSecretKey string `env:"NANGO_SECRET_KEY"`  // Nango secret key for API auth

	// MCP Server
	MCPPort    int    `env:"MCP_PORT" envDefault:"8081"`
	MCPBaseURL string `env:"MCP_BASE_URL" envDefault:"http://localhost:8081"`
}

func Load() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	// Enforce AWS KMS or Vault in production — AEAD is not allowed.
	if cfg.Environment == "production" && cfg.KMSType != "awskms" && cfg.KMSType != "vault" {
		return nil, fmt.Errorf("KMS_TYPE must be 'awskms' or 'vault' in production (got %q)", cfg.KMSType)
	}

	// Require at least one Redis connection method.
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

// VaultConfig returns a crypto.VaultConfig populated from the Config.
// Returns nil if KMS_TYPE is not "vault".
func (c *Config) VaultConfig() *crypto.VaultConfig {
	if c.KMSType != "vault" {
		return nil
	}
	return &crypto.VaultConfig{
		Address:         c.VaultAddress,
		Token:           c.VaultToken,
		Namespace:       c.VaultNamespace,
		MountPath:       c.VaultMountPath,
		KeyName:         c.KMSKey,
		CACert:          c.VaultCACert,
		ClientCert:      c.VaultClientCert,
		ClientKey:       c.VaultClientKey,
	}
}
