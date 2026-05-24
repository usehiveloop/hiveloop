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

**200 OK** — returns a JSON array of `AgentResponse` objects (the same shape as `GET /agents/{agent_id}`, with `system_prompt` truncated to 100 characters):

```json
[
  {
    "id": "code-reviewer",
    "name": "Code Reviewer",
    "system_prompt": "You are a senior engineer who reviews PRs against …",
    "provider": { "provider_type": "anthropic", "model": "claude-sonnet-4-20250514", "base_url": null },
    "tools": [],
    "mcp_servers": [],
    "skills": [],
    "integrations": [],
    "config": { "...": "..." },
    "subagents": [],
    "permissions": {},
    "version": "3",
    "active_conversations": 5,
    "metrics": { "...": "..." },
    "registered_tools": [{ "name": "Read", "description": "..." }]
  }
]
```

See [Get Agent Details](#get-agent-details) below for the full field reference. The list endpoint returns the same `AgentResponse` shape; only `system_prompt` (and each subagent's `system_prompt`) is truncated to 100 characters. The `artifacts` field is included when the agent has it set.

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
  "description": "Reviews PRs against project conventions",
  "system_prompt": "You are a senior engineer...",
  "provider": {
    "provider_type": "anthropic",
    "model": "claude-sonnet-4-20250514",
    "base_url": null
  },
  "tools": [],
  "mcp_servers": [],
  "skills": [],
  "integrations": [],
  "artifacts": {
    "upload_url": "https://control-plane.example.com/uploads",
    "max_size_bytes": 524288000,
    "accepted_file_types": ["csv", "md", "video/*"]
  },
  "config": { "...": "..." },
  "subagents": [],
  "permissions": { "bash": "require_approval" },
  "webhook_url": "https://...",
  "version": "3",
  "updated_at": "2026-04-25T10:00:34Z",
  "active_conversations": 5,
  "metrics": { "...": "..." },
  "registered_tools": [
    { "name": "Read", "description": "Read a file..." },
    { "name": "upload_to_workspace", "description": "Upload a file..." }
  ]
}
```

### Response Fields

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Agent identifier |
| `name` | string | Human-readable name |
| `description` | string \| null | Optional human-readable description |
| `system_prompt` | string | System prompt for the agent (may be truncated to 100 chars on the list endpoint) |
| `provider` | object | LLM provider summary (`provider_type`, `model`, `base_url`); `api_key` is intentionally omitted |
| `tools` | array | Agent-defined tool overrides (empty when the agent uses bridge's full built-in toolset) |
| `mcp_servers` | array | MCP server connections configured on the agent |
| `skills` | array | Skill definitions registered on the agent |
| `integrations` | array | Integration definitions exposed to the agent |
| `artifacts` | object \| absent | Workspace artifact upload configuration. Omitted from the JSON when not set on the definition. |
| `config` | object | Agent configuration (`max_tokens`, `immortal`, `history_strip`, `tool_requirements`, etc. — see [Agent Configuration](../core-concepts/agents.md#configuration)) |
| `subagents` | array | Subagent summaries (`name`, `description`, truncated `system_prompt`, `provider`, `config`, `tools`, `mcp_servers`, `registered_tools`) |
| `permissions` | object | Per-tool permission overrides (`tool_name → "allow" | "deny" | "require_approval"`) |
| `webhook_url` | string \| null | Webhook URL for event delivery if configured |
| `version` | string \| null | Current version (if set) |
| `updated_at` | string \| null | Last-updated timestamp for change detection |
| `active_conversations` | number | Number of currently active conversations for this agent |
| `metrics` | object | Live `MetricsSnapshot` for this agent |
| `registered_tools` | array | All tools the agent currently sees (built-in + MCP + integrations + `upload_to_workspace`), sorted by name. Each entry is `{ name, description }`. |

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

## Get Metrics

Get operational metrics for all agents.

### Request

```
GET /metrics
```

No authentication required.

### Response

**200 OK**
```json
{
  "timestamp": "2026-04-05T12:00:00Z",
  "agents": [
    {
      "agent_id": "agent-1",
      "agent_name": "My Agent",
      "input_tokens": 15000,
      "output_tokens": 3000,
      "total_tokens": 18000,
      "total_requests": 25,
      "failed_requests": 1,
      "active_conversations": 3,
      "total_conversations": 10,
      "tool_calls": 42,
      "avg_latency_ms": 1250.5,
      "tool_call_details": [
        {
          "tool_name": "bash",
          "total_calls": 20,
          "successes": 18,
          "failures": 2,
          "success_rate": 0.9,
          "avg_latency_ms": 500.0
        }
      ]
    }
  ],
  "global": {
    "total_agents": 3,
    "total_active_conversations": 5,
    "uptime_secs": 3600
  }
}
```

### Response Fields

**Top-level:**

| Field | Type | Description |
|-------|------|-------------|
| `timestamp` | string | ISO 8601 timestamp of the snapshot |
| `agents` | array | Per-agent metrics |
| `global` | object | Aggregate metrics across all agents |

**Per-agent (`agents[]`):**

| Field | Type | Description |
|-------|------|-------------|
| `agent_id` | string | Unique agent identifier |
| `agent_name` | string | Human-readable agent name |
| `input_tokens` | integer | Total input tokens consumed |
| `output_tokens` | integer | Total output tokens generated |
| `total_tokens` | integer | Sum of input + output tokens |
| `total_requests` | integer | Total LLM requests made |
| `failed_requests` | integer | Number of failed LLM requests |
| `active_conversations` | integer | Currently active conversations |
| `total_conversations` | integer | Total conversations ever created |
| `tool_calls` | integer | Total tool calls executed |
| `avg_latency_ms` | float | Average LLM request latency in milliseconds |
| `tool_call_details` | array | Per-tool breakdown of call statistics |

**Per-tool (`agents[].tool_call_details[]`):**

| Field | Type | Description |
|-------|------|-------------|
| `tool_name` | string | Name of the tool |
| `total_calls` | integer | Total number of calls to this tool |
| `successes` | integer | Number of successful completions |
| `failures` | integer | Number of failed completions |
| `success_rate` | float | Success rate (successes / total_calls), range 0.0 to 1.0 |
| `avg_latency_ms` | float | Average tool call latency in milliseconds |

**Global (`global`):**

| Field | Type | Description |
|-------|------|-------------|
| `total_agents` | integer | Number of loaded agents |
| `total_active_conversations` | integer | Active conversations across all agents |
| `uptime_secs` | integer | Seconds since Bridge started |

### Example

```bash
curl http://localhost:8080/metrics
```

**Note:** This endpoint returns JSON, not Prometheus format. For Prometheus integration, use a JSON exporter. See [Monitoring](../deployment/monitoring.md) for details.

---

## ToolApprovalRequired Event Data

When a tool call requires approval (permission set to `require_approval`), the SSE and WebSocket event data includes these fields:

| Field | Type | Description |
|-------|------|-------------|
| `request_id` | string | Unique ID for this approval request (use to approve/deny) |
| `tool_name` | string | Name of the tool being called |
| `tool_call_id` | string | The LLM's tool call ID |
| `arguments` | object | Arguments passed to the tool |
| `integration_name` | string \| null | Integration name if this is an integration tool (e.g., `"github"`). Null for non-integration tools. |
| `integration_action` | string \| null | Integration action if this is an integration tool (e.g., `"create_pull_request"`). Null for non-integration tools. |

The `integration_name` and `integration_action` fields let your UI distinguish between regular tool approvals and integration-specific approvals. For example, you could show a different approval dialog for a GitHub "create_pull_request" action than for a generic "bash" command.

Example event data for an integration tool:

```json
{
  "request_id": "req-abc123",
  "tool_name": "github__create_pull_request",
  "tool_call_id": "call-789",
  "arguments": {
    "title": "Fix login bug",
    "base": "main",
    "head": "fix/login"
  },
  "integration_name": "github",
  "integration_action": "create_pull_request"
}
```

Example event data for a regular tool:

```json
{
  "request_id": "req-def456",
  "tool_name": "bash",
  "tool_call_id": "call-012",
  "arguments": {
    "command": "rm -rf /data"
  },
  "integration_name": null,
  "integration_action": null
}
```

---

## See Also

- [Conversations API](conversations-api.md) — Create conversations, send messages
- [SSE Events](sse-events.md) — Full SSE event reference including tool_approval_required
- [Tools](../core-concepts/tools.md) — Tool permissions and approvals
- [Monitoring](../deployment/monitoring.md) — Metrics and monitoring guide
