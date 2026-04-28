# DEPLOYMENT PROMPT: Self-Hosted Daytona Infrastructure via Ansible

You are deploying a self-hosted Daytona sandbox infrastructure across 4 servers using Ansible. Read this entire prompt before writing any code. Execute each phase sequentially, validating before moving to the next.

---

## ARCHITECTURE OVERVIEW

```
┌─────────────────────────────────────────────────────────────────────┐
│                        CONTROL PLANE VPS                            │
│                    (Hetzner VPS — "control")                        │
│                                                                     │
│  ┌──────────┐ ┌───────┐ ┌───────┐ ┌──────────┐ ┌────────────────┐ │
│  │ Daytona  │ │ Proxy │ │  Dex  │ │ Registry │ │ Custom Preview │ │
│  │   API    │ │       │ │ (OIDC)│ │  (OCI)   │ │  Proxy (Caddy) │ │
│  │ :3000    │ │ :4000 │ │ :5556 │ │  :6000   │ │  :80/:443      │ │
│  └──────────┘ └───────┘ └───────┘ └──────────┘ └────────────────┘ │
│  ┌──────────┐ ┌───────┐ ┌───────┐ ┌──────────┐ ┌────────────────┐ │
│  │ Postgres │ │ Redis │ │ MinIO │ │SSH Gateww │ │ Domain         │ │
│  │  :5432   │ │ :6379 │ │ :9000 │ │  :2222   │ │ Gatekeeper     │ │
│  └──────────┘ └───────┘ └───────┘ └──────────┘ │  :5555 (int)   │ │
│                                                 └────────────────┘ │
└─────────────────────────────────────────────────────────────────────┘
         │              │              │
         ▼              ▼              ▼
┌──────────────┐ ┌──────────────┐ ┌──────────────┐
│   Runner 1   │ │   Runner 2   │ │   Runner 3   │
│ (bare metal) │ │ (bare metal) │ │ (bare metal) │
│    :3003     │ │    :3003     │ │    :3003     │
└──────────────┘ └──────────────┘ └──────────────┘
```

**Servers:**

| Alias | Role | Description |
|-------|------|-------------|
| `control` | Control Plane VPS | Hetzner VPS. Runs ALL control plane services via Docker Compose. |
| `runner1` | Sandbox Runner 1 | Bare metal. Runs the Daytona runner + hosts sandbox containers. |
| `runner2` | Sandbox Runner 2 | Bare metal. Same as runner1. |
| `runner3` | Sandbox Runner 3 | Bare metal. Same as runner1. |

**Domains:**

| Domain | Record Type | Points To | Purpose |
|--------|------------|-----------|---------|
| `api.daytona.ziraloop.com` | A | Control VPS public IP | Daytona API + Dashboard |
| `dex.daytona.ziraloop.com` | A | Control VPS public IP | OIDC authentication |
| `*.preview.ziraloop.com` | A (wildcard) | Control VPS public IP | Primary preview proxy domain |
| `*.preview.useportal.app` | A (wildcard) | Control VPS public IP | Dynamic alias domain (example) |
| `*.preview.<anything>` | A (wildcard) | Control VPS public IP | Any future domain — just point DNS, it works instantly |

**Critical requirement — TRULY DYNAMIC DOMAINS:** Adding a new custom preview domain (e.g., `*.preview.clientbrand.com`) must require NOTHING more than the client creating a wildcard DNS record pointing to the control VPS IP. No config file changes. No Caddy restart. No Ansible re-run. No domain list. The system must accept and auto-provision TLS for any domain that resolves to the VPS, in real time, on first request.

---

## HOW DYNAMIC DOMAINS WORK

The system uses **Caddy's on-demand TLS** combined with a **Domain Gatekeeper** microservice:

```
User visits: https://3000-abc123.preview.clientbrand.com
    │
    ▼
1. DNS resolves *.preview.clientbrand.com → Control VPS IP
    │
    ▼
2. Caddy receives TLS handshake for unknown domain
    │
    ▼
3. Caddy asks Domain Gatekeeper: "Is 3000-abc123.preview.clientbrand.com allowed?"
   GET http://gatekeeper:5555/check?domain=3000-abc123.preview.clientbrand.com
    │
    ▼
4. Gatekeeper validates:
   - Does the domain resolve to OUR VPS IP? (DNS resolution check)
   - Does the subdomain match the {port}-{sandboxId} pattern?
   - Is the request rate within limits?
   → Returns 200 (allow) or 403 (deny)
    │
    ▼
5. Caddy provisions a TLS cert from Let's Encrypt (HTTP-01 challenge)
   Certificate is cached — subsequent requests are instant
    │
    ▼
6. Caddy forwards to Daytona Proxy with X-Forwarded-Host header
    │
    ▼
7. Daytona Proxy parses {port}-{sandboxId} from subdomain → routes to runner
```

The **primary domain** (`*.preview.ziraloop.com`) uses a pre-provisioned wildcard cert via Cloudflare DNS challenge for maximum reliability. All **dynamic alias domains** use on-demand TLS with HTTP-01 challenge — no Cloudflare integration needed for those domains.

---

## PROJECT STRUCTURE

Create this exact directory structure:

```
daytona-infra/
├── ansible.cfg
├── inventory/
│   └── hosts.yml                    # All 4 servers
├── group_vars/
│   ├── all.yml                      # Shared variables (domains, versions)
│   ├── control.yml                  # Control plane secrets
│   └── runners.yml                  # Runner variables
├── host_vars/
│   ├── control.yml                  # Control VPS specific (IP, etc.)
│   ├── runner1.yml                  # Runner 1 specific
│   ├── runner2.yml                  # Runner 2 specific
│   └── runner3.yml                  # Runner 3 specific
├── roles/
│   ├── common/                      # Base OS setup (all servers)
│   │   ├── tasks/
│   │   │   └── main.yml
│   │   ├── handlers/
│   │   │   └── main.yml
│   │   └── templates/
│   │       └── sysctl.conf.j2
│   ├── docker/                      # Docker CE installation (all servers)
│   │   ├── tasks/
│   │   │   └── main.yml
│   │   └── handlers/
│   │       └── main.yml
│   ├── control_plane/               # Docker Compose stack for VPS
│   │   ├── tasks/
│   │   │   └── main.yml
│   │   ├── templates/
│   │   │   ├── docker-compose.yml.j2
│   │   │   ├── dex-config.yml.j2
│   │   │   ├── Caddyfile.j2
│   │   │   └── gatekeeper.py.j2
│   │   └── files/
│   │       ├── Dockerfile.caddy
│   │       └── Dockerfile.gatekeeper
│   └── runner/                      # Runner setup for bare metal
│       ├── tasks/
│       │   └── main.yml
│       ├── templates/
│       │   └── daytona-runner.service.j2
│       └── handlers/
│           └── main.yml
├── playbooks/
│   ├── site.yml                     # Master playbook (runs all phases)
│   ├── phase1-common.yml            # OS prep on all servers
│   ├── phase2-docker.yml            # Docker on all servers
│   ├── phase3-control-plane.yml     # Deploy control plane on VPS
│   ├── phase4-runners.yml           # Deploy runners on bare metal
│   └── phase5-validate.yml          # End-to-end validation
├── scripts/
│   └── generate-secrets.sh          # Pre-deployment secret generation
└── secrets/                         # gitignored — generated secrets
    └── .gitkeep
```

