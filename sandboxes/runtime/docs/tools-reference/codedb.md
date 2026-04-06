# CodeDB

Advanced code search and analysis via an external MCP server.

---

## What Is CodeDB?

CodeDB is an external MCP server that provides advanced code search and analysis tools. It replaces Bridge's built-in filesystem and search tools with more powerful alternatives optimized for code navigation.

Source: [github.com/justrach/codedb](https://github.com/justrach/codedb)

---

## Enabling CodeDB

### Environment Variable

```bash
BRIDGE_CODEDB_ENABLED=true
```

### Config File

In `config.toml`:

```toml
codedb_enabled = true
```

---

## Binary Configuration

By default, Bridge looks for a `codedb` binary in your PATH. You can specify an explicit path:

### Environment Variable

```bash
BRIDGE_CODEDB_BINARY=/usr/local/bin/codedb
```

### Config File

In `config.toml`:

```toml
codedb_binary = "/usr/local/bin/codedb"
```

---

## What Happens When Enabled

When CodeDB is enabled, two things change:

1. **Built-in tools are removed** -- The built-in `grep`, `read`, and `glob` tools are NOT registered for any agent.
2. **CodeDB MCP server is auto-injected** -- The CodeDB MCP server is automatically added to every agent's tool set.

This is a global setting. All agents on the Bridge instance are affected.

---

## Tools Provided

CodeDB exposes these tools via MCP:

| Tool | What It Does |
|------|--------------|
| `codedb_search` | Semantic and keyword code search |
| `codedb_read` | Read file contents with code-aware context |
| `codedb_outline` | Get the outline (functions, classes, etc.) of a file |
| `codedb_symbol` | Look up symbol definitions and references |
| `codedb_tree` | Display directory tree structure |
| `codedb_word` | Word-level search across the codebase |
| `codedb_hot` | Find frequently changed files (hot spots) |
| `codedb_deps` | Analyze file and module dependencies |

---

## Tool Filtering

CodeDB tools respect the agent's tool allow-list. If an agent specifies a `tools` array, only CodeDB tools that match the allow-list are exposed.

```json
{
  "id": "search-only-agent",
  "tools": ["codedb_search", "codedb_read"]
}
```

This agent would only have access to `codedb_search` and `codedb_read`, not the full set of CodeDB tools.

---

## When to Use CodeDB vs Built-in Tools

| Scenario | Recommendation |
|----------|----------------|
| Large codebases with many files | CodeDB -- better search relevance and performance |
| Semantic code search (find by meaning, not just text) | CodeDB -- supports semantic search |
| Symbol navigation and dependency analysis | CodeDB -- purpose-built for this |
| Simple file reads and basic grep | Built-in tools work fine |
| Environments where you cannot install additional binaries | Built-in tools (no external dependency) |

---

## Setup

1. Download the CodeDB binary from [github.com/justrach/codedb](https://github.com/justrach/codedb)
2. Place it in your PATH or configure `BRIDGE_CODEDB_BINARY`
3. Set `BRIDGE_CODEDB_ENABLED=true`
4. Restart Bridge

All agents will automatically gain CodeDB tools and lose the built-in `grep`, `read`, and `glob` tools.

---

## See Also

- [Tools Concept](../core-concepts/tools.md) -- How tools work
- [Search Tools](search-tools.md) -- Built-in search (used when CodeDB is disabled)
- [Filesystem Tools](filesystem-tools.md) -- Built-in filesystem tools (used when CodeDB is disabled)
