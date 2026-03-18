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

```json
{
  "user_id": "user-123",
  "metadata": {
    "source": "web-chat",
    "campaign": "summer-sale"
  }
}
```

### Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `user_id` | string | No | Identifier for the user |
| `metadata` | object | No | Arbitrary data to associate with conversation |

### Response

```json
{
  "conversation_id": "conv-abc123def456",
  "agent_id": "greeter",
  "created_at": "2026-01-15T10:30:00Z"
}
```

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `conversation_id` | string | Unique ID for this conversation |
| `agent_id` | string | Which agent is handling this |
| `created_at` | string | When created (ISO 8601) |

### Errors

| Status | Code | Meaning |
|--------|------|---------|
| 404 | `AGENT_NOT_FOUND` | Agent doesn't exist |

### Example

```bash
curl -X POST http://localhost:8080/agents/greeter/conversations \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "user-123",
    "metadata": {"source": "web-chat"}
  }'
```

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
  "role": "user",
  "content": "Hello, how are you?"
}
```

### Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `role` | string | Yes | `user` or `system` |
| `content` | string | Yes | Message content |

### Response

```json
{
  "message_id": "msg-789xyz",
  "status": "queued"
}
```

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `message_id` | string | Unique ID for this message |
| `status` | string | Always `queued` |

### Errors

| Status | Code | Meaning |
|--------|------|---------|
| 404 | `CONVERSATION_NOT_FOUND` | Conversation doesn't exist |
| 400 | `INVALID_ROLE` | Role must be `user` or `system` |

### Example

```bash
curl -X POST http://localhost:8080/conversations/conv-abc123/messages \
  -H "Content-Type: application/json" \
  -d '{
    "role": "user",
    "content": "What can you help me with?"
  }'
```

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

Server-Sent Events stream:

```
event: message_start
data: {"message_id": "msg-001"}

event: content_delta
data: {"delta": "Hello"}

event: content_delta  
data: {"delta": " there"}

event: tool_call_start
data: {"tool_call_id": "call-123", "tool_name": "read"}

event: tool_call_result
data: {"tool_call_id": "call-123", "result": "..."}

event: message_end
data: {"finish_reason": "stop"}
```

### Event Types

See [SSE Events](sse-events.md) for complete list.

### Reconnection

If the connection drops, reconnect with `Last-Event-ID` header:

```bash
curl -N http://localhost:8080/conversations/conv-abc123/stream \
  -H "Last-Event-ID: evt-456"
```

You'll receive events after the specified ID.

### Errors

| Status | Code | Meaning |
|--------|------|---------|
| 404 | `CONVERSATION_NOT_FOUND` | Conversation doesn't exist |
| 409 | `ALREADY_STREAMING` | Another client is already connected |

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

Useful for "stop generating" buttons in UIs.

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
  "status": "deleted"
}
```

### Errors

| Status | Code | Meaning |
|--------|------|---------|
| 404 | `CONVERSATION_NOT_FOUND` | Conversation doesn't exist |

### Example

```bash
curl -X DELETE http://localhost:8080/conversations/conv-abc123
```

---

## Conversation State

Conversations have these states:

| State | Description |
|-------|-------------|
| `idle` | Waiting for user input |
| `processing` | Agent is generating |
| `waiting_for_approval` | Tool needs approval |
| `error` | An error occurred |
| `ended` | Conversation finished |

Check state by reconnecting to the stream or handling webhook events.

---

## See Also

- [SSE Events](sse-events.md) — All streaming event types
- [Agents API](agents-api.md) — Tool approvals
- [Webhooks](../core-concepts/webhooks.md) — Event notifications
