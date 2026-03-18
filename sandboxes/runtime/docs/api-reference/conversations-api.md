# Conversations API

Create conversations, send messages, and end conversations.

---

## Create Conversation

Start a new conversation with an agent.

### Request

```
POST /agents/{agent_id}/conversations
```

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
