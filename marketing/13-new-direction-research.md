# ZiraLoop — New Direction Research

## Rebrand: "The Secure Access Layer for Your AI Agents"

---

## Competitive Context: One (withone.ai)

One (withone.ai) positions as "Command center for your AI workforce" — agent infrastructure that connects AI agents to 250+ SaaS apps with managed authentication, 47,500+ tools, workflow runtime, and agent deployment.

**One's core products:**
- **AuthKit** — Plaid-like embeddable connect flow, OAuth 2.0/1.0a/API key support, automatic token refresh, tenant isolation
- **One Flow** — Workflow runtime with 12 step types, branching, loops, parallel execution
- **One Agent** — Deploy agents to Slack, Telegram, WhatsApp with cron/webhook triggers
- **One Skills** — 47,500+ vetted integration tools
- **Bridge** — Convert API docs to MCP servers
- **CLI** — `npm i -g @withone/cli` for discovering and executing actions

**One's pricing:** Free ($0, 1M API calls) / Starter ($29/mo) / Pro ($199/mo) / Enterprise (custom)

**Key stats:** 47,856 tools, 255 platforms, 17,000+ developers, 99.9% uptime, <100ms p95 latency

**Where ZiraLoop is stronger:**
1. Security depth — envelope encryption, AES-256-GCM, KMS wrapping, sealed memory, 3-tier cache, instant revocation. One claims "end-to-end encryption" with no architecture detail.
2. Scoping granularity — per-action, per-connection, per-resource scoping validated against an embedded catalog. One offers "tenant isolation with scoped credentials."
3. LLM-native features — short-lived proxy tokens, auth scheme abstraction across LLM providers, streaming proxy with sub-5ms overhead, token usage capture (input/output/cached/reasoning). One doesn't do this.
4. Connect widget completeness — handles both API key input (LLM providers) AND OAuth flows (SaaS integrations) AND resource selection.

**Where One is stronger:**
1. Catalog scale — 47,500 tools vs. ~45 provider catalogs (but ours have execution configs and resource definitions)
2. Agent runtime — deploy agents to Slack/Telegram/WhatsApp, cron, webhooks, event triggers
3. Workflow engine — branching, loops, parallel execution
4. CLI developer experience
5. Marketing maturity

---

## Three Core Problems ZiraLoop Solves

### Problem 1: Agents Run with God-Mode Credentials

**Industry data:**

- **45.6%** of organizations use shared API keys for agent authentication — giving every agent the same access as every other agent (Gravitee State of AI Agent Security 2026)
- **Only 21.9%** of organizations treat agents as independent identity-bearing entities (Gravitee 2026)
- Agent-involved breaches grew **340% YoY** between 2024-2025, with **1 in 8** enterprise breaches involving an agentic system (SecurityWeek)
- A compromised agent credential gives attackers access "equivalent to that agent's permissions for weeks or months" (Beam AI)
- **48%** of cybersecurity professionals identify agentic AI as the single most dangerous attack vector (Dark Reading)
- Bessemer's CISO guidance: "ensure every agent has a managed identity with scoped authentication — not a shared API key with god-mode access" (BVP)
- McKinsey red-team exercise: their internal AI platform "Lilli" was compromised by an autonomous agent that "gained broad system access in under two hours" (BVP)
- **25.5%** of deployed agents can create and task other agents — cascading autonomy with no permission boundary (Gravitee 2026)
- Multi-agent lateral movement: "if one agent in a pipeline is compromised, it can pass malicious instructions or escalated permissions to downstream agents" (Strata)
- **Only 37-40%** of organizations have true containment controls (purpose binding and kill-switch capability), despite 58-59% claiming monitoring/oversight (BVP)

**The pattern:** Developers give agents a full API key. The agent only needs to send messages in one Slack channel, but it has access to every channel, every workspace, every admin action. When that agent is compromised — through prompt injection, supply chain attack, or sandbox escape — the blast radius is everything.

