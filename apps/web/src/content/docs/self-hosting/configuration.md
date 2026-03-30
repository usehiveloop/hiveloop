---
title: Configuration Reference
description: Detailed configuration options for LLMVault services
---

# Configuration Reference

This guide covers detailed configuration options for all LLMVault components.

## Database Configuration

### PostgreSQL

LLMVault uses PostgreSQL as its primary database for storing organizations, integrations, and encrypted credentials.

#### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `DB_HOST` | `localhost` | PostgreSQL server hostname |
| `DB_PORT` | `5432` | PostgreSQL server port |
| `DB_USER` | `llmvault` | Database username |
| `DB_PASSWORD` | *required* | Database password |
| `DB_NAME` | `llmvault` | Database name |
| `DB_SSLMODE` | `disable` | SSL mode (`disable`, `require`, `verify-ca`, `verify-full`) |

#### SSL Modes

| Mode | Description | Use Case |
|------|-------------|----------|
| `disable` | No SSL/TLS | Local development only |
| `require` | SSL required, no cert verification | Production with trusted network |
| `verify-ca` | SSL with CA verification | Production with custom CA |
| `verify-full` | SSL with hostname verification | Maximum security |

#### Connection String (Advanced)

For complex connection scenarios, you can use `DATABASE_URL`:

```bash
DATABASE_URL=postgres://user:password@host:port/dbname?sslmode=require&connect_timeout=10
```

#### Docker Compose Example

```yaml
services:
  postgres:
    image: postgres:17-alpine
    environment:
      POSTGRES_DB: llmvault
      POSTGRES_USER: llmvault
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}
    volumes:
      - pgdata:/var/lib/postgresql/data
      - ./docker/postgres/init.sql:/docker-entrypoint-initdb.d/01-init.sql
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U llmvault"]
      interval: 5s
      timeout: 5s
      retries: 10
```

#### AWS RDS Example

```yaml
# llmvault-values.yaml (Helm)
environment:
  DB_HOST: "llmvault-db.cluster-xxx.us-east-1.rds.amazonaws.com"
  DB_PORT: "5432"
  DB_USER: "llmvault"
  DB_NAME: "llmvault"
  DB_SSLMODE: require

secrets:
  DB_PASSWORD: "your-rds-password"
```

#### Database Initialization

When using Docker Compose, an init script (`docker/postgres/init.sql`) runs automatically to set up the required database extensions. No manual database setup is needed — the schema is managed by LLMVault and migrations run automatically on startup.

## Redis Configuration

### Connection Options

LLMVault supports two ways to connect to Redis:

#### Option 1: Redis URL (Recommended for TLS)

| Variable | Description |
|----------|-------------|
| `REDIS_URL` | Full Redis URL with auth and options |

Examples:

```bash
# Standard Redis
REDIS_URL=redis://localhost:6379/0

# Redis with password
REDIS_URL=redis://:password@localhost:6379/0

# Redis with TLS (AWS ElastiCache)
REDIS_URL=rediss://llmvault-cache.xxx.cache.amazonaws.com:6379

# Redis with query params
REDIS_URL=redis://localhost:6379/0?pool_size=10&min_idle_conns=5
```

#### Option 2: Individual Settings

| Variable | Default | Description |
|----------|---------|-------------|
| `REDIS_ADDR` | `localhost:6379` | Redis host:port |
| `REDIS_PASSWORD` | *empty* | Redis password |
| `REDIS_DB` | `0` | Database number (0-15) |

### Cache Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `REDIS_CACHE_TTL` | `30m` | Time-to-live for cached credentials |

Valid TTL formats: `30s`, `5m`, `1h`, `24h`

### Docker Compose Example

```yaml
services:
  redis:
    image: redis:7-alpine
    command: >
      redis-server
      --appendonly yes
      --maxmemory 512mb
      --maxmemory-policy allkeys-lru
    volumes:
      - redisdata:/data
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 5s
      retries: 5
```

### AWS ElastiCache Example

```bash
# Primary endpoint (with cluster mode disabled)
REDIS_URL=rediss://master.llmvault-cache.xxx.usw2.cache.amazonaws.com:6379

# Configuration endpoint (with cluster mode enabled)
REDIS_URL=rediss://clustercfg.llmvault-cache.xxx.usw2.cache.amazonaws.com:6379
```

