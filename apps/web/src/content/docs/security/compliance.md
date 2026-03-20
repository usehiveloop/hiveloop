---
title: Compliance
description: Compliance certifications and security standards.
---

# Compliance

LLMVault is designed to meet enterprise security and compliance requirements. This document outlines our compliance posture across SOC 2, GDPR, and HIPAA frameworks.

## SOC 2

### Overview

SOC 2 (System and Organization Controls 2) is a security framework developed by the AICPA. It evaluates organizations on five Trust Service Criteria:

- **Security** - Protection against unauthorized access
- **Availability** - System availability for operation and use
- **Processing Integrity** - Complete, valid, accurate, timely processing
- **Confidentiality** - Designated confidential information is protected
- **Privacy** - Personal information is collected, used, retained, and disposed of properly

### LLMVault Controls

#### Security (Common Criteria)

| Control | Implementation | Evidence |
|---------|---------------|----------|
| CC6.1 - Logical access | API keys + JWT tokens | Auth middleware (`internal/middleware/apikeyauth.go`) |
| CC6.2 - Access removal | Token revocation | Revoke endpoint (`internal/handler/tokens.go`) |
| CC6.3 - Access monitoring | Audit logging | Audit middleware (`internal/middleware/audit.go`) |
| CC6.4 - Encryption | AES-256-GCM envelope encryption | `internal/crypto/envelope.go` |
| CC6.5 - Key management | AWS KMS / Vault integration | `internal/crypto/kms.go` |
| CC6.6 - Security infrastructure | Network isolation, TLS | Config validation (`internal/config/config.go`) |
| CC6.7 - Security incident detection | Audit log monitoring | Audit handler (`internal/handler/audit.go`) |

#### Availability

| Control | Implementation |
|---------|---------------|
| A1.1 - Backup and recovery | Redis persistence, Postgres WAL |
| A1.2 - System monitoring | Health check endpoints |
| A1.3 - Incident response | Runbooks, on-call procedures |

#### Processing Integrity

| Control | Implementation |
|---------|---------------|
| PI1.1 - Entity authorization | Token scoping (`internal/mcp/scope.go`) |
| PI1.2 - System processing | Request validation, rate limiting |
| PI1.3 - Error handling | Structured error responses |
| PI1.4 - System processing integrity | Singleflight for cache consistency |

#### Confidentiality

| Control | Implementation |
|---------|---------------|
| C1.1 - Confidentiality | Envelope encryption |
| C1.2 - Access constraints | Org-scoped queries |

### SOC 2 Type II

**Current Status:** In Progress

**Timeline:**
- Type I: Q2 2025 (point-in-time assessment)
- Type II: Q4 2025 (6-month operational assessment)

**Scope:**
- LLMVault API and proxy services
- Encryption and key management systems
- Audit logging and monitoring
- Access control systems

### Customer Responsibilities

Customers preparing for their own SOC 2 audits should:

1. **Document key management**
   - KMS key rotation procedures
   - Key access policies
   - Emergency key revocation process

2. **Configure audit logging**
   - Enable all audit events
   - Set appropriate retention (90+ days)
   - Export to SIEM

3. **Implement access controls**
   - Principle of least privilege for API keys
   - Regular access reviews
   - Automated revocation of unused keys

4. **Maintain documentation**
   - Architecture diagrams
   - Data flow documentation
   - Incident response procedures

## GDPR

### Data Protection Principles

LLMVault implements GDPR requirements:

| Principle | Implementation |
|-----------|---------------|
| **Lawfulness, Fairness, Transparency** | Clear privacy policy, audit trails |
| **Purpose Limitation** | API keys only for credential management |
| **Data Minimization** | Only store necessary metadata |
| **Accuracy** | APIs for data correction |
| **Storage Limitation** | Configurable retention policies |
| **Integrity and Confidentiality** | Encryption at rest and in transit |
| **Accountability** | Audit logs, compliance documentation |

### Data Subject Rights