**How ZiraLoop solves this (existing API surface):**

| API Endpoint / Feature | What It Does |
|---|---|
| `POST /v1/tokens` with `scopes` | Mint a short-lived JWT scoped to specific connections, specific actions, and specific resources. An agent token can be limited to `slack.send_message` on channel `#support` only — nothing else. |
| `TokenScope` validation (`internal/mcp/scope.go`) | Every scope is validated against the embedded action catalog at mint time. You can't grant an action that doesn't exist in the catalog. |
| `catalog.ValidateResources()` | Requested resources are checked against the connection's allowed resource set. An agent can't access a GitHub repo it wasn't granted. |
| `POST /v1/tokens` with `ttl` | Tokens auto-expire. A 1-hour agent session gets a 1-hour token. No lingering credentials. |
| `DELETE /v1/tokens/{jti}` | Instant revocation propagated via Redis pub/sub across all proxy instances in sub-millisecond time. |
| `POST /v1/credentials` with `remaining` + `refill_amount` + `refill_interval` | Rate limiting built into the credential itself. Cap an agent at 100 requests/hour at the infrastructure level. |
| `POST /v1/identities` with `ratelimits` | Per-identity rate limits with configurable name, limit, and duration_ms — an end-user's agent can't burn through the org's quota. |
| Identity-scoped credentials (`identity_id` on credentials) | Each end-user's credentials are isolated. Agent A can't see Agent B's keys even within the same org. |
| `GET /v1/connections/available-scopes` | Discover exactly what actions and resources each connection permits — before minting a token. Returns connection, provider, actions (key, display_name, description, resource_type), and configured resources. |
| MCP server (`/{jti}/*` on separate port) | Each token gets its own MCP server instance via `mcpserver.BuildServer()`, exposing only the tools that token's scopes allow. An agent connecting via MCP literally cannot see tools it doesn't have permission to use. Supports both Streamable HTTP and legacy SSE transports. |
| `ValidateJTIMatch` middleware | URL JTI must match JWT JTI — prevents token reuse across MCP endpoints. |
| `ValidateHasScopes` middleware | Token must have scopes to access MCP — prevents unscoped tokens from getting tool access. |

**Headline:** "Every agent gets exactly the permissions it needs. Nothing more. Enforced at the infrastructure level, not the application level."

---

### Problem 2: Credentials Are Scattered, Exposed, and Unmanaged

**Industry data:**

- **1.27 million AI service secrets** leaked on public GitHub in 2025 — an **81% YoY surge** (GitGuardian State of Secrets Sprawl 2026)
- LLM infrastructure secrets (orchestration, RAG, vector stores) leaked **5x faster** than core model provider keys (GitGuardian 2026)
- **24,008 secrets** exposed in MCP configuration files alone, with **2,117 confirmed valid** (GitGuardian 2026)
- **64% of valid secrets from 2022 are still active** in 2026 — credentials never get rotated (GitGuardian 2026)
- Claude Code-assisted commits show a **3.2% secret-leak rate** vs. 1.5% baseline (GitGuardian/OECD)
- Shadow AI breaches cost **$670,000 more** than standard incidents — averaging **$4.63M** per breach (IBM Cost of a Data Breach 2025)
- **28% of incidents** originate entirely outside repositories — in Slack, Jira, Confluence (GitGuardian 2026)
- Secrets in collaboration tools are **13 percentage points more likely** to be categorized as critical (GitGuardian 2026)
- **28.65 million** hardcoded secrets added to public GitHub in 2025, a **34% increase** YoY — the largest single-year jump recorded (GitGuardian 2026)
- Internal repositories are **6x more likely** to contain hardcoded secrets than public ones (GitGuardian 2026)
- Standard secret managers protect storage but not exposure: "keys are in the agent's memory, in its environment, in its HTTP headers, and in its logs" (Dev.to)

