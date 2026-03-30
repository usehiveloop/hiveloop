# Proxy — Sub-5ms Proxy to Any Provider

> One proxy. Every LLM provider. Every SaaS app. Sub-5ms overhead.

---

## The Problem

AI agents need to call LLM providers and SaaS APIs. But each provider authenticates differently, streams responses differently, and reports usage differently. Teams end up building custom HTTP clients for every provider, managing credentials in application code, and missing usage data because they can't parse streaming response formats.

Meanwhile, the performance overhead of routing through a proxy matters. LLM responses are latency-sensitive — especially for streaming. Users notice every extra millisecond of time-to-first-byte. A slow proxy layer defeats the purpose.

### Industry Data

- **Bifrost** (Go-based gateway) adds only **11 microseconds** of overhead at 5,000 req/s — proving that Go proxy architectures can be effectively zero-cost ([Maxim AI](https://www.getmaxim.ai/articles/top-5-llm-gateways-in-2026-for-enterprise-grade-reliability-and-scale/))
- Python-based gateways (LiteLLM) introduce **500us to 40-50ms** overhead with degradation at scale ([Maxim AI](https://www.getmaxim.ai/articles/top-5-llm-gateways-in-2026-for-enterprise-grade-reliability-and-scale/))
- Cloudflare AI Gateway adds **10-50ms** proxy latency ([Maxim AI](https://www.getmaxim.ai/articles/top-5-llm-gateways-in-2026-for-enterprise-grade-reliability-and-scale/))
- Go-based gateways show **54x faster P99 latency** and **9.4x higher throughput** vs. Python on identical hardware ([Maxim AI](https://www.getmaxim.ai/articles/top-5-llm-gateways-in-2026-for-enterprise-grade-reliability-and-scale/))
- Teams with LLM monitoring at launch spend **38.7% less** on API costs in month one — visibility drives optimization ([Braintrust](https://www.braintrust.dev/articles/best-llm-monitoring-tools-2026))
- "5% of requests consume 50% of tokens" — granular per-request tracking is essential for cost control ([TrueFoundry](https://www.truefoundry.com/blog/best-ai-observability-platforms-for-llms-in-2026))

---

## The Solution

LLMVault Proxy is a Go-based streaming reverse proxy that handles both LLM providers and SaaS apps. It resolves encrypted credentials from a 3-tier cache, attaches the correct auth header for the provider, streams responses with `FlushInterval: -1`, and captures token usage from streaming chunks in real-time — all with sub-5ms hot-path overhead.

---

## Two Proxy Modes

### LLM Proxy — `/v1/proxy/*`

For proxying requests to LLM providers (OpenAI, Anthropic, Google, etc.):

```bash
POST /v1/proxy/v1/messages
Authorization: Bearer ptok_eyJhbG...
Content-Type: application/json

{
  "model": "claude-sonnet-4-6",
  "messages": [{ "role": "user", "content": "Hello" }],
  "stream": true
}
```

The proxy:
1. Validates the `ptok_` JWT (signature, expiry, revocation)
2. Resolves the credential from the 3-tier cache (memguard → Redis → Postgres/KMS)
3. Decrypts the API key in memory
4. Detects the auth scheme from the credential (Bearer, x-api-key, query_param, api-key)
5. Attaches the correct auth header to the upstream request
6. Streams the response back with immediate chunk flushing
7. Captures usage data from streaming chunks without adding latency
8. Zeros the decrypted key from memory

The agent's `ptok_` token never touches the provider. The provider's API key never touches the agent.

### SaaS Proxy — `/v1/connections/{id}/proxy/*`

For proxying requests to SaaS APIs (Slack, GitHub, HubSpot, etc.) through Nango:

```bash
POST /v1/connections/conn_abc123/proxy/api/chat.postMessage
Authorization: Bearer org_api_key
Content-Type: application/json

{
  "channel": "C01234",
  "text": "Hello from the agent"
}
```

The proxy:
1. Validates the API key and org context
2. Looks up the connection and its integration
3. Forwards via `nango.RawProxyRequest()` with managed OAuth headers
4. Nango handles token refresh if the access token has expired
5. Returns the upstream response with status code and content-type passthrough

---

## Auth Scheme Abstraction

Every LLM provider authenticates differently. The proxy handles this automatically:

| Provider | Auth Scheme | How the Proxy Handles It |
|---|---|---|
| OpenAI | `Authorization: Bearer sk-...` | Attaches as Bearer token |
| Anthropic | `x-api-key: sk-ant-...` | Attaches as custom header |
| Google | `?key=AIza...` | Appends as query parameter |
| Azure | `api-key: ...` | Attaches as custom header |
| All others | Bearer (default) | Attaches as Bearer token |

Configured via `knownAuthSchemes` map. Supports 101 LLM providers and 3,183 models via the embedded provider registry.

---

## Real-Time Usage Capture

The proxy captures token usage from every response without adding latency, using `CaptureTransport` — a wrapper around the HTTP transport:

### Streaming Responses (SSE)

For streaming responses (`text/event-stream`), the capture transport wraps the response body:
- Parses each `data: ` SSE event as it flows through
- Extracts usage from the final/summary chunk
- Captures TTFB (time-to-first-byte) from the first chunk
- No buffering — chunks flow to the client immediately

### Non-Streaming Responses

For JSON responses, reads the body, extracts usage, and re-serves it.

### Provider-Aware Parsing

Usage normalization across three response formats:

| Provider | Input Field | Output Field | Cache Field | Reasoning Field |
|---|---|---|---|---|
| **OpenAI** | `usage.prompt_tokens` | `usage.completion_tokens` | `prompt_tokens_details.cached_tokens` | `completion_tokens_details.reasoning_tokens` |
| **Anthropic** | `usage.input_tokens` | `usage.output_tokens` | `usage.cache_read_input_tokens` | — |
| **Google** | `usageMetadata.promptTokenCount` | `usageMetadata.candidatesTokenCount` | `usageMetadata.cachedContentTokenCount` | — |

All normalized to: `input_tokens`, `output_tokens`, `cached_tokens`, `reasoning_tokens`.

### Model Extraction

The proxy peeks at the first 2,048 bytes of the request body to extract the `model` field without buffering the entire payload. The body is reconstructed intact for the upstream.

---

## Captured Data Per Request

Every proxied request generates a `CapturedData` record:

| Field | Description |
|---|---|
| `provider_id` | Auto-detected provider (openai, anthropic, google, etc.) |
| `model` | Extracted from request body |
| `is_streaming` | Whether the response was SSE |
| `input_tokens` | Prompt/input token count |
| `output_tokens` | Completion/output token count |
| `cached_tokens` | Cache-hit tokens (prompt caching) |
| `reasoning_tokens` | Chain-of-thought tokens (OpenAI o-series) |
| `ttfb_ms` | Time to first byte |
| `total_ms` | Total request duration |
| `upstream_status` | HTTP status from the provider |
| `error_type` | Classified: timeout, connection_error, rate_limit, auth, upstream_error, client_error |
| `error_message` | First 512 bytes of error response |

---

## Middleware Stack

Every proxy request passes through:

1. **TokenAuth** — validates `ptok_` JWT (signature, expiry, revocation via database check)
2. **IdentityRateLimit** — enforces per-identity rate limits from the identity's `ratelimits` config
3. **RemainingCheck** — enforces per-credential and per-token request caps (`remaining` / `refill_amount` / `refill_interval`)
4. **Audit** — logs the request to the audit trail
5. **Generation** — saves the captured data as a generation record

---

## Full API Surface

### LLM Proxy
| Endpoint | Description |
|---|---|
| `* /v1/proxy/*` | Catch-all reverse proxy. Any HTTP method, any path, any body forwarded to upstream LLM provider. Auth: proxy token (`ptok_`). |

### SaaS Proxy
| Endpoint | Description |
|---|---|
| `* /v1/connections/{id}/proxy/*` | Proxy to upstream SaaS API via Nango. Any method/path/body/query. Auth: API key or JWT. |

---

## Sources

- [Maxim AI — Top 5 LLM Gateways in 2026](https://www.getmaxim.ai/articles/top-5-llm-gateways-in-2026-for-enterprise-grade-reliability-and-scale/)
- [Braintrust — Best LLM Monitoring Tools 2026](https://www.braintrust.dev/articles/best-llm-monitoring-tools-2026)
- [TrueFoundry — Best AI Observability Platforms 2026](https://www.truefoundry.com/blog/best-ai-observability-platforms-for-llms-in-2026)
