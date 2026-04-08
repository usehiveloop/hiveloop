# Tools Reference

Complete reference for all tools available to agents.

---

## Tool Permissions

Control tool execution behavior per-agent using the `permissions` field. Tools not listed default to `allow`.

```json
{
  "permissions": {
    "bash": "require_approval",
    "write": "require_approval",
    "edit": "require_approval",
    "Read": "allow",
    "Grep": "allow"
  }
}
```

| Permission | Behavior |
|------------|----------|
| `allow` | Runs immediately (default for all tools) |
| `require_approval` | Pauses and waits for user approval via the approvals API |
| `deny` | Blocked — returns an error to the LLM. The tool is still visible to the LLM but execution fails with an error message |

---

## Disabling Tools

Use `disabled_tools` in the agent config to completely remove tools from the agent. Disabled tools are removed from the registry before the agent is built — the LLM never sees them. This takes priority over everything else, including `permissions` and `tools` allow-lists.

Works for any tool type: built-in, MCP, codedb, spider, integration, and skills.

```json
{
  "config": {
    "disabled_tools": ["bash", "write", "edit", "multiedit", "apply_patch"]
  }
}
```

**`disabled_tools` vs `permissions: deny`:**

| | `disabled_tools` | `permissions: "deny"` |
|---|---|---|
| LLM sees the tool | No | Yes |
| LLM can attempt to call it | No | Yes (gets an error back) |
| Wastes a tool call turn | No | Yes |
| Use when | Tool should not exist for this agent | Tool exists but needs to be gated |

---

## All Built-in Tools

These tools are always available to every agent. Tool names are case-sensitive — use them exactly as shown.

### Filesystem

| Tool Name | Description |
|-----------|-------------|
| `Read` | Read file contents with line numbers. Supports offset/limit for large files, images, PDFs |
| `write` | Create or overwrite a file |
| `edit` | Make targeted string replacements in a file. Supports `replace_all` for bulk renames |
| `multiedit` | Apply multiple edits to a file in a single call |
| `apply_patch` | Apply unified diff patches |
| `Glob` | Find files by glob pattern (e.g. `**/*.rs`, `src/**/*.ts`) |
| `Grep` | Search file contents using regex. Supports context lines, file type filtering, output modes |
| `LS` | List directory contents |

### Shell

| Tool Name | Description |
|-----------|-------------|
| `bash` | Run shell commands. Supports `background: true` for long-running tasks |

### Web (always available)

| Tool Name | Description | Requires |
|-----------|-------------|----------|
| `web_fetch` | Fetch a single URL and convert to markdown. Three-tier: spider crate, fallback service, reqwest | — |

### Web (requires `BRIDGE_WEB_URL`)

These tools are registered when the `BRIDGE_WEB_URL` environment variable is set, pointing to a Spider API instance.

| Tool Name | Description |
|-----------|-------------|
| `web_search` | Search the web. Supports `fetch_page_content` to retrieve full pages in one call |
| `web_crawl` | Crawl a website following links. Control with `limit`, `depth`, `request` mode |
| `web_get_links` | Extract all links from a webpage |
| `web_screenshot` | Take a screenshot of a webpage. Returns base64 PNG |
| `web_transform` | Convert HTML to markdown without HTTP requests |

### Agent Orchestration

| Tool Name | Description |
|-----------|-------------|
| `agent` | Self-delegation — the agent delegates a task to itself in a fresh context |
| `sub_agent` | Spawn a subagent (foreground or background) |
| `parallel_agent` | Run multiple subagents concurrently |
| `batch` | Execute multiple tools sequentially in a single call |
| `join` | Wait for a background task to complete by task ID |

### Task Management

| Tool Name | Description |
|-----------|-------------|
| `todowrite` | Create or update the task/todo list (replace-all semantics) |
| `todoread` | Read the current task/todo list |

### Journal (immortal mode only)

These tools are registered when `config.immortal` is set on the agent. They provide persistent notes that survive context chain handoffs.

| Tool Name | Description |
|-----------|-------------|
| `journal_write` | Write a high-signal entry (decisions, discoveries, preferences) |
| `journal_read` | Read all journal entries |

### Code Intelligence

| Tool Name | Description | Requires |
|-----------|-------------|----------|
| `lsp` | Query language servers for diagnostics, hover info, completions | LSP manager configured |

