package system

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/redis/go-redis/v9"
)

const cacheKeyPrefix = "system:task:cache:"

// Cache is the storage interface for cached task results. Backed by Redis in
// production; tests inject an in-memory implementation.
type Cache interface {
	Get(ctx context.Context, key string) (*CompletionResult, bool, error)
	Set(ctx context.Context, key string, val *CompletionResult, ttl time.Duration) error
}

// CacheKey derives a stable cache key from the (task identity, resolved
// model, args). Bumping task.Version invalidates the entire entry set for
// that task without touching unrelated keys.
//
// Args are canonicalised to sorted-key JSON so {"a":1,"b":2} and
// {"b":2,"a":1} hash to the same key.
func CacheKey(task Task, model string, args map[string]any) (string, error) {
	canonical, err := canonicaliseArgs(args)
	if err != nil {
		return "", fmt.Errorf("canonicalise args: %w", err)
	}
	h := sha256.New()
	h.Write([]byte(task.Name))
	h.Write([]byte{0})
	h.Write([]byte(task.Version))
	h.Write([]byte{0})
	h.Write([]byte(model))
	h.Write([]byte{0})
	h.Write(canonical)
	return cacheKeyPrefix + hex.EncodeToString(h.Sum(nil)), nil
}

// RedisCache implements Cache against go-redis.
type RedisCache struct {
	client *redis.Client
}

// NewRedisCache constructs a redis-backed cache. Pass nil to disable caching
// (Get always misses, Set is a no-op). The disabled path keeps callers from
// having to nil-check at the call site.
func NewRedisCache(client *redis.Client) *RedisCache {
	return &RedisCache{client: client}
}

// Get returns the cached result for key, or (_, false, nil) on miss. A miss
// is not an error — callers proceed to upstream.
func (c *RedisCache) Get(ctx context.Context, key string) (*CompletionResult, bool, error) {
	if c == nil || c.client == nil {
		return nil, false, nil
	}
	raw, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, false, nil
		}
		return nil, false, err
	}
	var v CompletionResult
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, false, fmt.Errorf("decode cache value: %w", err)
	}
	return &v, true, nil
}

// Set stores val under key with the given TTL. ttl <= 0 is a no-op.
func (c *RedisCache) Set(ctx context.Context, key string, val *CompletionResult, ttl time.Duration) error {
	if c == nil || c.client == nil || ttl <= 0 {
		return nil
	}
	raw, err := json.Marshal(val)
	if err != nil {
		return fmt.Errorf("encode cache value: %w", err)
	}
	if err := c.client.Set(ctx, key, raw, ttl).Err(); err != nil {
		return fmt.Errorf("redis SET: %w", err)
	}
	return nil
}

func canonicaliseArgs(args map[string]any) ([]byte, error) {
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	ordered := make([][2]any, len(keys))
	for i, k := range keys {
		ordered[i] = [2]any{k, args[k]}
	}
	return json.Marshal(ordered)
}
