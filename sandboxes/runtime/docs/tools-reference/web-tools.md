# Web Tools

Fetch web pages and search the internet.

---

## web_fetch

Fetch and extract content from a URL.

### Parameters

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `url` | string | Yes | URL to fetch |

### Example

```json
{
  "name": "web_fetch",
  "arguments": {
    "url": "https://docs.rs/tokio/latest/tokio/"
  }
}
```

### Result

```json
{
  "success": true,
  "result": "tokio - An event-driven...",
  "metadata": {
    "title": "tokio - Rust",
    "url": "https://docs.rs/tokio/latest/tokio/",
    "content_type": "text/html"
  }
}
```

### Use Cases

- Read documentation pages
- Check status of external services
- Fetch configuration from URLs
- Verify links

### Limitations

- Fetches HTML and extracts text
- May not work with JavaScript-heavy sites
- 30-second timeout

---

## web_search

Search the web.

### Parameters

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `query` | string | Yes | Search query |
| `num_results` | number | No | Number of results (default: 5) |

### Example

```json
{
  "name": "web_search",
  "arguments": {
    "query": "Bridge AI runtime documentation",
    "num_results": 5
  }
}
```

### Result

```json
{
  "success": true,
  "result": [
    {
      "title": "Bridge Documentation",
      "url": "https://docs.bridge.example.com",
      "snippet": "Bridge is an AI runtime..."
    }
  ]
}
```

### When to Use

Use `web_search` when:
- Looking up documentation
- Finding examples
- Researching current information
- Checking for updates

---

## Combining Tools

Search, then fetch:

```
1. web_search for "Rust error handling best practices"
2. web_fetch the most relevant result
3. Read and summarize for the user
```

---

## Rate Limits

Web tools may have rate limits depending on the underlying service. If you hit limits:

- Wait between requests
- Cache results when possible
- Use specific URLs rather than broad searches

---

## See Also

- [grep](search-tools.md) — Search local files
