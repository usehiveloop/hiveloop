---
title: Encryption
description: Details of LLMVault's envelope encryption and key management.
---

# Encryption

LLMVault uses **envelope encryption** - a two-layer encryption scheme that combines the performance of symmetric encryption with the key management benefits of a centralized KMS.

## Envelope Encryption

### What is Envelope Encryption?

Envelope encryption is a practice where:
1. A **Data Encryption Key (DEK)** encrypts the actual data (API key)
2. The DEK itself is encrypted by a **Key Encryption Key (KEK)** from the KMS
3. Only the encrypted DEK is stored alongside the encrypted data

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         ENVELOPE ENCRYPTION FLOW                            │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  Plaintext API Key                                                          │
│       │                                                                     │
│       ▼                                                                     │
│  ┌─────────────────┐     ┌─────────────┐     ┌─────────────────────────┐   │
│  │  Generate DEK   │────►│ AES-256-GCM │────►│   Encrypted API Key     │   │
│  │  (256-bit)      │     │  Encrypt    │     │   (stored in Postgres)  │   │
│  └─────────────────┘     └─────────────┘     └─────────────────────────┘   │
│           │                                                                 │
│           ▼                                                                 │
│  ┌─────────────────┐     ┌─────────────┐     ┌─────────────────────────┐   │
│  │   Plaintext DEK │────►│  KMS Wrap   │────►│   Wrapped DEK           │   │
│  │                 │     │  (KEK)      │     │   (stored in Postgres)  │   │
│  └─────────────────┘     └─────────────┘     └─────────────────────────┘   │
│           │                                                                 │
│           ▼                                                                 │
│     [DEK immediately zeroed from memory]                                    │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### AES-256-GCM Implementation

LLMVault uses AES-256-GCM (Galois/Counter Mode) for all data encryption:

```go
// From: internal/crypto/envelope.go
package crypto

import (
    "crypto/aes"
    "crypto/cipher"
    "crypto/rand"
    "io"
)

// EncryptCredential encrypts plaintext using AES-256-GCM with the given DEK.
// The nonce is prepended to the ciphertext.
func EncryptCredential(plaintext []byte, dek []byte) ([]byte, error) {
    block, err := aes.NewCipher(dek)
    if err != nil {
        return nil, fmt.Errorf("creating cipher: %w", err)
    }

    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return nil, fmt.Errorf("creating GCM: %w", err)
    }

    nonce := make([]byte, gcm.NonceSize())
    if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
        return nil, fmt.Errorf("generating nonce: %w", err)
    }

    // Seal appends the authentication tag to the ciphertext
    return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// DecryptCredential decrypts ciphertext using AES-256-GCM with the given DEK.
// Expects the nonce prepended to the ciphertext.
func DecryptCredential(ciphertext []byte, dek []byte) ([]byte, error) {
    block, err := aes.NewCipher(dek)
    if err != nil {
        return nil, fmt.Errorf("creating cipher: %w", err)
    }

    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return nil, fmt.Errorf("creating GCM: %w", err)
    }

    nonceSize := gcm.NonceSize()
    if len(ciphertext) < nonceSize {
        return nil, fmt.Errorf("ciphertext too short")
    }

    nonce, ciphertextBody := ciphertext[:nonceSize], ciphertext[nonceSize:]
    plaintext, err := gcm.Open(nil, nonce, ciphertextBody, nil)
    if err != nil {
        return nil, fmt.Errorf("decrypting: %w", err)
    }

    return plaintext, nil
}
```

**Security Properties:**
- **256-bit keys**: Brute-force resistant (2^256 key space)
- **Authenticated encryption**: GCM provides both confidentiality and integrity
- **Unique nonces**: 96-bit random nonce for each encryption
- **No padding oracle attacks**: GCM is a stream cipher mode

### DEK Generation

Each credential gets a unique, cryptographically random DEK:

