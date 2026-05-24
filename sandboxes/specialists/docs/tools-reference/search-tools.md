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

Search the web. **Note: This tool requires configuration and will only be available if properly set up.**

### Prerequisites

The `web_search` tool requires the `SEARCH_ENDPOINT` environment variable to be set. Without it, the tool will not be registered and will be unavailable.

```bash
# Required environment variable
export SEARCH_ENDPOINT="https://your-search-api.com/search"
```

The endpoint must accept POST requests with a JSON body containing `{ "q": "search query" }` and return results in Serper API format.

### Parameters

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `query` | string | Yes | Search query string |

### Example

```json
{
  "name": "web_search",
  "arguments": {
    "query": "Rust async await tutorial"
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
      "snippet": "Asynchronous programming in Rust...",
      "position": 1
    },
    {
      "title": "Rust Async/Await",
      "url": "https://example.com/rust-async",
      "snippet": "Learn how to use async/await...",
      "position": 2
    }
  ]
}
```

### Response Format

The search endpoint must return a JSON response in Serper format:

```json
{
  "organic": [
    {
      "title": "Result Title",
      "link": "https://example.com",
      "snippet": "Result description...",
      "position": 1
    }
  ],
  "knowledgeGraph": {
    "title": "Knowledge Graph Title",
    "description": "Description...",
    "attributes": { "key": "value" }
  }
}
```

### Use Cases

- Look up documentation
- Find examples
- Research topics
- Check current information

### Limitations & Behavior

- **15-second timeout** per search request
- **Retry logic**: Automatically retries on server errors (5xx) or request failures with exponential backoff (3 attempts, 500ms-5s delay)
- Returns knowledge graph data (when available) as the first result with `position: 0`
- If the endpoint returns an error, the tool will fail with the HTTP status code and response body

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
