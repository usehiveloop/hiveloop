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
| `BRIDGE_STORAGE_PATH` | No | — | Path to the local sqlite DB. When set, bridge persists agent state, conversations, the webhook outbox, journal entries, and the `artifact_uploads` resume table. When unset, the system runs in-memory only. |
| `BRIDGE_ATTACHMENTS_DIR` | No | `./.bridge-attachments` | Root directory for `full_message` attachments (large per-message payloads offloaded to disk). Cleaned up when the conversation ends. |
| `BRIDGE_WEB_URL` | No | — | Base URL of an external web tools service. When set, bridge registers `web_search`, `web_crawl`, `web_get_links`, `web_screenshot`, `web_transform` and routes them to this URL. |
| `BRIDGE_DISABLE_CACHE_CONTROL` | No | `false` | When `true`, disables the `cache_control` middleware on the LLM provider stack. Diagnostic only — leave off in production. |
| `BRIDGE_DISABLE_RTK` | No | `false` | When `true`, disables the rtk filter pipeline that bash output is routed through (token-efficient command output). |
| `BRIDGE_TOOL_CHOICE` | No | provider default | Optional override for the LLM `tool_choice` parameter (`auto`, `any`, `none`). Routed through the `tool_choice` middleware. |

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

# Install every bundled server (~30)
bridge install-lsp all

