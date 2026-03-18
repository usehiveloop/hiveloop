# Handling Webhooks

Receive and process events from Bridge.

---

## Setting Up Your Endpoint

Create an HTTP endpoint that accepts POST requests:

```javascript
app.post('/webhooks/bridge', express.json(), async (req, res) => {
  // Process the event
  await handleWebhook(req.body);
  
  // Acknowledge quickly
  res.json({ status: 'ok' });
});
```

Configure the URL in your agent:

```json
{
  "id": "my-agent",
  "webhook_url": "https://your-api.com/webhooks/bridge"
}
```

---

## Event Structure

Every webhook has this format:

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
      "content": "Hello!"
    }
  }
}
```

### Common Fields

| Field | Description |
|-------|-------------|
| `event_id` | Unique ID (use for deduplication) |
| `event_type` | What happened |
| `timestamp` | When it happened |
| `agent_id` | Which agent |
| `conversation_id` | Which conversation |
| `data` | Event-specific data |

---

## Saving to Database

The most common webhook handler saves events:

```javascript
async function handleWebhook(event) {
  await db.events.insert({
    event_id: event.event_id,
    type: event.event_type,
    agent_id: event.agent_id,
    conversation_id: event.conversation_id,
    data: event.data,
    created_at: event.timestamp
  });
}
```

---

## Common Event Handlers

### Save Messages

```javascript
if (event.event_type === 'conversation.message') {
  await db.messages.insert({
    conversation_id: event.conversation_id,
    role: event.data.message.role,
    content: event.data.message.content,
    created_at: event.timestamp
  });
}
```

### Track Token Usage

```javascript
if (event.event_type === 'tokens.used') {
  await db.usage.insert({
    conversation_id: event.conversation_id,
    agent_id: event.agent_id,
    input_tokens: event.data.input_tokens,
    output_tokens: event.data.output_tokens,
    cost: event.data.cost,
    timestamp: event.timestamp
  });
}
```

### Update Conversation Status

```javascript
if (event.event_type === 'conversation.ended') {
  await db.conversations.update(
    { id: event.conversation_id },
    { status: 'ended', ended_at: event.timestamp }
  );
}
```

---

## Verifying Signatures

Always verify webhooks came from Bridge:

```javascript
const crypto = require('crypto');

function verifyWebhook(payload, signature, secret) {
  const expected = crypto
    .createHmac('sha256', secret)
    .update(JSON.stringify(payload))
    .digest('hex');
  
  return signature === `sha256=${expected}`;
}

app.post('/webhooks/bridge', (req, res) => {
  const signature = req.headers['x-bridge-signature'];
  
  if (!verifyWebhook(req.body, signature, WEBHOOK_SECRET)) {
    return res.status(401).json({ error: 'Invalid signature' });
  }
  
  // Process webhook...
});
```

---

## Deduplication

Bridge may send the same event multiple times (retries). Use `event_id` to deduplicate:

```javascript
async function handleWebhook(event) {
  // Check if already processed
  const exists = await db.events.findOne({
    event_id: event.event_id
  });
  
  if (exists) {
    return; // Already handled
  }
  
  // Process and save
  await processEvent(event);
  await db.events.insert({ event_id: event.event_id });
}
```

---

## Quick Response

Respond within seconds to avoid retries:

```javascript
app.post('/webhooks/bridge', async (req, res) => {
  // Queue for processing
  await queue.add(req.body);
  
  // Respond immediately
  res.json({ status: 'ok' });
});

// Process asynchronously
queue.process(async (job) => {
  await handleWebhook(job.data);
});
```

---

## Error Handling

Return 200 for successful receipt, error codes for actual problems:

| Status | When to use |
|--------|-------------|
| 200 | Received and will process |
| 400 | Payload malformed (won't retry) |
| 401 | Invalid signature (won't retry) |
| 500 | Server error (will retry) |

---

## Retry Behavior

Bridge retries with exponential backoff:

| Attempt | Delay |
|---------|-------|
| 1 | Immediate |
| 2 | 1 second |
| 3 | 2 seconds |
| 4 | 4 seconds |
| 5+ | Doubles each time (max ~4 minutes) |

After ~10 retries, the event is dropped.

---

## Debugging Webhooks

### Log All Events

```javascript
app.post('/webhooks/bridge', (req, res) => {
  console.log('Webhook received:', req.body.event_type);
  console.log(JSON.stringify(req.body, null, 2));
  res.json({ status: 'ok' });
});
```

### Use Webhook.site

Test with a temporary endpoint:

```json
{
  "webhook_url": "https://webhook.site/your-unique-id"
}
```

### Check Bridge Logs

```bash
docker logs bridge 2>&1 | grep webhook
```

---

## Complete Example

```javascript
const express = require('express');
const crypto = require('crypto');

const app = express();
const WEBHOOK_SECRET = process.env.BRIDGE_WEBHOOK_SECRET;

app.post('/webhooks/bridge', express.json(), async (req, res) => {
  // Verify signature
  const signature = req.headers['x-bridge-signature'];
  const payload = JSON.stringify(req.body);
  
  const expected = crypto
    .createHmac('sha256', WEBHOOK_SECRET)
    .update(payload)
    .digest('hex');
  
  if (signature !== `sha256=${expected}`) {
    return res.status(401).json({ error: 'Invalid signature' });
  }
  
  const event = req.body;
  
  // Deduplicate
  if (await db.events.findOne({ event_id: event.event_id })) {
    return res.json({ status: 'already_processed' });
  }
  
  // Handle by type
  switch (event.event_type) {
    case 'conversation.message':
      await db.messages.insert({
        conversation_id: event.conversation_id,
        ...event.data.message
      });
      break;
      
    case 'tokens.used':
      await db.usage.insert({
        conversation_id: event.conversation_id,
        ...event.data
      });
      break;
  }
  
  res.json({ status: 'ok' });
});

app.listen(3000);
```

---

## See Also

- [Webhooks](../core-concepts/webhooks.md) — Event types reference
- [Webhook Security](../core-concepts/webhooks.md#verifying-webhooks) — Verification details
