# Bridge Runtime Engine — Implementation Plan

## 1. Project Overview

A single Rust binary that runs AI agents. On startup, it fetches agent definitions from a control plane, establishes MCP connections, and exposes a REST API for agent conversations with SSE streaming. It delivers webhooks for every agent event and supports hot-reload of agent configurations with graceful drain.

### Constraints
- Binary size: < 25 MB (expected: ~4-6 MB)
- Memory at rest: < 25 MB (expected: ~3-5 MB)
- Single binary, zero external runtime dependencies

---

## 2. Technology Stack

| Layer | Crate | Version | Purpose |
|---|---|---|---|
| **Agent Framework** | `rig-core` | 0.31 | Agent loop, tool calling, multi-provider LLM, streaming |
| **MCP Client** | `rmcp` | 0.17 | Official MCP SDK — stdio + Streamable HTTP transports |
| **HTTP Server** | `axum` | 0.8 | REST API + SSE streaming |
| **Async Runtime** | `tokio` | 1.x | Full async runtime, channels, task tracking |
| **Agent Map** | `dashmap` | 6.x | Sharded concurrent hashmap for agent registry |
| **HTTP Client** | `reqwest` | 0.12 | LLM API calls, web fetch tool, webhook delivery |
| **Serialization** | `serde` + `serde_json` | 1.x | All JSON serialization/deserialization |
| **JSON Schema** | `jsonschema` + `schemars` | 0.38 / 0.8 | Schema generation from Rust types, LLM output validation |
| **Retry** | `backon` | 1.x | Exponential backoff for LLM calls, webhooks, sync |
| **Rate Limiting** | `governor` | 0.8 | Per-agent and global rate limiting |
| **Middleware** | `tower` + `tower-http` | 0.5 / 0.6 | Timeout, CORS, request logging |
| **Webhook Signing** | `hmac` + `sha2` + `base64` | 0.12 / 0.10 / 0.22 | HMAC-SHA256 webhook signatures |
| **Content Extraction** | `dom_smoothie` | 0.15 | Mozilla Readability algorithm — article extraction |
| **HTML to Markdown** | `htmd` | 0.5 | Turndown.js-equivalent HTML→Markdown conversion |
| **File Walking** | `ignore` | 0.4 | Gitignore-aware recursive directory walking |
| **Glob Matching** | `globset` | 0.4 | High-performance glob pattern matching (ripgrep ecosystem) |
| **Content Search** | `grep-regex` + `grep-searcher` | 0.1 | Ripgrep's actual search engine — context lines, line numbers, file type filters |
| **Regex** | `regex` | 1.x | Ripgrep-compatible regex for Grep tool |
| **Config** | `figment` | 0.10 | Runtime config from env vars + TOML |
| **Logging** | `tracing` + `tracing-subscriber` | 0.1 / 0.3 | Structured logging (fmt layer only, no OTel) |
| **Graceful Drain** | `tokio-util` | 0.7 | CancellationToken + TaskTracker |
| **IDs** | `uuid` | 1.x | Conversation and request IDs |
| **Time** | `chrono` | 0.4 | Timestamps for events and metrics |

### Explicitly NOT included
- **OpenTelemetry** — no distributed tracing, no OTel SDK
- **svix** — webhook delivery is DIY with reqwest + hmac + backon
- **scraper** — replaced by dom_smoothie + htmd for better readability extraction

### Web Tool Rationale

For Claude Code-quality web content extraction, we use a two-stage pipeline:

1. **`dom_smoothie`** — Implements Mozilla's Readability algorithm (same heuristics as Firefox Reader View). Strips navigation, sidebars, ads, scripts, footers. Extracts the main article content. Tested as the only Rust readability crate that correctly identified main content on all benchmark pages ([source](https://emschwartz.me/comparing-13-rust-crates-for-extracting-text-from-html/)).

2. **`htmd`** — Converts cleaned HTML to Markdown. Inspired by turndown.js (the JavaScript gold standard), passes all turndown.js test cases. Preserves headings, lists, links, code blocks, and tables.

Fallback: if dom_smoothie fails to extract content (non-article pages), we fall back to `htmd` directly on the full HTML with script/style tags stripped.

---

