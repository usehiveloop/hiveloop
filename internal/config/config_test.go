package config

import (
	"testing"
	"time"
)

func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("PORT", "8080")
	t.Setenv("LOG_LEVEL", "info")
	t.Setenv("LOG_FORMAT", "json")
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/db")
	t.Setenv("KMS_TYPE", "aead")
	t.Setenv("KMS_KEY", "dGVzdC1rZXktMzItYnl0ZXMtbG9uZy1lbm91Z2gh")
	t.Setenv("REDIS_ADDR", "localhost:6379")
	t.Setenv("REDIS_DB", "0")
	t.Setenv("REDIS_CACHE_TTL", "30m")
	t.Setenv("MEM_CACHE_TTL", "5m")
	t.Setenv("MEM_CACHE_MAX_SIZE", "10000")
	t.Setenv("JWT_SIGNING_KEY", "test-signing-key")
	t.Setenv("CORS_ORIGINS", "http://localhost:3000")
}

func TestLoad_AllRequired(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Port != 8080 {
		t.Errorf("expected port 8080, got %d", cfg.Port)
	}
	if cfg.KMSType != "aead" {
		t.Errorf("expected KMS type 'aead', got %q", cfg.KMSType)
	}
	if cfg.MemCacheTTL != 5*time.Minute {
		t.Errorf("expected mem cache TTL 5m, got %v", cfg.MemCacheTTL)
	}
	if cfg.MemCacheMaxSize != 10000 {
		t.Errorf("expected mem cache max size 10000, got %d", cfg.MemCacheMaxSize)
	}
	if cfg.RedisCacheTTL != 30*time.Minute {
		t.Errorf("expected redis cache TTL 30m, got %v", cfg.RedisCacheTTL)
	}
	if cfg.RedisDB != 0 {
		t.Errorf("expected redis DB 0, got %d", cfg.RedisDB)
	}
	if cfg.RedisPassword != "" {
		t.Errorf("expected empty redis password, got %q", cfg.RedisPassword)
	}
	if cfg.CORSOrigins[0] != "http://localhost:3000" {
		t.Errorf("expected CORS origin 'http://localhost:3000', got %q", cfg.CORSOrigins[0])
	}
}

func TestLoad_CustomValues(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("PORT", "9090")
	t.Setenv("MEM_CACHE_TTL", "10m")
	t.Setenv("MEM_CACHE_MAX_SIZE", "5000")
	t.Setenv("REDIS_CACHE_TTL", "1h")
	t.Setenv("REDIS_DB", "2")
	t.Setenv("REDIS_PASSWORD", "secret")
	t.Setenv("CORS_ORIGINS", "http://localhost:3000,https://app.llmvault.dev")
	t.Setenv("ZITADEL_DOMAIN", "https://auth.llmvault.dev")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Port != 9090 {
		t.Errorf("expected port 9090, got %d", cfg.Port)
	}
	if cfg.KMSType != "aead" {
		t.Errorf("expected KMS type 'aead', got %q", cfg.KMSType)
	}
	if cfg.MemCacheTTL != 10*time.Minute {
		t.Errorf("expected mem cache TTL 10m, got %v", cfg.MemCacheTTL)
	}
	if cfg.MemCacheMaxSize != 5000 {
		t.Errorf("expected mem cache max size 5000, got %d", cfg.MemCacheMaxSize)
	}
	if cfg.RedisCacheTTL != time.Hour {
		t.Errorf("expected redis cache TTL 1h, got %v", cfg.RedisCacheTTL)
	}
	if cfg.RedisDB != 2 {
		t.Errorf("expected redis DB 2, got %d", cfg.RedisDB)
	}
	if cfg.RedisPassword != "secret" {
		t.Errorf("expected redis password 'secret', got %q", cfg.RedisPassword)
	}
	if len(cfg.CORSOrigins) != 2 {
		t.Errorf("expected 2 CORS origins, got %d", len(cfg.CORSOrigins))
	}
	if cfg.ZitadelDomain != "https://auth.llmvault.dev" {
		t.Errorf("expected ZITADEL domain 'https://auth.llmvault.dev', got %q", cfg.ZitadelDomain)
	}
}

func TestLoad_RequiredFieldsPopulated(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.DatabaseURL == "" {
		t.Error("DATABASE_URL should not be empty")
	}
	if cfg.KMSType == "" {
		t.Error("KMS_TYPE should not be empty")
	}
	if cfg.RedisAddr == "" {
		t.Error("REDIS_ADDR should not be empty")
	}
	if cfg.JWTSigningKey == "" {
		t.Error("JWT_SIGNING_KEY should not be empty")
	}
}

func TestLoad_ProductionRejectsAEAD(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("ENVIRONMENT", "production")
	t.Setenv("KMS_TYPE", "aead")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error: AEAD should be rejected in production")
	}
}

func TestLoad_ProductionAllowsAWSKMS(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("ENVIRONMENT", "production")
	t.Setenv("KMS_TYPE", "awskms")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.KMSType != "awskms" {
		t.Errorf("expected KMS type 'awskms', got %q", cfg.KMSType)
	}
}

func TestLoad_DevelopmentAllowsAEAD(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("ENVIRONMENT", "development")
	t.Setenv("KMS_TYPE", "aead")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.KMSType != "aead" {
		t.Errorf("expected KMS type 'aead', got %q", cfg.KMSType)
	}
}
