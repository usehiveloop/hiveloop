# Configuration

Bridge loads configuration from three sources, in order of priority:

1. **Built-in defaults** — Sensible starting points
2. **Environment variables** — `BRIDGE_*` prefixed vars
3. **config.toml** — File in the working directory (if present)

Later sources override earlier ones.

---

## Environment Variables

These are the main configuration options:

| Variable | Required | Default | What it does |
|----------|----------|---------|--------------|
| `BRIDGE_CONTROL_PLANE_API_KEY` | **Yes** | — | Secret key for `/push/*` endpoints. Also used as bearer token. |
| `BRIDGE_LISTEN_ADDR` | No | `0.0.0.0:8080` | Where Bridge listens for connections |
| `BRIDGE_LOG_LEVEL` | No | `info` | How much to log: `debug`, `info`, `warn`, `error` |
| `BRIDGE_LOG_FORMAT` | No | `text` | Log format: `text` or `json` |
| `BRIDGE_CONTROL_PLANE_URL` | No | — | Your control plane URL (used by some tools) |
| `BRIDGE_WEBHOOK_URL` | No | — | Where to send webhook events |
| `BRIDGE_DRAIN_TIMEOUT_SECS` | No | `60` | How long to wait for conversations to finish before shutdown |
| `BRIDGE_MAX_CONCURRENT_CONVERSATIONS` | No | unlimited | Limit concurrent conversations |

---

## Minimal Configuration

For local development, you only need:

```bash
export BRIDGE_CONTROL_PLANE_API_KEY="<bridge-control-plane-api-key>"
./target/release/bridge
```

---

## Using config.toml

Create a `config.toml` in the directory where you run Bridge:

```toml
# Where Bridge listens
listen_addr = "0.0.0.0:8080"

# Security
control_plane_api_key = "<bridge-control-plane-api-key>"

# Logging
log_level = "info"
log_format = "text"

# Optional: Webhook delivery
webhook_url = "https://your-api.com/webhooks/bridge"

# Optional: Control plane URL (for tool proxying)
control_plane_url = "https://your-api.com"

# Optional: Graceful shutdown timeout
drain_timeout_secs = 60
```

---

## LSP Configuration

If you want code intelligence tools, configure LSP servers:

```toml
[lsp.rust]
command = ["rust-analyzer"]
extensions = ["rs"]

[lsp.typescript]
command = ["typescript-language-server", "--stdio"]
extensions = ["ts", "tsx", "js", "jsx"]

[lsp.python]
command = ["pylsp"]
extensions = ["py"]
```

Bridge will start these servers when needed for the `lsp_query` tool.

---

## Production Configuration

For production, use environment variables or a mounted config file:

```bash
# Required
export BRIDGE_CONTROL_PLANE_API_KEY="$(cat /run/secrets/bridge_api_key)"

# Recommended
export BRIDGE_LOG_LEVEL="info"
export BRIDGE_LOG_FORMAT="json"
export BRIDGE_DRAIN_TIMEOUT_SECS="120"

# If using webhooks
export BRIDGE_WEBHOOK_URL="https://api.yourservice.com/webhooks/bridge"

# Optional: limit resources
export BRIDGE_MAX_CONCURRENT_CONVERSATIONS="1000"

./bridge
```

---

## Configuration Precedence Example

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

- `log_level = "debug"` (environment overrides config)
- `listen_addr = "0.0.0.0:8080"` (from config)

---

## Validation

Bridge validates configuration on startup. If something is wrong, it will print an error and exit immediately. For example:

```
ERROR: BRIDGE_CONTROL_PLANE_API_KEY is required
```

Fix the issue and restart.

---

## See Also

- [Docker deployment](docker.md) — Configuration in containers
- [Kubernetes deployment](../deployment/kubernetes.md) — ConfigMaps and secrets
- [Environment Variables Reference](../reference/environment-variables.md) — Complete list