## 3. Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                         BRIDGE BINARY                               │
│                                                                     │
│  ┌──────────────┐  ┌───────────────┐  ┌──────────────────────────┐ │
│  │  HTTP Server  │  │  Config Sync  │  │  Webhook Dispatcher      │ │
│  │  (axum)       │  │  (poller)     │  │  (reqwest + hmac)        │ │
│  │  - REST API   │  │  - interval   │  │  - async delivery        │ │
│  │  - SSE stream │  │    pull from  │  │  - HMAC-SHA256 signing   │ │
│  │  - /metrics   │  │    control    │  │  - exponential backoff   │ │
│  │  - /health    │  │    plane      │  │    via backon             │ │
│  └──────┬────────┘  └──────┬────────┘  └────────────┬─────────────┘ │
│         │                  │                        │               │
│  ┌──────▼──────────────────▼────────────────────────▼─────────────┐ │
│  │                    AGENT SUPERVISOR                             │ │
│  │                                                                 │ │
│  │  agent_map: DashMap<AgentId, Arc<AgentState>>                   │ │
│  │                                                                 │ │
│  │  global_cancel: CancellationToken     (shutdown signal)         │ │
│  │  global_tracker: TaskTracker          (drain all tasks)         │ │
│  │                                                                 │ │
│  │  For each agent:                                                │ │
│  │  ┌────────────────────────────────────────────────────────────┐ │ │
│  │  │  AgentState                                                │ │ │
│  │  │  ├─ definition: AgentDefinition    (identity, prompt, etc) │ │ │
│  │  │  ├─ rig_agent: rig::Agent          (configured LLM agent)  │ │ │
│  │  │  ├─ mcp_clients: Vec<McpClient>    (rmcp connections)      │ │ │
│  │  │  ├─ tool_registry: ToolRegistry    (Read,Glob,LS,Grep,Web+MCP) │ │ │
│  │  │  ├─ conversations: DashMap<ConvId, ConversationHandle>     │ │ │
│  │  │  ├─ cancel: CancellationToken      (per-agent cancel)      │ │ │
│  │  │  ├─ tracker: TaskTracker           (per-agent drain)       │ │ │
│  │  │  └─ metrics: AgentMetrics          (atomic counters)       │ │ │
│  │  └────────────────────────────────────────────────────────────┘ │ │
│  └─────────────────────────────────────────────────────────────────┘ │
│                                                                     │
│  ┌─────────────────────────────────────────────────────────────────┐ │
│  │  METRICS COLLECTOR                                               │ │
│  │  Per-agent atomic counters:                                      │ │
│  │    input_tokens, output_tokens, total_requests,                  │ │
│  │    active_conversations, total_conversations,                    │ │
│  │    tool_calls, errors, latency_sum, latency_count               │ │
│  │  Exposed via GET /metrics as JSON                                │ │
│  └─────────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────────┘
```

---

## 4. Crate Structure

```
useportal-bridge/
├── Cargo.toml                        # Workspace root
├── plan.md                           # This file
├── crates/
│   ├── bridge-core/                  # Shared types, traits, errors
│   │   └── src/
│   │       ├── lib.rs
│   │       ├── agent.rs              # AgentDefinition, AgentId, AgentConfig
│   │       ├── conversation.rs       # ConversationId, Message, Role, ToolCall
│   │       ├── tool.rs               # ToolDefinition, ToolSchema
│   │       ├── skill.rs              # SkillDefinition, SkillId
│   │       ├── provider.rs           # ProviderType, ModelConfig
│   │       ├── mcp.rs                # McpServerDefinition
│   │       ├── webhook.rs            # WebhookEvent, WebhookPayload
│   │       ├── metrics.rs            # AgentMetrics (atomic counters), MetricsSnapshot
│   │       ├── config.rs             # RuntimeConfig
│   │       └── error.rs              # BridgeError, Result type alias
│   │
│   ├── bridge-llm/                   # rig-core integration
│   │   └── src/
│   │       ├── lib.rs
│   │       ├── factory.rs            # Build rig Agent from AgentDefinition
│   │       ├── providers.rs          # Provider client constructors per ProviderType
│   │       ├── streaming.rs          # Streaming response adapter (rig → SSE events)
│   │       └── tool_adapter.rs       # Bridge our ToolDefinition → rig Tool trait
│   │
│   ├── bridge-mcp/                   # MCP connection management
│   │   └── src/
│   │       ├── lib.rs
│   │       ├── manager.rs            # McpManager: create/destroy connections per agent
│   │       ├── connection.rs         # Single MCP client connection lifecycle
│   │       └── tool_bridge.rs        # Discover MCP tools → register as rig tools
│   │
│   ├── bridge-tools/                 # Built-in tools (all readonly, high-performance)
│   │   └── src/
│   │       ├── lib.rs
│   │       ├── registry.rs           # ToolRegistry: lookup by name, merge built-in + MCP
│   │       ├── read.rs               # Read file contents with line numbers, offset/limit
│   │       ├── glob.rs               # Find files by glob pattern (ignore + globset)
│   │       ├── ls.rs                 # List directory contents with metadata
│   │       ├── grep.rs               # Ripgrep-powered content search with context lines
│   │       ├── web_search.rs         # Search API wrapper (Brave/Tavily/SerpAPI)
│   │       ├── web_fetch.rs          # URL fetch → dom_smoothie → htmd → Markdown
│   │       └── skill_tools.rs        # Skill fetch/activate tools
│   │
│   ├── bridge-runtime/               # Core runtime engine
│   │   └── src/
│   │       ├── lib.rs
│   │       ├── supervisor.rs         # AgentSupervisor: manages all agents lifecycle
│   │       ├── agent_state.rs        # AgentState: per-agent runtime state
│   │       ├── agent_map.rs          # DashMap<AgentId, Arc<AgentState>> wrapper
│   │       ├── conversation.rs       # ConversationEngine: runs agent loop per conversation
│   │       ├── drain.rs              # Graceful drain: wait for in-flight, then swap
│   │       └── token_tracker.rs      # Atomic token usage recording
│   │
│   ├── bridge-api/                   # HTTP server
│   │   └── src/
│   │       ├── lib.rs
│   │       ├── router.rs             # Axum Router with all routes
│   │       ├── state.rs              # AppState (Arc<AgentSupervisor>, webhook sender, etc.)
│   │       ├── middleware.rs          # Request ID injection, error formatting
│   │       ├── handlers/
│   │       │   ├── mod.rs
│   │       │   ├── conversations.rs  # POST create, POST message, DELETE end
│   │       │   ├── agents.rs         # GET list, GET detail
│   │       │   ├── stream.rs         # GET SSE stream for conversation
│   │       │   ├── metrics.rs        # GET /metrics — JSON metrics snapshot
│   │       │   └── health.rs         # GET /health — readiness + liveness
│   │       └── sse.rs                # SSE event formatting helpers
│   │
│   ├── bridge-sync/                  # Control plane synchronization
│   │   └── src/
│   │       ├── lib.rs
│   │       ├── poller.rs             # Interval-based control plane polling
│   │       ├── diff.rs               # Diff current agents vs fetched: added/updated/removed
│   │       └── updater.rs            # Apply diffs: add agent, drain+update, drain+remove
│   │
│   ├── bridge-webhooks/              # Webhook delivery
│   │   └── src/
│   │       ├── lib.rs
│   │       ├── signer.rs             # HMAC-SHA256 signing with timestamp
│   │       ├── dispatcher.rs         # Async delivery: fire-and-forget with retry via backon
│   │       └── events.rs             # Serialize webhook events to JSON payloads
│   │
│   └── bridge-bin/                   # Binary entrypoint
│       └── src/
│           └── main.rs               # Startup sequence, signal handling, shutdown
│
├── e2e/                              # E2E test infrastructure
│   ├── mock-control-plane/           # Full mock control plane (separate binary)
│   │   └── src/
│   │       ├── main.rs               # Axum server: agent CRUD, webhook receiver
│   │       ├── store.rs              # In-memory agent store with change tracking
│   │       ├── routes.rs             # All control plane API endpoints
│   │       └── webhook_log.rs        # Records received webhooks for assertion
│   │
│   └── tests/                        # E2E test suite
│       ├── harness.rs                # Start mock CP + bridge binary, health wait
│       ├── test_agent_loading.rs     # Verify agents fetched and loaded on startup
│       ├── test_conversations.rs     # Full conversation lifecycle
│       ├── test_streaming.rs         # SSE streaming responses
│       ├── test_webhooks.rs          # Webhook delivery for all event types
│       ├── test_hot_reload.rs        # Agent add/update/delete via control plane
│       ├── test_drain.rs             # Graceful drain during updates
│       ├── test_concurrent.rs        # Multiple conversations, multiple agents
│       ├── test_mcp.rs               # MCP tool discovery and execution
│       ├── test_tools.rs             # Built-in tool execution (Read, Glob, LS, Grep, web)
│       ├── test_metrics.rs           # Token tracking and metrics endpoint
│       ├── test_errors.rs            # Error handling (bad requests, timeouts)
│       └── test_skills.rs            # Skill fetch and activation
│
└── fixtures/                         # Test fixtures
    ├── agents/                       # Sample agent definitions (JSON)
    ├── mcp-servers/                  # Test MCP server configs
    ├── conversations/                # Canned conversation histories
    ├── html/                         # Sample HTML pages for web_fetch tests
    └── workspace/                    # Mock file tree for Read/Glob/LS/Grep tests
        ├── src/
        │   ├── main.rs
        │   ├── lib.rs
        │   └── utils/
        │       └── helpers.rs
        ├── tests/
        │   └── integration.rs
        ├── .gitignore
        └── README.md
```

---

## 5. Dependency Configuration

### Workspace Cargo.toml

```toml
[workspace]
resolver = "2"
members = [
    "crates/bridge-core",
    "crates/bridge-llm",
    "crates/bridge-mcp",
    "crates/bridge-tools",
    "crates/bridge-runtime",
    "crates/bridge-api",
    "crates/bridge-sync",
    "crates/bridge-webhooks",
    "crates/bridge-bin",
    "e2e/mock-control-plane",
]

[workspace.dependencies]
# Async runtime
tokio = { version = "1", features = ["full"] }
tokio-util = { version = "0.7", features = ["rt"] }
tokio-stream = "0.1"

# HTTP server
axum = "0.8"
tower = { version = "0.5", features = ["timeout", "limit"] }
tower-http = { version = "0.6", features = ["trace", "cors", "request-id"] }

# HTTP client
reqwest = { version = "0.12", default-features = false, features = [
    "rustls-tls", "json", "stream"
] }

# Agent framework
rig-core = { version = "0.31", default-features = false, features = [
    "derive", "rmcp", "reqwest-rustls"
] }

# MCP
rmcp = { version = "0.17", default-features = false, features = [
    "client",
    "transport-io",
    "transport-child-process",
    "transport-streamable-http-client",
    "transport-streamable-http-client-reqwest",
    "macros",
] }

# Serialization
serde = { version = "1", features = ["derive"] }
serde_json = "1"

# Schema
jsonschema = "0.38"
schemars = "0.8"

# Concurrency
dashmap = "6"

# Resilience
backon = "1"
governor = "0.8"

# Webhook signing
hmac = "0.12"
sha2 = "0.10"
base64 = "0.22"

# Content extraction
dom_smoothie = { version = "0.15", features = ["aho-corasick"] }
htmd = "0.5"

# File system tools (readonly, high-performance)
ignore = "0.4"              # gitignore-aware directory walking (ripgrep ecosystem)
globset = "0.4"             # high-performance glob matching (ripgrep ecosystem)
grep-regex = "0.1"          # ripgrep's regex matcher
grep-searcher = "0.1"       # ripgrep's file searcher (context lines, line numbers)
grep-matcher = "0.1"        # ripgrep's matcher trait
regex = "1"                 # regex engine shared with grep-*

