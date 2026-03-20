---
title: Credentials
description: Secure storage and lifecycle management of LLM API credentials with envelope encryption, auto-detection, and request caps.
---

# Credentials

Credentials are the foundation of LLMVault's security model. They store encrypted LLM API keys with fine-grained access controls, request caps, and automatic lifecycle management.

## What is a Credential?

A credential represents a stored LLM provider API key. It contains:

- **Encrypted API key** - Protected using AES-256-GCM envelope encryption
- **Base URL** - The upstream provider endpoint
- **Auth scheme** - How to authenticate (Bearer, x-api-key, api-key, query_param)
- **Provider ID** - Reference to the provider in the embedded catalog
- **Identity association** - Optional link to an identity for rate limiting
- **Request caps** - Optional limits on API call volume
- **Metadata** - JSONB field for custom tags and filtering

```go
type Credential struct {
    ID             uuid.UUID  `gorm:"type:uuid;primaryKey"`
    OrgID          uuid.UUID  `gorm:"type:uuid;not null;index"`
    Label          string     `gorm:"not null;default:''"`
    BaseURL        string     `gorm:"not null"`
    AuthScheme     string     `gorm:"not null"`
    IdentityID     *uuid.UUID `gorm:"type:uuid;index"`
    EncryptedKey   []byte     `gorm:"type:bytea;not null"`
    WrappedDEK     []byte     `gorm:"type:bytea;not null"`
    Remaining      *int64     // Request cap remaining
    RefillAmount   *int64     // Auto-refill amount
    RefillInterval *string    // Go duration format (e.g., "1h", "24h")
    LastRefillAt   *time.Time
    ProviderID     string     `gorm:"default:''"`
    Meta           JSON       `gorm:"type:jsonb;default:'{}'"`
    RevokedAt      *time.Time
    CreatedAt      time.Time
}
```

## Envelope Encryption

LLMVault uses a two-layer envelope encryption scheme to protect API keys:

### Layer 1: Data Encryption Key (DEK)

Each credential gets a unique 256-bit random DEK generated using `crypto/rand`:

```go
func GenerateDEK() ([]byte, error) {
    key := make([]byte, 32)
    if _, err := io.ReadFull(rand.Reader, key); err != nil {
        return nil, fmt.Errorf("generating DEK: %w", err)
    }
    return key, nil
}
```

The API key is encrypted using **AES-256-GCM** with the DEK:

```go
func EncryptCredential(plaintext []byte, dek []byte) ([]byte, error) {
    block, _ := aes.NewCipher(dek)
    gcm, _ := cipher.NewGCM(block)
    nonce := make([]byte, gcm.NonceSize())
    io.ReadFull(rand.Reader, nonce)
    return gcm.Seal(nonce, nonce, plaintext, nil)
}
```

The ciphertext format: `[nonce (12 bytes)][ciphertext + tag]`

### Layer 2: Key Encryption Key (KEK)

The DEK itself is encrypted by a Key Management System (KMS):

```go
// Wrap DEK via KMS
wrappedDEK, err := kms.Wrap(ctx, dek)

// Store in database
// - EncryptedKey: AES-256-GCM encrypted API key
// - WrappedDEK: KMS-encrypted DEK
```

### Supported KMS Backends

| Backend | Use Case |
|---------|----------|
| **AEAD** | Local AES-256-GCM for development/single-node |
| **AWS KMS** | Production AWS deployments |
| **Vault Transit** | HashiCorp Vault for enterprise |

## Three-Tier Credential Cache

Credentials are resolved through a high-performance 3-tier cache:

```
L1 (Memory) → L2 (Redis) → L3 (Postgres + KMS)
~0.01ms       ~0.5ms       ~3-8ms
```

### L1: In-Memory Cache

- Stores **decrypted** credentials in `memguard` enclaves
- mlocked memory, encrypted at rest in RAM
- LRU eviction with configurable TTL (default: 5 minutes)
- Hard expiry prevents stale data (default: 15 minutes)

```go
type CachedCredential struct {
    Enclave    *memguard.Enclave  // Sealed plaintext API key
    BaseURL    string
    AuthScheme string
    OrgID      uuid.UUID
    CachedAt   time.Time
    HardExpiry time.Time
}
```

### L2: Redis Cache

- Stores **still-encrypted** credentials
- DEK remains KMS-wrapped
- JSON serialized with fields: `ek` (encrypted key), `wd` (wrapped DEK), `bu`, `as`, `oi`
- TTL: 30 minutes

### L3: Database + KMS

