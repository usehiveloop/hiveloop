# BYOK — Ship Bring Your Own Key in Your App

> Let your customers use their own LLM keys. Ship it in days, not months.

---

## The Problem

"Bring Your Own Key" (BYOK) is no longer a nice-to-have — it's becoming a standard expectation for AI-powered SaaS. Enterprise customers want to use their own LLM API keys for cost control, compliance, and data sovereignty. Developers and power users want to use their own keys to avoid per-seat pricing and access the latest models. But implementing BYOK securely is deceptively complex.

Most teams start with the obvious approach: accept the customer's API key, encrypt it, store it in the database, decrypt when needed. It works until it doesn't. The key ends up in a log. A sandbox gets compromised. Revocation takes minutes. Every provider authenticates differently. Enterprise customers ask "How are our keys encrypted at rest?" and you don't have a good answer.

Building BYOK properly — with envelope encryption, KMS integration, sealed memory, multi-tier caching, auth scheme abstraction, and instant revocation — is 3-6 months of senior engineering time. Time better spent on your core product.

### Industry Data

- **BYOK adoption is accelerating**: JetBrains, GitHub Copilot, Vercel, OpenRouter, Cloudflare, and Factory have all shipped BYOK support. It's becoming table stakes for AI-powered developer tools and SaaS. ([JetBrains](https://blog.jetbrains.com/ai/2025/12/bring-your-own-key-byok-is-now-live-in-jetbrains-ides/), [Vercel](https://vercel.com/docs/ai-gateway/authentication-and-byok/byok), [Cloudflare](https://developers.cloudflare.com/ai-gateway/configuration/bring-your-own-keys/))
- "BYOK is the subtle shift that could reshape how we pay for AI" — the model is spreading beyond developer tools into mainstream SaaS ([Medium/Enrique Dans](https://medium.com/enrique-dans/byok-the-subtle-shift-that-could-reshape-how-we-pay-for-ai-9e165d9e63cd))
- **Gartner estimates 60%** of enterprises will prioritize BYOK in multi-cloud strategies to balance security and operational efficiency ([Geekflare](https://geekflare.com/guides/byok-ai-business-strategy/))
- Enterprise buyers are increasingly using BYOK as a "litmus test for whether a SaaS platform is enterprise-ready" ([Lexology](https://www.lexology.com/library/detail.aspx?g=0dd39f24-ee15-4bb4-8e8f-1ed2abe7ac31))
- Governments and regulators are pushing stricter data sovereignty requirements, "further incentivizing businesses to adopt BYOK" ([Geekflare](https://geekflare.com/guides/byok-ai-business-strategy/))
- Regulations like HIPAA, GDPR, and PCI DSS "often require strict control over data access and encryption practices" — BYOK helps organizations meet these standards ([IBM](https://www.ibm.com/think/topics/byok))
- Global AI SaaS spending projected to cross **$300B** in 2026 ([BetterCloud](https://www.bettercloud.com/monitor/saas-statistics/))
- "Why building your own BYOK is a trap" — WorkOS documents the engineering complexity of proper key management, recommending purpose-built solutions over custom implementations ([WorkOS](https://workos.com/blog/byok-with-vault))

---

## The Solution

ZiraLoop makes BYOK a feature you ship in days. Your customers connect their LLM provider, ZiraLoop encrypts and stores the key, and your app proxies all LLM requests through ZiraLoop — never touching the plaintext key.

---

## How It Works

### Step 1: Customer Connects Their Provider

Embed the Connect widget in your app. Your customer picks a provider and enters their API key:

```jsx
import { ZiraLoopConnect } from '@ziraloop/react'

<ZiraLoopConnect
  sessionToken="sess_..."
  onConnect={(connection) => {
    // connection.id is the credential ID
    // Use it to mint tokens and proxy requests
  }}
/>
```

The widget:
- Shows all 101 supported LLM providers with logos and model counts
- Validates the API key against the provider in real-time
- Encrypts with envelope encryption before storage
- Returns a credential ID to your app

### Step 2: Mint a Proxy Token

Your backend mints a short-lived token for the customer's session:

```bash
POST /v1/tokens
{
  "credential_id": "cred_abc123",
  "ttl": "1h",
  "remaining": 1000
}
```

Returns a `ptok_` token that:
- Is scoped to that one credential
- Expires automatically after 1 hour
- Is capped at 1,000 requests
- Can be revoked instantly

### Step 3: Proxy LLM Requests

Your app sends LLM requests through ZiraLoop using the proxy token:

```bash
POST /v1/proxy/v1/chat/completions
Authorization: Bearer ptok_eyJhbG...
Content-Type: application/json

{
  "model": "gpt-4o",
  "messages": [{ "role": "user", "content": "Hello" }]
}
```

ZiraLoop:
1. Validates the token
2. Resolves the encrypted credential from cache (sub-5ms)
3. Decrypts the API key in memory
4. Attaches the correct auth header for the provider
5. Streams the response back to your app
6. Zeros the key from memory
7. Captures usage (tokens, cost, latency) automatically

**Your app never sees the real API key. Your customer's key never touches your code.**

---

## Why This Beats Building In-House

| Concern | Build In-House | ZiraLoop |
|---|---|---|
| **Encryption** | Basic AES, key in env var | Envelope encryption, KMS-wrapped DEK, sealed memory |
| **Key exposure** | Key in app memory, logs, caches | Key never leaves the proxy, zeroed after use |
| **Multi-provider auth** | if/else per provider | Automatic auth scheme detection (Bearer, x-api-key, query_param) |
| **Revocation** | Cache TTL (minutes) | Redis pub/sub (sub-millisecond) |
| **Streaming** | Custom SSE handling per provider | Built-in with immediate chunk flushing |
| **Usage tracking** | Not built | Automatic: tokens, cost, latency per request |
| **Rate limiting** | Application-level | Infrastructure-level: per-credential, per-identity, per-token |
| **Enterprise audit** | Not built | Full audit trail + generation logs |
| **Time to ship** | 3-6 months | Days |

---

## BYOK Use Cases

### AI-Powered SaaS with Plan Tiers
```
Free tier → Platform's pooled keys, limited models
Pro tier  → BYOK, any model, usage dashboard
Enterprise → BYOK, multi-key pools, spend caps, compliance reports
```

### Cloud IDE with AI Coding
```
Students → Platform keys with spend caps
Pro users → BYOK via Connect widget
Each sandbox → Scoped token with TTL and request cap
```

### AI Agent Platform
```
Each customer → Connects their own LLM key
Each agent session → Gets a scoped, time-limited token
Platform → Full visibility into usage per customer, per agent
```

### Internal AI Tools
```
Each team → Connects their department's LLM key
IT → Sets rate limits and model restrictions per team
Finance → Gets cost attribution by team and project
```

---

## What Customers See

Through the Connect widget, your end-users experience:

1. **Provider Selection** — pick from 101 LLM providers (OpenAI, Anthropic, Google, Mistral, Groq, etc.)
2. **Guided Setup** — each provider shows setup instructions and a link to get an API key
3. **Key Validation** — real-time validation against the provider before storing
4. **Connection Confirmation** — success state with provider and status indicator
5. **Connection Management** — view active connections, verify key health, disconnect

All with your branding via customizable theming (colors, border radius, fonts).

---

## Full API Surface for BYOK

### Credential Storage
| Endpoint | Description |
|---|---|
| `POST /v1/credentials` | Store encrypted API key. Auto-detects provider from base_url or provider_id. Envelope encryption. Optional identity_id, rate limits, metadata. |
| `GET /v1/credentials` | List credentials per org (filterable by identity_id, external_id). |
| `DELETE /v1/credentials/{id}` | Revoke key. Instant propagation. |

### Token Minting
| Endpoint | Description |
|---|---|
| `POST /v1/tokens` | Mint proxy token scoped to credential. Configurable TTL, request caps (remaining/refill), metadata. |
| `DELETE /v1/tokens/{jti}` | Instant revocation. |

### LLM Proxy
| Endpoint | Description |
|---|---|
| `* /v1/proxy/*` | Streaming reverse proxy. Any method/path/body forwarded to upstream LLM provider with managed auth. |

### Widget
| Endpoint | Description |
|---|---|
| `POST /v1/connect/sessions` | Create scoped widget session for your end-user. |
| `POST /v1/widget/connections` | Store key via widget (validates, encrypts, stores). |
| `POST /v1/widget/connections/{id}/verify` | Re-verify stored key against provider. |
| `GET /v1/widget/connections` | List connected providers for the user. |
| `DELETE /v1/widget/connections/{id}` | Disconnect provider. |

### Provider Discovery
| Endpoint | Description |
|---|---|
| `GET /v1/providers` | List all 101 LLM providers (public, no auth). |
| `GET /v1/providers/{id}` | Get provider detail with all models. |
| `GET /v1/providers/{id}/models` | List models for a provider. |

### Usage & Billing Data
| Endpoint | Description |
|---|---|
| `GET /v1/generations` | Per-request usage: tokens, cost, model, latency. Filterable by credential_id (= per customer). |
| `GET /v1/reporting` | Aggregated analytics. Group by credential (= per customer) to get billing data. |
| `GET /v1/usage` | Org dashboard with top credentials, spend over time, token volumes. |

---

## Supported LLM Providers

101 providers including: OpenAI, Anthropic, Google, Google Vertex, Azure, AWS Bedrock, Mistral, Groq, Cohere, Perplexity, xAI, Together AI, Deep Infra, Cerebras, Fireworks, OpenRouter, Cloudflare AI, Venice, and dozens more.

3,183 models tracked in the registry with auto-detection from base URL.

---

## Sources

- [JetBrains — BYOK Is Now Live](https://blog.jetbrains.com/ai/2025/12/bring-your-own-key-byok-is-now-live-in-jetbrains-ides/)
- [Vercel — BYOK Documentation](https://vercel.com/docs/ai-gateway/authentication-and-byok/byok)
- [Cloudflare — Bring Your Own Keys](https://developers.cloudflare.com/ai-gateway/configuration/bring-your-own-keys/)
- [Geekflare — Why BYOK Is the Strategic Choice for AI in 2026](https://geekflare.com/guides/byok-ai-business-strategy/)
- [IBM — What Is BYOK?](https://www.ibm.com/think/topics/byok)
- [WorkOS — Why Building Your Own BYOK Is a Trap](https://workos.com/blog/byok-with-vault)
- [Medium/Enrique Dans — BYOK: The Subtle Shift](https://medium.com/enrique-dans/byok-the-subtle-shift-that-could-reshape-how-we-pay-for-ai-9e165d9e63cd)
- [Lexology — How BYOK Strengthens Trust in SaaS](https://www.lexology.com/library/detail.aspx?g=0dd39f24-ee15-4bb4-8e8f-1ed2abe7ac31)
- [BetterCloud — 2026 SaaS Statistics](https://www.bettercloud.com/monitor/saas-statistics/)
