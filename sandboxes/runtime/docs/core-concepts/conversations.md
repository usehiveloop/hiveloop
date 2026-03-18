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

## Conversation Limits

### Turn Limits (`max_turns`)

Configure the maximum number of back-and-forth exchanges per conversation:

```json
{
  "config": {
    "max_turns": 50
  }
}
```

| Attribute | Value |
|-----------|-------|
| **Type** | `Option<u32>` |
| **Default** | `null` (unlimited) |
| **When exceeded** | Conversation ends with `max_turns_exceeded` error |

When `max_turns` is reached:
1. An `error` SSE event with code `max_turns_exceeded` is sent
2. A `turn_completed` event is sent  
3. The conversation ends gracefully
4. Webhook events are dispatched (if configured)

### Response Timeout

Each agent turn has a timeout to prevent hung conversations:

| Attribute | Value |
|-----------|-------|
| **Timeout** | 180 seconds (3 minutes) |
| **Applies to** | Each `agent.chat()` call including internal tool loops |
| **Exception** | Disabled when any tool has `require_approval` permission |
| **On timeout** | `agent_timeout` error is sent, turn ends |

### Empty Response Recovery

If the agent returns an empty response, Bridge attempts recovery:

| Attribute | Value |
|-----------|-------|
| **Max continuations** | 1 attempt with the main agent |
| **Fallback** | No-tools agent retry with enriched history |
| **Final fallback** | Static message: "I completed the requested tasks using the available tools." |

---

## Message History

Bridge maintains the full message history for each conversation. This history:

- Is sent to the AI provider on each turn
- Includes user messages and assistant responses
- Can include tool results (shown to the AI as context)

### History Compaction

Long conversations get expensive (more tokens = more cost, more latency). Compaction summarizes old history to save tokens:

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
      "token_budget": 100000,
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

### Compaction Configuration

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `token_budget` | `u32` | 100,000 | Trigger compaction when estimated tokens exceed this |
| `tail_messages` | `u32` | 10 | Keep this many recent messages untouched after compaction |
| `summary_provider` | `ProviderConfig` | **required** | Cheaper model to create the summary |
| `summary_prompt` | `Option<String>` | built-in | Custom system prompt for summarization |

### Token Estimation

Tokens are estimated using:
- **Encoder**: tiktoken `cl100k_base`
- **Overhead**: +4 tokens per message for framing
- **Content**: All message text, tool calls, and tool results

### Split Boundary Alignment

When compacting, the split point is adjusted so the tail starts at a **user message**, not in the middle of an assistant/tool-result exchange. This preserves conversation coherence.

### Compaction Webhook Event

When compaction occurs, a `conversation_compacted` webhook is dispatched with:

```json
{
  "summary": "User asked to refactor auth module...",
  "messages_compacted": 35,
  "pre_compaction_tokens": 120000,
  "post_compaction_tokens": 15000
}
```

---

## Subagent Depth Limits

When agents spawn subagents, depth is limited to prevent unbounded recursion:

| Attribute | Value |
|-----------|-------|
| **Maximum depth** | 3 levels |
| **Applies to** | `Agent`, `ParallelAgent`, `Bash` tools |
| **On exceed** | Error: "Maximum subagent depth (3) reached" |

The depth counter increments for each nested subagent call.

---

## Channel Capacities

Internal channels have bounded capacities for backpressure:

| Channel | Capacity | Purpose |
|---------|----------|---------|
| Message queue | 32 | User messages to conversation |
| SSE events | 256 | Streaming events to clients |
| Notifications | 64 | Background task completions |

When channels are full, backpressure applies to prevent memory exhaustion.

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
