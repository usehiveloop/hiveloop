# Todo Tool

Manage a todo list within a conversation.

---

## Overview

The todo tool lets agents track tasks and show progress to users. It's useful for:

- Multi-step operations
- Long-running tasks
- Showing work progress
- Organizing complex requests

---

## Tools

The todo system consists of two tools:

### `todowrite`

Creates or replaces the entire todo list. Each call sends the complete list.

```json
{
  "name": "todowrite",
  "arguments": {
    "todos": [
      { "content": "Read the README file", "status": "pending", "priority": "high" },
      { "content": "Check for TODO comments", "status": "in_progress", "priority": "medium" }
    ]
  }
}
```

**Replace-All Semantics:**
- Each call replaces the ENTIRE todo list
- Always include all items (completed, in-progress, pending) in every call
- To update a single item's status, send the full list with that item changed

### `todoread`

Reads the current todo list. Takes no parameters.

```json
{
  "name": "todoread",
  "arguments": {}
}
```

Returns:
```json
{
  "todos": [
    { "content": "Read the README file", "status": "pending", "priority": "high" }
  ],
  "total": 1,
  "incomplete_count": 1
}
```

---

## Todo Structure

Each todo has:

| Field | Type | Description |
|-------|------|-------------|
| `content` | string | Brief description of the task |
| `status` | string | One of: `pending`, `in_progress`, `completed`, `cancelled` |
| `priority` | string | One of: `high`, `medium`, `low` |

**Status Values:**
- `pending` — Task not yet started
- `in_progress` — Currently working on (limit to ONE task at a time)
- `completed` — Task finished successfully
- `cancelled` — Task no longer needed

**Priority Levels:**
- `high` — Critical or blocking tasks
- `medium` — Standard tasks
- `low` — Nice-to-have or deferred tasks

---

## Limits and Persistence

| Aspect | Behavior |
|--------|----------|
| **Maximum todos** | No hard limit enforced |
| **Persistence** | In-memory only — todos are lost when the conversation ends |
| **IDs** | Todos do not have individual IDs; the list is replaced as a whole |

---

## Example Workflow

User: "Review this codebase for issues"

```
Agent creates todos:
  ☐ Read project structure [high]
  ☐ Check for TODO/FIXME comments [medium]
  ☐ Review main entry points [high]
  ☐ Look for security issues [high]

Agent works through them:
  ☑ Read project structure [high]
  ☐ Check for TODO/FIXME comments [medium] (in_progress)
  ...

Agent reports findings.
```

---

## Events

Todo changes emit a `todo_updated` event to the SSE stream:

```
event: todo_updated
data: {"todos": [{"content": "Read README", "status": "in_progress", "priority": "high"}]}
```

The event contains the **complete current todo list**, not individual changes.

**Webhook Event:** `todo_updated` — See [Webhooks](../core-concepts/webhooks.md)

---

## When to Use

### Use This Tool When:

1. Complex multistep tasks — When a task requires 3 or more distinct steps
2. Non-trivial and complex tasks — Tasks requiring careful planning
3. User explicitly requests todo list
4. User provides multiple tasks (numbered or comma-separated)
5. After receiving new instructions — Capture requirements as todos
6. After completing a task — Mark complete and add follow-up tasks

### Don't Use When:

1. Single, straightforward task
2. Task is trivial (less than 3 trivial steps)
3. Task is purely conversational

---

## Best Practices

### Create Todos for Multi-Step Tasks

**Good:**
> I'll help you refactor this code. Let me break it down:
> - Read the current implementation
> - Identify issues
> - Write the refactored version
> - Run tests

**Not as good:**
> I'll refactor this code for you. [Does everything without showing progress]

### Keep Todos Concrete

**Good:** "Find all functions over 50 lines"

**Vague:** "Review code"

### Update Status in Real-Time

- Mark tasks `in_progress` when starting (only ONE at a time)
- Mark `completed` IMMEDIATELY after finishing
- Mark `cancelled` for tasks that become irrelevant

### Set Appropriate Priorities

Use `high` priority sparingly for critical/blocking tasks. Most tasks should be `medium`.

---

## See Also

- [batch](batch-tool.md) — Execute multiple operations
- [SSE Events](../api-reference/sse-events.md) — Real-time event streaming
