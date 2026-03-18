# Search Tools

Search file contents and the web.

---

## grep

Search files with regular expressions.

### Parameters

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `pattern` | string | Yes | Regex pattern to search for |
| `path` | string | Yes | Directory or file to search |
| `file_pattern` | string | No | Only search files matching this glob |

### Example

```json
{
  "name": "grep",
  "arguments": {
    "pattern": "function",
    "path": "/home/user/project/src",
    "file_pattern": "*.js"
  }
}
```

### Result

```json
{
  "success": true,
  "result": [
    {
      "file": "/home/user/project/src/utils.js",
      "line": 5,
      "content": "function helper() {",
      "match": "function"
    },
    {
      "file": "/home/user/project/src/main.js",
      "line": 10,
      "content": "function main() {",
      "match": "function"
    }
  ]
}
```

### Pattern Examples

```json
// Find TODOs
{ "pattern": "TODO|FIXME", "path": "." }

// Find specific function
{ "pattern": "function calculateTotal", "path": "." }

// Find variable assignments
{ "pattern": "const.*=.*require", "path": "." }

// Find class definitions
{ "pattern": "class \\w+", "path": "." }
```

### Limitations

- Returns first 100 matches by default
- Searches text files only
- Binary files are skipped

---

## web_search

Search the web.

### Parameters

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `query` | string | Yes | Search query |
| `num_results` | number | No | Max results (default: 5) |

### Example

```json
{
  "name": "web_search",
  "arguments": {
    "query": "Rust async await tutorial",
    "num_results": 3
  }
}
```

### Result

```json
{
  "success": true,
  "result": [
    {
      "title": "Asynchronous Programming in Rust",
      "url": "https://rust-lang.github.io/async-book/",
      "snippet": "Asynchronous programming in Rust..."
    },
    {
      "title": "Rust Async/Await",
      "url": "https://example.com/rust-async",
      "snippet": "Learn how to use async/await..."
    }
  ]
}
```

### Use Cases

- Look up documentation
- Find examples
- Research topics
- Check current information

---

## Choosing grep vs web_search

| Use | Tool |
|-----|------|
| Search codebase | `grep` |
| Find docs online | `web_search` |
| Check for TODOs | `grep` |
| Research APIs | `web_search` |

---

## Advanced grep Patterns

### Find All Functions

```json
{
  "pattern": "^(async\\s+)?function\\s+\\w+|const\\s+\\w+\\s*=\\s*(async\\s+)?(\\(.*?\\)|\\w+)\\s*=>",
  "path": "src"
}
```

### Find Imports

```json
{
  "pattern": "^(import|require)",
  "path": "."
}
```

### Find Console Logs

```json
{
  "pattern": "console\\.(log|warn|error)",
  "path": "src"
}
```

---

## See Also

- [bash](bash-tool.md) — For complex searches with `find` and `grep`
- [web_fetch](web-tools.md) — Fetch specific web pages
