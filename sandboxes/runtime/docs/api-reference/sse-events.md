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
  const data = JSON.parse(e.data);
  console.log(data);
});

eventSource.addEventListener('content_delta', (e) => {
  const data = JSON.parse(e.data);
  console.log(data);
});
```

**Note:** The SSE stream can only be consumed by one client at a time. If another client is already connected, the endpoint will return a 404 error until the previous connection is closed.

---

## Event Format

Each event has an event type and JSON data:

```
event: {event_type}
data: {json_payload}
```

Multiple events can be sent in sequence:

```
event: message_start
data: {"type":"message_start","conversation_id":"conv-abc","message_id":"msg-001"}

event: content_delta
data: {"type":"content_delta","delta":"Hello","message_id":"msg-001"}

event: message_end
data: {"type":"message_end","message_id":"msg-001","usage":{"input_tokens":50,"output_tokens":10}}

event: done
data: {"type":"done"}
```

---

## Message Events

### `message_start`

A new assistant message has started.

```json
{
  "type": "message_start",
  "conversation_id": "conv-abc123",
  "message_id": "msg-001"
}
```

| Field | Type | Description |
|-------|------|-------------|
| `type` | string | Always `"message_start"` |
| `conversation_id` | string | The conversation ID |
| `message_id` | string | Provider-assigned message ID |

### `content_delta`

A chunk of text content from the assistant. Multiple deltas make up the full message.

```json
{
  "type": "content_delta",
  "delta": " chunk of text",
  "message_id": "msg-001"
}
```

| Field | Type | Description |
|-------|------|-------------|
| `type` | string | Always `"content_delta"` |
| `delta` | string | The text chunk |
| `message_id` | string | Provider-assigned message ID |

### `message_end`

The assistant message is complete.

```json
{
  "type": "message_end",
  "message_id": "msg-001",
  "usage": {
    "input_tokens": 150,
    "output_tokens": 75
  }
}
```

| Field | Type | Description |
|-------|------|-------------|
| `type` | string | Always `"message_end"` |
| `message_id` | string | Provider-assigned message ID |
| `usage` | object | Token usage for this response |
| `usage.input_tokens` | number | Number of input tokens consumed |
| `usage.output_tokens` | number | Number of output tokens generated |

**Note:** Unlike the LLM's native response, Bridge does not include a `finish_reason` field. The completion status is inferred from the stream ending normally or via the `done` event.

---

## Tool Events

### `tool_call_start`

The agent is calling a tool.

```json
{
  "type": "tool_call_start",
  "id": "call-123",
  "name": "read",
  "arguments": {
    "path": "/path/to/file"
  }
}
```

| Field | Type | Description |
|-------|------|-------------|
| `type` | string | Always `"tool_call_start"` |
| `id` | string | Tool call ID (correlates with `tool_call_result`) |
| `name` | string | Name of the tool being called |
| `arguments` | object | Arguments passed to the tool |

### `tool_call_result`

The tool has finished executing.

```json
{
  "type": "tool_call_result",
  "id": "call-123",
  "result": "file contents...",
  "is_error": false
}
```

| Field | Type | Description |
|-------|------|-------------|
| `type` | string | Always `"tool_call_result"` |
| `id` | string | Tool call ID (matches `tool_call_start`) |
| `result` | string | Result from the tool execution (JSON string or plain text) |
| `is_error` | boolean | Whether the tool execution resulted in an error |

### `tool_approval_required`

A tool requires user approval before running (when tool permission is set to `require_approval`).

```json
{
  "type": "tool_approval_required",
  "request_id": "req-456",
  "tool_name": "bash",
  "tool_call_id": "call-789",
  "arguments": {
    "command": "rm -rf /data"
  },
  "integration_name": null,
  "integration_action": null
}
```

| Field | Type | Description |
|-------|------|-------------|
| `type` | string | Always `"tool_approval_required"` |
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
  "type": "tool_approval_resolved",
  "request_id": "req-456",
  "decision": "approve"
}
```

| Field | Type | Description |
|-------|------|-------------|
| `type` | string | Always `"tool_approval_resolved"` |
| `request_id` | string | The approval request ID that was resolved |
| `decision` | string | Either `"approve"` or `"deny"` |

---

