# Manual Steps Required Outside Ansible

This documents every manual step that was needed beyond running the Ansible playbooks to get the self-hosted Daytona infrastructure fully operational.

---

## 1. DNS Records

Create A records pointing to the control plane VPS IP before running Phase 2.

| Record | Type | Value |
|--------|------|-------|
| `api.daytona.usehiveloop.com` | A | `<VPS_IP>` |
| `dex.daytona.usehiveloop.com` | A | `<VPS_IP>` |
| `*.preview.usehiveloop.com` | A | `<VPS_IP>` |
| `acme-dns-api.daytona.usehiveloop.com` | A | `<VPS_IP>` |
| `caddy-admin.daytona.usehiveloop.com` | A | `<VPS_IP>` |

### Custom domains — acme-dns zone delegation

Custom preview domains (users bringing their own domain) rely on acme-dns running on the VPS. The zone `acme.usehiveloop.com` must be delegated to it as authoritative.

| Record | Type | Value |
|--------|------|-------|
| `auth.acme.usehiveloop.com` | A | `<VPS_IP>` |
| `acme.usehiveloop.com` | NS | `auth.acme.usehiveloop.com` |

In Cloudflare, the NS record on `acme.usehiveloop.com` tells resolvers to ask the acme-dns server (at `auth.acme.usehiveloop.com`) for anything under `acme.usehiveloop.com`. Cloudflare proxying must be **off** (gray cloud) for `auth.acme.usehiveloop.com` so UDP/TCP :53 traffic reaches the VPS.

For dynamic alias domains (e.g. `*.preview.useportal.app`), add an A record pointing to the same VPS IP. No server-side changes needed — Caddy auto-provisions TLS via on-demand HTTP-01 challenge.

---

## 2. Cloudflare R2 Setup

### Create R2 Bucket
1. Go to Cloudflare Dashboard → R2 Object Storage
2. Create a bucket named `daytona`

