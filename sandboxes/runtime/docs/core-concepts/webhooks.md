# Webhooks

Webhooks are how Bridge talks back to your control plane. When things happen — messages sent, tools called, conversations ending — Bridge sends events to your webhook URL.

---

## Why Webhooks?

Bridge doesn't store data permanently. It sends events to you so you can:

- Save conversation history
- Track token usage
- Monitor for errors
- Trigger side effects in your system

```
User sends message ──► Bridge ──► AI provider
                         │
                         ▼
                    Your webhook
                    (save to DB,
                     update UI,
                     log metrics)
```

---

## How Webhooks Work

1. You configure a `webhook_url` in your agent definition
2. When events happen, Bridge POSTs to that URL
3. Your server receives and processes the event
4. Bridge retries on failure (with backoff)

---

## Configuring Webhooks

Add a webhook URL to your agent:

```json
{
  "id": "my-agent",
  "webhook_url": "https://api.yourservice.com/webhooks/bridge",
  "webhook_secret": "<webhook-secret>",
  ...
}
```

Or set a global webhook URL for all agents:

```bash
export BRIDGE_WEBHOOK_URL="https://api.yourservice.com/webhooks/bridge"
```

### Webhook Secret

The secret is used to sign webhook payloads. Always set a secret in production so you can verify webhooks came from Bridge.

---

## Webhook Events

Bridge sends these event types:

### Conversation Events

| Event | Event Type (JSON) | When it fires | Data Fields |
|-------|-------------------|---------------|-------------|
| Conversation Created | `conversation_created` | New conversation started | `{}` |
| Message Received | `message_received` | User message received | `content`, `attachment_path` (null when no attachment) |
| Conversation Ended | `conversation_ended` | Conversation ended | `{}` |
| Chain Started | `chain_started` | Just before an immortal-mode chain handoff begins | `chain_index`, `trigger_token_count` |
| Chain Completed | `chain_completed` | Chain handoff finished successfully | `chain_index`, `duration_ms`, `carry_forward_tokens`, `verified` |
| Chain Failed | `chain_failed` | Chain handoff attempt errored | `chain_index`, `error` |
| Context Pressure Warning | `context_pressure_warning` | Cumulative tool-output bytes exceed ~1.5× immortal `token_budget` (once per turn) | `turn_count`, `cumulative_tool_bytes`, `token_budget` |

### Response Events

| Event | Event Type (JSON) | When it fires | Data Fields |
|-------|-------------------|---------------|-------------|
| Response Started | `response_started` | Assistant started responding | `conversation_id`, `message_id` |
| Response Chunk | `response_chunk` | Streaming chunk generated | `delta`, `message_id` |
| Response Completed | `response_completed` | Assistant finished responding | `message_id`, `input_tokens`, `output_tokens`, `model`, `timestamp`, `full_response` |
| Turn Completed | `turn_completed` | Turn/stream completed | `input_tokens`, `output_tokens`, `model`, `timestamp`, `turn_number`, `cumulative_input_tokens`, `cumulative_output_tokens` |

### Tool Events

| Event | Event Type (JSON) | When it fires | Data Fields |
|-------|-------------------|---------------|-------------|
| Tool Call Started | `tool_call_started` | Tool was invoked | `id`, `name`, `arguments` |
| Tool Call Completed | `tool_call_completed` | Tool finished executing | `id`, `tool_name`, `result`, `is_error`, `duration_ms` |
| Tool Approval Required | `tool_approval_required` | Tool needs user approval | `request_id`, `tool_name`, `tool_call_id`, `arguments`, `integration_name`, `integration_action` |
| Tool Approval Resolved | `tool_approval_resolved` | User approved/denied tool | `request_id`, `decision` (`"approve"` or `"deny"`) |

### Reasoning Events

| Event | Event Type (JSON) | When it fires | Data Fields |
|-------|-------------------|---------------|-------------|
| Reasoning Delta | `reasoning_delta` | Reasoning/thinking chunk from the model | `delta`, `message_id` |

### Subagent Events

| Event | Event Type (JSON) | When it fires | Data Fields |
|-------|-------------------|---------------|-------------|
| Subagent Started | `sub_agent_started` | A subagent was spawned | `subagent_name`, `mode`, `parent_conversation_id`, `depth` |
| Subagent Completed | `sub_agent_completed` | A subagent finished execution | `subagent_name`, `mode`, `task_id`, `duration_ms`, `is_error` |

### Chain Events (Immortal Conversations)

| Event | Event Type (JSON) | When it fires | Data Fields |
|-------|-------------------|---------------|-------------|
| Chain Started | `chain_started` | Context chain handoff beginning | `chain_index`, `reason`, `token_count` |
| Chain Completed | `chain_completed` | Context chain handoff finished | `chain_index`, `journal_entry_count`, `carry_forward_messages` |

### Other Events