# Logging
tracing = "0.1"
tracing-subscriber = { version = "0.3", features = ["fmt", "env-filter", "json"] }

# Config
figment = { version = "0.10", features = ["toml", "env"] }

# Utilities
uuid = { version = "1", features = ["v4", "serde"] }
chrono = { version = "0.4", features = ["serde"] }
thiserror = "2"
anyhow = "1"
futures = "0.3"
async-trait = "0.1"

[profile.release]
strip = true
lto = true
opt-level = "z"
codegen-units = 1
panic = "abort"
```

---

## 6. Detailed Implementation — Per Crate

### 6.1 bridge-core

The foundation types shared by every other crate. Zero business logic.

#### `agent.rs`
```rust
pub type AgentId = String;

pub struct AgentDefinition {
    pub id: AgentId,
    pub name: String,
    pub system_prompt: String,
    pub provider: ProviderConfig,
    pub tools: Vec<ToolDefinition>,          // agent-defined tools
    pub mcp_servers: Vec<McpServerDefinition>, // MCP connections
    pub skills: Vec<SkillDefinition>,
    pub config: AgentConfig,
    pub subagents: Vec<AgentDefinition>,     // nested subagent definitions
    pub webhook_url: Option<String>,
    pub webhook_secret: Option<String>,
}

pub struct AgentConfig {
    pub max_tokens: Option<u32>,
    pub max_turns: Option<u32>,
    pub temperature: Option<f64>,
    pub json_schema: Option<serde_json::Value>,  // structured output schema
    pub rate_limit_rpm: Option<u32>,              // requests per minute
}

pub struct ProviderConfig {
    pub provider_type: ProviderType,  // openai, anthropic, google, etc.
    pub model: String,                // gpt-4o, claude-sonnet-4-20250514, etc.
    pub api_key: String,
    pub base_url: Option<String>,     // custom endpoint
}
```

#### `conversation.rs`
```rust
pub type ConversationId = String;

pub struct Message {
    pub role: Role,
    pub content: Vec<ContentBlock>,
    pub timestamp: chrono::DateTime<chrono::Utc>,
}

pub enum Role { User, Assistant, System, Tool }

pub enum ContentBlock {
    Text(String),
    ToolCall(ToolCall),
    ToolResult(ToolResult),
    Image { media_type: String, data: String },
}

pub struct ToolCall {
    pub id: String,
    pub name: String,
    pub arguments: serde_json::Value,
}

pub struct ToolResult {
    pub tool_call_id: String,
    pub content: String,
    pub is_error: bool,
}
```

#### `metrics.rs`
```rust
use std::sync::atomic::{AtomicU64, Ordering};

pub struct AgentMetrics {
    pub input_tokens: AtomicU64,
    pub output_tokens: AtomicU64,
    pub total_requests: AtomicU64,
    pub failed_requests: AtomicU64,
    pub active_conversations: AtomicU64,
    pub total_conversations: AtomicU64,
    pub tool_calls: AtomicU64,
    pub latency_sum_ms: AtomicU64,        // sum for computing average
    pub latency_count: AtomicU64,
}

/// Snapshot for JSON serialization (GET /metrics)
pub struct MetricsSnapshot {
    pub agent_id: AgentId,
    pub agent_name: String,
    pub input_tokens: u64,
    pub output_tokens: u64,
    pub total_tokens: u64,
    pub total_requests: u64,
    pub failed_requests: u64,
    pub active_conversations: u64,
    pub total_conversations: u64,
    pub tool_calls: u64,
    pub avg_latency_ms: f64,
}
```

#### `webhook.rs`
```rust
pub enum WebhookEventType {
    ConversationCreated,
    MessageReceived,        // user message in
    ResponseStarted,        // assistant started generating
    ResponseChunk,          // streaming chunk
    ResponseCompleted,      // full response done
    ToolCallStarted,
    ToolCallCompleted,
    ConversationEnded,
    AgentError,
}

pub struct WebhookPayload {
    pub event_type: WebhookEventType,
    pub agent_id: AgentId,
    pub conversation_id: ConversationId,
    pub timestamp: chrono::DateTime<chrono::Utc>,
    pub data: serde_json::Value,
}
```

#### `mcp.rs`
```rust
pub struct McpServerDefinition {
    pub name: String,
    pub transport: McpTransport,
}

pub enum McpTransport {
    Stdio {
        command: String,
        args: Vec<String>,
        env: HashMap<String, String>,
    },
    StreamableHttp {
        url: String,
        headers: HashMap<String, String>,
    },
}
```

#### `config.rs`
```rust
pub struct RuntimeConfig {
    pub control_plane_url: String,
    pub control_plane_api_key: String,
    pub listen_addr: String,              // e.g. "0.0.0.0:8080"
    pub sync_interval_secs: u64,          // polling interval (default: 30)
    pub drain_timeout_secs: u64,          // max wait for drain (default: 60)
    pub max_concurrent_conversations: Option<usize>,
    pub log_level: String,                // e.g. "info"
    pub log_format: LogFormat,            // text or json
}

pub enum LogFormat { Text, Json }
```

---

### 6.2 bridge-llm

Wraps `rig-core` to build configured LLM agents from our `AgentDefinition`.

#### `factory.rs`

The core function: `build_agent(definition: &AgentDefinition, tools: Vec<impl Tool>) -> Result<rig::Agent>`

1. Match on `definition.provider.provider_type` to create the correct rig provider client:
   - `ProviderType::OpenAI` → `rig::providers::openai::Client::new(&api_key)`
   - `ProviderType::Anthropic` → `rig::providers::anthropic::Client::new(&api_key)`
   - `ProviderType::Google` → `rig::providers::gemini::Client::new(&api_key)`
   - etc. for all providers rig supports
2. Call `client.agent(&model_name)` to get an agent builder
3. Set `.preamble(&system_prompt)` for the system prompt
4. Set `.max_tokens(n)` and `.temperature(t)` from config
5. Add all tools via `.tool(tool)` for each registered tool
6. If `json_schema` is set, configure structured output
7. Build and return the agent

#### `streaming.rs`

Adapter that converts rig's streaming response into a `tokio::sync::mpsc::Sender<SseEvent>` stream:
- Maps rig's stream chunks to our SSE event format
- Tracks token usage from response metadata
- Emits webhook events for each chunk

#### `tool_adapter.rs`

Bridges our `ToolDefinition` type to rig's `Tool` trait:
- Implements `rig::tool::Tool` for a `DynamicTool` struct
- `DynamicTool` holds: name, description, JSON schema, and a callback function
- The callback dispatches to: built-in tool execution, MCP tool call, or subagent delegation

---

### 6.3 bridge-mcp

Manages MCP client connections per agent.

#### `manager.rs`

```rust
pub struct McpManager {
    /// Active connections keyed by (agent_id, server_name)
    connections: DashMap<(AgentId, String), McpConnection>,
}

impl McpManager {
    /// Connect to all MCP servers for an agent
    pub async fn connect_agent(&self, agent_id: &AgentId, servers: &[McpServerDefinition]) -> Result<Vec<McpConnection>>;

    /// Disconnect all MCP servers for an agent (during drain)
    pub async fn disconnect_agent(&self, agent_id: &AgentId) -> Result<()>;

    /// Discover tools from all connected MCP servers for an agent
    pub async fn discover_tools(&self, agent_id: &AgentId) -> Result<Vec<DiscoveredMcpTool>>;
}
```

#### `connection.rs`

Wraps a single `rmcp` client connection:
- For `McpTransport::Stdio`: use `rmcp::transport::child_process` to spawn the MCP server
- For `McpTransport::StreamableHttp`: use `rmcp::transport::streamable_http_client` with reqwest
- Handles connection initialization (capabilities exchange)
- Provides `call_tool(name, args)` and `list_tools()` methods

#### `tool_bridge.rs`

Converts discovered MCP tools into rig-compatible tools:
- Each MCP tool becomes a `DynamicTool` where the callback calls `mcp_connection.call_tool()`
- Preserves the tool's JSON schema from MCP discovery
- Handles serialization of arguments and deserialization of results

---

### 6.4 bridge-tools

Built-in tools shipped with the binary. All built-in filesystem and search tools are **strictly readonly** and designed for maximum performance. They use the ripgrep ecosystem internally — the same libraries that power ripgrep itself.

#### `registry.rs`

```rust
pub struct ToolRegistry {
    builtin_tools: HashMap<String, Arc<dyn ToolExecutor>>,
    mcp_tools: HashMap<String, McpToolHandle>,
}