| Right | LLMVault Support |
|-------|-----------------|
| **Access** | Export audit logs, credential metadata |
| **Rectification** | Update via API endpoints |
| **Erasure (Right to be Forgotten)** | Revoke credentials, soft delete |
| **Restriction of Processing** | Revoke tokens, disable API keys |
| **Data Portability** | JSON export of all data |
| **Objection** | Stop processing by revocation |

### Technical Measures

```go
// From: internal/handler/credentials.go

// Right to Erasure: Soft delete with revocation
func (h *CredentialHandler) Revoke(w http.ResponseWriter, r *http.Request) {
    now := time.Now()
    result := h.db.Model(&model.Credential{}).
        Where("id = ? AND org_id = ? AND revoked_at IS NULL", credID, org.ID).
        Update("revoked_at", &now)
    
    // Invalidate all cache tiers
    _ = h.cacheManager.InvalidateCredential(r.Context(), credID)
}
```

### Data Processing Agreement (DPA)

A Data Processing Agreement is available for Enterprise customers. Key terms:

- **Processor**: LLMVault processes API credentials on behalf of customers
- **Subprocessors**: AWS (hosting), HashiCorp (KMS option), Redis Labs (cache)
- **Data Location**: Configurable by region for self-hosted
- **Security Measures**: Encryption, access controls, audit logging
- **Breach Notification**: 24-hour notification of security incidents
- **Audit Rights**: Annual audit rights for Enterprise customers

### International Data Transfers

For EU customers:
- Self-hosting available for data residency
- AWS KMS regions can be specified
- No data leaves your VPC with self-hosted option

## HIPAA

### Covered Entity Considerations

LLMVault provides security controls that support HIPAA compliance, but customers must:

1. **Complete a BAA** with their cloud provider (AWS, etc.)
2. **Use appropriate KMS** (dedicated keys, no shared KMS)
3. **Enable comprehensive audit logging**
4. **Implement access controls** per HIPAA requirements
5. **Conduct risk assessment** of their specific deployment

### Security Rule Mapping

| HIPAA Requirement | LLMVault Implementation |
|------------------|------------------------|
| **164.312(a)(1) - Access Control** | API key auth, token scoping, RBAC |
| **164.312(a)(2)(i) - Unique User IDs** | Org-scoped credentials, identity linking |
| **164.312(a)(2)(ii) - Emergency Access** | Break-glass procedures for credential recovery |
| **164.312(a)(2)(iv) - Encryption** | AES-256-GCM envelope encryption |
| **164.312(b) - Audit Controls** | Comprehensive audit logging |
| **164.312(c)(1) - Integrity** | GCM authenticated encryption |
| **164.312(d) - Person/Entity Authentication** | Multi-layer auth (API keys, JWT tokens) |
| **164.312(e)(1) - Transmission Security** | TLS 1.2+ for all connections |

### Recommended Configuration

For HIPAA-aligned deployments:

```bash
# Required: External KMS (not AEAD)
KMS_TYPE=awskms
KMS_KEY=arn:aws:kms:us-east-1:ACCOUNT:key/KEY-ID

# Required: TLS everywhere
DB_SSLMODE=require
REDIS_URL=rediss://...  # Note: rediss:// for TLS

# Required: Audit logging (minimum 6 years for HIPAA)
# Configure Postgres retention policies

# Recommended: Private subnets, VPC endpoints
# No public internet access for KMS
```

### Business Associate Agreement

LLMVault does not sign BAAs directly. Customers should:

1. Self-host LLMVault in their AWS account
2. Sign BAA with AWS for underlying infrastructure
3. Use AWS KMS with dedicated Customer Managed Keys (CMKs)
4. Implement their own access and audit controls

## Industry Standards

### Cryptographic Standards

| Standard | Implementation |
|----------|---------------|
| **FIPS 140-2** | AWS KMS FIPS-compliant endpoints available |
| **NIST SP 800-57** | Key hierarchy follows guidelines |
| **NIST SP 800-131A** | TLS 1.2+, SHA-256, AES-256 |

### Security Frameworks

