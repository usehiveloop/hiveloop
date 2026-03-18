# Batch Tool

Execute multiple tools in one call.

---

## Overview

The batch tool lets agents run several tools at once. Instead of calling tools one by one:

```
1. read file A
2. read file B  
3. read file C
```

Agents can batch them:

```
Batch: read A, read B, read C (all at once)
```

This is faster and more efficient.

---

## Parameters

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `tool_calls` | array | Yes | List of tool calls to execute (max 25) |

### Tool Call Structure

Each call in the array:

```json
{
  "tool": "tool_name",
  "parameters": { ... }
}
```

---

## Example

Read multiple files at once:

```json
{
  "name": "batch",
  "arguments": {
    "tool_calls": [
      {
        "tool": "read",
        "parameters": { "path": "/project/README.md" }
      },
      {
        "tool": "read",
        "parameters": { "path": "/project/package.json" }
      },
      {
        "tool": "read",
        "parameters": { "path": "/project/src/main.js" }
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
      "tool": "read",
      "success": true,
      "result": "# Project..."
    },
    {
      "tool": "read",
      "success": true,
      "result": "{\"name\": \"project\"}"
    },
    {
      "tool": "read",
      "success": true,
      "result": "console.log('main');"
    }
  ],
  "total": 3,
  "succeeded": 3,
  "failed": 0,
  "message": "All 3 tools executed successfully.\n\nKeep using the batch tool for optimal performance!"
}
```

---

## Execution

### Parallel Execution

Commands execute **in parallel** using `futures::future::join_all`. There's no guaranteed order.

### Error Handling

**Individual command failures don't stop other commands.** Each tool runs independently and reports its own success/failure status:

```json
{
  "results": [
    { "tool": "read", "success": true, "result": "..." },
    { "tool": "read", "success": false, "error": "File not found" },
    { "tool": "read", "success": true, "result": "..." }
  ],
  "total": 3,
  "succeeded": 2,
  "failed": 1
}
```

### Result Truncation

Batch results are subject to truncation if they exceed:
- **2000 lines** (MAX_LINES)
- **50KB** (MAX_BYTES)

When truncated, the full output is persisted to a temporary file and the response includes:
```
... [N lines, M bytes truncated. Full output saved to: /path/to/file.txt] ...
```

Use `read` with offset/limit or `grep` to view specific sections of large outputs.

---

## Limitations

### Maximum 25 Commands Per Batch

- Hard limit: `MAX_BATCH_SIZE = 25`
- **Behavior when exceeded**: Commands beyond 25 are **discarded** and marked as failed with error: `"Maximum of 25 tools allowed in batch"`
- The first 25 commands still execute normally

### No Recursive Batching

Recursive batch calls are explicitly disallowed and return error: `"Recursive batch calls are not allowed"`

### No External Tools

External tools (MCP, environment tools) **cannot be batched**. Only built-in tools registered in the tool registry are supported. If you try to batch an external tool, you'll get an error like:
```
Tool 'xxx' not in registry. External tools (MCP, environment) cannot be batched — call them directly.
Available tools: read, write, bash, grep, ...
```

### No Output Chaining

Commands don't depend on each other. You cannot use the output of one command as input to another within the same batch.

### Empty Batches Not Allowed

An empty `tool_calls` array returns error: `"No tool calls provided"`

---

## Use Cases

### Read Multiple Files

When analyzing a codebase:

```json
{
  "tool_calls": [
    { "tool": "read", "parameters": { "path": "package.json" } },
    { "tool": "read", "parameters": { "path": "tsconfig.json" } },
    { "tool": "read", "parameters": { "path": "README.md" } }
  ]
}
```

### Search Multiple Patterns

```json
{
  "tool_calls": [
    { "tool": "grep", "parameters": { "pattern": "TODO", "path": "." } },
    { "tool": "grep", "parameters": { "pattern": "FIXME", "path": "." } },
    { "tool": "grep", "parameters": { "pattern": "XXX", "path": "." } }
  ]
}
```

### Gather Information

```json
{
  "tool_calls": [
    { "tool": "bash", "parameters": { "command": "git log --oneline -5" } },
    { "tool": "bash", "parameters": { "command": "git status" } },
    { "tool": "ls", "parameters": { "path": "." } }
  ]
}
```

### Multi-part Edits

Edit multiple files or multiple locations in the same file:

```json
{
  "tool_calls": [
    { "tool": "edit", "parameters": { "path": "src/main.rs", "old": "...", "new": "..." } },
    { "tool": "edit", "parameters": { "path": "src/lib.rs", "old": "...", "new": "..." } }
  ]
}
```

---

## When to Use

| Scenario | Use Batch? |
|----------|------------|
| Read 3+ config files | Yes |
| Read file, process, write | No (sequential dependency) |
| Search for TODOs and FIXMEs | Yes |
| Build project, run tests | No (sequential) |
| Multiple independent bash commands | Yes |
| Multi-part edits on same/different files | Yes |

Batching tool calls yields 2-5x efficiency gain and provides better UX.

---

## See Also

- [spawn_agent](agent-tools.md) — Parallel agent execution
