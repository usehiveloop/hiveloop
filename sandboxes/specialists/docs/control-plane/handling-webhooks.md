# Handling Webhooks

Receive and process events from Bridge.

---

## Setting Up Your Endpoint

Create an HTTP endpoint that accepts POST requests. The body is always a **JSON array** of events, ordered by `sequence_number`. Events in a batch always belong to the same conversation.

```javascript
app.post('/webhooks/bridge', express.raw({ type: 'application/json' }), async (req, res) => {
  // Verify signature before processing
  const signature = req.headers['x-webhook-signature'];
  const timestamp = req.headers['x-webhook-timestamp'];

  if (!verifyWebhook(req.body, signature, WEBHOOK_SECRET, timestamp)) {
    return res.status(401).json({ error: 'Invalid signature' });
  }

  // Parse the event batch and process each event
  const events = JSON.parse(req.body);
  for (const event of events) {
    await queue.add(event);
  }

  // Acknowledge quickly
  res.json({ status: 'ok' });
});
```

Configure the URL in your agent:

```json
{
  "id": "my-agent",
  "webhook_url": "https://your-api.com/webhooks/bridge",
  "webhook_secret": "<webhook-secret>"
}
```

---

## Event Structure

The webhook body is always a **JSON array** of events. Each event has this format:

```json
[
  {
    "event_type": "response_completed",
    "timestamp": "2026-01-15T10:30:00Z",
    "agent_id": "my-agent",
    "conversation_id": "conv-def456",
    "sequence_number": 3,
    "data": {
      "input_tokens": 150,
      "output_tokens": 42,
      "full_response": "Hello! How can I help?"
    },
    "webhook_url": "https://your-api.com/webhooks/bridge",
    "webhook_secret": "<webhook-secret>"
  }
]
```

### Common Fields

| Field | Type | Description |
|-------|------|-------------|
| `event_type` | string | Event type in snake_case (e.g., `message_received`, `tool_call_completed`) |
| `timestamp` | string | ISO 8601 timestamp in UTC |
| `agent_id` | string | Which agent triggered the event |
| `conversation_id` | string | Which conversation |
| `sequence_number` | integer | Monotonically increasing per conversation (starts at 1) |
| `data` | object | Event-specific data (see below) |

Use `sequence_number` with `conversation_id` for deduplication and ordering.

---

## Event Types Reference

### Conversation Events

#### `conversation_created`
```json
{
  "event_type": "conversation_created",
  "data": {}
}
```

#### `message_received`
```json
{
  "event_type": "message_received",
  "data": {
    "content": "Hello, can you help me?"
  }
}
```

#### `conversation_ended`
```json
{
  "event_type": "conversation_ended",
  "data": {}
}
```

