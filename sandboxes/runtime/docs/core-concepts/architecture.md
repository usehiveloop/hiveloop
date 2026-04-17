# Architecture

Bridge is organized as a Rust workspace with multiple crates, each handling a specific concern.

---

## System Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                         Bridge                               │
│                                                              │
│  ┌──────────┐    ┌──────────┐    ┌──────────────────────┐   │
│  │   API    │───►│ Runtime  │───►│        LLM           │   │
│  │  Layer   │    │  Layer   │    │     Providers        │   │
│  └──────────┘    └──────────┘    └──────────────────────┘   │
│       │                │                                      │
│       ▼                ▼                                      │
│  ┌──────────┐    ┌──────────┐    ┌──────────┐               │
│  │  Tools   │    │   MCP    │    │ Webhooks │               │
│  └──────────┘    └──────────┘    └──────────┘               │
│       │                                                       │
│       ▼                                                       │
│  ┌──────────┐                                                │
│  │   LSP    │                                                │
│  └──────────┘                                                │
└─────────────────────────────────────────────────────────────┘
```

---

## Crate Structure

| Crate | Purpose | Package Name |
|-------|---------|--------------|
| **bridge** | Main binary, configuration loading, startup | `bridge` |
| **api** | HTTP handlers, routing, middleware, SSE streaming | `api` |
| **core** | Domain models, error types, configuration schemas | `core` (lib: `bridge_core`) |
| **runtime** | Agent supervision, conversation management, state | `runtime` |
| **llm** | Provider integrations (Anthropic, OpenAI-compatible, etc.) | `llm` |
| **tools** | Built-in tool implementations | `tools` |
| **mcp** | Model Context Protocol client | `mcp` |
| **webhooks** | Webhook dispatch with HMAC signing | `webhooks` |
| **lsp** | Language Server Protocol integration | `lsp` |

---

## Dependency Graph

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
    └── (external deps only)

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

## Data Flow: A Message Journey

Here's what happens when a user sends a message:

```
1. HTTP Request
   POST /conversations/{id}/messages
   ↓
2. API Layer (api crate)
   - Validate request
   - Extract conversation ID
   - Find the agent runner
   ↓
3. Runtime Layer (runtime crate)
   - Queue the message
   - Check conversation state
   - Trigger agent turn
   ↓
4. LLM Layer (llm crate)
   - Build the prompt from history
   - Call the AI provider
   - Stream the response
   ↓
5. Tool Execution (if needed)
   - LLM requests tool use
   - tools/mcp crate runs the tool
   - Result goes back to LLM
   ↓
6. Response Streaming
   - Events flow back through the stack
   - API layer sends SSE events
   - Client receives real-time updates
