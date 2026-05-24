# Hydrating Conversations

Restore conversation history so users can continue where they left off.

---

## Why Hydrate?

Bridge keeps conversation state in memory. If Bridge restarts, that state is lost. Hydration lets you restore it.

Common scenarios:

| Scenario | How hydration helps |
|----------|---------------------|
| Bridge restarts | Restore active conversations |
| Scaling up | Move conversations to new instances |
| Long delays | Resume after hours/days |
| Multi-device | Continue on phone after desktop |

---

## How It Works

1. Your control plane saves messages via webhooks
2. When needed, push the history back to Bridge
3. Bridge reconstructs the conversation state
4. User continues seamlessly

```
User stops chatting
        │
        ▼
Bridge sends webhook ──► Your DB saves history
        │
[Hours pass, Bridge restarts]
        │
        ▼
User returns
        │
        ▼
Your backend fetches history ──► Hydrates Bridge
        │
        ▼
User continues conversation
```

---

## Saving History

First, save messages as they happen:

```javascript
app.post('/webhooks/bridge', async (req, res) => {
  const event = req.body;
  
  if (event.event_type === 'conversation.message') {
    await db.messages.insert({
      conversation_id: event.conversation_id,
      message: event.data.message,
      timestamp: event.timestamp
    });
  }
  
  res.json({ status: 'ok' });
});
```

---

## Restoring History

When a user returns, hydrate the conversation:

```javascript
async function continueConversation(conversationId, agentId) {
  // Fetch history from your database
  const history = await db.messages.find({
    conversation_id: conversationId
  });
  
  // Hydrate Bridge
  await fetch(`http://bridge/push/agents/${agentId}/conversations`, {
    method: 'POST',
    headers: {
      'Authorization': `Bearer ${BRIDGE_API_KEY}`,
      'Content-Type': 'application/json'
    },
    body: JSON.stringify({
      conversation_id: conversationId,
      messages: history.map(h => ({
        role: h.message.role,
        content: h.message.content
      })),
      user_id: 'user-123',
      metadata: { restored: true }
    })
  });
  
  return conversationId;
}
```

---

## Message Format

Messages must include `role` and `content`:

```json
{
  "messages": [
    { "role": "user", "content": "Hello" },
    { "role": "assistant", "content": "Hi there!" },
    { "role": "user", "content": "Help me with something" },
    { "role": "assistant", "content": "Sure, what do you need?" }
  ]
}
```

Roles:
- `user` — What the user said
- `assistant` — What the AI said
- `system` — System messages (rarely needed)

---

## Partial Hydration

You don't need full history. Bridge only needs recent messages:

```javascript
// Get last 20 messages instead of all 500
const history = await db.messages
  .find({ conversation_id: convId })
  .sort({ timestamp: -1 })
  .limit(20);
```

The AI only sees what you hydrate. Older messages are "forgotten" but the conversation can continue.

---

## Checking if Hydration is Needed

Try to create a conversation. If it already exists, hydrate instead:

```javascript
async function getOrCreateConversation(convId, agentId) {
  try {
    // Try to send a message
    await bridge.sendMessage(convId, { test: true });
    return convId; // Conversation exists
  } catch (err) {
    if (err.code === 'CONVERSATION_NOT_FOUND') {
      // Hydrate and retry
      await hydrateConversation(convId, agentId);
      return convId;
    }
    throw err;
  }
}
```

---

## Example: Resume After Bridge Restart

```javascript
// On startup: restore active conversations
async function restoreConversations() {
  const activeConvs = await db.conversations.find({
    status: 'active',
    updated_at: { $gt: Date.now() - 24 * 60 * 60 * 1000 } // Last 24 hours
  });
  
  for (const conv of activeConvs) {
    const messages = await db.messages.find({
      conversation_id: conv.id
    });
    
    await hydrateConversation(conv.id, conv.agent_id, messages);
    console.log(`Restored conversation ${conv.id}`);
  }
}

// Run on control plane startup
await restoreConversations();
```

---

## Handling Conflicts

What if Bridge still has the conversation?

Bridge will reject hydration if the conversation ID already exists. Check first:

```javascript
async function hydrateIfNeeded(convId, agentId, messages) {
  const exists = await checkConversationExists(convId);
  
  if (exists) {
    console.log('Conversation already active');
    return;
  }
  
  await hydrateConversation(convId, agentId, messages);
}
```

Or just try and handle the error:

```javascript
try {
  await hydrateConversation(...);
} catch (err) {
  if (err.code === 'CONVERSATION_EXISTS') {
    // Already there, continue
  } else {
    throw err;
  }
}
```

---

## Best Practices

### Store All Messages

Don't filter when saving — store everything:

```javascript
// Good: save all
await db.messages.insert(event.data.message);

// Bad: only saving user messages loses context
if (event.data.message.role === 'user') {
  await db.messages.insert(event.data.message);
}
```

### Include Metadata

Track why conversations were hydrated:

```javascript
await hydrateConversation(..., {
  metadata: {
    restored: true,
    original_created_at: conv.created_at,
    bridge_instance: 'instance-2'
  }
});
```

### Clean Up Old Conversations

Don't hydrate conversations from months ago:

```javascript
const ONE_DAY = 24 * 60 * 60 * 1000;

if (Date.now() - conv.last_activity > ONE_DAY) {
  // Don't restore, treat as new
  return createNewConversation();
}
```

---

## See Also

- [Push API](../api-reference/push-api.md) — Hydration endpoint
- [Webhooks](handling-webhooks.md) — Saving history
- [Conversations](../core-concepts/conversations.md) — How conversations work
