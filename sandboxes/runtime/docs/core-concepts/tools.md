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
| `agent` | Self-delegation — spawn a clone of the parent agent |
| `sub_agent` | Start a named subagent (foreground, or `runInBackground` for fire-and-forget) |

Parallel fan-out: the LLM emits multiple `sub_agent` tool_use blocks in one assistant turn; the runtime runs them concurrently. Background results arrive as user-turn injections on the next turn — there is no separate wait/join tool.

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

### Maximum Tools Per Agent

There is no hard limit on the total number of tools an agent can have, but specific tools have batch limits:

| Context | Limit | Behavior When Exceeded |
|---------|-------|------------------------|
| `batch` tool | 25 tools | Excess tools are discarded with error |

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

### Permission Inheritance

Subagents automatically inherit integration tools from their parent agent with the same permission levels. This ensures consistent access control across the agent hierarchy.

**Important notes:**
- Subagents cannot spawn other subagents (prevents unbounded recursion)
- Integration actions with `deny` permission are never exposed to the LLM
- Non-`allow` integration permissions are automatically injected into the agent's permissions map

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

## Tool Name Format

### Case Sensitivity

Tool names are **case-insensitive**. The following all refer to the same tool:
- `read`
- `Read`
- `READ`

### Integration Tool Naming

Integration tools follow a specific naming convention:

```
{integration}__{action}
```

For example: `github__create_pull_request`, `slack__send_message`

The double underscore (`__`) separates the integration name from the action name.

### Tool Name Suggestions

If an agent calls a non-existent tool, Bridge suggests the closest match using Levenshtein distance (similarity > 0.4):

```
Unknown tool 'bassh'. Did you mean 'bash'? Available tools: [bash, read, edit, ...]
```

---

## Tool Resolution and Repair

When the LLM calls a tool, Bridge performs multi-step name resolution before executing it. This means agents rarely fail due to tool name typos -- Bridge auto-repairs common mistakes.

### Resolution Steps

1. **Exact match** -- The tool name is looked up against the registered tool names. If found, it executes immediately.
2. **Normalization** -- Quotes are stripped and whitespace is trimmed. For example, `"read"` and ` read ` both resolve to `read`.
3. **Case-insensitive match** -- If exact match fails, Bridge tries a case-insensitive lookup. `Read`, `READ`, and `read` all resolve to the same tool.
4. **Fuzzy match (auto-repair)** -- If case-insensitive match fails, Bridge computes Levenshtein distance against all registered tool names. If the best match has similarity > 0.8, Bridge auto-repairs and executes the correct tool. For example, `rea` resolves to `read`.
5. **Error with suggestion** -- If no match meets the auto-repair threshold, Bridge returns an error. If the closest match has similarity > 0.4, the error includes a suggestion:

```
Unknown tool 'bassh'. Did you mean 'bash'? Available tools: [bash, read, edit, ...]
```

### What This Means in Practice

- **Wrong casing** is always auto-corrected (step 3)
- **Extra whitespace or quotes** are always stripped (step 2)
- **Minor typos** (1-2 characters off) are usually auto-repaired (step 4)
- **Completely wrong names** get a helpful suggestion pointing to the closest match (step 5)

You do not need to configure this behavior. It applies to all tool calls automatically.

---

## Tool Timeouts

Different tools have different timeout behaviors:

| Tool | Default Timeout | Maximum | Notes |
|------|-----------------|---------|-------|
| `bash` | 120s (2 min) | 600s (10 min) | Configurable per command |
| `web_fetch` | 30s | — | 10 redirect limit |
| `web_search` | 15s | — | — |
| Integration tools | 30s | — | 3 retries with exponential backoff |
| Foreground subagent | 120s | — | — |
| Background subagent | 300s (5 min) | — | Surfaced as `[ERROR]` in the injected user-turn message |

### Timeout Behavior

- **Bash**: Kills process tree on timeout (prevents orphaned children)
- **Integration tools**: Retry on 5xx server errors, fail fast on 4xx client errors
- **Subagents**: Returns "timeout" status with error message

---

## JSON Schema Limitations

Bridge processes tool parameter schemas to ensure compatibility with various LLM providers:

### Schema Flattening

Schemas are automatically flattened to:
- Inline `$ref` references (resolves `#/definitions/` and `#/$defs/`)
- Remove schemars-specific keys (`$schema`, `title`, `definitions`, `$defs`)
- Simplify `oneOf`/`anyOf`/`allOf` patterns

### Type Enforcement

Every schema node must have a valid `type` field (required by Gemini's API):
- Missing types are inferred from structure (`properties` → `object`, `items` → `array`)
- Empty string types are removed
- Enum fields default to `string` type

### Best Practices

- Avoid deeply nested `$ref` chains
- Use simple types where possible
- Test schemas with `flatten_schema()` to verify compatibility

---

## Output Limits and Truncation

Tool output is automatically limited to prevent context overflow:

| Limit | Value | Behavior |
|-------|-------|----------|
| Max lines | 2,000 | Truncates with notice |
| Max bytes | 50 KB | Persists full output to disk |
| Max line length | 2,000 chars | Truncates long lines |

When output exceeds limits:
1. Content is truncated (head or tail based on tool)
2. Full output is saved to temp file (`/tmp/bridge_tool_output/`)
3. Response includes path to full output
4. Files older than 7 days are auto-cleaned

### Tool-Specific Limits

| Tool | Limit |
|------|-------|
| `read` | 2,000 lines, 50 KB |
| `bash` | 50 KB (spills to disk) |
| `web_fetch` | 50,000 chars content, 5 MB response size |
| `batch` | Aggregated results truncated to 50 KB |

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
