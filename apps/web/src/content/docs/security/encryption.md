---
title: Encryption
description: How LLMVault encrypts your API keys with envelope encryption and KMS integration.
---

# Encryption

LLMVault uses **envelope encryption** -- a two-layer encryption scheme that combines the performance of symmetric encryption with the key management benefits of a centralized KMS.

## Envelope Encryption

### What is Envelope Encryption?

Envelope encryption is a practice where:

1. A **Data Encryption Key (DEK)** encrypts the actual data (your API key)
2. The DEK itself is encrypted by a **Key Encryption Key (KEK)** managed by your KMS
3. Only the encrypted DEK is stored alongside the encrypted data

```
┌─────────────────────────────────────────────────────────────────────────┐
│                       ENVELOPE ENCRYPTION FLOW                          │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  Plaintext API Key                                                      │
│       │                                                                 │
│       v                                                                 │
│  ┌─────────────────┐     ┌─────────────┐     ┌───────────────────────┐ │
│  │  Generate DEK   │────>│ AES-256-GCM │────>│  Encrypted API Key    │ │
│  │  (256-bit)      │     │  Encrypt    │     │  (stored in database) │ │
│  └─────────────────┘     └─────────────┘     └───────────────────────┘ │
│           │                                                             │
│           v                                                             │
│  ┌─────────────────┐     ┌─────────────┐     ┌───────────────────────┐ │
│  │  Plaintext DEK  │────>│  KMS Wrap   │────>│  Wrapped DEK          │ │
│  │                 │     │  (KEK)      │     │  (stored in database) │ │
│  └─────────────────┘     └─────────────┘     └───────────────────────┘ │
│           │                                                             │
│           v                                                             │
│     [DEK immediately zeroed from memory]                                │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

This means that even if an attacker gains full access to your database, they see only ciphertext. Decrypting a credential requires access to both the database **and** the KMS.

### How Your Keys Are Protected

LLMVault uses **AES-256-GCM** (Galois/Counter Mode) for all data encryption. Here is what that gives you:

- **256-bit keys**: Brute-force resistant with a 2^256 key space
- **Authenticated encryption**: GCM provides both confidentiality and integrity -- any tampering with the ciphertext is detected
- **Unique nonces**: A 96-bit random nonce is generated for every encryption operation, ensuring no two encryptions produce the same output
- **No padding attacks**: GCM is a stream cipher mode, immune to padding oracle attacks

### One Key Per Credential

Every credential you store in LLMVault gets its own unique, cryptographically random 256-bit DEK. This isolation means:

- Compromise of one credential's DEK does not affect any other credential
- DEKs are generated from an OS-level cryptographically secure random number generator (CSPRNG)
- DEKs are never persisted in plaintext -- they are wrapped by the KMS before storage

## Key Management Service (KMS)

### Supported Backends

LLMVault supports three KMS backends. Production deployments **must** use AWS KMS or HashiCorp Vault.

| Backend | Environment | Description |
|---------|-------------|-------------|
| **AWS KMS** | Production | Fully managed, FIPS 140-2 validated |
| **HashiCorp Vault** | Production | Self-managed, multi-cloud, on-premise |
| **AEAD (Local)** | Development only | Local AES-GCM wrapper, blocked in production |

### AWS KMS

AWS KMS provides fully managed key encryption with hardware security module (HSM) backing.

**Configuration:**

```bash
KMS_TYPE=awskms
KMS_KEY=arn:aws:kms:us-east-1:ACCOUNT:key/KEY-ID
AWS_REGION=us-east-1
```

**Features:**

- Automatic credential chain resolution (IAM roles, environment variables, instance profiles)
- Support for KMS key aliases and ARNs
- Regional endpoint configuration
- CloudTrail integration for KMS audit logging
- FIPS 140-2 validated endpoints available

### HashiCorp Vault

For multi-cloud or on-premise deployments, LLMVault integrates with Vault's Transit secrets engine.

**Configuration:**

```bash
KMS_TYPE=vault
KMS_KEY=llmvault-key
VAULT_ADDRESS=https://vault.internal:8200
VAULT_TOKEN=s.xxxxxxxx
VAULT_MOUNT_PATH=transit
```

**Optional configuration:**

| Variable | Description |
|----------|-------------|
| `VAULT_NAMESPACE` | Vault Enterprise namespace |
| `VAULT_CA_CERT` | Path to CA certificate for TLS verification |
| `VAULT_CLIENT_CERT` | Path to client certificate for mTLS |
| `VAULT_CLIENT_KEY` | Path to client key for mTLS |

**Features:**

- Automatic key rotation within Vault
- Namespace support (Vault Enterprise)
- mTLS authentication
- Auto-unseal with cloud KMS

### AEAD (Development Only)

For local development, a local AES-GCM wrapper is available. LLMVault will refuse to start in production mode with this backend.

**Configuration:**

```bash
KMS_TYPE=aead
KMS_KEY=base64-encoded-32-byte-key
```

Generate a development key:

```bash
openssl rand -base64 32
```

## Key Rotation

### DEK Rotation

DEK rotation is recommended when:

- A credential is suspected to be compromised
- Your periodic rotation policy requires it
- You are rotating your KMS key (optional but recommended)

The rotation process is handled transparently:

```
1. Decrypt existing credential with current DEK
           │
           v
