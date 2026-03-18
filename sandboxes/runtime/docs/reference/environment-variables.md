# Environment Variables

Complete reference for Bridge environment variables.

**Note:** Bridge also has a CLI. Run `bridge --help` to see available commands like `bridge tools list --json`.

---

## Required

### `BRIDGE_CONTROL_PLANE_API_KEY`

API key for authenticating push endpoints.

```bash
export BRIDGE_CONTROL_PLANE_API_KEY="<bridge-control-plane-api-key>"
```

Also used as the bearer token for `/push/*` routes.

**Validation:** Must be non-empty. Bridge will fail to start if not set.

---

## Optional

### `BRIDGE_LISTEN_ADDR`

Address and port to listen on.

- **Default:** `0.0.0.0:8080`
- **Format:** `<ip_address>:<port>`
- **Example:** `127.0.0.1:3000`, `0.0.0.0:8080`

### `BRIDGE_LOG_LEVEL`

How much to log. Ignored if `RUST_LOG` is set (see below).

- **Default:** `info`
- **Valid values:** `debug`, `info`, `warn`, `error`

### `RUST_LOG`

Standard Rust logging directive that overrides `BRIDGE_LOG_LEVEL` if set.

- **Format:** Uses [`tracing-subscriber` env-filter syntax](https://docs.rs/tracing-subscriber/latest/tracing_subscriber/filter/struct.EnvFilter.html)
- **Example:** `RUST_LOG=debug` or `RUST_LOG=my_crate=debug,hyper=error`

```bash
# Set specific module levels
export RUST_LOG="bridge=debug,hyper=warn"
```

### `BRIDGE_LOG_FORMAT`

Log output format.

- **Default:** `text`
- **Valid values:** `text`, `json`

Use `json` for production (easier to parse by log aggregation systems).

### `BRIDGE_CONTROL_PLANE_URL`

Your control plane URL. Used by integration tools to proxy requests to your backend.

- **Default:** `http://localhost:3000` (if not set)
- **Example:** `https://api.example.com`

```bash
export BRIDGE_CONTROL_PLANE_URL="https://api.example.com"
```

### `BRIDGE_WEBHOOK_URL`

Where to send webhook events. When set, all SSE events are also dispatched as webhooks to this URL, signed with the control plane API key.

- **Default:** (not set — webhooks disabled)
- **Example:** `https://api.example.com/webhooks/bridge`

```bash
export BRIDGE_WEBHOOK_URL="https://api.example.com/webhooks/bridge"
```

### `BRIDGE_DRAIN_TIMEOUT_SECS`

Graceful shutdown timeout.

- **Default:** `60`
- **Unit:** seconds
- **Format:** Positive integer (u64)

How long to wait for conversations to finish before forcing shutdown.

### `BRIDGE_MAX_CONCURRENT_CONVERSATIONS`

Limit concurrent conversations across all agents.

- **Default:** (not set — unlimited)
- **Format:** Positive integer
- **Example:** `1000`

Use to prevent resource exhaustion. When the limit is reached, new conversations will be rejected until existing ones complete.

---

## Configuration File (config.toml)

Some configuration options can only be set via the `config.toml` file, not environment variables:

### LSP Configuration

Configure Language Server Protocol servers for code intelligence tools:

```toml
# Disable all LSP servers
lsp = false

# Or configure specific servers
[lsp.rust]
command = ["rust-analyzer"]
extensions = ["rs"]

[lsp.typescript]
command = ["typescript-language-server", "--stdio"]
extensions = ["ts", "tsx", "js", "jsx"]
env = { "TSSERVER_LOG" = "verbose" }
disabled = false
```

**Note:** The `[lsp]` section cannot be set via environment variables. Use the config file for LSP configuration.

---

## Configuration Precedence

Configuration is loaded in this order (later sources override earlier ones):

1. **Built-in defaults** — Sensible starting points
2. **`config.toml`** — File in the working directory (if present)
3. **Environment variables** — `BRIDGE_*` prefixed vars (highest priority)

### Example

Given this `config.toml`:

```toml
log_level = "info"
listen_addr = "0.0.0.0:8080"
```

And these environment variables:

```bash
export BRIDGE_LOG_LEVEL="debug"
```

The actual values will be:

- `log_level = "debug"` (environment overrides config.toml)
- `listen_addr = "0.0.0.0:8080"` (from config.toml)

---

## Docker Secrets

Bridge does not automatically support the `_FILE` suffix pattern for Docker secrets. To use secrets in Docker:

**Option 1: Use environment variables with Docker Compose secrets**

```yaml
services:
  bridge:
    image: bridge:latest
    secrets:
      - bridge_api_key
    environment:
      BRIDGE_CONTROL_PLANE_API_KEY: /run/secrets/bridge_api_key
    entrypoint: ["sh", "-c"]
    command: >
      'export BRIDGE_CONTROL_PLANE_API_KEY=$$(cat /run/secrets/bridge_api_key) &&
       exec bridge'

secrets:
  bridge_api_key:
    file: ./secrets/api_key.txt
```

**Option 2: Use a startup script**

```bash
#!/bin/sh
# entrypoint.sh
if [ -f "$BRIDGE_CONTROL_PLANE_API_KEY_FILE" ]; then
  export BRIDGE_CONTROL_PLANE_API_KEY=$(cat "$BRIDGE_CONTROL_PLANE_API_KEY_FILE")
fi
exec "$@"
```

---

## Example Configuration

```bash
# Required
export BRIDGE_CONTROL_PLANE_API_KEY="<bridge-control-plane-api-key>"

# Recommended for production
export BRIDGE_LOG_LEVEL="info"
export BRIDGE_LOG_FORMAT="json"
export BRIDGE_DRAIN_TIMEOUT_SECS="120"
export BRIDGE_WEBHOOK_URL="https://api.example.com/webhooks/bridge"
export BRIDGE_CONTROL_PLANE_URL="https://api.example.com"

# Optional limits
export BRIDGE_MAX_CONCURRENT_CONVERSATIONS="1000"

# Run
./bridge
```

---

## Validation

Bridge validates configuration on startup. If something is wrong, it will print an error and exit immediately. Common errors:

```
ERROR: failed to load configuration: missing field `control_plane_api_key`
```

Fix the issue and restart.

---

## See Also

- [Configuration](../getting-started/configuration.md) — Full configuration guide
- [Docker Deployment](../deployment/docker-deployment.md) — Configuration in containers
- [Kubernetes](../deployment/kubernetes.md) — ConfigMaps and secrets
