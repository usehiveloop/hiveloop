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

### Request fields

| Field | Type | Description |
|-------|------|-------------|
| `content` | string | The LLM-visible text. Optional when [`full_message`](#large-payloads-via-full_message) is also supplied. |
| `system_reminder` | string \| null | Optional — prepended to the user message wrapped in `<system-reminder>` tags. |
| `full_message` | string \| null | Optional — full payload spilled to disk so the agent can query it only when needed. See below. |

---

### Large payloads via `full_message`

When the full input to the agent is too big to send on every turn (stack traces, log dumps, file contents, transcripts, structured error reports), set `full_message` to the complete payload and put a shorter summary in `content`. Bridge will:

1. Write `full_message` to `{attachments_root}/{conversation_id}/{uuid}.txt` — typical `attachments_root` is `./.bridge-attachments` relative to the bridge process's working directory. Override with `BRIDGE_ATTACHMENTS_DIR`.
2. Append a `<system-reminder>` block to the content you sent, pointing the agent at the absolute path of the attachment and hinting which tools to use to inspect it. The tool hint adapts to the agent's registered tools:
   - `RipGrep` + `Read` present → recommends both
   - only `RipGrep` or only `Read` → recommends that one
   - `AstGrep` or `bash` fallback → uses those (avoiding `cat` to prevent re-inflating context)
   - no filesystem/search tool → explicitly tells the agent it cannot read the file and should treat the summary as authoritative
3. If `content` is empty or missing, bridge auto-generates a summary from the first ~500 bytes of `full_message` so the agent still sees something useful.

Failures (disk full, permission denied) are **never** surfaced as API errors — bridge logs a warning and delivers the message with just `content`. This makes `full_message` safe to add to any send-message call without worrying about availability of the attachment layer.

#### Example

```bash
curl -X POST http://localhost:8080/conversations/conv-abc123/messages \
  -H "Content-Type: application/json" \
  -d '{
    "content": "Please investigate the crash. Summary: null pointer in handler::auth::verify.",
    "full_message": "<... 300KB of stack trace, request headers, env dump ...>"
  }'
```

The agent's LLM input becomes:

```
Please investigate the crash. Summary: null pointer in handler::auth::verify.

<system-reminder>
The user's message was truncated because it was too long. The complete
original payload is saved to `/var/run/bridge/.bridge-attachments/conv-abc123/8f6…e2.txt`.
Use the `RipGrep` tool to search the file for specifics, or the `Read`
tool to open a specific byte/line range.
</system-reminder>
```

#### Cleanup

Attachment files are scoped to their conversation. When the conversation is deleted via `DELETE /conversations/{id}` (or ends through the normal lifecycle), bridge removes the `{conversation_id}/` directory beneath the attachments root. Failures during cleanup are logged and swallowed — the API response is not affected.

#### Observability

The `message_received` event now includes an `attachment_path` field. When `full_message` was not used, it's `null`; when an attachment was written, it carries the absolute path for clients that want to display a link or side-load the file.

```json
{
  "event_type": "message_received",
  "data": {
    "content": "… (the composed text including the system-reminder)",
    "attachment_path": "/var/run/bridge/.bridge-attachments/conv-abc123/8f6…e2.txt"
  }
}
```

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

### Immortal Mode (In-Place Compaction)

Long conversations get expensive (more tokens = more cost, more latency). Bridge's **immortal mode** keeps them running indefinitely by compacting the eligible head of history *in place* — replacing it with a single user message containing a structured summary derived from the messages it replaced. There is no separate LLM summarization call: compaction is pure code, deterministic, and free.

```
Before compaction (history above token budget):
[user, assistant, user, assistant, ... up to retention_window before tail]

After compaction:
[<structured summary of compacted slice>, retention tail (verbatim)]
```

Configure per agent:

```json
{
  "config": {
    "immortal": {
      "token_budget": 100000,
      "retention_window": 10,
      "eviction_window": 1.0,
      "expose_journal_tools": true
    }
  }
}
```

### `ImmortalConfig` Reference

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `token_budget` | `u32` | 100,000 | Trigger compaction when estimated history tokens exceed this |
| `retention_window` | `u32` | 0 | Number of most-recent messages preserved verbatim (never compacted) |
| `eviction_window` | `f64` | 1.0 | Maximum fraction (0.0–1.0) of total tokens eligible for compaction in any single pass |
| `expose_journal_tools` | `bool` | true | When true, register `journal_read` / `journal_write` for the agent's own use |

The system prompt and the very first user message are always preserved.

### Token Estimation

Tokens are estimated using:
- **Encoder**: tiktoken `cl100k_base`
- **Overhead**: +4 tokens per message for framing
- **Content**: All message text, tool calls, and tool results

### Compaction Hooks

A second compaction pass runs *inside* rig's tool loop (not just at the top of each bridge turn) so single-bridge-turn agents — where most wall-clock time is spent inside the LLM provider's tool loop — also compact when they cross the budget.

### Chain Events

Immortal-mode compaction emits webhook / SSE events on chain handoff:

| Event | When | Payload (selection) |
|-------|------|---------------------|
| `chain_started` | Just before a chain handoff begins | `chain_index`, `trigger_token_count` |
| `chain_completed` | After successful handoff | `chain_index`, `duration_ms`, `carry_forward_tokens`, `verified` |
| `chain_failed` | A chain-handoff attempt errored | `chain_index`, `error` (the conversation continues with oversized history) |
| `context_pressure_warning` | Once per turn when cumulative tool-output bytes exceed ~1.5× `token_budget` | `turn_count`, `cumulative_tool_bytes`, `token_budget` |

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
