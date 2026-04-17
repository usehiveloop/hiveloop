# All Tools Reference

Every tool available to Bridge agents, with exact names and descriptions.

Any tool listed here can be disabled per-agent via `config.disabled_tools`:

```json
{
  "config": {
    "disabled_tools": ["bash", "write", "edit"]
  }
}
```

Disabled tools are completely removed — the LLM never sees them. See [Tools Reference](index.md) for the full permissions model.

---

## Filesystem

| Tool | Description |
|------|-------------|
| `Read` | Read a file from the local filesystem. Supports offset/limit for reading specific line ranges, images (PNG, JPG), PDFs (with page ranges), and Jupyter notebooks. Returns content with line numbers. |
| `write` | Write a file to the local filesystem. Overwrites existing files. Creates parent directories if needed. |
| `edit` | Perform exact string replacements in files. Finds `old_string` and replaces with `new_string`. Supports `replace_all` for bulk renames. Fails if `old_string` is not unique unless `replace_all` is set. |
| `multiedit` | Make multiple find-and-replace edits to a single file in one operation. More efficient than calling `edit` multiple times. |
| `apply_patch` | Apply a file-oriented diff patch. Supports creating, modifying, and deleting files using a stripped-down diff format. |
| `Glob` | Fast file pattern matching. Supports glob patterns like `**/*.rs` or `src/**/*.ts`. Returns matching file paths sorted by modification time. |
| `Grep` | Fast content search using regex. Supports context lines (`-A`, `-B`, `-C`), file type filtering, glob filtering, and output modes (`content`, `files_with_matches`, `count`). |
| `LS` | List files and directories at a given path. Prefer `Glob` and `Grep` when you know what to search for. |

## Shell

| Tool | Description |
|------|-------------|
| `bash` | Execute shell commands with optional timeout. Supports `background: true` for long-running tasks that return a task ID. Working directory persists between calls. |

## Web

| Tool | Description | Requires |
|------|-------------|----------|
| `web_fetch` | Fetch a single URL and extract readable content as markdown. Three-tier strategy: spider crate, fallback service, reqwest + readability. Supports markdown, text, and HTML output formats. | Always available |
| `web_search` | Search the web and return results with titles, descriptions, and URLs. Set `fetch_page_content: true` to retrieve full page content for each result in one call. Includes year-aware search guidance. | `BRIDGE_WEB_URL` |
| `web_crawl` | Crawl a website starting from a URL, following links to discover and return page content. Control scope with `limit` (max pages), `depth` (max link depth), and `request` mode (http/chrome/smart). Set `readability: true` for clean content extraction. | `BRIDGE_WEB_URL` |
| `web_get_links` | Extract all links from a webpage. Returns a list of discovered URLs. Useful for site structure discovery before targeted crawling. | `BRIDGE_WEB_URL` |
| `web_screenshot` | Take a screenshot of a webpage. Returns base64-encoded PNG. Use `request: "chrome"` for accurate rendering. Supports `wait_for_selector` to wait for dynamic content. | `BRIDGE_WEB_URL` |
| `web_transform` | Convert HTML content to markdown or plain text without making HTTP requests. Processes HTML you already have. Supports batch transformation of multiple items. | `BRIDGE_WEB_URL` |

## Agent Orchestration

| Tool | Description |
|------|-------------|
| `agent` | Launch a clone of yourself to handle a focused task autonomously. The clone runs in a fresh context with the same tools and system prompt. Supports `runInBackground`. |
| `sub_agent` | Launch a named subagent (from the agent's `subagents` list). Runs foreground by default; pass `runInBackground: true` to fire-and-forget — the subagent's final output is auto-injected into the parent's next user turn. Fan-out by emitting multiple `sub_agent` tool_use blocks in one assistant turn. |
| `batch` | Execute multiple independent tool calls concurrently in a single request. Reduces latency by parallelizing tools that don't depend on each other. |

## Task Management

| Tool | Description |
|------|-------------|
| `todowrite` | Create and manage a structured task list. Uses replace-all semantics — each call sends the complete list. Track tasks as pending, in_progress, completed, or cancelled with priority levels. |
| `todoread` | Read the current task/todo list. Returns all items with their status and priority. |

## Journal (Immortal Mode)

Available when the agent has `config.immortal` set. Journal entries survive context chain handoffs.

| Tool | Description |
|------|-------------|
| `journal_write` | Write a high-signal entry to the conversation journal. Record key decisions, discoveries, user preferences, or constraints. Entries persist across context resets. Use sparingly — only for information that must not be lost. |
| `journal_read` | Read all journal entries. Returns agent notes and checkpoint summaries from all previous context chains, with chain index and timestamps. |

## Code Intelligence

| Tool | Description | Requires |
|------|-------------|----------|
| `lsp` | Query Language Server Protocol servers for diagnostics, hover information, go-to-definition, find references, and completions. | LSP manager configured |
| `skill` | Invoke a skill defined in the agent's skills array. Skills are reusable prompt templates that can accept arguments. | Agent has `skills` defined |

## Integration Tools

Defined per-agent in the `integrations` array. Each integration action becomes a tool named `{integration}_{action}`. Permissions are set per-action.

Example: An integration named `github` with action `create_pull_request` becomes a tool called `github_create_pull_request`.

## MCP Server Tools

Any MCP server connected to the agent exposes its tools. Tool names and descriptions are defined by the MCP server. Configure via the agent's `mcp_servers` array.