2. Generate a new DEK
           │
           v
3. Re-encrypt the API key with the new DEK
           │
           v
4. Wrap the new DEK with your KMS
           │
           v
5. Update the database record atomically
           │
           v
6. Invalidate all cache tiers
```

### KMS Key Rotation

Both AWS KMS and Vault Transit support automatic key rotation:

**AWS KMS:**

- Enable automatic annual rotation in the AWS console or via API
- Previous key versions remain available for decryption
- New encryptions automatically use the latest key material

**HashiCorp Vault Transit:**

```bash
# Rotate key manually
vault write -f transit/keys/llmvault-key/rotate

# Enable automatic rotation
vault write transit/keys/llmvault-key/config \
  auto_rotate_period=768h  # 32 days
```

- Vault Transit maintains key versions automatically
- Decryption uses the correct version without configuration changes
- Set `min_decryption_version` to prevent downgrade attacks

## Memory Protection

### The Problem

Even with perfect encryption at rest, credentials must be decrypted in memory to proxy requests. This creates potential vulnerabilities:

- **Swap/pagefile**: Memory may be written to disk by the operating system
- **Core dumps**: Process crashes can dump memory contents
- **Debuggers**: Attached debuggers can read process memory

### How LLMVault Protects Memory

LLMVault uses hardware-backed memory protection for all decrypted credentials:

| Protection | What It Does |
|------------|-------------|
| **mlock** | Prevents sensitive memory pages from being swapped to disk |
| **Guard pages** | Memory barriers that detect and prevent buffer overflows |
| **Auto-zeroing** | Source data is wiped immediately after being sealed into protected memory |
| **Secure destruction** | Memory is cryptographically wiped when credentials are evicted from cache |

When a credential is decrypted for proxying, the plaintext is immediately sealed into a protected memory enclave. The original plaintext buffer is zeroed. When the credential is no longer needed (cache eviction, credential revocation, or process shutdown), the protected memory is securely destroyed.

### Cache Security

LLMVault's three-tier cache ensures credentials are protected at every level:

- **L1 (In-Memory)**: Plaintext API keys are held only in sealed, mlock'd memory enclaves with automatic expiry
- **L2 (Redis)**: Stores only encrypted values -- a Redis breach exposes no plaintext
- **L3 (Database)**: Stores only encrypted values and wrapped DEKs

When a credential is revoked, all cache tiers are purged immediately, and other instances are notified to purge their local caches as well.

## Encryption in Transit

All connections must use TLS in production:

| Connection | Requirement |
|-----------|-------------|
| **Database** | `sslmode=require` or `sslmode=verify-full` |
| **Redis** | `rediss://` URL scheme (TLS) |
| **KMS** | HTTPS for Vault, TLS 1.2+ for AWS |
| **Upstream LLM providers** | HTTPS enforced |

**Configuration example:**

```bash
# Database TLS
DB_SSLMODE=require

# Redis TLS
REDIS_URL=rediss://redis.internal:6380

# Vault TLS
VAULT_ADDRESS=https://vault.internal:8200
VAULT_CA_CERT=/etc/ssl/certs/vault-ca.pem
```

## Monitoring

Monitor these indicators for encryption health:

| Metric | What to Watch For |
|--------|-------------------|
| KMS decrypt latency | Spikes above 100ms may indicate KMS issues |
| Cold cache hit rate | Sudden spikes mean more KMS calls than expected |
| Decryption errors | Any sustained error rate above 0.1% needs investigation |
| Memory protection failures | Any failure should trigger an alert |

## Key Takeaways

1. **Every credential has a unique DEK** -- compromise of one does not affect others
2. **DEKs never touch disk unencrypted** -- they are always KMS-wrapped before storage
3. **Memory is protected** -- mlock and auto-zeroing prevent plaintext leaks
4. **Production requires external KMS** -- AWS KMS or HashiCorp Vault only
5. **Cache is encrypted at L2/L3** -- only L1 has plaintext, and it is held in sealed memory
6. **Database breach is insufficient** -- an attacker needs both database access and KMS access to decrypt anything
