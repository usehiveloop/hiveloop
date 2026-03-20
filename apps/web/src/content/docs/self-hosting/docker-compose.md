---
title: Docker Compose Deployment
description: Deploy LLMVault using Docker Compose with all required services
---

# Docker Compose Deployment

This guide walks you through deploying LLMVault using Docker Compose. This is the quickest way to get started with self-hosting.

## Prerequisites

### Required Software

| Tool | Version | Purpose |
|------|---------|---------|
| Docker | 24.0+ | Container runtime |
| Docker Compose | 2.20+ | Multi-container orchestration |
| OpenSSL | Any | Key generation |

### System Requirements

- **OS**: Linux, macOS, or Windows (WSL2)
- **CPU**: 2+ cores
- **Memory**: 2GB+ available for containers
- **Storage**: 10GB+ free space

## Quick Start

### 1. Clone the Repository

```bash
git clone https://github.com/llmvault/llmvault.git
cd llmvault
```

### 2. Create Environment File

```bash
cp .env.example .env
```

Generate required secrets:

```bash
# Generate a secure JWT signing key
export JWT_SIGNING_KEY=$(openssl rand -hex 32)
echo "JWT_SIGNING_KEY=$JWT_SIGNING_KEY" >> .env

# Generate KMS key for development (base64-encoded 32-byte key)
export KMS_KEY=$(openssl rand -base64 32)
echo "KMS_KEY=$KMS_KEY" >> .env

# Set PostgreSQL password
export POSTGRES_PASSWORD=$(openssl rand -base64 24)
echo "POSTGRES_PASSWORD=$POSTGRES_PASSWORD" >> .env
```

### 3. Configure Environment Variables

Edit `.env` with your settings:

```bash
# === Required Settings ===

# Environment
ENVIRONMENT=production

# Server
PORT=8080
LOG_LEVEL=info
LOG_FORMAT=json

# Database
DB_HOST=postgres
DB_PORT=5432
DB_USER=llmvault
DB_PASSWORD=${POSTGRES_PASSWORD}
DB_NAME=llmvault
DB_SSLMODE=disable

# KMS (use 'vault' for production)
KMS_TYPE=aead
KMS_KEY=${KMS_KEY}

# Redis
REDIS_ADDR=redis:6379
REDIS_PASSWORD=
REDIS_DB=0
REDIS_CACHE_TTL=30m

# L1 Cache
MEM_CACHE_TTL=5m
MEM_CACHE_MAX_SIZE=10000

# JWT
JWT_SIGNING_KEY=${JWT_SIGNING_KEY}

# CORS (your dashboard domain)
CORS_ORIGINS=https://vault.yourcompany.com

# Logto (use hosted or self-hosted)
LOGTO_ENDPOINT=https://auth.yourcompany.com
LOGTO_AUDIENCE=https://api.llmvault.dev
LOGTO_M2M_APP_ID=your-m2m-app-id
LOGTO_M2M_APP_SECRET=your-m2m-app-secret

# Nango (use hosted or self-hosted)
NANGO_ENDPOINT=https://integrations.yourcompany.com
NANGO_SECRET_KEY=your-nango-secret-key
```

### 4. Start Infrastructure Services

```bash
# Start PostgreSQL, Redis, and Vault
make vault-up
```

Or manually:

```bash
docker compose up -d postgres redis vault
```

Wait for services to be healthy:

```bash
# Postgres
until docker compose exec -T postgres pg_isready -U llmvault -q 2>/dev/null; do sleep 1; done
echo "✓ Postgres ready"

# Redis
until docker compose exec -T redis redis-cli ping 2>/dev/null | grep -q PONG; do sleep 1; done
echo "✓ Redis ready"

# Vault
until docker compose exec -T vault vault status 2>/dev/null | grep -q "Version"; do sleep 1; done
echo "✓ Vault ready"
```

### 5. Build and Run LLMVault

```bash
# Build the Docker image
make docker-build

# Or build manually
docker build \
  --build-arg VERSION=$(git describe --tags --always) \
  --build-arg COMMIT=$(git rev-parse --short HEAD) \
  -t llmvault/llmvault:latest \
  -f Dockerfile .
```

Run the container:

