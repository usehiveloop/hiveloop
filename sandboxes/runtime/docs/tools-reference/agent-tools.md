# Agent Tools

Spawn and manage subagents for parallel work.

---

## Overview

These tools let agents delegate work to other agents:

- **agent** — Start a subagent to handle a task (foreground or background)
- **parallel_agent** — Start multiple subagents at once with concurrency control
- **join** — Wait for background subagents to finish and get results

Useful for:
- Parallel processing (analyze 10 files at once)
- Specialization (different agents for different tasks)
- Fan-out/fan-in patterns
- Long-running background tasks

---

## agent

Start a single subagent.

### Parameters

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `subagent` | string | Yes | Subagent name (must match a defined subagent name) |
| `prompt` | string | Yes | Detailed task description for the subagent |
| `description` | string | Yes | Short (3-5 word) description of the task (e.g., "Fix login bug") |
| `background` | boolean | No | Run in background and return immediately with task_id (default: false) |
| `task_id` | string | No | Resume a previous subagent session by providing its task_id |

### Foreground Execution (Default)

Runs the subagent and blocks until completion.

```json
{
  "name": "agent",
  "arguments": {
    "subagent": "code_reviewer",
    "description": "Review auth function",
    "prompt": "Review this function for security issues:\n\nfunction authenticate(user, pass) { ... }"
  }
}
```

### Foreground Result

```
task_id: task-abc123 (for resuming)

<task_result>
Result from the subagent...
</task_result>
```

### Background Execution

For long-running tasks, set `background: true` to get a task_id immediately:

```json
{
  "name": "agent",
  "arguments": {
    "subagent": "code_reviewer",
    "description": "Review all files",
    "prompt": "Review all JavaScript files in src/...",
    "background": true
  }
}
```

### Background Result

```json
{
  "task_id": "task-abc123",
  "status": "running",
  "message": "Background task started. You will be notified when it completes."
}
```

Use the `task_id` with the `join` tool to collect results later.

### Resuming Subagent Sessions

Provide a `task_id` to continue a previous subagent session with its conversation history:

```json
{
  "name": "agent",
  "arguments": {
    "subagent": "code_reviewer",
    "description": "Continue review",
    "prompt": "Now check the utils.js file too",
    "task_id": "task-abc123"
  }
}
```

---

## parallel_agent

Start multiple subagents at once with concurrency limiting.

### Parameters

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `tasks` | array | Yes | Array of task specifications (max 25 tasks) |
| `max_concurrent` | number | No | Maximum concurrent subagents (default: 5, max: 25) |
| `timeout_secs` | number | No | Timeout per task in seconds (default: 300 = 5 min) |

### Task Specification

Each item in `tasks` must include:

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `subagent` | string | Yes | Which subagent to use |
| `prompt` | string | Yes | Detailed task instructions |
| `description` | string | Yes | Short (3-5 word) description |

### Example

Review 5 files in parallel:

```json
{
  "name": "parallel_agent",
  "arguments": {
    "max_concurrent": 5,
    "tasks": [
      {
        "subagent": "code_reviewer",
        "description": "Review auth.js",
        "prompt": "Review src/auth.js for security issues"
      },
      {
        "subagent": "code_reviewer",
        "description": "Review db.js",
        "prompt": "Review src/db.js for SQL injection risks"
      },
      {
        "subagent": "code_reviewer",
        "description": "Review api.js",
        "prompt": "Review src/api.js for input validation"
      }
    ]
  }
}
```

### Result

```json
{
  "results": [
    {
      "description": "Review auth.js",
      "subagent": "code_reviewer",
      "status": "completed",
      "task_id": "task-xxx",
      "output": "No issues found"
    },
    {
      "description": "Review db.js",
      "subagent": "code_reviewer",
      "status": "failed",
      "error": "File not found"
    }
  ],
  "all_succeeded": false,
  "total": 3,
  "succeeded": 2,
  "failed": 1,
  "elapsed_secs": 4.52
}
```

### Status Values

- `"completed"` — Task succeeded with output
- `"failed"` — Task failed with error message
- `"timeout"` — Task exceeded timeout_secs

---

## join

Wait for background subagent tasks to finish and get results.

### Parameters

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `task_ids` | array | Yes | Array of task IDs from background agent calls |
| `timeout_secs` | number | No | Maximum wait time in seconds (default: 300 = 5 min) |

### Example

```json
{
  "name": "join",
  "arguments": {
    "task_ids": ["task-1", "task-2", "task-3"],
    "timeout_secs": 600
  }
}
```

### Result

```json
{
  "completed": [
    {
      "task_id": "task-1",
      "status": "completed",
      "output": "No issues found in auth.js"
    },
    {
      "task_id": "task-2",
      "status": "completed",
      "output": "Found 2 issues in db.js: ..."
    },
    {
      "task_id": "task-3",
      "status": "failed",
      "error": "Subagent error: file not found"
    }
  ],
  "all_succeeded": false,
  "total": 3,
  "succeeded": 2,
  "failed": 1,
  "not_found": 0
}
```

### Status Values

- `"completed"` — Task succeeded with output
- `"failed"` — Task failed with error message
- `"timeout"` — Join timeout reached before task completed
- `"not_found"` — Task ID does not exist (already consumed or never created)

---

## Defining Subagents

Add subagents to your agent definition using the `subagents` field:

