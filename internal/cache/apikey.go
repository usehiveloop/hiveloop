package cache

import (
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/golang-lru/v2/expirable"
)

// CachedAPIKey holds the minimal data needed for API key auth lookups.
type CachedAPIKey struct {
	ID        uuid.UUID
	OrgID     uuid.UUID
	Scopes    []string
	ExpiresAt *time.Time
}

// APIKeyCache is an in-memory LRU cache for API key auth lookups, keyed by key hash.
type APIKeyCache struct {
	lru *expirable.LRU[string, *CachedAPIKey]
}

// NewAPIKeyCache creates a new API key cache.
func NewAPIKeyCache(maxSize int, ttl time.Duration) *APIKeyCache {
	return &APIKeyCache{
		lru: expirable.NewLRU[string, *CachedAPIKey](maxSize, nil, ttl),
	}
}

// Get retrieves a cached API key by hash. Returns nil, false on miss or if the key has expired.
func (c *APIKeyCache) Get(keyHash string) (*CachedAPIKey, bool) {
	entry, ok := c.lru.Get(keyHash)
	if !ok {
		return nil, false
	}
	if entry.ExpiresAt != nil && time.Now().After(*entry.ExpiresAt) {
		c.lru.Remove(keyHash)
		return nil, false
	}
	return entry, true
}

// Set stores an API key in the cache.
func (c *APIKeyCache) Set(keyHash string, key *CachedAPIKey) {
	c.lru.Add(keyHash, key)
}

// Invalidate removes a single API key from the cache.
func (c *APIKeyCache) Invalidate(keyHash string) {
	c.lru.Remove(keyHash)
}

// Purge removes all entries from the cache.
func (c *APIKeyCache) Purge() {
	c.lru.Purge()
}
