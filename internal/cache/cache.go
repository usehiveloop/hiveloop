package cache

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/awnumar/memguard"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"

	"github.com/useportal/llmvault/internal/crypto"
	"github.com/useportal/llmvault/internal/model"
)

// DecryptedCredential is the fully resolved, plaintext credential returned
// to callers of the cache manager.
type DecryptedCredential struct {
	APIKey     []byte
	BaseURL    string
	AuthScheme string
}

// Manager orchestrates the 3-tier cache: L1 (memory) → L2 (Redis) → L3 (Postgres + KMS).
type Manager struct {
	memory      *MemoryCache
	redisCache  *RedisCache
	dekCache    *DEKCache
	revokedTok  *RevokedTokenCache
	invalidator *Invalidator
	kms         *crypto.KeyWrapper
	db          *gorm.DB

	flight singleflight.Group

	// Hard expiry for L1 entries — absolute max time to serve from memory.
	hardExpiry time.Duration
}

// NewManager creates a cache manager wired to all three tiers.
func NewManager(
	memCache *MemoryCache,
	redisCache *RedisCache,
	dekCache *DEKCache,
	revokedTok *RevokedTokenCache,
	invalidator *Invalidator,
	kms *crypto.KeyWrapper,
	db *gorm.DB,
	hardExpiry time.Duration,
) *Manager {
	return &Manager{
		memory:      memCache,
		redisCache:  redisCache,
		dekCache:    dekCache,
		revokedTok:  revokedTok,
		invalidator: invalidator,
		kms:         kms,
		db:          db,
		hardExpiry:  hardExpiry,
	}
}

// GetDecryptedCredential resolves a credential through the 3-tier cache.
// L1 hit → ~0.01ms, L2 hit → ~0.5ms, L3 hit → ~3-8ms.
// Uses singleflight to deduplicate concurrent requests for the same credential.
func (m *Manager) GetDecryptedCredential(ctx context.Context, credentialID string, orgID uuid.UUID) (*DecryptedCredential, error) {
	// L1: Check in-memory cache
	if cached, ok := m.memory.Get(credentialID); ok && cached.OrgID == orgID {
		buf, err := cached.Enclave.Open()
		if err != nil {
			// Enclave corrupted — fall through to lower tiers
			m.memory.Invalidate(credentialID)
		} else {
			apiKey := make([]byte, buf.Size())
			copy(apiKey, buf.Bytes())
			buf.Destroy()
			return &DecryptedCredential{
				APIKey:     apiKey,
				BaseURL:    cached.BaseURL,
				AuthScheme: cached.AuthScheme,
			}, nil
		}
	}

	// Singleflight: collapse concurrent requests for same credential
	type result struct {
		cred *DecryptedCredential
		err  error
	}
	v, err, _ := m.flight.Do(credentialID, func() (any, error) {
		return m.resolveFromLowerTiers(ctx, credentialID, orgID)
	})
	if err != nil {
		return nil, err
	}
	return v.(*DecryptedCredential), nil
}

// resolveFromLowerTiers checks L2 then L3, promoting results upward.
func (m *Manager) resolveFromLowerTiers(ctx context.Context, credentialID string, orgID uuid.UUID) (*DecryptedCredential, error) {
	// L2: Check Redis
	redisCred, err := m.redisCache.Get(ctx, credentialID)
	if err != nil {
		// Redis error — fall through to L3 (graceful degradation)
		redisCred = nil
	}

	if redisCred != nil && redisCred.OrgID == orgID.String() {
		// Decrypt using DEK (may be cached in DEKCache)
		apiKey, err := m.decryptWithDEKCache(ctx, credentialID, redisCred.EncryptedKey, redisCred.WrappedDEK)
		if err != nil {
			// Decryption failed — fall through to L3
			_ = m.redisCache.Invalidate(ctx, credentialID)
		} else {
			cred := &DecryptedCredential{
				APIKey:     apiKey,
				BaseURL:    redisCred.BaseURL,
				AuthScheme: redisCred.AuthScheme,
			}
			// Promote to L1
			m.promoteToL1(credentialID, orgID, apiKey, redisCred.BaseURL, redisCred.AuthScheme)
			return cred, nil
		}
	}

	// L3: Postgres + KMS (cold path)
	return m.resolveFromDB(ctx, credentialID, orgID)
}