### Create R2 API Token
1. Go to R2 → Manage R2 API Tokens → Create API Token
2. **Permission: Admin Read & Write** (not just Object Read & Write — Daytona's VolumeManager calls ListBuckets on startup which requires admin scope)
3. Scope: Apply to all buckets (or at minimum the `daytona` bucket)
4. Copy the Access Key ID and Secret Access Key into `.env`

**Important**: "Object Read & Write" is insufficient. The Daytona API performs a `ListBuckets` call on startup for S3 connectivity validation, which returns 403 with bucket-scoped tokens. The token must have **Admin Read & Write** permission.

---

## 3. Cloudflare API Token for Caddy TLS

1. Go to Cloudflare Dashboard → My Profile → API Tokens → Create Token
2. Use "Edit zone DNS" template
3. Zone Resources: Include → Specific zone → `usehiveloop.com`
4. Copy the token into `.env` as `CLOUDFLARE_API_TOKEN`

This is used by Caddy for DNS-01 challenges to provision wildcard certs for `*.preview.usehiveloop.com`, `api.daytona.usehiveloop.com`, `dex.daytona.usehiveloop.com`, `acme-dns-api.daytona.usehiveloop.com`, and `caddy-admin.daytona.usehiveloop.com`.

---

## 3b. Hiveloop Internal Secret (for custom-domain backend → Caddy)

The backend talks to the acme-dns API proxy and Caddy admin API proxy using a shared secret sent in the `X-Internal-Secret` header.

Generate and export before running Phase 2:

```bash
export HIVELOOP_INTERNAL_SECRET="$(openssl rand -hex 32)"
```

Add the same value to the backend's runtime env as `INTERNAL_DOMAIN_SECRET`, alongside:

```
ACME_DNS_API_URL=https://acme-dns-api.daytona.usehiveloop.com
CADDY_ADMIN_URL=https://caddy-admin.daytona.usehiveloop.com
```

---

## 4. Dex Admin Password

Generate a bcrypt hash for the Dex admin password:

```bash
htpasswd -nBC 10 "" | tr -d ':\n'
```

Enter your desired password when prompted. Put the hash in `.env` using **single quotes** (critical — double quotes cause shell expansion of `$` characters in the bcrypt hash):

```bash
export DAYTONA_DEX_ADMIN_PASSWORD_HASH='$2y$10$...'
```

Login credentials for the Daytona dashboard:
- Email: `admin@usehiveloop.com`
- Password: whatever you entered when generating the hash

---

## 5. Daytona Dashboard — Generate API Key

After Phase 2 deploys the control plane and you log into the dashboard:

1. Go to `https://api.daytona.usehiveloop.com/dashboard`
2. Log in with the Dex admin credentials
3. Navigate to the dashboard and set the **default region** to `us-bare-metal` (required — without this, snapshot creation returns 428)
4. Navigate to API Keys section
5. Generate a new API key (will start with `dtn_`)
6. Update the repo's `.env` file at `/Users/bahdcoder/code/llmvault.dev/.env`:
   ```
   SANDBOX_PROVIDER_KEY=dtn_<your-new-key>
   ```

**Important**: Do NOT put `DAYTONA_API_KEY` in the ansible `.env` file. The Daytona Go SDK reads `DAYTONA_API_KEY` from the environment as a fallback, and if it contains a stale key from a previous DB, all SDK operations will get 403.

**Note**: If the Postgres database is ever wiped, you must log in again, set the default region again, and generate a new API key.

---

## 6. Runner DinD — First Image Pull

After deploying runners (Phase 3), each runner's Docker-in-Docker starts with an empty image cache. The default snapshot image needs to be pulled into the DinD before sandboxes can start. This happens automatically via the API's PULL_SNAPSHOT job, but if it doesn't complete in time, you can trigger it manually on each runner:

```bash
# On each runner (replace with actual snapshot ref from API):
docker exec daytona-runner docker pull 77.42.84.83:6000/daytona/daytona-fb0bf34ac63267bbd81f9af7461716cbc55b78fa3bef35e516588a5cccaa9d59:daytona
```

The DinD data is persisted at `/opt/daytona-runner/dind-data/`, so the image survives runner restarts. However, if this directory is deleted, the image must be pulled again.

---

## 7. Repo `.env` — Sandbox Provider Configuration

Update the ZiraLoop repo's `.env` (not the ansible `.env`) to point to the self-hosted Daytona:

```
SANDBOX_PROVIDER_ID=daytona
SANDBOX_PROVIDER_URL=https://api.daytona.usehiveloop.com/api
SANDBOX_PROVIDER_KEY=dtn_<api-key-from-dashboard>
SANDBOX_TARGET=us
```

The `SANDBOX_TARGET=us` must match the `DEFAULT_REGION_ID` configured on the API (`us`).

---

## Critical Architecture Decisions

### Runner Networking: No `--network host`, No Host Docker Socket

The runner container must NOT use `--network host` or mount the host's Docker socket. The correct configuration is:

- **No `--network host`**: The runner binary and DinD daemon must share the same network namespace (the container's default bridge). With `--network host`, the runner's `localhost` becomes the host's localhost, but DinD sandboxes bind to the DinD's internal bridge — making the toolbox unreachable from the runner process.

- **No `-v /var/run/docker.sock`**: Mounting the host Docker socket causes the runner to use the HOST Docker daemon for sandbox creation. The toolbox binary gets injected via volume mount from the runner container's filesystem, but the host Docker daemon looks for that path on the HOST filesystem where it doesn't exist, resulting in `/usr/local/bin/daytona: is a directory` errors.

- **No `-v /var/lib/daytona`**: Mounting the host's `/var/lib/daytona` overwrites the runner container's internal data directory which contains the toolbox binary.

- **`DOCKER_TLS_CERTDIR=` (empty)**: Must be set to disable DinD TLS. Without this, the DinD generates TLS certificates and the runner's port 3003 gets wrapped in TLS, causing connection resets.

- **Port mapping `-p 3003:3003`**: Required so the API can reach the runner's HTTP API from outside.

### DinD Data Persistence

The DinD Docker data is persisted via `-v /opt/daytona-runner/dind-data:/var/lib/docker`. This ensures:
- Pulled images survive runner restarts
- Sandbox containers and their state persist across restarts
- Build cache is retained

If this directory is cleared, ALL DinD images must be re-pulled (including the ~7GB default snapshot), which takes several minutes.

### Registry Authentication

The runner's DinD needs Docker credentials to pull from and push to the internal registry at `<VPS_IP>:6000`. This is handled by:
1. A `docker login` during Ansible deployment that creates `/opt/daytona-runner/docker-config/config.json`
2. Mounting this config into the runner container at `/root/.docker:ro`

### Insecure Registry

The internal registry runs over HTTP (not HTTPS) on port 6000. The runner's DinD must be configured to trust it via `/etc/docker/daemon.json`:
```json
{"insecure-registries": ["<VPS_IP>:6000"]}
```
This is templated by Ansible and mounted into the runner container.

---

## Troubleshooting Reference

### Problem: Runner logs show "401 Unauthorized"
**Cause**: No runner record exists in the API database. The `DEFAULT_RUNNER_NAME` env var was not set on the API.
**Fix**: Ensure `DEFAULT_RUNNER_NAME=default` is set on the API service. Without it, the API never creates the default runner record, so no token will match.

### Problem: API logs show "No available runners"
**Cause**: Same as above, or the runner's health check has gone stale.
**Fix**: Set `DEFAULT_RUNNER_NAME=default` and restart the API. If the runner was restarted, also restart the API so it re-registers the runner.

### Problem: "timeout waiting for daemon to start"
**Cause (1)**: The runner container is using `--network host`, which separates the runner process's localhost from the DinD's bridge network. The toolbox starts inside the sandbox container but the runner can't reach it on `localhost:2280`.
**Fix**: Remove `--network host`, use `-p 3003:3003` instead, and set `DOCKER_TLS_CERTDIR=` (empty).

**Cause (2)**: The DinD doesn't have the snapshot image cached. First sandbox creation triggers an image pull that takes minutes, exceeding the daemon start timeout.
**Fix**: Pre-pull the snapshot image into DinD: `docker exec daytona-runner docker pull <registry-image-ref>`. With persistent DinD data, this only needs to be done once.

### Problem: "/usr/local/bin/daytona: is a directory: permission denied"
**Cause**: The host Docker socket was mounted into the runner container. When the runner creates a sandbox, the HOST Docker daemon tries to mount the toolbox binary from the HOST filesystem, where the path doesn't exist (or is a directory created by Ansible).
**Fix**: Do NOT mount `/var/run/docker.sock`. Let the runner use its own DinD daemon.

### Problem: Runner push to registry stuck / "no basic auth credentials"
**Cause**: The DinD Docker daemon doesn't have registry credentials.
**Fix**: Mount Docker config with registry credentials: `-v /opt/daytona-runner/docker-config:/root/.docker:ro`. The credentials are created by the Ansible playbook via `docker login`.

### Problem: "server gave HTTP response to HTTPS client" on registry pull
**Cause**: The DinD daemon.json doesn't include the registry as an insecure registry, or TLS is interfering.
**Fix**: Mount the daemon.json with `insecure-registries` configured, and set `DOCKER_TLS_CERTDIR=` to disable DinD TLS.

### Problem: Connection reset on runner API port 3003
**Cause**: DinD TLS is enabled (default). The DinD generates TLS certs in `/certs` and wraps all connections in TLS.
**Fix**: Set `DOCKER_TLS_CERTDIR=` (empty) to disable TLS. Clear `/opt/daytona-runner/dind-data/` to remove existing TLS certs, then restart.

### Problem: Dex crashes with "malformed bcrypt hash"
**Cause**: The `$` characters in the bcrypt hash were interpreted by the shell when sourcing the `.env` file (double quotes).
**Fix**: Use single quotes: `export DAYTONA_DEX_ADMIN_PASSWORD_HASH='$2y$10$...'`

### Problem: Caddy crashes with "on_demand_tls 'interval' option is no longer supported"
**Cause**: Newer Caddy versions removed `interval` and `burst` from `on_demand_tls`.
**Fix**: Remove those lines, keep only the `ask` directive.

### Problem: Dashboard CORS error for Dex
**Cause**: Dex is on a separate domain and doesn't set CORS headers.
**Fix**: Add CORS headers in Caddy's Dex site block.

### Problem: Snapshot creation returns 403
**Cause (1)**: The `Memory` field in `CreateSnapshotParams` exceeds the org's `max_memory_per_sandbox` quota. The Daytona API interprets memory values in **GB**, not MB. Passing `Memory: 2048` means 2048 GB, which exceeds any reasonable quota.
**Fix**: Use GB values (e.g., `Memory: 2` for 2 GB).

**Cause (2)**: A snapshot with the same name already exists.
**Fix**: Delete the existing snapshot or use a different name.

**Cause (3)**: A stale `DAYTONA_API_KEY` environment variable is set from sourcing the ansible `.env`.
**Fix**: Remove `DAYTONA_API_KEY` from the ansible `.env` and unset it in your shell.

### Problem: Snapshot creation returns 428 "Precondition Required"
**Cause**: The user's organization doesn't have a default region set.
**Fix**: Log into the dashboard and set the default region, or update the database: `UPDATE organization SET "defaultRegionId" = 'us' WHERE "defaultRegionId" IS NULL;`

### Problem: Default snapshot stuck in "pulling" state
**Cause**: The runner can't push the snapshot image to the registry (auth issues, insecure registry not configured, or DinD networking problems).
**Fix**: Check runner logs for push errors. Ensure registry credentials and insecure-registry config are mounted into the runner container.

### Problem: Runner marked as "UNRESPONSIVE" by API
**Cause**: After runner restart, the runner gets a fresh start but the API still has the old runner record. The health check interval elapses.
**Fix**: Restart the API to re-register the default runner.
