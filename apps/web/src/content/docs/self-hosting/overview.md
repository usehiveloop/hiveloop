---
title: Self-Hosting Overview
description: Learn about LLMVault's architecture, components, and requirements for self-hosting
---

# Self-Hosting Overview

LLMVault can be self-hosted on your own infrastructure, giving you complete control over your data and deployment. This guide covers the architecture, components, and requirements for running LLMVault in your own environment.

## Architecture

LLMVault follows a microservices architecture with the following core components:

```
┌─────────────────────────────────────────────────────────────────────┐
│                         LLMVault Stack                              │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  ┌──────────────┐     ┌──────────────┐     ┌──────────────┐        │
│  │   Web App    │────▶│  LLMVault    │────▶│  PostgreSQL  │        │
│  │  (Next.js)   │     │   Server     │     │   (Data)     │        │
│  └──────────────┘     └──────┬───────┘     └──────────────┘        │
│                              │                                      │
│                              ▼                                      │
│  ┌──────────────┐     ┌──────────────┐     ┌──────────────┐        │
│  │   Logto      │◀────│    Redis     │     │   HashiCorp  │        │
│  │   (Auth)     │     │   (Cache)    │◀────│    Vault     │        │
│  └──────────────┘     └──────────────┘     │    (KMS)     │        │
│                                            └──────────────┘        │
│                              │                                      │
│                              ▼                                      │
│                       ┌──────────────┐                              │
│                       │    Nango     │                              │
│                       │(OAuth Proxy) │                              │
│                       └──────────────┘                              │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

## Core Components

### 1. LLMVault Server (Go)

The main API server built with Go 1.25+, providing:

- **LLM Proxy**: Route requests to 200+ LLM providers with unified authentication
- **Credential Vault**: Secure storage of API keys using envelope encryption
- **Integration Management**: Connect to providers via OAuth or API keys
- **Organization & Access Control**: Multi-tenant organization support
- **MCP Server**: Model Context Protocol server for AI agent integrations

**Resource Requirements:**
- CPU: 1+ cores (2+ recommended for production)
- Memory: 512MB minimum (1GB+ recommended)
- Ports: 8080 (API), 8081 (MCP)

### 2. PostgreSQL (Required)

Primary database for storing:
- Organization data
- Integration configurations
- Credential metadata (encrypted)
- Token and access logs

**Version:** PostgreSQL 15+ (tested with PostgreSQL 17)

**Resource Requirements:**
- Storage: 10GB minimum (grows with usage)
- Memory: 512MB+ recommended
- Extensions: `pgcrypto` (auto-enabled via init script)

### 3. Redis (Required)

Caching layer for:
- L2 cache for decrypted credentials
- Session tokens
- Rate limiting data

**Version:** Redis 7+

**Resource Requirements:**
- Memory: 256MB minimum (configurable via `maxmemory`)
- Persistence: AOF recommended for production

### 4. KMS Provider (Required)

Key Management Service for envelope encryption of credentials. Three options available:

| Provider | Use Case | Production Ready |
|----------|----------|------------------|
| **AEAD** | Local development, single-node | ❌ Not for production |
| **AWS KMS** | AWS deployments | ✅ Yes |
| **HashiCorp Vault** | Multi-cloud, on-premise | ✅ Yes |

### 5. Logto (Required for Auth)

Identity and access management providing:
- OIDC/OAuth2 authentication
- Organization-based multi-tenancy
- Machine-to-machine (M2M) authentication

**Options:**
- **Hosted**: Use `https://auth.dev.llmvault.dev` (staging) or managed service
- **Self-hosted**: Deploy your own Logto instance

### 6. Nango (Required for OAuth)

OAuth integration proxy supporting 250+ providers:
- Secure OAuth token management
- Automatic token refresh
- Webhook notifications

**Options:**
- **Hosted**: Use `https://integrations.dev.llmvault.dev` (staging)
- **Self-hosted**: Deploy your own Nango instance

### 7. Web Dashboard (Next.js)

Optional web interface for:
- Organization management
- Integration configuration
- Credential management
- Usage analytics

## System Requirements

### Minimum Requirements (Development)

| Component | CPU | Memory | Storage |
|-----------|-----|--------|---------|
| LLMVault Server | 1 core | 512MB | - |
| PostgreSQL | 1 core | 512MB | 10GB |
| Redis | 1 core | 256MB | - |
| **Total** | **2 cores** | **1.5GB** | **10GB** |

### Recommended (Production)

| Component | CPU | Memory | Storage |
|-----------|-----|--------|---------|
| LLMVault Server | 2+ cores | 1GB+ | - |
| PostgreSQL | 2 cores | 2GB+ | 50GB+ |
| Redis | 1 core | 512MB+ | - |
| Vault (KMS) | 1 core | 512MB | 10GB |
| **Total** | **6+ cores** | **4GB+** | **60GB+** |

## Network Requirements

### Ports

| Port | Service | Description |
|------|---------|-------------|
| 80/443 | HTTP/HTTPS | Web traffic (via Nginx) |
| 8080 | LLMVault API | Internal API server |
| 8081 | MCP Server | Model Context Protocol |
| 5432 | PostgreSQL | Database (internal only) |
| 6379 | Redis | Cache (internal only) |
| 8200 | Vault | KMS (internal only) |

### External Dependencies

The following endpoints must be accessible for full functionality:

| Endpoint | Purpose |
|----------|---------|
| `https://auth.dev.llmvault.dev` | Logto authentication (staging) |
| `https://integrations.dev.llmvault.dev` | Nango OAuth proxy (staging) |
| Provider APIs | OpenRouter, Fireworks, etc. |

## Security Considerations

### Encryption Layers

1. **Transport**: TLS 1.2+ for all external communications
2. **At Rest**: AES-256-GCM for credential storage
3. **Key Management**: Envelope encryption with external KMS

### Required for Production

- ✅ External KMS (AWS KMS or HashiCorp Vault)
- ✅ TLS certificates (valid, not self-signed)
- ✅ Network segmentation (internal services not exposed)
- ✅ Regular backups
- ✅ Audit logging

## Deployment Options

### 1. Docker Compose (Easiest)

Best for: Small teams, development, single-node deployments

- All services in one compose file
- Automatic service discovery
- Volume-based persistence

See [Docker Compose Guide](./docker-compose) for details.

### 2. Kubernetes

Best for: Production, high availability, auto-scaling

- Helm chart available
- Horizontal Pod Autoscaling
- Health checks and rolling updates

See [Kubernetes Guide](./kubernetes) for details.

### 3. Manual Deployment

Best for: Custom infrastructure, VMs, bare metal

- Binary deployment
- Manual service configuration
- Full control over each component

## Data Flow

```
1. User Request
        │
        ▼
2. Nginx (reverse proxy)
        │
        ▼
3. LLMVault Server (auth check)
        │
        ├──▶ Logto (verify token)
        │
        ▼
4. Retrieve credentials
        │
        ├──▶ Redis (L2 cache check)
        │
        ├──▶ PostgreSQL (encrypted DEK)
        │
        └──▶ Vault/AWS KMS (unwrap DEK)
        │
        ▼
5. Decrypt credentials (in-memory)
        │
        ▼
6. Forward to LLM provider
```

## Next Steps

Choose your deployment method:

- [Docker Compose Deployment](./docker-compose) - Quick start with Docker
- [Kubernetes Deployment](./kubernetes) - Production-grade orchestration
- [Configuration Reference](./configuration) - Detailed configuration options
- [Environment Variables](./environment) - Complete environment variable reference
