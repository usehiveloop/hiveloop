# Web Tools

Fetch web pages and search the internet.

---

## web_fetch

Fetch and extract content from a URL.

### Parameters

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `url` | string | Yes | URL to fetch. Must be a fully-formed valid URL. HTTP is upgraded to HTTPS. |
| `max_length` | number | No | Maximum content length in characters. Default: 50000 |
| `format` | string | No | Output format: `markdown` (default), `text`, or `html` |

### Output Formats

- **`markdown`** (default): Converts HTML to Markdown using Mozilla Readability algorithm for article extraction, with fallback to full HTML-to-Markdown conversion
- **`text`**: Strips all HTML tags, returns plain text
- **`html`**: Returns raw HTML as-is

### Example

```json
{
  "name": "web_fetch",
  "arguments": {
    "url": "https://docs.rs/tokio/latest/tokio/",
    "format": "markdown",
    "max_length": 10000
  }
}
```

### Result

```json
{
  "success": true,
  "result": {
    "title": "tokio - Rust",
    "content": "tokio - An event-driven...",
    "url": "https://docs.rs/tokio/latest/tokio/"
  }
}
```

### Use Cases

- Read documentation pages
- Check status of external services
- Fetch configuration from URLs
- Verify links

### Limitations & Behavior

- **30-second timeout** per request
- **5MB response size limit** - responses larger than 5MB are rejected
- **Maximum 10 redirect hops** - fails if redirect chain exceeds this limit
- **HTML only** - rejects non-HTML content types (e.g., PDF, JSON) with an error
- **Cloudflare handling**: On 403 responses with `cf-mitigated: challenge` header, automatically retries with a simpler User-Agent
- **Image support**: Image content types (except SVG) are returned as base64 data URLs
- **Article extraction**: Uses Mozilla Readability algorithm; may not work with JavaScript-heavy sites or non-article pages
- **Authentication**: Will fail for authenticated or private URLs (pages behind login walls)

### Error Conditions

| Error | Cause |
|-------|-------|
| `URL must not be empty` | Empty or whitespace-only URL provided |
| `Request timed out` | Request exceeded 30-second timeout |
| `Too many redirects` | Redirect chain exceeded 10 hops |
| `Response too large` | Response body exceeds 5MB limit |
| `Non-HTML content type` | Content-Type is not HTML/XHTML/SVG |
| `HTTP error {status}` | Server returned non-success status (4xx, 5xx) |

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
| `query` | string | Yes | Search query |

### Example

```json
{
  "name": "web_search",
  "arguments": {
    "query": "Bridge AI runtime documentation"
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
      "snippet": "Bridge is an AI runtime...",
      "position": 1
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

### When to Use

Use `web_search` when:
- Looking up documentation
- Finding examples
- Researching current information
- Checking for updates

### Limitations & Behavior

- **15-second timeout** per search request
- **Retry logic**: Automatically retries on server errors (5xx) or request failures with exponential backoff (3 attempts, 500ms-5s delay)
- Returns knowledge graph data (when available) as the first result with `position: 0`
- If the endpoint returns an error, the tool will fail with the HTTP status code and response body

---

## Combining Tools

Search, then fetch:

```
1. web_search for "Rust error handling best practices"
2. web_fetch the most relevant result
3. Read and summarize for the user
```

---

## Configuration

### web_fetch
No configuration required. Works out of the box.

### web_search
Requires the `SEARCH_ENDPOINT` environment variable:

```bash
export SEARCH_ENDPOINT="https://your-search-service.com/search"
```

The search endpoint is expected to:
- Accept POST requests with JSON body: `{ "q": "search query" }`
- Return JSON in Serper API format with `organic` and optional `knowledgeGraph` fields
- Handle API key authentication internally (if required by your search provider)

Popular search providers that use Serper format:
- [Serper.dev](https://serper.dev) - Google Search API
- Self-hosted search services compatible with Serper format

---

## Rate Limits

Rate limits depend on the underlying services:

- **web_fetch**: No built-in rate limits, but respect target site's robots.txt and terms of service
- **web_search**: Depends on your search provider (configure rate limiting at the search endpoint)

If you hit limits:
- Wait between requests
- Cache results when possible
- Use specific URLs rather than broad searches

---

## See Also

- [grep](search-tools.md) — Search local files
