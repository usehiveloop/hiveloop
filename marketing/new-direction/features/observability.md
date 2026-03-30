# Observability — See Everything Your Agents Do

> Every request. Every token. Every dollar. Tracked automatically.

---

## The Problem

Most AI agent platforms operate as black boxes. Agents make LLM calls, but nobody knows which model was used, how many tokens were consumed, what it cost, how long it took, or whether errors are spiking. When the monthly API bill arrives, teams can't explain why it tripled. When an agent starts failing, there's no trace to debug.

This is not an edge case. It's the norm.

### Industry Data

- AI-driven LLM observability has evolved from "a niche debugging utility into a mandatory infrastructure layer for modern MLOps pipelines" ([Energent AI](https://www.energent.ai/energent/compare/en/ai-driven-llm-observability))
- Teams with monitoring at launch spend **38.7% less** on LLM API costs in month one ([Braintrust](https://www.braintrust.dev/articles/best-llm-monitoring-tools-2026))
- **5% of requests consume 50% of tokens** — granular per-request tracking is essential for identifying cost drivers ([TrueFoundry](https://www.truefoundry.com/blog/best-ai-observability-platforms-for-llms-in-2026))
- Enterprise requirements: "distributed tracing with span-level visibility, real-time metrics covering latency distributions, error rates, token usage, cache hit rates, and cost analytics, governance and audit trails with complete records of who used which model, with what data, and when" ([Maxim AI](https://www.getmaxim.ai/articles/top-enterprise-ai-gateways-for-llm-observability-in-2026/))
- **More than 50%** of AI agents run without any security oversight or logging ([Gravitee 2026](https://www.gravitee.io/blog/state-of-ai-agent-security-2026-report-when-adoption-outpaces-control))
- **Only 47.1%** of agents are actively monitored or secured ([Gravitee 2026](https://www.gravitee.io/blog/state-of-ai-agent-security-2026-report-when-adoption-outpaces-control))
- **82% of executives** feel confident existing policies protect them — while technical teams report extensive gaps in observability ([Gravitee 2026](https://www.gravitee.io/blog/state-of-ai-agent-security-2026-report-when-adoption-outpaces-control))

---

## The Solution

LLMVault Observability automatically tracks every request flowing through the proxy — token usage, cost, latency, errors, model, provider, user, and more. No SDK instrumentation required. Because the proxy sees every request, observability is a free byproduct of the architecture.

---

## Four Observability Layers

### 1. Generations — Per-Request Detail

Every LLM request through the proxy creates a generation record with full metadata:

```bash
GET /v1/generations?model=claude-sonnet-4-6&user_id=user_42
```

Each generation captures:

| Field | Description |
|---|---|
| `id` | Unique generation ID |
| `credential_id` | Which credential was used |
| `identity_id` | Which end-user's identity |
| `token_jti` | Which proxy token (links to scopes) |
| `provider_id` | Detected provider (openai, anthropic, google, etc.) |
| `model` | Model name extracted from request body |
| `request_path` | Upstream API path called |
| `is_streaming` | Whether response was SSE streaming |
| `input_tokens` | Prompt/input token count |
| `output_tokens` | Completion/output token count |
| `cached_tokens` | Cache-hit tokens (prompt caching) |
| `reasoning_tokens` | Chain-of-thought tokens (o-series models) |
| `cost` | Estimated cost in USD |
| `ttfb_ms` | Time to first byte (streaming latency) |
| `total_ms` | Total request duration |
| `upstream_status` | HTTP status from provider |
| `user_id` | Custom user identifier (passed via headers) |
| `tags` | Custom string tags for grouping |
| `error_type` | Classified: timeout, rate_limit, auth, upstream_error, client_error |
| `error_message` | Error detail (first 512 bytes) |
| `ip_address` | Client IP address |
| `created_at` | Timestamp |

**Filterable by:** model, provider_id, credential_id, user_id, tags (array containment), error_type.

### 2. Reporting — Aggregated Analytics

Flexible analytics with multi-dimensional grouping:

```bash
GET /v1/reporting?group_by=model,user&date_part=day&start_date=2026-03-01&end_date=2026-03-28
```

Returns aggregated rows with:

| Metric | Description |
|---|---|
| `period` | Time bucket (hour or day) |
| `request_count` | Total requests in period |
| `input_tokens` | Sum of input tokens |
| `output_tokens` | Sum of output tokens |
| `cached_tokens` | Sum of cached tokens |
| `reasoning_tokens` | Sum of reasoning tokens |
| `total_cost` | Sum of estimated cost |
| `avg_ttfb_ms` | Average time to first byte |
| `p50_ttfb_ms` | Median TTFB (50th percentile) |
| `p95_ttfb_ms` | 95th percentile TTFB |
| `error_count` | Requests with errors |

**Group by:** model, provider, credential, user, identity (comma-separated for multi-dimensional).
**Date granularity:** hour or day.
**Filters:** model, provider_id, credential_id, user_id, tags.

### 3. Usage Dashboard — Org Overview

High-level dashboard data for the entire organization:

```bash
GET /v1/usage
```

Returns a comprehensive snapshot:

| Section | Metrics |
|---|---|
| **Credentials** | total, active, revoked |
| **Tokens** | total, active, expired, revoked |
| **API Keys** | total, active, revoked |
| **Identities** | total count |
| **Requests** | total, today, yesterday, last 7 days, last 30 days |
| **Daily Requests** | Time series array |
| **Top Credentials** | Highest-usage credentials |
| **Spend Over Time** | Cost time series |
| **Token Volumes** | Input/output token time series |
| **Latency** | Average, p50, p95, p99 |
| **Top Models** | Most-used models by request count |
| **Top Users** | Most active users/identities |
| **Error Rates** | Error counts by type over time |

### 4. Audit Trail — Compliance Record

Every API operation logged:

```bash
GET /v1/audit?action=proxy.request
```

Each audit entry includes:

| Field | Description |
|---|---|
| `action` | Operation type (credential.create, token.mint, proxy.request, etc.) |
| `method` | HTTP method |
| `path` | Request path |
| `status` | Response status code |
| `latency_ms` | Operation duration |
| `credential_id` | Associated credential (if applicable) |
| `identity_id` | Associated identity |
| `ip_address` | Client IP |
| `created_at` | Timestamp |

---

## Full API Surface

### Generations
| Endpoint | Description |
|---|---|
| `GET /v1/generations` | List generation records. Cursor paginated. Filterable by model, provider_id, credential_id, user_id, tags, error_type. |
| `GET /v1/generations/{id}` | Get single generation detail. |

### Reporting
| Endpoint | Description |
|---|---|
| `GET /v1/reporting` | Aggregated analytics. Params: group_by (model, provider, credential, user, identity), date_part (hour, day), start_date, end_date, model, provider_id, credential_id, user_id, tags. Returns up to 1,000 rows. |

### Usage
| Endpoint | Description |
|---|---|
| `GET /v1/usage` | Org-level usage dashboard. Full snapshot of credentials, tokens, api_keys, identities, requests, daily_requests, top_credentials, spend_over_time, token_volumes, latency, top_models, top_users, error_rates. |

### Audit
| Endpoint | Description |
|---|---|
| `GET /v1/audit` | Audit log entries. Cursor paginated. Filterable by action. |

---

## Sources

- [Energent AI — AI-Driven LLM Observability: 2026 Market Report](https://www.energent.ai/energent/compare/en/ai-driven-llm-observability)
- [Braintrust — Best LLM Monitoring Tools 2026](https://www.braintrust.dev/articles/best-llm-monitoring-tools-2026)
- [TrueFoundry — Best AI Observability Platforms 2026](https://www.truefoundry.com/blog/best-ai-observability-platforms-for-llms-in-2026)
- [Maxim AI — Top Enterprise AI Gateways for LLM Observability](https://www.getmaxim.ai/articles/top-enterprise-ai-gateways-for-llm-observability-in-2026/)
- [Gravitee — State of AI Agent Security 2026](https://www.gravitee.io/blog/state-of-ai-agent-security-2026-report-when-adoption-outpaces-control)
