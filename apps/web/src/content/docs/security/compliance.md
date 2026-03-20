---
title: Compliance
description: Compliance certifications and security standards.
---

# Compliance

LLMVault is designed to meet enterprise security and compliance requirements. This document outlines our compliance posture across SOC 2, GDPR, and HIPAA frameworks.

## SOC 2

### Overview

SOC 2 (System and Organization Controls 2) is a security framework developed by the AICPA. It evaluates organizations on five Trust Service Criteria:

- **Security** -- Protection against unauthorized access
- **Availability** -- System availability for operation and use
- **Processing Integrity** -- Complete, valid, accurate, timely processing
- **Confidentiality** -- Designated confidential information is protected
- **Privacy** -- Personal information is collected, used, retained, and disposed of properly

### LLMVault Controls

#### Security (Common Criteria)

| Control | How LLMVault Addresses It |
|---------|--------------------------|
| CC6.1 - Logical access | Organization API keys and scoped JWT tokens enforce authentication |
| CC6.2 - Access removal | Token revocation with instant cache purge across all tiers |
| CC6.3 - Access monitoring | Comprehensive audit logging of all API and proxy requests |
| CC6.4 - Encryption | AES-256-GCM envelope encryption with per-credential keys |
| CC6.5 - Key management | AWS KMS and HashiCorp Vault integration for key wrapping |
| CC6.6 - Security infrastructure | TLS required for all connections, network isolation support |
| CC6.7 - Security incident detection | Audit log monitoring with SIEM integration |

#### Availability

| Control | How LLMVault Addresses It |
|---------|--------------------------|
| A1.1 - Backup and recovery | Database WAL archiving, Redis persistence |
| A1.2 - System monitoring | Health check endpoints, cache and KMS metrics |
| A1.3 - Incident response | Documented runbooks and on-call procedures |

#### Processing Integrity

| Control | How LLMVault Addresses It |
|---------|--------------------------|
| PI1.1 - Entity authorization | Token scoping with per-action, per-resource constraints |
| PI1.2 - System processing | Request validation and rate limiting |
| PI1.3 - Error handling | Structured error responses with safe error messages |
| PI1.4 - System processing integrity | Cache consistency guarantees |

#### Confidentiality

| Control | How LLMVault Addresses It |
|---------|--------------------------|
| C1.1 - Confidentiality | Envelope encryption with unique DEKs per credential |
| C1.2 - Access constraints | All queries scoped to the authenticated organization |

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
   - Export to your SIEM

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

| Principle | How LLMVault Addresses It |
|-----------|--------------------------|
| **Lawfulness, Fairness, Transparency** | Clear privacy policy, comprehensive audit trails |
| **Purpose Limitation** | API keys used only for credential management and proxying |
| **Data Minimization** | Only necessary metadata is stored alongside encrypted credentials |
| **Accuracy** | APIs available for updating credential metadata |
| **Storage Limitation** | Configurable retention policies for audit logs and soft-deleted data |
| **Integrity and Confidentiality** | Encryption at rest (AES-256-GCM) and in transit (TLS 1.2+) |
| **Accountability** | Audit logs and compliance documentation |

### Data Subject Rights

| Right | LLMVault Support |
|-------|-----------------|
| **Access** | Export audit logs and credential metadata via API |
| **Rectification** | Update credential and identity metadata via API |
| **Erasure (Right to be Forgotten)** | Revoke credentials, triggering cache purge and soft delete |
| **Restriction of Processing** | Revoke tokens and disable API keys to stop processing |
| **Data Portability** | JSON export of all organization data via API |
| **Objection** | Stop processing by credential revocation |

### Exercising Data Subject Rights

Use the SDK to support data subject requests:

```typescript
import { LLMVault } from "@llmvault/sdk";
const vault = new LLMVault({ apiKey: "your-api-key" });

// Right to Access: export audit logs for a subject
const { data: logs } = await vault.audit.list({
  limit: 100,
  action: "proxy.request"
});

// Right to Erasure: revoke a credential
// This purges all cache tiers and soft-deletes the record
await vault.credentials.revoke("credential-id");
```

### Data Processing Agreement (DPA)

A Data Processing Agreement is available for Enterprise customers. Key terms:

- **Processor**: LLMVault processes API credentials on behalf of customers
- **Subprocessors**: AWS (hosting), HashiCorp (KMS option)
- **Data Location**: Configurable by region for self-hosted deployments
- **Security Measures**: Encryption, access controls, audit logging
- **Breach Notification**: 24-hour notification of security incidents
- **Audit Rights**: Annual audit rights for Enterprise customers

### International Data Transfers

For EU customers:
- Self-hosting is available for full data residency control
- AWS KMS regions can be specified to keep key material in-region
- No data leaves your infrastructure with the self-hosted option

## HIPAA

### Covered Entity Considerations

LLMVault provides security controls that support HIPAA compliance, but customers must:

1. **Complete a BAA** with their cloud provider (AWS, etc.)
2. **Use appropriate KMS** -- dedicated keys, not shared across environments
3. **Enable comprehensive audit logging** with 6-year retention
4. **Implement access controls** per HIPAA requirements
5. **Conduct a risk assessment** of their specific deployment

### Security Rule Mapping