**The pattern:** Agents need access to LLM providers AND SaaS apps. The keys end up in env vars, config files, .env files, MCP configs, log output, and chat threads. Each is a point of exposure. When a dev tool, a sandbox, or a CI runner is compromised, those keys give attackers access to everything. And they stay valid for years because nobody rotates them.

**How ZiraLoop solves this (existing API surface):**

| API Endpoint / Feature | What It Does |
|---|---|
| `POST /v1/credentials` | Store LLM API keys with envelope encryption (AES-256-GCM + KMS-wrapped DEK). The plaintext key is zeroed from memory immediately after encryption. Supports 101 providers, auto-detects provider from base_url. |
| `POST /v1/integrations` with Nango | Store OAuth credentials (client_id, client_secret) in Nango as source of truth. Supports OAuth 2.0, OAuth 1.0a, API keys, Basic Auth, APP, MCP_OAUTH2, MCP_OAUTH2_GENERIC, INSTALL_PLUGIN, CUSTOM, TBA, OAUTH2_CC, JWT, and more. |
| `/v1/proxy/*` (LLM proxy) | Streaming reverse proxy. Resolves encrypted key from 3-tier cache, attaches correct auth header (Bearer, x-api-key, query_param, api-key per `knownAuthSchemes`), streams response via `httputil.ReverseProxy` with `FlushInterval: -1`. Sub-5ms overhead. **The agent never sees the real key.** |
| `/v1/connections/{id}/proxy/*` (SaaS proxy) | Same proxy pattern for SaaS apps via `nango.RawProxyRequest()`. Any HTTP method, any path, any body/query forwarded to upstream with managed OAuth tokens and automatic token refresh. |
| 3-tier cache (memguard-sealed -> Redis -> Postgres/KMS) | Hot-path credential resolution in <5ms. Even if Redis is compromised, attacker gets sealed memory blobs — not usable credentials. |
| `DELETE /v1/credentials/{id}` and `DELETE /v1/connections/{id}` | Instant revocation. Redis pub/sub propagates across all proxy instances in sub-millisecond time. No stale credential window. |
| `CaptureTransport` (`internal/proxy/capture.go`) | Wraps the proxy's HTTP transport to capture response metadata (usage tokens, TTFB, status, errors) without adding latency. Works for both streaming SSE and non-streaming responses. |
| Usage parsing (OpenAI, Anthropic, Google) | Normalizes token usage across providers: `input_tokens`, `output_tokens`, `cached_tokens`, `reasoning_tokens`. Parses both streaming chunks and complete responses. |
| `POST /v1/widget/connections/{id}/verify` | Validates stored API keys against the provider (decrypts, tests, re-zeros). Detects revoked/expired keys proactively. |
| `GET /v1/audit` | Full audit trail — action, method, path, status, latency_ms, credential_id, identity_id, ip_address. Every credential lifecycle event logged. |
| `GET /v1/generations` | Every LLM request logged: model, provider_id, input/output/cached/reasoning tokens, cost, ttfb_ms, total_ms, upstream_status, user_id, tags, error_type, error_message, ip_address. Filterable by model, provider, credential, user, tags, error_type. |
| `GET /v1/reporting` | Aggregated analytics with flexible grouping (model, provider, credential, user, identity) and time granularity (hour/day). Includes request_count, token sums, total_cost, avg/p50/p95 TTFB latency, error_count. Date range filtering, dimension filtering, tag filtering. |
| `GET /v1/usage` | Org-level dashboard: credential stats, token stats, api_key stats, identity count, request breakdowns (total, today, yesterday, 7d, 30d), daily_requests, top_credentials, spend_over_time, token_volumes, latency stats, top_models, top_users, error_rates. |

**Headline:** "One vault for every credential your agents need. LLM keys and SaaS tokens. Encrypted, proxied, never exposed to application code."

---