---

## INVENTORY FILE

### `inventory/hosts.yml`

```yaml
all:
  children:
    control:
      hosts:
        control:
          ansible_host: "{{ control_ip }}"
          ansible_user: root
          ansible_ssh_private_key_file: ~/.ssh/id_ed25519
    runners:
      hosts:
        runner1:
          ansible_host: "{{ runner1_ip }}"
          ansible_user: root
          ansible_ssh_private_key_file: ~/.ssh/id_ed25519
          runner_name: runner-1
          runner_index: 1
        runner2:
          ansible_host: "{{ runner2_ip }}"
          ansible_user: root
          ansible_ssh_private_key_file: ~/.ssh/id_ed25519
          runner_name: runner-2
          runner_index: 2
        runner3:
          ansible_host: "{{ runner3_ip }}"
          ansible_user: root
          ansible_ssh_private_key_file: ~/.ssh/id_ed25519
          runner_name: runner-3
          runner_index: 3
```

The actual IPs will be provided by the user at runtime via `--extra-vars` or by filling in `host_vars/`. Design the playbooks to accept these as variables with no hardcoded IPs anywhere.

---

## GROUP VARIABLES

### `group_vars/all.yml`

```yaml
---
# =============================================================================
# DAYTONA INFRASTRUCTURE — SHARED VARIABLES
# =============================================================================

# Versions
daytona_version: "latest"
daytona_sandbox_image: "daytonaio/sandbox:0.4.3"
caddy_version: "2"
postgres_version: "16-alpine"
redis_version: "7-alpine"
minio_version: "latest"
dex_version: "v2.39.1"
registry_version: "2"

# =============================================================================
# DOMAIN CONFIGURATION
# =============================================================================

# Control plane domains (static, Cloudflare DNS challenge certs)
api_domain: "api.daytona.ziraloop.com"
dex_domain: "dex.daytona.ziraloop.com"

# Primary preview domain (static wildcard cert via Cloudflare DNS challenge)
# This is the domain Daytona's PROXY_DOMAIN is set to.
primary_preview_domain: "preview.ziraloop.com"

# Proxy
proxy_protocol: "https"

# =============================================================================
# DYNAMIC DOMAIN SUPPORT
# =============================================================================
# Dynamic alias domains require NO configuration here.
# Any domain whose wildcard DNS (*.preview.X) resolves to the control VPS IP
# will automatically work. Caddy uses on-demand TLS with HTTP-01 challenge.
#
# The Domain Gatekeeper validates requests by checking:
# 1. The domain resolves to our VPS IP (prevents cert abuse)
# 2. The subdomain matches {port}-{id} pattern
#
# To add a new alias domain (e.g., *.preview.clientbrand.com):
#   → Client adds DNS: *.preview.clientbrand.com A → <control VPS IP>
#   → Done. First request auto-provisions TLS cert. No server changes needed.
# =============================================================================

# Region
region_id: "us"
region_name: "us-bare-metal"

# Paths on control VPS
control_base_dir: "/opt/daytona"
control_data_dir: "/opt/daytona/data"
control_config_dir: "/opt/daytona/config"

# Paths on runners
runner_base_dir: "/opt/daytona-runner"
runner_data_dir: "/var/lib/daytona"
runner_log_dir: "/var/log/daytona"

# Runner resources (per runner — adjust to match actual hardware)
runner_cpu: 8
runner_memory: 32
runner_disk: 200

# Sandbox quotas
org_quota_total_cpu: 10000
org_quota_total_memory: 10000
org_quota_total_disk: 100000
org_quota_max_cpu_per_sandbox: 8
org_quota_max_memory_per_sandbox: 16
org_quota_max_disk_per_sandbox: 100

# Docker network
docker_network_name: "daytona-net"

# Ports (internal)
api_port: 3000
proxy_port: 4000
dex_port: 5556
registry_port: 6000
minio_port: 9000
minio_console_port: 9001
ssh_gateway_port: 2222
postgres_port: 5432
redis_port: 6379
runner_api_port: 3003
gatekeeper_port: 5555
```

### `group_vars/control.yml`

This file holds secrets. Keep it out of version control — it's gitignored.

```yaml
---
# =============================================================================
# CONTROL PLANE SECRETS
# =============================================================================

# These are PLACEHOLDERS. Run scripts/generate-secrets.sh first,
# then paste the generated values here.

# Encryption
encryption_key: "GENERATE_ME"
encryption_salt: "GENERATE_ME"

# API keys
proxy_api_key: "GENERATE_ME"
admin_api_key: "GENERATE_ME"
ssh_gateway_api_key: "GENERATE_ME"
default_runner_api_key: "GENERATE_ME"

# Database
postgres_user: "daytona"
postgres_password: "GENERATE_ME"
postgres_db: "daytona"

# MinIO
minio_root_user: "minioadmin"
minio_root_password: "GENERATE_ME"

# Registry
registry_admin_user: "admin"
registry_admin_password: "GENERATE_ME"

# SSH Gateway keys (base64-encoded)
ssh_gateway_private_key: "GENERATE_ME"
ssh_gateway_public_key: "GENERATE_ME"
ssh_gateway_host_key: "GENERATE_ME"

# Dex
dex_admin_email: "admin@hivelooploop.com"
dex_admin_password_hash: "GENERATE_ME"  # bcrypt hash

# Cloudflare (for primary domain wildcard TLS only)
cloudflare_api_token: "GENERATE_ME"
```

### `group_vars/runners.yml`

```yaml
---
runner_token: "{{ default_runner_api_key }}"
```

---

## SECRETS GENERATION SCRIPT

### `scripts/generate-secrets.sh`

```bash
#!/usr/bin/env bash
set -euo pipefail

echo "=== Daytona Infrastructure — Secret Generation ==="
echo ""

SECRETS_DIR="$(cd "$(dirname "$0")/../secrets" && pwd)"
mkdir -p "$SECRETS_DIR"

gen() { openssl rand -hex "$1"; }

cat > "$SECRETS_DIR/generated-secrets.yml" << EOF
# Generated at $(date -u +"%Y-%m-%dT%H:%M:%SZ")
# Copy these values into group_vars/control.yml

encryption_key: "$(gen 32)"
encryption_salt: "$(gen 32)"
proxy_api_key: "$(gen 24)"
admin_api_key: "$(gen 24)"
ssh_gateway_api_key: "$(gen 24)"
default_runner_api_key: "$(gen 24)"
postgres_password: "$(gen 24)"
minio_root_password: "$(gen 24)"
registry_admin_password: "$(gen 16)"
EOF

# SSH Gateway keypair
ssh-keygen -t ed25519 -f "$SECRETS_DIR/ssh_gateway_key" -N "" -q
echo "ssh_gateway_private_key: \"$(base64 -w0 "$SECRETS_DIR/ssh_gateway_key")\"" >> "$SECRETS_DIR/generated-secrets.yml"
echo "ssh_gateway_public_key: \"$(base64 -w0 "$SECRETS_DIR/ssh_gateway_key.pub")\"" >> "$SECRETS_DIR/generated-secrets.yml"

# SSH Host key
ssh-keygen -t ed25519 -f "$SECRETS_DIR/ssh_host_key" -N "" -q
echo "ssh_gateway_host_key: \"$(base64 -w0 "$SECRETS_DIR/ssh_host_key")\"" >> "$SECRETS_DIR/generated-secrets.yml"

# Dex admin password
DEX_PASS="$(gen 16)"
DEX_HASH=$(htpasswd -nbBC 10 "" "$DEX_PASS" | cut -d: -f2)
echo "dex_admin_password_hash: \"$DEX_HASH\"" >> "$SECRETS_DIR/generated-secrets.yml"
echo "# dex_admin_password_plaintext: \"$DEX_PASS\"  (save this somewhere safe)" >> "$SECRETS_DIR/generated-secrets.yml"

echo ""
echo "Secrets written to: $SECRETS_DIR/generated-secrets.yml"
echo "Copy them into group_vars/control.yml and keep that file out of version control."
```

