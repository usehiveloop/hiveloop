# Vault — Enterprise-Grade Credential Custody

> Your agents' credentials deserve a vault, not an environment variable.

---

## The Problem

AI agent credentials are the new secrets sprawl — and the numbers are staggering. Agents need API keys and OAuth tokens to function, but those credentials end up scattered across environment variables, config files, MCP configurations, log output, CI runners, and chat threads. Each is a point of exposure. When any one of them is compromised, the attacker gets full access to everything the credential unlocks.

### Industry Data

- **1.27 million AI service secrets** leaked on public GitHub in 2025 — an **81% YoY surge** ([GitGuardian State of Secrets Sprawl 2026](https://securityboulevard.com/2026/03/the-state-of-secrets-sprawl-2026-ai-service-leaks-surge-81-and-29m-secrets-hit-public-github/))
- LLM infrastructure secrets leaked **5x faster** than core model provider keys ([GitGuardian 2026](https://securityboulevard.com/2026/03/the-state-of-secrets-sprawl-2026-ai-service-leaks-surge-81-and-29m-secrets-hit-public-github/))
- **24,008 secrets** exposed in MCP configuration files alone, with **2,117 confirmed valid** ([GitGuardian 2026](https://securityboulevard.com/2026/03/the-state-of-secrets-sprawl-2026-ai-service-leaks-surge-81-and-29m-secrets-hit-public-github/))
- **64% of valid secrets from 2022 are still active** in 2026 — credentials never get rotated ([GitGuardian 2026](https://securityboulevard.com/2026/03/the-state-of-secrets-sprawl-2026-ai-service-leaks-surge-81-and-29m-secrets-hit-public-github/))
- Claude Code-assisted commits show a **3.2% secret-leak rate** vs. 1.5% baseline ([OECD AI](https://oecd.ai/en/incidents/2026-03-17-2273))
- Shadow AI breaches cost **$670,000 more** than standard incidents — averaging **$4.63M per breach** ([IBM Cost of a Data Breach 2025](https://www.bvp.com/atlas/securing-ai-agents-the-defining-cybersecurity-challenge-of-2026))
- **28.65 million** hardcoded secrets added to public GitHub in 2025, a **34% increase** YoY ([GitGuardian 2026](https://securityboulevard.com/2026/03/the-state-of-secrets-sprawl-2026-ai-service-leaks-surge-81-and-29m-secrets-hit-public-github/))
- Internal repositories are **6x more likely** to contain hardcoded secrets than public ones ([GitGuardian 2026](https://securityboulevard.com/2026/03/the-state-of-secrets-sprawl-2026-ai-service-leaks-surge-81-and-29m-secrets-hit-public-github/))
- **28% of incidents** originate outside repositories — in Slack, Jira, Confluence ([GitGuardian 2026](https://securityboulevard.com/2026/03/the-state-of-secrets-sprawl-2026-ai-service-leaks-surge-81-and-29m-secrets-hit-public-github/))
- Standard secret managers protect storage but not exposure: "keys are in the agent's memory, in its environment, in its HTTP headers, and in its logs" ([Dev.to](https://dev.to/jonathanfishner/why-your-ai-agents-api-keys-are-a-ticking-time-bomb-12pm))

---

## The Solution

ZiraLoop Vault is a purpose-built credential custody layer for AI agents. It stores both LLM API keys and SaaS OAuth tokens with enterprise-grade encryption, and proxies all requests so credentials never touch application code, agent memory, or logs.

---

## Encryption Architecture

### Envelope Encryption

Every credential is protected with a two-layer encryption model:

1. **Data Encryption Key (DEK)** — A unique random 256-bit key is generated for every credential
2. **AES-256-GCM encryption** — The credential is encrypted locally using the DEK
3. **KMS wrapping** — The DEK is wrapped (encrypted) by a Key Management Service
4. **Storage** — Only encrypted blobs are stored in Postgres. The plaintext key and DEK are zeroed from memory immediately.

Even if the database AND Redis are both compromised, the attacker gets encrypted blobs that are useless without KMS access.

### KMS Options

| KMS Backend | Use Case | Configuration |
|---|---|---|
| **AEAD** | Local development | Symmetric key via `KMS_KEY` env var |
| **AWS KMS** | Production | AWS region + KMS key ARN |
| **Vault Transit** | Self-hosted production | HashiCorp Vault Transit backend |

### 3-Tier Cache

Credential resolution follows a three-tier cache for sub-5ms hot-path performance:

```
L1: In-memory (memguard-sealed) — 0.01ms
L2: Redis (encrypted blobs) — 1-3ms
L3: Postgres + KMS unwrap — 3-8ms
```

Even the in-memory cache uses memguard to prevent keys from being extracted via memory dumps. The Redis layer stores encrypted blobs — if Redis is compromised, the attacker gets nothing usable.

---

## Dual Credential Types

ZiraLoop stores two categories of credentials through the same security model:

### LLM API Keys (Encrypted Locally)

```bash
POST /v1/credentials
{
  "label": "customer_42_anthropic",
  "base_url": "https://api.anthropic.com",
  "api_key": "sk-ant-...",
  "identity_id": "ident_abc123"
}
```

The API key is encrypted with envelope encryption. The provider is auto-detected from the base_url. Auth scheme (Bearer, x-api-key, query_param) is resolved automatically. Supports 101 LLM providers.

### SaaS OAuth Tokens (Managed via Nango)

```bash
POST /v1/integrations
{
  "provider": "slack",
  "display_name": "Slack - Support Team",
  "credentials": {
    "type": "OAUTH2",
    "client_id": "...",
    "client_secret": "..."
  }
}
```

OAuth credentials are stored in Nango (source of truth). Nango handles token refresh, re-authentication, and provider-specific flows. ZiraLoop stores reference records with metadata.

---

## Instant Revocation

When a credential is revoked, it dies everywhere immediately:

- `DELETE /v1/credentials/{id}` — soft-deletes the credential
- Redis pub/sub broadcasts the revocation to every proxy instance
- Sub-millisecond propagation — no stale credential window
- Same pattern for connections: `DELETE /v1/connections/{id}` revokes in Nango and locally

---

## Full API Surface

### Credential Management
| Endpoint | Description |
|---|---|
| `POST /v1/credentials` | Create encrypted credential. Auto-detects provider from base_url. Generates unique DEK, encrypts with AES-256-GCM, wraps DEK via KMS. Supports optional identity_id, external_id, rate limits (remaining/refill_amount/refill_interval), JSONB metadata. |
| `GET /v1/credentials` | List credentials (cursor paginated). Filterable by identity_id, external_id, JSONB meta (@> operator). |
| `GET /v1/credentials/{id}` | Get credential detail (includes request_count, last_used_at). |
| `DELETE /v1/credentials/{id}` | Revoke credential. Soft-delete with instant cache invalidation via pub/sub. |

### Integration Management
| Endpoint | Description |
|---|---|
| `POST /v1/integrations` | Create integration in Nango. Validates credentials against provider auth_mode. Stores nango_config (logo, callback_url, auth_mode, docs, credentials_schema). |
| `GET /v1/integrations` | List integrations (filterable by provider, meta). |
| `GET /v1/integrations/{id}` | Get integration with live nango_config from Nango API. |
| `PUT /v1/integrations/{id}` | Update credentials or metadata. Pushes credential changes to Nango. |
| `DELETE /v1/integrations/{id}` | Soft-delete integration and remove from Nango. |

### Connection Management
| Endpoint | Description |
|---|---|
| `POST /v1/integrations/{id}/connections` | Create connection after OAuth flow. Links to identity. |
| `GET /v1/connections/{id}` | Get connection with live provider_config from Nango (credentials, connection_config, metadata). Strips sensitive internal fields (jwtToken). |
| `DELETE /v1/connections/{id}` | Revoke connection in both Nango and local database. |

---

## Sources

- [GitGuardian — State of Secrets Sprawl 2026](https://securityboulevard.com/2026/03/the-state-of-secrets-sprawl-2026-ai-service-leaks-surge-81-and-29m-secrets-hit-public-github/)
- [OECD AI — AI Coding Assistants Drive Surge in Secret Leaks](https://oecd.ai/en/incidents/2026-03-17-2273)
- [BVP — Securing AI Agents 2026](https://www.bvp.com/atlas/securing-ai-agents-the-defining-cybersecurity-challenge-of-2026)
- [Dev.to — Why Your AI Agent's API Keys Are a Ticking Time Bomb](https://dev.to/jonathanfishner/why-your-ai-agents-api-keys-are-a-ticking-time-bomb-12pm)