```json
{
  "id": "senior-engineer",
  "name": "Senior Engineer",
  "system_prompt": "You are a senior engineer...",
  "subagents": [
    {
      "id": "code-reviewer-v2",
      "name": "code_reviewer",
      "description": "Security-focused code reviewer with expertise in vulnerability detection",
      "system_prompt": "You are a code reviewer. Focus on security issues...",
      "provider": {
        "provider_type": "open_ai",
        "model": "gpt-4o",
        "api_key": "${OPENAI_API_KEY}"
      },
      "config": {
        "max_turns": 10
      }
    },
    {
      "id": "test-writer-v1",
      "name": "test_writer",
      "description": "Test generation specialist",
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

### Subagent Fields

| Field | Required | Description |
|-------|----------|-------------|
| `id` | Yes | Unique identifier for this subagent |
| `name` | Yes | Name used when invoking via `subagent` parameter |
| `description` | Yes | Description shown in available subagents list |
| `system_prompt` | Yes | System prompt for the subagent |
| `provider` | Yes | LLM provider configuration |
| `config` | No | Agent config (max_tokens, max_turns, temperature, etc.) |
| `tools` | No | Custom tools available to this subagent |
| `skills` | No | Skills available to this subagent |
| `subagents` | No | Nested subagents (respects depth limits) |

---

## Complete Example

User: "Review all JavaScript files in src/"

```
Agent:
1. glob to find all JS files
2. parallel_agent to review each file
3. join to collect results
4. Summarize findings to user
```

Step by step:

```json
// 1. Find files
{
  "name": "glob",
  "arguments": { "pattern": "src/**/*.js" }
}

// 2. Review in parallel
{
  "name": "parallel_agent",
  "arguments": {
    "max_concurrent": 5,
    "tasks": [
      {"subagent": "code_reviewer", "description": "Review auth.js", "prompt": "Review /project/src/auth.js"},
      {"subagent": "code_reviewer", "description": "Review db.js", "prompt": "Review /project/src/db.js"},
      {"subagent": "code_reviewer", "description": "Review api.js", "prompt": "Review /project/src/api.js"}
    ]
  }
}

// 3. Agent summarizes results for user
```

---

## Limits and Timeouts

### Concurrency Limits

| Limit | Value |
|-------|-------|
| parallel_agent max tasks | 25 per call |
| parallel_agent default concurrency | 5 |
| parallel_agent max concurrency | 25 |

### Timeout Defaults

| Operation | Default | Maximum |
|-----------|---------|---------|
| Foreground agent | 120 seconds | 120 seconds |
| Background agent | 300 seconds (5 min) | 300 seconds |
| parallel_agent per task | 300 seconds | Configurable via `timeout_secs` |
| join wait | 300 seconds (5 min) | Configurable via `timeout_secs` |

### Subagent Depth Limit

Maximum subagent nesting depth is **3 levels**:
- Level 0: Parent agent
- Level 1: Subagent
- Level 2: Subagent's subagent
- Level 3: Maximum depth

Attempting to spawn subagents beyond depth 3 returns an error: "Maximum subagent depth (3) reached"

### What Happens When Limits Are Exceeded

**parallel_agent tasks > 25:**
```
Error: "Maximum 25 tasks allowed"
```

**Depth limit exceeded:**
```
Error: "Maximum subagent depth (3) reached"
```

**Unknown subagent:**
```
Error: "Unknown subagent 'xyz'. Available: [abc, def]"
```

**Timeout reached (foreground agent):**
```
Error: "Subagent timed out after 120s"
```

**Timeout reached (parallel_agent task):**
```json
{
  "status": "timeout",
  "error": "Timeout after 300s"
}
```

**Timeout reached (join):**
Pending tasks return with `"status": "timeout"`

---

## Error Handling

### Failed Subagents in parallel_agent

Failed tasks don't stop other tasks. Results include success/failure for each:

```json
{
  "results": [
    {"status": "completed", "output": "..."},
    {"status": "failed", "error": "File not found"},
    {"status": "completed", "output": "..."}
  ],
  "all_succeeded": false,
  "succeeded": 2,
  "failed": 1
}
```

### Handling Partial Failures

Always check `all_succeeded` or individual statuses:

```javascript
// Example response parsing
const result = JSON.parse(join_result);
if (!result.all_succeeded) {
  for (const task of result.completed) {
    if (task.status === 'failed') {
      console.log(`Task ${task.task_id} failed: ${task.error}`);
    } else if (task.status === 'timeout') {
      console.log(`Task ${task.task_id} timed out`);
    } else if (task.status === 'not_found') {
      console.log(`Task ${task.task_id} not found`);
    }
  }
}
```

---

## Best Practices

### Keep Subagent Tasks Focused

**Good:** "Review src/auth.js for security issues"

**Too broad:** "Review the entire codebase"

### Match Subagent to Task

Use specialized subagents:

```json
{
  "subagents": [
    {
      "id": "security-expert",
      "name": "security_reviewer",
      "description": "Security vulnerability detection specialist"
    },
    {
      "id": "perf-expert",
      "name": "performance_reviewer",
      "description": "Performance optimization specialist"
    },
    {
      "id": "style-expert",
      "name": "style_reviewer",
      "description": "Code style and best practices specialist"
    }
  ]
}
```

### Use Background Mode for Long Tasks

Set `background: true` for tasks that may take longer than a few seconds:

```json
{
  "name": "agent",
  "arguments": {
    "subagent": "explorer",
    "description": "Analyze large codebase",
    "prompt": "Find all API endpoints in this 100k line codebase...",
    "background": true
  }
}
```

Then use `join` to collect results when needed.

### Choose the Right Tool

| Use Case | Tool | Why |
|----------|------|-----|
| Single quick task | `agent` | Simple, blocks until done |
| Multiple independent tasks | `parallel_agent` | Built-in concurrency control |
| Long-running task | `agent` with `background: true` | Returns immediately, join later |
| Simple file reads | `batch` | Lower overhead than subagents |

---

## See Also

- [batch](batch-tool.md) — Parallel tool execution (lower overhead than subagents)
- [Agents](../core-concepts/agents.md) — Agent configuration