| Event | Event Type (JSON) | When it fires | Data Fields |
|-------|-------------------|---------------|-------------|
| Todo Updated | `todo_updated` | Todo list updated | `todos` |
| Agent Error | `agent_error` | Error occurred | `code`, `message` |
| Background Task Completed | `background_task_completed` | Background task finished | `task_id`, `description`, `output`, `is_error` |
| Done | `done` | Response stream complete (terminal signal) | `{}` |

---

## Webhook Payload Format

The webhook body is always a **JSON array** of events. Even a single event is wrapped in an array. Events within a batch belong to the same conversation and are ordered by `sequence_number`.

```json
[
  {
    "event_id": "evt-abc123",
    "event_type": "response_started",
    "timestamp": "2026-01-15T10:30:00Z",
    "agent_id": "my-agent",
    "conversation_id": "conv-def456",
    "sequence_number": 1,
    "data": {
      "conversation_id": "conv-def456",
      "message_id": "msg-001"
    }
  },
  {
    "event_id": "evt-abc124",
    "event_type": "response_completed",
    "timestamp": "2026-01-15T10:30:02Z",
    "agent_id": "my-agent",
    "conversation_id": "conv-def456",
    "sequence_number": 2,
    "data": {
      "message_id": "msg-001",
      "input_tokens": 150,
      "output_tokens": 42,
      "model": "claude-sonnet-4-20250514",
      "timestamp": "2026-01-15T10:30:02Z",
      "full_response": "Hello! How can I help?"
    }
  }
]
```

**Note:** `webhook_url` and `webhook_secret` are never included in the event payload. They are resolved at delivery time by the webhook worker.

### Common Fields

| Field | Type | Description |
|-------|------|-------------|
| `event_id` | string | Globally unique event identifier |
| `event_type` | string | Event type in snake_case |
| `timestamp` | string | ISO 8601 timestamp (UTC) |
| `agent_id` | string | Agent identifier |
| `conversation_id` | string | Conversation identifier |
| `sequence_number` | integer | Global monotonically increasing counter |
| `data` | object | Event-specific data (varies by event type) |

Use `sequence_number` for ordering and deduplication. Sequence numbers are globally monotonic across all agents and conversations — use `conversation_id` to filter events for a specific conversation.

---

## Verifying Webhooks

Bridge signs webhooks with HMAC-SHA256. The signature is computed over the message format: `{timestamp}.{payload}` where timestamp is Unix seconds and payload is the raw JSON body.

### Headers

| Header | Description |
|--------|-------------|
| `X-Webhook-Signature` | Base64-encoded HMAC-SHA256 signature |
| `X-Webhook-Timestamp` | Unix timestamp (seconds) used for signing |

### Python Example

```python
import hmac
import hashlib
import base64

def verify_webhook(payload: bytes, signature: str, secret: str, timestamp: str) -> bool:
    """
    Verify webhook signature.
    
    Args:
        payload: Raw request body bytes
        signature: Value from X-Webhook-Signature header (base64)
        secret: Webhook secret
        timestamp: Value from X-Webhook-Timestamp header
    """
    # Message format: {timestamp}.{payload}
    message = f"{timestamp}.".encode() + payload
    
    # Compute expected signature
    expected = hmac.new(
        secret.encode(),
        message,
        hashlib.sha256
    ).digest()
    expected_b64 = base64.b64encode(expected).decode()
    
    # Constant-time comparison
    return hmac.compare_digest(expected_b64, signature)

# In your webhook handler:
signature = request.headers.get("X-Webhook-Signature")
timestamp = request.headers.get("X-Webhook-Timestamp")
if not verify_webhook(request.body, signature, WEBHOOK_SECRET, timestamp):
    raise ValueError("Invalid signature")

# Body is always a JSON array
events = json.loads(request.body)
for event in events:
    process_event(event)
```

### Node.js Example

```javascript
const crypto = require('crypto');

function verifyWebhook(payload, signature, secret, timestamp) {
  // Message format: {timestamp}.{payload}
  const message = timestamp + '.' + payload;

  const expected = crypto
    .createHmac('sha256', secret)
    .update(message)
    .digest('base64');

  return signature === expected;
}

// In your webhook handler:
const signature = req.headers['x-webhook-signature'];
const timestamp = req.headers['x-webhook-timestamp'];
const payload = req.body; // raw body bytes/string

if (!verifyWebhook(payload, signature, WEBHOOK_SECRET, timestamp)) {
  return res.status(401).json({ error: 'Invalid signature' });
}

// Body is always a JSON array
const events = JSON.parse(payload);
for (const event of events) {
  handleEvent(event);
}
```

---

## Handling Webhooks

### Acknowledge Quickly

Respond with 200 OK as soon as you receive the webhook. Bridge retries on failures, so slow responses cause duplicate processing.

```python
@app.post("/webhooks/bridge")
async def handle_webhook(request):
    events = await request.json()  # Always a JSON array
    for event in events:
        await queue.put(event)
    return {"status": "ok"}  # Respond immediately
```

