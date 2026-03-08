package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	// Server
	Port      int    `env:"PORT" envDefault:"8080"`
	LogLevel  string `env:"LOG_LEVEL" envDefault:"info"`
	LogFormat string `env:"LOG_FORMAT" envDefault:"json"`

	// Postgres
	DatabaseURL string `env:"DATABASE_URL,required"`

	// Vault
	VaultAddr    string `env:"VAULT_ADDR,required"`
	VaultToken   string `env:"VAULT_TOKEN,required"`
	VaultKeyName string `env:"VAULT_KEY_NAME" envDefault:"proxy-bridge-master"`

	// Redis
	RedisAddr     string        `env:"REDIS_ADDR,required"`
	RedisPassword string        `env:"REDIS_PASSWORD" envDefault:""`
	RedisDB       int           `env:"REDIS_DB" envDefault:"0"`
	RedisCacheTTL time.Duration `env:"REDIS_CACHE_TTL" envDefault:"30m"`

	// L1 Cache (in-memory)
	MemCacheTTL     time.Duration `env:"MEM_CACHE_TTL" envDefault:"5m"`
	MemCacheMaxSize int           `env:"MEM_CACHE_MAX_SIZE" envDefault:"10000"`

	// JWT (for sandbox proxy tokens)
	JWTSigningKey string `env:"JWT_SIGNING_KEY,required"`

	// ZITADEL (Identity & Auth)
	ZitadelDomain       string `env:"ZITADEL_DOMAIN" envDefault:"http://localhost:8085"`
	ZitadelClientID     string `env:"ZITADEL_CLIENT_ID"`
	ZitadelClientSecret string `env:"ZITADEL_CLIENT_SECRET"`
	ZitadelCredsFile    string `env:"ZITADEL_CREDS_FILE" envDefault:"./docker/zitadel/bootstrap/api-credentials.json"`
}

func Load() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	return cfg, nil
}
