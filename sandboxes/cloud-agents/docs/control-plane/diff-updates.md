# Diff Updates

Update many agents at once with incremental changes.

---

## When to Use Diff Updates

Use diff updates when you need to change multiple agents:

| Scenario | Example |
|----------|---------|
| Update shared prompt | Change the footer in all agent prompts |
| Rotate API keys | Update provider keys across all agents |
| Add a tool | Give all agents access to a new tool |
| Version bump | Increment all versions for a release |

---

## How Diff Updates Work

Instead of pushing full agent definitions, you specify:

1. **Which agents** to update
2. **What fields** to change
3. **New version** for all affected agents

```javascript
await fetch('http://bridge/push/diff', {
  method: 'POST',
  headers: { 'Authorization': 'Bearer ...' },
  body: JSON.stringify({
    updates: [
      { agent_id: 'agent1', updates: { temperature: 0.5 } },
      { agent_id: 'agent2', updates: { temperature: 0.5 } }
    ],
    new_version: '2'
  })
});
```

---

## Update Format

Each update specifies an agent and the fields to change:

```json
{
  "updates": [
    {
      "agent_id": "support-agent",
      "updates": {
        "system_prompt": "Updated prompt...",
        "config": {
          "temperature": 0.3
        }
      }
    }
  ],
  "new_version": "2"
}
```

### Nested Updates

Updates are merged shallowly. For nested objects like `config`, provide the full object:

```json
{
  "updates": {
    "config": {
      "temperature": 0.3,        // Changed
      "max_tokens": 2048         // Must include unchanged fields too
    }
  }
}
```

---

## Common Update Patterns

### Update All Agent Prompts

```javascript
const agents = await db.agents.findAll();

const updates = agents.map(agent => ({
  agent_id: agent.id,
  updates: {
    system_prompt: agent.system_prompt + '\n\nAlways be concise.'
  }
}));

await bridge.pushDiff(updates, '2');
```

### Rotate API Keys

```javascript
const newKey = process.env.NEW_ANTHROPIC_KEY;
const agents = await db.agents.findAll();

const updates = agents.map(agent => ({
  agent_id: agent.id,
  updates: {
    provider: {
      ...agent.provider,
      api_key: newKey
    }
  }
}));

await bridge.pushDiff(updates, '3');
```

### Add a Tool to All Agents

```javascript
const agents = await db.agents.findAll();

const updates = agents.map(agent => ({
  agent_id: agent.id,
  updates: {
    tools: [...agent.tools, 'new_tool']
  }
}));

await bridge.pushDiff(updates, '4');
```

---

## Response

Bridge returns which agents were updated:

```json
{
  "updated": ["agent1", "agent2", "agent3"],
  "errors": [
    { "agent_id": "agent4", "error": "Agent not found" }
  ]
}
```

---

## Error Handling

Individual failures don't stop the batch:

```javascript
const result = await bridge.pushDiff(updates, '2');

if (result.errors.length > 0) {
  console.error('Failed updates:', result.errors);
  // Handle partial failure
}
```

---

## Version Requirements

Diff updates require a `new_version`. All updated agents get this version.

If an agent is already at `new_version`, it's skipped (no change).

---

## Alternative: Direct Updates

For single agents, use direct PUT:

```javascript
// Instead of diff
await fetch('http://bridge/push/agents/agent1', {
  method: 'PUT',
  body: JSON.stringify({ ...agent, temperature: 0.5, version: '2' })
});
```

Use diff updates when:
- Updating 3+ agents
- Changes are similar across agents

Use direct updates when:
- Updating 1-2 agents
- Changes are unique per agent

---

## Best Practices

### Test First

Test your diff on a single agent:

```javascript
// Test with one agent
await bridge.pushDiff([updates[0]], 'test-version');

// Verify, then apply to all
await bridge.pushDiff(updates, '2');
```

### Backup Definitions

Store agent definitions before bulk updates:

```javascript
const backup = await db.agents.findAll();
await db.agent_backups.insert({
  created_at: new Date(),
  agents: backup
});
```

### Validate Changes

Check updates before sending:

```javascript
function validateUpdate(update) {
  if (!update.agent_id) throw new Error('Missing agent_id');
  if (!update.updates) throw new Error('Missing updates');
}

updates.forEach(validateUpdate);
```

---

## See Also

- [Push API](../api-reference/push-api.md) — Diff endpoint reference
- [Pushing Agents](pushing-agents.md) — Full agent pushes
