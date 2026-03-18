# Conversations

A conversation is a single chat session between a user and an agent.

---

## What's in a Conversation?

```
Conversation: conv-abc123
├── Agent ID: greeter
├── User ID: user-456
├── Created: 2026-01-15T10:30:00Z
├── Messages: [
│   {"role": "user", "content": "Hello!"}
│   {"role": "assistant", "content": "Hi there! How can I help?"}
│   {"role": "user", "content": "What's the weather?"}
│   {"role": "assistant", "content": "..."}
│ ]
└── State: active
```

A conversation tracks:
- Who's talking (user ID)
- What was said (message history)
- Turn state (waiting for user, processing, etc.)
- Tool approval requests (pending permissions)

---

## Conversation Lifecycle

```
1. CREATED
   POST /agents/{id}/conversations
   Returns conversation_id
   ↓
2. USER_MESSAGE
   POST /conversations/{id}/messages
   User sends a message
   ↓
3. AGENT_TURN
   Agent processes the message
   May call tools, stream responses
   ↓
   (repeat USER_MESSAGE → AGENT_TURN as needed)
   ↓
4. ENDED
   DELETE /conversations/{id}
   Or max turns reached
```

---

## Creating a Conversation

```bash
curl -X POST http://localhost:8080/agents/greeter/conversations \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "user-123",
    "metadata": {
      "source": "web-chat",
      "campaign": "summer-sale"
    }
  }'
```

Response:

```json
{
  "conversation_id": "conv-abc123def456",
  "agent_id": "greeter",
  "created_at": "2026-01-15T10:30:00Z"
}
```

The `conversation_id` is used for all future operations on this conversation.

---

## Sending Messages

```bash
curl -X POST http://localhost:8080/conversations/conv-abc123/messages \
  -H "Content-Type: application/json" \
  -d '{
    "role": "user",
    "content": "What can you help me with?"
  }'
```

The message is queued for the agent. To see the response, connect to the stream.

---

## Streaming Responses

Connect to the SSE stream:

```bash
curl -N http://localhost:8080/conversations/conv-abc123/stream
```

Events flow in real time:

```
event: message_start
data: {"message_id": "msg-001"}

event: content_delta
data: {"delta": "I"}

event: content_delta
data: {"delta": " can"}

event: content_delta
data: {"delta": " help"}

event: message_end
data: {}
```

See [SSE Events](../api-reference/sse-events.md) for all event types.

---

## Conversation State

A conversation is always in one of these states:

| State | Meaning |
|-------|---------|
| `idle` | Waiting for user input |
| `processing` | Agent is generating a response |
| `waiting_for_approval` | Tool call needs user permission |
| `error` | Something went wrong |
| `ended` | Conversation is finished |

Check state via webhooks or by reconnecting to the stream.

---

## Message History

Bridge maintains the full message history for each conversation. This history:

- Is sent to the AI provider on each turn
- Includes user messages and assistant responses
- Can include tool results (shown to the AI as context)

### History Limits

Two limits control history:

1. **`max_turns`** — Maximum number of back-and-forth exchanges
   - Default: 50
   - When reached, the conversation ends

2. **`compaction`** — Summarize old history to save tokens
   - When token budget exceeded, old messages are summarized
   - Keeps recent messages (configured by `tail_messages`)

---

## Compaction Explained

Long conversations get expensive (more tokens = more cost, more latency). Compaction helps:

```
Before compaction (100 messages):
[user, assistant, user, assistant, ... 100 times]

After compaction:
[summary of first 90 messages, user, assistant, user, assistant]
```

Configure compaction per agent:

```json
{
  "config": {
    "compaction": {
      "token_budget": 80000,
      "tail_messages": 10,
      "summary_provider": {
        "provider_type": "anthropic",
        "model": "claude-haiku-4-5-20251001",
        "api_key": "<anthropic-api-key>"
      }
    }
  }
}
```

- **token_budget** — When history exceeds this, compact it
- **tail_messages** — Keep this many recent messages untouched
- **summary_provider** — Cheaper model to create the summary

---

## Hydrating Conversation History

If your control plane saves conversation history, you can restore it:

```bash
curl -X POST http://localhost:8080/push/agents/greeter/conversations \
  -H "Authorization: Bearer ..." \
  -H "Content-Type: application/json" \
  -d '{
    "conversation_id": "conv-abc123",
    "messages": [
      {"role": "user", "content": "Hello"},
      {"role": "assistant", "content": "Hi there!"}
    ],
    "user_id": "user-123",
    "metadata": {...}
  }'
```

This is useful when:
- Restoring after Bridge restarts
- Moving conversations between Bridge instances
- Implementing "continue conversation" after long gaps

See [Hydrating Conversations](../control-plane/hydrating-conversations.md).

---

## Ending Conversations

Delete a conversation to free up resources:

```bash
curl -X DELETE http://localhost:8080/conversations/conv-abc123
```

Or let them expire automatically (if you implement cleanup in your control plane).

---

## See Also

- [SSE Events](../api-reference/sse-events.md) — All streaming event types
- [Hydrating Conversations](../control-plane/hydrating-conversations.md) — Restore history
- [API Reference](../api-reference/conversations-api.md) — Complete API
