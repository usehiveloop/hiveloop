---
title: Security Overview
description: A complete overview of how LLMVault protects your customers' API keys at every layer.
---

# Security Overview

LLMVault is built with a security-first architecture designed to protect sensitive LLM API credentials at every layer. This document outlines our comprehensive threat model, defense-in-depth security layers, and compliance certifications.

## Threat Model

LLMVault is designed to defend against the following threat categories:

### External Threats

- **Credential Theft**: Attackers attempting to steal plaintext API keys from storage or memory
- **Database Breaches**: Unauthorized access to the Postgres database containing encrypted credentials
- **Network Eavesdropping**: Interception of credentials in transit
- **Token Replay**: Use of stolen or revoked proxy tokens
- **Privilege Escalation**: Attempts to access credentials belonging to other organizations

### Internal Threats

- **Insider Access**: Employees with database access attempting to read customer credentials
- **Memory Dumps**: Core dumps or swap files containing decrypted keys
- **Cache Poisoning**: Manipulation of cached credential data
- **Side-Channel Attacks**: Timing or observation attacks against the encryption system

### Infrastructure Threats

- **KMS Compromise**: Attacks against the key management service
- **Redis Breach**: Unauthorized access to the Redis cache layer
- **Supply Chain**: Compromised dependencies or build artifacts

## Security Layers

LLMVault implements defense-in-depth with multiple independent security layers:

### Layer 1: Envelope Encryption

All API keys are encrypted using **AES-256-GCM** with unique Data Encryption Keys (DEKs). The DEKs are themselves encrypted by a Key Management Service (KMS) using industry-standard algorithms.

```go
// From internal/crypto/envelope.go
func EncryptCredential(plaintext []byte, dek []byte) ([]byte, error) {
    block, err := aes.NewCipher(dek)
    gcm, err := cipher.NewGCM(block)
    nonce := make([]byte, gcm.NonceSize())
    return gcm.Seal(nonce, nonce, plaintext, nil), nil
}
```

**Key Points:**
- Each credential has a unique 256-bit DEK
- AES-256-GCM provides authenticated encryption
- Nonce is randomly generated for each encryption and prepended to ciphertext
- No plaintext keys ever touch persistent storage

### Layer 2: KMS Integration

DEKs are wrapped (encrypted) using an external KMS. LLMVault supports three KMS backends:

| Provider | Use Case | Production Ready |
|----------|----------|------------------|
| **AWS KMS** | AWS deployments | ✅ Yes |
| **HashiCorp Vault** | Multi-cloud, on-premise | ✅ Yes |
| **AEAD (Local)** | Development only | ❌ No |

**Production Enforcement:**
```go
// From internal/config/config.go
if cfg.Environment == "production" && cfg.KMSType != "awskms" && cfg.KMSType != "vault" {
    return nil, fmt.Errorf("KMS_TYPE must be 'awskms' or 'vault' in production")
}
```

### Layer 3: Memory Protection

Decrypted credentials are protected in memory using the `memguard` library:

- **mlock**: Prevents sensitive memory from being swapped to disk
- **Enclaves**: Sealed memory regions for plaintext keys
- **Auto-zeroing**: Keys are automatically wiped from memory when no longer needed

```go
// From internal/cache/cache.go
dekEnclave := memguard.NewEnclave(dek)
// Zero the plaintext DEK immediately after sealing
for i := range dek {
    dek[i] = 0
}
```

### Layer 4: Three-Tier Caching

The cache architecture ensures decrypted keys exist in memory only when actively needed:

```
┌─────────────────────────────────────────────────────────────────┐
│  L1: In-Memory (memguard enclaves)  ←  ~0.01ms access time     │
│  L2: Redis (still-encrypted values)   ←  ~0.5ms access time     │
│  L3: Postgres + KMS                   ←  ~3-8ms access time     │
└─────────────────────────────────────────────────────────────────┘
```

**Security Benefits:**
- L2/L3 store only encrypted data
- DEKs are cached separately in sealed memory
- Hard expiry limits exposure window
- Singleflight prevents thundering herd on cache misses

### Layer 5: Token-Based Access Control

Proxy tokens (`ptok_*`) are short-lived JWTs with embedded scope constraints:

```go
// From internal/token/jwt.go
type Claims struct {
    OrgID        string `json:"org_id"`
    CredentialID string `json:"cred_id"`
    ScopeHash    string `json:"scope_hash,omitempty"`
    jwt.RegisteredClaims
}
```