- Postgres stores `encrypted_key` and `wrapped_dek`
- Cold path: fetch from DB, unwrap DEK via KMS, decrypt API key
- Singleflight deduplication prevents thundering herd

## Request Caps and Lifecycle

### Request Caps

Credentials support optional request caps with automatic refill:

```json
{
  "remaining": 1000,
  "refill_amount": 1000,
  "refill_interval": "1h"
}
```

| Field | Description |
|-------|-------------|
| `remaining` | Current request budget |
| `refill_amount` | Amount to refill when interval elapses |
| `refill_interval` | Go duration (e.g., "1h", "24h", "168h") |

### Lazy Refill Algorithm

Refills happen on-demand when a cap is exhausted:

1. Check if `now - last_refill_at >= refill_interval`
2. Use optimistic locking to prevent double-refill
3. Update `remaining = refill_amount`, `last_refill_at = now`
4. Reset Redis counter

```go
// Optimistic locking: only update if last_refill_at matches
result := db.Model(&model.Credential{}).
    Where("id = ? AND (last_refill_at = ? OR (last_refill_at IS NULL AND ? = ?))",
        credentialID, lastRefill, lastRefill, cred.CreatedAt).
    Updates(map[string]any{
        "remaining":      *cred.RefillAmount,
        "last_refill_at": now,
    })
```

### Redis Counter Management

Counters are stored in Redis with atomic Lua scripts:

```lua
-- Decrement script
local v = redis.call("GET", KEYS[1])
if v == false then return -1 end  -- No cap configured
local n = tonumber(v)
if n <= 0 then return 0 end       -- Exhausted
redis.call("DECR", KEYS[1])
return 1                          -- Success
```

Key format: `pbreq:cred:{credential_id}`

## Revocation

Credentials are soft-deleted via revocation:

```go
now := time.Now()
db.Model(&model.Credential{}).
    Where("id = ? AND org_id = ? AND revoked_at IS NULL", credID, orgID).
    Update("revoked_at", &now)
```

Revocation triggers:
- Immediate invalidation of all cache tiers (L1, L2, DEK cache)
- Cross-instance pub/sub notification
- All tokens minted against this credential become invalid

## Creating a Credential

```bash
curl -X POST https://api.llmvault.dev/v1/credentials \
  -H "Authorization: Bearer {org_token}" \
  -H "Content-Type: application/json" \
  -d '{
    "provider_id": "openai",
    "base_url": "https://api.openai.com/v1",
    "auth_scheme": "bearer",
    "api_key": "sk-...",
    "label": "Production OpenAI",
    "external_id": "user_12345",
    "remaining": 10000,
    "refill_amount": 10000,
    "refill_interval": "24h",
    "meta": {"environment": "production", "team": "ai"}
  }'
```

### Auto-Creating Identities

If `external_id` is provided without an existing identity, one is auto-created:

```go
if req.ExternalID != nil && *req.ExternalID != "" {
    var ident model.Identity
    err := db.Where("external_id = ? AND org_id = ?", *req.ExternalID, org.ID).First(&ident).Error
    if err == gorm.ErrRecordNotFound {
        ident = model.Identity{
            ID:         uuid.New(),
            OrgID:      org.ID,
            ExternalID: *req.ExternalID,
        }
        db.Create(&ident)
    }
    identityID = &ident.ID
}
```

## Filtering and Listing

Credentials support powerful filtering:

```bash
# Filter by identity
curl "https://api.llmvault.dev/v1/credentials?identity_id=uuid"

# Filter by external ID
curl "https://api.llmvault.dev/v1/credentials?external_id=user_12345"

# Filter by metadata (JSONB containment)
curl "https://api.llmvault.dev/v1/credentials?meta={\"team\":\"ai\"}"

# Cursor pagination
curl "https://api.llmvault.dev/v1/credentials?limit=50&cursor=eyJpZCI6..."
```

## Auth Schemes

| Scheme | Header/Param |
|--------|--------------|
| `bearer` | `Authorization: Bearer {api_key}` |
| `x-api-key` | `x-api-key: {api_key}` |
| `api-key` | `api-key: {api_key}` |
| `query_param` | `?key={api_key}` |

## Security Properties

1. **Encryption at rest**: API keys encrypted with AES-256-GCM
2. **Key isolation**: Unique DEK per credential
3. **Memory protection**: `memguard` enclaves for decrypted keys
4. **Zeroization**: Keys wiped from memory immediately after use
5. **SSRF protection**: Base URL validation against private IPs
6. **Audit logging**: Every proxy request logged with credential ID