| HIPAA Requirement | How LLMVault Addresses It |
|-------------------|--------------------------|
| **164.312(a)(1) - Access Control** | API key authentication, token scoping, organization isolation |
| **164.312(a)(2)(i) - Unique User IDs** | Organization-scoped credentials with identity linking |
| **164.312(a)(2)(ii) - Emergency Access** | Break-glass procedures for credential recovery |
| **164.312(a)(2)(iv) - Encryption** | AES-256-GCM envelope encryption with per-credential keys |
| **164.312(b) - Audit Controls** | Comprehensive audit logging with configurable retention |
| **164.312(c)(1) - Integrity** | GCM authenticated encryption detects any tampering |
| **164.312(d) - Person/Entity Authentication** | Multi-layer authentication (API keys, JWT tokens) |
| **164.312(e)(1) - Transmission Security** | TLS 1.2+ required for all connections |

### Recommended Configuration

For HIPAA-aligned deployments:

```bash
# Required: External KMS (AEAD is blocked in production)
KMS_TYPE=awskms
KMS_KEY=arn:aws:kms:us-east-1:ACCOUNT:key/KEY-ID

# Required: TLS everywhere
DB_SSLMODE=require
REDIS_URL=rediss://...  # Note: rediss:// for TLS

# Required: Audit logging with minimum 6-year retention
# Configure database retention policies accordingly

# Recommended: Private subnets, VPC endpoints
# No public internet access for KMS or database
```

### Business Associate Agreement

LLMVault does not sign BAAs directly. For HIPAA compliance:

1. Self-host LLMVault in your own AWS account
2. Sign a BAA with AWS for the underlying infrastructure
3. Use AWS KMS with dedicated Customer Managed Keys (CMKs)
4. Implement your own organizational access and audit controls

## Industry Standards

### Cryptographic Standards

| Standard | How LLMVault Aligns |
|----------|---------------------|
| **FIPS 140-2** | AWS KMS FIPS-validated endpoints available |
| **NIST SP 800-57** | Key hierarchy follows NIST key management guidelines |
| **NIST SP 800-131A** | TLS 1.2+, SHA-256, AES-256 throughout |

### Security Frameworks

| Framework | Alignment |
|-----------|-----------|
| **CIS Controls** | Controls 3 (data protection), 6 (access management), 8 (audit log management), 14 (security awareness) |
| **ISO 27001** | A.10 (cryptography), A.12 (operations security), A.13 (communications security) |
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
- **Infrastructure Diagrams**
- **Data Flow Diagrams**
- **Incident Response Plan**
- **Business Continuity Plan**

### Audit Support

For customer audits, LLMVault provides:

1. Architecture documentation
2. Control mapping matrices
3. Sample audit evidence
4. Questionnaire responses

Contact support@llmvault.dev for audit support requests.

## Self-Assessment Checklist

Use this checklist to assess your LLMVault deployment:

### Encryption

- [ ] Production uses AWS KMS or HashiCorp Vault (not AEAD)
- [ ] KMS keys are dedicated (not shared across environments)
- [ ] Key rotation is enabled in your KMS
- [ ] TLS is used for all connections (database, cache, upstream)

### Access Control

- [ ] API keys follow the principle of least privilege
- [ ] Unused API keys are revoked
- [ ] Token TTLs are set to the minimum viable duration
- [ ] Token scoping is used for integration access

### Audit Logging

- [ ] All API and proxy requests are logged
- [ ] Log retention meets your compliance requirements
- [ ] Logs are exported to your SIEM
- [ ] Alerting is configured for anomalies

### Infrastructure

- [ ] Deployed in private subnets
- [ ] VPC endpoints used for AWS services
- [ ] Network ACLs restrict traffic
- [ ] DDoS protection enabled

### Incident Response

- [ ] Credential and token revocation procedures are documented
- [ ] Contact information is current
- [ ] Escalation paths are defined
- [ ] Regular incident response drills are conducted

## Shared Responsibility Model

```
┌───────────────────────────────────────────────────────────────────┐
│                          CUSTOMER                                 │
│  ┌───────────────────────────────────────────────────────────┐   │
│  │  API key management and rotation                          │   │
│  │  Token scoping configuration                              │   │
│  │  Access policies and user management                      │   │
│  │  Audit log monitoring and alerting                        │   │
│  │  Data classification and handling                         │   │
│  └───────────────────────────────────────────────────────────┘   │
├───────────────────────────────────────────────────────────────────┤
│                          LLMVAULT                                 │
│  ┌───────────────────────────────────────────────────────────┐   │
│  │  Application security and hardening                       │   │
│  │  Encryption implementation and key management             │   │
│  │  Audit logging infrastructure                             │   │
│  │  Vulnerability management and patching                    │   │
│  │  Security updates and advisories                          │   │
│  └───────────────────────────────────────────────────────────┘   │
├───────────────────────────────────────────────────────────────────┤
│                  INFRASTRUCTURE (Customer Cloud)                   │
│  ┌───────────────────────────────────────────────────────────┐   │
│  │  Physical security                                        │   │
│  │  Network security and isolation                           │   │
│  │  Host security and patching                               │   │
│  │  KMS availability and key protection                      │   │
│  └───────────────────────────────────────────────────────────┘   │
└───────────────────────────────────────────────────────────────────┘
```

## Contact

For compliance questions:

- **General Inquiries**: compliance@llmvault.dev
- **Security Issues**: security@llmvault.dev
- **Audit Support**: support@llmvault.dev

## Related Documentation

- [Security Overview](/docs/security/overview) - Security architecture
- [Encryption](/docs/security/encryption) - Encryption and key management details
- [Audit Logging](/docs/security/audit-logging) - Audit logging and monitoring
- [Self-Hosting](/docs/self-hosting/overview) - Deployment for compliance
