# Architecture Deep Dive

Internal architecture of Bridge.

---

## Crate Dependencies

```
bridge (binary)
├── api
│   ├── bridge_core
│   ├── runtime
│   ├── llm
│   └── webhooks
├── runtime
│   ├── bridge_core
│   ├── llm
│   ├── mcp
│   ├── tools
│   ├── lsp
│   └── webhooks
├── webhooks
│   └── bridge_core
├── mcp
│   ├── bridge_core
│   └── tools
└── lsp

llm
├── bridge_core
├── tools
└── webhooks

tools
├── bridge_core
└── lsp

mcp
├── bridge_core
└── tools

webhooks
└── bridge_core

bridge_core
└── (no internal deps)
```

---

## API Layer (`api`)

HTTP request handling.

### Responsibilities

- Route requests via Axum
- Validate input
- Authenticate push endpoints (bearer token)
- Stream SSE events
- Manage SSE stream registry in `AppState`

### Key Files

| File | Purpose |
|------|---------|
| `router.rs` | Route definitions with authentication layers |
| `sse.rs` | Server-Sent Events streaming utilities |
| `middleware.rs` | Bearer token authentication |
| `state.rs` | `AppState` with supervisor, SSE streams, permission manager |
| `handlers/health.rs` | Health check endpoint |
| `handlers/agents.rs` | List/get agents |
| `handlers/conversations.rs` | Create conversations, send messages, abort |
| `handlers/stream.rs` | SSE stream endpoint |
| `handlers/metrics.rs` | Metrics snapshot endpoint |
| `handlers/permissions.rs` | Tool approval management |
| `handlers/push.rs` | Control plane push endpoints |

---

## Runtime Layer (`runtime`)

Agent and conversation management.

### Components

| Module | Purpose |
|--------|---------|
| `supervisor.rs` | Central `AgentSupervisor` for agent lifecycle |
| `agent_map.rs` | `AgentMap` — concurrent DashMap of agents |
| `agent_runner.rs` | Per-agent event loop, subagent support, `AgentSessionStore` |
| `agent_state.rs` | `AgentState` — complete state for one agent |
| `conversation.rs` | `ConversationParams`, `run_conversation()` event loop |
| `compaction.rs` | History summarization when token limits reached |
| `drain.rs` | Graceful agent drain for zero-downtime updates |
| `system_reminder.rs` | Periodic system reminder injection |
| `token_tracker.rs` | Token usage tracking per agent |
| `permission_manager.rs` | Runtime permission manager integration |

### Agent State Machine

```
┌─────────┐    message     ┌────────────┐
│  Idle   │ ─────────────► │ Processing │
└─────────┘                └─────┬──────┘
     ▲                           │
     │         complete          │ tool calls
     └───────────────────────────┤
                                 ▼
                          ┌────────────┐
                          │ ToolCalls  │
                          └─────┬──────┘
                                │
                                │ execute
                                ▼
                          ┌────────────┐
                          │ Processing │ (loop)
                          └────────────┘
```

### Conversation Flow

1. `supervisor.create_conversation()` creates `ConversationHandle`
2. `conversation.rs` spawns async task running `run_conversation()`
3. Message loop waits on `message_rx` channel
4. On message: `process_turn()` executes LLM interaction
5. Tool calls execute within `AGENT_CONTEXT.scope()`
6. Results stream back via `SseEvent` to client

---

## LLM Layer (`llm`)

Provider integrations.

### Structure

```
llm/
├── providers.rs      # Provider dispatch, `BridgeAgent`, `PromptResponse`
├── factory.rs        # `build_agent()` — creates rig-core agents
├── streaming.rs      # `SseEvent`, `TokenUsage` — SSE streaming types
├── tool_adapter.rs   # `adapt_tools()`, `DynamicTool` — tool bridging
└── tool_hook.rs      # `ToolCallEmitter` — intercepts tool calls
```

### Provider Support

Providers are implemented via `rig-core`:
- Anthropic (Claude)
- OpenAI-compatible (GPT-4, etc.)