```go
// From: internal/crypto/envelope.go
// GenerateDEK creates a 256-bit random data encryption key.
func GenerateDEK() ([]byte, error) {
    key := make([]byte, 32) // 256 bits
    if _, err := io.ReadFull(rand.Reader, key); err != nil {
        return nil, fmt.Errorf("generating DEK: %w", err)
    }
    return key, nil
}
```

**Key Characteristics:**
- Generated using `crypto/rand` (OS-level CSPRNG)
- 32 bytes (256 bits) of entropy
- Never reused across credentials
- Never persisted in plaintext

## Key Management Service (KMS)

### Supported KMS Backends

LLMVault abstracts KMS operations through a unified `KeyWrapper` interface:

```go
// From: internal/crypto/kms.go
type KeyWrapper struct {
    wrapper wrapping.Wrapper
}

// Wrap encrypts plaintext (typically a DEK) and returns the serialized blob
func (kw *KeyWrapper) Wrap(ctx context.Context, plaintext []byte) ([]byte, error) {
    blob, err := kw.wrapper.Encrypt(ctx, plaintext)
    if err != nil {
        return nil, fmt.Errorf("kms encrypt: %w", err)
    }
    data, err := proto.Marshal(blob)
    if err != nil {
        return nil, fmt.Errorf("marshal blob: %w", err)
    }
    return data, nil
}

// Unwrap decrypts a serialized blob back to plaintext
func (kw *KeyWrapper) Unwrap(ctx context.Context, ciphertext []byte) ([]byte, error) {
    var blob wrapping.BlobInfo
    if err := proto.Unmarshal(ciphertext, &blob); err != nil {
        return nil, fmt.Errorf("unmarshal blob: %w", err)
    }
    plaintext, err := kw.wrapper.Decrypt(ctx, &blob)
    if err != nil {
        return nil, fmt.Errorf("kms decrypt: %w", err)
    }
    return plaintext, nil
}
```

### AWS KMS

For AWS deployments, LLMVault integrates with AWS KMS:

```go
// From: internal/crypto/kms.go
func NewAWSKMSWrapper(kmsKeyID, region string) (*KeyWrapper, error) {
    w := awskms.NewWrapper()
    cfg := map[string]string{
        "kms_key_id": kmsKeyID,
    }
    if region != "" {
        cfg["region"] = region
    }
    _, err := w.SetConfig(context.Background(), wrapping.WithConfigMap(cfg))
    if err != nil {
        return nil, fmt.Errorf("configuring AWS KMS wrapper: %w", err)
    }
    return &KeyWrapper{wrapper: w}, nil
}
```

**Configuration:**
```bash
KMS_TYPE=awskms
KMS_KEY=arn:aws:kms:us-east-1:ACCOUNT:key/KEY-ID
AWS_REGION=us-east-1
```

**Features:**
- Automatic credential chain resolution (IAM roles, env vars, etc.)
- Support for KMS key aliases and ARNs
- Regional endpoint configuration
- CloudTrail integration for audit logging

### HashiCorp Vault

For multi-cloud or on-premise deployments:

```go
// From: internal/crypto/kms.go
type VaultConfig struct {
    Address    string // Vault server address
    Token      string // Vault authentication token
    Namespace  string // Optional Vault Enterprise namespace
    MountPath  string // Transit engine mount path (default: "transit")
    KeyName    string // Name of the encryption key in Vault
    CACert     string // Path to CA certificate (optional)
    ClientCert string // Path to client certificate (optional)
    ClientKey  string // Path to client key (optional)
}

func NewVaultTransitWrapper(cfg VaultConfig) (*KeyWrapper, error) {
    w := transit.NewWrapper()
    configMap := map[string]string{
        "address": cfg.Address,
        "token":   cfg.Token,
        "key_name": cfg.KeyName,
    }
    // ... additional config options
    _, err := w.SetConfig(context.Background(), wrapping.WithConfigMap(configMap))
    if err != nil {
        return nil, fmt.Errorf("configuring Vault Transit wrapper: %w", err)
    }
    return &KeyWrapper{wrapper: w}, nil
}
```

