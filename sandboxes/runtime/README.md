# Portal Bridge

A multi-agent LLM runtime that manages AI-powered conversations with tool execution, MCP server integration, and real-time streaming. Bridge starts with zero agents and exposes an HTTP API — the control plane pushes agent definitions to it via the `/push/*` endpoints. Once agents are loaded, clients can create conversations and stream responses in real time.

## Quick Start

### Prerequisites

- [Rust](https://rustup.rs/) (stable toolchain)

### Build

```bash
# Debug
make build

# Release (optimized, stripped)
make build-release
```

### Configure

Bridge loads configuration in this order (later sources override earlier ones):

1. Built-in defaults
2. `config.toml` in the working directory (if present)
3. Environment variables prefixed with `BRIDGE_`

#### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `BRIDGE_CONTROL_PLANE_URL` | No | `""` | Control plane URL, used by integration tools to proxy requests |
| `BRIDGE_CONTROL_PLANE_API_KEY` | Yes | `""` | API key for control plane authentication (also used as bearer token for `/push/*` routes) |
| `BRIDGE_LISTEN_ADDR` | No | `0.0.0.0:8080` | Address and port to listen on |
| `BRIDGE_LOG_LEVEL` | No | `info` | Log level (`debug`, `info`, `warn`, `error`) |
| `BRIDGE_LOG_FORMAT` | No | `text` | Log format (`text` or `json`) |
| `BRIDGE_WEBHOOK_URL` | No | — | Webhook URL for event delivery (HMAC-signed) |
| `BRIDGE_DRAIN_TIMEOUT_SECS` | No | `60` | Graceful shutdown timeout in seconds |
| `BRIDGE_MAX_CONCURRENT_CONVERSATIONS` | No | unlimited | Max concurrent conversations |

#### config.toml

```toml
control_plane_url = "http://localhost:3000"
control_plane_api_key = "<bridge-control-plane-api-key>"
listen_addr = "0.0.0.0:8080"
log_level = "info"
log_format = "text"
drain_timeout_secs = 60
webhook_url = "https://your-control-plane/webhooks"

# Optional: LSP servers for code intelligence tools
# Set to false to disable, or configure per-server:
[lsp.rust]
command = ["rust-analyzer"]
extensions = ["rs"]

[lsp.typescript]
command = ["typescript-language-server", "--stdio"]
extensions = ["ts", "tsx", "js", "jsx"]
```

### Run

```bash
# With environment variables
BRIDGE_CONTROL_PLANE_URL=http://localhost:3000 \
BRIDGE_CONTROL_PLANE_API_KEY=your-key \
make run

# Or with a config.toml in the working directory
make run

# Release mode
make run-release
```

The server starts on **port 8080** by default.

## API Endpoints

### Health & Metrics

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Health check with uptime |
| GET | `/metrics` | Per-agent metrics (tokens, requests, latency) |

### Agents

| Method | Path | Description |
|--------|------|-------------|
| GET | `/agents` | List all loaded agents |
| GET | `/agents/{agent_id}` | Get agent details |

### Conversations

| Method | Path | Description |
|--------|------|-------------|
| POST | `/agents/{agent_id}/conversations` | Create a new conversation |
| POST | `/conversations/{conv_id}/messages` | Send a message |
| GET | `/conversations/{conv_id}/stream` | SSE stream for real-time events |
| POST | `/conversations/{conv_id}/abort` | Abort the current turn |
| DELETE | `/conversations/{conv_id}` | End a conversation |

### Tool Approvals

| Method | Path | Description |
|--------|------|-------------|
| GET | `/agents/{agent_id}/conversations/{conv_id}/approvals` | List pending approvals |
| POST | `/agents/{agent_id}/conversations/{conv_id}/approvals/{request_id}` | Resolve a single approval |
| POST | `/agents/{agent_id}/conversations/{conv_id}/approvals` | Bulk resolve approvals |

### Control Plane Push (bearer token required)

| Method | Path | Description |
|--------|------|-------------|
| POST | `/push/agents` | Bulk load agents |
| PUT | `/push/agents/{agent_id}` | Upsert a single agent |
| DELETE | `/push/agents/{agent_id}` | Remove an agent |
| PATCH | `/push/agents/{agent_id}/api-key` | Rotate an agent's LLM API key at runtime (no drain) |
| POST | `/push/agents/{agent_id}/conversations` | Hydrate conversation history |
| POST | `/push/diff` | Apply incremental agent diffs |

Full OpenAPI spec is available in [`openapi.json`](openapi.json). Regenerate it with:

```bash
make openapi
```

## SSE Events

The `/conversations/{conv_id}/stream` endpoint emits these Server-Sent Events:

| Event | Description |
|-------|-------------|
| `message_start` | New assistant message begins |
| `content_delta` | Text content chunk |
| `tool_call_start` | Tool call initiated |
| `tool_call_result` | Tool execution result |
| `tool_approval_required` | Tool call waiting for user approval |
| `tool_approval_resolved` | Approval decision made |
| `todo_updated` | Task list update |
| `message_end` | Message complete |
| `error` | Error occurred |
| `done` | Stream terminated |

## Supported LLM Providers

Bridge supports native and OpenAI-compatible providers. Native providers use each vendor's own API format and auth mechanism; OpenAI-compatible providers use the `/chat/completions` format with a custom `base_url`.

| Provider | Type | Notes |
|----------|------|-------|
| Anthropic | Native | `x-api-key` auth, `/v1/messages` endpoint |
| Google Gemini | Native | API key as query param, Gemini content format |
| Cohere | Native | Bearer auth, `/v2/chat` endpoint |
| OpenAI | OpenAI-compatible | `base_url` required |
| Groq | OpenAI-compatible | |
| DeepSeek | OpenAI-compatible | |
| Mistral | OpenAI-compatible | |
| xAI | OpenAI-compatible | |
| Together AI | OpenAI-compatible | |
| Fireworks AI | OpenAI-compatible | |
| Ollama | OpenAI-compatible | |
| Custom | OpenAI-compatible | |

Provider, model, API key, and `base_url` are configured per-agent via the control plane. OpenAI-compatible providers require `base_url` in the agent definition. Native providers (Anthropic, Gemini, Cohere) use their default endpoints if `base_url` is omitted.

## Development

```bash
make help          # Show all available targets
make check         # Type-check all crates
make fmt           # Format code
make lint          # Run clippy
make test          # Run unit tests
make test-e2e      # Run end-to-end tests
make test-all      # Run everything (requires API keys below)
make setup-lsp     # Install LSP servers for integration tests
```

### E2E Test API Keys

The end-to-end tests make real LLM calls. Set these environment variables (or add them to `.env`):

| Variable | Required for |
|----------|-------------|
| `FIREWORKS_API_KEY` | OpenAI-compatible provider tests |
| `ANTHROPIC_API_KEY` | Anthropic native provider test |
| `GEMINI_API_KEY` | Gemini native provider test |
| `COHERE_API_KEY` | Cohere native provider test |

## Project Structure

```
crates/
  bridge/    # Main binary, config loading, entry point
  api/       # HTTP handlers, routing, middleware, SSE
  core/      # Domain models, schemas, error types, config
  llm/       # LLM provider integration, tool call execution, permissions
  runtime/   # Agent supervisor, conversation management
  tools/     # Built-in tool implementations (filesystem, bash, search)
  mcp/       # Model Context Protocol client (stdio + HTTP transports)
  lsp/       # Language Server Protocol integration
  webhooks/  # Webhook dispatching with HMAC signing
e2e/         # End-to-end test harness and mock services
fixtures/    # Test data (agent definitions, workspaces)
```
