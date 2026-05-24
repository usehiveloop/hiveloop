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

### `BRIDGE_WEBSOCKET_ENABLED`

Enable the WebSocket event stream endpoint at `/ws/events`. When enabled, a single WebSocket connection receives ALL events from ALL agents and conversations — a more efficient alternative to webhooks for high-throughput control planes.

- **Default:** `false`
- **Valid values:** `true`, `false`

```bash
export BRIDGE_WEBSOCKET_ENABLED="true"
```

Can be used alongside webhooks (both enabled) or as the sole event delivery mechanism (no `BRIDGE_WEBHOOK_URL`). Clients authenticate via the `?token=` query parameter using the control plane API key.

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

### `BRIDGE_MAX_CONCURRENT_LLM_CALLS`

Global ceiling on simultaneous LLM API calls across all agents.

- **Default:** `500`
- **Format:** Positive integer
- **Example:** `200`

Prevents overwhelming upstream LLM providers when many conversations are active at once. Calls beyond the limit are queued until a slot opens.

```bash
export BRIDGE_MAX_CONCURRENT_LLM_CALLS="200"
```

### `BRIDGE_STORAGE_PATH`

Path to a SQLite database for persistence.

- **Default:** (not set — persistence disabled)
- **Format:** File path (string)
- **Example:** `/var/lib/bridge/bridge.db`

When set, enables persistent storage for:
- Agent definitions
- Conversation history
- Event log
- Metrics snapshots
- Subagent session persistence

When unset, all of the above are ephemeral and lost on restart.

```bash
export BRIDGE_STORAGE_PATH="/var/lib/bridge/bridge.db"
```

### `BRIDGE_ATTACHMENTS_DIR`

