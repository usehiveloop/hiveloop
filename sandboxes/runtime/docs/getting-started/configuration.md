# Configuration

Bridge loads configuration from three sources, in order of priority:

1. **Built-in defaults** — Sensible starting points
2. **`config.toml`** — File in the working directory (if present)
3. **Environment variables** — `BRIDGE_*` prefixed vars (highest priority)

Later sources override earlier ones.

---

## Environment Variables

### Required

| Variable | Description |
|----------|-------------|
| `BRIDGE_CONTROL_PLANE_API_KEY` | Secret key for `/push/*` endpoints. Also used as bearer token. |

### Optional

| Variable | Default | Description |
|----------|---------|-------------|
| `BRIDGE_LISTEN_ADDR` | `0.0.0.0:8080` | Where Bridge listens for connections |
| `BRIDGE_LOG_LEVEL` | `info` | How much to log: `debug`, `info`, `warn`, `error` |
| `RUST_LOG` | — | Overrides `BRIDGE_LOG_LEVEL`. Uses [env-filter syntax](https://docs.rs/tracing-subscriber/latest/tracing_subscriber/filter/struct.EnvFilter.html) |
| `BRIDGE_LOG_FORMAT` | `text` | Log format: `text` or `json` |
| `BRIDGE_CONTROL_PLANE_URL` | `http://localhost:3000` | Your control plane URL (used by integration tools) |
| `BRIDGE_WEBHOOK_URL` | — | Where to send webhook events |
| `BRIDGE_WEBSOCKET_ENABLED` | `false` | Enable WebSocket event stream on `/ws/events` |
| `BRIDGE_DRAIN_TIMEOUT_SECS` | `60` | How long to wait for conversations to finish before shutdown |
| `BRIDGE_MAX_CONCURRENT_CONVERSATIONS` | unlimited | Limit concurrent conversations |

See the [Environment Variables Reference](../reference/environment-variables.md) for complete details including validation rules and format requirements.

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

# Optional: Control plane URL (for tool proxying)
control_plane_url = "https://your-api.com"

# Optional: Webhook delivery
webhook_url = "https://your-api.com/webhooks/bridge"

# Optional: WebSocket event stream (all events on a single connection)
websocket_enabled = true

# Optional: Graceful shutdown timeout
drain_timeout_secs = 60

# Optional: Limit concurrent conversations
max_concurrent_conversations = 1000
```

**Note:** Environment variables override values in `config.toml`. For example, if both set `log_level`, the environment variable wins.

---

## LSP Configuration

If you want code intelligence tools, configure LSP servers in `config.toml` (LSP cannot be configured via environment variables):

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

To disable all LSP servers:

```toml
lsp = false
```

Bridge will start these servers when needed for the `lsp_query` tool.

### LSP Server Options

Each LSP server supports these options:

| Option | Required | Description |
|--------|----------|-------------|
| `command` | Yes | Array of command and arguments to launch the server |
| `extensions` | No | File extensions this server handles |
| `env` | No | Environment variables for the server process |
| `initialization_options` | No | Custom initialization options (JSON) |
| `disabled` | No | Set to `true` to disable this server |

Example with all options:

```toml
[lsp.rust]
command = ["rust-analyzer"]
extensions = ["rs"]
env = { "RUST_LOG" = "debug" }
initialization_options = { "checkOnSave" = true }
disabled = false
```

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

# If using WebSocket event stream (alternative or complement to webhooks)
export BRIDGE_WEBSOCKET_ENABLED="true"

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
ERROR: failed to load configuration: missing field `control_plane_api_key`
```

Fix the issue and restart.

### Validation Rules

| Variable | Rules |
|----------|-------|
| `BRIDGE_CONTROL_PLANE_API_KEY` | Required, non-empty string |
| `BRIDGE_LISTEN_ADDR` | Valid socket address (IP:port) |
| `BRIDGE_LOG_LEVEL` | One of: `debug`, `info`, `warn`, `error` |
| `BRIDGE_LOG_FORMAT` | One of: `text`, `json` |
| `BRIDGE_DRAIN_TIMEOUT_SECS` | Positive integer (seconds) |
| `BRIDGE_MAX_CONCURRENT_CONVERSATIONS` | Positive integer (or omit for unlimited) |

---

## Docker Secrets

Bridge does not natively support the `_FILE` suffix pattern for Docker secrets. To use Docker secrets, you have two options:

**Option 1: Load secrets in entrypoint script**

Create an `entrypoint.sh`:

```bash
#!/bin/sh
if [ -f "$BRIDGE_CONTROL_PLANE_API_KEY_FILE" ]; then
  export BRIDGE_CONTROL_PLANE_API_KEY=$(cat "$BRIDGE_CONTROL_PLANE_API_KEY_FILE")
fi
exec "$@"
```

**Option 2: Use init containers or secret injection**

Many orchestration platforms can inject secrets as environment variables directly.

---

## See Also

- [Docker deployment](docker.md) — Configuration in containers
- [Kubernetes deployment](../deployment/kubernetes.md) — ConfigMaps and secrets
- [Environment Variables Reference](../reference/environment-variables.md) — Complete list with format details