### Tool Hook System

`tool_hook.rs` provides `ToolCallEmitter` which:
- Intercepts all tool calls before execution
- Routes to permission manager if approval required
- Executes agent/parallel_agent calls in-place (preserves task-local context)
- Handles `AGENT_CONTEXT` extraction for subagent spawns

### Adding a Provider

1. Implement `LLMProvider` trait from `rig-core`
2. Add to `factory.rs`
3. Update documentation

---

## Tools Layer (`tools`)

Built-in tool implementations.

### Tool Registration

Tools register explicitly via `ToolRegistry`:

```rust
let mut registry = ToolRegistry::new();
registry.register(Arc::new(MyTool::new()));
```

Registration happens in `builtin.rs`:
- `register_builtin_tools()` — full tool set
- `register_builtin_tools_for_subagent()` — excludes agent tool (prevents recursion)
- `register_filtered_builtin_tools()` — only specified tools

### Tool Trait

```rust
#[async_trait]
pub trait ToolExecutor: Send + Sync {
    fn name(&self) -> &str;
    fn description(&self) -> &str;
    fn parameters_schema(&self) -> serde_json::Value;
    async fn execute(&self, args: serde_json::Value) -> Result<String, String>;
}
```

### Built-in Tools

| Category | Tools |
|----------|-------|
| Filesystem | `read`, `write`, `edit`, `apply_patch`, `multiedit`, `ls`, `Glob`, `Grep` |
| Shell | `bash` |
| Web | `web_fetch`, `web_search` |
| Agents | `agent`, `parallel_agent` |
| Tasks | `todowrite`, `todoread`, `join` |
| Batch | `batch` |
| LSP | `lsp` |

### Task-Local Context

The `AGENT_CONTEXT` task-local provides:

```rust
tokio::task_local! {
    pub static AGENT_CONTEXT: AgentContext;
}

pub struct AgentContext {
    pub conversation_id: String,
    pub agent_id: String,
    pub subagent_runner: Arc<dyn SubAgentRunner>,
    pub task_registry: Arc<TaskRegistry>,
    pub stream_tx: mpsc::Sender<AgentTaskNotification>,
}
```

Used by:
- `agent.rs` — spawns subagents
- `parallel_agent.rs` — spawns parallel subagents
- `bash.rs` — streams command output notifications

---

## MCP Layer (`mcp`)

Model Context Protocol client.

### Structure

| Module | Purpose |
|--------|---------|
| `connection.rs` | `McpConnection`, `McpToolInfo` — per-server connection |
| `manager.rs` | `McpManager` — shared across agents |
| `tool_bridge.rs` | `McpToolExecutor`, `bridge_mcp_tools()` — tool adapter |

### Transports

- `stdio` — Local command (via `rmcp`)
- `http` — Remote server (Streamable HTTP)

### Connection Lifecycle

```
Connect → Initialize → Tool Discovery → Ready → Calls → Shutdown
```

Managed by `McpManager::connect_agent()` which:
1. Connects to each configured MCP server
2. Discovers available tools
3. Bridges tools via `McpToolExecutor`
4. Returns tool list for agent registration

---

## Webhooks Layer (`webhooks`)

Webhook dispatch with HMAC signing.

### Structure

| Module | Purpose |
|--------|---------|
| `event_bus.rs` | `EventBus` — single entry point for all events, stamps global `sequence_number`, fans out to DB/WS/SSE/HTTP |
| `delivery.rs` | Webhook HTTP delivery — per-conversation batching with exponential backoff retry |
| `signer.rs` | `sign_webhook()`, `verify_webhook()` — HMAC-SHA256 |

### Delivery

- All events use the unified `BridgeEvent` type (defined in `core::event`)
- `EventBus.emit()` serialises sequence assignment under mutex for ordering guarantee
- Fans out to 4 channels simultaneously: DB persistence, WebSocket broadcast, per-conversation SSE, webhook HTTP delivery
- Webhook HTTP delivery: per-conversation workers, batched JSON array POST, exponential backoff via `backon`
- HMAC-SHA256 signature in `X-Webhook-Signature` header