Make it executable. The user runs this BEFORE any Ansible playbook.

---

## ROLE: common

### `roles/common/tasks/main.yml`

Runs on ALL 4 servers. Handles:

1. `apt update && apt upgrade -y`
2. Install base packages: `curl`, `wget`, `gnupg`, `ca-certificates`, `lsb-release`, `unzip`, `jq`, `htop`, `iotop`, `net-tools`, `ufw`, `fail2ban`, `apt-transport-https`, `software-properties-common`
3. Set timezone to UTC
4. Configure sysctl tuning (use template `sysctl.conf.j2`):
   - `net.core.somaxconn=65535`
   - `net.ipv4.ip_local_port_range=1024 65535`
   - `fs.inotify.max_user_instances=8192`
   - `fs.inotify.max_user_watches=524288`
   - `vm.max_map_count=262144`
   - `net.ipv4.ip_forward=1`
5. Configure file descriptor limits:
   - `/etc/security/limits.conf`: soft/hard nofile 65536, nproc 32768 for all users
6. Configure UFW:
   - Default deny incoming, allow outgoing
   - Allow SSH (port 22) from anywhere (or restricted IP if provided)
   - Enable UFW
7. Enable and configure fail2ban with default SSH jail
8. Ensure cgroups v2 is enabled (check `/sys/fs/cgroup/cgroup.controllers` exists)
9. Reboot if kernel parameters changed (use handler)

### `roles/common/templates/sysctl.conf.j2`

Standard sysctl template with the values listed above. Use `ansible.builtin.sysctl` module or template + `sysctl -p` handler.

---

## ROLE: docker

### `roles/docker/tasks/main.yml`

Runs on ALL 4 servers. Handles:

1. Add Docker's official GPG key and apt repository (follow Docker's official Ubuntu install docs)
2. Install `docker-ce`, `docker-ce-cli`, `containerd.io`, `docker-buildx-plugin`, `docker-compose-plugin`
3. Enable and start Docker service
4. Configure Docker daemon (`/etc/docker/daemon.json`):
   ```json
   {
     "log-driver": "json-file",
     "log-opts": {
       "max-size": "50m",
       "max-file": "3"
     },
     "storage-driver": "overlay2",
     "default-ulimits": {
       "nofile": {
         "Name": "nofile",
         "Hard": 65536,
         "Soft": 65536
       }
     }
   }
   ```
5. Restart Docker after config change (handler)
6. Verify Docker is running: `docker info`
7. Pull common images to warm cache:
   - On control: pull all control plane images (postgres, redis, minio, dex, registry)
   - On runners: pull `daytonaio/daytona-runner:{{ daytona_version }}` and `{{ daytona_sandbox_image }}`

---

## ROLE: control_plane

### `roles/control_plane/tasks/main.yml`

Runs ONLY on the `control` VPS. This is the most complex role. Execute in this order:

1. **Create directory structure:**
   ```
   {{ control_base_dir }}/
   ├── docker-compose.yml
   ├── config/
   │   ├── dex/
   │   │   └── config.yml
   │   ├── caddy/
   │   │   ├── Caddyfile
   │   │   └── Dockerfile
   │   ├── gatekeeper/
   │   │   ├── gatekeeper.py
   │   │   ├── Dockerfile
   │   │   └── requirements.txt
   │   ├── registry/
   │   │   └── htpasswd
   │   └── postgres/
   └── data/
       ├── postgres/
       ├── redis/
       ├── minio/
       ├── registry/
       └── dex/
   ```

2. **Generate registry htpasswd file:**
   ```bash
   docker run --rm httpd:2 htpasswd -Bbn {{ registry_admin_user }} {{ registry_admin_password }} > {{ control_config_dir }}/registry/htpasswd
   ```

3. **Template all config files** (see templates below)

4. **Build the Caddy image** with Cloudflare DNS plugin:
   ```bash
   cd {{ control_config_dir }}/caddy && docker build -t daytona-caddy:custom .
   ```

5. **Build the Domain Gatekeeper image:**
   ```bash
   cd {{ control_config_dir }}/gatekeeper && docker build -t daytona-gatekeeper:custom .
   ```

