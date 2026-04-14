# Tool Index

Complete index of all tools available to Bridge agents.

---

## Filesystem

| Name | Description |
|------|-------------|
| `Grep` | Search file contents using regular expressions. Supports glob/type filters, context lines, and multiple output modes (content, file paths, counts). Results sorted by modification time. |
| `Read` | Read a file from the filesystem. Returns up to 2000 lines with line numbers. Supports offset/limit for partial reads. Handles images (returns base64), PDFs (page ranges), and Jupyter notebooks. |
| `Glob` | Find files by glob pattern (e.g. `**/*.ts`, `src/**/*.rs`). Returns matching paths sorted by modification time. Respects `.gitignore`. |
| `LS` | List files and directories as a tree structure. Uses absolute paths. Respects `.gitignore`. |

## Write & Edit

| Name | Description |
|------|-------------|
| `bash` | Execute a shell command with optional timeout. Persistent working directory across calls. Supports background execution via `task_id`. |
| `edit` | Find-and-replace in a file. Requires prior `Read`. Uses a chain of matching strategies from exact to fuzzy. Supports `replace_all` for bulk renames. |
| `write` | Write or overwrite a file. Creates parent directories automatically. Requires prior `Read` for existing files. |
| `apply_patch` | Apply a unified-diff-style patch to one or more files. Supports add, delete, update, and move/rename operations. |
| `multiedit` | Multiple find-and-replace edits on a single file in one atomic operation. All edits succeed or none are applied. |

## Web

| Name | Description |
|------|-------------|
| `web_fetch` | Fetch a URL and extract readable content. Converts HTML to markdown/text/html using Readability. Follows redirects. Truncates at configurable max length. |
| `web_search` | Search the web and return results with titles, descriptions, and URLs. Optionally fetches full page content for each result. Requires `BRIDGE_WEB_URL`. |
| `web_crawl` | Crawl a website following links from a starting URL. Configurable depth, page limit, and rendering mode (http/chrome/smart). Requires `BRIDGE_WEB_URL`. |
| `web_get_links` | Extract all links from a webpage. Returns URLs found on the page. Requires `BRIDGE_WEB_URL`. |
| `web_screenshot` | Take a screenshot of a webpage. Returns base64-encoded PNG. Supports `wait_for_selector` for dynamic content. Requires `BRIDGE_WEB_URL`. |
| `web_transform` | Convert HTML to markdown or plain text without making HTTP requests. Batch-capable. Requires `BRIDGE_WEB_URL`. |

## Task Management

| Name | Description |
|------|-------------|
| `todowrite` | Create or replace the task list. Each task has content, status (pending/in_progress/completed/cancelled), and priority (high/medium/low). |
| `todoread` | Read the current task list. Returns all tasks with status and priority. |
| `journal_write` | Write an entry to the conversation journal. Persists across context resets during chain handoffs. Supports optional category. |
| `journal_read` | Read all journal entries including notes and checkpoint summaries from previous context chains. |

## Code Intelligence

| Name | Description |
|------|-------------|
| `lsp` | Query Language Server Protocol servers. Supports go-to-definition, find-references, hover, document/workspace symbols, call hierarchy, and diagnostics. Auto-starts LSP servers. Only available when LSP is configured. |

## Agent Orchestration

| Name | Description |
|------|-------------|
| `agent` | Launch a clone of the parent agent for a focused subtask. Shares system prompt, tools, and capabilities. Supports background execution. Not available to subagents. |
| `sub_agent` | Launch a named subagent (from the agent's `subagents` list) to handle a task. Supports background execution. Not available to subagents. |
| `parallel_agent` | Spawn up to 25 subagents in parallel and wait for all to complete. Supports per-task timeout and max concurrency. Not available to subagents. |
| `join` | Wait for background subagent tasks to complete. Blocks until all specified task IDs finish or timeout. Returns combined results. |
| `batch` | Execute 1-25 independent tool calls concurrently in a single operation. Partial failures don't stop other calls. No recursive batching. |

## Skills

| Name | Description |
|------|-------------|
| `skill` | Load a skill by name or ID. Returns skill content with variable substitution (`{{args}}`, `$ARGUMENTS`, `$1`). Supports requesting specific supporting files via the `file` parameter. Only available when the agent has skills defined. |

## Integration Tools

| Name | Description |
|------|-------------|
| `{integration}__{action}` | Dynamically registered per-agent. Each integration action becomes a tool (e.g. `github__create_pull_request`). Execution is proxied through the control plane to the external service. Schema and description defined per-action. |

## MCP Tools

| Name | Description |
|------|-------------|
| *(varies)* | Tools provided by MCP servers connected to the agent. Each MCP server advertises its own tools with names and schemas. Bridged into the agent's tool registry at load time. Available to both parent agents and subagents. |

---

## Availability

| Tool | Parent | Subagent | Condition |
|------|--------|----------|-----------|
| Grep, Read, Glob, LS | Yes | Yes | Always |
| bash, edit, write, apply_patch, multiedit | Yes | Yes | Always |
| web_fetch | Yes | Yes | Always |
| web_search, web_crawl, web_get_links, web_screenshot, web_transform | Yes | Yes | `BRIDGE_WEB_URL` set |
| todowrite, todoread | Yes | Yes | Always |
| journal_write, journal_read | Yes | Yes | Always |
| lsp | Yes | No | LSP configured |
| agent, sub_agent, parallel_agent | Yes | No | Always (prevents recursion) |
| join | Yes | No | Always |
| batch | Yes | Yes | Always |
| skill | Yes | Yes | Agent/subagent has `skills` defined |
| Integration tools | Yes | Inherited from parent | Agent has `integrations` defined |
| MCP tools | Yes | Yes (own servers) | Agent/subagent has `mcp_servers` defined |
