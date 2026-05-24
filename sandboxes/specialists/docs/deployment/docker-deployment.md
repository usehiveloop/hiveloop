# Docker Deployment

Run Bridge in Docker containers.

---

## Dockerfile

Create a `Dockerfile`:

```dockerfile
# Build stage
FROM rust:1.82 as builder

WORKDIR /app
COPY . .
RUN cargo build --release -p bridge

# Runtime stage
FROM debian:bookworm-slim

RUN apt-get update && \
    apt-get install -y ca-certificates curl && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=builder /app/target/release/bridge /usr/local/bin/bridge

EXPOSE 8080

USER nobody

HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
  CMD curl -f http://localhost:8080/health || exit 1

CMD ["bridge"]
```

**Notes:**
- Uses `rust:1.82` for building (match your MSRV)
- Runtime image: `debian:bookworm-slim` (~30MB base)
- Installs `ca-certificates` for HTTPS and `curl` for health checks
- Runs as `nobody` user for security
- Health check hits `/health` endpoint every 30s

---

## Build

```bash
docker build -t bridge:latest .
```

---

## Run

```bash
docker run -d \
  --name bridge \
  -p 8080:8080 \
  -e BRIDGE_CONTROL_PLANE_API_KEY="<bridge-control-plane-api-key>" \
  -e BRIDGE_LOG_FORMAT="json" \
  -e BRIDGE_WEBHOOK_URL="https://api.example.com/webhooks" \
  -e BRIDGE_DRAIN_TIMEOUT_SECS="60" \
  --restart unless-stopped \
  bridge:latest
```

---

## Docker Compose

Create `docker-compose.yml`:

```yaml
version: "3.8"

services:
  bridge:
    build: .
    ports:
      - "8080:8080"
    environment:
      - BRIDGE_CONTROL_PLANE_API_KEY=${BRIDGE_API_KEY}
      - BRIDGE_LOG_FORMAT=json
      - BRIDGE_LOG_LEVEL=info
      - BRIDGE_WEBHOOK_URL=${WEBHOOK_URL}
      - BRIDGE_DRAIN_TIMEOUT_SECS=60
    volumes:
      - ./config.toml:/app/config.toml:ro
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8080/health"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 10s
    restart: unless-stopped
    deploy:
      resources:
        limits:
          cpus: '2'
          memory: 2G
        reservations:
          cpus: '500m'
          memory: 512M
```

Create `.env`:

```bash
BRIDGE_API_KEY=<bridge-control-plane-api-key>
WEBHOOK_URL=https://api.example.com/webhooks
```

Run:

```bash
docker-compose up -d
```

**Resource limits:**
- CPU: 2 cores limit, 0.5 cores reserved
- Memory: 2GB limit, 512MB reserved
- Adjust based on your agent workload

---

## Multi-Instance with Compose

```yaml
version: "3.8"

services:
  bridge-1:
    build: .
    ports:
      - "8081:8080"
    environment:
      - BRIDGE_CONTROL_PLANE_API_KEY=${BRIDGE_API_KEY}
    # ...

  bridge-2:
    build: .
    ports:
      - "8082:8080"
    environment:
      - BRIDGE_CONTROL_PLANE_API_KEY=${BRIDGE_API_KEY}
    # ...

  nginx:
    image: nginx:alpine
    ports:
      - "8080:80"
    volumes:
      - ./nginx.conf:/etc/nginx/nginx.conf:ro
    depends_on:
      - bridge-1
      - bridge-2
```

---

## Secrets Management

Use Docker secrets with an entrypoint script:

```yaml
version: "3.8"

services:
  bridge:
    image: bridge:latest
    secrets:
      - bridge_api_key
    environment:
      - BRIDGE_CONTROL_PLANE_API_KEY_FILE=/run/secrets/bridge_api_key
    entrypoint: ["sh", "-c"]
    command: >
      'export BRIDGE_CONTROL_PLANE_API_KEY=$$(cat /run/secrets/bridge_api_key) &&
       exec bridge'

secrets:
  bridge_api_key:
    external: true
```

Create the secret:

```bash
echo "<bridge-control-plane-api-key>" | docker secret create bridge_api_key -
```

**Note:** Bridge does not natively support the `_FILE` suffix pattern. The entrypoint script reads the secret file and exports it as the environment variable.

---

## Graceful Shutdown

Bridge handles SIGTERM for graceful shutdown:

```yaml
services:
  bridge:
    image: bridge:latest
    stop_signal: SIGTERM
    stop_grace_period: 90s  # Should be > DRAIN_TIMEOUT_SECS
```

On shutdown:
1. Container receives SIGTERM
2. Bridge stops accepting new connections
3. Waits up to `DRAIN_TIMEOUT_SECS` (default: 60s) for active conversations
4. Exits cleanly

---

## Logging

View logs:

```bash
docker logs -f bridge
```

With JSON format, pipe to jq:

```bash
docker logs bridge 2>&1 | jq .
```

Configure log rotation:

```yaml
services:
  bridge:
    logging:
      driver: "json-file"
      options:
        max-size: "100m"
        max-file: "3"
```

---

## Updates

Update to new version:

```bash
# Pull new code
git pull

# Rebuild
docker-compose build

# Restart with graceful shutdown
docker-compose up -d
```

---

## See Also

- [Binary Deployment](binary-deployment.md) — Non-Docker option
- [Kubernetes](kubernetes.md) — Orchestrated deployment
