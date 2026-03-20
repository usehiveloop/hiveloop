---
title: Environment Variables Reference
description: Complete reference of all environment variables for LLMVault
---

# Environment Variables Reference

This document provides a complete reference of all environment variables used by LLMVault.

## Quick Reference Table

### Required Variables

These variables must be set for LLMVault to start:

| Variable | Description | Example |
|----------|-------------|---------|
| `PORT` | HTTP server port | `8080` |
| `LOG_LEVEL` | Logging level | `info` |
| `LOG_FORMAT` | Log output format | `json` |
| `DB_HOST` | PostgreSQL hostname | `localhost` |
| `DB_USER` | PostgreSQL username | `llmvault` |
| `DB_PASSWORD` | PostgreSQL password | `secure-password` |
| `DB_NAME` | PostgreSQL database name | `llmvault` |
| `KMS_TYPE` | KMS provider type | `vault` |
| `REDIS_URL` or `REDIS_ADDR` | Redis connection | `localhost:6379` |
| `REDIS_CACHE_TTL` | Redis cache TTL | `30m` |
| `MEM_CACHE_TTL` | In-memory cache TTL | `5m` |
| `MEM_CACHE_MAX_SIZE` | In-memory cache size | `10000` |
| `JWT_SIGNING_KEY` | JWT signing key | `hex-encoded-key` |

### Required for Production

Additional variables required when `ENVIRONMENT=production`:

| Variable | Description | Example |
|----------|-------------|---------|
| `KMS_KEY` | KMS key identifier | `alias/llmvault-prod` |
| `AWS_REGION` | AWS region (for AWS KMS) | `us-east-1` |
| `VAULT_ADDRESS` | Vault URL (for Vault KMS) | `https://vault.company.com` |
| `VAULT_TOKEN` | Vault token (for Vault KMS) | `s.xxx` |
| `LOGTO_ENDPOINT` | Logto authentication URL | `https://auth.company.com` |
| `LOGTO_AUDIENCE` | Logto API resource | `https://api.llmvault.dev` |
| `LOGTO_M2M_APP_ID` | Logto M2M app ID | `abc123` |
| `LOGTO_M2M_APP_SECRET` | Logto M2M app secret | `xyz789` |
| `NANGO_ENDPOINT` | Nango OAuth proxy URL | `https://integrations.company.com` |
| `NANGO_SECRET_KEY` | Nango API secret | `secret-key` |

## Complete Variable Reference

### Core Server

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `ENVIRONMENT` | `development` | No | Environment mode: `development` or `production` |
| `PORT` | - | Yes | HTTP server port |
| `LOG_LEVEL` | - | Yes | Log level: `debug`, `info`, `warn`, `error` |
| `LOG_FORMAT` | - | Yes | Log format: `json` or `text` |

**Notes:**
- In production (`ENVIRONMENT=production`), `KMS_TYPE` must be `awskms` or `vault` (AEAD is not allowed)
- Use `LOG_FORMAT=json` for production to enable structured logging
- Use `LOG_LEVEL=warn` or `error` in production to reduce log volume

### Database (PostgreSQL)

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `DB_HOST` | `localhost` | Yes | PostgreSQL server hostname or IP |
| `DB_PORT` | `5432` | No | PostgreSQL server port |
| `DB_USER` | `llmvault` | Yes | PostgreSQL username |
| `DB_PASSWORD` | - | Yes | PostgreSQL password |
| `DB_NAME` | `llmvault` | Yes | PostgreSQL database name |
| `DB_SSLMODE` | `disable` | No | SSL mode: `disable`, `require`, `verify-ca`, `verify-full` |

**SSL Modes:**
- `disable`: No SSL (development only)
- `require`: SSL required, no verification
- `verify-ca`: SSL with CA certificate verification
- `verify-full`: SSL with hostname verification

**Docker Compose Port Mapping:**
When using Docker Compose, PostgreSQL is exposed on port `5433` on the host:
```bash
DB_HOST=localhost
DB_PORT=5433
```

From inside Docker containers:
```bash
DB_HOST=postgres
DB_PORT=5432
```