pub trait ToolExecutor: Send + Sync {
    fn name(&self) -> &str;
    fn description(&self) -> &str;
    fn schema(&self) -> serde_json::Value;
    async fn execute(&self, args: serde_json::Value) -> Result<String>;
}
```

---

#### File System Tools (readonly, high-performance)

#### `read.rs` — Read file contents

Reads file contents with line numbering, offset/limit pagination for large files.

```rust
pub struct ReadTool;

#[derive(Deserialize, JsonSchema)]
pub struct ReadArgs {
    pub file_path: String,             // absolute path to file
    pub offset: Option<usize>,         // start line (1-indexed, default: 1)
    pub limit: Option<usize>,          // max lines to return (default: 2000)
}

#[derive(Serialize)]
pub struct ReadResult {
    pub content: String,               // lines with "  N\t" prefix (cat -n format)
    pub total_lines: usize,
    pub lines_read: usize,
    pub truncated: bool,               // true if file has more lines beyond offset+limit
}
```

Implementation details:
- Uses `tokio::fs::File` + `tokio::io::BufReader` + `tokio::io::AsyncBufReadExt::lines()`
- Line numbers formatted as `  N\t` prefix (matching `cat -n` output) for easy reference
- Lines longer than 2000 characters are truncated with `...` suffix
- Default limit: 2000 lines from start of file
- Returns `total_lines` so the caller knows if there's more content
- Handles binary file detection: reads first 8KB, checks for null bytes — returns error message if binary
- Returns clear error for: file not found, permission denied, is-a-directory

#### `glob.rs` — Find files by glob pattern

Fast file pattern matching using the ripgrep ecosystem (`ignore` + `globset`).

```rust
pub struct GlobTool;

#[derive(Deserialize, JsonSchema)]
pub struct GlobArgs {
    pub pattern: String,               // glob pattern, e.g. "src/**/*.ts"
    pub path: Option<String>,          // directory to search in (default: working dir)
}

#[derive(Serialize)]
pub struct GlobResult {
    pub files: Vec<FileEntry>,
    pub total_matches: usize,
    pub truncated: bool,               // true if > 1000 matches
}

#[derive(Serialize)]
pub struct FileEntry {
    pub path: String,                  // relative path from search root
    pub modified: Option<String>,      // ISO 8601 modified time
}
```

Implementation details:
- Uses `ignore::WalkBuilder` for directory traversal — automatically respects `.gitignore`, `.ignore`, `.git/info/exclude`
- Uses `globset::GlobBuilder` to compile the pattern once, then match against each walked path
- Results sorted by modification time (most recently modified first) — most relevant files surface first
- Cap at 1000 results to prevent memory bloat, set `truncated: true` if exceeded
- Supports standard glob syntax: `*`, `**`, `?`, `[abc]`, `{a,b}`
- Runs traversal on `tokio::task::spawn_blocking` to avoid blocking the async runtime on large directory trees
- Symlinks are followed by default

#### `ls.rs` — List directory contents

Lists directory entries with file metadata.

```rust
pub struct LsTool;

#[derive(Deserialize, JsonSchema)]
pub struct LsArgs {
    pub path: String,                  // directory path
}

#[derive(Serialize)]
pub struct LsResult {
    pub entries: Vec<DirEntry>,
    pub total_entries: usize,
}

#[derive(Serialize)]
pub struct DirEntry {
    pub name: String,
    pub entry_type: EntryType,         // file, directory, symlink
    pub size: Option<u64>,             // bytes (files only)
    pub modified: Option<String>,      // ISO 8601
}

#[derive(Serialize)]
pub enum EntryType { File, Directory, Symlink }
```

Implementation details:
- Uses `tokio::fs::read_dir` for async directory reading
- Collects metadata (type, size, modified) via `tokio::fs::metadata` per entry
- Sorted: directories first, then files, alphabetically within each group
- Cap at 1000 entries — deep directories should use Glob instead
- Returns clear errors for: not-a-directory, not found, permission denied

#### `grep.rs` — Content search (ripgrep-powered)

Full content search using ripgrep's actual search engine (`grep-searcher` + `grep-regex`). Supports context lines, file type filters, case-insensitive search, and multiple output modes.

```rust
pub struct GrepTool;

#[derive(Deserialize, JsonSchema)]
pub struct GrepArgs {
    pub pattern: String,               // regex pattern to search for
    pub path: Option<String>,          // file or directory to search (default: working dir)
    pub glob: Option<String>,          // glob filter, e.g. "*.ts", "*.{ts,tsx}"
    pub file_type: Option<String>,     // ripgrep type, e.g. "js", "py", "rust"
    pub case_insensitive: Option<bool>,// default: false
    pub context_before: Option<usize>, // lines before match (default: 0)
    pub context_after: Option<usize>,  // lines after match (default: 0)
    pub context: Option<usize>,        // lines before AND after (shorthand)
    pub output_mode: Option<OutputMode>, // content, files_with_matches, count
    pub max_results: Option<usize>,    // cap results (default: 200)
}

#[derive(Deserialize, JsonSchema)]
pub enum OutputMode {
    Content,              // show matching lines with context
    FilesWithMatches,     // just file paths (default)
    Count,                // match count per file
}

#[derive(Serialize)]
pub struct GrepResult {
    pub matches: Vec<GrepMatch>,
    pub total_matches: usize,
    pub files_searched: usize,
    pub truncated: bool,
}

#[derive(Serialize)]
pub struct GrepMatch {
    pub file: String,                  // file path
    pub line_number: Option<u64>,      // line number (content mode)
    pub content: Option<String>,       // matching line text (content mode)
    pub context_before: Option<Vec<String>>, // lines before match
    pub context_after: Option<Vec<String>>,  // lines after match
    pub count: Option<u64>,            // match count (count mode)
}
```

Implementation details:
- Uses `grep_regex::RegexMatcher` to compile the search pattern
- Uses `grep_searcher::Searcher` with `grep_searcher::SinkBuilder` for file searching
- Uses `ignore::WalkBuilder` for directory traversal (respects gitignore)
- File type filtering via `ignore::types::TypesBuilder` — supports all ripgrep built-in types (js, py, rust, go, java, ts, etc.)
- Glob filtering via `ignore::overrides::OverrideBuilder`
- Context lines (`-B`, `-A`, `-C` equivalent) handled natively by `grep_searcher::Searcher`
- Default output mode: `FilesWithMatches` (fastest — just returns paths)
- `Content` mode includes line numbers and matching text with optional context
- Default cap: 200 matches to prevent memory issues on broad searches
- Runs on `tokio::task::spawn_blocking` since grep-searcher is synchronous
- Pattern uses ripgrep syntax (not PCRE) — literal braces need escaping
- Multiline search support when explicitly requested

---

#### Web Tools

#### `web_search.rs`

Search API wrapper supporting configurable backends:

```rust
pub struct WebSearchTool {
    client: reqwest::Client,
    provider: SearchProvider,
    api_key: String,
}

pub enum SearchProvider { Brave, Tavily, SerpApi }
```

- Takes a query string, returns structured search results (title, url, snippet)
- Configurable max results, region, language
- Uses `backon` for retries on transient failures

#### `web_fetch.rs`

Two-stage content extraction pipeline:

```rust
pub struct WebFetchTool {
    client: reqwest::Client,
}

impl WebFetchTool {
    pub async fn fetch(&self, url: &str) -> Result<FetchResult> {
        // 1. HTTP GET with timeout and redirect following
        let html = self.client.get(url)
            .timeout(Duration::from_secs(30))
            .send().await?
            .text().await?;

        // 2. Try dom_smoothie readability extraction first
        if let Ok(article) = self.extract_article(&html, url) {
            return Ok(FetchResult {
                title: article.title,
                content: article.markdown_content,
                url: url.to_string(),
            });
        }

        // 3. Fallback: strip script/style tags, convert full HTML to markdown
        let markdown = self.fallback_convert(&html);
        Ok(FetchResult {
            title: None,
            content: markdown,
            url: url.to_string(),
        })
    }