### Problem 3: Connecting Agents to Apps Is a Fragmented, Insecure Mess

**Industry data:**

- **88% of organizations** reported confirmed or suspected AI agent security incidents in the past year (Gravitee 2026)
- **Only 14.4%** of agents went live with full security/IT approval — the rest were deployed ad hoc (Gravitee 2026)
- **More than 50%** of agents run without any security oversight or logging (Gravitee 2026)
- **82% of executives** feel confident existing policies protect them — contradicting technical reality (Gravitee 2026)
- The Drift OAuth supply chain attack compromised **700+ organizations** through a single stolen integration token (Reco AI 2025 Year in Review)
- The OpenClaw malicious skills crisis: **1,184 malicious skills** confirmed — the largest supply chain attack targeting AI agent infrastructure to date (Antiy CERT)
- The OpenAI plugin ecosystem attack: compromised agent credentials harvested from **47 enterprise deployments**, access maintained for six months before discovery (Reco AI)
- **Only 47.1%** of agents are actively monitored or secured on average (Gravitee 2026)
- **27.2%** of organizations have reverted to custom, hardcoded authorization logic for agents (Gravitee 2026)
- NIST has launched an AI Agent Standards Initiative to address permission and identity challenges (Pillsbury Law)
- Gartner projects **40% of enterprise apps** will embed task-specific AI agents by 2026, up from <5% in 2025 (Gartner)

**The pattern:** Platforms need agents to connect to Slack, GitHub, HubSpot, Stripe, and dozens of other services. Each has a different auth flow (OAuth 2.0, API keys, webhook secrets, app installations). Developers build custom integration code, hardcode credentials, skip token refresh, and create ungoverned access paths. The result: a sprawling, fragmented, unauditable web of agent-to-app connections with no central point of control.

**How ZiraLoop solves this (existing API surface):**

| API Endpoint / Feature | What It Does |
|---|---|
| `POST /v1/integrations` | Create a managed integration for any of 250+ Nango providers. Validates credentials against provider auth_mode, pushes to Nango (source of truth for OAuth), stores reference record locally with nango_config (logo, callback_url, auth_mode, docs, setup_guide_url, credentials_schema). |
| `GET /v1/integrations/providers` | Discover all available providers with auth_mode and webhook configuration requirements. |
| `validateCredentials()` | Validates credential structure against provider auth mode: OAuth 2.0/1.0a require client_id + client_secret, APP requires app_id + app_link + private_key, MCP_OAUTH2 static requires client_id + client_secret, API_KEY/BASIC/NONE reject credentials. |
| Connect Widget (`apps/connect/`) | Full React embeddable widget: ProviderSelection, ProviderDetail, ApiKeyInput + Validating, IntegrationProviderSelection, IntegrationDetail, IntegrationAuth (OAuth flow), IntegrationResourceSelection, IntegrationConnect, ConnectedList, ConnectionCard, RevokeConfirm, IntegrationDisconnectConfirm, Success/IntegrationSuccess, ResourceSelectionSuccess. Themed via ThemeContext, communicates via ParentEventContext. |
| `POST /v1/connect/sessions` | Create scoped widget sessions: `allowed_integrations` (restrict which integrations are shown), `permissions` (create/list/delete/verify), `allowed_origins` (CORS enforcement, validated as subset of org-level origins), `identity_id` or `external_id` (auto-upserts identity), configurable TTL (max 30min, default 15min). |
| `POST /v1/integrations/{id}/connections` | Register authenticated connections from Nango's OAuth flow. Each connection is tied to an identity and an integration. Meta includes resource selections. |
| 45+ provider action catalogs (`internal/mcp/catalog/providers/*.actions.json`) | Embedded action definitions for: Slack, GitHub (4 auth variants), Stripe (5 variants), HubSpot, Salesforce, Jira (5 variants), Notion, Confluence (3 variants), Zoom, Linear, Shopify, Discord, Figma, Zendesk, PagerDuty, Intercom, Box, Sentry, Asana, GitLab, Twilio, Cloudflare, Vercel, Braintree. Each defines: actions with execution configs (method, path, body_mapping, query_mapping, headers, response_path), resource types with discovery (display_name, id_field, name_field, list_action, request_config), and access levels (read/write). |
| `GET /v1/catalog/integrations` | Public discovery API for browsing all integration providers with action counts and resource counts. |
| `GET /v1/catalog/integrations/{id}` | Detailed integration info with all resources and actions listed. |
| `GET /v1/catalog/integrations/{id}/actions` | List actions for a provider, supports `?access=read|write` filter. |
| `GET /v1/widget/integrations/{id}/resources/{type}/available` | Resource discovery — list specific Slack channels, GitHub repos, Jira projects available on a connection via `resources.Discovery`. Uses `ResourceDef.ListAction` to call the provider API through Nango proxy, with configurable `RequestConfig` (custom method, headers, query_params, body_template, response_path). |
| MCP server with scoped tools | `mcpserver.BuildServer()` dynamically builds an MCP server from token scopes. Each tool maps to a catalog action on a specific connection. Execution goes through Nango proxy with managed auth. Server is cached per JTI with auto-expiry matching token TTL. |
| `/v1/connections/{id}/proxy/*` | Direct proxy to any provider API through Nango's `RawProxyRequest()`. Any HTTP method, path, query, and body forwarded. Managed auth headers and token refresh handled automatically. Content-Type passthrough, status code passthrough. |
| `buildConnectionProviderConfig()` | Extracts credentials, connection_config, metadata, and provider from Nango — strips sensitive internal fields (jwtToken). Returned on `GET /v1/connections/{id}`. |