**Token Properties:**
- Maximum TTL: 24 hours (enforced at mint time)
- Unique JTI (JWT ID) for revocation tracking
- Scope hash binding prevents scope tampering
- Tokens prefixed with `ptok_` for easy identification

### Layer 6: Authentication & Authorization

Multiple authentication layers ensure only authorized access:

**Organization API Keys (`llmv_sk_*`):**
```go
// From internal/middleware/apikeyauth.go
keyHash := model.HashAPIKey(rawKey)
// L1: Check in-memory cache
if cached, ok := keyCache.Get(keyHash); ok {
    // Validate organization active status
    if !org.Active {
        return http.StatusForbidden, "organization is inactive"
    }
}
```

**Token Authentication:**
```go
// From internal/middleware/tokenauth.go
claims, err := token.Validate(signingKey, jwtString)
// Check if the token has been revoked
var tokenRecord model.Token
result := db.Where("jti = ?", claims.ID).First(&tokenRecord)
if tokenRecord.RevokedAt != nil {
    return http.StatusUnauthorized, "token has been revoked"
}
```

### Layer 7: Audit Logging

Every API and proxy request is logged for security monitoring:

```go
// From internal/middleware/audit.go
entry := model.AuditEntry{
    Action:   a,
    Metadata: model.JSON{
        "method": r.Method, 
        "path": r.URL.Path, 
        "status": sw.status, 
        "latency_ms": time.Since(start).Milliseconds()
    },
}
```

**Logged Fields:**
- Organization ID
- Credential ID (for proxy requests)
- Identity ID (when applicable)
- IP address
- HTTP method and path
- Response status code
- Request latency

## Certifications

### SOC 2 Type II

LLMVault is designed to meet SOC 2 Type II requirements for:

- **Security**: Comprehensive access controls and encryption
- **Availability**: HA deployment options with Redis clustering
- **Processing Integrity**: Audit trails for all credential operations
- **Confidentiality**: Encryption at rest and in transit

### GDPR Compliance

- Data encryption at rest using customer-controlled KMS keys
- Right to erasure via credential revocation APIs
- Audit trails for all data access
- Data residency options via self-hosting

### HIPAA Considerations

While LLMVault provides security controls suitable for HIPAA (encryption, audit logs, access controls), customers in regulated industries should:

1. Use AWS KMS or HashiCorp Vault with dedicated key hierarchies
2. Enable comprehensive audit logging
3. Deploy in VPC-isolated environments
4. Complete a Business Associate Agreement (BAA) with their cloud provider

## Security Best Practices

### For Production Deployments

1. **Always use AWS KMS or Vault** - Never use AEAD encryption in production
2. **Enable TLS everywhere** - Between app, Postgres, Redis, and KMS
3. **Use dedicated KMS keys** - Separate keys per environment/tenant
4. **Monitor audit logs** - Set up alerts for unusual access patterns
5. **Regular key rotation** - Rotate KMS keys according to your policy
6. **Network isolation** - Deploy in private subnets with VPC endpoints

### Credential Lifecycle

```
┌─────────────┐    ┌─────────────┐    ┌─────────────┐    ┌─────────────┐
│   Create    │───►│    Use      │───►│   Rotate    │───►│   Revoke    │
│ (encrypt)   │    │  (decrypt)  │    │  (re-enc)   │    │ (invalidate)│
└─────────────┘    └─────────────┘    └─────────────┘    └─────────────┘
       │                  │                  │                  │
       ▼                  ▼                  ▼                  ▼
   Generate DEK      3-tier cache      New DEK/KMS       Purge all
   KMS wrap          Token scoping     Re-encrypt        caches
```

### Incident Response

In the event of a security incident:

1. **Credential Compromise Suspected**: Revoke the credential immediately - all cache tiers will be purged
2. **Token Leaked**: Revoke via `DELETE /v1/tokens/{jti}` - takes effect within seconds
3. **KMS Key Compromised**: Rotate KMS key and re-wrap all DEKs
4. **Database Breach**: Rotate KMS key; encrypted data is useless without KMS access

## Vulnerability Disclosure

We take security seriously. If you discover a vulnerability:

1. **Do not** open a public issue
2. Email security@llmvault.dev with details
3. Allow 90 days for remediation before public disclosure
4. We offer bug bounties for verified critical vulnerabilities

## Related Documentation

- [Encryption](/docs/security/encryption) - Deep dive into envelope encryption
- [Token Scoping](/docs/security/token-scoping) - Fine-grained access control
- [Audit Logging](/docs/security/audit-logging) - Security monitoring
- [Compliance](/docs/security/compliance) - Regulatory compliance details
