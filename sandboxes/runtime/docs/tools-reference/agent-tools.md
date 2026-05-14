# Agent Tools

Delegate work to subagents. Single tool, two modes: blocking and background.

---

## Overview

Bridge ships two subagent tools:

- **`agent`** — Delegates to a clone of the parent agent (self-delegation).
- **`sub_agent`** — Delegates to a named subagent defined in the agent's `subagents` list.

Both tools support a `run_in_background` flag. When set, the call returns immediately with a `task_id` and the subagent runs asynchronously; its final output is automatically injected into the parent's next user turn as a `[Background Agent Task Completed]` message. There is no separate join/wait tool — the parent simply keeps working and picks up the result whenever it arrives.

Fan-out is achieved by the LLM emitting multiple `sub_agent` tool_use blocks in a single assistant turn. The runtime executes them in parallel. No dedicated "parallel agent" tool is needed.

---

## sub_agent

Spawn a named subagent.

### Parameters

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `subagentName` | string | Yes | Subagent name (must match a subagent defined on the parent). |
| `prompt` | string | Yes | Detailed task description. |
| `description` | string | Yes | Short (3–5 word) title, e.g. `"Review auth flow"`. |
| `runInBackground` | boolean | No | When `true`, returns immediately with a `task_id`; the result is injected into the next user turn. Default: `false`. |
| `taskId` | string | No | Resume a prior subagent session by its `task_id`. |

### Foreground (default)

Blocks until the subagent finishes, then returns its full output as the tool result.

```json
{
  "name": "sub_agent",
  "arguments": {
    "subagentName": "code_reviewer",
    "description": "Review auth function",
    "prompt": "Review this function for security issues:\n\nfunction authenticate(user, pass) { ... }"
  }
}
```

**Response**

```
task_id: conv-abc-task-xyz (for resuming)

<task_result>
Result from the subagent...
</task_result>
```

### Background

Returns immediately. The subagent continues running; when it finishes, the parent sees a user-turn message on its next turn:

```
[Background Agent Task Completed]
task_id: conv-abc-task-xyz
description: Review auth function

<task_result>
...full output...
</task_result>
```

```json
{
  "name": "sub_agent",
  "arguments": {
    "subagentName": "explorer",
    "description": "Map API surface",
    "prompt": "Enumerate every HTTP endpoint in crates/api and summarize their shapes.",
    "runInBackground": true
  }
}
```

**Immediate response**

```json
{
  "task_id": "conv-abc-task-xyz",
  "status": "running",
  "message": "Background subagent started. Its final output will appear in your next user turn — do not poll or wait."
}
```

**Do not** poll, sleep, or call a wait tool. The delivery is automatic.

### Parallel fan-out

To run several subagents concurrently, emit multiple `sub_agent` tool_use blocks in a single assistant turn. The runtime schedules them in parallel and they each produce their own tool_result.

```json
[
  { "name": "sub_agent", "arguments": { "subagentName": "code_reviewer", "description": "Review auth.js", "prompt": "Review src/auth.js ..." } },
  { "name": "sub_agent", "arguments": { "subagentName": "code_reviewer", "description": "Review db.js",   "prompt": "Review src/db.js ..." } },
  { "name": "sub_agent", "arguments": { "subagentName": "code_reviewer", "description": "Review api.js",  "prompt": "Review src/api.js ..." } }
]
```

Combine with `runInBackground: true` to fan out long-running work and return control to the parent agent without waiting on any of them.

### Resuming a subagent session

Pass a prior `task_id` to continue the same subagent conversation with its history intact.

```json
{
  "name": "sub_agent",
  "arguments": {
    "subagentName": "code_reviewer",
    "description": "Continue review",
    "prompt": "Now check the utils.js file too",
    "taskId": "conv-abc-task-xyz"
  }
}
```

---

## agent (self-delegation)

Identical parameters to `sub_agent`, except there is no `subagentName` — it always targets a clone of the parent agent. Useful for spinning up a scratch workspace that shares the parent's tool set without polluting its context.

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `prompt` | string | Yes | Detailed task description. |
| `description` | string | Yes | Short (3–5 word) title. |
| `runInBackground` | boolean | No | Same semantics as on `sub_agent`. |
| `taskId` | string | No | Resume a previous self-delegation session. |

---

## Session persistence and resumption

Every subagent invocation is assigned a `task_id` of the form `{conversation_uuid}-{task_uuid}`. Passing that id back as `taskId` resumes the same subagent conversation with its full history.

- **In-memory (default):** sessions live in a concurrent hash map — fast, lost on restart.
- **Persistent:** when `BRIDGE_STORAGE_PATH` is set, subagent sessions are also written to SQLite. On restart, sessions are restored from disk and a background task that was mid-flight can be resumed by its `task_id`.

Sessions are cleaned up when the parent conversation ends (DELETE, max_turns, cancellation).

---

## Task budget

Each conversation has a per-conversation budget limiting the total number of subagent tasks spawned across its tree.

| Setting | Default | Description |
|---------|---------|-------------|
| `config.max_tasks_per_conversation` | 50 | Maximum subagent tasks per conversation (foreground + background, across all depths). |

