# Permissions — Zero-Trust Permissions for Every Agent

> Every agent gets exactly the permissions it needs. Nothing more.

---

## The Problem

The default mode for AI agents in production today is god-mode: agents get a full API key that grants access to everything the key can reach. When the agent only needs to send a message in one Slack channel, it has access to every channel, every workspace, every admin action. When that agent is compromised — through prompt injection, supply chain attack, or sandbox escape — the blast radius is the entire account.

This isn't a theoretical risk. It's the defining cybersecurity challenge of 2026.

### Industry Data

- **45.6%** of organizations use shared API keys for agent-to-agent authentication — giving every agent the same access as every other ([Gravitee State of AI Agent Security 2026](https://www.gravitee.io/blog/state-of-ai-agent-security-2026-report-when-adoption-outpaces-control))
- **Only 21.9%** treat AI agents as independent identity-bearing entities ([Gravitee 2026](https://www.gravitee.io/blog/state-of-ai-agent-security-2026-report-when-adoption-outpaces-control))
- Agent-involved breaches grew **340% YoY** between 2024-2025. **1 in 8** enterprise breaches now involve an agentic system; in financial services and healthcare, it's **1 in 5** ([SecurityWeek](https://www.securityweek.com/the-blast-radius-problem-stolen-credentials-are-weaponizing-agentic-ai/))
- **48%** of cybersecurity professionals identify agentic AI as the single most dangerous attack vector (Dark Reading poll via [BVP](https://www.bvp.com/atlas/securing-ai-agents-the-defining-cybersecurity-challenge-of-2026))
- **25.5%** of deployed agents can create and task other agents — cascading autonomy with no permission boundary ([Gravitee 2026](https://www.gravitee.io/blog/state-of-ai-agent-security-2026-report-when-adoption-outpaces-control))
- Multi-agent lateral movement: "if one agent is compromised, it can pass malicious instructions or escalated permissions to downstream agents" ([Strata](https://www.strata.io/blog/agentic-identity/a-guide-to-agentic-ai-risks-in-2026/))
- McKinsey red-team exercise: internal AI platform "Lilli" compromised — autonomous agent "gained broad system access in under two hours" ([BVP](https://www.bvp.com/atlas/securing-ai-agents-the-defining-cybersecurity-challenge-of-2026))
- **Only 37-40%** of organizations have true containment controls (purpose binding + kill-switch) despite 58-59% claiming oversight ([BVP](https://www.bvp.com/atlas/securing-ai-agents-the-defining-cybersecurity-challenge-of-2026))
- Bessemer's CISO guidance: "ensure every agent has a managed identity with scoped authentication — not a shared API key with god-mode access" ([BVP](https://www.bvp.com/atlas/securing-ai-agents-the-defining-cybersecurity-challenge-of-2026))
- NIST has launched an AI Agent Standards Initiative specifically to address agent permission and identity challenges ([Pillsbury Law](https://www.pillsburylaw.com/en/news-and-insights/nist-ai-agent-standards.html))

---

## The Solution

LLMVault Permissions enforces action-level, resource-level, time-limited scoping for every agent credential — validated against an embedded catalog of provider actions. Agents can only do what they're explicitly allowed to do, for as long as they're allowed to do it.

---

## How Scoping Works

### 1. Discover Available Scopes

Before minting a token, discover what's possible:

```bash
GET /v1/connections/available-scopes
```

Returns every active connection with its catalog actions and configured resources:

```json
[
  {
    "connection_id": "conn_abc123",
    "integration_id": "integ_def456",
    "provider": "slack",
    "display_name": "Slack - Support Team",
    "actions": [
      { "key": "send_message", "display_name": "Send Message", "resource_type": "channel" },
      { "key": "list_channels", "display_name": "List Channels", "access": "read" }
    ],
    "resources": {
      "channel": {
        "display_name": "Channels",
        "selected": [
          { "id": "C01234", "name": "#support" },
          { "id": "C05678", "name": "#general" }
        ]
      }
    }
  }
]
```

### 2. Mint a Scoped Token

Create a short-lived token that grants access to specific actions on specific connections with specific resources:

```bash
POST /v1/tokens
{
  "credential_id": "cred_abc123",
  "ttl": "1h",
  "scopes": {
    "scopes": [
      {
        "connection_id": "conn_abc123",
        "actions": ["send_message"],
        "resources": {
          "channel": ["C01234"]
        }
      }
    ]
  }
}
```

This token can:
- Send messages in #support via Slack
- Nothing else

It cannot list channels, cannot access #general, cannot call any other Slack API, cannot access any other connection. And it expires in 1 hour.

### 3. Catalog Validation

Every scope is validated at mint time against the embedded action catalog:

- **`catalog.ValidateActions(provider, actions)`** — Checks every action key exists for the provider. Wildcards (`*`) are explicitly rejected — you must name each action.
- **`catalog.ValidateResources(provider, actions, requestedResources, allowedResources)`** — Checks resource types match action resource_types, and requested resource IDs are within the connection's allowed set.
- **Connection verification** — Each connection_id must exist, belong to the org, and not be revoked. The integration must not be soft-deleted.

If any validation fails, the token is not minted. There is no way to create an overpermissioned token.

---

## Permission Layers

LLMVault enforces permissions at five layers:

### Layer 1: Token Scopes (Action + Resource Level)
Per-action, per-resource, per-connection scoping on every minted token. Validated against the catalog.

### Layer 2: Token TTL (Time Level)
Tokens auto-expire. Configurable from seconds to 24 hours. A 1-hour agent session gets a 1-hour token.

### Layer 3: Token Revocation (Kill Switch)
`DELETE /v1/tokens/{jti}` — instant revocation propagated via Redis pub/sub. Sub-millisecond across all instances.

### Layer 4: Credential Rate Limits (Request Level)
Built into the credential itself:
- `remaining` — total requests allowed
- `refill_amount` + `refill_interval` — automatic refill (e.g., 100 requests per hour)

Enforced by `RemainingCheck` middleware on every proxy request.

### Layer 5: Identity Rate Limits (User Level)
Per-identity rate limits configured as an array of `{name, limit, duration_ms}` rules. Enforced by `IdentityRateLimit` middleware. An end-user's agents can't burn through the org's quota.

---

## Full API Surface

### Token Management
| Endpoint | Description |
|---|---|
| `POST /v1/tokens` | Mint proxy token. Accepts: credential_id, ttl, scopes (connection_id + actions + resources), remaining, refill_amount, refill_interval, meta. Returns: `ptok_` JWT, expires_at, jti, mcp_endpoint (if scopes present). |
| `GET /v1/tokens` | List tokens (cursor paginated, filterable by credential_id). Shows scopes, remaining, revoked_at, expires_at. |
| `DELETE /v1/tokens/{jti}` | Revoke token instantly. Propagates via Redis pub/sub. |

### Scope Discovery
| Endpoint | Description |
|---|---|
| `GET /v1/connections/available-scopes` | List all active connections with their catalog actions and configured resources. |
| `GET /v1/catalog/integrations` | Browse all integration providers with action/resource counts. |
| `GET /v1/catalog/integrations/{id}` | Get provider detail with all actions and resources. |
| `GET /v1/catalog/integrations/{id}/actions` | List actions (supports `?access=read|write` filter). |

### Identity & Rate Limiting
| Endpoint | Description |
|---|---|
| `POST /v1/identities` | Create identity with external_id and rate limits array. |
| `PUT /v1/identities/{id}` | Update identity rate limits and metadata. |

---

## The Catalog

LLMVault ships with embedded action catalogs for 45+ providers, compiled at build time via `go:embed`. Each catalog defines:

- **Actions** — key, display_name, description, access level (read/write), resource_type, JSON Schema parameters, execution config (HTTP method, path, body/query mapping, headers, response_path)
- **Resources** — resource types with display_name, id_field, name_field, list_action, and optional request_config for discovery
- **Execution configs** — how to call the provider API through the Nango proxy

Providers with catalogs include: Slack, GitHub (4 auth variants), Stripe (5 variants), HubSpot, Salesforce, Jira (5 variants), Notion, Confluence (3 variants), Zoom, Linear, Shopify, Discord, Figma, Zendesk, PagerDuty, Intercom, Box, Sentry, Asana, GitLab, Twilio, Cloudflare, Vercel, Braintree, and more.

---

## Sources

- [Gravitee — State of AI Agent Security 2026](https://www.gravitee.io/blog/state-of-ai-agent-security-2026-report-when-adoption-outpaces-control)
- [BVP — Securing AI Agents: The Defining Cybersecurity Challenge of 2026](https://www.bvp.com/atlas/securing-ai-agents-the-defining-cybersecurity-challenge-of-2026)
- [SecurityWeek — The Blast Radius Problem](https://www.securityweek.com/the-blast-radius-problem-stolen-credentials-are-weaponizing-agentic-ai/)
- [Strata — A Guide to Agentic AI Risks in 2026](https://www.strata.io/blog/agentic-identity/a-guide-to-agentic-ai-risks-in-2026/)
- [Pillsbury Law — NIST AI Agent Standards Initiative](https://www.pillsburylaw.com/en/news-and-insights/nist-ai-agent-standards.html)