Note: AWS ElastiCache uses `rediss://` (with double 's') for TLS connections.

## KMS Provider Configuration

LLMVault supports three KMS providers for envelope encryption:

### 1. AEAD (Development Only)

Local AES-256-GCM encryption using a base64-encoded key. **Not recommended for production.**

| Variable | Description |
|----------|-------------|
| `KMS_TYPE` | Set to `aead` |
| `KMS_KEY` | Base64-encoded 32-byte key |

Generate a key:

```bash
openssl rand -base64 32
# Output: abc123... (44 characters)
```

Example:

```bash
KMS_TYPE=aead
KMS_KEY=YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXoxMjM0NTY=
```

**Security Warning:** With AEAD, losing the key means losing access to all encrypted credentials. Store the key securely (e.g., AWS Secrets Manager, Kubernetes secrets).

### 2. AWS KMS

Production-ready KMS using AWS Key Management Service.

| Variable | Description |
|----------|-------------|
| `KMS_TYPE` | Set to `awskms` |
| `KMS_KEY` | KMS Key ID, ARN, or alias |
| `AWS_REGION` | AWS region (e.g., `us-east-1`) |

AWS credentials are resolved via the standard credential chain:

1. Environment variables (`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`)
2. Shared credentials file (`~/.aws/credentials`)
3. IAM role (EC2 instance profile, ECS task role, EKS IRSA)

#### Examples

```bash
# Using key alias
KMS_TYPE=awskms
KMS_KEY=alias/llmvault-production
AWS_REGION=us-east-1

# Using key ID
KMS_TYPE=awskms
KMS_KEY=arn:aws:kms:us-east-1:123456789:key/12345678-1234-1234-1234-123456789012
AWS_REGION=us-east-1
```

#### IAM Policy Required

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "kms:Encrypt",
        "kms:Decrypt",
        "kms:ReEncryptFrom",
        "kms:ReEncryptTo",
        "kms:GenerateDataKey",
        "kms:GenerateDataKeyWithoutPlaintext",
        "kms:DescribeKey"
      ],
      "Resource": "arn:aws:kms:us-east-1:ACCOUNT:key/KEY-ID"
    }
  ]
}
```

#### Setup Script

Use the provided script to create KMS keys:

```bash
./scripts/setup-aws-kms.sh prod
```

This creates:
- KMS key with alias
- IAM user for application access
- IAM policy with required permissions
- Credentials file for local development

### 3. HashiCorp Vault

Production-ready KMS using HashiCorp Vault Transit engine.

| Variable | Default | Description |
|----------|---------|-------------|
| `KMS_TYPE` | - | Set to `vault` |
| `KMS_KEY` | - | Name of the encryption key in Vault |
| `VAULT_ADDRESS` | - | Vault server URL (e.g., `https://vault.example.com:8200`) |
| `VAULT_TOKEN` | - | Vault authentication token |
| `VAULT_NAMESPACE` | *empty* | Vault Enterprise namespace |
| `VAULT_MOUNT_PATH` | `transit` | Transit engine mount path |
| `VAULT_CA_CERT` | *empty* | Path to CA certificate for TLS |
| `VAULT_CLIENT_CERT` | *empty* | Path to client certificate |
| `VAULT_CLIENT_KEY` | *empty* | Path to client key |

#### Examples

```bash
# Basic Vault setup
KMS_TYPE=vault
KMS_KEY=llmvault-key
VAULT_ADDRESS=https://vault.internal.company.com:8200
VAULT_TOKEN=s.token-from-auth-method

# With TLS certificates
KMS_TYPE=vault
KMS_KEY=llmvault-key
VAULT_ADDRESS=https://vault.internal.company.com:8200
VAULT_TOKEN=s.token-from-auth-method
VAULT_CA_CERT=/certs/ca.crt
VAULT_CLIENT_CERT=/certs/client.crt
VAULT_CLIENT_KEY=/certs/client.key

# With custom mount path
KMS_TYPE=vault
KMS_KEY=llmvault-key
VAULT_ADDRESS=https://vault.internal.company.com:8200
VAULT_TOKEN=s.token-from-auth-method
VAULT_MOUNT_PATH=security/transit
```

