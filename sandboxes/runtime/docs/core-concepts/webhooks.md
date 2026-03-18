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

| Event | When it fires |
|-------|---------------|
| `conversation.created` | New conversation started |
| `conversation.message` | New message in conversation |
| `conversation.ended` | Conversation ended |

### Message Events

| Event | When it fires |
|-------|---------------|
| `message.started` | Assistant started responding |
| `message.completed` | Assistant finished responding |
| `message.error` | Error during generation |

### Tool Events

| Event | When it fires |
|-------|---------------|
| `tool.called` | Tool was invoked |
| `tool.completed` | Tool finished executing |
| `tool.approval_requested` | Tool needs user approval |
| `tool.approval_resolved` | User approved/denied tool |

### Token Usage

| Event | When it fires |
|-------|---------------|
| `tokens.used` | After each turn with token count |

---

## Webhook Payload Format

```json
{
  "event_id": "evt-abc123",
  "event_type": "conversation.message",
  "timestamp": "2026-01-15T10:30:00Z",
  "agent_id": "my-agent",
  "conversation_id": "conv-def456",
  "data": {
    "message": {
      "role": "assistant",
      "content": "Hello! How can I help?"
    }
  }
}
```

### Common Fields

| Field | Description |
|-------|-------------|
| `event_id` | Unique ID for this event (use for deduplication) |
| `event_type` | What happened |
| `timestamp` | When it happened (ISO 8601) |
| `agent_id` | Which agent |
| `conversation_id` | Which conversation |
| `data` | Event-specific data |

---

## Verifying Webhooks

Bridge signs webhooks with HMAC-SHA256. Verify the signature to ensure the webhook came from Bridge:

### Python Example

```python
import hmac
import hashlib

def verify_webhook(payload: bytes, signature: str, secret: str) -> bool:
    expected = hmac.new(
        secret.encode(),
        payload,
        hashlib.sha256
    ).hexdigest()
    return hmac.compare_digest(f"sha256={expected}", signature)

# In your webhook handler:
signature = request.headers.get("X-Bridge-Signature")
if not verify_webhook(request.body, signature, WEBHOOK_SECRET):
    raise ValueError("Invalid signature")
```

### Node.js Example

```javascript
const crypto = require('crypto');

function verifyWebhook(payload, signature, secret) {
  const expected = crypto
    .createHmac('sha256', secret)
    .update(payload)
    .digest('hex');
  return signature === `sha256=${expected}`;
}
```

### Headers

| Header | Description |
|--------|-------------|
| `X-Bridge-Signature` | HMAC-SHA256 signature |
| `X-Bridge-Event-ID` | Same as `event_id` in body |
| `X-Bridge-Event-Type` | Same as `event_type` in body |

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

Use `event_id` to deduplicate:

```python
if await db.events.find_one({"event_id": event["event_id"]}):
    return  # Already processed

await process_event(event)
await db.events.insert_one({"event_id": event["event_id"]})
```

### Handle Retries

Bridge retries on these status codes:

- 408 (timeout)
- 429 (rate limit)
- 5xx (server errors)

It does NOT retry on:

- 2xx (success)
- 4xx (client errors, except 408/429)

---

## Retry Behavior

Bridge retries failed webhooks with exponential backoff:

1. Immediately
2. After 1 second
3. After 2 seconds
4. After 4 seconds
5. After 8 seconds
6. After 16 seconds
7. After 32 seconds
8. After 64 seconds
9. After 128 seconds
10. After 256 seconds (then stops)

Events that permanently fail are logged but not retried indefinitely.

---

## Common Patterns

### Saving Conversation History

```python
if event["event_type"] == "conversation.message":
    await db.messages.insert_one({
        "conversation_id": event["conversation_id"],
        "message": event["data"]["message"],
        "timestamp": event["timestamp"]
    })
```

### Tracking Token Usage

```python
if event["event_type"] == "tokens.used":
    await db.usage.insert_one({
        "agent_id": event["agent_id"],
        "input_tokens": event["data"]["input_tokens"],
        "output_tokens": event["data"]["output_tokens"],
        "cost": event["data"]["cost"]
    })
```

### Updating User Interface

```python
if event["event_type"] == "message.completed":
    await websocket.broadcast(event["conversation_id"], {
        "type": "message",
        "content": event["data"]["message"]["content"]
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

---

## See Also

- [Handling Webhooks](../control-plane/handling-webhooks.md) — Complete integration guide
- [API Reference](../api-reference/index.md) — Bridge API
