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
  "tools": [{"name": "read", "description": "...", "parameters_schema": {}}],
  "mcp_servers": [...],
  "skills": [...],
  "integrations": [...],
  "config": {
    "max_tokens": 4096,
    "max_turns": 50,
    "temperature": 0.2,
    "compaction": {
      "token_budget": 100000,
      "tail_messages": 10,
      "summary_provider": { ... }
    }
  },
  "subagents": [...],
  "permissions": {"bash": "require_approval"},
  "version": "3",
  "webhook_url": "https://...",
  "webhook_secret": "<webhook-secret>"
}
```

### Core Fields

| Field | Required | Type | Description |
|-------|----------|------|-------------|
| `id` | **Yes** | string | Unique identifier. Used in URLs like `/agents/{id}/conversations` |
| `name` | **Yes** | string | Human-readable name, shown in UIs |
| `system_prompt` | **Yes** | string | Instructions given to the AI at the start of every conversation |
| `provider` | **Yes** | object | LLM provider configuration (see below) |
| `description` | No | string | Human-readable description. Used in tool documentation when this agent is a subagent |
| `version` | No | string | String you control. Changing it triggers an agent update |
| `updated_at` | No | string | Last updated timestamp for change detection |

### Provider Configuration

The `provider` object is required and has the following fields:

| Field | Required | Type | Description |
|-------|----------|------|-------------|
| `provider_type` | **Yes** | string | LLM provider type. See valid values below |
| `model` | **Yes** | string | Model identifier (e.g., "gpt-4o", "claude-sonnet-4-20250514") |
| `api_key` | **Yes** | string | API key for authentication |
| `base_url` | No | string | Optional custom endpoint URL |

#### Valid Provider Types

| Value | Description | Aliases Accepted |
|-------|-------------|------------------|
| `open_ai` | OpenAI (GPT-4o, etc.) | `openai`, `open_ai` |
| `anthropic` | Anthropic (Claude, etc.) | `anthropic` |
| `google` | Google (Gemini, etc.) | `google` |
| `groq` | Groq | `groq` |
| `deep_seek` | DeepSeek | `deepseek`, `deep_seek` |
| `mistral` | Mistral | `mistral` |
| `cohere` | Cohere | `cohere` |
| `x_ai` | xAI (Grok, etc.) | `xai`, `x_ai` |
| `together` | Together AI | `together` |
| `fireworks` | Fireworks AI | `fireworks` |
| `ollama` | Ollama (local models) | `ollama` |
| `custom` | Custom provider with custom base URL | `custom` |

### Capabilities

| Field | Required | Type | Description |
|-------|----------|------|-------------|
| `tools` | No | array | List of built-in tools the agent can use. Empty array means all built-in tools available |
| `mcp_servers` | No | array | External MCP servers to connect to |
| `skills` | No | array | Reusable prompt templates the agent can invoke |
| `integrations` | No | array | External service integrations |
| `subagents` | No | array | Child agents this agent can spawn |

#### Tool Definition

Each tool in the `tools` array requires:

| Field | Required | Type | Description |
|-------|----------|------|-------------|
| `name` | **Yes** | string | Unique name of the tool |
| `description` | **Yes** | string | Human-readable description of what the tool does |
| `parameters_schema` | **Yes** | object | JSON Schema for the tool's parameters |

#### MCP Server Definition

Each MCP server in the `mcp_servers` array requires:

| Field | Required | Type | Description |
|-------|----------|------|-------------|
| `name` | **Yes** | string | Name of the MCP server |
| `transport` | **Yes** | object | Transport configuration (see below) |

**MCP Transport Types:**

1. **Stdio Transport** (`type: "stdio"`):
   - `command` (required): Command to execute
   - `args` (optional): Array of command arguments
   - `env` (optional): Map of environment variables

2. **Streamable HTTP Transport** (`type: "streamable_http"`):
   - `url` (required): Server URL
   - `headers` (optional): Map of additional HTTP headers

#### Skill Definition

Each skill in the `skills` array requires:

| Field | Required | Type | Description |
|-------|----------|------|-------------|
| `id` | **Yes** | string | Unique identifier for the skill |
| `title` | **Yes** | string | Human-readable title |
| `description` | **Yes** | string | Description of what the skill does |
| `content` | **Yes** | string | Full skill prompt/instructions content |
| `parameters_schema` | No | object | Optional JSON Schema for structured parameters |

#### Integration Definition

Each integration in the `integrations` array requires:

| Field | Required | Type | Description |
|-------|----------|------|-------------|
| `name` | **Yes** | string | Integration identifier (e.g., "github", "slack") |
| `description` | **Yes** | string | Human-readable description |
| `actions` | **Yes** | array | Available actions within this integration |

Each action in `actions` requires:

| Field | Required | Type | Description |
|-------|----------|------|-------------|
| `name` | **Yes** | string | Action identifier (e.g., "create_pull_request") |
| `description` | **Yes** | string | Human-readable description |
| `parameters_schema` | **Yes** | object | JSON Schema for the action's parameters |
| `permission` | **Yes** | string | Permission level: `allow`, `deny`, or `require_approval` |

### Configuration

The `config` object supports the following optional fields:

| Field | Type | Description | Constraints |
|-------|------|-------------|-------------|
| `max_tokens` | integer | Maximum tokens for LLM response | `>= 0` |
| `max_turns` | integer | Maximum back-and-forth exchanges before compaction | `>= 0` |
| `temperature` | number | Randomness (0 = deterministic, 1 = creative) | No explicit range |
| `json_schema` | object | JSON schema for structured output | Valid JSON Schema |
| `rate_limit_rpm` | integer | Rate limit in requests per minute | `>= 0` |
| `compaction` | object | Conversation compaction configuration | See below |
| `tool_calls_only` | boolean | Accept tool-only turns as success | Default: `false` |
| `max_tasks_per_conversation` | integer | Max subagent tasks per conversation | Default: `50` |
| `max_concurrent_conversations` | integer | Per-agent concurrent conversation limit | Overrides global setting |

#### `tool_calls_only`

When set to `true`, the agent can complete a turn with only tool calls and no text response. Normally, if the LLM produces tool calls but no accompanying text, Bridge treats this as an incomplete response and enters a recovery loop (continuation attempts, then a no-tools retry agent). With `tool_calls_only` enabled, tool-only turns are accepted as success.

**When to use:** Agents that primarily perform actions rather than generating text. For example, a background automation agent that reads files, runs commands, and writes results without needing to narrate what it did.

```json
{
  "config": {
    "tool_calls_only": true
  }
}
```

#### `max_tasks_per_conversation`

Maximum number of subagent tasks (foreground + background) that can be spawned within a single conversation. This limit is shared across the entire conversation tree, including nested subagents. When the limit is reached, further `spawn_agent` or `parallel_agent` calls return an error.

Default: `50`

**When to use:** Lower this for agents that should stay focused. Raise it for orchestration agents that legitimately need many parallel workers.

```json
{
  "config": {
    "max_tasks_per_conversation": 100
  }
}
```

#### `max_concurrent_conversations`

Per-agent limit on the number of concurrent conversations. Overrides the global `BRIDGE_MAX_CONCURRENT_CONVERSATIONS` environment variable for this specific agent. When the limit is reached, new conversation requests return **429 Too Many Requests**.

```json
{
  "config": {
    "max_concurrent_conversations": 10
  }
}
```

**When to use:** Protect expensive agents (e.g., using costly models) from being overwhelmed. Leave unset to use the global default.

#### Compaction Configuration

When `compaction` is specified, it controls how conversation history is summarized:

| Field | Required | Type | Description | Default |
|-------|----------|------|-------------|---------|
| `token_budget` | No | integer | Token threshold to trigger compaction | `100000` |
| `tail_messages` | No | integer | Recent messages to preserve after compaction | `10` |
| `summary_prompt` | No | string | Custom system prompt for summarization | Built-in default |
| `summary_provider` | **Yes** | object | Provider config for the summarization model | - |

### History Compaction

Long conversations accumulate tokens. Compaction keeps them manageable by summarizing older messages while preserving recent context.

#### What Triggers Compaction

Before each LLM call, Bridge estimates the total token count of the conversation. If the estimated tokens exceed the `compaction.token_budget` (default: 100,000), compaction runs automatically.

Token estimation uses the tiktoken `cl100k_base` tokenizer. A fast heuristic check runs first, and the full tokenizer is only invoked when the heuristic is close to the budget.

#### How It Works

1. Messages are split into two groups:
   - **Head** — all messages except the most recent N. These are summarized by a separate LLM call.
   - **Tail** — the most recent N messages (default: 10), preserved verbatim.
2. The summary LLM condenses the head into a single system message.
3. The conversation continues with the summary + tail messages, significantly reducing token count.

The summary LLM call uses the `summary_provider` configuration, which can be a different (cheaper/faster) model than the agent's main provider.

#### CompactionConfig Fields

| Field | Required | Type | Description | Default |
|-------|----------|------|-------------|---------|
| `token_budget` | No | integer | Token threshold that triggers compaction | `100000` |
| `tail_messages` | No | integer | Number of recent messages preserved verbatim | `10` |
| `summary_prompt` | No | string | Custom system prompt for the summarization LLM call | Built-in default |
| `summary_provider` | **Yes** | object | Provider configuration for the summarization model (same shape as agent `provider`) | - |

#### Events

When compaction occurs, Bridge fires a `ConversationCompacted` event. This can be observed via webhooks or the streaming API.

#### Recommended Configuration

**Long-running conversations** (support agents, coding assistants):

```json
{
  "compaction": {
    "token_budget": 80000,
    "tail_messages": 15,
    "summary_provider": {
      "provider_type": "anthropic",
      "model": "claude-haiku-4-20250514",
      "api_key": "<anthropic-api-key>"
    }
  }
}
```

Lower the budget to compact earlier, preserve more tail messages to retain recent context. Use a fast, cheap model for summarization.

**Short conversations** (single-task agents, Q&A bots):

Compaction is usually unnecessary. Either omit the `compaction` config entirely, or set a high budget:

```json
{
  "compaction": {
    "token_budget": 200000,
    "tail_messages": 5,
    "summary_provider": {
      "provider_type": "open_ai",
      "model": "gpt-4o-mini",
      "api_key": "<provider-api-key>"
    }
  }
}
```

---

### Permissions

The `permissions` object maps tool names to permission levels. Tools not listed default to `allow`.

| Permission Value | Behavior |
|------------------|----------|
| `allow` | Execute immediately without approval |
| `deny` | Block execution and return an error to the LLM |
| `require_approval` | Pause execution and wait for user approval via HTTP |

Example:
```json
{
  "permissions": {
    "bash": "require_approval",
    "write": "require_approval",
    "read": "allow"
  }
}
```

### Webhooks

| Field | Required | Type | Description |
|-------|----------|------|-------------|
| `webhook_url` | No | string | URL for event delivery |
| `webhook_secret` | No | string | Secret for HMAC signing of webhook payloads |

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

The version is a simple string with no format constraints. Any change to the version string triggers an update.

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

**Drain timeout:** 60 seconds (hardcoded)

If the timeout is reached, old conversations are forcefully dropped:
```
warn: drain timeout reached, forcing shutdown
```

During draining, both versions run simultaneously.

---

## System Limits and Constraints

### Subagent Depth
- **Maximum nesting depth:** 3 levels
- Attempting to spawn deeper results in error: "Maximum subagent depth (3) reached"

### Timeouts
- **Agent chat timeout:** 180 seconds per LLM call
- **Drain timeout:** 60 seconds during agent updates

### Parallel Agent Tool
- **Maximum tasks per call:** 25

### No Hard Limits On
The following have no explicit maximums in the code (practical limits apply):
- Number of tools per agent
- Number of skills per agent  
- Number of integrations per agent
- Number of subagents per agent
- Number of MCP servers per agent
- String field lengths (id, name, system_prompt, etc.)

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
      "id": "code-reviewer-v2",
      "name": "code_reviewer",
      "description": "Code review specialist",
      "system_prompt": "You are a code reviewer...",
      "provider": { ... },
      "config": { "max_turns": 10 }
    },
    {
      "id": "test-agent-v1",
      "name": "test_writer",
      "description": "Test writing specialist",
      "system_prompt": "You write tests...",
      "provider": { ... },
      "config": { "max_turns": 10 }
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

Subagents:
- Inherit parent's integration tools with the same permissions
- Get built-in tools but no MCP servers (to prevent unbounded recursion)
- Have their own `config` for max_turns, etc.
- Must include `description` for tool documentation

---

## See Also

- [Pushing Agents](../control-plane/pushing-agents.md) — How to push agent definitions
- [Tools](tools.md) — Tool system overview
- [MCP](mcp.md) — External tool servers
- [Skills](skills.md) — Reusable prompts