| Framework | Alignment |
|-----------|-----------|
| **CIS Controls** | Controls 3, 6, 8, 14 implemented |
| **ISO 27001** | A.10 (crypto), A.12 (ops security), A.13 (comm security) |
| **NIST CSF** | Protect (PR.AC, PR.DS), Detect (DE.AE), Respond (RS.AN) |

## Certifications Roadmap

### 2025

| Quarter | Certification | Status |
|---------|--------------|--------|
| Q2 | SOC 2 Type I | In Progress |
| Q3 | ISO 27001 | Planned |
| Q4 | SOC 2 Type II | Planned |

### 2026

| Quarter | Certification | Status |
|---------|--------------|--------|
| Q1 | PCI DSS (for payment data) | Under Evaluation |
| Q2 | HIPAA BAA Program | Under Evaluation |

## Compliance Documentation

### Available Documents

Enterprise customers can request:

- **SOC 2 Type II Report** (available Q4 2025)
- **Penetration Test Reports** (annual)
- **Vulnerability Scan Reports** (quarterly)
- **Infrastructure Diagrams** (CND level)
- **Data Flow Diagrams**
- **Incident Response Plan**
- **Business Continuity Plan**

### Audit Support

For customer audits, LLMVault provides:

1. **Architecture documentation**
2. **Control mapping matrices**
3. **Sample audit evidence**
4. **Questionnaire responses**

Contact support@llmvault.dev for audit support requests.

## Self-Assessment Checklist

Use this checklist to assess your LLMVault deployment:

### Encryption

- [ ] Production uses AWS KMS or HashiCorp Vault (not AEAD)
- [ ] KMS keys are dedicated (not shared across environments)
- [ ] Key rotation is enabled in KMS
- [ ] TLS is used for all connections (DB, Redis, upstream)

### Access Control

- [ ] API keys have minimal required scopes
- [ ] Unused API keys are revoked
- [ ] Token TTLs are set appropriately
- [ ] Token scoping is used for integrations

### Audit Logging

- [ ] All API requests are logged
- [ ] Log retention meets compliance requirements
- [ ] Logs are exported to SIEM
- [ ] Alerting is configured for anomalies

### Infrastructure

- [ ] Deployed in private subnets
- [ ] VPC endpoints used for AWS services
- [ ] Network ACLs restrict traffic
- [ ] DDoS protection enabled

### Incident Response

- [ ] Revocation procedures documented
- [ ] Contact information current
- [ ] Escalation paths defined
- [ ] Regular drills conducted

## Shared Responsibility Model

```
┌─────────────────────────────────────────────────────────────────────┐
│                          CUSTOMER                                   │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │  • API key management                                        │   │
│  │  • Token scoping configuration                               │   │
│  │  • Access policies                                           │   │
│  │  • Audit log monitoring                                      │   │
│  │  • Data classification                                       │   │
│  └─────────────────────────────────────────────────────────────┘   │
├─────────────────────────────────────────────────────────────────────┤
│                          LLMVAULT                                   │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │  • Application security                                      │   │
│  │  • Encryption implementation                                 │   │
│  │  • Audit logging                                             │   │
│  │  • Vulnerability management                                  │   │
│  │  • Security updates                                          │   │
│  └─────────────────────────────────────────────────────────────┘   │
├─────────────────────────────────────────────────────────────────────┤
│                       INFRASTRUCTURE (Customer Cloud)               │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │  • Physical security                                         │   │
│  │  • Network security                                          │   │
│  │  • Host security                                             │   │
│  │  • KMS availability                                          │   │
│  └─────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────┘
```

## Contact

For compliance questions:

- **General Inquiries**: compliance@llmvault.dev
- **Security Issues**: security@llmvault.dev
- **Audit Support**: support@llmvault.dev

## Related Documentation

- [Security Overview](/docs/security/overview) - Security architecture
- [Encryption](/docs/security/encryption) - Technical encryption details
- [Audit Logging](/docs/security/audit-logging) - Compliance logging
- [Self-Hosting](/docs/self-hosting/overview) - Deployment for compliance