# Start the server (no install step)
bridge
```

Already-installed servers are skipped. Per-server failures are non-fatal — if a server's underlying package manager isn't on the host (e.g. `cargo`, `go`, `python3`, `npm`), that id is logged as a warning and skipped; the command still exits 0. The tail of the log summarises which ids were skipped so the operator can install the missing toolchain and re-run `bridge install-lsp <id>`.

Only servers with broadly-available install methods (npm, pip, cargo, go, or self-contained curl/wget downloads) are bundled. Servers that would require niche toolchains (`opam`, `gem`, `dart pub`, `cs`/Coursier, `dotnet tool`, `ghcup`, `nix`) have been dropped from the installer — the runtime can still launch them if the binary is already on `PATH`, you just need to install it yourself.

**Bundled servers:**
- **JavaScript/TypeScript**: `typescript`, `eslint`, `biome`, `deno`, `vue`, `svelte`, `astro`, `tailwindcss`
- **Systems**: `rust`, `go`, `zig`, `clangd`
- **Python**: `python` (pyright), `ruff`, `pylsp`
- **Config / infra**: `yaml-ls`, `dockerfile`, `terraform`, `graphql`, `cmake`, `ansible`
- **JVM / BEAM / JVM-ish**: `jdtls` (Java), `elixir-ls`, `clojure-lsp`
- **Misc**: `php`, `bash`, `prisma`, `elm`, `tinymist` (Typst), `vimls`

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

SSE uses legacy wire-level event names that map to bridge's internal `event_type` field. The mapping is fixed in `crates/api/src/sse.rs`. WebSocket and webhook channels use the JSON `event_type` (snake_case enum) instead — see [docs/api-reference/sse-events.md](docs/api-reference/sse-events.md#sse-event-name-mapping) for the full table.

| SSE Event Name | Internal `event_type` | Description |
|----------------|------------------------|-------------|
| `conversation_created` | `conversation_created` | Conversation initialized |
| `message_received` | `message_received` | User message received (carries `attachment_path` when `full_message` was supplied) |
| `message_start` | `response_started` | Assistant began generating a response |
| `content_delta` | `response_chunk` | Streaming response text chunk |
| `message_end` | `response_completed` | Assistant finished its response |
| `reasoning_delta` | `reasoning_delta` | Extended thinking / reasoning chunk |
| `tool_call_start` | `tool_call_started` | Tool call initiated |
| `tool_call_result` | `tool_call_completed` | Tool call finished (result or error in payload) |
| `tool_approval_required` | `tool_approval_required` | Tool call waiting for user approval |
| `tool_approval_resolved` | `tool_approval_resolved` | Approval decision made |
| `todo_updated` | `todo_updated` | Task list update (see [Todo Tools](#todo-tools)) |
| `background_task_completed` | `background_task_completed` | Background bash/subagent task finished |
| `sub_agent_started` | `sub_agent_started` | Sub-agent conversation spawned |
| `sub_agent_completed` | `sub_agent_completed` | Sub-agent conversation finished |
| `chain_started` | `chain_started` | Immortal-mode chain handoff begun |
| `chain_completed` | `chain_completed` | Immortal-mode chain handoff finished successfully |
| `chain_failed` | `chain_failed` | Immortal-mode chain handoff attempt errored (conversation continues with oversized history) |
| `context_pressure_warning` | `context_pressure_warning` | Cumulative tool-output bytes exceeded ~1.5× immortal `token_budget` (once per turn) |
| `turn_completed` | `turn_completed` | Agent turn finished (all tool calls resolved) |
| `conversation_ended` | `conversation_ended` | Conversation terminated |
| `error` | `agent_error` | Error occurred during agent execution |
| `done` | `done` | Stream terminated |

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

## Workspace Artifacts

Agents can stream files (CSVs, videos, audio, markdown, etc.) from their sandbox up to the control plane by setting an `artifacts` block on the agent definition. When present, bridge auto-registers a single tool — `upload_to_workspace` — backed by a tus.io v1.0.0 resumable upload client.

### Configuration

```json
{
  "id": "agent_demo",
  "name": "Demo",
  "system_prompt": "...",
  "provider": { "...": "..." },
  "artifacts": {
    "upload_url": "https://control-plane.example.com/workspaces/ws_42/uploads",
    "download_url": "https://control-plane.example.com/workspaces/ws_42/files",
    "max_size_bytes": 524288000,
    "accepted_file_types": ["csv", "md", "video/*", "audio/mpeg"],
    "max_concurrent_uploads": 4,
    "chunk_size_bytes": 8388608,
    "headers": { "X-Workspace-Id": "ws_42" }
  }
}
```

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `upload_url` | string | yes | tus.io creation endpoint. Must be `http`/`https`. |
| `download_url` | string | no | Surfaced back to the agent in the tool response. |
| `max_size_bytes` | number | yes | Hard ceiling enforced before any network I/O. |
| `accepted_file_types` | string[] | yes | Each entry is either a bare extension (`csv`) or a MIME type (`text/csv`, `video/*`). |
| `max_concurrent_uploads` | number | no | Per-agent concurrency cap. Default `4`. |
| `chunk_size_bytes` | number | no | PATCH chunk size. Default `8 MiB`. |
| `headers` | object | no | Forwarded on every TUS request (creation + chunks). |

### Tool: `upload_to_workspace`

Arguments:

```json
{
  "path": "/workspace/output/report.csv",
  "content_type": "text/csv",
  "metadata": { "run_id": "r_123", "label": "weekly" }
}
```

| Arg | Required | Notes |
|-----|----------|-------|
| `path` | yes | Absolute path inside the agent's sandbox. Refuses missing, non-regular, or empty files. |
| `content_type` | no | MIME override; bridge guesses from the extension via `mime_guess` otherwise. |
| `metadata` | no | Free-form `string → string` map. Keys must be ASCII and contain no spaces or commas (TUS Creation extension constraint); invalid keys are dropped. The map is encoded into the `Upload-Metadata` header alongside auto-injected `filename` and `sha256` entries. |

Result (returned as a JSON string):

```json
{
  "artifact_id": "<sha256-derived idempotency key>",
  "upload_url": "<tus location URL>",
  "download_url": "<value of artifacts.download_url, or null>",
  "size": 12345,
  "content_type": "text/csv",
  "sha256": "<hex-encoded SHA-256 of the file>"
}
```

### Resilience

- **Streaming**: chunks are read into a buffer of size `chunk_size_bytes` and PATCH'd as they go — never the whole file in memory.
- **Retry**: transient `5xx` and network errors are retried with jittered exponential backoff (up to 6 retries — 7 attempts total — with delays from 250 ms to 30 s). `4xx` responses other than `409` are fatal; `409 Conflict` is handled separately as an offset realign.
- **Resume**: when `BRIDGE_STORAGE_PATH` is set, in-flight uploads persist `(idempotency_key, location, bytes_sent)` to the local sqlite. If bridge restarts, a re-call of the tool with the same file (`agent_id + abs_path + file SHA-256` match) re-`HEAD`s the server, realigns to the authoritative `Upload-Offset`, and continues from there. Without a storage backend, the same recovery still happens within a single tool call but does not survive a process restart.
- **Idempotency**: completing the same upload twice is a no-op — the cached control-plane response is returned without re-uploading.
- **Integrity**: every PATCH carries an `Upload-Checksum: sha256 …` header; the full-file SHA-256 is computed pre-upload and included in the tool result for downstream verification.
- **Auth**: when `BRIDGE_CONTROL_PLANE_API_KEY` is set, every TUS request (creation, HEAD, PATCH) carries `Authorization: Bearer <key>`.
- **Concurrency**: a per-agent semaphore caps concurrent uploads at `max_concurrent_uploads` across all of that agent's conversations.

`search_workspace`, `download_from_workspace`, and any other companion tools are out of scope for bridge — the control plane wires them in per agent via the standard `mcp_servers` field on `AgentDefinition`.

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
