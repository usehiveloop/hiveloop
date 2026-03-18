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
| Message Received | `message_received` | User message received | `content: string` |
| Conversation Ended | `conversation_ended` | Conversation ended | `{}` |
| Conversation Compacted | `conversation_compacted` | History was summarized | `summary`, `messages_compacted`, `pre_compaction_tokens`, `post_compaction_tokens` |

### Response Events

| Event | Event Type (JSON) | When it fires | Data Fields |
|-------|-------------------|---------------|-------------|
| Response Started | `response_started` | Assistant started responding | `{}` |
| Response Chunk | `response_chunk` | Streaming chunk generated | `delta: string` |
| Response Completed | `response_completed` | Assistant finished responding | `input_tokens`, `output_tokens`, `full_response` |
| Turn Completed | `turn_completed` | Turn/stream completed | `{}` |

### Tool Events

| Event | Event Type (JSON) | When it fires | Data Fields |
|-------|-------------------|---------------|-------------|
| Tool Call Started | `tool_call_started` | Tool was invoked | `tool_name`, `arguments` |
| Tool Call Completed | `tool_call_completed` | Tool finished executing | `tool_name`, `result`, `is_error` |
| Tool Approval Required | `tool_approval_required` | Tool needs user approval | `request_id`, `tool_name`, `tool_call_id`, `arguments` |
| Tool Approval Resolved | `tool_approval_resolved` | User approved/denied tool | `request_id`, `decision` ("approve" or "deny") |

### Other Events

| Event | Event Type (JSON) | When it fires | Data Fields |
|-------|-------------------|---------------|-------------|
| Todo Updated | `todo_updated` | Todo list updated | `todos: array` |
| Agent Error | `agent_error` | Error occurred | `code`, `message` |

---

## Webhook Payload Format

```json
{
  "event_type": "response_completed",
  "timestamp": "2026-01-15T10:30:00Z",
  "agent_id": "my-agent",
  "conversation_id": "conv-def456",
  "data": {
    "input_tokens": 150,
    "output_tokens": 42,
    "full_response": "Hello! How can I help?"
  },
  "webhook_url": "https://api.yourservice.com/webhooks/bridge",
  "webhook_secret": "<webhook-secret>"
}
```

### Common Fields

| Field | Type | Description |
|-------|------|-------------|
| `event_type` | string | Event type in snake_case |
| `timestamp` | string | ISO 8601 timestamp (UTC) |
| `agent_id` | string | Agent identifier |
| `conversation_id` | string | Conversation identifier |
| `data` | object | Event-specific data (varies by event type) |
| `webhook_url` | string | Target webhook URL |
| `webhook_secret` | string | Secret used for signing |

**Note:** There is no `event_id` field. Use the combination of `timestamp`, `conversation_id`, and `event_type` for deduplication if needed.

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
```

---

## Handling Webhooks

### Acknowledge Quickly

Respond with 200 OK as soon as you receive the webhook. Bridge retries on failures, so slow responses cause duplicate processing.

```python
@app.post("/webhooks/bridge")
async def handle_webhook(request):
    # Queue for async processing
    await queue.put(request.json)
    return {"status": "ok"}  # Respond immediately
```

### Handle Duplicates

Bridge may send the same event multiple times during retries. Use a combination of fields to deduplicate:

```python
# Create a unique key from timestamp + conversation_id + event_type
event_key = f"{event['timestamp']}:{event['conversation_id']}:{event['event_type']}"

if await db.events.find_one({"event_key": event_key}):
    return  # Already processed

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

## Event Ordering

Bridge does **not** guarantee strict ordering of webhook deliveries. Events are sent as they occur, and retries can cause out-of-order delivery. Use the `timestamp` field to order events chronologically if needed.

---

## Common Patterns

### Saving Conversation History

```python
if event["event_type"] == "response_completed":
    await db.messages.insert_one({
        "conversation_id": event["conversation_id"],
        "response": event["data"]["full_response"],
        "input_tokens": event["data"]["input_tokens"],
        "output_tokens": event["data"]["output_tokens"],
        "timestamp": event["timestamp"]
    })
```

### Tracking Tool Usage

```python
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
DEBUG webhooks::dispatcher > Sending webhook to https://...
DEBUG webhooks::dispatcher > Webhook delivered: 200 OK
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

## See Also

- [Handling Webhooks](../control-plane/handling-webhooks.md) — Complete integration guide
- [API Reference](../api-reference/index.md) — Bridge API
