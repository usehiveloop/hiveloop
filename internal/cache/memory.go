package cache

import (
	"time"

	"github.com/awnumar/memguard"
	"github.com/google/uuid"
	"github.com/hashicorp/golang-lru/v2/expirable"
)

// CachedCredential holds a decrypted credential in sealed, mlocked memory.
type CachedCredential struct {
	Enclave    *memguard.Enclave
	BaseURL    string
	AuthScheme string
	OrgID      uuid.UUID
	CachedAt   time.Time
	HardExpiry time.Time
}

// MemoryCache is an L1 in-memory LRU cache with TTL-based expiry.
// Values are sealed in memguard enclaves (mlocked, encrypted at rest in RAM).
type MemoryCache struct {
	lru *expirable.LRU[string, *CachedCredential]
}

// NewMemoryCache creates a new in-memory credential cache.
func NewMemoryCache(maxSize int, ttl time.Duration) *MemoryCache {
	onEvict := func(_ string, v *CachedCredential) {
		if v != nil && v.Enclave != nil {
			// Open and destroy to zero out memory
			if buf, err := v.Enclave.Open(); err == nil {
				buf.Destroy()
			}
		}
	}
	return &MemoryCache{
		lru: expirable.NewLRU[string, *CachedCredential](maxSize, onEvict, ttl),
	}
}

// Get retrieves a cached credential by ID. Returns nil, false on miss.
func (c *MemoryCache) Get(credentialID string) (*CachedCredential, bool) {
	entry, ok := c.lru.Get(credentialID)
	if !ok {
		return nil, false
	}
	if time.Now().After(entry.HardExpiry) {
		c.lru.Remove(credentialID)
		return nil, false
	}
	return entry, true
}

// Set stores a credential in the cache.
func (c *MemoryCache) Set(credentialID string, cred *CachedCredential) {
	c.lru.Add(credentialID, cred)
}

// Invalidate removes a single credential from the cache.
func (c *MemoryCache) Invalidate(credentialID string) {
	c.lru.Remove(credentialID)
}

// Purge removes all entries from the cache.
func (c *MemoryCache) Purge() {
	c.lru.Purge()
}

// Len returns the number of entries in the cache.
func (c *MemoryCache) Len() int {
	return c.lru.Len()
}

// DEKCache caches unwrapped DEKs in sealed memory keyed by credential ID.
// This avoids repeated KMS unwrap calls on L2 cache hits.
type DEKCache struct {
	lru *expirable.LRU[string, *memguard.Enclave]
}

// NewDEKCache creates a new DEK cache.
func NewDEKCache(maxSize int, ttl time.Duration) *DEKCache {
	onEvict := func(_ string, v *memguard.Enclave) {
		if v != nil {
			if buf, err := v.Open(); err == nil {
				buf.Destroy()
			}
		}
	}
	return &DEKCache{
		lru: expirable.NewLRU[string, *memguard.Enclave](maxSize, onEvict, ttl),
	}
}

// Get retrieves a cached DEK enclave.
func (d *DEKCache) Get(credentialID string) (*memguard.Enclave, bool) {
	return d.lru.Get(credentialID)
}

// Set stores a DEK enclave.
func (d *DEKCache) Set(credentialID string, enclave *memguard.Enclave) {
	d.lru.Add(credentialID, enclave)
}

// Invalidate removes a DEK from the cache.
func (d *DEKCache) Invalidate(credentialID string) {
	d.lru.Remove(credentialID)
}
