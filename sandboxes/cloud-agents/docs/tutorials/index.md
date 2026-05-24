# Tutorials

Step-by-step guides for building with Bridge.

---

## Available Tutorials

| Tutorial | What You'll Build | Time |
|----------|-------------------|------|
| [Customer Support Agent](customer-support-agent.md) | An agent that helps customers with order lookups and refunds | 30 min |
| [Code Review Agent](code-review-agent.md) | An agent that reviews code and suggests improvements | 45 min |
| [Multi-Agent System](multi-agent-system.md) | A parent agent that delegates to specialized subagents | 40 min |
| [GitHub Integration](github-integration.md) | An agent that creates pull requests | 35 min |

---

## Tutorial Approach

Each tutorial follows this structure:

1. **What you'll build** — Overview of the end result
2. **Prerequisites** — What you need before starting
3. **Step-by-step** — Detailed instructions
4. **Complete code** — Full working example
5. **Next steps** — Where to go from here

---

## Before You Start

Make sure you have:

- Bridge installed and running
- An API key from an AI provider
- `curl` or similar for testing
- For integration tutorials: a control plane configured with the required integrations

Need help? Start with [Getting Started](../getting-started/index.md).

---

## Important Notes

### Tool Names Are Case-Sensitive

When specifying tools in your agent definitions, use the exact tool names:

| Correct | Incorrect |
|---------|-----------|
| `Read` | `read` |
| `Grep` | `grep` |
| `Glob` | `glob` |
| `write` | `Write` |
| `edit` | `Edit` |
| `bash` | `Bash` |
| `LS` | `ls` |
| `web_search` | `web-search` |

See [Tools Reference](../tools-reference/index.md) for the complete list.

### Integrations Require Control Plane Support

Integration actions are forwarded to your control plane at:
```
{control_plane_url}/integrations/{integration_name}/actions/{action_name}
```

Make sure your control plane implements the required integration endpoints.

### Approval API Format

When approving tool calls, use the correct JSON format:

```bash
# Bulk approval
curl -X POST http://localhost:8080/agents/{agent_id}/conversations/{conv_id}/approvals \
  -H "Content-Type: application/json" \
  -d '{
    "request_ids": ["req-abc123", "req-def456"],
    "decision": "approve"
  }'

# Single approval
curl -X POST http://localhost:8080/agents/{agent_id}/conversations/{conv_id}/approvals/{request_id} \
  -H "Content-Type: application/json" \
  -d '{"decision": "approve"}'
```

---

## Suggest a Tutorial

Missing a tutorial? Open an issue with:

- What you want to build
- Your use case
- What you struggled with

We add tutorials based on community needs.
