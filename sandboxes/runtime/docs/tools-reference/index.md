# Tools Reference

Complete reference for all tools available to agents.

---

## Tool Types

Bridge provides three types of tools:

### Built-in Tools
Tools included with Bridge:
- Filesystem: read, write, edit, ls, glob
- Search: grep, web_search
- Execution: bash, web_fetch
- Agents: spawn_agent, parallel_agent, join
- Tasks: todo, batch

### MCP Tools
External tools from MCP servers:
- Database queries
- Custom business logic
- Third-party integrations

### Integration Tools
HTTP endpoints defined in your agent:
- Connect to your existing APIs
- Custom actions

---

## Tool Configuration

Enable tools per-agent:

```json
{
  "id": "my-agent",
  "tools": ["read", "write", "bash", "grep"],
  "mcp_servers": [...],
  "integrations": [...]
}
```

---

## Tool Permissions

Control tool behavior:

```json
{
  "permissions": {
    "read": "allow",
    "write": "require_approval",
    "bash": "deny"
  }
}
```

| Permission | Behavior |
|------------|----------|
| `allow` | Runs automatically |
| `require_approval` | Waits for user confirmation |
| `deny` | Cannot be used |

---

## Tool Calling Format

When an agent uses a tool, it sends:

```json
{
  "name": "read",
  "arguments": {
    "path": "/path/to/file"
  }
}
```

Bridge executes the tool and returns:

```json
{
  "success": true,
  "result": "file contents...",
  "error": null
}
```

---

## Tool List

### Filesystem
- [read](filesystem-tools.md) — Read file contents
- [write](filesystem-tools.md) — Create or overwrite files
- [edit](filesystem-tools.md) — Make targeted changes
- [ls](filesystem-tools.md) — List directories
- [glob](filesystem-tools.md) — Find files by pattern

### Search
- [grep](search-tools.md) — Search file contents
- [web_search](web-tools.md) — Search the web

### Execution
- [bash](bash-tool.md) — Run shell commands
- [web_fetch](web-tools.md) — Fetch web pages

### Agent Management
- [spawn_agent](agent-tools.md) — Start a subagent
- [parallel_agent](agent-tools.md) — Run agents in parallel
- [join](agent-tools.md) — Wait for agents to finish

### Tasks
- [todo](todo-tool.md) — Manage todo lists
- [batch](batch-tool.md) — Execute multiple tools

### Code Intelligence
- [lsp_query](lsp-tools.md) — Query language servers

### Skills
- [skill](skill-tool.md) — Load a skill

### Custom
- [Custom tools](custom-tools.md) — Build your own

---

## See Also

- [Tools Concept](../core-concepts/tools.md) — How tools work
- [MCP](../core-concepts/mcp.md) — External tool servers
