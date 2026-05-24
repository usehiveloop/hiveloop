package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/awnumar/memguard"
	"github.com/google/uuid"
	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/model"
)

// DecryptedCredential is the fully resolved, plaintext credential returned
// to callers of the cache manager.
type DecryptedCredential struct {
	APIKey     []byte
	BaseURL    string
	AuthScheme string
	ProviderID string
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

// GetDecryptedCredentialByID looks up a credential by id alone, reading the
// owning org from the row.
func (m *Manager) GetDecryptedCredentialByID(ctx context.Context, credentialID string) (*DecryptedCredential, error) {
	var orgIDOnly struct {
		OrgID *uuid.UUID `gorm:"column:org_id"`
	}
	if err := m.db.WithContext(ctx).
		Table("credentials").
		Select("org_id").
		Where("id = ? AND revoked_at IS NULL", credentialID).
		Take(&orgIDOnly).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("credential not found or revoked")
		}
		return nil, fmt.Errorf("db lookup: %w", err)
	}
	if orgIDOnly.OrgID == nil {
		return m.GetDecryptedCredential(ctx, credentialID, uuid.Nil)
	}
	return m.GetDecryptedCredential(ctx, credentialID, *orgIDOnly.OrgID)
}

// GetDecryptedCredential resolves a credential through the 3-tier cache.
// L1 hit → ~0.01ms, L2 hit → ~0.5ms, L3 hit → ~3-8ms.
// Uses singleflight to deduplicate concurrent requests for the same credential.
func (m *Manager) GetDecryptedCredential(ctx context.Context, credentialID string, orgID uuid.UUID) (*DecryptedCredential, error) {

	if cached, ok := m.memory.Get(credentialID); ok && cached.OrgID == orgID {
		buf, err := cached.Enclave.Open()
		if err != nil {

			m.memory.Invalidate(credentialID)
		} else {
			apiKey := make([]byte, buf.Size())
			copy(apiKey, buf.Bytes())
			buf.Destroy()
			return &DecryptedCredential{
				APIKey:     apiKey,
				BaseURL:    cached.BaseURL,
				AuthScheme: cached.AuthScheme,
				ProviderID: cached.ProviderID,
			}, nil
		}
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

	redisCred, err := m.redisCache.Get(ctx, credentialID)
	if err != nil {

		redisCred = nil
	}

	if redisCred != nil && redisCred.OrgID == orgID.String() {

		apiKey, err := m.decryptWithDEKCache(ctx, credentialID, redisCred.EncryptedKey, redisCred.WrappedDEK)
		if err != nil {

			_ = m.redisCache.Invalidate(ctx, credentialID)
		} else {
			cred := &DecryptedCredential{
				APIKey:     apiKey,
				BaseURL:    redisCred.BaseURL,
				AuthScheme: redisCred.AuthScheme,
				ProviderID: redisCred.ProviderID,
			}

			m.promoteToL1(credentialID, orgID, apiKey, redisCred.BaseURL, redisCred.AuthScheme, redisCred.ProviderID)
			return cred, nil
		}
	}

	return m.resolveFromDB(ctx, credentialID, orgID)
}

// resolveFromDB fetches from Postgres, decrypts via KMS, and promotes to L2 + L1.
func (m *Manager) resolveFromDB(ctx context.Context, credentialID string, orgID uuid.UUID) (*DecryptedCredential, error) {
	var dbCred model.Credential
	query := m.db.WithContext(ctx).Where("id = ? AND revoked_at IS NULL", credentialID)
	if orgID == uuid.Nil {
		query = query.Where("org_id IS NULL")
	} else {
		query = query.Where("org_id = ?", orgID)
	}
	err := query.First(&dbCred).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("credential not found or revoked")
		}
		return nil, fmt.Errorf("db lookup: %w", err)
	}

	dek, err := m.kms.Unwrap(ctx, dbCred.WrappedDEK)
	if err != nil {
		return nil, fmt.Errorf("kms unwrap: %w", err)
	}

	apiKey, err := crypto.DecryptCredential(dbCred.EncryptedKey, dek)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}

	dekEnclave := memguard.NewEnclave(dek)
	m.dekCache.Set(credentialID, dekEnclave)

	for i := range dek {
		dek[i] = 0
	}

	_ = m.redisCache.Set(ctx, credentialID, &RedisCredential{
		EncryptedKey: dbCred.EncryptedKey,
		WrappedDEK:   dbCred.WrappedDEK,
		BaseURL:      dbCred.BaseURL,
		AuthScheme:   dbCred.AuthScheme,
		ProviderID:   dbCred.ProviderID,
		OrgID:        orgID.String(),
	})

	m.promoteToL1(credentialID, orgID, apiKey, dbCred.BaseURL, dbCred.AuthScheme, dbCred.ProviderID)

	return &DecryptedCredential{
		APIKey:     apiKey,
		BaseURL:    dbCred.BaseURL,
		AuthScheme: dbCred.AuthScheme,
		ProviderID: dbCred.ProviderID,
	}, nil
}

// decryptWithDEKCache decrypts an API key using a DEK from the DEK cache
// (or falls back to KMS unwrap if the DEK isn't cached).
func (m *Manager) decryptWithDEKCache(ctx context.Context, credentialID string, encryptedKey, wrappedDEK []byte) ([]byte, error) {

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

		m.dekCache.Invalidate(credentialID)
	}

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

	dekEnclave := memguard.NewEnclave(dek)
	m.dekCache.Set(credentialID, dekEnclave)
	for i := range dek {
		dek[i] = 0
	}

	return apiKey, nil
}

// promoteToL1 seals a copy of the plaintext API key in memguard and stores it in L1.
// NOTE: memguard.NewEnclave zeroes the source slice, so we copy first.
func (m *Manager) promoteToL1(credentialID string, orgID uuid.UUID, apiKey []byte, baseURL, authScheme, providerID string) {
	keyCopy := make([]byte, len(apiKey))
	copy(keyCopy, apiKey)
	enclave := memguard.NewEnclave(keyCopy)
	m.memory.Set(credentialID, &CachedCredential{
		Enclave:    enclave,
		BaseURL:    baseURL,
		AuthScheme: authScheme,
		ProviderID: providerID,
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
