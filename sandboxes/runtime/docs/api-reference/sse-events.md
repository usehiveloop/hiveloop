# SSE Events

Bridge uses Server-Sent Events (SSE) to stream real-time updates to clients.

---

## Connecting

Connect to the stream endpoint:

```bash
curl -N http://localhost:8080/conversations/{conversation_id}/stream \
  -H "Accept: text/event-stream"
```

Or in JavaScript:

```javascript
const eventSource = new EventSource(
  'http://localhost:8080/conversations/conv-123/stream'
);

eventSource.addEventListener('message_start', (e) => {
  const event = JSON.parse(e.data);  // Full BridgeEvent object
  console.log('Message started:', event.data.message_id);
});

eventSource.addEventListener('content_delta', (e) => {
  const event = JSON.parse(e.data);
  process.stdout.write(event.data.delta);  // Stream text to UI
});
```

**Note:** The SSE stream can only be consumed by one client at a time. If another client is already connected, the endpoint will return a 404 error until the previous connection is closed.

---

## Event Format

Each event has an SSE event name and a JSON data payload. The data payload is the full `BridgeEvent` object (the same structure used by webhooks and WebSocket):

```
event: {sse_event_name}
data: {"event_id":"...","event_type":"...","agent_id":"...","conversation_id":"...","timestamp":"...","sequence_number":1,"data":{...}}
```

Multiple events can be sent in sequence:

```
event: message_start
data: {"event_id":"evt-001","event_type":"response_started","agent_id":"my-agent","conversation_id":"conv-abc","timestamp":"2026-01-15T10:30:00Z","sequence_number":1,"data":{"conversation_id":"conv-abc","message_id":"msg-001"}}

event: content_delta
data: {"event_id":"evt-002","event_type":"response_chunk","agent_id":"my-agent","conversation_id":"conv-abc","timestamp":"2026-01-15T10:30:00Z","sequence_number":2,"data":{"delta":"Hello","message_id":"msg-001"}}

event: message_end
data: {"event_id":"evt-003","event_type":"response_completed","agent_id":"my-agent","conversation_id":"conv-abc","timestamp":"2026-01-15T10:30:01Z","sequence_number":3,"data":{"message_id":"msg-001","input_tokens":50,"output_tokens":10,"model":"claude-sonnet-4-20250514","timestamp":"2026-01-15T10:30:01Z","full_response":"Hello"}}

event: done
data: {"event_id":"evt-004","event_type":"done","agent_id":"my-agent","conversation_id":"conv-abc","timestamp":"2026-01-15T10:30:01Z","sequence_number":4,"data":{}}
```

### SSE Event Name Mapping

SSE uses different event names than the internal `event_type` for some events:

| SSE Event Name | Internal `event_type` |
|---|---|
| `message_start` | `response_started` |
| `content_delta` | `response_chunk` |
| `message_end` | `response_completed` |
| `tool_call_start` | `tool_call_started` |
| `tool_call_result` | `tool_call_completed` |
| `error` | `agent_error` |

All other events use the same name for both SSE event name and `event_type`.

---

## Conversation Events

### `conversation_created`

A new conversation was created.

```json
{
  "event_id": "evt-001",
  "event_type": "conversation_created",
  "agent_id": "my-agent",
  "conversation_id": "conv-abc123",
  "timestamp": "2026-01-15T10:30:00Z",
  "sequence_number": 1,
  "data": {}
}
```

### `message_received`

A user message was received.

```json
{
  "event_id": "evt-002",
  "event_type": "message_received",
  "agent_id": "my-agent",
  "conversation_id": "conv-abc123",
  "timestamp": "2026-01-15T10:30:00Z",
  "sequence_number": 2,
  "data": {
    "content": "Hello, can you help me?"
  }
}
```

| Data Field | Type | Description |
|------------|------|-------------|
| `content` | string | The user's message content |

### `conversation_ended`

The conversation was ended (via `DELETE /conversations/{id}`).

```json
{
  "event_id": "evt-099",
  "event_type": "conversation_ended",
  "agent_id": "my-agent",
  "conversation_id": "conv-abc123",
  "timestamp": "2026-01-15T10:35:00Z",
  "sequence_number": 99,
  "data": {}
}
```

---

## Message Events

### `message_start`

