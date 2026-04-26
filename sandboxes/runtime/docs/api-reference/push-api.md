# Push API

The Push API is how your control plane sends agent definitions to Bridge. All endpoints require authentication.

---

## Authentication

All push endpoints require a bearer token:

```bash
Authorization: Bearer {BRIDGE_CONTROL_PLANE_API_KEY}
```

Missing or invalid tokens return **401 Unauthorized** with error code `unauthorized`.

See [Authentication](authentication.md).

---

## Bulk Load Agents

Push multiple agents at once. Use this on startup.

### Request

```
POST /push/agents
```

### Request Body

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `agents` | array | Yes | List of agent definitions |

#### Agent Definition Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | Yes | Unique agent identifier |
| `name` | string | Yes | Human-readable name |
| `system_prompt` | string | Yes | System prompt for the agent |
| `provider` | object | Yes | LLM provider configuration |
| `provider.provider_type` | string | Yes | Provider: `open_ai`, `anthropic`, `google`, `groq`, `deep_seek`, `mistral`, `cohere`, `x_ai`, `together`, `fireworks`, `ollama`, `custom` |
| `provider.model` | string | Yes | Model identifier (e.g., `gpt-4o`, `claude-sonnet-4-20250514`) |
| `provider.api_key` | string | Yes | API key for authentication |
| `provider.base_url` | string | No | Optional custom endpoint URL |
| `description` | string | No | Description for subagent documentation |
| `tools` | array | No | Agent-defined tools (default: `[]`) |
| `tools[].name` | string | Yes | Tool name |
| `tools[].description` | string | Yes | Tool description |
| `tools[].parameters_schema` | object | Yes | JSON Schema for parameters |
| `mcp_servers` | array | No | MCP server connections (default: `[]`) |
| `mcp_servers[].name` | string | Yes | Server name |
| `mcp_servers[].transport` | object | Yes | Transport configuration |
| `skills` | array | No | Available skills (default: `[]`) |
| `integrations` | array | No | External integrations (default: `[]`) |
| `artifacts` | object | No | Workspace artifact upload configuration. When present, bridge auto-registers an `upload_to_workspace` tool. See [Artifacts Definition](../core-concepts/agents.md#artifacts-definition). |
| `config` | object | No | Agent configuration |
| `config.max_tokens` | number | No | Maximum tokens for LLM response |
| `config.max_turns` | number | No | Maximum conversation turns |
| `config.temperature` | number | No | Temperature for LLM sampling (0-1) |
| `config.json_schema` | object | No | JSON schema for structured output |
| `config.rate_limit_rpm` | number | No | Rate limit in requests per minute |
| `config.immortal` | object | No | In-place forgecode-style history compaction (replaces the previous `compaction` field). See [Immortal Mode](../core-concepts/agents.md#immortal-mode). |
| `config.history_strip` | object | No | Strip old tool-result bodies from history before sending to the LLM. See [History Stripping](../core-concepts/agents.md#history-stripping). |
| `config.system_reminder_refresh_turns` | number | No | Re-emit the stable system reminder every N turns (default `10`; values `<1` clamp to `1`) |
| `config.subagent_timeout_foreground_secs` | number | No | Foreground subagent timeout in seconds (default `300`) |
| `config.subagent_timeout_background_secs` | number | No | Background subagent timeout in seconds (default `300`) |
| `config.tool_calls_only` | boolean | No | Accept tool-only turns as success (default `false`) |
| `config.tool_requirements` | array | No | Declarative per-turn tool-call requirements |
| `config.disabled_tools` | string[] | No | Tools to remove from the agent (default `[]`) |
| `permissions` | object | No | Per-tool permission overrides (default: `{}`) |
| `webhook_url` | string | No | Webhook URL for event delivery |
| `webhook_secret` | string | No | Webhook secret for HMAC signing |
| `version` | string | No | Version for change detection |
| `updated_at` | string | No | Last updated timestamp |
| `subagents` | array | No | Nested subagent definitions (default: `[]`) |

### Response

**200 OK**
```json
{
  "loaded": 2
}
```

### Response Fields

| Field | Type | Description |
|-------|------|-------------|
| `loaded` | number | Number of agents received in the request |

### Behavior

- Loads all agents immediately
- **No version comparison** — existing agents are overwritten unconditionally
- For version-aware updates, use `PUT /push/agents/{agent_id}` or `POST /push/diff`
- Errors for individual agents are logged by Bridge but not returned in the response
- Returns `200 OK` as long as the request body is valid JSON

### Error Responses

| Status | Error Code | Description |
|--------|------------|-------------|
| 401 | `unauthorized` | Invalid or missing bearer token |

### Example

```bash
curl -X POST http://localhost:8080/push/agents \
  -H "Authorization: Bearer <bridge-control-plane-api-key>" \
  -H "Content-Type: application/json" \
  -d '{
    "agents": [
      {
        "id": "agent1",
        "name": "Agent One",
        "system_prompt": "You are helpful...",
        "provider": {
          "provider_type": "open_ai",
          "model": "gpt-4o",
          "api_key": "<provider-api-key>"
        },
        "version": "1"
      }
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

### Request Body

Same fields as an agent in the bulk endpoint. The `id` in the body must match the `agent_id` path parameter.

### Responses

**201 Created** — Agent was newly created
```json
{
  "status": "created"
}
```

**200 OK** — Agent was updated (version changed)
```json
{
  "status": "updated"
}
```

**200 OK** — Agent unchanged (same version)
```json
{
  "status": "unchanged"
}
```

### Error Responses

| Status | Error Code | Description |
|--------|------------|-------------|
| 400 | `invalid_request` | Path agent_id doesn't match body id |
| 401 | `unauthorized` | Invalid or missing bearer token |

### Example

```bash
curl -X PUT http://localhost:8080/push/agents/my-agent \
  -H "Authorization: Bearer <bridge-control-plane-api-key>" \
  -H "Content-Type: application/json" \
  -d '{
    "id": "my-agent",
    "name": "My Agent",
    "system_prompt": "You are helpful...",
    "provider": {
      "provider_type": "anthropic",
      "model": "claude-sonnet-4-20250514",
      "api_key": "<anthropic-api-key>"
    },
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

**200 OK**
```json
{
  "status": "removed"
}
```

### Behavior

- Agent enters draining state
- Existing conversations can finish
- New conversations are rejected
- Agent removed when all conversations end

### Error Responses

| Status | Error Code | Description |
|--------|------------|-------------|
| 401 | `unauthorized` | Invalid or missing bearer token |
| 404 | `agent_not_found` | Agent doesn't exist |

### Example

```bash
curl -X DELETE http://localhost:8080/push/agents/my-agent \
  -H "Authorization: Bearer <bridge-control-plane-api-key>"
```

---

## Hydrate Conversations

Restore conversations with existing message history.

### Request

```
POST /push/agents/{agent_id}/conversations
```

### Request Body

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `conversations` | array | Yes | List of conversation records to hydrate |
| `conversations[].id` | string | Yes | Conversation ID |
| `conversations[].agent_id` | string | Yes | Agent ID |
| `conversations[].user_id` | string | No | User identifier |
| `conversations[].messages` | array | Yes | Message history |
| `conversations[].messages[].role` | string | Yes | `user`, `assistant`, or `system` |
| `conversations[].messages[].content` | string/array | Yes | Message content |
| `conversations[].metadata` | object | No | Arbitrary metadata |

### Response

**200 OK**
```json
{
  "hydrated": 3
}
```

### Response Fields

| Field | Type | Description |
|-------|------|-------------|
| `hydrated` | number | Number of conversations hydrated |

### Error Responses

| Status | Error Code | Description |
|--------|------------|-------------|
| 401 | `unauthorized` | Invalid or missing bearer token |
| 404 | `agent_not_found` | Agent doesn't exist |
| 409 | `conflict` | Agent has active conversations |

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
    "conversations": [
      {
        "id": "conv-123",
        "agent_id": "my-agent",
        "user_id": "user-456",
        "messages": [
          {"role": "user", "content": "Hello"},
          {"role": "assistant", "content": "Hi there!"}
        ],
        "metadata": {"restored": true}
      }
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

### Request Body

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `api_key` | string | Yes | New API key |

```json
{
  "api_key": "<anthropic-api-key>"
}
```

### Response

**200 OK**
```json
{
  "status": "updated"
}
```

### Behavior

- Immediate update, no draining
- Active conversations continue with new key
- Useful for key rotation on compromised keys

### Error Responses

| Status | Error Code | Description |
|--------|------------|-------------|
| 401 | `unauthorized` | Invalid or missing bearer token |
| 404 | `agent_not_found` | Agent doesn't exist |

### Example

```bash
curl -X PATCH http://localhost:8080/push/agents/my-agent/api-key \
  -H "Authorization: Bearer <bridge-control-plane-api-key>" \
  -H "Content-Type: application/json" \
  -d '{"api_key": "<anthropic-api-key>"}'
```

---

## Apply Diff Update

Apply incremental changes to agents (add, update, remove).

### Request

```
POST /push/diff
```

### Request Body

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `added` | array | No | List of new agent definitions to add |
| `updated` | array | No | List of agent definitions to update |
| `removed` | array | No | List of agent IDs to remove |

```json
{
  "added": [],
  "updated": [
    {
      "id": "agent1",
      "name": "Updated Agent",
      "system_prompt": "Updated prompt...",
      "provider": {
        "provider_type": "anthropic",
        "model": "claude-sonnet-4-20250514",
        "api_key": "<anthropic-api-key>"
      }
    }
  ],
  "removed": ["old-agent-id"]
}
```

### Response

**200 OK**
```json
{
  "added": 0,
  "updated": 1,
  "removed": 1
}
```

### Response Fields

| Field | Type | Description |
|-------|------|-------------|
| `added` | number | Number of agents added |
| `updated` | number | Number of agents updated |
| `removed` | number | Number of agents removed |

### Error Responses

| Status | Error Code | Description |
|--------|------------|-------------|
| 401 | `unauthorized` | Invalid or missing bearer token |

### Use Case

Update many agents at once (e.g., changing a shared prompt template).

### Example

```bash
curl -X POST http://localhost:8080/push/diff \
  -H "Authorization: Bearer <bridge-control-plane-api-key>" \
  -H "Content-Type: application/json" \
  -d '{
    "added": [],
    "updated": [{"id": "agent1", "name": "Updated", ...}],
    "removed": ["agent-to-delete"]
  }'
```

---

## See Also

- [Pushing Agents](../control-plane/pushing-agents.md) — Complete guide
- [Hydrating Conversations](../control-plane/hydrating-conversations.md) — Restore history
- [Agents](../core-concepts/agents.md) — Agent lifecycle
