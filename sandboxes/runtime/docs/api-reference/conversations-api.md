# Conversations API

Create conversations, send messages, and end conversations.

---

## Create Conversation

Start a new conversation with an agent.

### Request

```
POST /agents/{agent_id}/conversations
```

### Body

All fields are optional. Send an empty body `{}` or omit the body entirely to use defaults.

```json
{
  "tool_names": ["read", "write", "bash"],
  "mcp_server_names": ["github", "jira"],
  "api_key": "<anthropic-api-key>",
  "subagent_api_keys": {
    "explorer": "<anthropic-api-key>",
    "coder": "<anthropic-api-key>"
  },
  "mcp_servers": [
    {
      "name": "project_issues",
      "transport": {
        "type": "streamable_http",
        "url": "https://mcp.example.com/tools",
        "headers": { "Authorization": "Bearer ..." }
      }
    }
  ]
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `tool_names` | string[] | No | Restrict available tools to this list. Names must match exactly (case-sensitive). Invalid names return 400. |
| `mcp_server_names` | string[] | No | Restrict available MCP servers. Only tools from these servers are available. Invalid names return 400. |
| `api_key` | string | No | Override the agent's LLM API key for this conversation only. Useful for per-user billing or testing different providers. |
| `subagent_api_keys` | object | No | Map of subagent name to API key override. Each named subagent uses the specified key instead of the agent's default. |
| `mcp_servers` | McpServerDefinition[] | No | **New in v0.18.0.** Attach additional MCP servers scoped to this conversation only. Connected at creation, torn down on any termination path (end, abort, shutdown, drain). See [Per-Conversation MCP](#per-conversation-mcp) below. |

**Notes:**
- When `tool_names` is omitted, all tools configured on the agent are available.
- When `mcp_server_names` is omitted, all configured MCP servers are available.
- The `api_key` override applies only to this conversation and does not affect other conversations or the agent's stored key.
- Keys in `subagent_api_keys` must match subagent names defined in the agent configuration.
- `tool_names` / `mcp_server_names` filter the **agent's** existing tools. `mcp_servers` are **added on top** of whatever survives that filter.

### Response

```json
{
  "conversation_id": "conv-abc123def456",
  "stream_url": "/conversations/conv-abc123def456/stream"
}
```

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `conversation_id` | string | Unique ID for this conversation |
| `stream_url` | string | Path to the SSE stream endpoint |

### Errors

| Status | Code | Meaning |
|--------|------|---------|
| 404 | `agent_not_found` | Agent doesn't exist |
| 400 | `invalid_request` | Unknown tool name in `tool_names`, unknown MCP server in `mcp_server_names`, empty `api_key`, or any `mcp_servers` validation failure (see below) |
| 429 | `capacity_exhausted` | Global or per-agent concurrent conversation limit reached |

### Example

```bash
# Create conversation
curl -X POST http://localhost:8080/agents/greeter/conversations

# Response: {"conversation_id":"conv-abc","stream_url":"/conversations/conv-abc/stream"}

# Connect to stream (in another terminal)
curl -N http://localhost:8080/conversations/conv-abc/stream \
  -H "Accept: text/event-stream"
