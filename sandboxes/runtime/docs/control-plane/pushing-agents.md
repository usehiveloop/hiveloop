# Pushing Agents

Send agent definitions to Bridge so they're ready to handle conversations.

---

## When to Push

Push agents in these situations:

| When | Why |
|------|-----|
| Control plane starts | Bridge has no agents on startup |
| Agent definitions change | Update running agents |
| New agent created | Make it available |
| Rolling update | Ensure all Bridge instances have agents |

---

## Push on Startup

Your control plane should push agents when it starts:

```javascript
async function pushAgents() {
  const agents = await db.agents.findAll();
  
  const response = await fetch('http://bridge:8080/push/agents', {
    method: 'POST',
    headers: {
      'Authorization': `Bearer ${BRIDGE_API_KEY}`,
      'Content-Type': 'application/json'
    },
    body: JSON.stringify({ agents })
  });
  
  const result = await response.json();
  console.log(`Loaded ${result.loaded} agents`);
}

// Run on startup
await pushAgents();
```

---

## Agent Definition Format

Complete agent definition:

```json
{
  "id": "support-agent",
  "name": "Customer Support",
  "system_prompt": "You are a helpful support agent...",
  "provider": {
    "provider_type": "anthropic",
    "model": "claude-sonnet-4-20250514",
    "api_key": "<anthropic-api-key>",
    "base_url": "https://api.anthropic.com"
  },
  "tools": ["read", "web_search"],
  "mcp_servers": [],
  "skills": [],
  "integrations": [],
  "subagents": [],
  "permissions": {},
  "config": {
    "max_tokens": 2048,
    "max_turns": 50,
    "temperature": 0.7
  },
  "webhook_url": "https://your-api.com/webhooks/bridge",
  "webhook_secret": "<webhook-secret>",
  "version": "1"
}
```

See [Agents](../core-concepts/agents.md) for field details.

---

## Versioning

The `version` field controls updates:

```javascript
// Version 1
await pushAgent({ id: 'my-agent', version: '1', ... });
// Result: created

// Same version again — no change
await pushAgent({ id: 'my-agent', version: '1', ... });
// Result: unchanged

// New version — triggers update
await pushAgent({ id: 'my-agent', version: '2', ... });
// Result: updated (drain + replace)
```

Version strings are opaque to Bridge. Use semver, dates, or build numbers — whatever works for you.

---

## Bulk vs Single

### Bulk Push (Recommended)

Push all agents at once:

```javascript
await fetch('http://bridge/push/agents', {
  method: 'POST',
  body: JSON.stringify({
    agents: [agent1, agent2, agent3]
  })
});
```

Good for:
- Startup initialization
- Bulk updates
- Sync operations

### Single Push

Update one agent:

```javascript
await fetch('http://bridge/push/agents/my-agent', {
  method: 'PUT',
  body: JSON.stringify(agent)
});
```

Good for:
- Individual agent updates
- Admin UI operations

---

## Handling Failures

Partial failures don't block:

```javascript
const response = await fetch('http://bridge/push/agents', {
  method: 'POST',
  body: JSON.stringify({ agents: [goodAgent, badAgent] })
});

const result = await response.json();
// { loaded: 1, errors: [{ agent_id: 'bad', error: '...' }] }
```

Always check the `errors` array and handle appropriately.

---

## Multiple Bridge Instances

If you run multiple Bridge instances, push to all of them:

```javascript
const bridgeInstances = [
  'http://bridge-1:8080',
  'http://bridge-2:8080',
  'http://bridge-3:8080'
];

async function pushToAll(agents) {
  await Promise.all(bridgeInstances.map(url =>
    fetch(`${url}/push/agents`, {
      method: 'POST',
      body: JSON.stringify({ agents })
    })
  ));
}
```

Consider using a load balancer that broadcasts push requests to all instances.

---

## Storing Agent Definitions

Your control plane should store agent definitions so you can:

- Push on startup
- Show in admin UI
- Track versions
- Rollback if needed

Example schema:

```sql
CREATE TABLE agents (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  definition JSONB NOT NULL,
  version TEXT NOT NULL,
  created_at TIMESTAMP DEFAULT NOW(),
  updated_at TIMESTAMP DEFAULT NOW()
);
```

---

## Best Practices

### Validate Before Push

Check your agent definition before pushing:

```javascript
function validateAgent(agent) {
  if (!agent.id) throw new Error('Missing id');
  if (!agent.system_prompt) throw new Error('Missing system_prompt');
  if (!agent.provider?.api_key) throw new Error('Missing API key');
  // ... etc
}
```

### Use Environment Variables for Secrets

Don't hardcode API keys:

```javascript
const agent = {
  ...definition,
  provider: {
    ...definition.provider,
    api_key: process.env.ANTHROPIC_API_KEY
  }
};
```

### Test Before Production

Push to a staging Bridge first:

```javascript
if (environment === 'production') {
  await pushTo('https://bridge.prod/push/agents');
} else {
  await pushTo('https://bridge.staging/push/agents');
}
```

---

## See Also

- [Push API](../api-reference/push-api.md) — Complete API reference
- [Agents](../core-concepts/agents.md) — Agent concepts
