package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	// Environment
	Environment string `env:"ENVIRONMENT" envDefault:"development"` // "development" or "production"

	// Server
	Port      int    `env:"PORT,required"`
	LogLevel  string `env:"LOG_LEVEL,required"`
	LogFormat string `env:"LOG_FORMAT,required"`

	// Postgres
	DatabaseURL string `env:"DATABASE_URL,required"`

	// KMS (key wrapping for credential encryption)
	KMSType   string `env:"KMS_TYPE,required"` // "aead" or "awskms"
	KMSKey    string `env:"KMS_KEY"`           // base64-encoded 32-byte key (aead) or AWS KMS key ID/ARN (awskms)
	AWSRegion string `env:"AWS_REGION"`        // AWS region for awskms (default: us-east-1)

	// Redis
	RedisAddr     string        `env:"REDIS_ADDR,required"`
	RedisPassword string        `env:"REDIS_PASSWORD"`
	RedisDB       int           `env:"REDIS_DB,required"`
	RedisCacheTTL time.Duration `env:"REDIS_CACHE_TTL,required"`

	// L1 Cache (in-memory)
	MemCacheTTL     time.Duration `env:"MEM_CACHE_TTL,required"`
	MemCacheMaxSize int           `env:"MEM_CACHE_MAX_SIZE,required"`

	// JWT (for sandbox proxy tokens)
	JWTSigningKey string `env:"JWT_SIGNING_KEY,required"`

	// CORS
	CORSOrigins []string `env:"CORS_ORIGINS,required" envSeparator:","`

	// ZITADEL (Identity & Auth)
	ZitadelDomain       string `env:"ZITADEL_DOMAIN"`
	ZitadelClientID     string `env:"ZITADEL_CLIENT_ID"`
	ZitadelClientSecret string `env:"ZITADEL_CLIENT_SECRET"`
	ZitadelAdminPAT     string `env:"ZITADEL_ADMIN_PAT"`
	ZitadelProjectID    string `env:"ZITADEL_PROJECT_ID"`
}

func Load() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	// Enforce AWS KMS in production — AEAD is not allowed.
	if cfg.Environment == "production" && cfg.KMSType != "awskms" {
		return nil, fmt.Errorf("KMS_TYPE must be 'awskms' in production (got %q)", cfg.KMSType)
	}

	// Fall back to reading admin PAT from file (written by ZITADEL itself).
	if cfg.ZitadelAdminPAT == "" {
		cfg.loadZitadelPATFile()
	}

	return cfg, nil
}

func (c *Config) loadZitadelPATFile() {
	data, err := os.ReadFile("docker/zitadel/bootstrap/admin.pat")
	if err != nil {
		return
	}
	if pat := strings.TrimSpace(string(data)); pat != "" {
		c.ZitadelAdminPAT = pat
	}
}