### Skills

| Tool Name | Description | Requires |
|-----------|-------------|----------|
| `skill` | Invoke a skill defined in the agent's `skills` array | Agent has skills defined |

---

## CodeDB Tools (MCP)

When `BRIDGE_CODEDB_ENABLED=true`, the CodeDB MCP server is auto-injected and provides these tools. They replace `Read`, `Grep`, and `Glob` with code-aware alternatives.

| Tool Name | Description |
|-----------|-------------|
| `codedb_search` | Semantic and keyword code search |
| `codedb_read` | Read file contents with code-aware context |
| `codedb_outline` | Get file outline (functions, classes, structs, etc.) |
| `codedb_symbol` | Look up symbol definitions and references |
| `codedb_tree` | Display directory tree structure |
| `codedb_word` | Word-level search across the codebase |
| `codedb_hot` | Find frequently changed files (hot spots) |
| `codedb_deps` | Analyze file and module dependencies |
| `codedb_edit` | Code-aware file editing |
| `codedb_bundle` | Bundle multiple files into a single context |
| `codedb_status` | Show indexing and project status |
| `codedb_changes` | Show recent file changes |
| `codedb_snapshot` | Capture project state snapshot |
| `codedb_remote` | Fetch remote repository information |
| `codedb_projects` | List indexed projects |
| `codedb_index` | Trigger re-indexing |

CodeDB tools respect the agent's `tools` allow-list:

```json
{
  "tools": ["codedb_search", "codedb_read", "codedb_outline"]
}
```

---

## Integration Tools

Integration tools are defined per-agent in the `integrations` array. Each integration action becomes a tool. Tool names follow the pattern `{integration_name}_{action_name}`.

Each action has its own permission level:

```json
{
  "integrations": [
    {
      "name": "github",
      "base_url": "https://api.example.com/integrations/github",
      "actions": [
        {
          "name": "create_pull_request",
          "description": "Create a PR",
          "parameters_schema": {"type": "object"},
          "permission": "require_approval"
        },
        {
          "name": "list_issues",
          "description": "List issues",
          "parameters_schema": {"type": "object"},
          "permission": "allow"
        }
      ]
    }
  ]
}
```

---

## MCP Server Tools

Any MCP server connected to the agent exposes its tools. Tool names are determined by the MCP server.

```json
{
  "mcp_servers": [
    {
      "name": "my-database",
      "transport": {
        "type": "stdio",
        "command": "my-db-mcp",
        "args": ["--port", "5432"]
      }
    }
  ]
}
```

---

## Example: Full Agent with Permissions

```json
{
  "id": "secure-coding-agent",
  "name": "Secure Coding Agent",
  "system_prompt": "You are a careful coding assistant.",
  "provider": {
    "provider_type": "open_ai",
    "model": "gpt-4o",
    "api_key": "<provider-api-key>"
  },
  "tools": [],
  "mcp_servers": [],
  "skills": [],
  "integrations": [],
  "config": {
    "max_turns": 50,
    "disabled_tools": ["parallel_agent", "batch"],
    "immortal": {
      "token_budget": 100000,
      "carry_forward_turns": 2,
      "checkpoint_provider": {
        "provider_type": "open_ai",
        "model": "gpt-4o-mini",
        "api_key": "<provider-api-key>"
      }
    }
  },
  "permissions": {
    "bash": "require_approval",
    "write": "require_approval",
    "edit": "require_approval",
    "multiedit": "require_approval",
    "apply_patch": "require_approval",
    "web_fetch": "allow",
    "web_crawl": "allow",
    "web_search": "allow",
    "Read": "allow",
    "Grep": "allow",
    "Glob": "allow",
    "LS": "allow",
    "todowrite": "allow",
    "todoread": "allow",
    "journal_write": "allow",
    "journal_read": "allow",
    "agent": "allow",
    "sub_agent": "deny"
  }
}
```

---

## See Also

- [Tools Concept](../core-concepts/tools.md) — How tools work
- [CodeDB](codedb.md) — Code intelligence MCP server
- [Agent Tools](agent-tools.md) — Subagent orchestration
- [Skill Tool](skill-tool.md) — Skill invocation
- [Integration Tools](integration-tools.md) — External service connectors
