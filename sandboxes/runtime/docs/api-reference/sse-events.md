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

eventSource.onmessage = (event) => {
  const data = JSON.parse(event.data);
  console.log(data);
};
```

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
data: {"message_id": "msg-001"}

event: content_delta
data: {"delta": "Hello"}

event: message_end
data: {}
```

---

## Message Events

### `message_start`

A new assistant message has started.

```json
{
  "message_id": "msg-abc123",
  "timestamp": "2026-01-15T10:30:00Z"
}
```

### `content_delta`

A chunk of text content. Multiple deltas make up the full message.

```json
{
  "delta": " chunk of text"
}
```

### `message_end`

The assistant message is complete.

```json
{
  "message_id": "msg-abc123",
  "finish_reason": "stop",
  "usage": {
    "input_tokens": 150,
    "output_tokens": 75
  }
}
```

`finish_reason` values:
- `stop` — Normal completion
- `max_tokens` — Hit token limit
- `tool_calls` — Stopped to call tools

---

## Tool Events

### `tool_call_start`

The agent is calling a tool.

```json
{
  "tool_call_id": "call-123",
  "tool_name": "read",
  "arguments": {
    "path": "/path/to/file"
  }
}
```

### `tool_call_result`

The tool has finished executing.

```json
{
  "tool_call_id": "call-123",
  "result": {
    "success": true,
    "content": "file contents..."
  }
}
```

### `tool_approval_required`

A tool needs user approval before running.

```json
{
  "request_id": "req-456",
  "tool_name": "bash",
  "arguments": {
    "command": "rm -rf /data"
  }
}
```

### `tool_approval_resolved`

User approved or denied a tool call.

```json
{
  "request_id": "req-456",
  "approved": true,
  "reason": "User confirmed"
}
```

---

## Todo Events

### `todo_created`

A new todo item was created.

```json
{
  "todo_id": "todo-789",
  "content": "Refactor auth module",
  "status": "pending"
}
```

### `todo_updated`

A todo item was updated.

```json
{
  "todo_id": "todo-789",
  "content": "Refactor auth module",
  "status": "in_progress"
}
```

---

## Error Events

### `error`

Something went wrong.

```json
{
  "code": "TOOL_EXECUTION_ERROR",
  "message": "Failed to read file: permission denied",
  "tool_call_id": "call-123"
}
```

Common error codes:
- `TOOL_EXECUTION_ERROR` — Tool failed
- `LLM_ERROR` — AI provider error
- `RATE_LIMITED` — Hit rate limit
- `CONTEXT_LENGTH_EXCEEDED` — Too many tokens

---

## Control Events

### `done`

The stream is ending normally.

```
event: done
data: {}
```

### `ping`

Keepalive to prevent connection timeout.

```
event: ping
data: {}
```

---

## Complete Example

A full conversation turn:

```
event: message_start
data: {"message_id": "msg-001"}

event: content_delta
data: {"delta": "I'll"}

event: content_delta
data: {"delta": " check"}

event: content_delta
data: {"delta": " the"}

event: content_delta
data: {"delta": " file"}

event: content_delta
data: {"delta": " for"}

event: content_delta
data: {"delta": " you."}

event: tool_call_start
data: {"tool_call_id": "call-001", "tool_name": "read", "arguments": {"path": "README.md"}}

event: tool_call_result
data: {"tool_call_id": "call-001", "result": {"success": true, "content": "# Project..."}}

event: content_delta
data: {"delta": "\n\nThe"}

event: content_delta
data: {"delta": " README"}

event: content_delta
data: {"delta": " says..."}

event: message_end
data: {"finish_reason": "stop", "usage": {"input_tokens": 50, "output_tokens": 100}}
```

---

## Reconnection

If the connection drops, reconnect with `Last-Event-ID`:

```bash
curl -N http://localhost:8080/conversations/conv-123/stream \
  -H "Last-Event-ID: evt-456" \
  -H "Accept: text/event-stream"
```

Bridge sends events after the specified ID.

---

## Handling in Browser

```javascript
class BridgeStream {
  constructor(conversationId, onEvent) {
    this.eventSource = new EventSource(
      `/conversations/${conversationId}/stream`
    );
    
    this.eventSource.addEventListener('message_start', (e) => {
      onEvent('message_start', JSON.parse(e.data));
    });
    
    this.eventSource.addEventListener('content_delta', (e) => {
      onEvent('content_delta', JSON.parse(e.data));
    });
    
    this.eventSource.addEventListener('tool_call_start', (e) => {
      onEvent('tool_call_start', JSON.parse(e.data));
    });
    
    this.eventSource.addEventListener('error', (e) => {
      onEvent('error', JSON.parse(e.data));
    });
  }
  
  close() {
    this.eventSource.close();
  }
}

// Usage
const stream = new BridgeStream('conv-123', (type, data) => {
  console.log(type, data);
});
```

---

## See Also

- [Conversations API](conversations-api.md) — Streaming endpoint
- [Webhooks](../core-concepts/webhooks.md) — Server-side events
