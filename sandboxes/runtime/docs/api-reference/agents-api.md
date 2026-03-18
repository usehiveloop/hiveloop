# Agents API

List agents and get agent details.

---

## List All Agents

Get all currently loaded agents.

### Request

```
GET /agents
```

No authentication required.

### Response

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

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `agents` | array | List of agent summaries |
| `agents[].id` | string | Agent ID |
| `agents[].name` | string | Human-readable name |
| `agents[].version` | string | Current version |

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

```json
{
  "id": "code-reviewer",
  "name": "Code Reviewer",
  "system_prompt": "You are a senior engineer...",
  "provider": {
    "provider_type": "anthropic",
    "model": "claude-sonnet-4-20250514"
  },
  "tools": ["read", "edit", "grep"],
  "mcp_servers": [],
  "skills": [],
  "integrations": [],
  "config": {
    "max_tokens": 4096,
    "max_turns": 50,
    "temperature": 0.2
  },
  "version": "3",
  "created_at": "2026-01-15T10:30:00Z",
  "updated_at": "2026-01-15T14:22:00Z"
}
```

### Fields

Most fields match the agent definition you pushed. Additional fields:

| Field | Type | Description |
|-------|------|-------------|
| `created_at` | string | When first pushed (ISO 8601) |
| `updated_at` | string | When last updated (ISO 8601) |

### Errors

| Status | Code | Meaning |
|--------|------|---------|
| 404 | `AGENT_NOT_FOUND` | Agent doesn't exist |

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

```json
{
  "approvals": [
    {
      "request_id": "req-abc123",
      "tool_name": "bash",
      "arguments": {
        "command": "rm -rf /important/data"
      },
      "requested_at": "2026-01-15T10:35:00Z"
    }
  ]
}
```

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `approvals` | array | Pending approval requests |
| `approvals[].request_id` | string | ID for resolving |
| `approvals[].tool_name` | string | Tool being called |
| `approvals[].arguments` | object | Tool arguments |
| `approvals[].requested_at` | string | When requested |

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

### Body

```json
{
  "approved": true,
  "reason": "User confirmed deletion"
}
```

### Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `approved` | boolean | Yes | `true` to approve, `false` to deny |
| `reason` | string | No | Optional reason for audit logs |

### Response

```json
{
  "status": "resolved"
}
```

### Errors

| Status | Code | Meaning |
|--------|------|---------|
| 404 | `APPROVAL_NOT_FOUND` | Request ID doesn't exist |
| 409 | `ALREADY_RESOLVED` | Already approved/denied |

### Example

```bash
# Approve
curl -X POST http://localhost:8080/agents/code-reviewer/conversations/conv-123/approvals/req-abc123 \
  -H "Content-Type: application/json" \
  -d '{"approved": true}'

# Deny
curl -X POST http://localhost:8080/agents/code-reviewer/conversations/conv-123/approvals/req-abc123 \
  -H "Content-Type: application/json" \
  -d '{"approved": false, "reason": "Too dangerous"}'
```

---

## Bulk Resolve Approvals

Resolve multiple approvals at once.

### Request

```
POST /agents/{agent_id}/conversations/{conversation_id}/approvals
```

### Body

```json
{
  "action": "approve_all"
}
```

Or:

```json
{
  "action": "deny_all",
  "reason": "User cancelled operation"
}
```

### Response

```json
{
  "resolved": 3
}
```

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `resolved` | number | How many approvals were resolved |

### Example

```bash
curl -X POST http://localhost:8080/agents/code-reviewer/conversations/conv-123/approvals \
  -H "Content-Type: application/json" \
  -d '{"action": "approve_all"}'
```

---

## See Also

- [Conversations API](conversations-api.md) — Create conversations, send messages
- [Tools](../core-concepts/tools.md) — Tool permissions and approvals
