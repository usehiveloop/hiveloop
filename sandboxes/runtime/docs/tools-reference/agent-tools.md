# Agent Tools

Spawn and manage subagents for parallel work.

---

## Overview

These tools let agents delegate work to other agents:

- **spawn_agent** — Start a subagent to handle a task
- **parallel_agent** — Start multiple subagents at once
- **join** — Wait for subagents to finish and get results

Useful for:
- Parallel processing (analyze 10 files at once)
- Specialization (different agents for different tasks)
- Fan-out/fan-in patterns

---

## spawn_agent

Start a single subagent.

### Parameters

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `agent` | string | Yes | Subagent name (from agent definition) |
| `prompt` | string | Yes | Task description for subagent |

### Example

```json
{
  "name": "spawn_agent",
  "arguments": {
    "agent": "code_reviewer",
    "prompt": "Review this function for security issues:\n\nfunction authenticate(user, pass) { ... }"
  }
}
```

### Result

```json
{
  "success": true,
  "result": {
    "task_id": "task-abc123",
    "status": "running"
  }
}
```

Save the `task_id` for joining later.

---

## parallel_agent

Start multiple subagents at once.

### Parameters

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `agent` | string | Yes | Subagent name |
| `prompts` | array | Yes | Array of task descriptions |

### Example

Review 5 files in parallel:

```json
{
  "name": "parallel_agent",
  "arguments": {
    "agent": "code_reviewer",
    "prompts": [
      "Review src/auth.js",
      "Review src/db.js",
      "Review src/api.js",
      "Review src/utils.js",
      "Review src/config.js"
    ]
  }
}
```

### Result

```json
{
  "success": true,
  "result": {
    "task_ids": ["task-1", "task-2", "task-3", "task-4", "task-5"]
  }
}
```

### Concurrency Limits

- Default: 5 parallel tasks
- Maximum: 25 tasks

---

## join

Wait for subagents to finish and get results.

### Parameters

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `task_ids` | array | Yes | Task IDs from spawn_agent or parallel_agent |
| `timeout_ms` | number | No | Maximum wait time (default: 300000 = 5 min) |

### Example

```json
{
  "name": "join",
  "arguments": {
    "task_ids": ["task-1", "task-2", "task-3"],
    "timeout_ms": 120000
  }
}
```

### Result

```json
{
  "success": true,
  "result": [
    {
      "task_id": "task-1",
      "status": "completed",
      "result": "No issues found in auth.js"
    },
    {
      "task_id": "task-2",
      "status": "completed",
      "result": "Found 2 issues in db.js: ..."
    },
    {
      "task_id": "task-3",
      "status": "failed",
      "error": "File not found"
    }
  ]
}
```

---

## Defining Subagents

Add subagents to your agent definition:

```json
{
  "id": "senior-engineer",
  "name": "Senior Engineer",
  "system_prompt": "You are a senior engineer...",
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

The `agent_id` refers to another agent pushed to Bridge.

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

// 2. Review in parallel (result: task-1, task-2, task-3...)
{
  "name": "parallel_agent",
  "arguments": {
    "agent": "code_reviewer",
    "prompts": [
      "Review /project/src/auth.js",
      "Review /project/src/db.js",
      "Review /project/src/api.js"
    ]
  }
}

// 3. Wait for completion
{
  "name": "join",
  "arguments": {
    "task_ids": ["task-1", "task-2", "task-3"]
  }
}

// 4. Agent summarizes results for user
```

---

## Error Handling

If a subagent fails:

```json
{
  "task_id": "task-2",
  "status": "failed",
  "error": "Tool execution error: file not found"
}
```

The parent agent sees this and can:
- Retry the task
- Skip that result
- Report the error to the user

---

## Timeouts

Join has a default 5-minute timeout. Adjust if needed:

```json
{
  "name": "join",
  "arguments": {
    "task_ids": ["task-1"],
    "timeout_ms": 600000  // 10 minutes
  }
}
```

If timeout is reached, you get partial results for completed tasks.

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
    { "name": "security_reviewer", "agent_id": "security-expert" },
    { "name": "performance_reviewer", "agent_id": "perf-expert" },
    { "name": "style_reviewer", "agent_id": "style-expert" }
  ]
}
```

### Handle Partial Failures

Always check which subagents succeeded:

```javascript
const results = await join(task_ids);
for (const r of results) {
  if (r.status === 'failed') {
    console.log(`Task ${r.task_id} failed: ${r.error}`);
  }
}
```

---

## See Also

- [batch](batch-tool.md) — Parallel tool execution
- [Agents](../core-concepts/agents.md) — Agent configuration