**Headline:** "Connect agents to 250+ apps through one API. OAuth, API keys, webhooks — managed, scoped, and auditable. Drop in the widget and ship in minutes."

---

## How the Three Problems Frame the Product

```
+------------------------------------------------------------------+
|          THE SECURE ACCESS LAYER FOR YOUR AI AGENTS              |
+------------------+------------------+----------------------------+
|   PROBLEM 1      |   PROBLEM 2      |   PROBLEM 3               |
|   God-Mode       |   Scattered      |   Fragmented              |
|   Credentials    |   Secrets        |   Connections             |
+------------------+------------------+----------------------------+
|   SOLUTION:      |   SOLUTION:      |   SOLUTION:               |
|   Scoped         |   One Vault      |   One Connect             |
|   Permissions    |   Zero Exposure  |   Layer                   |
+------------------+------------------+----------------------------+
| - Token scopes   | - Envelope       | - 250+ OAuth providers    |
| - Action-level   |   encryption     | - Embeddable widget       |
|   control        | - Streaming      | - 45+ action catalogs     |
| - Resource-level |   proxy (LLM +   | - Resource selection      |
|   control        |   SaaS)          | - MCP server per token    |
| - TTL + instant  | - 3-tier cache   | - Managed auth +          |
|   revocation     | - Audit trail    |   token refresh           |
| - Per-identity   | - Generation     | - Scoped widget           |
|   rate limits    |   logging        |   sessions                |
| - MCP tool       | - Cost tracking  | - Public discovery APIs   |
|   filtering      | - Key validation |                           |
+------------------+------------------+----------------------------+
```

---

## Complete API Surface Mapped to Problems

### Health & Discovery (No Auth)
- `GET /healthz` — Health check
- `GET /readyz` — Readiness check (Postgres + Redis)
- `GET /v1/providers` — List all 101 LLM providers (public)
- `GET /v1/providers/{id}` — Get provider detail with models
- `GET /v1/providers/{id}/models` — List models for a provider
- `GET /v1/catalog/integrations` — List all MCP integration providers (public)
- `GET /v1/catalog/integrations/{id}` — Get integration detail with actions
- `GET /v1/catalog/integrations/{id}/actions` — List actions (supports ?access=read|write)

