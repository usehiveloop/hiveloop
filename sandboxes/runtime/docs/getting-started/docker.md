# Docker Deployment

Run Bridge in Docker for consistent, portable deployments.

---

## Quick Start with Docker

### 1. Create a Dockerfile

```dockerfile
FROM rust:1.75 as builder

WORKDIR /app
COPY . .
RUN cargo build --release

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=builder /app/target/release/bridge /usr/local/bin/bridge

EXPOSE 8080

CMD ["bridge"]
```

### 2. Build the Image

```bash
docker build -t bridge:latest .
```

### 3. Run the Container

```bash
docker run -p 8080:8080 \
  -e BRIDGE_CONTROL_PLANE_API_KEY="<bridge-control-plane-api-key>" \
  -e BRIDGE_LOG_FORMAT="json" \
  bridge:latest
```

---

## Using Docker Compose

Create a `docker-compose.yml`:

```yaml
version: "3.8"

services:
  bridge:
    build: .
    ports:
      - "8080:8080"
    environment:
      - BRIDGE_CONTROL_PLANE_API_KEY=${BRIDGE_CONTROL_PLANE_API_KEY}
      - BRIDGE_LOG_LEVEL=info
      - BRIDGE_LOG_FORMAT=json
      - BRIDGE_WEBHOOK_URL=${WEBHOOK_URL}
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8080/health"]
      interval: 30s
      timeout: 10s
      retries: 3
    restart: unless-stopped
```

Create a `.env` file:

```bash
BRIDGE_CONTROL_PLANE_API_KEY=<bridge-control-plane-api-key>
WEBHOOK_URL=https://your-api.com/webhooks/bridge
```

Run it:

```bash
docker-compose up
```

---

## Multi-Stage Build (Recommended)

The Dockerfile above uses multi-stage builds to keep the final image small (~50MB instead of ~1GB).

If you need additional tools in the final image (like `git` for some MCP servers):

```dockerfile
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y \
    ca-certificates \
    git \
    && rm -rf /var/lib/apt/lists/*

# ... rest of the Dockerfile
```

---

## Mounting Config Files

Instead of environment variables, you can mount a config file:

```yaml
services:
  bridge:
    build: .
    ports:
      - "8080:8080"
    volumes:
      - ./config.toml:/app/config.toml:ro
    restart: unless-stopped
```

---

## Health Checks

Bridge provides a `/health` endpoint. Configure Docker to use it:

```dockerfile
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
  CMD curl -f http://localhost:8080/health || exit 1
```

Or in docker-compose:

```yaml
healthcheck:
  test: ["CMD", "wget", "--quiet", "--tries=1", "--spider", "http://localhost:8080/health"]
  interval: 30s
  timeout: 10s
  retries: 3
```

---

## Production Considerations

### Secrets Management

Don't put secrets in your Dockerfile or docker-compose.yml. Use:

- Docker secrets (Swarm mode)
- Environment files
- External secret stores (Vault, AWS Secrets Manager)

Example with Docker secrets:

```yaml
services:
  bridge:
    image: bridge:latest
    secrets:
      - bridge_api_key
    environment:
      - BRIDGE_CONTROL_PLANE_API_KEY_FILE=/run/secrets/bridge_api_key

secrets:
  bridge_api_key:
    external: true
```

### Logging

Use `json` format for production to make parsing easier:

```yaml
environment:
  - BRIDGE_LOG_FORMAT=json
```

### Resource Limits

Set appropriate limits:

```yaml
deploy:
  resources:
    limits:
      cpus: '2'
      memory: 2G
    reservations:
      cpus: '1'
      memory: 512M
```

---

## See Also

- [Binary Deployment](../deployment/binary-deployment.md) — Run without Docker
- [Kubernetes](../deployment/kubernetes.md) — Orchestrated deployments
- [Monitoring](../deployment/monitoring.md) — Observability setup