#### `chain_started` / `chain_completed` / `chain_failed`
Immortal-mode (`config.immortal`) compacts conversation history in place. The previous `conversation_compacted` event has been removed; subscribe to the chain events instead. See [SSE Events → Conversation / Chain Events](../api-reference/sse-events.md#conversation--chain-events) for full payloads.

```json
{
  "event_type": "chain_started",
  "data": {
    "chain_index": 1,
    "trigger_token_count": 105432
  }
}
```

```json
{
  "event_type": "chain_completed",
  "data": {
    "chain_index": 1,
    "duration_ms": 8123,
    "carry_forward_tokens": 24576,
    "verified": false
  }
}
```

#### `context_pressure_warning`
Fires once per turn when cumulative tool-output bytes exceed ~1.5× `immortal.token_budget`.

```json
{
  "event_type": "context_pressure_warning",
  "data": {
    "turn_count": 42,
    "cumulative_tool_bytes": 1572864,
    "token_budget": 100000
  }
}
```

### Response Events

#### `response_started`
Sent when the assistant begins generating a response.
```json
{
  "event_type": "response_started",
  "data": {}
}
```

#### `response_chunk`
Sent for each streaming chunk (if streaming is enabled).
```json
{
  "event_type": "response_chunk",
  "data": {
    "delta": "partial text"
  }
}
```

#### `response_completed`
Sent when the assistant finishes responding.
```json
{
  "event_type": "response_completed",
  "data": {
    "input_tokens": 150,
    "output_tokens": 42,
    "full_response": "Complete response text"
  }
}
```

#### `turn_completed`
Sent at the end of each turn (after `response_completed` or errors).
```json
{
  "event_type": "turn_completed",
  "data": {}
}
```

### Tool Events

#### `tool_call_started`
```json
{
  "event_type": "tool_call_started",
  "data": {
    "tool_name": "bash",
    "arguments": { "command": "ls -la" }
  }
}
```

#### `tool_call_completed`
```json
{
  "event_type": "tool_call_completed",
  "data": {
    "tool_name": "bash",
    "result": "file1.txt file2.txt",
    "is_error": false
  }
}
```

#### `tool_approval_required`
Sent when a tool with `require_approval` permission is called.
```json
{
  "event_type": "tool_approval_required",
  "data": {
    "request_id": "req-abc123",
    "tool_name": "bash",
    "tool_call_id": "call_123",
    "arguments": { "command": "rm -rf /" }
  }
}
```

#### `tool_approval_resolved`
Sent when a tool approval is approved or denied.
```json
{
  "event_type": "tool_approval_resolved",
  "data": {
    "request_id": "req-abc123",
    "decision": "approve"
  }
}
```

### Other Events

#### `todo_updated`
Sent when the todo list is modified via the `todowrite` tool.
```json
{
  "event_type": "todo_updated",
  "data": {
    "todos": [
      { "id": "1", "content": "Task 1", "status": "in_progress", "priority": "high" }
    ]
  }
}
```

#### `agent_error`
Sent when an error occurs during agent execution.
```json
{
  "event_type": "agent_error",
  "data": {
    "code": "agent_timeout",
    "message": "agent chat timed out after 180s"
  }
}
```

Common error codes:
- `max_turns_exceeded` — Maximum number of turns reached
- `aborted` — Turn was aborted by user
- `agent_timeout` — Agent response timed out
- `agent_error` — General agent error

---

## Saving to Database

The most common webhook handler saves events:

```javascript
async function handleWebhookBatch(events) {
  for (const event of events) {
    // Create unique key from conversation + sequence number
    const eventKey = `${event.conversation_id}:${event.sequence_number}`;

    // Check for duplicates
    const exists = await db.events.findOne({ event_key: eventKey });
    if (exists) continue;

    // Save the event
    await db.events.insert({
      event_key: eventKey,
      type: event.event_type,
      agent_id: event.agent_id,
      conversation_id: event.conversation_id,
      sequence_number: event.sequence_number,
      data: event.data,
      created_at: event.timestamp
    });
  }
}
```

---

## Common Event Handlers

### Save Messages

```javascript
for (const event of events) {
  if (event.event_type === 'response_completed') {
    await db.messages.insert({
      conversation_id: event.conversation_id,
      sequence_number: event.sequence_number,
      role: 'assistant',
      content: event.data.full_response,
      input_tokens: event.data.input_tokens,
      output_tokens: event.data.output_tokens,
      created_at: event.timestamp
    });
  }
}
```

### Track Tool Usage

```javascript
for (const event of events) {
  if (event.event_type === 'tool_call_completed') {
    await db.tool_calls.insert({
      conversation_id: event.conversation_id,
      agent_id: event.agent_id,
      tool_name: event.data.tool_name,
      result: event.data.result,
      is_error: event.data.is_error,
      timestamp: event.timestamp
    });
  }
}
```

### Update Conversation Status

```javascript
for (const event of events) {
  if (event.event_type === 'conversation_ended') {
    await db.conversations.update(
      { id: event.conversation_id },
      { status: 'ended', ended_at: event.timestamp }
    );
  }
}
```

### Handle Streaming Chunks

```javascript
for (const event of events) {
  if (event.event_type === 'response_chunk') {
    // Stream to connected clients via WebSocket
    await websocket.broadcast(event.conversation_id, {
      type: 'chunk',
      delta: event.data.delta
    });
  }
}
```

---

## Verifying Signatures

**Critical:** Always verify webhooks came from Bridge. The signature is computed over the full JSON array body:

```
HMAC-SHA256("{timestamp}.{payload}", secret)
```

The result is **base64-encoded** (not hex). The `payload` is the raw request body (the entire JSON array).

### JavaScript/Node.js

```javascript
const crypto = require('crypto');

function verifyWebhook(payload, signature, secret, timestamp) {
  // payload should be the raw body (Buffer or string)
  const message = timestamp + '.' + payload;

  const expected = crypto
    .createHmac('sha256', secret)
    .update(message)
    .digest('base64');

  // Use timing-safe comparison
  return crypto.timingSafeEqual(
    Buffer.from(signature),
    Buffer.from(expected)
  );
}

app.post('/webhooks/bridge', express.raw({ type: 'application/json' }), (req, res) => {
  const signature = req.headers['x-webhook-signature'];
  const timestamp = req.headers['x-webhook-timestamp'];

  if (!verifyWebhook(req.body, signature, WEBHOOK_SECRET, timestamp)) {
    return res.status(401).json({ error: 'Invalid signature' });
  }

  // Body is always a JSON array of events
  const events = JSON.parse(req.body);
  for (const event of events) {
    handleEvent(event);
  }

  res.json({ status: 'ok' });
});
```

### Python

```python
import hmac
import hashlib
import base64

def verify_webhook(payload: bytes, signature: str, secret: str, timestamp: str) -> bool:
    # Message format: {timestamp}.{payload}
    message = f"{timestamp}.".encode() + payload

    expected = hmac.new(
        secret.encode(),
        message,
        hashlib.sha256
    ).digest()
    expected_b64 = base64.b64encode(expected).decode()

    return hmac.compare_digest(expected_b64, signature)

@app.post('/webhooks/bridge')
async def handle_webhook(request):
    signature = request.headers.get('X-Webhook-Signature')
    timestamp = request.headers.get('X-Webhook-Timestamp')
    body = await request.body()

    if not verify_webhook(body, signature, WEBHOOK_SECRET, timestamp):
        raise HTTPException(status_code=401, detail='Invalid signature')

    events = json.loads(body)  # Always a JSON array
    for event in events:
        await process_event(event)
    return {'status': 'ok'}
```

---

## Deduplication

Bridge may send the same batch multiple times during retries. Use `conversation_id` + `sequence_number` as a unique key:

```javascript
async function handleWebhookBatch(events) {
  for (const event of events) {
    const eventKey = `${event.conversation_id}:${event.sequence_number}`;

    const exists = await db.events.findOne({ event_key: eventKey });
    if (exists) {
      continue; // Already handled
    }

    await processEvent(event);
    await db.events.insert({ event_key: eventKey, processed_at: new Date() });
  }
}
```

The `sequence_number` is unique per conversation and monotonically increasing, making it a reliable deduplication key.

---

## Quick Response

Respond within 10 seconds (the request timeout) to avoid retries:

```javascript
app.post('/webhooks/bridge', express.raw({ type: 'application/json' }), async (req, res) => {
  // Verify signature first
  const signature = req.headers['x-webhook-signature'];
  const timestamp = req.headers['x-webhook-timestamp'];

  if (!verifyWebhook(req.body, signature, WEBHOOK_SECRET, timestamp)) {
    return res.status(401).json({ error: 'Invalid signature' });
  }

  // Queue each event for processing (don't await the actual work)
  const events = JSON.parse(req.body);
  for (const event of events) {
    queue.add(event); // Fire and forget
  }

  // Respond immediately
  res.json({ status: 'ok' });
});

// Process asynchronously
queue.process(async (job) => {
  await handleEvent(job.data);
});
```

---

## Error Handling

Return appropriate status codes:

| Status | When to use | Will Retry? |
|--------|-------------|-------------|
| 200 | Received and will process | No |
| 401 | Invalid signature | No |
| 400 | Payload malformed | No |
| 500 | Server error | Yes |
| 503 | Service unavailable | Yes |

**Important:** Return 401 for invalid signatures. Don't retry authentication failures.

---

## Retry Behavior

Bridge retries with exponential backoff and jitter:

| Attempt | Delay (approximate) |
|---------|---------------------|
| 1 | Immediate |
| 2 | ~1 second |
| 3 | ~2 seconds |
| 4 | ~4 seconds |
| 5 | ~8 seconds |

**Configuration:**
- **Maximum attempts**: 5 total (1 initial + 4 retries)
- **Request timeout**: 10 seconds per attempt
- **Backoff**: Exponential with random jitter
- **Retry conditions**: 5xx server errors, 408 timeout, 429 rate limit

Events that fail all attempts are logged and dropped. There is no dead letter queue.

---

## Event Ordering and Batching

Bridge guarantees **strict in-order delivery per conversation**. Events for the same conversation are delivered sequentially, never concurrently. Each event carries a `sequence_number` that increases monotonically (1, 2, 3, ...) within a conversation.

When multiple events queue up for the same conversation, Bridge batches them into a single HTTP POST. The body is always a JSON array ordered by `sequence_number`:

```javascript
// Events within a batch are already in order — just iterate
for (const event of events) {
  console.log(`${event.conversation_id} seq=${event.sequence_number} type=${event.event_type}`);
}
```

Events for **different conversations** are delivered concurrently and independently, each with their own sequence numbering starting at 1.

---

## Debugging Webhooks

### Log All Events

```javascript
app.post('/webhooks/bridge', express.raw({ type: 'application/json' }), (req, res) => {
  const events = JSON.parse(req.body);
  console.log(`Webhook batch received: ${events.length} event(s)`);
  for (const event of events) {
    console.log(`  [${event.conversation_id} seq=${event.sequence_number}] ${event.event_type}`);
  }
  res.json({ status: 'ok' });
});
```

### Use Webhook.site

Test with a temporary endpoint:

```json
{
  "webhook_url": "https://webhook.site/your-unique-id",
  "webhook_secret": "<webhook-secret>"
}
```

### Check Bridge Logs

```bash
docker logs bridge 2>&1 | grep webhook
```

Look for:
- `DEBUG webhooks::dispatcher > conversation delivery worker started`
- `DEBUG webhooks::dispatcher > delivering webhook batch`
- `INFO  webhooks::dispatcher > webhook batch delivered`
- `ERROR webhooks::dispatcher > webhook batch delivery failed after all retries`

---

## Complete Example

```javascript
const express = require('express');
const crypto = require('crypto');

const app = express();
const WEBHOOK_SECRET = process.env.BRIDGE_WEBHOOK_SECRET;

// Raw body parser for signature verification
app.post('/webhooks/bridge', express.raw({ type: 'application/json' }), async (req, res) => {
  // Verify signature
  const signature = req.headers['x-webhook-signature'];
  const timestamp = req.headers['x-webhook-timestamp'];

  const message = timestamp + '.' + req.body;
  const expected = crypto
    .createHmac('sha256', WEBHOOK_SECRET)
    .update(message)
    .digest('base64');

  if (signature !== expected) {
    return res.status(401).json({ error: 'Invalid signature' });
  }

  // Body is always a JSON array of events (batched per conversation)
  const events = JSON.parse(req.body);

  for (const event of events) {
    // Deduplicate using conversation_id + sequence_number
    const eventKey = `${event.conversation_id}:${event.sequence_number}`;
    if (await db.events.findOne({ event_key: eventKey })) {
      continue;
    }

    // Handle by type
    switch (event.event_type) {
      case 'response_completed':
        await db.messages.insert({
          conversation_id: event.conversation_id,
          sequence_number: event.sequence_number,
          role: 'assistant',
          content: event.data.full_response,
          tokens: event.data.output_tokens,
          created_at: event.timestamp
        });
        break;

      case 'tool_call_completed':
        await db.tool_calls.insert({
          conversation_id: event.conversation_id,
          tool_name: event.data.tool_name,
          result: event.data.result,
          is_error: event.data.is_error,
          timestamp: event.timestamp
        });
        break;

      case 'agent_error':
        await db.errors.insert({
          conversation_id: event.conversation_id,
          code: event.data.code,
          message: event.data.message,
          timestamp: event.timestamp
        });
        break;
    }

    // Mark as processed
    await db.events.insert({ event_key: eventKey, processed_at: new Date() });
  }

  res.json({ status: 'ok' });
});

app.listen(3000);
```

---

## See Also

- [Webhooks](../core-concepts/webhooks.md) — Event types reference
- [API Reference](../api-reference/index.md) — Bridge API