### Organization Management (JWT / API Key)
- `POST /v1/orgs` — Create organization (JWT only)
- `GET /v1/orgs/current` — Get current org

### Credentials — Maps to Problem 2 (Scope: `credentials`)
- `POST /v1/credentials` — Create encrypted credential (envelope encryption, auto-detect provider)
- `GET /v1/credentials` — List credentials (cursor paginated, filterable by identity_id, external_id, meta)
- `GET /v1/credentials/{id}` — Get credential
- `DELETE /v1/credentials/{id}` — Revoke credential (soft-delete, instant propagation)

### Tokens — Maps to Problem 1 (Scope: `tokens`)
- `POST /v1/tokens` — Mint proxy token (ttl, scopes, remaining, refill_amount, refill_interval, meta)
- `GET /v1/tokens` — List tokens (cursor paginated, filterable by credential_id)
- `DELETE /v1/tokens/{jti}` — Revoke token (instant propagation)

### Identities — Maps to Problem 1 (Scope: `all`)
- `POST /v1/identities` — Create identity (external_id, meta, ratelimits)
- `GET /v1/identities` — List identities (cursor paginated)
- `GET /v1/identities/{id}` — Get identity
- `PUT /v1/identities/{id}` — Update identity (ratelimits, meta)
- `DELETE /v1/identities/{id}` — Delete identity

### API Keys (Scope: `all`)
- `POST /v1/api-keys` — Create API key (name, scopes: credentials|tokens|integrations|connect|all, optional expiry)
- `GET /v1/api-keys` — List API keys
- `DELETE /v1/api-keys/{id}` — Revoke API key

### Integrations — Maps to Problem 3 (Scope: `integrations`)
- `POST /v1/integrations` — Create integration (provider, display_name, credentials, meta)
- `GET /v1/integrations` — List integrations (filterable by provider, meta)
- `GET /v1/integrations/{id}` — Get integration (includes live nango_config)
- `PUT /v1/integrations/{id}` — Update integration (display_name, credentials, meta)
- `DELETE /v1/integrations/{id}` — Soft-delete integration
- `GET /v1/integrations/providers` — List available Nango providers

### Connections — Maps to Problem 3 (Scope: `integrations`)
- `POST /v1/integrations/{id}/connections` — Create connection (nango_connection_id, identity_id, meta)
- `GET /v1/integrations/{id}/connections` — List connections for integration
- `GET /v1/connections/{id}` — Get connection (includes provider_config from Nango)
- `DELETE /v1/connections/{id}` — Revoke connection
- `GET /v1/connections/available-scopes` — List active connections with catalog actions and resources
- `/v1/connections/{id}/proxy/*` — Proxy to upstream provider API via Nango

### Connect Sessions — Maps to Problem 3 (Scope: `connect`)
- `POST /v1/connect/sessions` — Create widget session (allowed_integrations, permissions, allowed_origins, identity, ttl)

### Connect Widget API — Maps to Problem 3 (Session Token Auth)
- `GET /v1/widget/session` — Get session info
- `GET /v1/widget/providers` — List LLM providers
- `GET /v1/widget/connections` — List credential connections for identity
- `POST /v1/widget/connections` — Create credential via widget (provider_id, api_key, validates key)
- `DELETE /v1/widget/connections/{id}` — Delete credential connection
- `POST /v1/widget/connections/{id}/verify` — Verify credential against provider
- `GET /v1/widget/integrations/providers` — List Nango providers
- `GET /v1/widget/integrations` — List integrations for widget
- `POST /v1/widget/integrations/{id}/connect-session` — Create Nango connect session (OAuth flow)
- `GET /v1/widget/integrations/{id}/resources/{type}/available` — Discover available resources
- `POST /v1/widget/integrations/{id}/connections` — Create integration connection
- `PATCH /v1/widget/integrations/{id}/connections/{connectionId}` — Update connection
- `DELETE /v1/widget/integrations/{id}/connections/{connectionId}` — Delete integration connection