### Handle Duplicates

Bridge may send the same batch multiple times during retries. Use `sequence_number` and `conversation_id` to deduplicate:

```python
for event in events:
    event_key = f"{event['conversation_id']}:{event['sequence_number']}"

    if await db.events.find_one({"event_key": event_key}):
        continue  # Already processed

    await process_event(event)
    await db.events.insert_one({"event_key": event_key})
```

### Handle Retries

Bridge retries on these status codes:

- 408 (timeout)
- 429 (rate limit)
- 5xx (server errors)

It does NOT retry on:

- 2xx (success)
- 4xx (client errors)

---

## Retry Behavior

Bridge retries failed webhooks with exponential backoff and jitter:

| Attempt | Delay |
|---------|-------|
| 1 | Immediate |
| 2 | ~1 second (with jitter) |
| 3 | ~2 seconds (with jitter) |
| 4 | ~4 seconds (with jitter) |
| 5 | ~8 seconds (with jitter) |

**Configuration:**
- **Max retries**: 5 attempts total
- **Request timeout**: 10 seconds
- **Backoff type**: Exponential with random jitter

Events that permanently fail (all 5 attempts exhausted) are logged and dropped. There is no dead letter queue.

---

## Event Ordering and Batching

Bridge guarantees **strict in-order delivery per conversation**. Events for the same conversation are always delivered sequentially, never concurrently. Each event carries a `sequence_number` that increases monotonically (1, 2, 3, ...) within a conversation.

When multiple events queue up for the same conversation, Bridge batches them into a single HTTP POST. The body is always a JSON array, ordered by `sequence_number`. A batch may contain one or more events, but all events in a batch belong to the same conversation.

Events for **different conversations** are delivered concurrently and independently.

---

## Common Patterns

### Saving Conversation History

```python
for event in events:
    if event["event_type"] == "response_completed":
        await db.messages.insert_one({
            "conversation_id": event["conversation_id"],
            "sequence_number": event["sequence_number"],
            "response": event["data"]["full_response"],
            "input_tokens": event["data"]["input_tokens"],
            "output_tokens": event["data"]["output_tokens"],
            "timestamp": event["timestamp"]
        })
```

### Tracking Tool Usage

```python
for event in events:
    if event["event_type"] == "tool_call_completed":
        await db.tool_calls.insert_one({
            "conversation_id": event["conversation_id"],
            "tool_name": event["data"]["tool_name"],
            "result": event["data"]["result"],
            "is_error": event["data"]["is_error"],
            "timestamp": event["timestamp"]
        })
```

### Handling Errors

```python
for event in events:
    if event["event_type"] == "agent_error":
        await db.errors.insert_one({
            "conversation_id": event["conversation_id"],
            "code": event["data"]["code"],
            "message": event["data"]["message"],
            "timestamp": event["timestamp"]
        })
```

### Updating User Interface

```python
for event in events:
    if event["event_type"] == "response_chunk":
        await websocket.broadcast(event["conversation_id"], {
            "type": "chunk",
            "delta": event["data"]["delta"]
        })
```

---

## Debugging Webhooks

### Check Delivery

Enable debug logging:

```bash
export BRIDGE_LOG_LEVEL=debug
```

Look for:

```
DEBUG webhooks::dispatcher > conversation delivery worker started
DEBUG webhooks::dispatcher > delivering webhook batch, batch_size=3
INFO  webhooks::dispatcher > webhook batch delivered, status=200
```

### Test Your Endpoint

Use webhook.site or similar to inspect payloads:

```json
{
  "webhook_url": "https://webhook.site/your-unique-url"
}
```

### Common Issues

| Issue | Fix |
|-------|-----|
| 404 errors | Check the URL is correct |
| Timeout | Respond 200 immediately, process async |
| SSL errors | Use valid certificates |
| Missing signatures | Set `webhook_secret` in agent config |
| Signature verification fails | Ensure you're using `{timestamp}.{payload}` format with base64 encoding |

---

## WebSocket Alternative

For high-throughput control planes, Bridge also supports a **WebSocket event stream** that delivers all events over a single persistent connection — eliminating per-event HTTP overhead, HMAC signing, and connection churn.

Enable it with:

```bash
export BRIDGE_WEBSOCKET_ENABLED="true"
```

Then connect to `ws://<bridge>/ws/events?token=<api_key>`. The WebSocket delivers the same event types as webhooks, with a global monotonic `sequence_number` for ordering.

You can use webhooks, WebSocket, or both simultaneously. See the [WebSocket Events](../api-reference/websocket-events.md) reference for details.

---

## See Also

- [Handling Webhooks](../control-plane/handling-webhooks.md) — Complete integration guide
- [WebSocket Events](../api-reference/websocket-events.md) — WebSocket event stream reference
- [API Reference](../api-reference/index.md) — Bridge API