    fn extract_article(&self, html: &str, url: &str) -> Result<Article> {
        // dom_smoothie with Markdown output mode
        let config = dom_smoothie::Config {
            text_mode: dom_smoothie::TextMode::Markdown,
            ..Default::default()
        };
        let mut readability = dom_smoothie::Readability::new(html, Some(url), config)?;
        readability.parse()
    }

    fn fallback_convert(&self, html: &str) -> String {
        // Use htmd to convert raw HTML to markdown
        // htmd handles script/style removal internally
        htmd::convert(html).unwrap_or_default()
    }
}
```

---

#### Skill Tools

#### `skill_tools.rs`

Two tools for the agent's skill system:
1. **`fetch_skills`** — takes a query, returns matching skills from the agent's skill definitions (fuzzy match on title + description)
2. **`activate_skill`** — takes a skill ID, fetches the full skill prompt/tools from the control plane, and adds them to the current conversation's tool set

---

### 6.5 bridge-runtime

The heart of the system. Manages agent lifecycle and conversations.

#### `supervisor.rs`

```rust
pub struct AgentSupervisor {
    agent_map: AgentMap,
    mcp_manager: Arc<McpManager>,
    webhook_dispatcher: Arc<WebhookDispatcher>,
    global_cancel: CancellationToken,
    global_tracker: TaskTracker,
    config: Arc<RuntimeConfig>,
}

impl AgentSupervisor {
    /// Load agents from control plane response
    pub async fn load_agents(&self, definitions: Vec<AgentDefinition>) -> Result<()>;

    /// Get agent state for serving a request
    pub fn get_agent(&self, agent_id: &AgentId) -> Option<Arc<AgentState>>;

    /// List all loaded agents
    pub fn list_agents(&self) -> Vec<AgentSummary>;

    /// Start a new conversation on an agent
    pub async fn create_conversation(
        &self,
        agent_id: &AgentId,
    ) -> Result<(ConversationId, mpsc::Receiver<SseEvent>)>;

    /// Send a message to an existing conversation
    pub async fn send_message(
        &self,
        agent_id: &AgentId,
        conversation_id: &ConversationId,
        message: Message,
    ) -> Result<()>;

    /// End a conversation
    pub async fn end_conversation(
        &self,
        agent_id: &AgentId,
        conversation_id: &ConversationId,
    ) -> Result<()>;

    /// Apply a config diff (called by sync poller)
    pub async fn apply_diff(&self, diff: AgentDiff) -> Result<()>;

    /// Graceful shutdown of all agents
    pub async fn shutdown(&self) -> Result<()>;

    /// Collect metrics for all agents
    pub fn collect_metrics(&self) -> Vec<MetricsSnapshot>;
}
```

#### `agent_state.rs`

```rust
pub struct AgentState {
    pub definition: AgentDefinition,
    pub rig_agent: rig::agent::Agent,          // configured rig agent
    pub mcp_clients: Vec<McpConnection>,
    pub tool_registry: ToolRegistry,
    pub conversations: DashMap<ConversationId, ConversationHandle>,
    pub cancel: CancellationToken,             // child of global cancel
    pub tracker: TaskTracker,
    pub metrics: Arc<AgentMetrics>,
    pub rate_limiter: Option<Arc<governor::DefaultDirectRateLimiter>>,
}

pub struct ConversationHandle {
    pub id: ConversationId,
    pub message_tx: mpsc::Sender<Message>,     // send messages into conversation
    pub created_at: chrono::DateTime<chrono::Utc>,
}
```

#### `conversation.rs`

The conversation engine runs as a spawned tokio task per conversation:

```rust
pub async fn run_conversation(
    agent_state: Arc<AgentState>,
    conversation_id: ConversationId,
    mut message_rx: mpsc::Receiver<Message>,
    sse_tx: mpsc::Sender<SseEvent>,
    webhook_dispatcher: Arc<WebhookDispatcher>,
    cancel: CancellationToken,
) {
    let mut history: Vec<Message> = vec![];

    loop {
        tokio::select! {
            _ = cancel.cancelled() => break,
            msg = message_rx.recv() => {
                let Some(msg) = msg else { break };
                history.push(msg.clone());

                // Fire webhook: MessageReceived
                webhook_dispatcher.dispatch(WebhookEvent::MessageReceived { ... });

                // Run the agent loop (rig handles tool calling internally)
                let response = agent_state.rig_agent
                    .stream_chat(&history)  // or .chat() for non-streaming
                    .await;

                match response {
                    Ok(stream) => {
                        // Stream chunks through SSE
                        // Track tokens from response metadata
                        // Fire webhooks for each step
                    }
                    Err(e) => {
                        // Send error event via SSE
                        // Fire webhook: AgentError
                    }
                }

                // Update metrics
                agent_state.metrics.total_requests.fetch_add(1, Ordering::Relaxed);
            }
        }
    }
}
```

#### `drain.rs`

Graceful drain for agent updates:

```rust
pub async fn drain_and_replace(
    agent_map: &AgentMap,
    agent_id: &AgentId,
    new_state: AgentState,
    timeout: Duration,
) -> Result<()> {
    let old_state = agent_map.get(agent_id)?;

    // 1. Signal old agent to stop accepting new conversations
    old_state.cancel.cancel();

    // 2. Close tracker (no new tasks) and wait for in-flight to finish
    old_state.tracker.close();
    tokio::select! {
        _ = old_state.tracker.wait() => {
            log::info!("Agent {} drained successfully", agent_id);
        }
        _ = tokio::time::sleep(timeout) => {
            log::warn!("Agent {} drain timed out after {:?}", agent_id, timeout);
        }
    }

    // 3. Disconnect old MCP connections
    for client in &old_state.mcp_clients {
        client.disconnect().await.ok();
    }

    // 4. Swap in new state
    agent_map.insert(agent_id.clone(), Arc::new(new_state));
}
```

---

### 6.6 bridge-api

Axum HTTP server exposing the REST API.

#### Routes

| Method | Path | Handler | Description |
|--------|------|---------|-------------|
| `GET` | `/health` | `health::check` | Liveness + readiness |
| `GET` | `/agents` | `agents::list` | List all loaded agents |
| `GET` | `/agents/:agent_id` | `agents::get` | Get agent details |
| `POST` | `/agents/:agent_id/conversations` | `conversations::create` | Start new conversation, returns conversation_id + SSE stream URL |
| `POST` | `/conversations/:conv_id/messages` | `conversations::send_message` | Send user message to conversation |
| `GET` | `/conversations/:conv_id/stream` | `stream::sse` | SSE stream for conversation events |
| `DELETE` | `/conversations/:conv_id` | `conversations::end` | End a conversation |
| `GET` | `/metrics` | `metrics::get` | JSON metrics for all agents |

#### SSE Event Types

```
event: message_start
data: {"conversation_id": "...", "message_id": "..."}

event: content_delta
data: {"delta": "Hello, ", "message_id": "..."}

event: tool_call_start
data: {"tool_call_id": "...", "tool_name": "grep", "arguments": {...}}

event: tool_call_result
data: {"tool_call_id": "...", "result": "...", "is_error": false}

event: message_end
data: {"message_id": "...", "usage": {"input_tokens": 150, "output_tokens": 42}}

event: error
data: {"code": "rate_limited", "message": "Too many requests"}

