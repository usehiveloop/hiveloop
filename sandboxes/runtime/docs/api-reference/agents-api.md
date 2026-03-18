# Agents API

List agents and get agent details. All endpoints return JSON responses.

---

## List All Agents

Get all currently loaded agents.

### Request

```
GET /agents
```

No authentication required.

### Response

**200 OK**
```json
{
  "agents": [
    {
      "id": "code-reviewer",
      "name": "Code Reviewer",
      "version": "3"
    },
    {
      "id": "support-agent",
      "name": "Customer Support",
      "version": "12"
    }
  ]
}
```

### Response Fields

| Field | Type | Description |
|-------|------|-------------|
| `agents` | array | List of agent summaries |
| `agents[].id` | string | Agent ID |
| `agents[].name` | string | Human-readable name |
| `agents[].version` | string \| null | Current version (if set) |

### Example

```bash
curl http://localhost:8080/agents
```

---

## Get Agent Details

Get full details for a specific agent.

### Request

```
GET /agents/{agent_id}
```

### Response

**200 OK**
```json
{
  "id": "code-reviewer",
  "name": "Code Reviewer",
  "system_prompt": "You are a senior engineer...",
  "version": "3",
  "active_conversations": 5
}
```

### Response Fields

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Agent identifier |
| `name` | string | Human-readable name |
| `system_prompt` | string | System prompt for the agent |
| `version` | string \| null | Current version (if set) |
| `active_conversations` | number | Number of active conversations for this agent |

### Error Responses

| Status | Error Code | Description |
|--------|------------|-------------|
| 404 | `agent_not_found` | Agent with the specified ID does not exist |

### Example

```bash
curl http://localhost:8080/agents/code-reviewer
```

---

## List Pending Approvals

Get tool approval requests waiting for user action.

### Request

```
GET /agents/{agent_id}/conversations/{conversation_id}/approvals
```

### Response

**200 OK**
```json
[
  {
    "id": "req-abc123",
    "agent_id": "code-reviewer",
    "conversation_id": "conv-123",
    "tool_name": "bash",
    "tool_call_id": "call_123",
    "arguments": {
      "command": "rm -rf /important/data"
    },
    "status": "pending",
    "created_at": "2026-01-15T10:35:00Z"
  }
]
```

### Response Fields

| Field | Type | Description |
|-------|------|-------------|
| `[]` | array | List of pending approval requests |
| `[].id` | string | Unique approval request ID |
| `[].agent_id` | string | Agent that initiated the tool call |
| `[].conversation_id` | string | Conversation where the tool call occurred |
| `[].tool_name` | string | Name of the tool being called |
| `[].tool_call_id` | string | LLM's tool call ID |
| `[].arguments` | object | Tool arguments |
| `[].status` | string | Current status: `pending`, `approved`, or `denied` |
| `[].created_at` | string | When the request was created (ISO 8601) |

### Example

```bash
curl http://localhost:8080/agents/code-reviewer/conversations/conv-123/approvals
```

---

## Resolve a Single Approval

Approve or deny a tool call.

### Request

```
POST /agents/{agent_id}/conversations/{conversation_id}/approvals/{request_id}
```

### Request Body

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `decision` | string | Yes | `"approve"` or `"deny"` |

```json
{
  "decision": "approve"
}
```

### Response

**200 OK**
```json
{
  "status": "resolved",
  "request_id": "req-abc123"
}
```

### Error Responses

| Status | Error Code | Description |
|--------|------------|-------------|
| 404 | `conversation_not_found` | Conversation doesn't exist |
| 404 | (empty body) | Approval request ID doesn't exist |

### Example

```bash
# Approve
curl -X POST http://localhost:8080/agents/code-reviewer/conversations/conv-123/approvals/req-abc123 \
  -H "Content-Type: application/json" \
  -d '{"decision": "approve"}'

# Deny
curl -X POST http://localhost:8080/agents/code-reviewer/conversations/conv-123/approvals/req-abc123 \
  -H "Content-Type: application/json" \
  -d '{"decision": "deny"}'
```

---

## Bulk Resolve Approvals

Resolve multiple approvals at once.

### Request

```
POST /agents/{agent_id}/conversations/{conversation_id}/approvals
```

### Request Body

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `request_ids` | array | Yes | List of approval request IDs to resolve |
| `decision` | string | Yes | `"approve"` or `"deny"` |

```json
{
  "request_ids": ["req-abc123", "req-def456"],
  "decision": "approve"
}
```

### Response

**200 OK**
```json
{
  "resolved": ["req-abc123", "req-def456"],
  "not_found": []
}
```

### Response Fields

| Field | Type | Description |
|-------|------|-------------|
| `resolved` | array | List of request IDs that were successfully resolved |
| `not_found` | array | List of request IDs that were not found |

### Example

```bash
curl -X POST http://localhost:8080/agents/code-reviewer/conversations/conv-123/approvals \
  -H "Content-Type: application/json" \
  -d '{
    "request_ids": ["req-abc123", "req-def456"],
    "decision": "approve"
  }'
```

---

## See Also

- [Conversations API](conversations-api.md) — Create conversations, send messages
- [Tools](../core-concepts/tools.md) — Tool permissions and approvals
