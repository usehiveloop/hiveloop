package counter

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/useportal/llmvault/internal/model"
)

// Counter manages atomic request-cap counters in Redis with lazy refill from Postgres.
type Counter struct {
	rdb *redis.Client
	db  *gorm.DB
}

// New creates a Counter backed by the given Redis and Postgres connections.
func New(rdb *redis.Client, db *gorm.DB) *Counter {
	return &Counter{rdb: rdb, db: db}
}

// credKey returns the Redis key for a credential counter.
func credKey(credentialID string) string {
	return "pbreq:cred:" + credentialID
}

// tokKey returns the Redis key for a token counter.
func tokKey(jti string) string {
	return "pbreq:tok:" + jti
}

// Seed initializes the Redis counter for a credential or token.
// If ttl > 0 the key will expire after the given duration.
func (c *Counter) Seed(ctx context.Context, key string, value int64, ttl time.Duration) error {
	if ttl > 0 {
		return c.rdb.Set(ctx, key, value, ttl).Err()
	}
	return c.rdb.Set(ctx, key, value, 0).Err()
}

// SeedCredential seeds the counter for a credential.
func (c *Counter) SeedCredential(ctx context.Context, credentialID string, value int64) error {
	return c.Seed(ctx, credKey(credentialID), value, 0)
}

// SeedToken seeds the counter for a token. The TTL is token expiry + 1 minute buffer.
func (c *Counter) SeedToken(ctx context.Context, jti string, value int64, tokenTTL time.Duration) error {
	return c.Seed(ctx, tokKey(jti), value, tokenTTL+time.Minute)
}

// decrementScript atomically decrements a key by 1 if it exists and is > 0.
// Returns:
//
//	1  = success (decremented)
//	0  = exhausted (counter is 0)
//	-1 = key does not exist (no cap configured)
var decrementScript = redis.NewScript(`
local v = redis.call("GET", KEYS[1])
if v == false then
    return -1
end
local n = tonumber(v)
if n <= 0 then
    return 0
end
redis.call("DECR", KEYS[1])
return 1
`)

// DecrementResult indicates the outcome of a Decrement call.
type DecrementResult int

const (
	DecrOK        DecrementResult = 1  // Decremented successfully
	DecrExhausted DecrementResult = 0  // Counter is at 0
	DecrNoCap     DecrementResult = -1 // No cap configured (key missing)
)

// Decrement atomically decrements the counter for the given key.
func (c *Counter) Decrement(ctx context.Context, key string) (DecrementResult, error) {
	val, err := decrementScript.Run(ctx, c.rdb, []string{key}).Int()
	if err != nil {
		return DecrNoCap, err
	}
	return DecrementResult(val), nil
}

// Undo increments the counter by 1 (used to roll back a token decrement when the
// credential counter rejects the request).
func (c *Counter) Undo(ctx context.Context, key string) error {
	return c.rdb.Incr(ctx, key).Err()
}

// Read returns the current counter value. Returns -1 if the key does not exist.
func (c *Counter) Read(ctx context.Context, key string) (int64, error) {
	val, err := c.rdb.Get(ctx, key).Int64()
	if err == redis.Nil {
		return -1, nil
	}
	return val, err
}

// Delete removes the counter key.
func (c *Counter) Delete(ctx context.Context, key string) error {
	return c.rdb.Del(ctx, key).Err()
}

// CredKey returns the Redis key for a credential counter.
func CredKey(credentialID string) string { return credKey(credentialID) }

// TokKey returns the Redis key for a token counter.
func TokKey(jti string) string { return tokKey(jti) }

// CheckAndRefillCredential checks whether a credential's counter needs refill
// and performs a lazy refill using Postgres optimistic locking.
// Returns true if a refill was performed.
func (c *Counter) CheckAndRefillCredential(ctx context.Context, credentialID string) (bool, error) {
	var cred model.Credential
	if err := c.db.Where("id = ?", credentialID).First(&cred).Error; err != nil {
		return false, fmt.Errorf("loading credential: %w", err)
	}

	if cred.RefillAmount == nil || cred.RefillInterval == nil {
		return false, nil
	}

	interval, err := time.ParseDuration(*cred.RefillInterval)
	if err != nil {
		return false, fmt.Errorf("invalid refill_interval %q: %w", *cred.RefillInterval, err)
	}

	now := time.Now()
	lastRefill := cred.CreatedAt
	if cred.LastRefillAt != nil {
		lastRefill = *cred.LastRefillAt
	}

	if now.Sub(lastRefill) < interval {
		return false, nil // not yet due
	}

	// Optimistic locking: only update if last_refill_at matches what we read
	result := c.db.Model(&model.Credential{}).
		Where("id = ? AND (last_refill_at = ? OR (last_refill_at IS NULL AND ? = ?))",
			credentialID, lastRefill, lastRefill, cred.CreatedAt).
		Updates(map[string]any{
			"remaining":      *cred.RefillAmount,
			"last_refill_at": now,
		})

	if result.Error != nil {
		return false, fmt.Errorf("updating credential: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return false, nil // another instance did the refill
	}

	// Reset Redis counter
	if err := c.SeedCredential(ctx, credentialID, *cred.RefillAmount); err != nil {
		return true, fmt.Errorf("reseeding redis: %w", err)
	}

	return true, nil
}

// CheckAndRefillToken performs the same lazy refill for token counters.
func (c *Counter) CheckAndRefillToken(ctx context.Context, jti string) (bool, error) {
	var tok model.Token
	if err := c.db.Where("jti = ?", jti).First(&tok).Error; err != nil {
		return false, fmt.Errorf("loading token: %w", err)
	}

	if tok.RefillAmount == nil || tok.RefillInterval == nil {
		return false, nil
	}

	interval, err := time.ParseDuration(*tok.RefillInterval)
	if err != nil {
		return false, fmt.Errorf("invalid refill_interval %q: %w", *tok.RefillInterval, err)
	}

	now := time.Now()
	lastRefill := tok.CreatedAt
	if tok.LastRefillAt != nil {
		lastRefill = *tok.LastRefillAt
	}

	if now.Sub(lastRefill) < interval {
		return false, nil
	}

	result := c.db.Model(&model.Token{}).
		Where("jti = ? AND (last_refill_at = ? OR (last_refill_at IS NULL AND ? = ?))",
			jti, lastRefill, lastRefill, tok.CreatedAt).
		Updates(map[string]any{
			"remaining":      *tok.RefillAmount,
			"last_refill_at": now,
		})

	if result.Error != nil {
		return false, fmt.Errorf("updating token: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return false, nil
	}

	ttlRemaining := time.Until(tok.ExpiresAt) + time.Minute
	if ttlRemaining < 0 {
		ttlRemaining = time.Minute
	}
	if err := c.SeedToken(ctx, jti, *tok.RefillAmount, ttlRemaining); err != nil {
		return true, fmt.Errorf("reseeding redis: %w", err)
	}

	return true, nil
}