**Configuration:**
```bash
KMS_TYPE=vault
KMS_KEY=llmvault-key
VAULT_ADDRESS=https://vault.internal:8200
VAULT_TOKEN=s.xxxxxxxx
VAULT_MOUNT_PATH=transit
```

**Features:**
- Automatic key rotation in Vault
- Namespace support (Enterprise)
- mTLS authentication
- Auto-unseal with cloud KMS

### AEAD (Development Only)

For local development, a local AES-GCM wrapper is available:

```go
// From: internal/crypto/kms.go
func NewAEADWrapper(keyBase64, keyID string) (*KeyWrapper, error) {
    w := aead.NewWrapper()
    _, err := w.SetConfig(context.Background(), wrapping.WithConfigMap(map[string]string{
        "aead_type": "aes-gcm",
        "key":       keyBase64,
        "key_id":    keyID,
    }))
    if err != nil {
        return nil, fmt.Errorf("configuring AEAD wrapper: %w", err)
    }
    return &KeyWrapper{wrapper: w}, nil
}
```

**⚠️ Production Enforcement:**
```go
// From: internal/config/config.go
if cfg.Environment == "production" && cfg.KMSType != "awskms" && cfg.KMSType != "vault" {
    return nil, fmt.Errorf("KMS_TYPE must be 'awskms' or 'vault' in production (got %q)", cfg.KMSType)
}
```

## DEK Rotation

### When to Rotate

DEK rotation is recommended when:
- A credential is suspected to be compromised
- Periodic rotation policy requires it
- KMS key is rotated (optional)

### Rotation Process

```
┌──────────────────────────────────────────────────────────────────────┐
│                      DEK ROTATION FLOW                               │
├──────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  1. Decrypt existing credential with current DEK                    │
│          │                                                           │
│          ▼                                                           │
│  2. Generate new DEK                                                │
│          │                                                           │
│          ▼                                                           │
│  3. Re-encrypt API key with new DEK                                  │
│          │                                                           │
│          ▼                                                           │
│  4. Wrap new DEK with KMS                                            │
│          │                                                           │
│          ▼                                                           │
│  5. Update database record                                           │
│          │                                                           │
│          ▼                                                           │
│  6. Invalidate all cache tiers                                       │
│                                                                      │
└──────────────────────────────────────────────────────────────────────┘
```

### KMS Key Rotation

Both AWS KMS and Vault Transit support automatic key rotation:

**AWS KMS:**
- Enable automatic rotation (annual)
- Previous key versions remain available for decryption
- New encryptions use the new key material

**Vault Transit:**
```bash
# Rotate key manually
vault write -f transit/keys/llmvault-key/rotate

# Enable automatic rotation
vault write transit/keys/llmvault-key/config \
  auto_rotate_period=768h  # 32 days
```

**Key Versioning:**
- Vault Transit maintains key versions
- Decryption automatically uses the correct version
- Set `min_decryption_version` to prevent downgrade attacks

## Memory Protection

### The Problem

Even with perfect encryption at rest, credentials must be decrypted in memory to use them. This creates vulnerabilities:

- **Swap/pagefile**: Memory may be written to disk
- **Core dumps**: Process crashes can dump memory
- **Debuggers**: Attached debuggers can read memory
- **Cold boot attacks**: RAM contents can be extracted

### Memguard Solution

LLMVault uses `memguard` to protect sensitive data in memory:

