package config

import (
	"testing"
)

func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("HIVY_PORT", "8080")
	t.Setenv("HIVY_LOG_LEVEL", "info")
	t.Setenv("HIVY_LOG_FORMAT", "json")
	t.Setenv("HIVY_DB_HOST", "localhost")
	t.Setenv("HIVY_DB_USER", "user")
	t.Setenv("HIVY_DB_PASSWORD", "pass")
	t.Setenv("HIVY_DB_NAME", "db")
	t.Setenv("HIVY_KMS_TYPE", "aead")
	t.Setenv("HIVY_KMS_KEY", "dGVzdC1rZXktMzItYnl0ZXMtbG9uZy1lbm91Z2gh")
	t.Setenv("HIVY_REDIS_ADDR", "localhost:6379")
	t.Setenv("HIVY_REDIS_DB", "0")
	t.Setenv("HIVY_REDIS_CACHE_TTL", "30m")
	// HIVY_REDIS_URL is not set here — HIVY_REDIS_ADDR is used as fallback for local dev
	t.Setenv("HIVY_MEM_CACHE_TTL", "5m")
	t.Setenv("HIVY_MEM_CACHE_MAX_SIZE", "10000")
	t.Setenv("HIVY_JWT_SIGNING_KEY", "test-signing-key")
	t.Setenv("HIVY_CORS_ORIGINS", "http://localhost:3000")
	t.Setenv("HIVY_AUTH_RSA_PRIVATE_KEY", "dGVzdC1wZW0=")
	t.Setenv("HIVY_FRONTEND_URL", "http://localhost:3000")
}

// TestLoad_NoRedisConfig tests error handling when neither HIVY_REDIS_URL nor HIVY_REDIS_ADDR is set.
// This is the only valuable test in this file as it tests actual error handling behavior.
// All other tests were removed as they test library configuration parsing behavior.
// See USELESS_TESTS_RECOMMENDATIONS.md for details.
func TestLoad_NoRedisConfig(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("HIVY_REDIS_ADDR", "")
	t.Setenv("HIVY_REDIS_URL", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when neither HIVY_REDIS_URL nor HIVY_REDIS_ADDR is set")
	}
}
