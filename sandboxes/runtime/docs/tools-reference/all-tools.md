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
| `agent` | Launch a clone of yourself to handle a focused task autonomously. The clone runs in a fresh context with the same tools and system prompt. |
| `sub_agent` | Launch a named subagent to handle complex, multistep tasks. Subagents can run in foreground (blocking) or background (concurrent). Each subagent has its own conversation history. |
| `parallel_agent` | Spawn multiple subagents in parallel and wait for all to complete. Useful for independent tasks that can run concurrently. |
| `batch` | Execute multiple independent tool calls concurrently in a single request. Reduces latency by parallelizing tools that don't depend on each other. |
| `join` | Wait for one or more background subagent tasks to complete by task ID. Returns the task results. |

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

## CodeDB (MCP)

Available when `BRIDGE_CODEDB_ENABLED=true`. Replaces `Read`, `Grep`, and `Glob` with code-aware alternatives. Tools are provided by the CodeDB MCP server.

| Tool | Description |
|------|-------------|
| `codedb_outline` | **Start here.** Get the structural outline of a file: all functions, structs, enums, imports, constants with line numbers. Returns 4-15x fewer tokens than reading the raw file. Always use before `codedb_read`. |
| `codedb_tree` | Get the full file tree of the indexed codebase with language detection, line counts, and symbol counts per file. Use first to understand project structure. |
| `codedb_symbol` | Find where a symbol is defined across the codebase. Returns file, line, and kind (function/struct/import). Use `body=true` to include source code. More precise than search — finds definitions, not text matches. |
| `codedb_search` | Full-text search across all indexed files. Returns matching lines with file paths and line numbers. Use `scope=true` to see the enclosing function/struct. For single identifiers, prefer `codedb_word` or `codedb_symbol`. |
| `codedb_word` | O(1) word lookup using inverted index. Finds all occurrences of an exact word (identifier) across the codebase. Much faster than search for single-word queries. |
| `codedb_find` | Fuzzy file search — finds files by approximate name. Typo-tolerant subsequence matching. Use when you know roughly what file you're looking for but not the exact path. |
| `codedb_read` | Read file contents. Use `codedb_outline` first to find line numbers, then read only that range with `line_start`/`line_end`. Use `compact=true` to skip comments and blanks. |
| `codedb_edit` | Apply a line-based edit to a file. Supports replace (range), insert (after line), and delete (range) operations. |
| `codedb_hot` | Get the most recently modified files, ordered by recency. Useful to see what's been actively worked on. |
| `codedb_deps` | Get reverse dependencies: which files import/depend on the given file. Useful for impact analysis before making changes. |
| `codedb_changes` | Get files that changed since a sequence number. Use with `codedb_status` to poll for changes. |
| `codedb_status` | Get current codedb status: number of indexed files and current sequence number. |
| `codedb_bundle` | Batch multiple queries in one call (max 20 ops). Bundle outline+symbol+search for efficiency. Avoid bundling multiple full file reads. |
| `codedb_snapshot` | Get the full pre-rendered snapshot of the codebase as a single JSON blob. Contains tree, outlines, symbol index, and dependency graph. |
| `codedb_remote` | Query any GitHub repo via codedb cloud intelligence. Gets file tree, symbol outlines, or searches code in external repos without cloning. |
| `codedb_projects` | List all locally indexed projects. Shows project paths, data directory hashes, and snapshot availability. |
| `codedb_index` | Index a local folder. Scans source files, builds outlines/trigrams/word indexes, creates a snapshot. After indexing, the folder is queryable via the `project` param. |

## Integration Tools

Defined per-agent in the `integrations` array. Each integration action becomes a tool named `{integration}_{action}`. Permissions are set per-action.

Example: An integration named `github` with action `create_pull_request` becomes a tool called `github_create_pull_request`.

## MCP Server Tools

Any MCP server connected to the agent exposes its tools. Tool names and descriptions are defined by the MCP server. Configure via the agent's `mcp_servers` array.