```

---

## The Runtime Layer

The runtime is the heart of Bridge. It manages:

### Agent Supervision
- Maintains a map of running agents (`AgentMap`)
- Handles agent lifecycle (start, update, drain)
- Restarts crashed agents
- Applies configuration diffs from control plane

### Conversation State
- Tracks active conversations per agent
- Manages message history
- Handles compaction (summarizing old messages)
- Provides per-conversation abort tokens

### Turn Management
- Processes one "turn" at a time (user message → AI response)
- Handles tool call loops
- Streams events to the client
- Wraps tool execution in `AGENT_CONTEXT` task-local scope

### Key Runtime Modules

| Module | Purpose |
|--------|---------|
| `supervisor.rs` | Central agent lifecycle management |
| `agent_map.rs` | Concurrent agent storage (DashMap) |
| `agent_runner.rs` | Per-agent event loop and subagent support |
| `agent_state.rs` | Complete runtime state for a single agent |
| `conversation.rs` | Conversation event loop and turn processing |
| `compaction.rs` | History summarization |
| `drain.rs` | Graceful shutdown with in-flight request draining |
| `system_reminder.rs` | Periodic system message injection |
| `token_tracker.rs` | Token usage tracking |
| `permission_manager.rs` | Runtime-side permission handling |

---

## The API Layer

The API layer handles HTTP concerns:

### Routing
- Public endpoints (`/agents`, `/conversations`)
- Push endpoints (`/push/*` — requires auth)
- Health and metrics (`/health`, `/metrics`)
- Tool approvals (`/agents/{id}/conversations/{id}/approvals`)

### Middleware
- Authentication for push endpoints (bearer token)
- Request logging via `TraceLayer`
- CORS (permissive)

### SSE Streaming
- Maintains long-lived connections
- Sends events as they happen
- Handles client disconnects
- Stores active streams in `AppState`

### Handler Modules

| Module | Endpoints |
|--------|-----------|
| `health.rs` | `GET /health` |
| `agents.rs` | `GET /agents`, `GET /agents/{id}` |
| `conversations.rs` | `POST /agents/{id}/conversations`, `POST /conversations/{id}/messages`, `DELETE /conversations/{id}`, `POST /conversations/{id}/abort` |
| `stream.rs` | `GET /conversations/{id}/stream` |
| `metrics.rs` | `GET /metrics` |
| `permissions.rs` | `GET/POST /agents/{id}/conversations/{id}/approvals` |
| `push.rs` | `POST /push/agents`, `PUT/DELETE /push/agents/{id}`, `POST /push/diff`, etc. |

---

## The Tools System

Tools are organized in the `tools` crate:

### Tool Registration

Tools are registered explicitly via the `ToolRegistry`:

```rust
let mut registry = ToolRegistry::new();
registry.register(Arc::new(BashTool::new()));
```

Built-in tools are registered by `register_builtin_tools()` in `builtin.rs`.

### Built-in Tools
- **Filesystem**: `read`, `write`, `edit`, `apply_patch`, `multiedit`, `ls`, `Glob`, `Grep`
- **Shell**: `bash` command execution
- **Web**: `web_fetch`, `web_search` (if SEARCH_ENDPOINT set)
- **Agent management**: `agent`, `sub_agent`
- **Task tracking**: `todowrite`, `todoread`
- **Batch execution**: `batch`
- **LSP integration**: `lsp` (if LSP manager provided)

### Tool Execution Context

Tools execute within a Tokio task-local `AGENT_CONTEXT` that provides:
- Conversation ID and agent ID
- Subagent runner for spawning child agents
- Notification channel for delivering background subagent results into the parent's next turn
- Task budget for capping subagent spawns per conversation

This context is set by the runtime's conversation loop and accessible via:
```rust
AGENT_CONTEXT.try_with(|ctx| { ... })
```

### MCP Tools

External tools accessed via MCP servers. The MCP crate handles:
- Connecting to stdio and HTTP servers
- Tool discovery
- Call execution via `McpToolExecutor`

---

## State Management

Bridge keeps state in memory. There's no database:

- **Agents** — Stored in an `AgentMap` (concurrent hash map via DashMap)
- **Conversations** — Stored per agent in `AgentState`
- **Message history** — Vector of messages in conversation state
- **SSE streams** — Stored in `AppState` (DashMap)
- **Task registry** — Background subagent tasks

This means:
- **Fast** — No database queries
- **Ephemeral** — Restarting Bridge clears all state
- **Scalable vertically** — Add RAM for more conversations

For persistence, your control plane:
1. Pushes agents on startup
2. Hydrates conversation history when needed
3. Receives webhooks to save events

---

## Threading Model

Bridge uses Tokio for async execution:

- **One async runtime** per process (`#[tokio::main]`)
- **Per-conversation tasks** — Each conversation runs in its own Tokio task
- **spawn_blocking** — CPU-intensive operations (grep, glob, ls) run in blocking threads
- **Task-local context** — `AGENT_CONTEXT` provides per-conversation data without explicit passing
- **Cancellation tokens** — Graceful shutdown via `tokio_util::sync::CancellationToken`
- **Rate limiting** — Per-agent request throttling via `governor`

---

## Key Design Patterns

### Task-Local Context
The `AGENT_CONTEXT` task-local variable carries conversation-scoped data:
- Eliminates need to pass context through every function
- Automatically propagates to subagent spawns
- Used by tools to access the subagent runner and send notifications

### Graceful Drain
The `drain.rs` module handles zero-downtime agent updates:
- Stop accepting new conversations
- Wait for in-flight turns to complete
- Replace agent definition
- Resume with new configuration

### Permission Manager
Centralized tool approval system:
- Intercepts tool calls requiring approval
- Queues approval requests per conversation
- Resumes execution after approval/denial

### Webhook Dispatch
Async webhook delivery with:
- HMAC-SHA256 signing
- Exponential backoff retry
- Separate dispatcher task

---

## Deployment Patterns

### Single Instance
Simplest setup. One Bridge process handles everything.

### Multiple Instances
Run multiple Bridge instances behind a load balancer:
- Push agents to all instances
- Route conversations consistently (sticky sessions)
- Each instance manages its own conversation state

### Read-Heavy with Edge
Run Bridge close to users (edge locations):
- Push agents to all regions
- Webhooks go to central control plane
- Low-latency streaming to users

---

## See Also

- [Development: Architecture Deep Dive](../development/architecture-deep-dive.md) — Code-level details
- [Deployment](../deployment/index.md) — Production setups