// resolveFromDB fetches from Postgres, decrypts via KMS, and promotes to L2 + L1.
func (m *Manager) resolveFromDB(ctx context.Context, credentialID string, orgID uuid.UUID) (*DecryptedCredential, error) {
	var dbCred model.Credential
	err := m.db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND revoked_at IS NULL", credentialID, orgID).
		First(&dbCred).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("credential not found or revoked")
		}
		return nil, fmt.Errorf("db lookup: %w", err)
	}

	// Unwrap DEK via KMS
	dek, err := m.kms.Unwrap(ctx, dbCred.WrappedDEK)
	if err != nil {
		return nil, fmt.Errorf("kms unwrap: %w", err)
	}

	// Decrypt API key
	apiKey, err := crypto.DecryptCredential(dbCred.EncryptedKey, dek)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}

	// Cache DEK in DEKCache
	dekEnclave := memguard.NewEnclave(dek)
	m.dekCache.Set(credentialID, dekEnclave)
	// Zero the plaintext DEK
	for i := range dek {
		dek[i] = 0
	}

	// Promote to L2 (Redis — still-encrypted values)
	_ = m.redisCache.Set(ctx, credentialID, &RedisCredential{
		EncryptedKey: dbCred.EncryptedKey,
		WrappedDEK:   dbCred.WrappedDEK,
		BaseURL:      dbCred.BaseURL,
		AuthScheme:   dbCred.AuthScheme,
		OrgID:        orgID.String(),
	})

	// Promote to L1 (memory — sealed plaintext)
	m.promoteToL1(credentialID, orgID, apiKey, dbCred.BaseURL, dbCred.AuthScheme)

	return &DecryptedCredential{
		APIKey:     apiKey,
		BaseURL:    dbCred.BaseURL,
		AuthScheme: dbCred.AuthScheme,
	}, nil
}

// decryptWithDEKCache decrypts an API key using a DEK from the DEK cache
// (or falls back to KMS unwrap if the DEK isn't cached).
func (m *Manager) decryptWithDEKCache(ctx context.Context, credentialID string, encryptedKey, wrappedDEK []byte) ([]byte, error) {
	// Try DEK cache first
	if enclave, ok := m.dekCache.Get(credentialID); ok {
		buf, err := enclave.Open()
		if err == nil {
			dek := make([]byte, buf.Size())
			copy(dek, buf.Bytes())
			buf.Destroy()

			apiKey, err := crypto.DecryptCredential(encryptedKey, dek)
			for i := range dek {
				dek[i] = 0
			}
			if err == nil {
				return apiKey, nil
			}
		}
		// DEK cache entry corrupted — invalidate and fall through
		m.dekCache.Invalidate(credentialID)
	}

	// DEK not cached — unwrap via KMS
	dek, err := m.kms.Unwrap(ctx, wrappedDEK)
	if err != nil {
		return nil, fmt.Errorf("kms unwrap: %w", err)
	}

	apiKey, err := crypto.DecryptCredential(encryptedKey, dek)
	if err != nil {
		for i := range dek {
			dek[i] = 0
		}
		return nil, fmt.Errorf("decrypt: %w", err)
	}

	// Cache the unwrapped DEK
	dekEnclave := memguard.NewEnclave(dek)
	m.dekCache.Set(credentialID, dekEnclave)
	for i := range dek {
		dek[i] = 0
	}

	return apiKey, nil
}