Root directory for conversation message attachments written via the `full_message` field on `POST /conversations/{id}/messages`. See [Large payloads via `full_message`](../core-concepts/conversations.md#large-payloads-via-full_message).

- **Default:** `.bridge-attachments` (relative to the bridge process's working directory)
- **Format:** Directory path (string)
- **Example:** `/var/run/bridge/attachments`

Each conversation gets its own subdirectory `{root}/{conversation_id}/`. Files are named with UUIDs. Whole subdirectory is deleted when the conversation ends.

```bash
export BRIDGE_ATTACHMENTS_DIR="/var/run/bridge/attachments"
```

### `BRIDGE_ALLOW_STDIO_MCP_FROM_API`

Allow API clients to attach `stdio`-transport MCP servers to a conversation via the `mcp_servers` field in `POST /agents/{id}/conversations`. See [Per-Conversation MCP Servers](../core-concepts/mcp.md#per-conversation-mcp-servers).

- **Default:** `false`
- **Valid values:** `true`, `false`
- **Added in:** v0.18.0

```bash
export BRIDGE_ALLOW_STDIO_MCP_FROM_API="true"
```

**Security note:** stdio transport spawns a subprocess with Bridge's privileges and environment. Leave this off unless every API caller is trusted AND Bridge is sandboxed. `streamable_http` per-conversation MCP servers are always allowed regardless of this flag — the gate is strictly on stdio. Agent-level MCP servers (from control-plane-pushed agent definitions) are unaffected.

### `BRIDGE_STANDALONE_AGENT`

Enable the environment system reminder. When true, every conversation automatically receives a `<system-reminder>` block describing the specialist sandbox: pre-installed tools (Node.js, Go, Rust, Python, databases, browsers, etc.), and live resource usage (memory, CPU, disk) read from cgroup files so the agent sees actual sandbox limits — not the host machine's.

Intended for standalone agents running in a Daytona dev-box sandbox.

- **Default:** `false`
- **Valid values:** `true`, `false`

```bash
export BRIDGE_STANDALONE_AGENT="true"
```

The environment snapshot refreshes every 5 turns to keep resource numbers current without adding overhead to every message. The pre-installed tools list is a static baseline from the sandbox template — it does not include tools the agent installs during the session (the agent already knows about those).

### `BRIDGE_VERIFIER_DISABLED`

Force-disable the [verifier agent](../core-concepts/verifier-agent.md) for every conversation, regardless of per-agent `config.verifier.enabled`. Use to kill verifier traffic in an outage without touching agent definitions or redeploying.

- **Default:** unset (verifier honors per-agent config)
- **Valid values:** any non-empty value disables the verifier; conventionally `1`
- **Added in:** v0.23.0

```bash
export BRIDGE_VERIFIER_DISABLED="1"
```

When set, bridge logs `BRIDGE_VERIFIER_DISABLED set; verifier off` once per conversation start and proceeds as if the agent had `verifier.enabled = false`. No `verifier_started` / `verifier_verdict` / `verifier_error` events fire.

### `BRIDGE_OTEL_ENDPOINT`

OpenTelemetry OTLP gRPC endpoint for trace export.

- **Default:** (not set — tracing disabled)
- **Format:** URL (string)
- **Example:** `http://localhost:4317`

When set, Bridge exports distributed traces to the specified OpenTelemetry collector.

```bash
export BRIDGE_OTEL_ENDPOINT="http://localhost:4317"
```

### `BRIDGE_OTEL_SERVICE_NAME`

Service name reported in OpenTelemetry traces. Only relevant when `BRIDGE_OTEL_ENDPOINT` is set.

- **Default:** `bridge`
- **Format:** String
- **Example:** `bridge-production`

```bash
export BRIDGE_OTEL_SERVICE_NAME="bridge-production"
```

### `BRIDGE_WEB_URL`

Base URL of an external web tools service. When set, bridge registers `web_search`, `web_crawl`, `web_get_links`, `web_screenshot`, and `web_transform` and routes them to this URL. Without it, only `web_fetch` (in-process) is available.

- **Default:** (not set — only `web_fetch` is registered)
- **Format:** URL (string)
- **Example:** `http://localhost:7100`

```bash
export BRIDGE_WEB_URL="http://web-tools.internal"
```

### `BRIDGE_DISABLE_CACHE_CONTROL`

When `true`, disables the `cache_control` middleware on the LLM provider stack. Diagnostic only — leaving caching off costs significant tokens against providers that support prompt caching (Anthropic, etc.). Leave unset in production.

- **Default:** `false`
- **Format:** `true` / `false`
- **Example:** `true`

```bash
export BRIDGE_DISABLE_CACHE_CONTROL="true"
```

### `BRIDGE_DISABLE_RTK`

When `true`, disables the rtk filter pipeline that the `bash` tool routes its output through. Without rtk, bash returns raw command output (much larger). Diagnostic only.

- **Default:** `false`
- **Format:** `true` / `false`

```bash
export BRIDGE_DISABLE_RTK="true"
```

### `BRIDGE_TOOL_CHOICE`

Optional global override for the LLM `tool_choice` parameter passed to providers. Routed through the `tool_choice` middleware on the provider stack.

- **Default:** provider default (typically `auto`)
- **Format:** one of `auto`, `any`, `none`
- **Example:** `auto`

```bash
export BRIDGE_TOOL_CHOICE="auto"
```

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
initialization_options = { "preferences" = { "importModuleSpecifierPreference" = "relative" } }
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
export BRIDGE_WEBSOCKET_ENABLED="true"
export BRIDGE_CONTROL_PLANE_URL="https://api.example.com"

# Optional limits
export BRIDGE_MAX_CONCURRENT_CONVERSATIONS="1000"
export BRIDGE_MAX_CONCURRENT_LLM_CALLS="500"

# Optional: persistence
export BRIDGE_STORAGE_PATH="/var/lib/bridge/bridge.db"


# Optional: OpenTelemetry tracing
export BRIDGE_OTEL_ENDPOINT="http://localhost:4317"
export BRIDGE_OTEL_SERVICE_NAME="bridge-production"

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

### Validation Summary

| Variable | Rules |
|----------|-------|
| `BRIDGE_CONTROL_PLANE_API_KEY` | Required, non-empty string |
| `BRIDGE_LISTEN_ADDR` | Valid socket address (IP:port) |
| `BRIDGE_LOG_LEVEL` | One of: `debug`, `info`, `warn`, `error` |
| `BRIDGE_LOG_FORMAT` | One of: `text`, `json` |
| `BRIDGE_DRAIN_TIMEOUT_SECS` | Positive integer (seconds) |
| `BRIDGE_MAX_CONCURRENT_CONVERSATIONS` | Positive integer (or omit for unlimited) |
| `BRIDGE_MAX_CONCURRENT_LLM_CALLS` | Positive integer |
| `BRIDGE_STORAGE_PATH` | Valid file path (or omit to disable) |
| `BRIDGE_ALLOW_STDIO_MCP_FROM_API` | `true` or `false` (default `false`) |
| `BRIDGE_STANDALONE_AGENT` | `true` or `false` (default `false`) |
| `BRIDGE_OTEL_ENDPOINT` | Valid URL (or omit to disable) |
| `BRIDGE_OTEL_SERVICE_NAME` | Non-empty string |

---

## See Also

- [Configuration](../getting-started/configuration.md) — Full configuration guide
- [Docker Deployment](../deployment/docker-deployment.md) — Configuration in containers
- [Kubernetes](../deployment/kubernetes.md) — ConfigMaps and secrets