event: done
data: {}
```

#### `metrics.rs` handler

Returns JSON consumed by the control plane:

```json
{
  "timestamp": "2026-03-02T13:00:00Z",
  "agents": [
    {
      "agent_id": "agent_1",
      "agent_name": "Support Bot",
      "input_tokens": 125000,
      "output_tokens": 42000,
      "total_tokens": 167000,
      "total_requests": 350,
      "failed_requests": 2,
      "active_conversations": 3,
      "total_conversations": 120,
      "tool_calls": 89,
      "avg_latency_ms": 1250.5
    }
  ],
  "global": {
    "total_agents": 5,
    "total_active_conversations": 12,
    "uptime_secs": 86400
  }
}
```

---

### 6.7 bridge-sync

Polls the control plane for agent definition changes.

#### `poller.rs`

```rust
pub async fn run_sync_loop(
    supervisor: Arc<AgentSupervisor>,
    config: Arc<RuntimeConfig>,
    cancel: CancellationToken,
) {
    let client = reqwest::Client::new();
    let mut interval = tokio::time::interval(
        Duration::from_secs(config.sync_interval_secs)
    );

    loop {
        tokio::select! {
            _ = cancel.cancelled() => break,
            _ = interval.tick() => {
                match fetch_agents(&client, &config).await {
                    Ok(definitions) => {
                        let diff = compute_diff(
                            &supervisor.list_agents(),
                            &definitions,
                        );
                        if !diff.is_empty() {
                            supervisor.apply_diff(diff).await.ok();
                        }
                    }
                    Err(e) => {
                        log::error!("Control plane sync failed: {}", e);
                    }
                }
            }
        }
    }
}
```

#### `diff.rs`

Compares current loaded agents against freshly fetched definitions:

```rust
pub struct AgentDiff {
    pub added: Vec<AgentDefinition>,
    pub updated: Vec<AgentDefinition>,   // definition changed
    pub removed: Vec<AgentId>,
}

pub fn compute_diff(
    current: &[AgentSummary],
    fetched: &[AgentDefinition],
) -> AgentDiff;
```

Diff detection: compare a hash/version field on each agent definition. The control plane should include a `version` or `updated_at` field that changes on any modification.

---

### 6.8 bridge-webhooks

Lightweight webhook delivery without svix.

#### `signer.rs`

```rust
use hmac::{Hmac, Mac};
use sha2::Sha256;

pub fn sign_webhook(
    payload: &[u8],
    secret: &str,
    timestamp: i64,
) -> String {
    let message = format!("{}.{}", timestamp, std::str::from_utf8(payload).unwrap());
    let mut mac = Hmac::<Sha256>::new_from_slice(secret.as_bytes()).unwrap();
    mac.update(message.as_bytes());
    let result = mac.finalize();
    base64::engine::general_purpose::STANDARD.encode(result.into_bytes())
}
```

#### `dispatcher.rs`

```rust
pub struct WebhookDispatcher {
    client: reqwest::Client,
    event_tx: mpsc::Sender<WebhookPayload>,
}

impl WebhookDispatcher {
    /// Fire-and-forget: sends event to background delivery task
    pub fn dispatch(&self, payload: WebhookPayload) {
        let _ = self.event_tx.try_send(payload);
    }
}

/// Background task that delivers webhooks with retries
async fn delivery_loop(
    client: reqwest::Client,
    mut rx: mpsc::Receiver<WebhookPayload>,
) {
    while let Some(payload) = rx.recv().await {
        let url = payload.webhook_url.clone();
        let secret = payload.webhook_secret.clone();
        let body = serde_json::to_vec(&payload).unwrap();

        // Fire-and-forget with retry
        tokio::spawn(async move {
            let result = (|| async {
                let timestamp = chrono::Utc::now().timestamp();
                let signature = sign_webhook(&body, &secret, timestamp);
                client.post(&url)
                    .header("Content-Type", "application/json")
                    .header("X-Webhook-Signature", &signature)
                    .header("X-Webhook-Timestamp", timestamp.to_string())
                    .body(body.clone())
                    .timeout(Duration::from_secs(10))
                    .send().await?
                    .error_for_status()
            })
            .retry(backon::ExponentialBuilder::default()
                .with_max_times(5)
                .with_jitter())
            .await;

            if let Err(e) = result {
                log::error!("Webhook delivery failed after retries: {}", e);
            }
        });
    }
}
```

---

### 6.9 bridge-bin

#### `main.rs`

```rust
#[tokio::main]
async fn main() -> anyhow::Result<()> {
    // 1. Load config
    let config = load_config()?;

    // 2. Initialize logging
    init_logging(&config);

    // 3. Create global cancellation token + task tracker
    let cancel = CancellationToken::new();
    let tracker = TaskTracker::new();

    // 4. Create shared components
    let mcp_manager = Arc::new(McpManager::new());
    let webhook_dispatcher = Arc::new(WebhookDispatcher::new());
    let supervisor = Arc::new(AgentSupervisor::new(
        mcp_manager, webhook_dispatcher.clone(), config.clone(), cancel.clone(),
    ));

    // 5. Fetch initial agents from control plane
    let agents = fetch_initial_agents(&config).await?;
    supervisor.load_agents(agents).await?;
    log::info!("Loaded {} agents", supervisor.list_agents().len());

    // 6. Start background tasks
    // Config sync poller
    tracker.spawn(run_sync_loop(supervisor.clone(), config.clone(), cancel.clone()));

    // Webhook delivery loop
    tracker.spawn(webhook_dispatcher.run(cancel.clone()));

    // 7. Start HTTP server
    let app = build_router(supervisor.clone());
    let listener = tokio::net::TcpListener::bind(&config.listen_addr).await?;
    log::info!("Listening on {}", config.listen_addr);

    // 8. Serve with graceful shutdown
    axum::serve(listener, app)
        .with_graceful_shutdown(shutdown_signal(cancel.clone()))
        .await?;

    // 9. Shutdown sequence
    log::info!("Shutting down...");
    cancel.cancel();
    tracker.close();
    tracker.wait().await;
    supervisor.shutdown().await?;
    log::info!("Shutdown complete");

    Ok(())
}

