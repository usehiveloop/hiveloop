# Bash Tool

Run shell commands using `/bin/sh`.

---

## Parameters

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `command` | string | Yes | Shell command to execute |
| `workdir` | string | No | Working directory (default: current directory) |
| `timeout` | number | No | Timeout in milliseconds (default: 120000, max: 600000) |
| `description` | string | No | Short description of the command (5-10 words) |
| `background` | boolean | No | Run in background, returns immediately with `task_id` |

## Example

```json
{
  "name": "bash",
  "arguments": {
    "command": "git status",
    "description": "Check git status"
  }
}
```

## Result

```json
{
  "output": "On branch main\nYour branch is up to date...",
  "exit_code": 0,
  "timed_out": false
}
```

---

## Common Use Cases

### Git Operations

```json
{
  "name": "bash",
  "arguments": {
    "command": "git log --oneline -5",
    "description": "Show recent commits"
  }
}
```

### Check Versions

```json
{
  "name": "bash",
  "arguments": {
    "command": "node --version",
    "description": "Check Node.js version"
  }
}
```

### Run Tests

```json
{
  "name": "bash",
  "arguments": {
    "command": "npm test",
    "timeout": 300000,
    "description": "Run test suite"
  }
}
```

### Find Files

```json
{
  "name": "bash",
  "arguments": {
    "command": "find . -name '*.py' | head -20",
    "description": "Find Python files"
  }
}
```

---

## Shell

Commands run with `/bin/sh -c`. This provides POSIX-compliant shell behavior. Bash-specific features may not be available.

---

## RTK Filter Pipeline

By default, bash output is routed through the **rtk filter pipeline** for token-efficient compression of common commands' output (e.g. `npm install`, `cargo build`, `pytest`, framework test runners). An in-process allowlist router decides which commands get filtered; everything else passes through verbatim.

Test-runner output is **never silently rewritten**. The runner's own summary line (`Tests: N passed`, `OK (N tests)`, etc.) is preserved verbatim — synthetic collapses like "artisan test: ok" caused multi-turn debugging dead-ends and have been removed.

To disable the pipeline entirely (diagnostic only — significantly increases output tokens):

```bash
export BRIDGE_DISABLE_RTK="true"
```

---

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1-255 | Command-specific error |
| `null` | Command was killed (timeout or signal) |

Non-zero exit codes don't fail the tool — they're reported in the result:

```json
{
  "output": "error: pathspec 'feature' did not match...",
  "exit_code": 1,
  "timed_out": false
}
```

The AI sees the error and can decide what to do.

---

## Timeout

Commands timeout after **120 seconds** (2 minutes) by default. You can specify a custom timeout up to **600,000 ms** (10 minutes).

```json
{
  "name": "bash",
  "arguments": {
    "command": "sleep 5",
    "timeout": 10000,
    "description": "Sleep with 10s timeout"
  }
}
```

On timeout, the entire process group is killed to prevent orphaned processes.

---

## Output Limits

Output is limited to **50,000 bytes**. Larger outputs are truncated with head (1,000 bytes) and tail (1,000 bytes) shown, and the full output spilled to a temp file:

```
[first 1000 bytes]

... [Output truncated. Full output (123456 bytes) saved to: /tmp/bridge_bash_abc123.txt] ...

[last 1000 bytes]
```

---

## Stdin

Commands run with stdin set to `/dev/null`. Commands that try to read from stdin will receive EOF immediately.

---

## Background Execution

Run long commands in the background:

```json
{
  "name": "bash",
  "arguments": {
    "command": "npm run build",
    "background": true,
    "description": "Build the project"
  }
}
```

Returns immediately:
```json
{
  "task_id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "running",
  "message": "Background command started. You will be notified when it completes."
}
```

The agent is notified when the command completes.

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

Commands run in the specified `workdir` (or current directory). They cannot escape via `..` or symlinks if the OS prevents it.

---

## Chaining Commands

Use shell operators to chain commands:

```json
{
  "name": "bash",
  "arguments": {
    "command": "cd src && npm run build",
    "description": "Build from src directory"
  }
}
```

```json
{
  "name": "bash",
  "arguments": {
    "command": "cat file.txt | grep pattern | wc -l",
    "description": "Count pattern matches"
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
    "command": "echo $PATH",
    "description": "Show PATH"
  }
}
```

---

## Error Handling

The bash tool succeeds if the command runs, even if the command itself fails:

```json
// This "succeeds" (tool runs) even though git fails
{
  "output": "fatal: not a git repository",
  "exit_code": 128,
  "timed_out": false
}
```

The AI should check `exit_code` and `output` to detect failures.

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
