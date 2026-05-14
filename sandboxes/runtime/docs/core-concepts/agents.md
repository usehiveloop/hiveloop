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
  "artifacts": {
    "upload_url": "https://control-plane.example.com/workspaces/ws_42/uploads",
    "max_size_bytes": 524288000,
    "accepted_file_types": ["csv", "md", "video/*"]
  },
  "config": {
    "max_tokens": 4096,
    "max_turns": 50,
    "temperature": 0.2,
    "immortal": {
      "token_budget": 100000,
      "retention_window": 10,
      "expose_journal_tools": true
    },
    "history_strip": {
      "enabled": true,
      "age_threshold": 10,
      "pin_recent_count": 3,
      "pin_errors": true
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
| `artifacts` | No | object | Workspace artifact upload configuration. When present, bridge auto-registers an `upload_to_workspace` tool. See [Artifacts Definition](#artifacts-definition). |
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

#### Artifacts Definition

Set `artifacts` to enable workspace file uploads. When present, bridge auto-registers a single tool — `upload_to_workspace` — backed by a tus.io v1.0.0 resumable upload client.

| Field | Required | Type | Description |
|-------|----------|------|-------------|
| `upload_url` | **Yes** | string | tus.io creation endpoint on the control plane. Must be `http`/`https`. |
| `download_url` | No | string | Optional canonical download URL surfaced back to the agent in the tool response. |
| `max_size_bytes` | **Yes** | integer | Hard ceiling enforced before any network I/O. Must be `> 0`. |
| `accepted_file_types` | **Yes** | string[] | Each entry is either a bare extension (`csv`) or a MIME type (`text/csv`, `video/*`). Must be non-empty. |
| `max_concurrent_uploads` | No | integer | Per-agent concurrency cap. Default `4`. Must be `> 0`. |
| `chunk_size_bytes` | No | integer | PATCH chunk size. Default `8 MiB` (8388608). Must be `> 0`. |
| `headers` | No | object | Extra `string → string` headers forwarded on every TUS request (creation + chunks). |

The `upload_to_workspace` tool takes `path` (absolute, inside the sandbox), optional `content_type` MIME override, and optional free-form `metadata` (`string → string` map; ASCII keys without spaces or commas). Bridge persists in-flight upload state to the local sqlite (when `BRIDGE_STORAGE_PATH` is set) so a re-call after a process restart resumes from the last server-acknowledged offset. Companion tools like `search_workspace` and `download_from_workspace` are out of scope for bridge — the control plane wires them in via `mcp_servers` if needed.

See the [Workspace Artifacts](../../README.md#workspace-artifacts) section in the README for the full configuration table, response shape, and resilience guarantees.

### Configuration

The `config` object supports the following optional fields:

| Field | Type | Description | Constraints |
|-------|------|-------------|-------------|
| `max_tokens` | integer | Maximum tokens for LLM response | `>= 0` |
| `max_turns` | integer | Maximum back-and-forth exchanges before compaction | `>= 0` |
| `temperature` | number | Randomness (0 = deterministic, 1 = creative) | No explicit range |
| `json_schema` | object | JSON schema for structured output | Valid JSON Schema |
| `rate_limit_rpm` | integer | Rate limit in requests per minute | `>= 0` |
| `immortal` | object | Immortal-conversation configuration (in-place forgecode-style compaction). When set, conversations chain into fresh context windows transparently. | See [Immortal Mode](#immortal-mode) |
| `history_strip` | object | Strip tool-result bodies from old messages before sending history to the LLM. Independent of immortal mode; applied at every send. | See [History Stripping](#history-stripping). Default: enabled. |
| `system_reminder_refresh_turns` | integer | Re-emit the stable system reminder (skills, subagents, todos) every N turns at the head of the user message. Always emitted on turn 0; thereafter on turns where `turn_count % N == 0`. | Default: `10`. Values `<1` clamp to `1`. |
| `tool_calls_only` | boolean | Accept tool-only turns as success | Default: `false` |
| `max_tasks_per_conversation` | integer | Max subagent tasks per conversation | Default: `50` |
| `max_concurrent_conversations` | integer | Per-agent concurrent conversation limit | Overrides global setting |
| `disabled_tools` | string[] | Tools to completely remove from the agent | Default: `[]` |
| `tool_requirements` | object[] | Declarative per-turn tool-call requirements | See [Tool Requirements](#tool-requirements) |
| `subagent_timeout_foreground_secs` | integer | Wall-clock timeout (seconds) when this agent is invoked as a foreground subagent. | Default: `300` |
| `subagent_timeout_background_secs` | integer | Wall-clock timeout (seconds) when this agent is invoked as a background subagent. | Default: `300` |

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

Maximum number of subagent tasks (foreground + background) that can be spawned within a single conversation. This limit is shared across the entire conversation tree, including nested subagents. When the limit is reached, further `agent` or `sub_agent` calls return an error.

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

#### Tool Requirements

`tool_requirements` declares tools that the agent **must** call under a given cadence, position, and minimum-call policy. Bridge evaluates every successful turn against each requirement and, on violation, either surfaces a warning or nudges the model on the next turn.

Typical uses:

- Force a journal/audit-log write every turn.
- Recall memory at the *start* of every turn (before any other work).
- Retain memory at the *end* of every few turns.
- Require a first-turn workspace-scan before anything else runs.

##### `ToolRequirement` fields

| Field | Required | Type | Description | Default |
|-------|----------|------|-------------|---------|
| `tool` | **Yes** | string | Tool name to require (built-in, MCP, integration, or custom). See [Tool-name matching](#tool-name-matching). | — |
| `cadence` | No | object | Which turns must satisfy this requirement. See [Cadence](#cadence). | `{ "type": "every_turn" }` |
| `position` | No | string | Where in the turn's tool-call sequence the call must appear: `"anywhere"`, `"turn_start"`, `"turn_end"`. | `"anywhere"` |
| `min_calls` | No | integer (≥1) | Minimum number of calls required in a qualifying turn. | `1` |
| `enforcement` | No | string | What bridge does on violation: `"next_turn_reminder"`, `"reprompt"`, `"warn"`. | `"next_turn_reminder"` |
| `reminder_message` | No | string | Custom reminder text injected on violation. Falls back to a generated default naming the tool and reason. | — |

##### Cadence

Cadence describes which turns must satisfy the requirement. It's a tagged object: `{ "type": "…" }` with optional inline fields.

| Type | Fields | Meaning |
|------|--------|---------|
| `every_turn` | — | Required on every single turn. |
| `first_turn_only` | — | Required only on the very first turn of the conversation. |
| `every_n_turns` | `n: integer` | Required whenever `n` turns have passed without the tool being called. "Reset on call" semantics — any successful call (on- or off-cycle) resets the counter. So `n=3` means "don't go more than 3 consecutive turns without calling this tool." |

##### Position

Position controls *where* the required call must sit among the turn's tool calls. Evaluation is **lenient**: read-only/metadata tools (`todoread`, `journal_read`) are exempt and don't disqualify a `turn_start` constraint.

| Value | Meaning |
|-------|---------|
| `anywhere` | The call may appear anywhere in the turn. |
| `turn_start` | Must come before any *substantive* (non-exempt) tool call this turn. |
| `turn_end` | Must come after any substantive tool call — i.e. be the final action. |

##### Enforcement

What bridge does when a requirement is violated.

| Value | Behavior | Cost |
|-------|----------|------|
| `next_turn_reminder` (default) | Emits a `tool_requirement_violated` event and attaches a `<system-reminder>` block to the **next** user message naming the missing tool(s). | No extra LLM call. |
| `reprompt` | Currently behaves like `next_turn_reminder` and logs a note. A future bridge release will make this re-run the agent synchronously in the same turn. | (future: +1 LLM call per violation) |
| `warn` | Emits the violation event and logs a warning but does not nudge the agent. | Observability only. |

##### Tool-name matching

To reduce MCP verbosity, `tool` matching is flexible:

- If `tool` contains `__` → **exact match** only (you opted into the full MCP tool name).
- Otherwise → matches the tool verbatim OR any registered tool whose full name ends with `__{tool}`.

So `"tool": "post_message"` matches `slack__post_message`, `discord__post_message`, *or* a plain `post_message` tool. If you need strict single-server binding, write the full name: `"slack__post_message"`.

Matching is case-sensitive.

##### Enforcement state

State is per-conversation:

- A turn counter bumps on every successful agent turn.
- Per-requirement "last satisfied turn" is recorded whenever the tool is actually called, regardless of whether other constraints were met. That lets `every_n_turns` cadence reset correctly on off-cycle calls.

Failed/rolled-back turns do not bump the counter.

##### Validation

Bridge rejects agent pushes where a `tool_requirements[i].tool` also appears in `disabled_tools` — that configuration would be unsatisfiable. Response: **400 Invalid Request** with a message identifying the conflicting tool.

##### Examples

**Journal every turn (what most agents want):**

```json
{
  "config": {
    "tool_requirements": [
      { "tool": "journal_write" }
    ]
  }
}
```

**Memory pattern (recall at turn start, retain every 3 turns):**

```json
{
  "config": {
    "tool_requirements": [
      {
        "tool": "memory_recall",
        "position": "turn_start",
        "reminder_message": "You must call memory_recall at the start of every turn before any other work."
      },
      {
        "tool": "memory_retain",
        "cadence": { "type": "every_n_turns", "n": 3 },
        "position": "turn_end",
        "reminder_message": "Retain new memory before finishing this turn."
      }
    ]
  }
}
```

**First-turn-only setup:**

```json
{
  "config": {
    "tool_requirements": [
      {
        "tool": "workspace_scan",
        "cadence": { "type": "first_turn_only" },
        "position": "turn_start"
      }
    ]
  }
}
```

**MCP tool without the server prefix (matches any `*__post_message` registration):**

```json
{
  "config": {
    "tool_requirements": [
      { "tool": "post_message", "enforcement": "warn" }
    ]
  }
}
```

**Minimum N calls per qualifying turn:**

```json
{
  "config": {
    "tool_requirements": [
      {
        "tool": "audit_log",
        "cadence": { "type": "every_turn" },
        "min_calls": 2
      }
    ]
  }
}
```

##### Event emitted on violation

Every violation fires an event (SSE + webhook + WebSocket, via the unified event bus). The payload includes the tool name, the reason (`InsufficientCalls` / `NotAtTurnStart` / `NotAtTurnEnd`), the enforcement variant, and the turn number:

```json
{
  "event_type": "agent_error",
  "data": {
    "code": "tool_requirement_violated",
    "tool": "memory_recall",
    "reason": "NotAtTurnStart",
    "enforcement": "NextTurnReminder",
    "turn": 4
  }
}
```

Clients that want to display a UI indicator or short-circuit a workflow should listen for this event. If you set `enforcement: "warn"`, this event is the only signal you get — no reminder attaches to the next turn.

### Immortal Mode

Long conversations accumulate tokens. Immortal mode keeps them running indefinitely by compacting the eligible head of history **in place** — no LLM summarization call, no separate summary message. The compactor replaces the eligible slice with a single user message containing a structured summary derived from the messages it replaced (forgecode-style). This is pure code, deterministic, and free.

When set, bridge also exposes `journal_read` / `journal_write` tools so the agent can record durable notes across context resets — opt out with `expose_journal_tools: false`.

#### What Triggers Compaction

Before each LLM call, bridge estimates total history tokens with the tiktoken `cl100k_base` tokenizer. If the estimate exceeds `immortal.token_budget`, compaction runs immediately. A second hook runs mid–rig-loop so single-bridge-turn agents (which spend most of their wall-clock inside the LLM provider's tool loop) compact too.

#### How It Works

1. The history is split:
   - **Retention tail** — the most recent `retention_window` messages, preserved verbatim.
   - **Eligible head** — everything between the initial user message and the retention tail. Up to `eviction_window` (fraction) of that range is the compactable slice.
2. The compactable slice is replaced in place with one user message containing a structured summary of what it carried (tool calls, decisions, file edits, etc.). The system prompt and the very first user message are never touched.

#### `ImmortalConfig` Fields

| Field | Required | Type | Description | Default |
|-------|----------|------|-------------|---------|
| `token_budget` | No | integer | Token threshold that triggers compaction. | `100000` |
| `retention_window` | No | integer | Number of most-recent messages preserved verbatim. Higher values keep more recent context pristine but shrink the eligible compaction range. | `0` |
| `eviction_window` | No | number | Maximum fraction (0.0–1.0) of total tokens eligible for compaction in any single pass. Lower values keep the head more stable across compactions; each pass takes a smaller slice. | `1.0` |
| `expose_journal_tools` | No | boolean | When true, registers `journal_read` / `journal_write` for the agent. The journal is the agent's own scratchpad — no longer read or written by the compaction engine itself. | `true` |

#### Events

Compaction fires a `ConversationCompacted` event observable via webhooks or the streaming API.

#### Recommended Configuration

**Long-running conversations** (support agents, coding assistants):

```json
{
  "immortal": {
    "token_budget": 80000,
    "retention_window": 20,
    "eviction_window": 0.5,
    "expose_journal_tools": true
  }
}
```

Lower the budget to compact earlier, raise `retention_window` to keep more recent context pristine, and lower `eviction_window` to trim the head more gradually.

**Short conversations** (single-task agents, Q&A bots): omit `immortal` entirely.

### History Stripping

Independent of immortal mode and applied at every send. Replaces the bodies of old tool results with markers (`<stripped>`) before the messages reach the LLM, while leaving everything else intact. The full bytes still live on disk via the spill pipeline; the agent can `RipGrep` the spill to recover the original content. Default: enabled.

#### `HistoryStripConfig` Fields

| Field | Required | Type | Description | Default |
|-------|----------|------|-------------|---------|
| `enabled` | No | boolean | Master switch. When false, strip is a no-op. | `true` |
| `age_threshold` | No | integer | Number of assistant messages that must follow a tool result before it becomes eligible for stripping. | `10` |
| `pin_recent_count` | No | integer | Always keep the most recent N tool results regardless of age. | `3` |
| `pin_errors` | No | boolean | When true, tool results with `is_error: true` are never stripped. | `true` |

To disable: `"history_strip": { "enabled": false }`. To turn off pinning of errors: `"pin_errors": false`. To keep all results indefinitely: raise `pin_recent_count`.

---

### Verifier

A small second LLM that judges, after every terminal-text turn, whether the agent really finished or stopped prematurely. On `needs_work + high` (within cap) it injects a synthetic user message and resumes the same turn — the agent sees what looks like a normal follow-up and keeps going. Off by default; opt in per agent via `config.verifier`.

```json
"verifier": {
  "enabled": true,
  "primary": {
    "provider": "open_ai",
    "model": "openai/gpt-5.4-nano",
    "api_key": "${OPENROUTER_API_KEY}",
    "base_url": "https://openrouter.ai/api/v1"
  },
  "max_reprompts_per_turn": 2,
  "require_high_confidence": true,
  "timeout_ms": 20000
}
```

Verifier failures (timeouts, HTTP errors, parse failures) never block the agent — they emit `verifier_error` and proceed. Force-disable globally with `BRIDGE_VERIFIER_DISABLED=1`. Full field reference, projection rules, verdict shape, and recommended configurations: [Verifier Agent](verifier-agent.md).

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
