package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const keyPrefix = "pbcred:"

// RedisCredential is the L2 cache representation. Values remain encrypted —
// the API key is still DEK-encrypted and the DEK is still Vault-wrapped.
type RedisCredential struct {
	EncryptedKey []byte `json:"ek"`
	WrappedDEK   []byte `json:"wd"`
	BaseURL      string `json:"bu"`
	AuthScheme   string `json:"as"`
	OrgID        string `json:"oi"`
}

// RedisCache is the L2 Redis-backed credential cache.
type RedisCache struct {
	client *redis.Client
	ttl    time.Duration
}

// NewRedisCache creates a new Redis credential cache.
func NewRedisCache(client *redis.Client, ttl time.Duration) *RedisCache {
	return &RedisCache{client: client, ttl: ttl}
}

// Get retrieves a credential from Redis. Returns nil, nil on cache miss.
func (r *RedisCache) Get(ctx context.Context, credentialID string) (*RedisCredential, error) {
	data, err := r.client.Get(ctx, keyPrefix+credentialID).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("redis get: %w", err)
	}

	var cred RedisCredential
	if err := json.Unmarshal(data, &cred); err != nil {
		return nil, fmt.Errorf("redis unmarshal: %w", err)
	}
	return &cred, nil
}

// Set stores a credential in Redis with TTL.
func (r *RedisCache) Set(ctx context.Context, credentialID string, cred *RedisCredential) error {
	data, err := json.Marshal(cred)
	if err != nil {
		return fmt.Errorf("redis marshal: %w", err)
	}
	return r.client.Set(ctx, keyPrefix+credentialID, data, r.ttl).Err()
}

// Invalidate removes a credential from Redis.
func (r *RedisCache) Invalidate(ctx context.Context, credentialID string) error {
	return r.client.Del(ctx, keyPrefix+credentialID).Err()
}

// RevokedTokenCache manages token revocation state in Redis.
// Revoked JTIs are stored with a TTL matching the token's original expiry.
type RevokedTokenCache struct {
	client *redis.Client
}

const revokedPrefix = "pbrev:"

// NewRevokedTokenCache creates a cache for tracking revoked token JTIs.
func NewRevokedTokenCache(client *redis.Client) *RevokedTokenCache {
	return &RevokedTokenCache{client: client}
}

// MarkRevoked stores a revoked JTI in Redis with a TTL.
func (r *RevokedTokenCache) MarkRevoked(ctx context.Context, jti string, ttl time.Duration) error {
	return r.client.Set(ctx, revokedPrefix+jti, "1", ttl).Err()
}

// IsRevoked checks if a JTI has been marked as revoked in Redis.
func (r *RevokedTokenCache) IsRevoked(ctx context.Context, jti string) (bool, error) {
	exists, err := r.client.Exists(ctx, revokedPrefix+jti).Result()
	if err != nil {
		return false, fmt.Errorf("redis exists: %w", err)
	}
	return exists > 0, nil
}