6. **UFW rules specific to control VPS:**
   - Allow 80/tcp (HTTP — Caddy + Let's Encrypt HTTP-01 challenge for dynamic domains)
   - Allow 443/tcp (HTTPS — Caddy)
   - Allow 2222/tcp (SSH Gateway)
   - Allow from each runner IP to port 6000 (registry pull)
   - Allow from each runner IP to port 9000 (MinIO S3)
   - Do NOT expose 3000, 4000, 5556, 5555 publicly — Caddy handles all public ingress

7. **Start docker compose:**
   ```bash
   cd {{ control_base_dir }} && docker compose up -d --build
   ```

8. **Wait for services to be healthy** (use `ansible.builtin.uri` to poll):
   - Postgres: `docker compose exec db pg_isready`
   - Redis: `docker compose exec redis redis-cli ping`
   - MinIO: HTTP GET to `:9000/minio/health/live`
   - API: HTTP GET to `https://{{ api_domain }}/api/health` (may take 30-60s for migrations)
   - Dex: HTTP GET to `https://{{ dex_domain }}/dex/.well-known/openid-configuration`

9. **Create MinIO bucket:**
   ```bash
   docker compose exec minio mc alias set local http://localhost:9000 {{ minio_root_user }} {{ minio_root_password }}
   docker compose exec minio mc mb local/daytona --ignore-existing
   ```

---

### TEMPLATE: `roles/control_plane/templates/docker-compose.yml.j2`

```yaml
networks:
  {{ docker_network_name }}:
    driver: bridge

volumes:
  postgres_data:
  redis_data:
  minio_data:
  registry_data:
  dex_data:
  caddy_data:
  caddy_config:

services:
  # =========================================================================
  # DATABASE
  # =========================================================================
  db:
    image: postgres:{{ postgres_version }}
    container_name: daytona-db
    restart: unless-stopped
    networks:
      - {{ docker_network_name }}
    volumes:
      - postgres_data:/var/lib/postgresql/data
    environment:
      POSTGRES_USER: "{{ postgres_user }}"
      POSTGRES_PASSWORD: "{{ postgres_password }}"
      POSTGRES_DB: "{{ postgres_db }}"
    ports:
      - "127.0.0.1:{{ postgres_port }}:5432"
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U {{ postgres_user }} -d {{ postgres_db }}"]
      interval: 10s
      timeout: 5s
      retries: 5

  # =========================================================================
  # REDIS
  # =========================================================================
  redis:
    image: redis:{{ redis_version }}
    container_name: daytona-redis
    restart: unless-stopped
    networks:
      - {{ docker_network_name }}
    volumes:
      - redis_data:/data
    ports:
      - "127.0.0.1:{{ redis_port }}:6379"
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 10s
      timeout: 5s
      retries: 5
    command: redis-server --appendonly yes --maxmemory 512mb --maxmemory-policy allkeys-lru

  # =========================================================================
  # MINIO (S3-COMPATIBLE STORAGE)
  # =========================================================================
  minio:
    image: minio/minio:{{ minio_version }}
    container_name: daytona-minio
    restart: unless-stopped
    networks:
      - {{ docker_network_name }}
    volumes:
      - minio_data:/data
    environment:
      MINIO_ROOT_USER: "{{ minio_root_user }}"
      MINIO_ROOT_PASSWORD: "{{ minio_root_password }}"
    ports:
      - "{{ minio_port }}:9000"
      - "127.0.0.1:{{ minio_console_port }}:9001"
    command: server /data --console-address ":9001"
    healthcheck:
      test: ["CMD", "mc", "ready", "local"]
      interval: 10s
      timeout: 5s
      retries: 5

  # =========================================================================
  # DOCKER REGISTRY (INTERNAL SNAPSHOT STORE)
  # =========================================================================
  registry:
    image: registry:{{ registry_version }}
    container_name: daytona-registry
    restart: unless-stopped
    networks:
      - {{ docker_network_name }}
    volumes:
      - registry_data:/var/lib/registry
      - {{ control_config_dir }}/registry/htpasswd:/auth/htpasswd:ro
    environment:
      REGISTRY_AUTH: htpasswd
      REGISTRY_AUTH_HTPASSWD_REALM: "Daytona Registry"
      REGISTRY_AUTH_HTPASSWD_PATH: /auth/htpasswd
      REGISTRY_STORAGE_DELETE_ENABLED: "true"
    ports:
      - "{{ registry_port }}:5000"
    healthcheck:
      test: ["CMD", "wget", "--spider", "-q", "http://localhost:5000/v2/"]
      interval: 10s
      timeout: 5s
      retries: 5

  # =========================================================================
  # DEX (OIDC PROVIDER)
  # =========================================================================
  dex:
    image: dexidp/dex:{{ dex_version }}
    container_name: daytona-dex
    restart: unless-stopped
    networks:
      - {{ docker_network_name }}
    volumes:
      - {{ control_config_dir }}/dex/config.yml:/etc/dex/config.docker.yaml:ro
      - dex_data:/var/dex
    command: ["dex", "serve", "/etc/dex/config.docker.yaml"]

  # =========================================================================
  # DAYTONA API
  # =========================================================================
  api:
    image: daytonaio/daytona-api:{{ daytona_version }}
    container_name: daytona-api
    restart: unless-stopped
    networks:
      - {{ docker_network_name }}
    depends_on:
      db:
        condition: service_healthy
      redis:
        condition: service_healthy
      minio:
        condition: service_healthy
      registry:
        condition: service_healthy
      dex:
        condition: service_started
    environment:
      PORT: "3000"
      ENVIRONMENT: "production"
      ENCRYPTION_KEY: "{{ encryption_key }}"
      ENCRYPTION_SALT: "{{ encryption_salt }}"
      DB_HOST: "db"
      DB_PORT: "5432"
      DB_USERNAME: "{{ postgres_user }}"
      DB_PASSWORD: "{{ postgres_password }}"
      DB_DATABASE: "{{ postgres_db }}"
      REDIS_HOST: "redis"
      REDIS_PORT: "6379"
      OIDC_CLIENT_ID: "daytona"
      OIDC_ISSUER_BASE_URL: "http://dex:5556/dex"
      PUBLIC_OIDC_DOMAIN: "https://{{ dex_domain }}/dex"
      OIDC_AUDIENCE: "daytona"
      DASHBOARD_URL: "https://{{ api_domain }}/dashboard"
      DASHBOARD_BASE_API_URL: "https://{{ api_domain }}"
      DEFAULT_SNAPSHOT: "{{ daytona_sandbox_image }}"
      TRANSIENT_REGISTRY_URL: "http://registry:5000"
      TRANSIENT_REGISTRY_ADMIN: "{{ registry_admin_user }}"
      TRANSIENT_REGISTRY_PASSWORD: "{{ registry_admin_password }}"
      TRANSIENT_REGISTRY_PROJECT_ID: "daytona"
      INTERNAL_REGISTRY_URL: "http://registry:5000"
      INTERNAL_REGISTRY_ADMIN: "{{ registry_admin_user }}"
      INTERNAL_REGISTRY_PASSWORD: "{{ registry_admin_password }}"
      INTERNAL_REGISTRY_PROJECT_ID: "daytona"
      S3_ENDPOINT: "http://minio:9000"
      S3_STS_ENDPOINT: "http://minio:9000/minio/v1/assume-role"
      S3_REGION: "us-east-1"
      S3_ACCESS_KEY: "{{ minio_root_user }}"
      S3_SECRET_KEY: "{{ minio_root_password }}"
      S3_DEFAULT_BUCKET: "daytona"
      S3_ACCOUNT_ID: "/"
      S3_ROLE_NAME: "/"
      PROXY_DOMAIN: "{{ primary_preview_domain }}"
      PROXY_PROTOCOL: "{{ proxy_protocol }}"
      PROXY_API_KEY: "{{ proxy_api_key }}"
      PROXY_TEMPLATE_URL: "{{ proxy_protocol }}://{% raw %}{{PORT}}-{{sandboxId}}{% endraw %}.{{ primary_preview_domain }}"
      PROXY_TOOLBOX_BASE_URL: "{{ proxy_protocol }}://{{ primary_preview_domain }}"
      DEFAULT_RUNNER_DOMAIN: "{{ hostvars['runner1']['ansible_host'] }}:{{ runner_api_port }}"
      DEFAULT_RUNNER_API_URL: "http://{{ hostvars['runner1']['ansible_host'] }}:{{ runner_api_port }}"
      DEFAULT_RUNNER_PROXY_URL: "http://{{ hostvars['runner1']['ansible_host'] }}:{{ runner_api_port }}"
      DEFAULT_RUNNER_API_KEY: "{{ default_runner_api_key }}"
      DEFAULT_RUNNER_CPU: "{{ runner_cpu }}"
      DEFAULT_RUNNER_MEMORY: "{{ runner_memory }}"
      DEFAULT_RUNNER_DISK: "{{ runner_disk }}"
      DEFAULT_RUNNER_API_VERSION: "2"
      DEFAULT_ORG_QUOTA_TOTAL_CPU_QUOTA: "{{ org_quota_total_cpu }}"
      DEFAULT_ORG_QUOTA_TOTAL_MEMORY_QUOTA: "{{ org_quota_total_memory }}"
      DEFAULT_ORG_QUOTA_TOTAL_DISK_QUOTA: "{{ org_quota_total_disk }}"
      DEFAULT_ORG_QUOTA_MAX_CPU_PER_SANDBOX: "{{ org_quota_max_cpu_per_sandbox }}"
      DEFAULT_ORG_QUOTA_MAX_MEMORY_PER_SANDBOX: "{{ org_quota_max_memory_per_sandbox }}"
      DEFAULT_ORG_QUOTA_MAX_DISK_PER_SANDBOX: "{{ org_quota_max_disk_per_sandbox }}"
      DEFAULT_ORG_QUOTA_SNAPSHOT_QUOTA: "1000"
      DEFAULT_ORG_QUOTA_MAX_SNAPSHOT_SIZE: "1000"
      DEFAULT_ORG_QUOTA_VOLUME_QUOTA: "10000"
      SSH_GATEWAY_API_KEY: "{{ ssh_gateway_api_key }}"
      SSH_GATEWAY_URL: "{{ api_domain }}:{{ ssh_gateway_port }}"
      SSH_GATEWAY_COMMAND: "ssh -p {{ ssh_gateway_port }} {% raw %}{{TOKEN}}{% endraw %}@{{ api_domain }}"
      SSH_GATEWAY_PUBLIC_KEY: "{{ ssh_gateway_public_key }}"
      DEFAULT_REGION_ID: "{{ region_id }}"
      DEFAULT_REGION_NAME: "{{ region_name }}"
      DEFAULT_REGION_ENFORCE_QUOTAS: "false"
      RUN_MIGRATIONS: "true"
      SKIP_USER_EMAIL_VERIFICATION: "true"
      MAX_AUTO_ARCHIVE_INTERVAL: "43200"
      NOTIFICATION_GATEWAY_DISABLED: "true"
      MAINTENANCE_MODE: "false"
      ADMIN_API_KEY: "{{ admin_api_key }}"
      ADMIN_TOTAL_CPU_QUOTA: "0"
      ADMIN_TOTAL_MEMORY_QUOTA: "0"
      ADMIN_TOTAL_DISK_QUOTA: "0"
      ADMIN_MAX_CPU_PER_SANDBOX: "0"
      ADMIN_MAX_MEMORY_PER_SANDBOX: "0"
      ADMIN_MAX_DISK_PER_SANDBOX: "0"
      ADMIN_SNAPSHOT_QUOTA: "100"
      ADMIN_MAX_SNAPSHOT_SIZE: "100"
      ADMIN_VOLUME_QUOTA: "0"
      POSTHOG_API_KEY: ""
      POSTHOG_HOST: ""
      POSTHOG_ENVIRONMENT: "self-hosted"
      OTEL_ENABLED: "false"

  # =========================================================================
  # DAYTONA PROXY (internal only — Caddy is the public face)
  # =========================================================================
  proxy:
    image: daytonaio/daytona-proxy:{{ daytona_version }}
    container_name: daytona-proxy
    restart: unless-stopped
    networks:
      - {{ docker_network_name }}
    depends_on:
      - api
      - redis
    # NO ports: mapping — only Caddy talks to this via Docker network
    environment:
      DAYTONA_API_URL: "http://api:3000/api"
      PROXY_PORT: "4000"
      PROXY_API_KEY: "{{ proxy_api_key }}"
      PROXY_PROTOCOL: "{{ proxy_protocol }}"
      COOKIE_DOMAIN: "{{ primary_preview_domain }}"
      OIDC_CLIENT_ID: "daytona"
      OIDC_CLIENT_SECRET: ""
      OIDC_DOMAIN: "http://dex:5556/dex"
      OIDC_PUBLIC_DOMAIN: "https://{{ dex_domain }}/dex"
      OIDC_AUDIENCE: "daytona"
      REDIS_HOST: "redis"
      REDIS_PORT: "6379"
      TOOLBOX_ONLY_MODE: "false"
      PREVIEW_WARNING_ENABLED: "false"
      SHUTDOWN_TIMEOUT_SEC: "3600"

  # =========================================================================
  # SSH GATEWAY
  # =========================================================================
  ssh-gateway:
    image: daytonaio/daytona-ssh-gateway:{{ daytona_version }}
    container_name: daytona-ssh-gateway
    restart: unless-stopped
    networks:
      - {{ docker_network_name }}
    depends_on:
      - api
    ports:
      - "{{ ssh_gateway_port }}:2222"
    environment:
      API_URL: "http://api:3000/api"
      API_KEY: "{{ ssh_gateway_api_key }}"
      SSH_PRIVATE_KEY: "{{ ssh_gateway_private_key }}"
      SSH_HOST_KEY: "{{ ssh_gateway_host_key }}"
      SSH_GATEWAY_PORT: "2222"

  # =========================================================================
  # DOMAIN GATEKEEPER
  #
  # Validates on-demand TLS requests from Caddy. Prevents certificate abuse
  # by verifying the requesting domain resolves to our VPS IP.
  # This is what makes truly dynamic domains possible with zero config.
  # =========================================================================
  gatekeeper:
    build:
      context: {{ control_config_dir }}/gatekeeper
      dockerfile: Dockerfile
    container_name: daytona-gatekeeper
    restart: unless-stopped
    networks:
      - {{ docker_network_name }}
    environment:
      EXPECTED_IP: "{{ hostvars['control']['ansible_host'] }}"
      ALLOWED_PORT_RANGE: "3000-9999"
      LOG_LEVEL: "info"

  # =========================================================================
  # CADDY (PUBLIC INGRESS — TLS TERMINATION — DYNAMIC DOMAIN ROUTING)
  #
  # Three responsibilities:
  # 1. Static TLS for api.daytona.ziraloop.com + dex.daytona.ziraloop.com
  # 2. Wildcard TLS for *.preview.ziraloop.com (primary, Cloudflare DNS)
  # 3. On-demand TLS for ANY other domain (dynamic, HTTP-01 challenge)
  # =========================================================================
  caddy:
    build:
      context: {{ control_config_dir }}/caddy
      dockerfile: Dockerfile
    container_name: daytona-caddy
    restart: unless-stopped
    networks:
      - {{ docker_network_name }}
    depends_on:
      - api
      - proxy
      - gatekeeper
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - {{ control_config_dir }}/caddy/Caddyfile:/etc/caddy/Caddyfile:ro
      - caddy_data:/data
      - caddy_config:/config
    environment:
      CLOUDFLARE_API_TOKEN: "{{ cloudflare_api_token }}"
```

---

### TEMPLATE: `roles/control_plane/templates/Caddyfile.j2`

This is the most critical config file. It has THREE tiers:

1. **Static sites** — API and Dex with Cloudflare DNS challenge certs
2. **Primary preview domain** — `*.preview.ziraloop.com` with Cloudflare DNS challenge wildcard cert (always reliable)
3. **Dynamic catch-all** — ANY other domain, on-demand TLS via HTTP-01, validated by Gatekeeper. **No domain list. No restart needed. Ever.**

```caddyfile
# =============================================================================
# GLOBAL OPTIONS
# =============================================================================
{
    email admin@hivelooploop.com

    # ON-DEMAND TLS: Caddy asks the Gatekeeper before issuing a cert
    # for any domain not explicitly listed in a site block below.
    # This is what makes dynamic alias domains work with ZERO config changes.
    on_demand_tls {
        ask http://gatekeeper:{{ gatekeeper_port }}/check
        interval 2m
        burst 10
    }
}

# =============================================================================
# TIER 1: STATIC SERVICES
# Certs provisioned via Cloudflare DNS challenge (no port 80 needed)
# =============================================================================

# API + Dashboard
{{ api_domain }} {
    tls {
        dns cloudflare {env.CLOUDFLARE_API_TOKEN}
    }
    reverse_proxy api:{{ api_port }} {
        header_up X-Real-IP {remote_host}
        header_up X-Forwarded-Proto {scheme}
    }
}

# Dex OIDC
{{ dex_domain }} {
    tls {
        dns cloudflare {env.CLOUDFLARE_API_TOKEN}
    }
    reverse_proxy dex:{{ dex_port }} {
        header_up X-Real-IP {remote_host}
        header_up X-Forwarded-Proto {scheme}
    }
}

# =============================================================================
# TIER 2: PRIMARY PREVIEW DOMAIN — *.preview.ziraloop.com
# Wildcard cert via Cloudflare DNS challenge (most reliable, no HTTP-01)
# This is what PROXY_DOMAIN is set to in the Daytona API config.
# =============================================================================
*.{{ primary_preview_domain }} {
    tls {
        dns cloudflare {env.CLOUDFLARE_API_TOKEN}
    }

    reverse_proxy proxy:{{ proxy_port }} {
        # CRITICAL: Forward the original host so Daytona Proxy can parse
        # the {port}-{sandboxId} pattern from the subdomain
        header_up X-Forwarded-Host {host}
        header_up X-Daytona-Skip-Preview-Warning true
        header_up X-Real-IP {remote_host}

        # WebSocket passthrough
        header_up Connection {header.Connection}
        header_up Upgrade {header.Upgrade}

        # No timeouts — sandbox connections can be long-lived
        transport http {
            dial_timeout 30s
            response_header_timeout 0
            read_timeout 0
            write_timeout 0
        }
    }

    log {
        output stdout
        format json
    }
}

# =============================================================================
# TIER 3: DYNAMIC ALIAS DOMAINS — ANY DOMAIN THAT RESOLVES TO THIS SERVER
#
# This is the magic catch-all block. It handles ANY HTTPS request to a domain
# NOT matched by Tier 1 or Tier 2 above. On first request, Caddy will:
#
#   1. Ask the Gatekeeper: GET http://gatekeeper:5555/check?domain=<fqdn>
#   2. Gatekeeper checks: does this domain resolve to our IP?
#   3. If approved, Caddy provisions TLS cert via HTTP-01 challenge (~2-5s)
#   4. Cert is cached — all subsequent requests are instant
#   5. Request is forwarded to Daytona Proxy with X-Forwarded-Host
#
# TO ADD A NEW ALIAS DOMAIN:
#   → Add DNS: *.preview.clientbrand.com A → <this VPS IP>
#   → DONE. First request triggers cert provisioning. Zero server changes.
# =============================================================================
:443 {
    tls {
        on_demand
    }

    reverse_proxy proxy:{{ proxy_port }} {
        header_up X-Forwarded-Host {host}
        header_up X-Daytona-Skip-Preview-Warning true
        header_up X-Real-IP {remote_host}

        header_up Connection {header.Connection}
        header_up Upgrade {header.Upgrade}

        transport http {
            dial_timeout 30s
            response_header_timeout 0
            read_timeout 0
            write_timeout 0
        }
    }

    log {
        output stdout
        format json
    }
}
```

---

### TEMPLATE: `roles/control_plane/templates/gatekeeper.py.j2`

The Domain Gatekeeper is a lightweight Python (FastAPI) service that validates on-demand TLS requests from Caddy. It prevents certificate abuse by ensuring the requesting domain actually resolves to our VPS IP.

```python
"""
Domain Gatekeeper for Caddy On-Demand TLS.

Caddy calls GET /check?domain=<fqdn> before issuing a certificate.
Returns 200 to allow, 403 to deny.

Validation rules:
1. The domain must resolve (via DNS) to our expected VPS IP
2. The subdomain must look like a Daytona preview URL ({port}-{id}.*)
3. Rate limiting to prevent Let's Encrypt abuse
"""

import os
import re
import socket
import time
import logging
from collections import defaultdict
from fastapi import FastAPI, Query, Response

app = FastAPI()
logger = logging.getLogger("gatekeeper")
logging.basicConfig(
    level=getattr(logging, os.environ.get("LOG_LEVEL", "info").upper()),
    format="%(asctime)s [%(levelname)s] %(message)s"
)

EXPECTED_IP = os.environ.get("EXPECTED_IP", "127.0.0.1")
PORT_RANGE = os.environ.get("ALLOWED_PORT_RANGE", "3000-9999")
MIN_PORT, MAX_PORT = map(int, PORT_RANGE.split("-"))

# Rate limiting: max 30 new domains per minute
rate_limit = defaultdict(list)
RATE_WINDOW = 60
RATE_MAX = 30

# Cache: approved domains (avoid repeated DNS lookups)
approved_cache: dict[str, float] = {}
CACHE_TTL = 300  # 5 minutes

# Pattern: {port}-{sandboxId}.anything.anything (at least 2 dots total)
PREVIEW_PATTERN = re.compile(r"^(\d+)-([a-zA-Z0-9_-]+)\..+\..+$")


def resolve_to_ips(hostname: str) -> list[str]:
    """Resolve a hostname to its IP addresses."""
    try:
        results = socket.getaddrinfo(hostname, None, socket.AF_INET)
        return list(set(r[4][0] for r in results))
    except socket.gaierror:
        return []


def check_rate_limit() -> bool:
    """Returns True if within rate limit."""
    now = time.time()
    key = "global"
    rate_limit[key] = [t for t in rate_limit[key] if now - t < RATE_WINDOW]
    if len(rate_limit[key]) >= RATE_MAX:
        return False
    rate_limit[key].append(now)
    return True


@app.get("/check")
def check_domain(domain: str = Query(...)):
    """
    Caddy calls this endpoint before issuing an on-demand TLS certificate.
    200 = allow, 403 = deny.
    """

    # 1. Check cache
    now = time.time()
    if domain in approved_cache and (now - approved_cache[domain]) < CACHE_TTL:
        logger.debug(f"Cache hit (approved): {domain}")
        return Response(status_code=200)

    # 2. Rate limit
    if not check_rate_limit():
        logger.warning(f"Rate limited: {domain}")
        return Response(status_code=429)

    # 3. Validate subdomain pattern: {port}-{sandboxId}.something.domain.tld
    match = PREVIEW_PATTERN.match(domain)
    if not match:
        logger.info(f"Denied (bad pattern): {domain}")
        return Response(status_code=403)

    port = int(match.group(1))
    if not (MIN_PORT <= port <= MAX_PORT):
        logger.info(f"Denied (port out of range {port}): {domain}")
        return Response(status_code=403)

    # 4. DNS resolution check — does this domain resolve to our IP?
    resolved_ips = resolve_to_ips(domain)
    if EXPECTED_IP not in resolved_ips:
        logger.info(
            f"Denied (DNS mismatch): {domain} resolved to {resolved_ips}, "
            f"expected {EXPECTED_IP}"
        )
        return Response(status_code=403)

    # 5. Approved — cache it
    approved_cache[domain] = now
    logger.info(f"Approved: {domain}")
    return Response(status_code=200)


@app.get("/health")
def health():
    return {"status": "ok", "expected_ip": EXPECTED_IP}
```

### FILE: `roles/control_plane/files/Dockerfile.gatekeeper`

```dockerfile
FROM python:3.12-slim
WORKDIR /app
RUN pip install --no-cache-dir fastapi uvicorn
COPY gatekeeper.py .
EXPOSE 5555
CMD ["uvicorn", "gatekeeper:app", "--host", "0.0.0.0", "--port", "5555", "--log-level", "info"]
```

### FILE: `roles/control_plane/files/Dockerfile.caddy`

```dockerfile
FROM caddy:2-builder AS builder
RUN xcaddy build \
    --with github.com/caddy-dns/cloudflare

FROM caddy:2
COPY --from=builder /usr/bin/caddy /usr/bin/caddy
```

---

### TEMPLATE: `roles/control_plane/templates/dex-config.yml.j2`

```yaml
issuer: https://{{ dex_domain }}/dex

storage:
  type: sqlite3
  config:
    file: /var/dex/dex.db

web:
  http: 0.0.0.0:5556

staticClients:
  - id: daytona
    name: Daytona
    redirectURIs:
      - https://{{ api_domain }}
      - https://{{ api_domain }}/api/oauth2-redirect.html
      - https://{{ primary_preview_domain }}/callback
      # Dynamic alias domains do NOT need Dex redirect URIs.
      # Preview traffic uses token-based auth (X-Daytona-Preview-Token),
      # not OIDC callbacks. Only the dashboard login flow uses Dex.
    public: true

enablePasswordDB: true
staticPasswords:
  - email: "{{ dex_admin_email }}"
    hash: "{{ dex_admin_password_hash }}"
    username: "admin"
    userID: "08a8684b-db88-4b73-90a9-3cd1661f5466"
```

---

## ROLE: runner

### `roles/runner/tasks/main.yml`

Runs on ALL 3 runner hosts. Handles:

1. **Create directories:**
   ```
   {{ runner_base_dir }}/
   {{ runner_data_dir }}/
   {{ runner_data_dir }}/snapshots/
   {{ runner_data_dir }}/volumes/
   {{ runner_log_dir }}/
   ```

2. **UFW rules specific to runners:**
   - Allow port `{{ runner_api_port }}` from the control VPS IP only:
     ```
     ufw allow from {{ hostvars['control']['ansible_host'] }} to any port {{ runner_api_port }}
     ```

3. **Pull runner image:**
   ```bash
   docker pull daytonaio/daytona-runner:{{ daytona_version }}
   ```

4. **Pull sandbox base image (warm cache):**
   ```bash
   docker pull {{ daytona_sandbox_image }}
   ```

5. **Template and install systemd service** (see template below)

6. **Enable and start the service:**
   ```bash
   systemctl daemon-reload
   systemctl enable daytona-runner
   systemctl start daytona-runner
   ```

7. **Wait for runner to be healthy:**
   Poll `http://localhost:{{ runner_api_port }}/health` until 200, timeout 60s.

---

### TEMPLATE: `roles/runner/templates/daytona-runner.service.j2`

```ini
[Unit]
Description=Daytona Sandbox Runner ({{ runner_name }})
After=docker.service
Requires=docker.service

[Service]
Type=simple
Restart=always
RestartSec=10
TimeoutStartSec=120

ExecStartPre=-/usr/bin/docker stop daytona-runner
ExecStartPre=-/usr/bin/docker rm daytona-runner

ExecStart=/usr/bin/docker run \
  --name daytona-runner \
  --privileged \
  --network host \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v {{ runner_data_dir }}:/var/lib/daytona \
  -v {{ runner_log_dir }}:/var/log/daytona \
  -e DAYTONA_API_URL=https://{{ api_domain }}/api \
  -e DAYTONA_RUNNER_TOKEN={{ runner_token }} \
  -e VERSION=0.0.1 \
  -e ENVIRONMENT=production \
  -e API_PORT={{ runner_api_port }} \
  -e LOG_FILE_PATH=/var/log/daytona/runner.log \
  -e RESOURCE_LIMITS_DISABLED=false \
  -e AWS_ENDPOINT_URL=http://{{ hostvars['control']['ansible_host'] }}:{{ minio_port }} \
  -e AWS_REGION=us-east-1 \
  -e AWS_ACCESS_KEY_ID={{ minio_root_user }} \
  -e AWS_SECRET_ACCESS_KEY={{ minio_root_password }} \
  -e AWS_DEFAULT_BUCKET=daytona \
  -e DAEMON_START_TIMEOUT_SEC=60 \
  -e SANDBOX_START_TIMEOUT_SEC=30 \
  -e RUNNER_DOMAIN={{ ansible_host }}:{{ runner_api_port }} \
  -e POLL_TIMEOUT=30s \
  -e POLL_LIMIT=10 \
  -e HEALTHCHECK_INTERVAL=30s \
  -e HEALTHCHECK_TIMEOUT=10s \
  -e API_VERSION=2 \
  -e INTER_SANDBOX_NETWORK_ENABLED=false \
  -e VOLUME_CLEANUP_INTERVAL=30s \
  -e CPU_USAGE_SNAPSHOT_INTERVAL=5s \
  -e ALLOCATED_RESOURCES_SNAPSHOT_INTERVAL=5s \
  -e SNAPSHOT_ERROR_CACHE_RETENTION=10m \
  daytonaio/daytona-runner:{{ daytona_version }}

ExecStop=/usr/bin/docker stop -t 30 daytona-runner

[Install]
WantedBy=multi-user.target
```

---

## PLAYBOOKS

### `playbooks/site.yml`

```yaml
---
- name: "Daytona Infrastructure — Full Deployment"
  import_playbook: phase1-common.yml
- import_playbook: phase2-docker.yml
- import_playbook: phase3-control-plane.yml
- import_playbook: phase4-runners.yml
- import_playbook: phase5-validate.yml
```

### `playbooks/phase1-common.yml`

```yaml
---
- name: "Phase 1 — Base OS Setup (All Servers)"
  hosts: all
  become: true
  roles:
    - common
  tags: [phase1, common]
```

### `playbooks/phase2-docker.yml`

```yaml
---
- name: "Phase 2 — Docker Installation (All Servers)"
  hosts: all
  become: true
  roles:
    - docker
  tags: [phase2, docker]
```

### `playbooks/phase3-control-plane.yml`

```yaml
---
- name: "Phase 3 — Control Plane Deployment (VPS)"
  hosts: control
  become: true
  roles:
    - control_plane
  tags: [phase3, control]
```

### `playbooks/phase4-runners.yml`

```yaml
---
- name: "Phase 4 — Runner Deployment (Bare Metal)"
  hosts: runners
  become: true
  roles:
    - runner
  tags: [phase4, runners]
```

### `playbooks/phase5-validate.yml`

```yaml
---
- name: "Phase 5 — End-to-End Validation"
  hosts: control
  become: true
  tags: [phase5, validate]
  tasks:
    - name: Check API health
      ansible.builtin.uri:
        url: "https://{{ api_domain }}/api/health"
        method: GET
        status_code: 200
        timeout: 30
      retries: 5
      delay: 10

    - name: Check Dex OIDC discovery
      ansible.builtin.uri:
        url: "https://{{ dex_domain }}/dex/.well-known/openid-configuration"
        method: GET
        status_code: 200
        timeout: 10

    - name: Check Domain Gatekeeper health
      ansible.builtin.uri:
        url: "http://localhost:{{ gatekeeper_port }}/health"
        method: GET
        status_code: 200
        timeout: 10

    - name: Check primary preview domain responds
      ansible.builtin.uri:
        url: "https://3000-healthcheck.{{ primary_preview_domain }}"
        method: GET
        status_code: [200, 404, 502]
        timeout: 15
      ignore_errors: true

    - name: Check runners are reachable
      ansible.builtin.uri:
        url: "http://{{ hostvars[item]['ansible_host'] }}:{{ runner_api_port }}/health"
        method: GET
        status_code: 200
        timeout: 10
      loop: "{{ groups['runners'] }}"
      ignore_errors: true

    - name: Deployment summary
      ansible.builtin.debug:
        msg: |
          ╔══════════════════════════════════════════════════════════════════╗
          ║              DAYTONA DEPLOYMENT COMPLETE                        ║
          ╠══════════════════════════════════════════════════════════════════╣
          ║                                                                ║
          ║  API:        https://{{ api_domain }}                           ║
          ║  Dashboard:  https://{{ api_domain }}/dashboard                ║
          ║  Dex OIDC:   https://{{ dex_domain }}/dex                      ║
          ║  Login:      {{ dex_admin_email }}                              ║
          ║                                                                ║
          ║  Primary Preview Domain:                                       ║
          ║    *.{{ primary_preview_domain }}                               ║
          ║    Example: https://3000-<id>.{{ primary_preview_domain }}      ║
          ║                                                                ║
          ║  Dynamic Alias Domains:                                        ║
          ║    Point *.preview.<yourdomain> A → {{ hostvars['control']['ansible_host'] }}          ║
          ║    It works instantly. No config changes needed.                ║
          ║                                                                ║
          ║  Runners:                                                      ║
          {% for runner in groups['runners'] %}
          ║    {{ runner }}: {{ hostvars[runner]['ansible_host'] }}:{{ runner_api_port }}            ║
          {% endfor %}
          ║                                                                ║
          ╚══════════════════════════════════════════════════════════════════╝
```

---

## ANSIBLE.CFG

```ini
[defaults]
inventory = inventory/hosts.yml
roles_path = roles
host_key_checking = False
retry_files_enabled = False
stdout_callback = yaml
timeout = 30
forks = 10

[privilege_escalation]
become = True
become_method = sudo
become_user = root

[ssh_connection]
pipelining = True
ssh_args = -o ControlMaster=auto -o ControlPersist=60s -o ServerAliveInterval=30
```

---

## .gitignore

```
secrets/
!secrets/.gitkeep
*.retry
*.pyc
__pycache__/
.vagrant/
*.log
```

---

## PRE-DEPLOYMENT CHECKLIST

Before running anything, the user must:

1. [ ] Have SSH access (key-based) to all 4 servers as root
2. [ ] Have 4 server IPs ready
3. [ ] Have a Cloudflare account with `ziraloop.com` zone
4. [ ] Have a Cloudflare API token with DNS:Edit for `ziraloop.com`
5. [ ] Run `chmod +x scripts/generate-secrets.sh && ./scripts/generate-secrets.sh`
6. [ ] Copy secrets from `secrets/generated-secrets.yml` into `group_vars/control.yml` (keep gitignored)
7. [ ] Fill in server IPs in `host_vars/` or pass via `--extra-vars`
9. [ ] Create DNS records:
   - `api.daytona.ziraloop.com` A → control VPS IP
   - `dex.daytona.ziraloop.com` A → control VPS IP
   - `*.preview.ziraloop.com` A → control VPS IP

---

## EXECUTION

```bash
# Full deployment (all phases)
ansible-playbook playbooks/site.yml \
  -e control_ip=<VPS_IP> \
  -e runner1_ip=<RUNNER1_IP> \
  -e runner2_ip=<RUNNER2_IP> \
  -e runner3_ip=<RUNNER3_IP>

# Individual phases:
ansible-playbook playbooks/phase1-common.yml -e ...
ansible-playbook playbooks/phase2-docker.yml -e ...
ansible-playbook playbooks/phase3-control-plane.yml -e ...
ansible-playbook playbooks/phase4-runners.yml -e ...
ansible-playbook playbooks/phase5-validate.yml -e ...
```

---

## ADDING A NEW DYNAMIC ALIAS DOMAIN

This is the whole point. Complete process for `*.preview.clientbrand.com`:

**Step 1 (only step):** Add a DNS record in the `clientbrand.com` DNS provider:

```
Type: A
Name: *.preview
Value: <control VPS IP>
```

**There is no Step 2.** On first request to `https://3000-abc123.preview.clientbrand.com`:

1. DNS resolves to VPS IP
2. Caddy receives TLS handshake for unknown domain
3. Caddy asks Gatekeeper → Gatekeeper resolves domain, confirms it points to our IP → 200
4. Caddy provisions TLS cert via HTTP-01 (~2-5 seconds)
5. Cert is cached, request forwarded to Daytona Proxy
6. Daytona Proxy parses `3000` + `abc123` → routes to correct runner
7. All subsequent requests to `*.preview.clientbrand.com` are instant

---

## IMPORTANT NOTES FOR THE AGENT

1. **Do NOT hardcode any IP addresses or secrets.** Everything comes from variables.
2. **The Caddy Dockerfile must be built on the VPS**, not pulled. It needs the `xcaddy` builder for the Cloudflare DNS plugin.
3. **The Gatekeeper Dockerfile must also be built on the VPS.** Simple Python/FastAPI app.
4. **The Daytona Proxy is NOT exposed publicly.** Only Caddy talks to it via Docker network. No `ports:` mapping on the proxy service.
5. **Port 80 MUST be open** for Let's Encrypt HTTP-01 challenges. Without port 80, dynamic domains cannot get certificates.
6. **Port 443 MUST be open** for HTTPS traffic. These are the ONLY two public ports besides SSH (22) and the SSH gateway (2222).
7. **Runner containers run with `--privileged` and `--network host`** — required for Docker socket access and sandbox networking.
8. **MinIO port 9000 must be accessible from runner IPs** — runners pull/push snapshots directly to MinIO.
9. **Registry port 6000 must be accessible from runner IPs** — runners pull snapshot images from the internal registry.
10. **The `{% raw %}...{% endraw %}` blocks** prevent Jinja2 from interpreting Daytona's `{{PORT}}` and `{{sandboxId}}`. Do not remove them.
11. **Runners use the API's public domain** (`https://api.daytona.ziraloop.com/api`) not Docker hostnames — they're on separate machines.
12. **Runners access MinIO via the control VPS's public IP** — not the `minio` hostname.
13. **Dex redirect URIs do NOT need dynamic alias domains.** Preview traffic uses token-based auth, not OIDC callbacks.
14. **The `runner_token` defaults to `default_runner_api_key`** (shared key strategy).
15. **Idempotency:** All tasks must be idempotent. Use `docker compose up -d` (not `down && up`), `creates:` conditions, and handlers for restarts.
16. **Every template must have a handler** that restarts the affected service on change.
17. **Phase 5 validation must never fail the playbook** — use `ignore_errors: true` on preview checks.
18. **Gatekeeper's `EXPECTED_IP` must be the control VPS's PUBLIC IP** — `{{ hostvars['control']['ansible_host'] }}`.
19. **Let's Encrypt rate limits:** 50 certs per registered domain per week, 300 new orders per 3 hours. The Gatekeeper's rate limiting (30/minute) helps. For very high domain counts, consider ZeroSSL via Caddy's `acme_ca` directive.