#### Vault Setup

1. **Enable Transit engine:**

```bash
vault secrets enable -path=transit transit
```

2. **Create encryption key:**

```bash
vault write -f transit/keys/llmvault-key
```

3. **Create policy:**

```hcl
# llmvault-policy.hcl
path "transit/keys/llmvault-key" {
  capabilities = ["read"]
}

path "transit/encrypt/llmvault-key" {
  capabilities = ["update"]
}

path "transit/decrypt/llmvault-key" {
  capabilities = ["update"]
}
```

```bash
vault policy write llmvault-production llmvault-policy.hcl
```

4. **Configure authentication:**

Choose one authentication method:

**AppRole (for VMs/containers):**

```bash
vault auth enable approle

vault write auth/approle/role/llmvault \
  token_policies="llmvault-production" \
  token_ttl=1h \
  token_max_ttl=4h

# Get credentials
vault read auth/approle/role/llmvault/role-id
vault write -f auth/approle/role/llmvault/secret-id
```

**Kubernetes (for EKS):**

```bash
vault auth enable kubernetes

vault write auth/kubernetes/config \
  kubernetes_host="https://$KUBERNETES_PORT_443_TCP_ADDR:443" \
  token_reviewer_jwt="$(cat /var/run/secrets/kubernetes.io/serviceaccount/token)" \
  kubernetes_ca_cert=@/var/run/secrets/kubernetes.io/serviceaccount/ca.crt

vault write auth/kubernetes/role/llmvault \
  bound_service_account_names=llmvault \
  bound_service_account_namespaces=llmvault \
  policies=llmvault-production \
  ttl=1h
```

For detailed Vault production setup, see [Vault Production Setup](/docs/vault-production-setup).

## Authentication Configuration

LLMVault uses embedded Go authentication with RSA key signing for JWT tokens.

### Configuration

| Variable | Description |
|----------|-------------|
| `AUTH_RSA_PRIVATE_KEY` | Base64-encoded RSA private key PEM |
| `AUTH_ISSUER` | JWT issuer (default: `llmvault`) |
| `AUTH_AUDIENCE` | JWT audience (default: `https://api.llmvault.dev`) |

### Generating Keys

```bash
# Generate base64-encoded RSA private key
openssl genrsa 2048 | base64 | tr -d '\n'
```

### Example

```bash
AUTH_RSA_PRIVATE_KEY=<base64-encoded RSA private key>
AUTH_ISSUER=llmvault
AUTH_AUDIENCE=https://api.llmvault.dev
```

## Nango Configuration

Nango provides OAuth integration management for 250+ providers.

| Variable | Description |
|----------|-------------|
| `NANGO_ENDPOINT` | Nango server URL |
| `NANGO_SECRET_KEY` | Nango secret key for API authentication |

### Example

```bash
# Using hosted Nango
NANGO_ENDPOINT=https://integrations.llmvault.dev
NANGO_SECRET_KEY=your-secret-key

# Self-hosted Nango
NANGO_ENDPOINT=https://integrations.internal.company.com
NANGO_SECRET_KEY=your-secret-key
```

## In-Memory Cache Configuration

LLMVault uses an L1 in-memory cache (LRU cache) in addition to Redis (L2 cache).

| Variable | Default | Description |
|----------|---------|-------------|
| `MEM_CACHE_TTL` | `5m` | Time-to-live for in-memory cached credentials |
| `MEM_CACHE_MAX_SIZE` | `10000` | Maximum number of items in the LRU cache |

### Example

```bash
# Production (shorter TTL for security)
MEM_CACHE_TTL=5m
MEM_CACHE_MAX_SIZE=10000

# Development (longer TTL for convenience)
MEM_CACHE_TTL=30m
MEM_CACHE_MAX_SIZE=5000
```

## JWT Configuration

JWT signing key for sandbox proxy tokens.

| Variable | Description |
|----------|-------------|
| `JWT_SIGNING_KEY` | HMAC-SHA256 signing key (minimum 32 characters) |

