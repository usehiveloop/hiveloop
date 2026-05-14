# Integration Tools

External service connectors managed by your control plane, exposed to agents as callable tools.

---

## What Are Integrations?

Integrations connect agents to external services like GitHub, Slack, Jira, or any custom API. Unlike MCP servers (which run as separate processes), integrations are HTTP endpoints managed by your control plane. Bridge calls them on the agent's behalf.

Each integration defines a set of **actions**. Each action becomes a tool the agent can call.

---

## Defining Integrations

Integrations are defined in the agent's `integrations` array:

```json
{
  "id": "dev-assistant",
  "integrations": [
    {
      "name": "github",
      "description": "GitHub repository management",
      "actions": [
        {
          "name": "create_pull_request",
          "description": "Create a new pull request in a repository",
          "parameters_schema": {
            "type": "object",
            "properties": {
              "repo": { "type": "string", "description": "Repository in owner/name format" },
              "title": { "type": "string", "description": "PR title" },
              "body": { "type": "string", "description": "PR description" },
              "head": { "type": "string", "description": "Branch to merge from" },
              "base": { "type": "string", "description": "Branch to merge into" }
            },
            "required": ["repo", "title", "head", "base"]
          },
          "permission": "require_approval"
        },
        {
          "name": "list_issues",
          "description": "List open issues in a repository",
          "parameters_schema": {
            "type": "object",
            "properties": {
              "repo": { "type": "string", "description": "Repository in owner/name format" },
              "state": { "type": "string", "enum": ["open", "closed", "all"] }
            },
            "required": ["repo"]
          },
          "permission": "allow"
        }
      ]
    },
    {
      "name": "slack",
      "description": "Slack messaging",
      "actions": [
        {
          "name": "send_message",
          "description": "Send a message to a Slack channel",
          "parameters_schema": {
            "type": "object",
            "properties": {
              "channel": { "type": "string" },
              "text": { "type": "string" }
            },
            "required": ["channel", "text"]
          },
          "permission": "allow"
        }
      ]
    }
  ]
}
```

---

## How Actions Become Tools

Each action is registered as a tool with the naming convention:

```
{integration_name}__{action_name}
```

The double underscore (`__`) separates the integration name from the action name.

From the example above, the agent would have these tools available:
- `github__create_pull_request`
- `github__list_issues`
- `slack__send_message`

The agent calls them just like any other tool:

```json
{
  "name": "github__create_pull_request",
  "arguments": {
    "repo": "myorg/myapp",
    "title": "Fix login bug",
    "head": "fix/login",
    "base": "main"
  }
}
```

---

## Execution Flow

When the agent calls an integration tool, Bridge sends an HTTP POST to your control plane:

```
POST {BRIDGE_CONTROL_PLANE_URL}/integrations/{integration_name}/actions/{action_name}
Content-Type: application/json

{
  "params": {
    "repo": "myorg/myapp",
    "title": "Fix login bug",
    "head": "fix/login",
    "base": "main"
  }
}
```

Your control plane handles the actual API call to the external service (GitHub, Slack, etc.) and returns the result.

---

## Permission Levels

Each action specifies a permission level:

| Permission | Behavior |
|------------|----------|
| `allow` | Executes immediately |
| `deny` | Action is never exposed to the LLM (completely hidden) |
| `require_approval` | Pauses execution, waits for user approval via HTTP |

```json
{
  "name": "delete_repository",
  "description": "Delete a repository",
  "permission": "deny"
}
```

Actions with `deny` permission are not registered as tools at all -- the LLM never sees them. Non-`allow` permissions are automatically injected into the agent's permissions map.

### Approval Flow

For `require_approval` actions:

1. Agent calls the integration tool
2. Bridge emits a `tool_approval_required` event
3. Your frontend presents the request to the user
4. User approves or denies via the HTTP API
5. If approved, Bridge executes the action; if denied, the agent receives an error

---

## Retry Behavior

Integration tool calls use exponential backoff for transient failures:

| Setting | Value |
|---------|-------|
| Initial delay | 500ms |
| Maximum delay | 5s |
| Maximum attempts | 3 |
| Request timeout | 30s |

### Error Handling

- **4xx responses** are returned to the agent as-is (client errors are not retried)
- **5xx responses** are retried with exponential backoff
- After all retries are exhausted, the error is returned to the agent

---

## Subagent Inheritance

Subagents automatically inherit their parent's integration tools with the same permission levels. This means:

- A subagent can call `github__list_issues` if its parent has that integration
- Permission levels carry over -- if `create_pull_request` requires approval for the parent, it requires approval for the subagent too
- Subagents do not need their own `integrations` array for inherited tools

---

## See Also

- [Tools Concept](../core-concepts/tools.md) -- How tools work
- [Custom Tools](custom-tools.md) -- Building your own tools
- [Agents](../core-concepts/agents.md) -- Agent configuration including integrations