```go
// From: internal/cache/memory.go
import "github.com/awnumar/memguard"

// CachedCredential holds a decrypted credential in sealed, mlocked memory.
type CachedCredential struct {
    Enclave    *memguard.Enclave  // Sealed, mlocked memory
    BaseURL    string
    AuthScheme string
    OrgID      uuid.UUID
    CachedAt   time.Time
    HardExpiry time.Time
}

// NewMemoryCache creates a new in-memory credential cache.
func NewMemoryCache(maxSize int, ttl time.Duration) *MemoryCache {
    onEvict := func(_ string, v *CachedCredential) {
        if v != nil && v.Enclave != nil {
            // Open and destroy to zero out memory
            if buf, err := v.Enclave.Open(); err == nil {
                buf.Destroy() // Securely wipes memory
            }
        }
    }
    return &MemoryCache{
        lru: expirable.NewLRU[string, *CachedCredential](maxSize, onEvict, ttl),
    }
}
```

### Memory Protection Features

| Feature | Implementation | Benefit |
|---------|---------------|---------|
| **mlock** | `mlockall()` syscall | Prevents swapping to disk |
| **Guard pages** | Memory barriers | Detects buffer overflows |
| **Auto-zeroing** | `memguard.NewEnclave()` | Wipes source data automatically |
| **Protected buffers** | `LockedBuffer` | Tamper-resistant memory regions |

### DEK Cache Security

The DEK cache stores unwrapped DEKs in sealed memory:

```go
// From: internal/cache/cache.go
// Cache DEK in DEKCache
dekEnclave := memguard.NewEnclave(dek)
m.dekCache.Set(credentialID, dekEnclave)

// Zero the plaintext DEK immediately
for i := range dek {
    dek[i] = 0
}

// Later: decrypt using cached DEK
if enclave, ok := m.dekCache.Get(credentialID); ok {
    buf, err := enclave.Open()
    if err == nil {
        dek := make([]byte, buf.Size())
        copy(dek, buf.Bytes())
        buf.Destroy() // Destroy after use
        
        apiKey, err := crypto.DecryptCredential(encryptedKey, dek)
        
        // Zero DEK after use
        for i := range dek {
            dek[i] = 0
        }
    }
}
```

### Cache Invalidation

When a credential is revoked, all traces are purged:

```go
// From: internal/cache/cache.go
func (m *Manager) InvalidateCredential(ctx context.Context, credentialID string) error {
    m.memory.Invalidate(credentialID)   // L1: Purge from memory
    m.dekCache.Invalidate(credentialID) // DEK: Purge wrapped keys
    
    if err := m.redisCache.Invalidate(ctx, credentialID); err != nil {
        return fmt.Errorf("redis invalidate: %w", err)
    }
    
    // Notify other instances
    return m.invalidator.PublishCredentialInvalidation(ctx, credentialID)
}
```

## Encryption in Transit

### TLS Requirements

All connections must use TLS:

- **Database**: `sslmode=require` for Postgres
- **Redis**: `rediss://` URL scheme for TLS
- **KMS**: HTTPS for Vault, TLS 1.2+ for AWS
- **Proxy**: HTTPS for upstream LLM providers

### Internal Communication

```go
// From: internal/config/config.go
func (c *Config) DatabaseDSN() string {
    return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
        url.QueryEscape(c.DBUser),
        url.QueryEscape(c.DBPassword),
        c.DBHost,
        c.DBPort,
        c.DBName,
        c.DBSSLMode,  // Set to "require" or "verify-full" in production
    )
}
```

## Security Metrics

Monitor these metrics for encryption health:

| Metric | Description | Alert Threshold |
|--------|-------------|-----------------|
| `kms_decrypt_latency` | Time to unwrap DEKs | > 100ms |
| `cache_l3_hit_rate` | Cold path (KMS) frequency | Sudden spike |
| `credential_decrypt_errors` | Decryption failures | > 0.1% |
| `memguard_alloc_errors` | Memory protection failures | Any |

## Key Takeaways

1. **Every credential has a unique DEK** - Compromise of one doesn't affect others
2. **DEKs never touch disk unencrypted** - KMS wrap/unwrap only
3. **Memory is protected** - mlock + auto-zeroing prevents leaks
4. **Production requires external KMS** - AWS KMS or Vault only
5. **Cache is encrypted at L2/L3** - Only L1 has plaintext (and it's mlocked)
