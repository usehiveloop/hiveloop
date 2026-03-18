# Tools

Tools are things an agent can do. Read a file, search the web, run a command — each is a tool.

---

## How Tools Work

```
1. User asks a question
   "Find all TODOs in the codebase"
   ↓
2. Agent decides to use a tool
   Calls grep with pattern "TODO"
   ↓
3. Tool executes
   Runs grep, returns results
   ↓
4. Agent sees results
   "I found 5 TODOs: ..."
   ↓
5. Agent responds to user
   Lists the TODOs found
```

The AI decides when to use tools based on the user's request and the tool descriptions.

---

## Tool Definition

Each tool has:

| Field | Purpose |
|-------|---------|
| `name` | How the agent refers to it |
| `description` | Tells the AI what it does |
| `parameters` | JSON schema for arguments |

Example:

```json
{
  "name": "read",
  "description": "Read the contents of a file",
  "parameters": {
    "type": "object",
    "properties": {
      "path": {
        "type": "string",
        "description": "Path to the file"
      }
    },
    "required": ["path"]
  }
}
```

---

## Built-in Tools

Bridge includes these tools out of the box:

### Filesystem
| Tool | What it does |
|------|--------------|
| `read` | Read a file's contents |
| `write` | Create or overwrite a file |
| `edit` | Make targeted edits to a file |
| `ls` | List directory contents |
| `glob` | Find files matching a pattern |

### Search
| Tool | What it does |
|------|--------------|
| `grep` | Search file contents with regex |
| `web_search` | Search the web |

### Execution
| Tool | What it does |
|------|--------------|
| `bash` | Run shell commands |
| `web_fetch` | Fetch a webpage |

### Agent Management
| Tool | What it does |
|------|--------------|
| `spawn_agent` | Start a subagent task |
| `parallel_agent` | Run multiple subagents at once |
| `join` | Wait for subagents to finish |

### Task Tracking
| Tool | What it does |
|------|--------------|
| `todo` | Manage a todo list |
| `batch` | Execute multiple tools at once |

### Code Intelligence
| Tool | What it does |
|------|--------------|
| `lsp_query` | Query language servers |

See [Tools Reference](../tools-reference/index.md) for full details on each tool.

---

## Configuring Tools for an Agent

Add tools to an agent definition:

```json
{
  "id": "code-helper",
  "tools": ["read", "edit", "bash", "grep"]
}
```

The agent can only use tools you explicitly list.

---

## Permission Levels

Control how tools behave with permissions:

```json
{
  "id": "code-helper",
  "tools": ["read", "edit", "bash"],
  "permissions": {
    "read": "allow",
    "edit": "require_approval",
    "bash": "deny"
  }
}
```

| Level | Behavior |
|-------|----------|
| `allow` | Tool runs automatically |
| `require_approval` | Pauses for user confirmation |
| `deny` | Tool cannot be used (overrides tools list) |

### Integration Actions

For integration tools, define permissions per action:

```json
{
  "integrations": [{
    "name": "github",
    "actions": [{
      "name": "create_pull_request",
      "permission": "require_approval"
    }]
  }]
}
```

---

## Tool Approvals

When a tool requires approval:

1. Agent calls the tool
2. Bridge pauses and emits `tool_approval_required` event
3. Your frontend shows the request to the user
4. User approves or denies
5. Tool runs (or doesn't) and continues

See [Handling Approvals](../api-reference/agents-api.md#tool-approvals).

---

## Tool Results

Tools return results in standard format:

```json
{
  "success": true,
  "result": "...",
  "error": null
}
```

Or on failure:

```json
{
  "success": false,
  "result": null,
  "error": "File not found: /path/to/file"
}
```

The agent sees these results and decides what to tell the user.

---

## MCP Tools

In addition to built-in tools, you can connect external tool servers via MCP:

```json
{
  "id": "my-agent",
  "mcp_servers": [{
    "name": "filesystem",
    "transport": {
      "type": "stdio",
      "command": "npx",
      "args": ["@modelcontextprotocol/server-filesystem", "/home/user/project"]
    }
  }]
}
```

MCP servers provide their own tools, which Bridge exposes to the agent just like built-in ones.

See [MCP](mcp.md) for more.

---

## Custom Tools

You can create your own tools by:

1. **Integration tools** — HTTP endpoints Bridge calls
2. **Custom MCP servers** — Build a server with your logic

See [Custom Tools](../tools-reference/custom-tools.md).

---

## Tool Selection Tips

### Start Minimal

Don't give an agent every tool. Start with what it actually needs:

```json
// Good: Focused
{
  "id": "doc-writer",
  "tools": ["read", "write", "web_search"]
}

// Bad: Kitchen sink
{
  "id": "doc-writer",
  "tools": ["read", "write", "edit", "ls", "glob", "grep", "bash", "web_search", "web_fetch", "spawn_agent", ...]
}
```

### Use Descriptions

Tool descriptions matter. The AI uses them to decide which tool to use:

```json
{
  "name": "search_docs",
  "description": "Search the internal documentation. Use this when the user asks about product features, pricing, or how-to questions.",
  ...
}
```

### Set Boundaries

Use permissions to prevent accidents:

```json
{
  "id": "code-reader",
  "tools": ["read", "grep", "bash"],
  "permissions": {
    "bash": "require_approval"  // Don't let it run random commands
  }
}
```

---

## See Also

- [Tools Reference](../tools-reference/index.md) — Complete tool documentation
- [MCP](mcp.md) — External tool servers
- [Custom Tools](../tools-reference/custom-tools.md) — Build your own
