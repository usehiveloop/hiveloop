# Environment Variables

Complete reference for Bridge environment variables.

---

## Required

### `BRIDGE_CONTROL_PLANE_API_KEY`

API key for authenticating push endpoints.

```bash
export BRIDGE_CONTROL_PLANE_API_KEY="<bridge-control-plane-api-key>"
```

Also used as the bearer token for `/push/*` routes.

---

## Optional

### `BRIDGE_LISTEN_ADDR`

Address and port to listen on.

- Default: `0.0.0.0:8080`
- Example: `127.0.0.1:3000`

### `BRIDGE_LOG_LEVEL`

How much to log.

- Values: `debug`, `info`, `warn`, `error`
- Default: `info`

### `BRIDGE_LOG_FORMAT`

Log output format.

- Values: `text`, `json`
- Default: `text`

Use `json` for production (easier to parse).

### `BRIDGE_CONTROL_PLANE_URL`

Your control plane URL.

Used by some tools to proxy requests to your backend.

### `BRIDGE_WEBHOOK_URL`

Where to send webhook events.

```bash
export BRIDGE_WEBHOOK_URL="https://api.example.com/webhooks/bridge"
```

### `BRIDGE_DRAIN_TIMEOUT_SECS`

Graceful shutdown timeout.

- Default: `60`
- Unit: seconds

How long to wait for conversations to finish before forcing shutdown.

### `BRIDGE_MAX_CONCURRENT_CONVERSATIONS`

Limit concurrent conversations.

- Default: unlimited
- Example: `1000`

Use to prevent resource exhaustion.

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

# Optional limits
export BRIDGE_MAX_CONCURRENT_CONVERSATIONS="1000"

# Run
./bridge
```

---

## Configuration Precedence

1. Built-in defaults
2. Environment variables
3. `config.toml` file

Later sources override earlier ones.

---

## See Also

- [Configuration](../getting-started/configuration.md)
