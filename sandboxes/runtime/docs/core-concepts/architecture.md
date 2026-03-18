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
└─────────────────────────────────────────────────────────────┘
```

---

## Crate Structure

| Crate | Purpose |
|-------|---------|
| **bridge** | Main binary, configuration loading, startup |
| **api** | HTTP handlers, routing, middleware, SSE streaming |
| **core** | Domain models, error types, configuration schemas |
| **runtime** | Agent supervision, conversation management, state |
| **llm** | Provider integrations (Anthropic, OpenAI-compatible, etc.) |
| **tools** | Built-in tool implementations |
| **mcp** | Model Context Protocol client |
| **webhooks** | Webhook dispatch with HMAC signing |
| **lsp** | Language Server Protocol integration |

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
- Maintains a map of running agents
- Handles agent lifecycle (start, update, drain)
- Restarts crashed agents

### Conversation State
- Tracks active conversations per agent
- Manages message history
- Handles compaction (summarizing old messages)

### Turn Management
- Processes one "turn" at a time (user message → AI response)
- Handles tool call loops
- Streams events to the client

---

## The API Layer

The API layer handles HTTP concerns:

### Routing
- Public endpoints (`/agents`, `/conversations`)
- Push endpoints (`/push/*` — requires auth)
- Health and metrics (`/health`, `/metrics`)

### Middleware
- Authentication for push endpoints
- Request logging
- CORS (if enabled)

### SSE Streaming
- Maintains long-lived connections
- Sends events as they happen
- Handles client disconnects

---

## The Tools System

Tools are organized in the `tools` crate:

### Built-in Tools
- Filesystem: read, write, edit, ls, glob
- Shell: bash command execution
- Search: grep, web search
- Agent management: spawn_agent, parallel_agent, join
- Task tracking: todo

### MCP Tools
External tools accessed via MCP servers. The MCP crate handles:
- Connecting to stdio and HTTP servers
- Tool discovery
- Call execution

---

## State Management

Bridge keeps state in memory. There's no database:

- **Agents** — Stored in an `AgentMap` (concurrent hash map)
- **Conversations** — Stored per agent in `AgentState`
- **Message history** — Vector of messages in conversation state

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

- One async runtime per process
- Each conversation gets its own task
- Tool calls may spawn blocking threads
- MCP servers run in separate processes

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