```bash
docker run -d \
  --name llmvault \
  --network host \
  -e ENVIRONMENT=production \
  -e PORT=8080 \
  -e DB_HOST=localhost \
  -e DB_PORT=5433 \
  -e DB_USER=llmvault \
  -e DB_PASSWORD="${POSTGRES_PASSWORD}" \
  -e DB_NAME=llmvault \
  -e DB_SSLMODE=disable \
  -e KMS_TYPE=aead \
  -e KMS_KEY="${KMS_KEY}" \
  -e REDIS_ADDR=localhost:6379 \
  -e JWT_SIGNING_KEY="${JWT_SIGNING_KEY}" \
  -e LOGTO_ENDPOINT="${LOGTO_ENDPOINT}" \
  -e LOGTO_AUDIENCE="${LOGTO_AUDIENCE}" \
  -e LOGTO_M2M_APP_ID="${LOGTO_M2M_APP_ID}" \
  -e LOGTO_M2M_APP_SECRET="${LOGTO_M2M_APP_SECRET}" \
  -e NANGO_ENDPOINT="${NANGO_ENDPOINT}" \
  -e NANGO_SECRET_KEY="${NANGO_SECRET_KEY}" \
  -p 80:80 \
  -p 8080:8080 \
  llmvault/llmvault:latest
```

### 6. Verify Deployment

```bash
# Check health endpoint
curl http://localhost/health

# Expected output:
healthy
```

## Docker Compose Configuration

### Full Production Stack

Create `docker-compose.prod.yml`:

```yaml
version: '3.8'

services:
  llmvault:
    image: llmvault/llmvault:latest
    ports:
      - "80:80"
      - "8080:8080"
    environment:
      - ENVIRONMENT=production
      - PORT=8080
      - LOG_LEVEL=info
      - LOG_FORMAT=json
      - DB_HOST=postgres
      - DB_PORT=5432
      - DB_USER=llmvault
      - DB_PASSWORD=${POSTGRES_PASSWORD}
      - DB_NAME=llmvault
      - DB_SSLMODE=disable
      - KMS_TYPE=vault
      - KMS_KEY=llmvault-key
      - VAULT_ADDRESS=http://vault:8200
      - VAULT_TOKEN=${VAULT_TOKEN}
      - REDIS_ADDR=redis:6379
      - REDIS_CACHE_TTL=30m
      - MEM_CACHE_TTL=5m
      - MEM_CACHE_MAX_SIZE=10000
      - JWT_SIGNING_KEY=${JWT_SIGNING_KEY}
      - CORS_ORIGINS=${CORS_ORIGINS}
      - LOGTO_ENDPOINT=${LOGTO_ENDPOINT}
      - LOGTO_AUDIENCE=${LOGTO_AUDIENCE}
      - LOGTO_M2M_APP_ID=${LOGTO_M2M_APP_ID}
      - LOGTO_M2M_APP_SECRET=${LOGTO_M2M_APP_SECRET}
      - NANGO_ENDPOINT=${NANGO_ENDPOINT}
      - NANGO_SECRET_KEY=${NANGO_SECRET_KEY}
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy
      vault:
        condition: service_healthy
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "wget", "-q", "--spider", "http://localhost:8080/health"]
      interval: 10s
      timeout: 5s
      retries: 3

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
    restart: unless-stopped

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
    restart: unless-stopped

  vault:
    image: hashicorp/vault:1.18
    cap_add:
      - IPC_LOCK
    environment:
      VAULT_DEV_ROOT_TOKEN_ID: ${VAULT_TOKEN}
      VAULT_DEV_LISTEN_ADDRESS: 0.0.0.0:8200
    volumes:
      - vaultdata:/vault/file
      - ./docker/vault/init.sh:/vault/init.sh:ro
    command: >
      sh -c "
        vault server -dev -dev-root-token-id=$${VAULT_DEV_ROOT_TOKEN_ID} -dev-listen-address=0.0.0.0:8200 &
        sleep 2 &&
        export VAULT_ADDR=http://localhost:8200 &&
        export VAULT_TOKEN=$${VAULT_DEV_ROOT_TOKEN_ID} &&
        /vault/init.sh &&
        wait
      "
    healthcheck:
      test: ["CMD", "vault", "status"]
      interval: 5s
      timeout: 5s
      retries: 10
      start_period: 10s

volumes:
  pgdata:
  redisdata:
  vaultdata:
```