A new assistant message has started.

```json
{
  "event_id": "evt-003",
  "event_type": "response_started",
  "agent_id": "my-agent",
  "conversation_id": "conv-abc123",
  "timestamp": "2026-01-15T10:30:00Z",
  "sequence_number": 3,
  "data": {
    "conversation_id": "conv-abc123",
    "message_id": "msg-001"
  }
}
```

| Data Field | Type | Description |
|------------|------|-------------|
| `conversation_id` | string | The conversation ID |
| `message_id` | string | Provider-assigned message ID |

### `content_delta`

A chunk of text content from the assistant. Multiple deltas make up the full message.

```json
{
  "event_id": "evt-004",
  "event_type": "response_chunk",
  "agent_id": "my-agent",
  "conversation_id": "conv-abc123",
  "timestamp": "2026-01-15T10:30:00Z",
  "sequence_number": 4,
  "data": {
    "delta": " chunk of text",
    "message_id": "msg-001"
  }
}
```

| Data Field | Type | Description |
|------------|------|-------------|
| `delta` | string | The text chunk |
| `message_id` | string | Provider-assigned message ID |

### `message_end`

The assistant message is complete.

```json
{
  "event_id": "evt-010",
  "event_type": "response_completed",
  "agent_id": "my-agent",
  "conversation_id": "conv-abc123",
  "timestamp": "2026-01-15T10:30:01Z",
  "sequence_number": 10,
  "data": {
    "message_id": "msg-001",
    "input_tokens": 150,
    "output_tokens": 75,
    "model": "claude-sonnet-4-20250514",
    "timestamp": "2026-01-15T10:30:01Z",
    "full_response": "Here's the complete response text..."
  }
}
```

| Data Field | Type | Description |
|------------|------|-------------|
| `message_id` | string | Provider-assigned message ID |
| `input_tokens` | number | Number of input tokens consumed |
| `output_tokens` | number | Number of output tokens generated |
| `model` | string | Model identifier used for this response |
| `timestamp` | string | ISO 8601 completion timestamp |
| `full_response` | string | The complete assembled response text |

---

## Tool Events

### `tool_call_start`

The agent is calling a tool.

```json
{
  "event_id": "evt-005",
  "event_type": "tool_call_started",
  "agent_id": "my-agent",
  "conversation_id": "conv-abc123",
  "timestamp": "2026-01-15T10:30:00Z",
  "sequence_number": 5,
  "data": {
    "id": "call-123",
    "name": "read",
    "arguments": {
      "path": "/path/to/file"
    }
  }
}
```

| Data Field | Type | Description |
|------------|------|-------------|
| `id` | string | Tool call ID (correlates with `tool_call_result`) |
| `name` | string | Name of the tool being called |
| `arguments` | object | Arguments passed to the tool |

### `tool_call_result`

The tool has finished executing.

```json
{
  "event_id": "evt-006",
  "event_type": "tool_call_completed",
  "agent_id": "my-agent",
  "conversation_id": "conv-abc123",
  "timestamp": "2026-01-15T10:30:01Z",
  "sequence_number": 6,
  "data": {
    "id": "call-123",
    "tool_name": "read",
    "result": "file contents...",
    "is_error": false,
    "duration_ms": 42
  }
}
```

| Data Field | Type | Description |
|------------|------|-------------|
| `id` | string | Tool call ID (matches `tool_call_start`) |
| `tool_name` | string | Name of the tool that was called |
| `result` | string | Result from the tool execution (JSON string or plain text) |
| `is_error` | boolean | Whether the tool execution resulted in an error |
| `duration_ms` | number | How long the tool call took, in milliseconds |

### `tool_approval_required`

A tool requires user approval before running (when tool permission is set to `require_approval`).

```json
{
  "event_id": "evt-007",
  "event_type": "tool_approval_required",
  "agent_id": "my-agent",
  "conversation_id": "conv-abc123",
  "timestamp": "2026-01-15T10:30:00Z",
  "sequence_number": 7,
  "data": {
    "request_id": "req-456",
    "tool_name": "bash",
    "tool_call_id": "call-789",
    "arguments": {
      "command": "rm -rf /data"
    },
    "integration_name": null,
    "integration_action": null
  }
}
```

