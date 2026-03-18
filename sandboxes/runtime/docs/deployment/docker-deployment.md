# Docker Deployment

Run Bridge in Docker containers.

---

## Dockerfile

Create a `Dockerfile`:

```dockerfile
# Build stage
FROM rust:1.75 as builder

WORKDIR /app
COPY . .
RUN cargo build --release

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
    volumes:
      - ./config.toml:/app/config.toml:ro
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8080/health"]
      interval: 30s
      timeout: 10s
      retries: 3
    restart: unless-stopped
    deploy:
      resources:
        limits:
          cpus: '2'
          memory: 2G
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

Use Docker secrets (Swarm mode):

```yaml
version: "3.8"

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

Create the secret:

```bash
echo "<bridge-control-plane-api-key>" | docker secret create bridge_api_key -
```

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

---

## Updates

Update to new version:

```bash
# Pull new code
git pull

# Rebuild
docker-compose build

# Restart
docker-compose up -d
```

---

## See Also

- [Binary Deployment](binary-deployment.md) — Non-Docker option
- [Kubernetes](kubernetes.md) — Orchestrated deployment