---

## LSP Layer (`lsp`)

Language Server Protocol integration.

### Structure

| Module | Purpose |
|--------|---------|
| `config.rs` | `LspServerConfig` — server configuration |
| `error.rs` | `LspError` — error types |
| `language.rs` | Language detection |
| `manager.rs` | `LspManager` — manages multiple LSP servers |
| `server.rs` | `ServerDef` — single server connection |

### Usage

- `LspManager` created in `main.rs`
- Passed to `register_builtin_tools_with_lsp()`
- `LspTool` provides code intelligence to agents
- `edit.rs`, `write.rs`, `multiedit.rs` trigger diagnostics refresh

---

## Core Layer (`bridge_core`)

Domain models and shared types.

### Modules

| Module | Types |
|--------|-------|
| `agent.rs` | `AgentDefinition`, `AgentConfig`, `AgentId`, `AgentSummary` |
| `config.rs` | `RuntimeConfig`, `LspConfig`, `LogFormat` |
| `conversation.rs` | `Message`, `Role`, `ContentBlock`, `ToolCall`, `ToolResult` |
| `error.rs` | `BridgeError`, `Result` |
| `integration.rs` | `IntegrationDefinition`, `IntegrationAction` |
| `mcp.rs` | `McpServerDefinition`, `McpTransport` |
| `metrics.rs` | `AgentMetrics`, `GlobalMetrics`, `MetricsResponse` |
| `permission.rs` | `ApprovalRequest`, `ApprovalDecision`, `ToolPermission` |
| `provider.rs` | `ProviderConfig`, `ProviderType` |
| `skill.rs` | `SkillDefinition`, `SkillId` |
| `tool.rs` | `ToolDefinition` |
| `webhook.rs` | `WebhookEventType`, `WebhookPayload` |

Note: Package is named `bridge_core` because `core` conflicts with Rust's std::core.

---

## Data Flow

```
HTTP Request
    ↓
api::router
    ↓
api::handlers::conversations::send_message
    ↓
runtime::supervisor::AgentSupervisor::send_message
    ↓
runtime::conversation::run_conversation (async task)
    ↓
llm::providers::BridgeAgent::prompt_stream
    ↓
External API (Anthropic/OpenAI)
    ↓
Stream chunks back
    ↓
llm::streaming::SseEvent
    ↓
api::sse
    ↓
Client
```

### Tool Call Flow

```
LLM requests tool
    ↓
llm::tool_hook::ToolCallEmitter
    ↓
Permission check (if required)
    ↓
tools::[tool]::execute (within AGENT_CONTEXT.scope)
    ↓
Result → LLM (continues conversation)
```

---

## Testing Strategy

| Test Type | Location | Purpose |
|-----------|----------|---------|
| Unit | Each crate | Individual functions |
| Integration | `api/tests.rs` | API endpoints |
| E2E | `e2e/` | Full workflows |

### Running Tests

```bash
# All tests
cargo test

# Specific crate
cargo test -p runtime

# E2E tests
cargo test -p bridge-e2e
```

---

## Key Design Decisions

### In-Memory State

Bridge keeps state in memory for speed. No database means:
- Fast access (no network round-trips)
- Simple operations (Rust data structures)
- Ephemeral (data lost on restart — control plane persists)

### Async Runtime

Tokio for all async operations:
- One runtime per process
- Per-conversation tasks
- `spawn_blocking` for CPU-intensive work (grep, glob, ls)
- Task-local `AGENT_CONTEXT` for implicit context passing

### No Polling

Push-based architecture:
- Control plane pushes to Bridge via `/push/*`
- Bridge sends webhooks back
- No polling loops anywhere

### Zero-Downtime Updates

Drain pattern for agent updates:
1. Mark agent as draining (no new conversations)
2. Wait for in-flight turns to complete
3. Replace agent with new config
4. Resume normal operation

---

## See Also

- [Architecture](../core-concepts/architecture.md) — High-level overview
- [Adding a Tool](adding-a-tool.md)