### KMS (Key Management Service)

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `KMS_TYPE` | - | Yes | KMS provider: `aead`, `awskms`, or `vault` |
| `KMS_KEY` | - | Conditional | KMS key identifier (required for all types) |
| `AWS_REGION` | `us-east-1` | Conditional | AWS region (required for `awskms`) |

#### AEAD (Development)

| Variable | Description |
|----------|-------------|
| `KMS_KEY` | Base64-encoded 32-byte AES key |

Generate a key:
```bash
openssl rand -base64 32
```

#### AWS KMS

| Variable | Default | Description |
|----------|---------|-------------|
| `KMS_KEY` | - | KMS Key ID, ARN, or alias (e.g., `alias/llmvault-prod`) |
| `AWS_REGION` | `us-east-1` | AWS region (e.g., `us-east-1`, `eu-west-1`) |
| `AWS_ACCESS_KEY_ID` | - | AWS access key (optional, uses IAM role if not set) |
| `AWS_SECRET_ACCESS_KEY` | - | AWS secret key (optional, uses IAM role if not set) |

AWS credentials resolution order:
1. Environment variables (`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`)
2. Shared credentials file (`~/.aws/credentials`)
3. Web identity token (EKS IRSA)
4. IAM role (EC2 instance profile, ECS task role)

#### HashiCorp Vault

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `KMS_KEY` | - | Yes | Name of the encryption key in Vault Transit |
| `VAULT_ADDRESS` | - | Yes | Vault server URL (e.g., `https://vault.company.com:8200`) |
| `VAULT_TOKEN` | - | Yes | Vault authentication token |
| `VAULT_NAMESPACE` | - | No | Vault Enterprise namespace |
| `VAULT_MOUNT_PATH` | `transit` | No | Transit engine mount path |
| `VAULT_CA_CERT` | - | No | Path to CA certificate for TLS |
| `VAULT_CLIENT_CERT` | - | No | Path to client certificate for mutual TLS |
| `VAULT_CLIENT_KEY` | - | No | Path to client key for mutual TLS |

### Redis

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `REDIS_URL` | - | Conditional | Full Redis URL (enables TLS if using `rediss://`) |
| `REDIS_ADDR` | `localhost:6379` | Conditional | Redis host:port (fallback if `REDIS_URL` not set) |
| `REDIS_PASSWORD` | - | No | Redis password |
| `REDIS_DB` | `0` | No | Redis database number (0-15) |
| `REDIS_CACHE_TTL` | - | Yes | Cache TTL for credentials (e.g., `30m`, `1h`) |

**Connection Priority:**
1. If `REDIS_URL` is set, it takes precedence
2. Otherwise, `REDIS_ADDR` is used with optional `REDIS_PASSWORD` and `REDIS_DB`

**URL Formats:**
```
redis://localhost:6379/0
redis://:password@localhost:6379/0
rediss://cache.amazonaws.com:6379  (TLS)
```

### In-Memory Cache (L1)

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `MEM_CACHE_TTL` | - | Yes | In-memory cache TTL (e.g., `5m`) |
| `MEM_CACHE_MAX_SIZE` | - | Yes | Maximum items in LRU cache |

**Recommendations:**
- Development: `MEM_CACHE_TTL=5m`, `MEM_CACHE_MAX_SIZE=5000`
- Production: `MEM_CACHE_TTL=5m`, `MEM_CACHE_MAX_SIZE=10000`

### JWT (Sandbox Proxy Tokens)

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `JWT_SIGNING_KEY` | - | Yes | HMAC-SHA256 signing key (min 32 characters) |

Generate a secure key:
```bash
# 256-bit key
openssl rand -hex 32

# 512-bit key
openssl rand -hex 64
```

**Security:**
- This key signs tokens for LLM proxy authentication
- Store securely (e.g., Kubernetes secrets, AWS Secrets Manager)
- Rotate periodically
- Use different keys for different environments

### CORS (Cross-Origin Resource Sharing)

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `CORS_ORIGINS` | - | No | Comma-separated list of allowed origins |

**Examples:**
```bash
# Single origin
CORS_ORIGINS=https://vault.yourcompany.com

# Multiple origins
CORS_ORIGINS=https://vault.yourcompany.com,https://admin.yourcompany.com

# Development
CORS_ORIGINS=http://localhost:3000,http://localhost:30112
```