### Settings (Scope: `all`)
- `GET /v1/settings/connect` — Get Connect widget settings (allowed_origins)
- `PUT /v1/settings/connect` — Update Connect settings

### Audit — Maps to Problem 2 (Any Auth)
- `GET /v1/audit` — List audit log entries (filterable by action, cursor paginated)

### Usage & Analytics — Maps to Problem 2 (Any Auth)
- `GET /v1/usage` — Org usage summary (credentials, tokens, api_keys, identities, requests, daily_requests, top_credentials, spend_over_time, token_volumes, latency, top_models, top_users, error_rates)
- `GET /v1/generations` — List generation records (filterable by model, provider_id, credential_id, user_id, tags, error_type)
- `GET /v1/generations/{id}` — Get generation detail
- `GET /v1/reporting` — Analytics report (group by model/provider/credential/user/identity, date_part hour/day, date range, p50/p95 TTFB)

### LLM Proxy — Maps to Problem 2 (Proxy Token Auth)
- `* /v1/proxy/*` — Catch-all reverse proxy (any method/path/body to upstream LLM provider). Middleware: TokenAuth, IdentityRateLimit, RemainingCheck, Audit, Generation capture.

### MCP Server — Maps to Problem 1 (Proxy Token Auth, Separate Port)
- `/{jti}/*` — MCP protocol endpoint (Streamable HTTP + SSE). Middleware: TokenAuth, ValidateJTIMatch, ValidateHasScopes. Dynamically builds scoped MCP server per token with tools from action catalog.

---

## Sources

- [Bessemer — Securing AI Agents: The Defining Cybersecurity Challenge of 2026](https://www.bvp.com/atlas/securing-ai-agents-the-defining-cybersecurity-challenge-of-2026)
- [Gravitee — State of AI Agent Security 2026 Report](https://www.gravitee.io/blog/state-of-ai-agent-security-2026-report-when-adoption-outpaces-control)
- [GitGuardian — State of Secrets Sprawl 2026](https://securityboulevard.com/2026/03/the-state-of-secrets-sprawl-2026-ai-service-leaks-surge-81-and-29m-secrets-hit-public-github/)
- [SecurityWeek — The Blast Radius Problem: Stolen Credentials Are Weaponizing Agentic AI](https://www.securityweek.com/the-blast-radius-problem-stolen-credentials-are-weaponizing-agentic-ai/)
- [Beam AI — AI Agent Security in 2026: Enterprise Risks & Best Practices](https://beam.ai/agentic-insights/ai-agent-security-in-2026-the-risks-most-enterprises-still-ignore)
- [Strata — A Guide to Agentic AI Risks in 2026](https://www.strata.io/blog/agentic-identity/a-guide-to-agentic-ai-risks-in-2026/)
- [Dev.to — Why Your AI Agent's API Keys Are a Ticking Time Bomb](https://dev.to/jonathanfishner/why-your-ai-agents-api-keys-are-a-ticking-time-bomb-12pm)
- [Reco AI — AI & Cloud Security Breaches: 2025 Year in Review](https://www.reco.ai/blog/ai-and-cloud-security-breaches-2025)
- [OECD AI — AI Coding Assistants Drive Surge in Secret Leaks on GitHub](https://oecd.ai/en/incidents/2026-03-17-2273)
- [CyberArk — AI Agents and Identity Risks: How Security Will Shift in 2026](https://www.cyberark.com/resources/blog/ai-agents-and-identity-risks-how-security-will-shift-in-2026)
- [Pillsbury Law — NIST Launches AI Agent Standards Initiative](https://www.pillsburylaw.com/en/news-and-insights/nist-ai-agent-standards.html)
- [One (withone.ai)](https://www.withone.ai/)
