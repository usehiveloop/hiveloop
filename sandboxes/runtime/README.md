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
| `BRIDGE_WEBSOCKET_ENABLED` | No | `false` | Enable WebSocket event stream on `/ws/events` |
| `BRIDGE_DRAIN_TIMEOUT_SECS` | No | `60` | Graceful shutdown timeout in seconds |
| `BRIDGE_MAX_CONCURRENT_CONVERSATIONS` | No | unlimited | Max concurrent conversations |
| `BRIDGE_MAX_CONCURRENT_LLM_CALLS` | No | `500` | Global ceiling on simultaneous outbound LLM API calls |
| `BRIDGE_SKILL_DISCOVERY_ENABLED` | No | `false` | Auto-discover skills from `.claude/skills/`, `.cursor/rules/`, etc. |
| `BRIDGE_SKILL_DISCOVERY_DIR` | No | cwd | Working directory for skill discovery |
| `BRIDGE_ALLOW_STDIO_MCP_FROM_API` | No | `false` | Allow API clients to attach `stdio` MCP servers per conversation |
| `BRIDGE_STANDALONE_AGENT` | No | `false` | Inject sandbox environment system reminder (resource limits, installed tools) |
| `BRIDGE_OTEL_ENDPOINT` | No | — | OpenTelemetry OTLP gRPC endpoint for trace export (e.g. `http://localhost:4317`) |
| `BRIDGE_OTEL_SERVICE_NAME` | No | `bridge` | OpenTelemetry service name |

#### config.toml

```toml
control_plane_url = "http://localhost:3000"
control_plane_api_key = "<bridge-control-plane-api-key>"
listen_addr = "0.0.0.0:8080"
log_level = "info"
log_format = "text"
drain_timeout_secs = 60
webhook_url = "https://your-control-plane/webhooks"
websocket_enabled = true  # Enable /ws/events endpoint
max_concurrent_llm_calls = 500
skill_discovery_enabled = false
skill_discovery_dir = "/path/to/workspace"
allow_stdio_mcp_from_api = false
standalone_agent = false
otel_endpoint = "http://localhost:4317"
otel_service_name = "bridge"

# Optional: webhook delivery tuning (ignored when webhook_url is not set)
[webhook_config]
max_concurrent_deliveries = 50
max_idle_connections = 20
delivery_timeout_secs = 10
max_retries = 3
worker_idle_timeout_secs = 300

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

### Install LSP Servers (Optional)

Install LSP server binaries for code intelligence support. This is a standalone subcommand — run it once (or as part of your image build), then start bridge normally:

```bash
# Install specific servers
bridge install-lsp rust,go,typescript

# Install all 40+ available servers
bridge install-lsp all

# Start the server (no install step)
bridge
```

Already-installed servers are skipped. The command exits with a non-zero status if any server fails.

**Available servers include:**
- **JavaScript/TypeScript**: typescript, eslint, biome, deno, vue, svelte, astro, tailwindcss
- **Systems**: rust, go, zig, clangd
- **Python**: python (pyright), ruff, pylsp
- **Web**: yaml, json, dockerfile, terraform, graphql
- **JVM**: jdtls (Java), kotlin-ls
- **Ruby**: ruby-lsp, ruby-lsp-official
- **Functional**: haskell, elixir, gleam, ocaml, elm, clojure
- **And more**: scala (metals), php, lua, bash, dart, cmake, ansible, vimls, nix, etc.

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
| GET | `/ws/events?token={api_key}` | WebSocket stream for all events (all agents/conversations) |
| GET | `/events?token={api_key}&after={seq}` | Poll for events (fallback when WS/SSE fails) |
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
| `conversation_created` | Conversation initialized |
| `message_received` | User message received |
| `message_start` | New assistant message begins |
| `content_delta` | Text content chunk |
| `reasoning_delta` | Extended thinking / reasoning chunk |
| `tool_call_start` | Tool call initiated |
| `tool_call_result` | Tool execution result |
| `tool_approval_required` | Tool call waiting for user approval |
| `tool_approval_resolved` | Approval decision made |
| `todo_updated` | Task list update (see [Todo Tools](#todo-tools)) |
| `background_task_completed` | Background bash/subagent task finished |
| `sub_agent_started` | Sub-agent conversation spawned |
| `sub_agent_completed` | Sub-agent conversation finished |
| `chain_started` | Multi-step chain began |
| `chain_completed` | Multi-step chain finished |
| `conversation_compacted` | Conversation context was compacted |
| `turn_completed` | Agent turn finished (all tool calls resolved) |
| `message_end` | Message complete |
| `conversation_ended` | Conversation terminated |
| `error` | Error occurred |
| `done` | Stream terminated |

## Todo Tools

Bridge includes built-in task management tools that agents can use to track progress. When both tools are enabled for an agent, a task list summary is automatically included in the system reminder.

### `todowrite` - Create/Update Task List

Replaces the entire todo list. Each call must include **all** items (completed, in-progress, and pending).

**Parameters:**
```json
{
  "todos": [
    {
      "content": "Task description",
      "status": "in_progress",
      "priority": "high"
    }
  ]
}
```

| Field | Type | Description |
|-------|------|-------------|
| `content` | string | Task description |
| `status` | string | One of: `pending`, `in_progress`, `completed`, `cancelled` |
| `priority` | string | One of: `high`, `medium`, `low` |

**Key behaviors:**
- **Replace-all semantics**: Each call replaces the entire list
- To update one item, send the full list with that item changed
- Only one task should be `in_progress` at a time
- Mark tasks `completed` immediately after finishing

### `todoread` - Read Current Task List

Takes no parameters. Returns the current todo list with `content`, `status`, and `priority` for each item.

**Example response:**
```json
{
  "todos": [
    {"content": "Implement feature", "status": "completed", "priority": "high"},
    {"content": "Write tests", "status": "in_progress", "priority": "high"},
    {"content": "Update docs", "status": "pending", "priority": "medium"}
  ],
  "total": 3,
  "incomplete_count": 2
}
```

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
  storage/   # Conversation persistence (SQLite backend)
  tools/     # Built-in tool implementations (filesystem, bash, search, todo)
  mcp/       # Model Context Protocol client (stdio + HTTP transports)
  lsp/       # Language Server Protocol integration
  webhooks/  # Webhook dispatching with HMAC signing
e2e/         # End-to-end test harness and mock services
fixtures/    # Test data (agent definitions, workspaces)
```