**Note:** Do not use `*` in production. Always specify exact origins.

### Logto (Authentication)

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `LOGTO_ENDPOINT` | - | Yes | Logto server base URL |
| `LOGTO_AUDIENCE` | - | Yes | API resource identifier |
| `LOGTO_M2M_APP_ID` | - | Yes | Machine-to-machine application ID |
| `LOGTO_M2M_APP_SECRET` | - | Yes | Machine-to-machine application secret |

**Examples:**
```bash
# Hosted Logto
LOGTO_ENDPOINT=https://auth.llmvault.dev
LOGTO_AUDIENCE=https://api.llmvault.dev

# Self-hosted
LOGTO_ENDPOINT=https://auth.internal.company.com
LOGTO_AUDIENCE=https://api.internal.company.com
```

### Nango (OAuth Integration Proxy)

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `NANGO_ENDPOINT` | - | Yes | Nango server base URL |
| `NANGO_SECRET_KEY` | - | Yes | Nango secret key for API authentication |

**Examples:**
```bash
# Hosted Nango
NANGO_ENDPOINT=https://integrations.llmvault.dev

# Self-hosted
NANGO_ENDPOINT=https://integrations.internal.company.com
```

### MCP Server (Model Context Protocol)

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `MCP_PORT` | `8081` | No | MCP server port |
| `MCP_BASE_URL` | `http://localhost:8081` | No | MCP server base URL |

**Examples:**
```bash
# Default (localhost)
MCP_PORT=8081
MCP_BASE_URL=http://localhost:8081

# Production
MCP_PORT=8081
MCP_BASE_URL=https://mcp.yourcompany.com
```

## Configuration Validation

LLMVault validates configuration on startup. Missing required variables will cause the application to exit with an error message.

### Validation Rules

1. **All required variables must be set**
2. **In production:**
   - `KMS_TYPE` must be `awskms` or `vault` (AEAD not allowed)
   - `DB_SSLMODE` should be `require` or stricter
3. **Either `REDIS_URL` or `REDIS_ADDR` must be set**
4. **JWT signing key must be at least 32 characters**

### Startup Log Examples

**Successful startup:**
```
INFO[0000] Starting LLMVault server
INFO[0000] Environment: production
INFO[0000] Port: 8080
INFO[0000] Database: postgres://llmvault@db:5432/llmvault?sslmode=require
INFO[0000] KMS: awskms (alias/llmvault-production)
INFO[0000] Server ready
```

**Configuration error:**
```
FATAL[0000] Failed to load config: parsing config: env: required environment variable "DB_PASSWORD" is not set
```

**Production AEAD error:**
```
FATAL[0000] Failed to load config: KMS_TYPE must be 'awskms' or 'vault' in production (got "aead")
```

## Environment-Specific Examples

### Local Development (.env)

```bash
# Server
ENVIRONMENT=development
PORT=8080
LOG_LEVEL=debug
LOG_FORMAT=text

# Database (Docker Compose)
DB_HOST=localhost
DB_PORT=5433
DB_USER=llmvault
DB_PASSWORD=localdev
DB_NAME=llmvault
DB_SSLMODE=disable

# KMS (AEAD for dev - generate with: openssl rand -base64 32)
KMS_TYPE=aead
KMS_KEY=YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXoxMjM0NTY=

# Redis (Docker Compose)
REDIS_ADDR=localhost:6379
REDIS_PASSWORD=
REDIS_DB=0
REDIS_CACHE_TTL=30m

# Cache
MEM_CACHE_TTL=5m
MEM_CACHE_MAX_SIZE=10000

# JWT (generate with: openssl rand -hex 32)
JWT_SIGNING_KEY=dev-signing-key-not-for-production

# CORS
CORS_ORIGINS=http://localhost:3000,http://localhost:30112

# Logto (staging)
LOGTO_ENDPOINT=https://auth.dev.llmvault.dev
LOGTO_AUDIENCE=https://api.llmvault.dev
LOGTO_M2M_APP_ID=your-staging-app-id
LOGTO_M2M_APP_SECRET=your-staging-app-secret

# Nango (staging)
NANGO_ENDPOINT=https://integrations.dev.llmvault.dev
NANGO_SECRET_KEY=your-staging-secret-key
```

