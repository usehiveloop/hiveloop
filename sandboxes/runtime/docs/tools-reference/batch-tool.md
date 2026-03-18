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
| `commands` | array | Yes | List of tool calls to execute |

### Command Structure

Each command in the array:

```json
{
  "tool": "tool_name",
  "arguments": { ... }
}
```

---

## Example

Read multiple files at once:

```json
{
  "name": "batch",
  "arguments": {
    "commands": [
      {
        "tool": "read",
        "arguments": { "path": "/project/README.md" }
      },
      {
        "tool": "read",
        "arguments": { "path": "/project/package.json" }
      },
      {
        "tool": "read",
        "arguments": { "path": "/project/src/main.js" }
      }
    ]
  }
}
```

### Result

```json
{
  "success": true,
  "result": [
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
  ]
}
```

---

## Execution

Commands execute in parallel where possible. There's no guaranteed order.

If one command fails, others still run:

```json
{
  "result": [
    { "tool": "read", "success": true, "result": "..." },
    { "tool": "read", "success": false, "error": "File not found" },
    { "tool": "read", "success": true, "result": "..." }
  ]
}
```

---

## Use Cases

### Read Multiple Files

When analyzing a codebase:

```json
{
  "commands": [
    { "tool": "read", "arguments": { "path": "package.json" } },
    { "tool": "read", "arguments": { "path": "tsconfig.json" } },
    { "tool": "read", "arguments": { "path": "README.md" } }
  ]
}
```

### Search Multiple Patterns

```json
{
  "commands": [
    { "tool": "grep", "arguments": { "pattern": "TODO", "path": "." } },
    { "tool": "grep", "arguments": { "pattern": "FIXME", "path": "." } },
    { "tool": "grep", "arguments": { "pattern": "XXX", "path": "." } }
  ]
}
```

### Gather Information

```json
{
  "commands": [
    { "tool": "bash", "arguments": { "command": "git log --oneline -5" } },
    { "tool": "bash", "arguments": { "command": "git status" } },
    { "tool": "ls", "arguments": { "path": "." } }
  ]
}
```

---

## Limitations

- Maximum 25 commands per batch
- Commands don't depend on each other (no output chaining)
- All execute in same working directory

---

## When to Use

| Scenario | Use Batch? |
|----------|------------|
| Read 3 config files | Yes |
| Read file, process, write | No (sequential dependency) |
| Search for TODOs and FIXMEs | Yes |
| Build project, run tests | No (sequential) |

---

## See Also

- [spawn_agent](agent-tools.md) — Parallel agent execution
