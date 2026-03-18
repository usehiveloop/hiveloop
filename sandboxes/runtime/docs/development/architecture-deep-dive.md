# Architecture Deep Dive

Internal architecture of Bridge.

---

## Crate Dependencies

```
bridge (binary)
├── api
│   ├── core
│   └── runtime
├── runtime
│   ├── core
│   ├── llm
│   ├── tools
│   └── mcp
├── llm
│   └── core
├── tools
│   └── core
├── mcp
│   └── core
├── webhooks
│   └── core
└── lsp
```

---

## API Layer (`api`)

HTTP request handling.

### Responsibilities

- Route requests
- Validate input
- Authenticate push endpoints
- Stream SSE events

### Key Files

- `router.rs` — Route definitions
- `sse.rs` — Server-Sent Events
- `middleware.rs` — Auth, logging

---

## Runtime Layer (`runtime`)

Agent and conversation management.

### Components

| Module | Purpose |
|--------|---------|
| `supervisor.rs` | Agent lifecycle |
| `agent_map.rs` | Agent storage |
| `agent_runner.rs` | Per-agent event loop |
| `conversation.rs` | Conversation state |
| `compaction.rs` | History summarization |

### Agent State Machine

```
Idle → Processing → ToolCalls → Processing → Complete
  ↑                                    ↓
  └────────────────────────────────────┘
```

---

## LLM Layer (`llm`)

Provider integrations.

### Structure

```
llm/
├── providers.rs      # Provider dispatch
├── factory.rs        # Provider creation
├── streaming.rs      # SSE streaming
├── tool_adapter.rs   # Tool format conversion
└── tool_hook.rs      # Tool interception
```

### Adding a Provider

1. Implement `LLMProvider` trait
2. Add to factory
3. Update documentation

---

## Tools Layer (`tools`)

Built-in tool implementations.

### Tool Registration

Tools register themselves:

```rust
// In tool implementation
inventory::submit! {
    ToolDefinition::new("tool_name", handler)
}
```

### Tool Execution

1. Parse arguments (JSON Schema)
2. Execute
3. Return result

---

## MCP Layer (`mcp`)

Model Context Protocol client.

### Transports

- `stdio` — Local command
- `http` — Remote server

### Connection Lifecycle

```
Connect → Initialize → Tool Discovery → Ready → Calls → Shutdown
```

---

## Data Flow

```
HTTP Request
    ↓
api::router
    ↓
runtime::supervisor
    ↓
runtime::conversation
    ↓
llm::providers
    ↓
External API
    ↓
Stream chunks back
    ↓
api::sse
    ↓
Client
```

---

## Testing Strategy

| Test Type | Location | Purpose |
|-----------|----------|---------|
| Unit | Each crate | Individual functions |
| Integration | `api/tests.rs` | API endpoints |
| E2E | `e2e/` | Full workflows |

---

## Key Design Decisions

### In-Memory State

Bridge keeps state in memory for speed. No database means:
- Fast access
- Simple operations
- Ephemeral (data lost on restart)

### Async Runtime

Tokio for all async operations.
- One runtime per process
- Per-conversation tasks
- Blocking operations in spawn_blocking

### No Polling

Push-based architecture:
- Control plane pushes to Bridge
- Bridge sends webhooks back
- No polling loops

---

## See Also

- [Architecture](../core-concepts/architecture.md) — High-level overview
- [Adding a Tool](adding-a-tool.md)
