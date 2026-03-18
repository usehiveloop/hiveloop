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

## Actions

The todo tool has several actions:

### add

Add a new todo item.

```json
{
  "name": "todo",
  "arguments": {
    "action": "add",
    "content": "Read the README file"
  }
}
```

### complete

Mark a todo as done.

```json
{
  "name": "todo",
  "arguments": {
    "action": "complete",
    "id": "todo-123"
  }
}
```

### update

Change a todo's content.

```json
{
  "name": "todo",
  "arguments": {
    "action": "update",
    "id": "todo-123",
    "content": "Read README and CONTRIBUTING"
  }
}
```

### remove

Delete a todo.

```json
{
  "name": "todo",
  "arguments": {
    "action": "remove",
    "id": "todo-123"
  }
}
```

### list

Show all todos.

```json
{
  "name": "todo",
  "arguments": {
    "action": "list"
  }
}
```

---

## Todo Structure

Each todo has:

| Field | Description |
|-------|-------------|
| `id` | Unique identifier |
| `content` | Todo text |
| `status` | `pending`, `in_progress`, or `done` |
| `created_at` | When created |

---

## Example Workflow

User: "Review this codebase for issues"

```
Agent creates todos:
  ☐ Read project structure
  ☐ Check for TODO/FIXME comments
  ☐ Review main entry points
  ☐ Look for security issues

Agent works through them:
  ☑ Read project structure
  ☐ Check for TODO/FIXME comments
  ...

Agent reports findings.
```

---

## Events

Todo changes emit events to the SSE stream:

```
event: todo_created
data: {"todo_id": "todo-1", "content": "Read README"}

event: todo_updated
data: {"todo_id": "todo-1", "content": "Read README", "status": "done"}
```

Your frontend can show these in real-time.

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

### Update Status as You Go

Mark todos in_progress when starting, done when finished.

---

## See Also

- [batch](batch-tool.md) — Execute multiple operations
