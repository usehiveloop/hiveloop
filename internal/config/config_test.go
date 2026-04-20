package config

import (
	"encoding/base64"
	"testing"
	"time"
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
	t.Setenv("AUTH_RSA_PRIVATE_KEY", base64.StdEncoding.EncodeToString([]byte("test-pem")))
	t.Setenv("FRONTEND_URL", "http://localhost:3000")
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
	t.Setenv("CORS_ORIGINS", "http://localhost:3000,https://app.hiveloop.com")

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
}

func TestLoad_AuthConfig_Defaults(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.AuthIssuer != "hiveloop" {
		t.Errorf("expected default AuthIssuer 'hiveloop', got %q", cfg.AuthIssuer)
	}
	if cfg.AuthAudience != "https://api.hiveloop.com" {
		t.Errorf("expected default AuthAudience 'https://api.hiveloop.com', got %q", cfg.AuthAudience)
	}
	if cfg.AuthAccessTokenTTL != 15*time.Minute {
		t.Errorf("expected default AuthAccessTokenTTL 15m, got %v", cfg.AuthAccessTokenTTL)
	}
	if cfg.AuthRefreshTokenTTL != 720*time.Hour {
		t.Errorf("expected default AuthRefreshTokenTTL 720h, got %v", cfg.AuthRefreshTokenTTL)
	}
}

func TestLoad_AuthConfig_CustomValues(t *testing.T) {
	setRequiredEnv(t)
	testPEM := base64.StdEncoding.EncodeToString([]byte("custom-test-pem"))
	t.Setenv("AUTH_RSA_PRIVATE_KEY", testPEM)
	t.Setenv("AUTH_ISSUER", "custom-issuer")
	t.Setenv("AUTH_AUDIENCE", "https://custom.example.com")
	t.Setenv("AUTH_ACCESS_TOKEN_TTL", "30m")
	t.Setenv("AUTH_REFRESH_TOKEN_TTL", "168h")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.AuthRSAPrivateKey != testPEM {
		t.Errorf("expected AuthRSAPrivateKey %q, got %q", testPEM, cfg.AuthRSAPrivateKey)
	}
	if cfg.AuthIssuer != "custom-issuer" {
		t.Errorf("expected AuthIssuer 'custom-issuer', got %q", cfg.AuthIssuer)
	}
	if cfg.AuthAudience != "https://custom.example.com" {
		t.Errorf("expected AuthAudience 'https://custom.example.com', got %q", cfg.AuthAudience)
	}
	if cfg.AuthAccessTokenTTL != 30*time.Minute {
		t.Errorf("expected AuthAccessTokenTTL 30m, got %v", cfg.AuthAccessTokenTTL)
	}
	if cfg.AuthRefreshTokenTTL != 168*time.Hour {
		t.Errorf("expected AuthRefreshTokenTTL 168h, got %v", cfg.AuthRefreshTokenTTL)
	}
}

func TestLoad_RequiredFieldsPopulated(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.DBHost == "" {
		t.Error("DB_HOST should not be empty")
	}
	if cfg.DatabaseDSN() == "" {
		t.Error("DatabaseDSN() should not be empty")
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

func TestLoad_RedisURL(t *testing.T) {
	setRequiredEnv(t)
	// Clear REDIS_ADDR — REDIS_URL should suffice
	t.Setenv("REDIS_ADDR", "")
	t.Setenv("REDIS_URL", "rediss://default:secret@redis.railway.internal:6379")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.RedisURL != "rediss://default:secret@redis.railway.internal:6379" {
		t.Errorf("unexpected REDIS_URL: %q", cfg.RedisURL)
	}
}

func TestLoad_NoRedisConfig(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("REDIS_ADDR", "")
	t.Setenv("REDIS_URL", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when neither REDIS_URL nor REDIS_ADDR is set")
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

func TestLoad_ProductionAllowsVault(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("ENVIRONMENT", "production")
	t.Setenv("KMS_TYPE", "vault")
	t.Setenv("VAULT_ADDRESS", "http://localhost:8200")
	t.Setenv("VAULT_TOKEN", "test-token")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.KMSType != "vault" {
		t.Errorf("expected KMS type 'vault', got %q", cfg.KMSType)
	}
	if cfg.VaultAddress != "http://localhost:8200" {
		t.Errorf("expected Vault address 'http://localhost:8200', got %q", cfg.VaultAddress)
	}
	if cfg.VaultToken != "test-token" {
		t.Errorf("expected Vault token 'test-token', got %q", cfg.VaultToken)
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

func TestLoad_VaultConfig(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("KMS_TYPE", "vault")
	t.Setenv("KMS_KEY", "my-encryption-key")
	t.Setenv("VAULT_ADDRESS", "http://vault:8200")
	t.Setenv("VAULT_TOKEN", "s.token")
	t.Setenv("VAULT_NAMESPACE", "my-namespace")
	t.Setenv("VAULT_MOUNT_PATH", "custom-transit")
	t.Setenv("VAULT_CA_CERT", "/path/to/ca.crt")
	t.Setenv("VAULT_CLIENT_CERT", "/path/to/client.crt")
	t.Setenv("VAULT_CLIENT_KEY", "/path/to/client.key")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Test VaultConfig() method
	vaultCfg := cfg.VaultConfig()
	if vaultCfg == nil {
		t.Fatal("expected VaultConfig to not be nil")
	}

	if vaultCfg.Address != "http://vault:8200" {
		t.Errorf("expected Vault address 'http://vault:8200', got %q", vaultCfg.Address)
	}
	if vaultCfg.Token != "s.token" {
		t.Errorf("expected Vault token 's.token', got %q", vaultCfg.Token)
	}
	if vaultCfg.Namespace != "my-namespace" {
		t.Errorf("expected Vault namespace 'my-namespace', got %q", vaultCfg.Namespace)
	}
	if vaultCfg.MountPath != "custom-transit" {
		t.Errorf("expected Vault mount path 'custom-transit', got %q", vaultCfg.MountPath)
	}
	if vaultCfg.KeyName != "my-encryption-key" {
		t.Errorf("expected Vault key name 'my-encryption-key', got %q", vaultCfg.KeyName)
	}
	if vaultCfg.CACert != "/path/to/ca.crt" {
		t.Errorf("expected CA cert '/path/to/ca.crt', got %q", vaultCfg.CACert)
	}
	if vaultCfg.ClientCert != "/path/to/client.crt" {
		t.Errorf("expected client cert '/path/to/client.crt', got %q", vaultCfg.ClientCert)
	}
	if vaultCfg.ClientKey != "/path/to/client.key" {
		t.Errorf("expected client key '/path/to/client.key', got %q", vaultCfg.ClientKey)
	}
}

func TestLoad_CORSOriginsOptional(t *testing.T) {
	setRequiredEnv(t)
	// Unset CORS_ORIGINS — should load successfully with empty slice
	t.Setenv("CORS_ORIGINS", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.CORSOrigins) != 0 {
		t.Errorf("expected empty CORS origins, got %v", cfg.CORSOrigins)
	}
}

func TestLoad_VaultConfig_ReturnsNilForNonVault(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("KMS_TYPE", "aead")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	vaultCfg := cfg.VaultConfig()
	if vaultCfg != nil {
		t.Error("expected VaultConfig to be nil when KMS_TYPE is not 'vault'")
	}
}
