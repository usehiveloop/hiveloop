---
title: Security Overview
description: A complete overview of how LLMVault protects your API keys at every layer.
---

# Security Overview

LLMVault is built with a security-first architecture designed to protect sensitive LLM API credentials at every layer. This document outlines our threat model, defense-in-depth security layers, and compliance posture.

## Threat Model

LLMVault is designed to defend against the following threat categories:

### External Threats

- **Credential Theft**: Attackers attempting to steal plaintext API keys from storage or memory
- **Database Breaches**: Unauthorized access to the database containing encrypted credentials
- **Network Eavesdropping**: Interception of credentials in transit
- **Token Replay**: Use of stolen or revoked proxy tokens
- **Privilege Escalation**: Attempts to access credentials belonging to other organizations

### Internal Threats

- **Insider Access**: Employees with database access attempting to read customer credentials
- **Memory Dumps**: Core dumps or swap files containing decrypted keys
- **Cache Poisoning**: Manipulation of cached credential data

### Infrastructure Threats

- **KMS Compromise**: Attacks against the key management service
- **Cache Breach**: Unauthorized access to the caching layer
- **Supply Chain**: Compromised dependencies or build artifacts

## Security Layers

LLMVault implements defense-in-depth with seven independent security layers:

### Layer 1: Envelope Encryption

All API keys are encrypted using **AES-256-GCM** with unique Data Encryption Keys (DEKs). Each credential gets its own randomly generated 256-bit key, so compromise of one credential does not affect others. The DEKs are themselves encrypted by a Key Management Service (KMS).

- Each credential has a unique 256-bit DEK
- AES-256-GCM provides authenticated encryption (confidentiality + integrity)
- A unique random nonce is generated for each encryption operation
- No plaintext keys ever touch persistent storage

Learn more in the [Encryption](/docs/security/encryption) deep dive.

### Layer 2: KMS Integration

DEKs are wrapped (encrypted) using an external KMS. LLMVault supports multiple KMS backends:

| Provider | Use Case | Production Ready |
|----------|----------|------------------|
| **AWS KMS** | AWS deployments | Yes |
| **HashiCorp Vault** | Multi-cloud, on-premise | Yes |
| **AEAD (Local)** | Development only | No |

LLMVault enforces that production deployments use AWS KMS or HashiCorp Vault. The local AEAD backend is blocked in production environments.

### Layer 3: Memory Protection

Decrypted credentials are protected in memory using hardware-backed security features:

- **mlock**: Prevents sensitive memory from being swapped to disk
- **Sealed enclaves**: Isolated memory regions for plaintext keys
- **Auto-zeroing**: Keys are automatically wiped from memory when no longer needed
- **Guard pages**: Memory barriers that detect buffer overflows

### Layer 4: Three-Tier Caching

The cache architecture ensures decrypted keys exist in memory only when actively needed:

```
┌──────────────────────────────────────────────────────────────┐
│  L1: In-Memory (sealed enclaves)    ~0.01ms access time     │
│  L2: Redis (encrypted values only)  ~0.5ms access time      │
│  L3: Database + KMS                 ~3-8ms access time       │
└──────────────────────────────────────────────────────────────┘
```

- L2 and L3 store only encrypted data -- a database or Redis breach exposes no plaintext
- DEKs are cached separately in sealed memory with hard expiry limits
- Cache entries expire automatically to limit the exposure window

### Layer 5: Token-Based Access Control

Proxy tokens (`ptok_*`) are short-lived JWTs with embedded scope constraints:

- **Maximum TTL**: 24 hours, enforced at mint time
- **Unique identifiers**: Every token has a unique JTI for revocation tracking
- **Scope binding**: A cryptographic hash of the token's scopes is embedded in the JWT, preventing tampering
- **Instant revocation**: Revoked tokens are rejected within seconds

Learn more in [Token Scoping](/docs/security/token-scoping).

### Layer 6: Authentication & Authorization

Multiple authentication layers ensure only authorized access:

- **Organization API Keys** (`llmv_sk_*`): Hashed and validated against an in-memory cache. Inactive organizations are rejected immediately.
- **Proxy Tokens** (`ptok_*`): Validated against signing keys and checked for revocation on every request.
- **Organization Isolation**: All queries are scoped to the authenticated organization. Cross-org access is structurally impossible.

### Layer 7: Audit Logging

Every API and proxy request is logged with:

- Organization ID
- Credential ID (for proxy requests)
- Identity ID (when applicable)
- IP address
- HTTP method and path
- Response status code
- Request latency

Learn more in [Audit Logging](/docs/security/audit-logging).

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

LLMVault provides security controls suitable for HIPAA-regulated environments (encryption, audit logs, access controls). Customers in regulated industries should:

1. Use AWS KMS or HashiCorp Vault with dedicated key hierarchies
2. Enable comprehensive audit logging
3. Deploy in VPC-isolated environments
4. Complete a Business Associate Agreement (BAA) with their cloud provider

Learn more in [Compliance](/docs/security/compliance).

## Security Best Practices

### For Production Deployments

1. **Always use AWS KMS or Vault** -- Never use AEAD encryption in production
2. **Enable TLS everywhere** -- Between your app, database, cache, and KMS
3. **Use dedicated KMS keys** -- Separate keys per environment or tenant
4. **Monitor audit logs** -- Set up alerts for unusual access patterns
5. **Rotate KMS keys regularly** -- Follow your organization's rotation policy
6. **Network isolation** -- Deploy in private subnets with VPC endpoints

### Credential Lifecycle

```
┌─────────────┐    ┌─────────────┐    ┌─────────────┐    ┌─────────────┐
│   Create    │───>│    Use      │───>│   Rotate    │───>│   Revoke    │
│ (encrypt)   │    │  (decrypt)  │    │  (re-enc)   │    │ (invalidate)│
└─────────────┘    └─────────────┘    └─────────────┘    └─────────────┘
       │                  │                  │                  │
       v                  v                  v                  v
   Generate DEK      3-tier cache      New DEK/KMS       Purge all
   KMS wrap          Token scoping     Re-encrypt        caches
```

### Incident Response

In the event of a security incident:

1. **Credential compromise suspected**: Revoke the credential immediately -- all cache tiers are purged automatically
2. **Token leaked**: Revoke via the SDK or API -- takes effect within seconds
3. **KMS key compromised**: Rotate your KMS key and re-wrap all DEKs
4. **Database breach**: Rotate your KMS key; encrypted data is useless without KMS access

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
