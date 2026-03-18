# Agents

An agent is a complete AI configuration. Think of it as a job description for an AI worker.

---

## What Makes Up an Agent?

```json
{
  "id": "code-reviewer",
  "name": "Code Reviewer",
  "system_prompt": "You are a senior engineer...",
  "provider": {
    "provider_type": "anthropic",
    "model": "claude-sonnet-4-20250514",
    "api_key": "<anthropic-api-key>"
  },
  "tools": ["read", "edit", "bash"],
  "mcp_servers": [...],
  "skills": [...],
  "integrations": [...],
  "config": {
    "max_tokens": 4096,
    "max_turns": 50,
    "temperature": 0.2
  },
  "subagents": [...],
  "permissions": {...},
  "version": "3"
}
```

### Core Fields

| Field | What it does |
|-------|--------------|
| `id` | Unique identifier. Used in URLs like `/agents/{id}/conversations` |
| `name` | Human-readable name, shown in UIs |
| `system_prompt` | Instructions given to the AI at the start of every conversation |
| `provider` | Which AI model to use and how to connect to it |
| `version` | String you control. Changing it triggers an agent update |

### Capabilities

| Field | What it does |
|-------|--------------|
| `tools` | List of built-in tools the agent can use |
| `mcp_servers` | External MCP servers to connect to |
| `skills` | Reusable prompt templates the agent can invoke |
| `integrations` | External service integrations |

### Configuration

| Field | What it does |
|-------|--------------|
| `config.max_tokens` | Maximum tokens per response |
| `config.max_turns` | Maximum back-and-forth exchanges before compaction |
| `config.temperature` | Randomness (0 = deterministic, 1 = creative) |
| `config.compaction` | How to summarize old conversation history |

### Permissions

| Field | What it does |
|-------|--------------|
| `permissions` | Map of tool names to permission levels |
| `subagents` | Child agents this agent can spawn |

---

## Agent Lifecycle

```
1. DEFINED (in your control plane)
   You write the agent definition
   ↓
2. PUSHED (to Bridge)
   POST /push/agents
   Bridge stores it in memory
   ↓
3. ACTIVE (ready for conversations)
   Users can create conversations with this agent
   ↓
4. DRAINING (during updates)
   Old version finishes active conversations
   New version takes new conversations
   ↓
5. REPLACED (update complete)
   Old version removed, new version fully active
```

---

## Versioning

The `version` field controls updates:

- **Same version** → No change (idempotent)
- **Different version** → Drain and replace

Example:

```bash
# Push version 1
curl -X PUT /push/agents/my-agent -d '{"version": "1", ...}'
# Response: {"status": "created"}

# Push version 1 again — no change
curl -X PUT /push/agents/my-agent -d '{"version": "1", ...}'
# Response: {"status": "unchanged"}

# Push version 2 — triggers update
curl -X PUT /push/agents/my-agent -d '{"version": "2", ...}'
# Response: {"status": "updated"}
```

---

## Draining Explained

When you update an agent, Bridge doesn't just kill active conversations. It:

1. Marks the old agent version as "draining"
2. Lets existing conversations finish naturally
3. Routes new conversations to the new version
4. Removes the old version when all conversations complete

You can configure the drain timeout:

```toml
drain_timeout_secs = 60  # Wait up to 60 seconds before forcing shutdown
```

During draining, both versions run simultaneously.

---

## System Prompt Best Practices

The system prompt shapes the agent's behavior:

### Be Specific

**❌ Vague:**
> Help users with their questions.

**✅ Specific:**
> You are a technical support agent for Acme Cloud Services. You help customers with:
> - Account access issues
> - Billing questions
> - API usage problems
> 
> Always verify the customer's account ID before discussing billing.

### Define Constraints

```
You are a code reviewer. Follow these rules:
- Check for security issues first
- Flag any use of eval() or innerHTML
- Suggest specific improvements, not just "this could be better"
- Format your response as a checklist
```

### Set the Format

```
Respond in this format:
THOUGHT: Brief reasoning about what you found
ACTION: What you're doing about it
RESULT: The actual response to the user
```

---

## Tool Selection

Choose tools based on what the agent needs to do:

| Agent Type | Typical Tools |
|------------|---------------|
| Customer support | `web_search` for looking up docs, `integration` for CRM lookup |
| Code review | `read`, `edit`, `bash` for git commands, `lsp_query` for type info |
| Data analysis | `read` for CSVs, `bash` for data processing, `write` for reports |
| DevOps assistant | `bash`, `read`, `edit` for config files |

Start minimal. Add tools when the agent actually needs them.

---

## Subagents

Agents can spawn other agents for specialized tasks:

```json
{
  "id": "parent-agent",
  "subagents": [
    {
      "name": "code_reviewer",
      "agent_id": "code-reviewer-v2"
    },
    {
      "name": "test_writer",
      "agent_id": "test-agent-v1"
    }
  ]
}
```

The parent can call `spawn_agent` to delegate work:

```json
{
  "name": "spawn_agent",
  "arguments": {
    "agent": "code_reviewer",
    "prompt": "Review this function for security issues..."
  }
}
```

---

## See Also

- [Pushing Agents](../control-plane/pushing-agents.md) — How to push agent definitions
- [Tools](tools.md) — Tool system overview
- [MCP](mcp.md) — External tool servers
- [Skills](skills.md) — Reusable prompts
