# LSP Tools

Query Language Server Protocol (LSP) servers for code intelligence.

---

## Overview

The `lsp_query` tool connects to language servers to provide:

- Go to definition
- Find references
- Type information
- Symbol search

Useful for code understanding and refactoring.

---

## lsp_query

Query a language server.

### Parameters

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `language` | string | Yes | Language server to use (e.g., "rust", "typescript") |
| `query_type` | string | Yes | Type of query |
| `file_path` | string | Yes | Path to the file |
| `position` | object | Yes | `{ line, character }` position in file |

### Query Types

| Type | Description |
|------|-------------|
| `definition` | Go to symbol definition |
| `references` | Find all references to symbol |
| `hover` | Get type/documentation info |
| `document_symbols` | List all symbols in file |

---

## Examples

### Go to Definition

```json
{
  "name": "lsp_query",
  "arguments": {
    "language": "rust",
    "query_type": "definition",
    "file_path": "/project/src/main.rs",
    "position": {
      "line": 10,
      "character": 15
    }
  }
}
```

Result:

```json
{
  "success": true,
  "result": [
    {
      "uri": "file:///project/src/lib.rs",
      "range": {
        "start": { "line": 25, "character": 0 },
        "end": { "line": 30, "character": 1 }
      }
    }
  ]
}
```

### Find References

```json
{
  "name": "lsp_query",
  "arguments": {
    "language": "typescript",
    "query_type": "references",
    "file_path": "/project/src/api.ts",
    "position": {
      "line": 5,
      "character": 10
    }
  }
}
```

### Get Type Info

```json
{
  "name": "lsp_query",
  "arguments": {
    "language": "rust",
    "query_type": "hover",
    "file_path": "/project/src/main.rs",
    "position": {
      "line": 15,
      "character": 8
    }
  }
}
```

Result:

```json
{
  "success": true,
  "result": {
    "contents": "```rust\nfn process(data: &str) -> Result<String, Error>\n```\n\nProcesses the input data..."
  }
}
```

---

## Configuration

Configure LSP servers in `config.toml`:

```toml
[lsp.rust]
command = ["rust-analyzer"]
extensions = ["rs"]

[lsp.typescript]
command = ["typescript-language-server", "--stdio"]
extensions = ["ts", "tsx", "js", "jsx"]

[lsp.python]
command = ["pylsp"]
extensions = ["py"]
```

### Fields

| Field | Description |
|-------|-------------|
| `command` | How to start the language server |
| `extensions` | File extensions this server handles |

---

## Installing Language Servers

Before using LSP tools, install the servers:

### Rust

```bash
rustup component add rust-analyzer
```

### TypeScript

```bash
npm install -g typescript-language-server typescript
```

### Python

```bash
pip install python-lsp-server
```

---

## Line and Character Numbers

Positions are 0-indexed:

- Line 0 = first line
- Character 0 = first character on the line

Most editors show 1-indexed numbers, so subtract 1 when using this tool.

---

## Common Errors

| Error | Fix |
|-------|-----|
| "LSP server not configured" | Add to `config.toml` |
| "Server not running" | Install the language server |
| "No result" | Cursor not on a symbol, or symbol not indexed |

---

## Use Cases

### Understanding Code

"What does this function do?"

```
lsp_query(hover) at function call
→ Shows documentation and signature
```

### Refactoring

"Where is this used?"

```
lsp_query(references) at symbol
→ Shows all usages
```

### Navigation

"Where is this defined?"

```
lsp_query(definition) at symbol
→ Jumps to definition
```

---

## See Also

- [LSP Configuration](../getting-started/configuration.md#lsp-configuration)
- [read](filesystem-tools.md) — Read file contents