When exhausted, further `agent` / `sub_agent` calls return:

```
Error: "Task budget exhausted: 50 of 50 task slots used. Wait for existing tasks to complete before spawning more."
```

---

## Defining subagents

Declare subagents in your agent definition:

```json
{
  "id": "senior-engineer",
  "name": "Senior Engineer",
  "system_prompt": "You are a senior engineer...",
  "subagents": [
    {
      "id": "code-reviewer-v2",
      "name": "code_reviewer",
      "description": "Security-focused code reviewer.",
      "system_prompt": "You are a code reviewer. Focus on security issues...",
      "provider": {
        "provider_type": "open_ai",
        "model": "gpt-4o",
        "api_key": "${OPENAI_API_KEY}"
      },
      "config": { "max_turns": 10 }
    },
    {
      "id": "test-writer-v1",
      "name": "test_writer",
      "description": "Test generation specialist.",
      "system_prompt": "You write comprehensive unit tests...",
      "provider": {
        "provider_type": "open_ai",
        "model": "gpt-4o",
        "api_key": "${OPENAI_API_KEY}"
      }
    }
  ]
}
```

### Subagent fields

| Field | Required | Description |
|-------|----------|-------------|
| `id` | Yes | Unique identifier. |
| `name` | Yes | Name used by the `subagentName` parameter. |
| `description` | Yes | Shown to the parent in the available-subagents list. |
| `system_prompt` | Yes | Subagent system prompt. |
| `provider` | Yes | LLM provider configuration. |
| `config` | No | Agent config (max_tokens, max_turns, temperature, etc.). |
| `tools` | No | Tool allowlist for this subagent. |
| `skills` | No | Skills available to this subagent. |
| `subagents` | No | Nested subagents (respects depth limit). |

---

## Complete example: review all JS files

```
User: "Review every JavaScript file in src/"

Agent:
  1. Glob    — find files
  2. sub_agent × N in parallel (one tool_use block per file)
  3. Summarize findings to the user
```

```json
// 1. Enumerate files
{ "name": "Glob", "arguments": { "pattern": "src/**/*.js" } }
```

```json
// 2. Three sub_agent calls emitted in a single assistant turn → run in parallel
{ "name": "sub_agent", "arguments": { "subagentName": "code_reviewer", "description": "Review auth.js", "prompt": "Review /project/src/auth.js" } }
{ "name": "sub_agent", "arguments": { "subagentName": "code_reviewer", "description": "Review db.js",   "prompt": "Review /project/src/db.js" } }
{ "name": "sub_agent", "arguments": { "subagentName": "code_reviewer", "description": "Review api.js",  "prompt": "Review /project/src/api.js" } }
```

For very long reviews, pass `runInBackground: true` and let the parent continue working (e.g., start reading docs) while each review completes in the background.

---

## Limits and timeouts

| Limit | Value |
|-------|-------|
| Foreground subagent timeout | 120 seconds |
| Background subagent timeout | 300 seconds (5 minutes) |
| Subagent nesting depth | 3 |
| Task budget per conversation | 50 (configurable via `config.max_tasks_per_conversation`) |

### Depth

- Level 0: Parent agent
- Level 1: Subagent
- Level 2: Subagent's subagent
- Level 3: Maximum

Exceeding depth returns: `"Maximum subagent depth (3) reached"`.

### Common errors

| Trigger | Error |
|---------|-------|
| Unknown subagent name | `Unknown subagent 'xyz'. Available: [abc, def]` |
| Depth limit exceeded | `Maximum subagent depth (3) reached` |
| Budget exhausted | `Task budget exhausted: N of N task slots used.` |
| Foreground timeout | `Subagent timed out after 120s` |
| Background timeout | `Background subagent timed out after 300s` (surfaces in the injected user-turn message as `[ERROR]`) |

---

## Best practices

### Keep subagent tasks focused

- **Good:** "Review src/auth.js for security issues"
- **Too broad:** "Review the entire codebase"

### Match subagent to task

Use specialized subagents — security reviewer, performance reviewer, style reviewer — rather than one generalist. Bridge shows only subagent name + description to the parent, so good descriptions are critical.

### Choose the right mode

| Use case | Mode | Why |
|----------|------|-----|
| Short task, need the result to continue | Foreground `sub_agent` | Blocks, returns output inline. |
| Independent tasks you need all of before proceeding | Multiple foreground `sub_agent` tool_use blocks in one turn | Runs in parallel, all outputs available next turn. |
| Long-running exploration, parent has other work | `runInBackground: true` | Parent continues; result appears automatically in a later turn. |
| Small mechanical work (file reads, etc.) | `batch` tool | Lower overhead than spawning an agent. |

### Don't poll

A background subagent's result is pushed as a user-turn injection. Never sleep, loop, or issue a "check status" call — the runtime drops the completion into your next turn automatically.

---

## See also

- [batch](batch-tool.md) — Parallel tool execution with lower overhead than subagents.
- [Agents](../core-concepts/agents.md) — Agent configuration.