```

**Important:** You must connect to the SSE stream *after* creating the conversation and *before* sending the first message. The stream is single-consumer — only one client can be connected at a time.

---

## Per-Conversation MCP

Since v0.18.0, `POST /agents/{agent_id}/conversations` accepts an `mcp_servers` field that attaches one or more MCP servers to a single conversation. The servers are connected at conversation creation, their tools are merged into the conversation's tool set on top of whatever the agent already exposes, and they are disconnected when the conversation ends by any path (`DELETE`, `abort`, shutdown signal, agent drain, `max_turns`, internal error).

Use this when the tool surface varies per call — for example, passing a user-specific API token into an HTTP MCP server, or attaching a dev-only tool to a single debugging session — without mutating the agent definition.

### Request shape

```json
{
  "mcp_servers": [
    {
      "name": "project_issues",
      "transport": {
        "type": "streamable_http",
        "url": "https://mcp.example.com/v1",
        "headers": {
          "Authorization": "Bearer <bridge-control-plane-api-key>",
          "X-Project-Id": "proj-42"
        }
      }
    }
  ]
}
```

Each entry is a `McpServerDefinition` with the same shape as agent-level MCP servers:

```json
{
  "name": "<unique-server-name>",
  "transport": {
    "type": "streamable_http",
    "url": "https://...",
    "headers": { "...": "..." }
  }
}
```

Or for stdio (gated — see [Security](#security) below):

```json
{
  "name": "<unique-server-name>",
  "transport": {
    "type": "stdio",
    "command": "/path/to/mcp-binary",
    "args": ["--flag"],
    "env": { "KEY": "value" }
  }
}
```

### Lifecycle

1. **On create** — Bridge connects to each server, lists its tools, and merges them into the per-conversation executor map. If any step fails, Bridge disconnects whatever connected partially and returns the error — no leaked processes, no dangling conversation handle.
2. **During the conversation** — The agent sees the per-conv MCP tools alongside the agent's existing tools. Tool calls resolve normally.
3. **On end** — The cleanup block in `run_conversation` calls `disconnect_agent(conversation_id)` on the MCP manager, tearing down every connection attached under that conversation's scope.

This runs on *every* termination path enumerated in the [conversation lifecycle audit](../development/architecture-deep-dive.md) — `DELETE`, `POST /abort`, agent drain/update, `SIGINT`, `SIGTERM`, `max_turns`, and internal errors — not just graceful `DELETE`.

### Security

By default, only `streamable_http` transport is accepted from the API. `stdio` transport spawns an arbitrary subprocess with Bridge's privileges, so it is gated behind a runtime flag:

```bash
# To allow stdio MCP servers from the API (NOT recommended in multi-tenant deployments)
export BRIDGE_ALLOW_STDIO_MCP_FROM_API=true
./bridge
```

Or in `config.toml`:

```toml
allow_stdio_mcp_from_api = true
```

When the flag is `false` (default), sending a stdio server in `mcp_servers` returns HTTP 400 with `"stdio transport not allowed from API"`. Keep it off unless your deployment already sandboxes Bridge and trusts every caller.

### Validation errors

All validations run **before** any connection attempt, so a rejected request never spawns a process or opens a socket.

| Error | Cause |
|---|---|
| `mcp_servers: server name cannot be empty` | An entry's `name` is empty or whitespace-only |
| `mcp_servers: duplicate server name '<name>'` | Two entries share the same `name` within a single request |
| `mcp_servers: stdio transport not allowed from API (server '<name>')` | `stdio` transport used while `allow_stdio_mcp_from_api=false` |
| `mcp_servers: failed to connect to '<name>'` | The server listed in the request failed to initialize |
| `mcp_servers: failed to list tools from '<name>': ...` | Connection succeeded but `tools/list` failed |
| `mcp_servers: tool '<tool>' from server '<name>' collides with an existing agent tool` | A tool advertised by the per-conv MCP server shares a name with a tool the agent already has |

### Name collision policy

If a per-conversation MCP server advertises a tool whose name matches one the agent already has (built-in or from an agent-level MCP server), the request is **rejected with HTTP 400**. There is no shadowing, no auto-prefixing — the error is the hint to rename the tool on the MCP side or use `tool_names` to filter the colliding built-in out of the agent's base surface for this conversation.

### Example: attaching a tenant-scoped HTTP MCP server

```bash
curl -X POST http://localhost:8080/agents/support-agent/conversations \
  -H "Content-Type: application/json" \
  -d '{
    "mcp_servers": [{
      "name": "tenant_billing",
      "transport": {
        "type": "streamable_http",
        "url": "https://mcp.internal.example.com/billing",
        "headers": {
          "Authorization": "Bearer <bridge-control-plane-api-key>",
          "X-Tenant-Id": "tenant-42"
        }
      }
    }]
  }'
```

The next `POST /conversations/{id}/messages` sees the agent's existing tools plus everything `tenant_billing` advertised. When the conversation is `DELETE`d (or aborted, or drained), the MCP connection is closed automatically.

### Example: narrowing the agent's base tools to avoid collisions

```bash
curl -X POST http://localhost:8080/agents/coding-agent/conversations \
  -H "Content-Type: application/json" \
  -d '{
    "tool_names": ["bash"],
    "mcp_servers": [{
      "name": "project_fs",
      "transport": { "type": "streamable_http", "url": "https://fs.example.com/mcp" }
    }]
  }'
```

The base agent is narrowed to just `bash`, removing whichever of `Glob`/`Grep`/`Read` would have collided with the MCP server's filesystem tools.

---

## Send Message

Send a message in a conversation.

### Request

```
POST /conversations/{conversation_id}/messages
```

### Body

```json
{
  "content": "Hello, how are you?"
}
```

### Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `content` | string | Yes | Message content to send to the agent |

### Response

```json
{
  "status": "accepted"
}
```

### Errors

| Status | Code | Meaning |
|--------|------|---------|
| 404 | `conversation_not_found` | Conversation doesn't exist |

### Example

```bash
curl -X POST http://localhost:8080/conversations/conv-abc123/messages \
  -H "Content-Type: application/json" \
  -d '{
    "content": "What can you help me with?"
  }'
```

**Note:** Messages are processed asynchronously. The response only confirms the message was queued. The agent's response will arrive via the SSE stream.

---

## Stream Events

Connect to the SSE stream for real-time events.

### Request

```
GET /conversations/{conversation_id}/stream
```

### Headers

```
Accept: text/event-stream
```

### Response

Server-Sent Events stream with the following event types:

```
event: message_start
data: {"type":"message_start","conversation_id":"conv-abc","message_id":"msg-001"}

