# LLMVault — Homepage Copy

---

## Hero Section

### Headline
Your customers' LLM keys deserve a vault, not a database column.

### Subheadline
LLMVault is the secure proxy layer for platforms that handle LLM API keys. Store credentials with envelope encryption, mint scoped tokens for sandboxes, and proxy requests to any provider — with sub-5ms overhead.

### Primary CTA
Get Started — Free

### Secondary CTA
Read the Docs

### Hero Code Snippet

```bash
# 1. Store a customer's API key (encrypted automatically)
curl -X POST https://api.llmvault.dev/v1/credentials \
  -H "Authorization: Bearer org_your_api_key" \
  -d '{
    "label": "customer_42_anthropic",
    "base_url": "https://api.anthropic.com",
    "auth_scheme": "x-api-key",
    "api_key": "sk-ant-..."
  }'

# 2. Mint a short-lived token for the sandbox
curl -X POST https://api.llmvault.dev/v1/tokens \
  -H "Authorization: Bearer org_your_api_key" \
  -d '{ "credential_id": "cred_uuid", "ttl": "1h" }'

# → { "token": "ptok_eyJhbG...", "expires_at": "..." }

# 3. Proxy LLM requests using the scoped token
curl -X POST https://api.llmvault.dev/v1/proxy/v1/messages \
  -H "Authorization: Bearer ptok_eyJhbG..." \
  -d '{ "model": "claude-sonnet-4-6", "messages": [...] }'

# → Streams the response directly from Anthropic. Your app never sees the real key.
```

---

## Problem Section

### Section Headline
You're building an AI product. Your customers' API keys are your problem.

### Copy
Every platform that supports "Bring Your Own Key" faces the same challenge. Your customers hand you their OpenAI, Anthropic, or Google keys — and trust that you'll keep them safe.

Most teams start with the obvious approach: encrypt the key, store it in Postgres, decrypt when needed. It works. Until it doesn't.

### Problem Cards

**The key ends up in a log somewhere.**
A debug statement, a middleware logger, an error handler that dumps context — and suddenly a customer's API key is sitting in plaintext in your log aggregator.

**Your sandbox gets compromised.**
AI agents run in sandboxes. If the sandbox has the real API key in an environment variable, a compromised sandbox means a compromised key.

**Revocation takes minutes, not milliseconds.**
A customer disconnects their key — but your cache layer keeps serving the old credential for the next 30 minutes.

**Every provider authenticates differently.**
OpenAI uses Bearer tokens. Anthropic uses x-api-key. Google uses query parameters. You're writing `if/else` branches for each one.

**Enterprise customers ask hard questions.**
"How are our keys encrypted at rest? What KMS do you use? Can we see an audit trail? Are you SOC 2 compliant?" — and you don't have great answers.

---

## How It Works Section

### Section Headline
Three API calls. Full LLM credential security.

### Step 1: Store
**Your customer connects their LLM provider.**
One API call stores the credential. LLMVault generates a unique data encryption key, encrypts the API key with AES-256-GCM, wraps the DEK with Vault Transit KMS, and stores only encrypted blobs. The plaintext key is zeroed from memory immediately.

```json
POST /v1/credentials
{
  "label": "acme_corp_openai",
  "base_url": "https://api.openai.com",
  "auth_scheme": "bearer",
  "api_key": "sk-..."
}
// → { "id": "cred_abc123", "label": "acme_corp_openai", ... }
// The API key is never stored in plaintext. Anywhere.
```

### Step 2: Mint
**Create scoped, short-lived tokens for your sandboxes or sessions.**
Each token is a JWT scoped to one credential, with a configurable TTL. Hand it to a sandbox, an agent, or a user session. It expires automatically. Revoke it instantly if needed.

```json
POST /v1/tokens
{ "credential_id": "cred_abc123", "ttl": "1h" }
// → { "token": "ptok_eyJhbG...", "expires_at": "2026-03-08T13:00:00Z" }
```

### Step 3: Proxy
**Your app sends LLM requests through LLMVault. The proxy handles auth.**
The sandbox uses the scoped token to make requests. LLMVault resolves the real API key from its encrypted store, attaches the correct auth header for the provider, and streams the response back. The sandbox never sees the real key.

```bash
POST /v1/proxy/v1/messages
Authorization: Bearer ptok_eyJhbG...
{ "model": "claude-sonnet-4-6", "messages": [{ "role": "user", "content": "Hello" }] }
// → Streamed response from Anthropic, through LLMVault, to your app.
```

---

## Features Section

### Section Headline
Built for LLM credential security. Nothing else.

### Feature Cards

#### Envelope Encryption
Every credential is encrypted with a unique DEK (AES-256-GCM). Every DEK is wrapped by HashiCorp Vault Transit KMS. Even if your database and Redis are both compromised, the attacker gets nothing usable without Vault access.

#### Sub-5ms Proxy Overhead
Three-tier cache: in-memory (sealed with memguard) → Redis → Postgres/Vault. Hot path resolves in 0.01ms. Cold path: 3-8ms. Your users will never notice the extra hop.