| Data Field | Type | Description |
|------------|------|-------------|
| `request_id` | string | Unique ID for this approval request (use this to approve/deny) |
| `tool_name` | string | Name of the tool being called |
| `tool_call_id` | string | The LLM's tool call ID |
| `arguments` | object | Arguments passed to the tool |
| `integration_name` | string \| null | Integration name if this is an integration tool (e.g., "github") |
| `integration_action` | string \| null | Integration action if this is an integration tool (e.g., "create_pull_request") |

### `tool_approval_resolved`

User approved or denied a tool call.

```json
{
  "event_id": "evt-008",
  "event_type": "tool_approval_resolved",
  "agent_id": "my-agent",
  "conversation_id": "conv-abc123",
  "timestamp": "2026-01-15T10:30:01Z",
  "sequence_number": 8,
  "data": {
    "request_id": "req-456",
    "decision": "approve"
  }
}
```

| Data Field | Type | Description |
|------------|------|-------------|
| `request_id` | string | The approval request ID that was resolved |
| `decision` | string | Either `"approve"` or `"deny"` |

---

## Todo Events

### `todo_updated`

The todo list was updated via the `todowrite` tool.

```json
{
  "event_id": "evt-020",
  "event_type": "todo_updated",
  "agent_id": "my-agent",
  "conversation_id": "conv-abc123",
  "timestamp": "2026-01-15T10:30:01Z",
  "sequence_number": 20,
  "data": {
    "todos": [
      {
        "content": "Refactor auth module",
        "status": "in_progress",
        "priority": "high"
      },
      {
        "content": "Write tests",
        "status": "pending",
        "priority": "medium"
      }
    ]
  }
}
```

| Data Field | Type | Description |
|------------|------|-------------|
| `todos` | array | The complete current todo list |
| `todos[].content` | string | Brief description of the task |
| `todos[].status` | string | Current status: `pending`, `in_progress`, `completed`, or `cancelled` |
| `todos[].priority` | string | Priority level: `high`, `medium`, or `low` |

---

## Reasoning Events

### `reasoning_delta`

A chunk of reasoning/thinking text from the model. Fired when using extended thinking models (e.g., DeepSeek R1, OpenAI o1). Multiple deltas make up the full reasoning trace.

```json
{
  "event_id": "evt-015",
  "event_type": "reasoning_delta",
  "agent_id": "my-agent",
  "conversation_id": "conv-abc123",
  "timestamp": "2026-01-15T10:30:00Z",
  "sequence_number": 15,
  "data": {
    "delta": "Let me think about this...",
    "message_id": "msg-uuid"
  }
}
```

| Data Field | Type | Description |
|------------|------|-------------|
| `delta` | string | The reasoning text chunk |
| `message_id` | string | Provider-assigned message ID |

---

## Conversation Events

### `conversation_compacted`

Fires when the conversation history is summarized to reduce token count. This happens automatically when the context window is getting full.

```json
{
  "event_id": "evt-030",
  "event_type": "conversation_compacted",
  "agent_id": "my-agent",
  "conversation_id": "conv-abc123",
  "timestamp": "2026-01-15T10:31:00Z",
  "sequence_number": 30,
  "data": {
    "summary": "The user asked about deploying a Rust web service. We discussed Docker configs and CI pipelines.",
    "messages_compacted": 12,
    "pre_compaction_tokens": 5000,
    "post_compaction_tokens": 1200
  }
}
```

| Data Field | Type | Description |
|------------|------|-------------|
| `summary` | string | The generated summary that replaces compacted messages |
| `messages_compacted` | number | Number of messages that were summarized |
| `pre_compaction_tokens` | number | Token count before compaction |
| `post_compaction_tokens` | number | Token count after compaction |

---

## Background Task Events

### `background_task_completed`

Fires when a background bash command or subagent task completes. Background tasks run outside the main conversation turn (e.g., long-running shell commands started with `run_in_background`).

```json
{
  "event_id": "evt-040",
  "event_type": "background_task_completed",
  "agent_id": "my-agent",
  "conversation_id": "conv-abc123",
  "timestamp": "2026-01-15T10:31:00Z",
  "sequence_number": 40,
  "data": {
    "task_id": "550e8400-e29b-41d4-a716-446655440000",
    "description": "run tests",
    "output": "all tests passed",
    "is_error": false
  }
}
```