async fn shutdown_signal(cancel: CancellationToken) {
    let ctrl_c = tokio::signal::ctrl_c();
    let mut sigterm = tokio::signal::unix::signal(
        tokio::signal::unix::SignalKind::terminate()
    ).unwrap();

    tokio::select! {
        _ = ctrl_c => {},
        _ = sigterm.recv() => {},
        _ = cancel.cancelled() => {},
    }
}
```

---

## 7. Test Suite 1 — Unit & Integration Tests (100% Coverage)

Every crate contains thorough unit and integration tests. Target: **100% line coverage** measured via `cargo-llvm-cov`.

### Per-Crate Test Plan

#### bridge-core tests
- Serialization/deserialization roundtrips for all types (serde)
- AgentDefinition validation (required fields, valid provider types)
- MetricsSnapshot construction from atomic counters
- WebhookPayload serialization matches expected JSON shape
- Config parsing from env vars and TOML

#### bridge-llm tests
- `factory::build_agent` creates correct provider client per ProviderType
- Tool adapter correctly implements rig's Tool trait
- Streaming adapter emits correct SSE events from mock rig stream
- Error handling: invalid API key, unknown model, unsupported provider
- JSON schema output validation when configured
- **Mock strategy**: Mock the rig provider client using a trait-based abstraction. Create `MockLlmProvider` that returns canned responses.

#### bridge-mcp tests
- McpManager connect/disconnect lifecycle for stdio transport
- McpManager connect/disconnect lifecycle for Streamable HTTP transport
- Tool discovery returns correct tool schemas
- Tool execution forwards arguments and returns results
- Connection error handling and reconnection
- Concurrent tool calls on same connection
- **Mock strategy**: Create a test MCP server binary (tiny, in-process) that responds with canned tools and results.

#### bridge-tools tests

**ReadTool:**
- Reads file with correct line numbers (cat -n format)
- Offset/limit pagination returns correct line range
- `total_lines` and `truncated` are accurate
- Lines > 2000 chars are truncated with `...`
- Binary file detection returns error message
- File not found → clear error
- Permission denied → clear error
- Directory path → clear error ("is a directory")

**GlobTool:**
- Pattern `*.rs` matches only Rust files in current dir
- Pattern `src/**/*.ts` matches nested TypeScript files
- Respects `.gitignore` — ignored files excluded
- Results sorted by modification time (newest first)
- Returns max 1000 results, sets `truncated: true` if more
- Brace patterns `{a,b}` work correctly
- Empty result set returns empty array, not error
- Non-existent directory → clear error

**LsTool:**
- Lists directory with correct entry types (file, directory, symlink)
- Files have size, directories do not
- Sorted: directories first, then files, alphabetically
- Modified time is valid ISO 8601
- Non-directory path → clear error
- Empty directory → empty entries array

**GrepTool:**
- Regex pattern finds correct matches
- `FilesWithMatches` mode returns only file paths
- `Content` mode returns line numbers and matching text
- `Count` mode returns per-file match counts
- Context lines (`-B`, `-A`, `-C`) include correct surrounding lines
- Case-insensitive search works
- Glob filter restricts to matching files
- File type filter (e.g., "rust") restricts correctly
- Respects `.gitignore`
- Max results cap works, sets `truncated: true`
- No matches → empty result, not error
- Invalid regex → clear error message
- Single file search (not directory) works

**WebSearchTool:**
- Correct API request format per provider (mock HTTP)
- Returns structured results (title, url, snippet)
- Retry on transient failures

**WebFetchTool:**
- dom_smoothie extraction on sample HTML pages
- htmd fallback when readability fails
- Handles malformed HTML, empty pages, redirects

**ToolRegistry:**
- Lookup by name, merge built-in + MCP tools, no duplicates

**SkillTools:**
- Fuzzy matching on skill definitions

**Mock strategy**: Use `wiremock` or `httpmock` for HTTP mocking. Use fixture HTML files. Use `tempfile` for filesystem tool tests with known directory structures.

#### bridge-runtime tests
- AgentSupervisor: load agents, get agent, list agents
- ConversationEngine: full agent loop with mock LLM (message → response)
- ConversationEngine: tool call → execution → feed back → continue
- ConversationEngine: multi-turn conversation with history
- ConversationEngine: respects max_turns limit
- ConversationEngine: concurrent conversations on same agent are isolated
- Drain: in-flight conversations complete before agent swap
- Drain: new conversations rejected during drain
- Drain: timeout triggers forced shutdown
- Rate limiting: requests exceeding limit are rejected
- Token tracking: correct counts after conversation
- AgentMap: concurrent read/write safety
- **Mock strategy**: Mock LLM provider + mock tools. Use `tokio::time::pause()` for deterministic timing.

#### bridge-api tests
- Each handler: correct HTTP status codes, response shapes
- SSE stream: events arrive in correct order
- SSE stream: stream closes on conversation end
- Metrics endpoint: returns correct JSON after mock conversations
- Health endpoint: returns ok when healthy, degraded when appropriate
- Error responses: 404 for unknown agent/conversation, 400 for bad input, 429 for rate limit
- CORS headers present
- Request ID propagation
- **Mock strategy**: Use `axum::test_helpers` or `tower::ServiceExt` to test handlers without network.

#### bridge-sync tests
- Poller fetches at correct interval
- Diff computation: detect added, updated, removed agents
- Updater: calls supervisor.apply_diff with correct changes
- Error handling: control plane unreachable, invalid response
- **Mock strategy**: Mock HTTP server returning agent manifests.

#### bridge-webhooks tests
- HMAC-SHA256 signing produces correct signatures
- Signature verification roundtrip
- Dispatcher delivers to correct URL with correct headers
- Retry logic: retries on 5xx, does not retry on 4xx
- Delivery timeout handling
- Event serialization matches expected format
- **Mock strategy**: Mock HTTP server that records received webhooks.

### Test Infrastructure

```toml
# workspace dev-dependencies
[workspace.dev-dependencies]
tokio-test = "0.4"
wiremock = "0.6"
assert_json_diff = "2"
tempfile = "3"
pretty_assertions = "1"
```

### Coverage Command

```bash
cargo llvm-cov --workspace --html --output-dir coverage/
# Open coverage/index.html to inspect per-file coverage
```

---

## 8. Test Suite 2 — End-to-End Tests

A completely separate test infrastructure that builds the real binary, starts it, and tests every endpoint.

### 8.1 Mock Control Plane

A standalone axum server (`e2e/mock-control-plane/`) that implements the full control plane API:

#### Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/agents` | Return full agent manifest |
| `GET` | `/agents/:id` | Return single agent definition |
| `POST` | `/agents` | Create a new agent (for hot-reload tests) |
| `PUT` | `/agents/:id` | Update agent definition |
| `DELETE` | `/agents/:id` | Delete agent |
| `GET` | `/agents/:id/skills/:skill_id` | Return full skill definition |
| `POST` | `/webhooks/receive` | Receive and log webhooks from bridge |
| `GET` | `/webhooks/log` | Return all received webhooks (for test assertions) |
| `DELETE` | `/webhooks/log` | Clear webhook log |

#### Internal State

```rust
pub struct MockControlPlane {
    agents: RwLock<HashMap<AgentId, AgentDefinition>>,
    webhook_log: RwLock<Vec<ReceivedWebhook>>,
    version_counter: AtomicU64,
}
```

The mock control plane is seeded with fixture agent definitions on startup. Tests can mutate the agent store and verify the bridge picks up changes.

#### Mock LLM Provider

The mock control plane also runs a fake LLM API endpoint that:
- Accepts OpenAI-compatible `/v1/chat/completions` requests
- Returns canned responses based on configurable rules
- Supports streaming (SSE chunks) and non-streaming
- Can be configured to return tool calls
- Tracks received requests for assertion

Agent definitions in E2E tests point their `api_key` and `base_url` at this mock LLM endpoint.

#### Mock MCP Server

A minimal MCP server (stdio transport) that:
- Exposes 2-3 test tools (echo, add_numbers, get_time)
- Returns predictable results for assertions
- Built as a small binary in `e2e/mock-mcp-server/`

### 8.2 E2E Test Harness

```rust
// e2e/tests/harness.rs

pub struct TestHarness {
    pub control_plane_url: String,
    pub bridge_url: String,
    control_plane_process: Child,
    bridge_process: Child,
    client: reqwest::Client,
}

impl TestHarness {
    pub async fn start() -> Self {
        // 1. Build bridge binary (cargo build --release)
        // 2. Start mock control plane on random port
        // 3. Start bridge binary pointing at mock control plane
        // 4. Wait for bridge /health to return 200
        // 5. Return harness with URLs and HTTP client
    }

    pub async fn stop(&mut self) {
        // Send SIGTERM to bridge, wait for graceful shutdown
        // Stop mock control plane
    }

    // Helper methods for tests:
    pub async fn create_conversation(&self, agent_id: &str) -> ConversationId;
    pub async fn send_message(&self, conv_id: &str, content: &str) -> Response;
    pub async fn stream_response(&self, conv_id: &str) -> Vec<SseEvent>;
    pub async fn get_metrics(&self) -> MetricsResponse;
    pub async fn get_webhook_log(&self) -> Vec<ReceivedWebhook>;
    pub async fn clear_webhook_log(&self);
    pub async fn add_agent_to_cp(&self, def: AgentDefinition);
    pub async fn update_agent_in_cp(&self, def: AgentDefinition);
    pub async fn delete_agent_from_cp(&self, agent_id: &str);
    pub async fn wait_for_sync(&self);  // wait for next sync cycle
}
```

### 8.3 E2E Test Cases

#### `test_agent_loading.rs`
- Bridge starts and fetches all agents from control plane
- `GET /agents` returns all loaded agents with correct details
- `GET /agents/:id` returns correct agent definition
- Each agent's MCP connections are established (verified by calling MCP tools)

#### `test_conversations.rs`
- Create conversation → returns conversation_id
- Send message → receive assistant response
- Multi-turn conversation maintains history
- End conversation → conversation cleaned up
- Create conversation on unknown agent → 404
- Send message to unknown conversation → 404
- Send message to ended conversation → 400

#### `test_streaming.rs`
- SSE stream connects successfully
- `message_start` event arrives first
- `content_delta` events contain response chunks
- `message_end` event contains token usage
- `done` event signals completion
- Stream closes after conversation ends
- Multiple concurrent SSE streams on same conversation work

