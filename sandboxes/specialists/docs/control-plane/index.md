# Control Plane Integration

This section is for building the control plane — the system that owns agent definitions and talks to Bridge.

---

## What is a Control Plane?

The control plane is your backend. It:

- Stores agent definitions
- Pushes them to Bridge
- Receives webhook events
- Manages conversation history
- Handles business logic

```
┌──────────────┐      Push agents       ┌─────────┐
│  Your Backend │ ─────────────────────► │ Bridge  │
│ (Control Plane)│                      │         │
└──────────────┘◄───────────────────────┴─────────┘
      ▲              Webhook events
      │
      ▼
┌──────────────┐
│   Your DB    │ ◄── Stores history, metrics
└──────────────┘
```

---

## Integration Flow

### 1. Push Agents on Startup

When your backend starts, push all agent definitions:

```javascript
await fetch('http://bridge/push/agents', {
  method: 'POST',
  headers: { 'Authorization': 'Bearer ...' },
  body: JSON.stringify({ agents: [...] })
});
```

### 2. Handle Webhooks

Receive events from Bridge:

```javascript
app.post('/webhooks/bridge', (req, res) => {
  // Save to database
  await db.events.insert(req.body);
  res.json({ status: 'ok' });
});
```

### 3. Proxy Frontend Requests

Your frontend talks to your backend, which proxies to Bridge:

```javascript
// Frontend → Your API → Bridge
app.post('/api/conversations', async (req, res) => {
  const response = await fetch('http://bridge/agents/.../conversations', {...});
  res.json(await response.json());
});
```

---

## Key Responsibilities

| What | Your Control Plane | Bridge |
|------|-------------------|--------|
| Agent definitions | ✅ Owns and stores | ✅ Runs what you push |
| Conversation history | ✅ Receives via webhooks, stores | ✅ Manages during conversation |
| User management | ✅ Your auth system | ❌ Not involved |
| Billing/metrics | ✅ Receive tokens.used events | ✅ Reports usage |
| Tool permissions | ✅ Configure in agent | ✅ Enforces |
| Running AI | ❌ | ✅ Calls LLM APIs |
| Streaming to users | ❌ | ✅ SSE stream |

---

## Read This Section

1. **[Pushing Agents](pushing-agents.md)** — How to send agent definitions
2. **[Hydrating Conversations](hydrating-conversations.md)** — Restore conversation history
3. **[Handling Webhooks](handling-webhooks.md)** — Process events from Bridge
4. **[Diff Updates](diff-updates.md)** — Update agents incrementally

---

## Example Control Plane (Pseudocode)

```javascript
// On startup: push agents
async function init() {
  const agents = await db.agents.findAll();
  await bridge.pushAgents(agents);
}

// Webhook handler: save events
app.post('/webhooks/bridge', async (req, res) => {
  const event = req.body;
  
  switch (event.event_type) {
    case 'conversation.message':
      await db.messages.insert(event.data);
      break;
    case 'tokens.used':
      await db.usage.track(event.data);
      break;
  }
  
  res.json({ status: 'ok' });
});

// Frontend proxy: create conversation
app.post('/api/conversations', async (req, res) => {
  const { agentId } = req.body;
  const conv = await bridge.createConversation(agentId, req.user.id);
  res.json(conv);
});
```

---

## See Also

- [Push API](../api-reference/push-api.md) — API reference
- [Webhooks](../core-concepts/webhooks.md) — Event types