| Data Field | Type | Description |
|------------|------|-------------|
| `task_id` | string | UUID identifying the background task |
| `description` | string | Human-readable description of the task |
| `output` | string | Output produced by the task |
| `is_error` | boolean | Whether the task completed with an error |

---

## Subagent Events

### `sub_agent_started`

Fires when a subagent is spawned. Subagents are child conversations that run tasks on behalf of the parent conversation.

```json
{
  "event_id": "evt-050",
  "event_type": "sub_agent_started",
  "agent_id": "my-agent",
  "conversation_id": "conv-abc123",
  "timestamp": "2026-01-15T10:30:02Z",
  "sequence_number": 50,
  "data": {
    "subagent_name": "explorer",
    "mode": "foreground",
    "parent_conversation_id": "conv-abc123",
    "depth": 1
  }
}
```

| Data Field | Type | Description |
|------------|------|-------------|
| `subagent_name` | string | Name of the subagent being spawned |
| `mode` | string | Either `"foreground"` (blocks parent) or `"background"` (runs concurrently) |
| `parent_conversation_id` | string | The conversation ID of the parent that spawned this subagent |
| `depth` | number | Nesting depth (1 = direct child of main conversation) |

### `sub_agent_completed`

Fires when a subagent finishes its task and returns control to the parent conversation.

```json
{
  "event_id": "evt-060",
  "event_type": "sub_agent_completed",
  "agent_id": "my-agent",
  "conversation_id": "conv-abc123",
  "timestamp": "2026-01-15T10:30:07Z",
  "sequence_number": 60,
  "data": {
    "subagent_name": "explorer",
    "mode": "foreground",
    "task_id": "conv-uuid-task-uuid",
    "duration_ms": 5234,
    "is_error": false
  }
}
```

| Data Field | Type | Description |
|------------|------|-------------|
| `subagent_name` | string | Name of the subagent that completed |
| `mode` | string | Either `"foreground"` or `"background"` |
| `task_id` | string | Unique identifier for the subagent's task |
| `duration_ms` | number | How long the subagent ran, in milliseconds |
| `is_error` | boolean | Whether the subagent completed with an error |

---

## Error Events

### `error`

Something went wrong during the conversation turn.

```json
{
  "event_id": "evt-070",
  "event_type": "agent_error",
  "agent_id": "my-agent",
  "conversation_id": "conv-abc123",
  "timestamp": "2026-01-15T10:30:01Z",
  "sequence_number": 70,
  "data": {
    "code": "TOOL_EXECUTION_ERROR",
    "message": "Failed to read file: permission denied"
  }
}
```

| Data Field | Type | Description |
|------------|------|-------------|
| `code` | string | Error code (see below) |
| `message` | string | Human-readable error message |

Common error codes:
- `TOOL_EXECUTION_ERROR` — Tool failed to execute
- `LLM_ERROR` — AI provider error
- `RATE_LIMITED` — Hit rate limit
- `CONTEXT_LENGTH_EXCEEDED` — Too many tokens
- `agent_timeout` — Agent chat timed out (180 seconds)
- `max_turns_exceeded` — Maximum conversation turns exceeded
- `aborted` — Turn was aborted by user
- `agent_error` — General agent error

---

## Control Events

### `done`

The response stream is complete. This event signals the end of the current turn.

```json
{
  "event_id": "evt-011",
  "event_type": "done",
  "agent_id": "my-agent",
  "conversation_id": "conv-abc123",
  "timestamp": "2026-01-15T10:30:01Z",
  "sequence_number": 11,
  "data": {}
}
```

**Note:** After receiving `done`, the connection remains open for the next user message. The stream only ends when:
- The conversation is deleted via `DELETE /conversations/{conv_id}`
- The agent is shut down
- The connection is dropped

### `turn_completed`

The conversation turn is fully complete. Includes cumulative token usage for the turn.

```json
{
  "event_id": "evt-012",
  "event_type": "turn_completed",
  "agent_id": "my-agent",
  "conversation_id": "conv-abc123",
  "timestamp": "2026-01-15T10:30:01Z",
  "sequence_number": 12,
  "data": {
    "input_tokens": 150,
    "output_tokens": 75,
    "model": "claude-sonnet-4-20250514",
    "timestamp": "2026-01-15T10:30:01Z",
    "turn_number": 1,
    "cumulative_input_tokens": 150,
    "cumulative_output_tokens": 75
  }
}
```