// promoteToL1 seals a copy of the plaintext API key in memguard and stores it in L1.
// NOTE: memguard.NewEnclave zeroes the source slice, so we copy first.
func (m *Manager) promoteToL1(credentialID string, orgID uuid.UUID, apiKey []byte, baseURL, authScheme string) {
	keyCopy := make([]byte, len(apiKey))
	copy(keyCopy, apiKey)
	enclave := memguard.NewEnclave(keyCopy)
	m.memory.Set(credentialID, &CachedCredential{
		Enclave:    enclave,
		BaseURL:    baseURL,
		AuthScheme: authScheme,
		OrgID:      orgID,
		CachedAt:   time.Now(),
		HardExpiry: time.Now().Add(m.hardExpiry),
	})
}

// InvalidateCredential removes a credential from all cache tiers and
// publishes an invalidation message for other instances.
func (m *Manager) InvalidateCredential(ctx context.Context, credentialID string) error {
	m.memory.Invalidate(credentialID)
	m.dekCache.Invalidate(credentialID)

	if err := m.redisCache.Invalidate(ctx, credentialID); err != nil {
		return fmt.Errorf("redis invalidate: %w", err)
	}

	return m.invalidator.PublishCredentialInvalidation(ctx, credentialID)
}

// InvalidateToken marks a token as revoked across all tiers.
func (m *Manager) InvalidateToken(ctx context.Context, jti string, ttl time.Duration) error {
	// Add to local in-memory set
	m.invalidator.revokedMu.Lock()
	m.invalidator.revokedSet[jti] = struct{}{}
	m.invalidator.revokedMu.Unlock()

	// Store in Redis with TTL
	if err := m.revokedTok.MarkRevoked(ctx, jti, ttl); err != nil {
		return fmt.Errorf("redis mark revoked: %w", err)
	}

	return m.invalidator.PublishTokenRevocation(ctx, jti)
}

// IsTokenRevoked checks all tiers for token revocation.
// L1 (in-memory set) → L2 (Redis) → L3 (Postgres).
func (m *Manager) IsTokenRevoked(ctx context.Context, jti string) (bool, error) {
	// L1: in-memory set
	if m.invalidator.IsTokenLocallyRevoked(jti) {
		return true, nil
	}

	// L2: Redis
	revoked, err := m.revokedTok.IsRevoked(ctx, jti)
	if err != nil {
		// Redis down — fall through to DB
		revoked = false
	}
	if revoked {
		// Promote to L1
		m.invalidator.revokedMu.Lock()
		m.invalidator.revokedSet[jti] = struct{}{}
		m.invalidator.revokedMu.Unlock()
		return true, nil
	}

	// L3: Postgres
	var count int64
	err = m.db.WithContext(ctx).
		Model(&model.Token{}).
		Where("jti = ? AND revoked_at IS NOT NULL", jti).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("db revocation check: %w", err)
	}
	if count > 0 {
		// Promote to L2 + L1
		_ = m.revokedTok.MarkRevoked(ctx, jti, 24*time.Hour)
		m.invalidator.revokedMu.Lock()
		m.invalidator.revokedSet[jti] = struct{}{}
		m.invalidator.revokedMu.Unlock()
		return true, nil
	}

	return false, nil
}

// Memory returns the L1 cache (for testing / metrics).
func (m *Manager) Memory() *MemoryCache { return m.memory }

// Invalidator returns the invalidator (for starting the subscription goroutine).
func (m *Manager) Invalidator() *Invalidator { return m.invalidator }

// --- Helper for building a complete Manager from config ---

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
func Build(cfg Config, redisClient *redis.Client, kms *crypto.KeyWrapper, db *gorm.DB) *Manager {
	memCache := NewMemoryCache(cfg.MemMaxSize, cfg.MemTTL)
	dekCache := NewDEKCache(cfg.DEKMaxSize, cfg.DEKTTL)
	redisCache := NewRedisCache(redisClient, cfg.RedisTTL)
	revokedTok := NewRevokedTokenCache(redisClient)
	invalidator := NewInvalidator(redisClient, memCache, dekCache)

	return NewManager(memCache, redisCache, dekCache, revokedTok, invalidator, kms, db, cfg.HardExpiry)
}

// ensure singleflight is used (compile-time check)
var _ sync.Locker = (*sync.Mutex)(nil)