### With External Services

If using managed services (RDS, ElastiCache, HCP Vault):

```yaml
services:
  llmvault:
    image: llmvault/llmvault:latest
    ports:
      - "80:80"
      - "8080:8080"
    environment:
      - ENVIRONMENT=production
      - PORT=8080
      - DB_HOST=${RDS_HOSTNAME}
      - DB_PORT=5432
      - DB_USER=${RDS_USERNAME}
      - DB_PASSWORD=${RDS_PASSWORD}
      - DB_NAME=llmvault
      - DB_SSLMODE=require
      - KMS_TYPE=awskms
      - KMS_KEY=${AWS_KMS_KEY_ID}
      - AWS_REGION=${AWS_REGION}
      - REDIS_URL=${ELASTICACHE_URL}
      - JWT_SIGNING_KEY=${JWT_SIGNING_KEY}
      - LOGTO_ENDPOINT=${LOGTO_ENDPOINT}
      - LOGTO_AUDIENCE=${LOGTO_AUDIENCE}
      - LOGTO_M2M_APP_ID=${LOGTO_M2M_APP_ID}
      - LOGTO_M2M_APP_SECRET=${LOGTO_M2M_APP_SECRET}
      - NANGO_ENDPOINT=${NANGO_ENDPOINT}
      - NANGO_SECRET_KEY=${NANGO_SECRET_KEY}
```

## Upgrading

### Standard Upgrade Process

1. **Backup your data**:

```bash
# Backup PostgreSQL
docker compose exec postgres pg_dump -U llmvault llmvault > backup_$(date +%Y%m%d).sql

# Backup Vault (if using dev mode)
docker compose exec vault vault operator raft snapshot save /tmp/vault.snap
docker cp llmvault-vault-1:/tmp/vault.snap ./vault_backup_$(date +%Y%m%d).snap
```

2. **Pull the latest image**:

```bash
docker pull llmvault/llmvault:latest
```

3. **Stop and recreate containers**:

```bash
docker compose down
docker compose up -d
```

4. **Verify the upgrade**:

```bash
# Check logs
docker compose logs -f llmvault

# Test health endpoint
curl http://localhost/health
```

### Database Migrations

LLMVault uses GORM auto-migration. Database schema updates happen automatically on startup. For large deployments, plan for brief downtime during major version upgrades.

## Troubleshooting

### Service Won't Start

```bash
# Check logs
docker compose logs llmvault

# Check environment variables
docker compose config

# Verify database connection
docker compose exec llmvault sh -c 'nc -zv $DB_HOST $DB_PORT'
```

### Database Connection Issues

```bash
# Test from inside the container
docker compose exec llmvault sh -c 'psql $DATABASE_URL -c "SELECT 1;"'

# Check PostgreSQL logs
docker compose logs postgres
```

### KMS Errors

```bash
# For Vault - check Vault status
docker compose exec vault vault status

# For AWS KMS - verify credentials
docker compose exec llmvault aws kms describe-key --key-id $KMS_KEY
```

### High Memory Usage

```bash
# Check memory stats
docker stats

# Adjust Redis maxmemory
docker compose exec redis redis-cli CONFIG SET maxmemory 256mb
```

## Production Checklist

Before deploying to production:

- [ ] Use external KMS (AWS KMS or HashiCorp Vault with auto-unseal)
- [ ] Enable PostgreSQL SSL (`DB_SSLMODE=require`)
- [ ] Enable Redis persistence (`--appendonly yes`)
- [ ] Use strong, unique passwords for all services
- [ ] Configure proper CORS origins (not `*`)
- [ ] Set up log aggregation
- [ ] Configure backup jobs
- [ ] Set up monitoring and alerting
- [ ] Use TLS certificates (not self-signed)
- [ ] Restrict network access (internal services not exposed)

## Related Guides

- [Configuration Reference](./configuration) - Detailed configuration options
- [Environment Variables](./environment) - Complete environment variable reference
- [Kubernetes Deployment](./kubernetes) - For high-availability deployments
