# MCP — A Scoped MCP Server for Every Agent

> Your agents see only the tools they're allowed to use.

---

## The Problem

The Model Context Protocol (MCP) has become the standard for connecting AI models to external tools and data sources. But MCP's rapid adoption has outpaced its security model. Most MCP servers expose all available tools to every connected client — with no permission boundaries, no credential isolation, and no audit trail. The result is a massive, ungoverned attack surface.

### Industry Data

- MCP has become "the backbone infrastructure for connecting AI models with external tools" in 2026, with Microsoft integrating MCP across Copilot Studio and Azure AI Foundry ([Red Hat](https://www.redhat.com/en/blog/model-context-protocol-mcp-understanding-security-risks-and-controls))
- **24,008 unique secrets** exposed in MCP configuration files, with **2,117 confirmed valid credentials** ([GitGuardian 2026](https://securityboulevard.com/2026/03/the-state-of-secrets-sprawl-2026-ai-service-leaks-surge-81-and-29m-secrets-hit-public-github/))
- "Large percentages of open MCP servers suffer OAuth flaws, command injection, unrestricted network access, file exposure, plaintext credentials, and seeded tool poisoning" ([Dark Analytics](https://www.darkanalytics.com/post/cybersecurity-vulnerabilities-in-model-context-protocol-mcp-implementations))
- **Tool Poisoning Attacks** — identified by Invariant Labs: malicious tool or server metadata is registered, compromising agent behavior or exfiltrating data ([Pillar Security](https://www.pillar.security/blog/the-security-risks-of-model-context-protocol-mcp))
- **Prompt Injection via MCP** — attackers trick AI models into running hidden commands through tool descriptions and parameters ([Practical DevSecOps](https://www.practical-devsecops.com/mcp-security-vulnerabilities/))
- The MCP authorization specification "includes implementation details that conflict with modern enterprise practices" — efforts underway to improve ([MCP Security Best Practices](https://modelcontextprotocol.io/specification/draft/basic/security_best_practices))
- Salesforce Agentforce has added enterprise governance for MCP to address these gaps ([Startup Hub AI](https://www.startuphub.ai/ai-news/ai-research/2026/securing-the-model-context-protocol-agentforce-adds-enterprise-governance/))
- OpenClaw malicious skills crisis: **1,184 malicious skills** confirmed across ClawHub — the largest supply chain attack targeting AI agent infrastructure ([Reco AI](https://www.reco.ai/blog/ai-and-cloud-security-breaches-2025))

---

## The Solution

ZiraLoop MCP takes the opposite approach from static, shared MCP servers. Every minted token gets its own dynamically-scoped MCP server instance that exposes only the tools that token's scopes allow. An agent connecting via MCP literally cannot see, discover, or call tools it doesn't have permission to use.

---

## How It Works

### 1. Mint a Token with Scopes

```bash
POST /v1/tokens
{
  "credential_id": "cred_abc123",
  "ttl": "2h",
  "scopes": {
    "scopes": [
      {
        "connection_id": "conn_slack_support",
        "actions": ["send_message", "list_channels"],
        "resources": { "channel": ["C01234"] }
      },
      {
        "connection_id": "conn_github_repo",
        "actions": ["create_issue", "list_issues"],
        "resources": { "repo": ["org/frontend"] }
      }
    ]
  }
}
```

Response includes the MCP endpoint:

```json
{
  "token": "ptok_eyJhbG...",
  "jti": "tok_xyz789",
  "expires_at": "2026-03-28T16:00:00Z",
  "mcp_endpoint": "https://mcp.ziraloop.com/tok_xyz789"
}
```

### 2. Connect the Agent to Its MCP Server

The agent connects to `mcp_endpoint` using either Streamable HTTP or legacy SSE transport. The MCP server presents only the tools the token's scopes allow:

**What the agent sees:**
- `slack_send_message` (channel: #support only)
- `slack_list_channels` (read-only)
- `github_create_issue` (repo: org/frontend only)
- `github_list_issues` (repo: org/frontend only)

**What the agent does NOT see:**
- Any other Slack actions (delete_message, manage_channels, admin actions)
- Any other GitHub actions (delete_repo, merge_pr, manage_webhooks)
- Any other connections (HubSpot, Stripe, Jira — invisible)
- Any other resources (other channels, other repos — inaccessible)

### 3. Tool Execution Through Managed Proxy

When the agent calls a tool, the MCP server:
1. Validates the action against the token's scopes
2. Validates any resource IDs against the allowed set
3. Maps parameters to the API call using the catalog's execution config (method, path, body_mapping, query_mapping, headers)
4. Proxies the request through Nango with managed authentication (automatic token refresh, rate limiting)
5. Extracts the response using the catalog's response_path
6. Returns the result to the agent

The agent never handles raw credentials. The MCP server handles auth, rate limiting, and error handling transparently.

---

## Architecture

```
Agent (Claude, Cursor, etc.)
  │
  │  MCP Protocol (Streamable HTTP or SSE)
  │  Authorization: Bearer ptok_...
  │
  ▼
ZiraLoop MCP Server (scoped to this token)
  │
  │  1. Validate JTI matches URL
  │  2. Validate token has scopes
  │  3. Build/retrieve cached server for this JTI
  │  4. Expose only scoped tools
  │
  ▼
Tool Execution
  │
  │  Catalog lookup → Execution config → Nango proxy
  │
  ▼
Provider API (Slack, GitHub, etc.)
  │  Auth headers injected by Nango
  │  Token refresh handled automatically
```

### Server Caching

MCP servers are cached per JTI via `mcpserver.ServerCache`:
- First request builds the server from token scopes + catalog
- Subsequent requests reuse the cached server
- Cache entry auto-expires when the token expires
- No stale servers — revoked tokens can't access cached servers (TokenAuth middleware validates first)

---

## Security Model

### What Makes This Different from Other MCP Servers

| Concern | Standard MCP Server | ZiraLoop MCP |
|---|---|---|
| **Tool visibility** | All tools visible to all clients | Only scoped tools visible per token |
| **Credential handling** | Credentials in config files or env vars | Credentials never exposed — managed proxy |
| **Permission granularity** | None or server-level | Action-level + resource-level per token |
| **Token refresh** | Manual or not handled | Automatic via Nango |
| **Time limiting** | None | TTL-based auto-expiry |
| **Revocation** | Restart the server | Instant via Redis pub/sub |
| **Audit** | None built-in | Every tool call logged |
| **Multi-tenancy** | Not supported | Org-scoped, identity-scoped |

### Middleware Stack

Every MCP request passes through:
1. **TokenAuth** — validates the `ptok_` JWT (signature, expiry, revocation check)
2. **ValidateJTIMatch** — URL `{jti}` must match the JWT's JTI claim (prevents token reuse across endpoints)
3. **ValidateHasScopes** — token must have scopes (prevents unscoped tokens from getting tool access)

---

## The Action Catalog

ZiraLoop ships with embedded action catalogs for 45+ providers (`internal/mcp/catalog/providers/*.actions.json`). Each provider defines:

### Actions
```json
{
  "send_message": {
    "display_name": "Send Message",
    "description": "Send a message to a Slack channel",
    "access": "write",
    "resource_type": "channel",
    "parameters": { "type": "object", "properties": { "text": { "type": "string" } } },
    "execution": {
      "method": "POST",
      "path": "/api/chat.postMessage",
      "body_mapping": { "text": "text", "channel": "$resource_id" }
    }
  }
}
```

### Resources
```json
{
  "channel": {
    "display_name": "Channels",
    "id_field": "id",
    "name_field": "name",
    "list_action": "list_channels",
    "request_config": {
      "response_path": "channels"
    }
  }
}
```

### Discovery API (Public, No Auth)
| Endpoint | Description |
|---|---|
| `GET /v1/catalog/integrations` | List all providers with action and resource counts |
| `GET /v1/catalog/integrations/{id}` | Get provider detail with all actions and resources |
| `GET /v1/catalog/integrations/{id}/actions` | List actions, supports `?access=read|write` filter |

---

## Full API Surface

### MCP Server (Separate Port)
| Route | Transport | Description |
|---|---|---|
| `/{jti}/*` | Streamable HTTP | Primary MCP transport — stateless, request-response |
| `/{jti}/*` | SSE (legacy) | Legacy MCP transport — server-sent events |

### Token Minting (includes MCP endpoint)
| Endpoint | Description |
|---|---|
| `POST /v1/tokens` | Mint token with scopes. Returns `mcp_endpoint` when scopes are present. |

---

## Supported Providers

Slack, GitHub (OAuth, App, App OAuth, PAT), Stripe (5 variants), HubSpot, Salesforce, Jira (OAuth, Basic, Data Center variants), Notion, Confluence (3 variants), Zoom, Linear, Shopify, Discord, Figma, Zendesk, PagerDuty, Intercom, Box, Sentry, Asana, GitLab, Twilio, Cloudflare, Vercel, Braintree — and growing.

---

## Sources

- [Red Hat — MCP: Understanding Security Risks and Controls](https://www.redhat.com/en/blog/model-context-protocol-mcp-understanding-security-risks-and-controls)
- [GitGuardian — State of Secrets Sprawl 2026](https://securityboulevard.com/2026/03/the-state-of-secrets-sprawl-2026-ai-service-leaks-surge-81-and-29m-secrets-hit-public-github/)
- [Pillar Security — The Security Risks of MCP](https://www.pillar.security/blog/the-security-risks-of-model-context-protocol-mcp)
- [Practical DevSecOps — MCP Security Vulnerabilities](https://www.practical-devsecops.com/mcp-security-vulnerabilities/)
- [MCP Specification — Security Best Practices](https://modelcontextprotocol.io/specification/draft/basic/security_best_practices)
- [Reco AI — AI & Cloud Security Breaches 2025](https://www.reco.ai/blog/ai-and-cloud-security-breaches-2025)
