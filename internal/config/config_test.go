package config

import (
	"testing"
)

func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("PORT", "8080")
	t.Setenv("LOG_LEVEL", "info")
	t.Setenv("LOG_FORMAT", "json")
	t.Setenv("DB_HOST", "localhost")
	t.Setenv("DB_USER", "user")
	t.Setenv("DB_PASSWORD", "pass")
	t.Setenv("DB_NAME", "db")
	t.Setenv("KMS_TYPE", "aead")
	t.Setenv("KMS_KEY", "dGVzdC1rZXktMzItYnl0ZXMtbG9uZy1lbm91Z2gh")
	t.Setenv("REDIS_ADDR", "localhost:6379")
	t.Setenv("REDIS_DB", "0")
	t.Setenv("REDIS_CACHE_TTL", "30m")
	// REDIS_URL is not set here — REDIS_ADDR is used as fallback for local dev
	t.Setenv("MEM_CACHE_TTL", "5m")
	t.Setenv("MEM_CACHE_MAX_SIZE", "10000")
	t.Setenv("JWT_SIGNING_KEY", "test-signing-key")
	t.Setenv("CORS_ORIGINS", "http://localhost:3000")
	t.Setenv("AUTH_RSA_PRIVATE_KEY", "dGVzdC1wZW0=")
	t.Setenv("FRONTEND_URL", "http://localhost:3000")
}

// TestLoad_NoRedisConfig tests error handling when neither REDIS_URL nor REDIS_ADDR is set.
// This is the only valuable test in this file as it tests actual error handling behavior.
// All other tests were removed as they test library configuration parsing behavior.
// See USELESS_TESTS_RECOMMENDATIONS.md for details.
func TestLoad_NoRedisConfig(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("REDIS_ADDR", "")
	t.Setenv("REDIS_URL", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when neither REDIS_URL nor REDIS_ADDR is set")
	}
}