#### Scoped, Short-Lived Tokens
Mint JWTs bound to a single credential with TTLs from seconds to 24 hours. Perfect for sandboxes, agent sessions, and temporary access. Auto-expire. Instant revocation.

#### Any Provider, One Interface
OpenAI (Bearer), Anthropic (x-api-key), Google (query_param), Fireworks (Bearer), OpenRouter (Bearer) — and any custom provider. LLMVault handles auth scheme differences automatically.

#### Instant Revocation
Customer disconnects their key? Revocation propagates to every proxy instance in sub-millisecond time via Redis pub/sub. No stale credentials. No cache window vulnerabilities.

#### Multi-Tenant Isolation
Every database query is scoped by org_id. Every Redis key is namespaced. Tenant A cannot access Tenant B's credentials — enforced at the architecture level, not just the application level.

---

## Architecture Section

### Section Headline
See exactly how your customers' keys are protected.

### Copy
We don't ask you to trust our marketing. We show you the architecture.

### Visual: Architecture Diagram

```
Your App                    LLMVault                         LLM Provider
  │                            │                                  │
  │  POST /v1/credentials      │                                  │
  │  (plaintext key)           │                                  │
  │ ─────────────────────────► │                                  │
  │                            │  Generate DEK                    │
  │                            │  AES-256-GCM encrypt key         │
  │                            │  Vault Transit wrap DEK          │
  │                            │  Store encrypted blobs in PG     │
  │                            │                                  │
  │                            │  ✓ Plaintext zeroed from memory  │
  │ ◄───────────────────────── │                                  │
  │  { "id": "cred_..." }     │                                  │
  │                            │                                  │
  │  POST /v1/tokens           │                                  │
  │ ─────────────────────────► │                                  │
  │ ◄───────────────────────── │                                  │
  │  { "token": "ptok_..." }  │                                  │
  │                            │                                  │
  │  POST /v1/proxy/v1/messages│                                  │
  │  Authorization: ptok_...   │                                  │
  │ ─────────────────────────► │                                  │
  │                            │  Validate JWT                    │
  │                            │  Resolve credential (L1→L2→L3)  │
  │                            │  Decrypt API key in memory       │
  │                            │  Attach provider auth header     │
  │                            │ ────────────────────────────────►│
  │                            │                                  │
  │                            │ ◄────────────────────────────────│
  │ ◄───────────────────────── │  Stream response through        │
  │  Streamed LLM response     │                                  │
```

### Link
[Read the full architecture documentation →](/docs/architecture)

---

## Use Cases Section

### Section Headline
Built for these exact problems.

### Card 1: Bring Your Own Key
Your SaaS product lets customers use their own LLM keys. LLMVault stores them securely and proxies every request — so your application code never handles plaintext credentials.
[Learn more →](/use-cases/bring-your-own-key)

### Card 2: Sandbox & Agent Credentials
Your AI agents run in sandboxed environments. Give each sandbox a short-lived, scoped token instead of a real API key. If the sandbox is compromised, revoke the token instantly.
[Learn more →](/use-cases/sandbox-credentials)

### Card 3: "Connect Provider" Widget
Add a polished "Connect Your LLM Provider" flow to your app. Customers pick a provider, enter their key, and they're connected. LLMVault handles encryption and proxying behind the scenes.
[Learn more →](/use-cases/connect-provider-widget)

### Card 4: Multi-Tenant AI Platforms
Build a platform where each tenant has isolated LLM access. Separate credentials, separate rate limits, separate audit trails — enforced at the infrastructure level.
[Learn more →](/use-cases/multi-tenant-ai-platforms)

---

## Social Proof Section (Placeholder)

### Before launch
"Trusted by engineers building the next generation of AI products."
[Join the waitlist →]

### After first customers
Logos + quote: "LLMVault let us ship BYOK support in 3 days instead of 3 months." — [Name], [Title], [Company]

---

## Open Source / Transparency Section

### Section Headline
Open architecture. No black boxes.

### Copy
LLMVault's security model is documented down to the encryption algorithm and cache invalidation protocol. We believe the best security is transparent security.

Read the architecture docs. Review the encryption model. Understand exactly how every customer key is protected at every layer.

[View Architecture →](/docs/architecture) · [Security Model →](/security)

---

## Final CTA Section

### Headline
Start securing LLM keys in 15 minutes.

### Subheadline
Free tier includes 10 credentials and 10,000 proxy requests/month. No credit card required.

### CTA
Get Started — Free

### Secondary
[Talk to Sales →](/contact) for enterprise and self-hosted deployments.

---

## SEO Metadata

**Title tag**: LLMVault — Secure Proxy Layer for LLM API Keys
**Meta description**: Store LLM API keys with envelope encryption, mint scoped tokens for sandboxes, and proxy requests to any provider with sub-5ms overhead. Built for platforms with BYOK.
**OG Title**: Your customers' LLM keys deserve a vault, not a database column.
**OG Description**: The secure proxy layer for AI platforms. Store keys. Mint tokens. Proxy requests.
