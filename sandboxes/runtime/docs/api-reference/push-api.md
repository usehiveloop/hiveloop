# Push API

The Push API is how your control plane sends agent definitions to Bridge. All endpoints require authentication.

---

## Authentication

All push endpoints require a bearer token:

```bash
Authorization: Bearer {BRIDGE_CONTROL_PLANE_API_KEY}
```

See [Authentication](authentication.md).

---

## Bulk Load Agents

Push multiple agents at once. Use this on startup.

### Request

```
POST /push/agents
```

### Body

```json
{
  "agents": [
    {
      "id": "support-agent",
      "name": "Customer Support",
      "system_prompt": "You are a helpful support agent...",
      "provider": {
        "provider_type": "anthropic",
        "model": "claude-sonnet-4-20250514",
        "api_key": "<anthropic-api-key>"
      },
      "tools": [],
      "skills": [],
      "integrations": [],
      "config": {
        "max_tokens": 2048,
        "max_turns": 50
      },
      "version": "1"
    }
  ]
}
```

### Response

```json
{
  "loaded": 1,
  "errors": []
}
```

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `loaded` | number | How many agents were loaded |
| `errors` | array | Any errors (empty on success) |

### Behavior

- New agents are created
- Existing agents with different versions are updated (drain + replace)
- Existing agents with same version are unchanged
- Errors for one agent don't affect others

### Example

```bash
curl -X POST http://localhost:8080/push/agents \
  -H "Authorization: Bearer <bridge-control-plane-api-key>" \
  -H "Content-Type: application/json" \
  -d '{
    "agents": [
      {"id": "agent1", "version": "1", ...},
      {"id": "agent2", "version": "3", ...}
    ]
  }'
```

---

## Upsert Single Agent

Create or update a single agent.

### Request

```
PUT /push/agents/{agent_id}
```

### Body

Same as an agent in the bulk endpoint.

### Responses

**Created** — `201 Created`
```json
{"status": "created"}
```

**Updated** — `200 OK`
```json
{"status": "updated"}
```

**Unchanged** — `200 OK`
```json
{"status": "unchanged"}
```

### Example

```bash
curl -X PUT http://localhost:8080/push/agents/my-agent \
  -H "Authorization: Bearer <bridge-control-plane-api-key>" \
  -H "Content-Type: application/json" \
  -d '{
    "id": "my-agent",
    "name": "My Agent",
    "system_prompt": "...",
    "provider": {...},
    "version": "2"
  }'
```

---

## Remove Agent

Remove an agent from Bridge.

### Request

```
DELETE /push/agents/{agent_id}
```

### Response

```json
{
  "status": "deleted"
}
```

### Behavior

- Agent enters draining state
- Existing conversations can finish
- New conversations are rejected
- Agent removed when all conversations end

### Errors

| Status | Code | Meaning |
|--------|------|---------|
| 404 | `AGENT_NOT_FOUND` | Agent doesn't exist |

### Example

```bash
curl -X DELETE http://localhost:8080/push/agents/my-agent \
  -H "Authorization: Bearer <bridge-control-plane-api-key>"
```

---

## Hydrate Conversation

Restore a conversation with existing message history.

### Request

```
POST /push/agents/{agent_id}/conversations
```

### Body

```json
{
  "conversation_id": "conv-existing-123",
  "user_id": "user-456",
  "messages": [
    {"role": "user", "content": "Hello"},
    {"role": "assistant", "content": "Hi there!"},
    {"role": "user", "content": "Help me with something"}
  ],
  "metadata": {
    "restored": true
  }
}
```

### Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `conversation_id` | string | Yes | Existing conversation ID |
| `user_id` | string | No | User identifier |
| `messages` | array | Yes | Message history |
| `metadata` | object | No | Arbitrary metadata |

### Response

```json
{
  "conversation_id": "conv-existing-123",
  "status": "hydrated"
}
```

### Use Cases

- Restoring after Bridge restart
- Migrating conversations between instances
- Implementing "continue where you left off"

### Example

```bash
curl -X POST http://localhost:8080/push/agents/my-agent/conversations \
  -H "Authorization: Bearer <bridge-control-plane-api-key>" \
  -H "Content-Type: application/json" \
  -d '{
    "conversation_id": "conv-123",
    "messages": [
      {"role": "user", "content": "Hello"}
    ]
  }'
```

---

## Rotate API Key

Update an agent's API key without draining.

### Request

```
PATCH /push/agents/{agent_id}/api-key
```

### Body

```json
{
  "api_key": "<anthropic-api-key>"
}
```

### Response

```json
{
  "status": "updated"
}
```

### Behavior

- Immediate update, no draining
- Active conversations continue with new key
- Useful for key rotation on compromised keys

### Example

```bash
curl -X PATCH http://localhost:8080/push/agents/my-agent/api-key \
  -H "Authorization: Bearer <bridge-control-plane-api-key>" \
  -H "Content-Type: application/json" \
  -d '{"api_key": "<anthropic-api-key>"}'
```

---

## Apply Diff Update

Apply incremental changes to agents.

### Request

```
POST /push/diff
```

### Body

```json
{
  "updates": [
    {
      "agent_id": "agent1",
      "updates": {
        "system_prompt": "Updated prompt..."
      }
    }
  ],
  "new_version": "2"
}
```

### Response

```json
{
  "updated": ["agent1"],
  "errors": []
}
```

### Use Case

Update many agents at once (e.g., changing a shared prompt template).

---

## See Also

- [Pushing Agents](../control-plane/pushing-agents.md) — Complete guide
- [Hydrating Conversations](../control-plane/hydrating-conversations.md) — Restore history
- [Agents](../core-concepts/agents.md) — Agent lifecycle