## Todo Events

### `todo_updated`

The todo list was updated via the `todowrite` tool.

```json
{
  "type": "todo_updated",
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
```

| Field | Type | Description |
|-------|------|-------------|
| `type` | string | Always `"todo_updated"` |
| `todos` | array | The complete current todo list |
| `todos[].content` | string | Brief description of the task |
| `todos[].status` | string | Current status: `pending`, `in_progress`, `completed`, or `cancelled` |
| `todos[].priority` | string | Priority level: `high`, `medium`, or `low` |

---

## Error Events

### `error`

Something went wrong during the conversation turn.

```json
{
  "type": "error",
  "code": "TOOL_EXECUTION_ERROR",
  "message": "Failed to read file: permission denied"
}
```

| Field | Type | Description |
|-------|------|-------------|
| `type` | string | Always `"error"` |
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
  "type": "done"
}
```

| Field | Type | Description |
|-------|------|-------------|
| `type` | string | Always `"done"` |

**Note:** After receiving `done`, the connection remains open for the next user message. The stream only ends when:
- The conversation is deleted via `DELETE /conversations/{conv_id}`
- The agent is shut down
- The connection is dropped

### `ping` (Keepalive)

Keepalive sent every 15 seconds to prevent connection timeout. This is an SSE comment, not a proper event:

```
:ping
```

Most SSE clients ignore comments automatically. No action is required.

---

## Complete Example

A full conversation turn:

```
event: message_start
data: {"type":"message_start","conversation_id":"conv-001","message_id":"msg-001"}

event: content_delta
data: {"type":"content_delta","delta":"I'll","message_id":"msg-001"}

event: content_delta
data: {"type":"content_delta","delta":" check","message_id":"msg-001"}

event: content_delta
data: {"type":"content_delta","delta":" the","message_id":"msg-001"}

event: content_delta
data: {"type":"content_delta","delta":" file","message_id":"msg-001"}

event: content_delta
data: {"type":"content_delta","delta":" for","message_id":"msg-001"}

event: content_delta
data: {"type":"content_delta","delta":" you.","message_id":"msg-001"}

event: tool_call_start
data: {"type":"tool_call_start","id":"call-001","name":"read","arguments":{"path":"README.md"}}

event: tool_call_result
data: {"type":"tool_call_result","id":"call-001","result":"{\"success\":true,\"content\":\"# Project...\"}","is_error":false}

event: content_delta
data: {"type":"content_delta","delta":"\n\nThe","message_id":"msg-001"}

event: content_delta
data: {"type":"content_delta","delta":" README","message_id":"msg-001"}

event: content_delta
data: {"type":"content_delta","delta":" says...","message_id":"msg-001"}

event: message_end
data: {"type":"message_end","message_id":"msg-001","usage":{"input_tokens":50,"output_tokens":100}}

event: done
data: {"type":"done"}
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
    
    // Message events
    this.eventSource.addEventListener('message_start', (e) => {
      this.onEvent('message_start', JSON.parse(e.data));
    });
    
    this.eventSource.addEventListener('content_delta', (e) => {
      this.onEvent('content_delta', JSON.parse(e.data));
    });
    
    this.eventSource.addEventListener('tool_call_start', (e) => {
      this.onEvent('tool_call_start', JSON.parse(e.data));
    });
    
    this.eventSource.addEventListener('tool_call_result', (e) => {
      this.onEvent('tool_call_result', JSON.parse(e.data));
    });
    
    this.eventSource.addEventListener('tool_approval_required', (e) => {
      this.onEvent('tool_approval_required', JSON.parse(e.data));
    });
    
    this.eventSource.addEventListener('done', (e) => {
      this.onEvent('done', JSON.parse(e.data));
      // Turn complete — UI can re-enable input
    });
    
    this.eventSource.addEventListener('error', (e) => {
      if (e.data) {
        this.onEvent('error', JSON.parse(e.data));
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

// Usage
const stream = new BridgeStream('conv-123', (type, data) => {
  console.log(type, data);
}, (err) => {
  console.error('Stream error:', err);
});
```

---

## See Also

- [Conversations API](conversations-api.md) — Streaming endpoint details
- [Webhooks](../core-concepts/webhooks.md) — Server-side event notifications