| Data Field | Type | Description |
|------------|------|-------------|
| `input_tokens` | number | Input tokens for this turn's final response |
| `output_tokens` | number | Output tokens for this turn's final response |
| `model` | string | Model identifier used |
| `timestamp` | string | ISO 8601 completion timestamp |
| `turn_number` | number | The turn number within the conversation |
| `cumulative_input_tokens` | number | Total input tokens across all turns |
| `cumulative_output_tokens` | number | Total output tokens across all turns |

### `ping` (Keepalive)

Keepalive sent every 15 seconds to prevent connection timeout. This is an SSE comment, not a proper event:

```
:ping
```

Most SSE clients ignore comments automatically. No action is required.

---

## Complete Example

A full conversation turn (data fields abbreviated for readability — actual payloads include all `BridgeEvent` fields):

```
event: message_start
data: {"event_id":"evt-001","event_type":"response_started","agent_id":"my-agent","conversation_id":"conv-001","timestamp":"2026-01-15T10:30:00Z","sequence_number":1,"data":{"conversation_id":"conv-001","message_id":"msg-001"}}

event: content_delta
data: {"event_id":"evt-002","event_type":"response_chunk","agent_id":"my-agent","conversation_id":"conv-001","timestamp":"2026-01-15T10:30:00Z","sequence_number":2,"data":{"delta":"I'll check the file for you.","message_id":"msg-001"}}

event: tool_call_start
data: {"event_id":"evt-003","event_type":"tool_call_started","agent_id":"my-agent","conversation_id":"conv-001","timestamp":"2026-01-15T10:30:00Z","sequence_number":3,"data":{"id":"call-001","name":"read","arguments":{"path":"README.md"}}}

event: tool_call_result
data: {"event_id":"evt-004","event_type":"tool_call_completed","agent_id":"my-agent","conversation_id":"conv-001","timestamp":"2026-01-15T10:30:01Z","sequence_number":4,"data":{"id":"call-001","tool_name":"read","result":"# Project...","is_error":false,"duration_ms":42}}

event: content_delta
data: {"event_id":"evt-005","event_type":"response_chunk","agent_id":"my-agent","conversation_id":"conv-001","timestamp":"2026-01-15T10:30:01Z","sequence_number":5,"data":{"delta":"The README says...","message_id":"msg-001"}}

event: message_end
data: {"event_id":"evt-006","event_type":"response_completed","agent_id":"my-agent","conversation_id":"conv-001","timestamp":"2026-01-15T10:30:01Z","sequence_number":6,"data":{"message_id":"msg-001","input_tokens":50,"output_tokens":100,"model":"claude-sonnet-4-20250514","timestamp":"2026-01-15T10:30:01Z","full_response":"I'll check the file for you.\n\nThe README says..."}}

event: done
data: {"event_id":"evt-007","event_type":"done","agent_id":"my-agent","conversation_id":"conv-001","timestamp":"2026-01-15T10:30:01Z","sequence_number":7,"data":{}}

event: turn_completed
data: {"event_id":"evt-008","event_type":"turn_completed","agent_id":"my-agent","conversation_id":"conv-001","timestamp":"2026-01-15T10:30:01Z","sequence_number":8,"data":{"input_tokens":50,"output_tokens":100,"model":"claude-sonnet-4-20250514","timestamp":"2026-01-15T10:30:01Z","turn_number":1,"cumulative_input_tokens":50,"cumulative_output_tokens":100}}
```

---

## Stream Behavior

### Connection Lifecycle

1. **Create conversation** → Returns `conversation_id` and starts SSE stream
2. **Connect to stream** → `GET /conversations/{conv_id}/stream`
3. **Send message** → `POST /conversations/{conv_id}/messages`
4. **Receive events** → Stream delivers events as they occur
5. **Turn completes** → `done` event sent
6. **Next message** → Send another message, continue from step 4
7. **End conversation** → `DELETE /conversations/{conv_id}` closes stream

### Buffering and Backpressure

The SSE stream has the following characteristics:

| Property | Value | Description |
|----------|-------|-------------|
| Event buffer | 256 events | Maximum unconsumed events buffered per conversation |
| Keepalive interval | 15 seconds | Ping sent to prevent timeout |
| Channel capacity | 256 | Internal mpsc channel buffer size |

If the client cannot keep up with the event rate:
- Events are buffered up to the limit
- When full, the sender will block until buffer space is available
- Slow consumers may cause the agent to pause processing

### When the Stream Ends

The SSE stream closes (and the connection terminates) when:

1. **Conversation deleted** — `DELETE /conversations/{conv_id}` is called
2. **Agent shutdown** — The bridge process is shutting down
3. **Max turns exceeded** — If `max_turns` is configured and reached
4. **Connection dropped** — Client disconnects

**Important:** The stream does **NOT** end after each turn. It remains open for the entire conversation lifetime.

### Reconnection Behavior

If the connection drops:

1. The client can reconnect by calling `GET /conversations/{conv_id}/stream` again
2. **Note:** Event replay is not currently supported — reconnection starts from the current point in time
3. Any events generated while disconnected are lost
4. The conversation remains active and can receive new messages

To check conversation state after reconnection:
- Send a new message to continue the conversation
- Check webhooks for event history (if configured)

---

## Handling in Browser

```javascript
class BridgeStream {
  constructor(conversationId, onEvent, onError) {
    this.conversationId = conversationId;
    this.onEvent = onEvent;
    this.onError = onError;
    this.eventSource = null;
    this.reconnectDelay = 1000;
    this.maxReconnectDelay = 30000;
    this.connect();
  }
  
  connect() {
    this.eventSource = new EventSource(
      `/conversations/${this.conversationId}/stream`
    );
    
    // Each event's data is a full BridgeEvent JSON object.
    // The SSE event name may differ from the internal event_type
    // (e.g., SSE "message_start" → event_type "response_started").
    
    // Message events
    this.eventSource.addEventListener('message_start', (e) => {
      const event = JSON.parse(e.data);
      this.onEvent('message_start', event.data, event);
    });
    
    this.eventSource.addEventListener('content_delta', (e) => {
      const event = JSON.parse(e.data);
      this.onEvent('content_delta', event.data, event);
    });
    
    this.eventSource.addEventListener('message_end', (e) => {
      const event = JSON.parse(e.data);
      this.onEvent('message_end', event.data, event);
    });
    
    this.eventSource.addEventListener('tool_call_start', (e) => {
      const event = JSON.parse(e.data);
      this.onEvent('tool_call_start', event.data, event);
    });
    
    this.eventSource.addEventListener('tool_call_result', (e) => {
      const event = JSON.parse(e.data);
      this.onEvent('tool_call_result', event.data, event);
    });
    
    this.eventSource.addEventListener('tool_approval_required', (e) => {
      const event = JSON.parse(e.data);
      this.onEvent('tool_approval_required', event.data, event);
    });
    
    this.eventSource.addEventListener('done', (e) => {
      const event = JSON.parse(e.data);
      this.onEvent('done', event.data, event);
      // Turn complete — UI can re-enable input
    });
    
    this.eventSource.addEventListener('error', (e) => {
      if (e.data) {
        const event = JSON.parse(e.data);
        this.onEvent('error', event.data, event);
      }
    });
    
    // Connection error / close
    this.eventSource.onerror = (err) => {
      console.error('SSE error:', err);
      if (this.onError) this.onError(err);
      // Auto-reconnect with exponential backoff
      this.reconnect();
    };
  }
  
  reconnect() {
    this.close();
    setTimeout(() => {
      this.connect();
      this.reconnectDelay = Math.min(this.reconnectDelay * 2, this.maxReconnectDelay);
    }, this.reconnectDelay);
  }
  
  close() {
    if (this.eventSource) {
      this.eventSource.close();
      this.eventSource = null;
    }
  }
}

// Usage — callback receives (sseEventName, eventData, fullBridgeEvent)
const stream = new BridgeStream('conv-123', (type, data, event) => {
  console.log(type, data);
  // event.event_id, event.sequence_number, etc. are also available
}, (err) => {
  console.error('Stream error:', err);
});
```

---

## See Also

- [Conversations API](conversations-api.md) — Streaming endpoint details
- [Webhooks](../core-concepts/webhooks.md) — Server-side event notifications
