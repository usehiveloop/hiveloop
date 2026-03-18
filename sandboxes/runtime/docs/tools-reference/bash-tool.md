# Bash Tool

Run shell commands.

---

## Parameters

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `command` | string | Yes | Shell command to execute |
| `working_dir` | string | No | Directory to run in (default: `.`) |

## Example

```json
{
  "name": "bash",
  "arguments": {
    "command": "git status"
  }
}
```

## Result

```json
{
  "success": true,
  "result": "On branch main\nYour branch is up to date...",
  "exit_code": 0
}
```

---

## Common Use Cases

### Git Operations

```json
{
  "name": "bash",
  "arguments": {
    "command": "git log --oneline -5"
  }
}
```

### Check Versions

```json
{
  "name": "bash",
  "arguments": {
    "command": "node --version"
  }
}
```

### Run Tests

```json
{
  "name": "bash",
  "arguments": {
    "command": "npm test"
  }
}
```

### Find Files

```json
{
  "name": "bash",
  "arguments": {
    "command": "find . -name '*.py' | head -20"
  }
}
```

---

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1-255 | Command-specific error |

Non-zero exit codes don't fail the tool — they're reported in the result:

```json
{
  "success": true,
  "result": "error: pathspec 'feature' did not match...",
  "exit_code": 1
}
```

The AI sees the error and can decide what to do.

---

## Timeout

Commands timeout after 60 seconds by default. Longer commands will be killed.

---

## Security

### Require Approval

Given the power of shell access, consider requiring approval:

```json
{
  "id": "my-agent",
  "tools": ["bash", "read"],
  "permissions": {
    "bash": "require_approval"
  }
}
```

### Restrict Commands

There's no built-in command filtering. If you need restrictions:

1. Don't give agents the `bash` tool
2. Use MCP servers for controlled operations
3. Wrap Bridge in a container with limited access

### Working Directory

Commands run in the agent's working directory. They cannot escape it.

---

## Chaining Commands

Use shell operators to chain commands:

```json
{
  "name": "bash",
  "arguments": {
    "command": "cd src && npm run build"
  }
}
```

```json
{
  "name": "bash",
  "arguments": {
    "command": "cat file.txt | grep pattern | wc -l"
  }
}
```

---

## Environment Variables

Commands inherit Bridge's environment:

```json
{
  "name": "bash",
  "arguments": {
    "command": "echo $PATH"
  }
}
```

---

## Error Handling

The bash tool succeeds if the command runs, even if the command itself fails:

```json
// This "succeeds" (tool runs) even though git fails
{
  "success": true,
  "result": "fatal: not a git repository",
  "exit_code": 128
}
```

The AI should check `exit_code` and `result` to detect failures.

---

## Alternatives

For safer alternatives to bash:

| Instead of | Use |
|------------|-----|
| `cat file` | `read` tool |
| `ls` | `ls` tool |
| `find` | `glob` tool |
| `grep` | `grep` tool |

---

## See Also

- [Filesystem Tools](filesystem-tools.md) — Safer file operations
- [grep](search-tools.md) — Built-in search