#### `test_webhooks.rs`
- `ConversationCreated` webhook fires on new conversation
- `MessageReceived` webhook fires on user message
- `ResponseStarted` webhook fires when assistant begins
- `ResponseCompleted` webhook fires with full response
- `ToolCallStarted` + `ToolCallCompleted` webhooks fire for tool use
- `ConversationEnded` webhook fires on delete
- Webhook payloads have valid HMAC signatures
- Webhook signature verification passes with correct secret

#### `test_hot_reload.rs`
- Add new agent via control plane → bridge picks it up after sync interval
- New agent is accessible via `GET /agents` and can serve conversations
- Update agent system prompt → bridge drains and reloads
- Conversations created after update use new prompt
- Delete agent via control plane → bridge removes it after sync
- Deleted agent returns 404 on `GET /agents/:id`

#### `test_drain.rs`
- Start a long conversation (mock LLM returns slow streaming response)
- Trigger agent update via control plane
- Verify in-flight conversation completes normally
- Verify new conversations use updated agent config
- Verify drain timeout is respected

#### `test_concurrent.rs`
- 10 concurrent conversations on the same agent — all isolated
- 5 conversations across 3 different agents — all isolated
- Rapid create/send/end cycles — no race conditions
- Verify token metrics are accurate after concurrent usage

#### `test_mcp.rs`
- Agent with MCP tools can discover tools (listed in agent details)
- Conversation triggers MCP tool call → correct result returned
- MCP tool failure is handled gracefully (error in response, not crash)

#### `test_tools.rs`
- ReadTool: agent reads a known file, response contains correct content
- GlobTool: agent finds files matching pattern in test directory
- LsTool: agent lists directory contents, sees expected entries
- GrepTool: agent searches for pattern, finds correct matches with line numbers
- Web search tool returns structured results (mock search API)
- Web fetch tool extracts article content (mock HTTP returning HTML fixture)
- All filesystem tools are strictly readonly — no writes or mutations occur

#### `test_metrics.rs`
- `GET /metrics` returns empty counters on fresh start
- After 3 conversations with known token counts:
  - input_tokens matches expected sum
  - output_tokens matches expected sum
  - total_requests = 3
  - total_conversations = 3
  - active_conversations = 0 (after ending all)
  - avg_latency_ms is reasonable

#### `test_errors.rs`
- Invalid JSON body → 400 with error message
- Missing required fields → 400 with field name
- LLM API error (mock returns 500) → error event on SSE stream
- Rate limited request → 429 with retry-after header
- Control plane unreachable → bridge continues serving with cached agents

#### `test_skills.rs`
- Agent with skills can list matching skills via tool
- Skill activation fetches full skill from control plane
- Activated skill's tools are available in subsequent turns

### 8.4 E2E Test Runner

```bash
# Run full E2E suite
cargo test --package bridge-e2e --test '*'

# Run specific E2E test
cargo test --package bridge-e2e --test test_conversations
```

E2E tests use `#[tokio::test]` with a shared harness that starts/stops the binary once per test file (using `once_cell::sync::Lazy` or similar).

---

## 9. Logging Strategy

Since we do not need distributed tracing or OpenTelemetry, we use `tracing` + `tracing-subscriber` purely as a structured logging library.

### Log Levels
- **ERROR**: Unrecoverable failures (control plane unreachable after all retries, MCP connection permanently failed)
- **WARN**: Recoverable issues (webhook delivery failed, single LLM call retry, drain timeout)
- **INFO**: Lifecycle events (agent loaded, conversation started/ended, config sync completed, binary startup/shutdown)
- **DEBUG**: Request/response details (LLM API calls, tool executions, MCP messages)

### Configuration
```bash
# Environment variable controls log level and per-module filtering
RUST_LOG=info                          # default
RUST_LOG=debug                         # verbose
RUST_LOG=bridge_runtime=debug,info     # verbose for runtime only
```

### Format
- **Development**: human-readable with colors (`tracing_subscriber::fmt`)
- **Production**: JSON lines for log aggregation (`tracing_subscriber::fmt::json()`)
- Controlled via `RuntimeConfig.log_format`

---

## 10. Implementation Phases

### Phase 1: Foundation (bridge-core + bridge-bin skeleton)
- All shared types, traits, errors
- RuntimeConfig loading via figment
- Logging initialization
- Binary skeleton with health endpoint
- **Deliverable**: binary starts, loads config, responds to `/health`

### Phase 2: LLM Integration (bridge-llm)
- Rig-core provider factory
- Tool adapter (ToolDefinition → rig Tool)
- Streaming response adapter
- Unit tests with mock provider
- **Deliverable**: can programmatically create a rig agent and get a response

### Phase 3: MCP Client (bridge-mcp)
- McpManager with rmcp
- Stdio + Streamable HTTP connection lifecycle
- Tool discovery and bridging to rig
- Unit tests with mock MCP server
- **Deliverable**: can connect to MCP servers and call tools

### Phase 4: Built-in Tools (bridge-tools)
- ToolRegistry
- Filesystem tools: Read, Glob, LS, Grep (ripgrep-powered, all readonly)
- Web tools: web_search, web_fetch (dom_smoothie + htmd)
- Skill fetch/activate tools
- Unit tests with tempdir fixtures, mock HTTP, fixture HTML
- **Deliverable**: all built-in tools execute correctly in isolation

### Phase 5: Runtime Engine (bridge-runtime)
- AgentSupervisor + AgentMap
- ConversationEngine (full agent loop)
- Graceful drain logic
- Token tracking
- Rate limiting
- Integration tests: full conversation with mock LLM
- **Deliverable**: can load agents, run conversations, track metrics

### Phase 6: HTTP API (bridge-api)
- Axum router with all handlers
- SSE streaming endpoint
- Metrics endpoint
- Error handling middleware
- Handler tests
- **Deliverable**: full REST API operational

### Phase 7: Control Plane Sync (bridge-sync)
- Poller with configurable interval
- Diff computation
- Agent add/update/delete with drain
- Tests with mock control plane responses
- **Deliverable**: agents hot-reload from control plane changes

### Phase 8: Webhooks (bridge-webhooks)
- HMAC-SHA256 signing
- Async dispatcher with backon retry
- Integration with conversation lifecycle events
- Tests with mock webhook receiver
- **Deliverable**: webhooks fire for all event types with correct signatures

### Phase 9: Wire Everything (bridge-bin)
- Complete startup sequence
- Signal handling (SIGTERM/SIGINT)
- Graceful shutdown orchestration
- Full integration test: start binary, run conversations, verify metrics
- **Deliverable**: complete working binary

### Phase 10: E2E Test Suite
- Mock control plane server
- Mock LLM API server
- Mock MCP server
- Test harness (start/stop binary)
- All E2E test cases (13 test files)
- **Deliverable**: full E2E coverage, CI-ready

---

## 11. Binary & Memory Budget

### Release Profile

```toml
[profile.release]
strip = true         # strip debug symbols + DWARF
lto = true           # link-time optimization
opt-level = "z"      # optimize for size
codegen-units = 1    # single codegen unit for max optimization
panic = "abort"      # no unwinding code
```

### Expected Measurements

| Metric | Budget | Expected | Notes |
|--------|--------|----------|-------|
| Binary size | < 25 MB | 4-6 MB | Measured with `ls -lh target/release/bridge` |
| Memory at rest (0 agents) | < 25 MB | ~2 MB | tokio + axum listener |
| Memory at rest (10 agents, HTTP MCP) | < 25 MB | ~3-5 MB | agent configs + connection state |
| Memory at rest (10 agents, 3 stdio MCP) | < 25 MB | ~3-5 MB ours | child processes have separate RSS |
| Memory per active conversation | — | ~50-200 KB | message history + SSE buffer |

### Verification Commands

```bash
# Binary size
cargo build --release && ls -lh target/release/bridge

# Memory at rest (RSS)
./target/release/bridge &
sleep 5
ps -o rss= -p $(pgrep bridge)  # in KB

# Binary bloat analysis
cargo install cargo-bloat
cargo bloat --release -n 20     # top 20 contributors
```