### Production - AWS (.env)

```bash
# Server
ENVIRONMENT=production
PORT=8080
LOG_LEVEL=warn
LOG_FORMAT=json

# Database (RDS)
DB_HOST=llmvault-db.cluster-xxx.us-east-1.rds.amazonaws.com
DB_PORT=5432
DB_USER=llmvault
DB_PASSWORD=your-secure-password
DB_NAME=llmvault
DB_SSLMODE=require

# KMS (AWS KMS)
KMS_TYPE=awskms
KMS_KEY=alias/llmvault-production
AWS_REGION=us-east-1

# Redis (ElastiCache)
REDIS_URL=rediss://llmvault-cache.xxx.cache.amazonaws.com:6379
REDIS_CACHE_TTL=30m

# Cache
MEM_CACHE_TTL=5m
MEM_CACHE_MAX_SIZE=10000

# JWT (from AWS Secrets Manager)
JWT_SIGNING_KEY=your-production-signing-key

# CORS
CORS_ORIGINS=https://vault.yourcompany.com

# Logto
LOGTO_ENDPOINT=https://auth.yourcompany.com
LOGTO_AUDIENCE=https://api.llmvault.dev
LOGTO_M2M_APP_ID=your-production-app-id
LOGTO_M2M_APP_SECRET=your-production-app-secret

# Nango
NANGO_ENDPOINT=https://integrations.yourcompany.com
NANGO_SECRET_KEY=your-production-secret-key
```

### Production - Kubernetes (ConfigMap + Secret)

**ConfigMap (non-sensitive):**
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: llmvault-config
data:
  ENVIRONMENT: "production"
  PORT: "8080"
  LOG_LEVEL: "warn"
  LOG_FORMAT: "json"
  DB_HOST: "llmvault-db.cluster-xxx.us-east-1.rds.amazonaws.com"
  DB_PORT: "5432"
  DB_USER: "llmvault"
  DB_NAME: "llmvault"
  DB_SSLMODE: "require"
  REDIS_URL: "rediss://llmvault-cache.xxx.cache.amazonaws.com:6379"
  REDIS_CACHE_TTL: "30m"
  MEM_CACHE_TTL: "5m"
  MEM_CACHE_MAX_SIZE: "10000"
  KMS_TYPE: "awskms"
  KMS_KEY: "alias/llmvault-production"
  AWS_REGION: "us-east-1"
  LOGTO_ENDPOINT: "https://auth.yourcompany.com"
  LOGTO_AUDIENCE: "https://api.llmvault.dev"
  NANGO_ENDPOINT: "https://integrations.yourcompany.com"
  CORS_ORIGINS: "https://vault.yourcompany.com"
```

**Secret (sensitive):**
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: llmvault-secrets
type: Opaque
stringData:
  DB_PASSWORD: "your-secure-password"
  JWT_SIGNING_KEY: "your-jwt-signing-key"
  LOGTO_M2M_APP_ID: "your-app-id"
  LOGTO_M2M_APP_SECRET: "your-app-secret"
  NANGO_SECRET_KEY: "your-secret-key"
```

## Security Best Practices

1. **Use secrets management:**
   - Kubernetes secrets
   - AWS Secrets Manager / Parameter Store
   - HashiCorp Vault
   - Docker secrets

2. **Rotate keys regularly:**
   - JWT signing keys
   - Database passwords
   - API keys

3. **Use different credentials per environment:**
   - Never share production credentials with staging/dev

4. **Enable encryption in transit:**
   - `DB_SSLMODE=require` or stricter
   - `rediss://` for Redis (TLS)
   - HTTPS for all external endpoints

5. **Restrict CORS origins:**
   - Never use `*` in production
   - Specify exact domains

## Related Guides

- [Configuration Reference](./configuration) - Detailed configuration options
- [Docker Compose Deployment](./docker-compose) - Docker deployment
- [Kubernetes Deployment](./kubernetes) - Kubernetes deployment
