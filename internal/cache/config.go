package cache

import (
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/crypto"
)

// Memory returns the L1 cache (for testing / metrics).
func (m *Manager) Memory() *MemoryCache { return m.memory }

// Invalidator returns the invalidator (for starting the subscription goroutine).
func (m *Manager) Invalidator() *Invalidator { return m.invalidator }

// Config holds all parameters needed to construct a cache Manager.
type Config struct {
	// L1
	MemMaxSize int
	MemTTL     time.Duration

	// L2
	RedisTTL time.Duration

	// DEK cache
	DEKMaxSize int
	DEKTTL     time.Duration

	// Hard expiry for L1 entries
	HardExpiry time.Duration
}

// DefaultConfig returns sensible defaults for the cache.
func DefaultConfig() Config {
	return Config{
		MemMaxSize: 10000,
		MemTTL:     5 * time.Minute,
		RedisTTL:   30 * time.Minute,
		DEKMaxSize: 1000,
		DEKTTL:     30 * time.Minute,
		HardExpiry: 15 * time.Minute,
	}
}

// Build constructs a fully wired cache Manager.
func Build(cfg Config, redisClient *redis.Client, kms *crypto.KeyWrapper, db *gorm.DB, apiKeyCache *APIKeyCache) *Manager {
	memCache := NewMemoryCache(cfg.MemMaxSize, cfg.MemTTL)
	dekCache := NewDEKCache(cfg.DEKMaxSize, cfg.DEKTTL)
	redisCache := NewRedisCache(redisClient, cfg.RedisTTL)
	revokedTok := NewRevokedTokenCache(redisClient)
	invalidator := NewInvalidator(redisClient, memCache, dekCache, apiKeyCache)

	return NewManager(memCache, redisCache, dekCache, revokedTok, invalidator, kms, db, cfg.HardExpiry)
}
