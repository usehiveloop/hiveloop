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

The `version` field controls updates for single-agent operations:

```javascript
// Version 1 - created
await fetch('/push/agents/my-agent', {
  method: 'PUT',
  body: JSON.stringify({ id: 'my-agent', version: '1', ... })
});
// Result: 201 Created, { "status": "created" }

// Same version again — no change (idempotent)
await fetch('/push/agents/my-agent', {
  method: 'PUT',
  body: JSON.stringify({ id: 'my-agent', version: '1', ... })
});
// Result: 200 OK, { "status": "unchanged" }

// New version — triggers update with drain
await fetch('/push/agents/my-agent', {
  method: 'PUT',
  body: JSON.stringify({ id: 'my-agent', version: '2', ... })
});
// Result: 200 OK, { "status": "updated" }
```

**Important**: Version comparison uses simple string equality. `version: "1.0"` and `version: "1.00"` are treated as different versions. Both agents having `version: null` or both having the same string value counts as "same version".

**Note**: Bulk push (`POST /push/agents`) does NOT perform version comparison. It loads all agents immediately, overwriting any existing agents with the same ID.

---

## Bulk vs Single

### Bulk Push (Recommended for Startup)

Push all agents at once:

```javascript
await fetch('http://bridge/push/agents', {
  method: 'POST',
  body: JSON.stringify({
    agents: [agent1, agent2, agent3]
  })
});
```

**Behavior:**
- Loads all agents immediately
- Overwrites existing agents with the same ID (no version check)
- Returns `{ "loaded": 3 }` with the count of agents received
- Errors for individual agents are logged by Bridge, not returned in the response

Good for:
- Startup initialization
- Initial bulk load
- When you want to ensure all agents are replaced

### Single Push (Upsert)

Update one agent with version-aware behavior:

```javascript
await fetch('http://bridge/push/agents/my-agent', {
  method: 'PUT',
  body: JSON.stringify(agent)
});
```

**Behavior:**
- Creates new agent if it doesn't exist (returns `201 Created`)
- Returns `200 OK` with `{"status": "unchanged"}` if version matches
- Returns `200 OK` with `{"status": "updated"}` if version differs (performs drain + replace)
- Returns `400 Bad Request` if path `agent_id` doesn't match body `id`

Good for:
- Individual agent updates
- Admin UI operations
- When you need version-aware updates

### Remove Agent

Delete an agent from Bridge:

```javascript
await fetch('http://bridge/push/agents/my-agent', {
  method: 'DELETE',
  headers: { 'Authorization': `Bearer ${BRIDGE_API_KEY}` }
});
// Result: { "status": "removed" }
```

**Behavior:**
- Returns `404` if agent doesn't exist
- Cancels the agent's cancellation token
- Waits for active conversations to complete
- Disconnects MCP servers
- Removes agent from the map

---

## Update Behavior (Drain and Replace)

When an agent is updated (via PUT with different version, or DELETE), Bridge performs a graceful transition:

1. **Cancel** the old agent's token (signals conversations to stop)
2. **Close** the task tracker (no new conversations can start)
3. **Wait** for in-flight conversations to complete (60 second timeout)
4. **Force** replacement if timeout is reached
5. **Swap** in the new agent state

This ensures active conversations have a chance to complete before the agent is replaced.

---

## Handling Failures

### Authentication Errors

All push endpoints return `401 Unauthorized` with error code `unauthorized` for missing or invalid bearer tokens.

### Validation Errors

Single upsert returns `400 Bad Request` with error code `invalid_request` if the path `agent_id` doesn't match the body `id`:

```javascript
const response = await fetch('http://bridge/push/agents/foo', {
  method: 'PUT',
  body: JSON.stringify({ id: 'bar', ... }) // Mismatch!
});
// Result: 400, { "error": { "code": "invalid_request", "message": "..." } }
```

### Not Found Errors

DELETE and hydrate endpoints return `404 Not Found` with error code `agent_not_found` if the agent doesn't exist.

### Bulk Push Limitations

**Important**: Bulk push (`POST /push/agents`) does NOT return per-agent errors in the response. Failures are logged by Bridge but the response only contains the count:

```javascript
const response = await fetch('http://bridge/push/agents', {
  method: 'POST',
  body: JSON.stringify({ agents: [goodAgent, badAgent] })
});

const result = await response.json();
// { "loaded": 2 } — even if one agent failed to load!
// Check Bridge logs for actual errors
```

For per-agent error handling, use single upsert (`PUT /push/agents/{id}`) or diff updates (`POST /push/diff`).

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

### Use Versioned Updates for Production

For production updates, use single upsert or diff updates to benefit from version-aware behavior:

```javascript
// Good: version-aware, graceful drain
await fetch(`/push/agents/${agent.id}`, {
  method: 'PUT',
  body: JSON.stringify(agent)
});

// Or use diff for batch updates
await fetch('/push/diff', {
  method: 'POST',
  body: JSON.stringify({
    added: [newAgent],
    updated: [changedAgent],
    removed: [oldAgentId]
  })
});
```

---

## See Also

- [Push API](../api-reference/push-api.md) — Complete API reference
- [Diff Updates](diff-updates.md) — Incremental updates
- [Agents](../core-concepts/agents.md) — Agent concepts