Generate a secure key:

```bash
openssl rand -hex 32
```

**Security Note:** This key is used to sign tokens for the LLM proxy. If compromised, an attacker could impersonate users. Rotate this key periodically.

## CORS Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `CORS_ORIGINS` | *empty* | Comma-separated list of allowed origins |

### Examples

```bash
# Single origin
CORS_ORIGINS=https://vault.yourcompany.com

# Multiple origins
CORS_ORIGINS=https://vault.yourcompany.com,https://admin.yourcompany.com,https://app.yourcompany.com

# Development (not for production)
CORS_ORIGINS=http://localhost:3000,http://localhost:30112
```

## Server Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | HTTP server port |
| `LOG_LEVEL` | `info` | Log level (`debug`, `info`, `warn`, `error`) |
| `LOG_FORMAT` | `json` | Log format (`json`, `text`) |
| `ENVIRONMENT` | `development` | Environment (`development`, `production`) |

### Log Levels

| Level | Description |
|-------|-------------|
| `debug` | Detailed information for debugging |
| `info` | General operational information |
| `warn` | Warning conditions |
| `error` | Error conditions |

### Example

```bash
# Production
ENVIRONMENT=production
PORT=8080
LOG_LEVEL=warn
LOG_FORMAT=json

# Development
ENVIRONMENT=development
PORT=8080
LOG_LEVEL=debug
LOG_FORMAT=text
```

## MCP Server Configuration

The Model Context Protocol (MCP) server runs as a separate service for AI agent integrations.

| Variable | Default | Description |
|----------|---------|-------------|
| `MCP_PORT` | `8081` | MCP server port |
| `MCP_BASE_URL` | `http://localhost:8081` | Base URL for MCP server |

### Example

```bash
MCP_PORT=8081
MCP_BASE_URL=https://mcp.yourcompany.com
```

## Complete Configuration Examples

### Development Configuration

```bash
# .env - Development
ENVIRONMENT=development
PORT=8080
LOG_LEVEL=debug
LOG_FORMAT=text

# Database
DB_HOST=localhost
DB_PORT=5433
DB_USER=llmvault
DB_PASSWORD=localdev
DB_NAME=llmvault
DB_SSLMODE=disable

# KMS (AEAD for dev)
KMS_TYPE=aead
KMS_KEY=your-base64-encoded-key

# Redis
REDIS_ADDR=localhost:6379
REDIS_PASSWORD=
REDIS_DB=0
REDIS_CACHE_TTL=30m

# Cache
MEM_CACHE_TTL=5m
MEM_CACHE_MAX_SIZE=10000

# JWT
JWT_SIGNING_KEY=dev-signing-key-not-for-production

# CORS
CORS_ORIGINS=http://localhost:3000,http://localhost:30112

# Auth (built-in — generate with: openssl genrsa 2048 | base64 | tr -d '\n')
AUTH_RSA_PRIVATE_KEY=<base64-encoded RSA private key>
AUTH_ISSUER=llmvault
AUTH_AUDIENCE=https://api.llmvault.dev

# Nango (staging)
NANGO_ENDPOINT=https://integrations.dev.llmvault.dev
NANGO_SECRET_KEY=your-staging-secret-key
```

### Production Configuration

```bash
# .env - Production
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

# JWT (from secrets manager)
JWT_SIGNING_KEY=your-production-signing-key

# CORS
CORS_ORIGINS=https://vault.yourcompany.com

# Auth (built-in — generate with: openssl genrsa 2048 | base64 | tr -d '\n')
AUTH_RSA_PRIVATE_KEY=<base64-encoded RSA private key>
AUTH_ISSUER=llmvault
AUTH_AUDIENCE=https://api.llmvault.dev

# Nango
NANGO_ENDPOINT=https://integrations.yourcompany.com
NANGO_SECRET_KEY=your-production-secret-key
```

## Related Guides

- [Environment Variables](./environment) - Complete reference table
- [Docker Compose Deployment](./docker-compose) - Docker deployment guide
- [Kubernetes Deployment](./kubernetes) - Kubernetes deployment guide