event: content_delta
data: {"type":"content_delta","delta":"Hello","message_id":"msg-001"}

event: content_delta  
data: {"type":"content_delta","delta":" there","message_id":"msg-001"}

event: tool_call_start
data: {"type":"tool_call_start","id":"call-123","name":"read","arguments":{"path":"file.txt"}}

event: tool_call_result
data: {"type":"tool_call_result","id":"call-123","result":"...","is_error":false}

event: message_end
data: {"type":"message_end","message_id":"msg-001","usage":{"input_tokens":50,"output_tokens":10}}

event: done
data: {"type":"done"}
```

### Event Types

See [SSE Events](sse-events.md) for the complete list of event types and their payloads.

Key events:
- `message_start` — Agent started responding
- `content_delta` — Text chunk (stream these to the user)
- `tool_call_start` — Agent invoked a tool
- `tool_call_result` — Tool execution completed
- `tool_approval_required` — Tool needs user approval (if permissions require it)
- `message_end` — Response complete with token usage
- `done` — Turn finished (stream stays open for next message)
- `error` — Something went wrong

### Stream Behavior

| Property | Value |
|----------|-------|
| Keepalive ping | Every 15 seconds (`:ping` comment) |
| Buffer size | 256 events |
| Concurrent connections | 1 per conversation |
| Stream lifetime | Entire conversation duration |

### Errors

| Status | Code | Meaning |
|--------|------|---------|
| 404 | `conversation_not_found` | Conversation doesn't exist or another client is already streaming |

### Example

```bash
# Connect to stream
curl -N http://localhost:8080/conversations/conv-abc123/stream \
  -H "Accept: text/event-stream"
```

**Note:** If you get a 404 when connecting to the stream, either:
1. The conversation doesn't exist
2. Another client is already connected to this conversation's stream

---

## Abort Current Turn

Cancel the agent's current response generation.

### Request

```
POST /conversations/{conversation_id}/abort
```

### Response

```json
{
  "status": "aborted"
}
```

### Example

```bash
curl -X POST http://localhost:8080/conversations/conv-abc123/abort
```

Useful for "stop generating" buttons in UIs. After aborting:
- An `error` event with code `aborted` is sent on the stream
- A `done` event follows to signal turn completion
- The conversation remains active for the next message
- The aborted user message is removed from history (no partial response)

---

## End Conversation

Delete a conversation and free up resources.

### Request

```
DELETE /conversations/{conversation_id}
```

### Response

```json
{
  "status": "ended"
}
```

### Errors

| Status | Code | Meaning |
|--------|------|---------|
| 404 | `conversation_not_found` | Conversation doesn't exist |

### Example

```bash
curl -X DELETE http://localhost:8080/conversations/conv-abc123
```

**Note:** This permanently ends the conversation:
- The SSE stream is closed
- All pending tool approvals are cancelled
- Session data is cleaned up
- The conversation ID becomes invalid

---

## Conversation States

Conversations have these states:

| State | Description |
|-------|-------------|
| `idle` | Waiting for user input |
| `processing` | Agent is generating a response |
| `waiting_for_approval` | Tool needs user approval |
| `error` | An error occurred (recoverable) |
| `ended` | Conversation was deleted |

Check state by:
- Listening to SSE events (most accurate)
- Attempting to send a message (fails if conversation ended)
- Connecting to the stream (404 if conversation doesn't exist)

---

## Timeout Behavior

| Timeout | Value | Description |
|---------|-------|-------------|
| Agent chat | 180 seconds | Maximum time for a single LLM call |
| With approvals | Unlimited | Timeout disabled when tools require approval |
| SSE keepalive | 15 seconds | Ping interval to keep connection alive |

When the agent chat times out:
- An `error` event with code `agent_timeout` is sent
- A `done` event follows
- The turn ends and the conversation waits for the next message

---

## Multi-Turn Conversation Flow

```
1. POST /agents/{agent_id}/conversations
   → Returns conversation_id

2. GET /conversations/{conv_id}/stream
   → SSE connection established

3. POST /conversations/{conv_id}/messages
   → Message queued

4. ← SSE events: message_start, content_delta, ..., message_end, done

5. (Repeat steps 3-4 for each turn)

6. DELETE /conversations/{conv_id}
   → Conversation ended, stream closed
```

**Important:** The SSE stream remains open across multiple turns. Do not reconnect between messages unless the connection drops.

---

## See Also

- [SSE Events](sse-events.md) — All streaming event types and payloads
- [Agents API](agents-api.md) — Tool approvals endpoint
- [Webhooks](../core-concepts/webhooks.md) — Event notifications
