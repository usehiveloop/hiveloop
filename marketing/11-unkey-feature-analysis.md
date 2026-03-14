# Unkey Feature Analysis — What LLMVault Should Steal

---

## What Unkey Is

Unkey is an open-source API key management platform for developers. It handles key creation, verification, rate limiting, usage tracking, and permissions. It's general-purpose — not LLM-specific — but several of their features map beautifully to LLMVault's use cases.

**Key insight**: Unkey thinks about API keys as *configurable objects with behavior* (rate limits, usage caps, expiration, metadata, permissions). LLMVault currently thinks about credentials as *encrypted blobs to proxy through*. Adopting Unkey's "smart key" philosophy would make LLMVault dramatically more powerful.

---

## Features to Adopt (Ranked by Impact for LLMVault)

---

### 1. Remaining Uses + Auto-Refill (Credit System)

**What Unkey does**: Every API key can have a `remaining` counter — a hard cap on how many times it can be used. When it hits 0, the key is rejected. You can configure automatic refills on a schedule (e.g., 10,000 uses, refills monthly on the 1st).

**Why this is perfect for LLMVault**: This is the simplest, most elegant implementation of usage caps for LLM tokens. Instead of complex dollar-based budgeting (which requires parsing provider responses for token counts), you can ship request-based caps immediately:

```json
POST /v1/tokens
{
  "credential_id": "cred_abc",
  "ttl": "24h",
  "remaining": 500,        // This token can make 500 requests, then it's dead
}

POST /v1/credentials
{
  "label": "acme_openai",
  "base_url": "https://api.openai.com",
  "auth_scheme": "bearer",
  "api_key": "sk-...",
  "remaining": 10000,       // 10K requests max on this credential
  "refill": {
    "amount": 10000,
    "interval": "monthly"   // Refills to 10K on the 1st of each month
  }
}
```

**LLMVault-specific value**:
- **Sandbox use case**: "This sandbox agent gets 200 requests, then it's cut off." No runaway loops.
- **Tiered plans**: Free customers get 1,000 requests/month (auto-refill), Pro gets 50,000.
- **Simple to implement**: Just an atomic counter in Redis. Decrement on each proxy request. No need to parse LLM response bodies for token counts on day one.
- **Composable with TTL**: A token with `remaining: 100` AND `ttl: 1h` expires at whichever comes first — time or usage.

**Priority**: Build this FIRST. It's low effort, high impact, and immediately differentiating.

---

### 2. Variable Cost per Request (Consumption-Based Rate Limiting)

**What Unkey does**: Instead of each request costing 1 "use," different requests can cost different amounts. You can specify a `cost` per verification call. A cheap operation costs 1 credit, an expensive one costs 10.

**Why this is perfect for LLMVault**: Different LLM models cost wildly different amounts. A GPT-4o-mini request and a Claude Opus request shouldn't consume the same number of credits from a budget.

```json
// When proxying, LLMVault could assign cost based on the model in the request body:
POST /v1/proxy/v1/chat/completions
{
  "model": "gpt-4o-mini",     // → costs 1 credit
  "messages": [...]
}

POST /v1/proxy/v1/chat/completions
{
  "model": "gpt-4",           // → costs 10 credits
  "messages": [...]
}
```

**LLMVault-specific value**:
- Platforms can define cost tables per model: `{ "gpt-4o-mini": 1, "gpt-4o": 5, "gpt-4": 10, "claude-opus": 15 }`
- Combines with the `remaining` counter: "This token has 1,000 credits. A mini request costs 1, an opus request costs 15."
- Enables **fair usage metering** without dollar-level precision — good enough for 90% of use cases and way simpler to implement than parsing provider billing responses
- Opens the door to "AI credits" as a pricing unit

**Priority**: Build alongside or right after remaining uses. The `remaining` counter just needs a variable decrement instead of always decrementing by 1.

---

### 3. Identities (Grouping Credentials Under a Customer)

**What Unkey does**: An "Identity" groups multiple API keys under a single user or organization. The identity can have shared metadata and — critically — **shared rate limits across all keys**.

**Why this is perfect for LLMVault**: A single customer of your platform might connect multiple LLM providers (OpenAI + Anthropic + Google). With Identities:

- **Shared rate limits**: "Customer X can't exceed 500 requests/minute across ALL their providers combined." Prevents abuse through provider-switching.
- **Shared budgets**: "Customer X has 50,000 credits/month, shared across all their connected providers."
- **Unified analytics**: "Show me all usage for Customer X" — across all their credentials, all providers.
- **External ID mapping**: Link an identity to your own user/org IDs (`externalId: "user_123_in_your_db"`).

```json
POST /v1/identities
{
  "external_id": "customer_42",
  "meta": { "plan": "pro", "company": "Acme Corp" },
  "ratelimit": {
    "limit": 500,
    "interval": "1m"
  },
  "remaining": 50000,
  "refill": { "amount": 50000, "interval": "monthly" }
}

// When creating a credential, link it to the identity:
POST /v1/credentials
{
  "identity_id": "ident_xyz",
  "label": "acme_openai",
  "base_url": "https://api.openai.com",
  ...
}
```

**LLMVault-specific value**:
- Solves the "one customer, many providers" problem elegantly
- Cross-provider rate limiting — unique to LLMVault (no LLM gateway does this)
- Cross-provider usage budgets — "50K credits across all providers"
- Makes analytics and billing natural: group by identity, not by individual credential

**Priority**: Build after remaining uses / variable cost. This is the feature that turns LLMVault from "per-credential management" to "per-customer platform."

---

### 4. Granular Per-Token Permissions (RBAC)

**What Unkey does**: API keys can have roles and permissions attached. On verification, you can check if a key has a specific permission. Permissions propagate globally in seconds.

**Why this is perfect for LLMVault**: Scoped tokens should carry more than just "can access credential X." They should carry fine-grained permissions about WHAT they can do through that credential:

```json
POST /v1/tokens
{
  "credential_id": "cred_abc",
  "ttl": "1h",
  "permissions": [
    "models:gpt-4o-mini",       // Can only use this model
    "models:gpt-4o",            // And this one
    "endpoints:chat.completions", // Can only do chat completions
    "endpoints:embeddings"        // And embeddings
    // Implicitly CANNOT do: image generation, audio, fine-tuning
  ]
}
```

**LLMVault-specific value**:
- **Model restrictions**: Sandbox token can only use cheap models (GPT-4o-mini), not expensive ones (GPT-4)
- **Endpoint restrictions**: Token can do chat completions but not image generation or fine-tuning
- **Replaces the entire "policy engine" concept** from our future features doc with a simpler, per-token permissions model
- **Enables plan-based access**: Free plan tokens get `models:*-mini` permissions. Pro tokens get `models:*`.

**Priority**: Medium. Can ship a basic version (model allowlist) quickly, expand to full RBAC later.

---

### 5. Per-Key / Per-Token Metadata

**What Unkey does**: Every key can carry arbitrary JSON metadata. This metadata is returned on verification and can be used for filtering, analytics, and business logic.

**Why this is perfect for LLMVault**: Credentials and tokens in LLMVault currently have minimal metadata (just a `label`). Rich metadata enables:

```json
POST /v1/credentials
{
  "label": "acme_openai",
  "base_url": "https://api.openai.com",
  "auth_scheme": "bearer",
  "api_key": "sk-...",
  "meta": {
    "customer_id": "cust_42",
    "workspace": "production",
    "connected_by": "user_jane@acme.com",
    "plan": "enterprise",
    "department": "engineering"
  }
}

POST /v1/tokens
{
  "credential_id": "cred_abc",
  "ttl": "1h",
  "meta": {
    "sandbox_id": "sbx_789",
    "agent_name": "code-reviewer",
    "session_id": "sess_xyz",
    "purpose": "pull-request-review"
  }
}
```

**LLMVault-specific value**:
- **Audit enrichment**: Audit logs show WHO connected the key, WHICH sandbox used it, for WHAT purpose
- **Analytics grouping**: "Show me usage by department" or "by agent_name" — without schema changes
- **Customer integration**: Platforms can attach their own IDs and context, making LLMVault data joinable with their own systems
- **Filtering**: "List all credentials where meta.plan = 'enterprise'"
- **Low effort, high flexibility**: Just a JSONB column. No schema migrations when customer needs change.

**Priority**: Very low effort, high value. Ship this early.

---

### 6. Standalone Rate Limiting (Decouple from Credentials)

**What Unkey does**: Rate limiting works independently of API key management. You can rate-limit by any identifier — IP address, user ID, session ID, endpoint — without creating an API key first.

**Why this is useful for LLMVault**: LLMVault currently rate-limits per org. But platforms need more granular control:

```json
// Rate limit by end-user (not just by org)
POST /v1/ratelimit
{
  "identifier": "end_user_42",
  "limit": 100,
  "interval": "1m"
}

// Rate limit by model
POST /v1/ratelimit
{
  "identifier": "model:gpt-4",
  "limit": 50,
  "interval": "1h"
}

// Rate limit by IP (for the Connect Widget)
POST /v1/ratelimit
{
  "identifier": "ip:203.0.113.42",
  "limit": 10,
  "interval": "1m"
}
```

**LLMVault-specific value**:
- **End-user rate limiting**: "Each end-user of the platform can make 100 LLM requests/minute" — the platform passes the user ID, LLMVault enforces the limit
- **Model-specific limits**: "Only 50 GPT-4 requests per hour across the whole org" — controls expensive model usage
- **Widget protection**: Rate-limit the Connect Provider widget by IP to prevent abuse
- **Globally distributed**: If LLMVault deploys at the edge, rate limits work globally

**Priority**: Medium. Useful but not critical for MVP. Per-org rate limiting (which LLMVault already has) covers the basics.

---

### 7. Key Recovery (Opt-In Decrypt-on-Demand)

**What Unkey does**: Their "Vault" feature lets you recover (decrypt) stored API keys via an API call with the `?decrypt=true` query parameter. Requires explicit opt-in and specific permissions.

**Why this is useful for LLMVault**: LLMVault's current stance is "we never return the plaintext key after storage." This is maximally secure but creates friction:
- Customer wants to migrate to another platform — can't export their key
- Platform needs to audit that the stored key matches what the customer intended
- Customer wants to rotate their key by seeing the old one first

**How to adapt for LLMVault**:
```json
// Opt-in at credential creation time
POST /v1/credentials
{
  "label": "acme_openai",
  "api_key": "sk-...",
  "recoverable": true    // Explicitly opt-in to key recovery
}

// Later, with elevated permissions:
GET /v1/credentials/cred_abc?decrypt=true
// → { "id": "cred_abc", "plaintext": "sk-...", ... }
// Audit log: "credential.decrypted" with IP, timestamp, user
```

**LLMVault-specific value**:
- Reduces customer lock-in anxiety ("what if I need my key back?")
- Enables key migration between platforms
- Useful for compliance audits
- Must be heavily guarded: opt-in, separate permission, audit logged, maybe 2FA-gated

**Priority**: Low-medium. Nice to have. The security-first stance of "never decrypt" is actually a selling point for most customers. Offer this as an opt-in for those who need it.

---

### 8. IP Whitelisting per Credential / Token

**What Unkey does**: Enterprise feature that restricts key usage to specific IP addresses or CIDR ranges.

**Why this is useful for LLMVault**: For sandbox environments, you know the IP range of your sandbox infrastructure. Restricting a scoped token to only work from specific IPs adds defense-in-depth:

```json
POST /v1/tokens
{
  "credential_id": "cred_abc",
  "ttl": "1h",
  "allowed_ips": ["10.0.0.0/8", "172.16.0.0/12"]  // Only sandbox network
}
```

**LLMVault-specific value**:
- If a scoped token leaks outside the sandbox, it's useless — wrong IP
- Enterprise customers love this for compliance
- Pairs well with the token's TTL and remaining uses for triple protection (time + usage + network)

**Priority**: Medium. Enterprise feature. Add when pursuing enterprise customers.

---

## Summary: What to Build & When

| Phase | Feature (from Unkey) | LLMVault Adaptation | Effort |
|-------|---------------------|---------------------|--------|
| **Next sprint** | Remaining uses + refill | Per-token and per-credential request caps with auto-refill | Low |
| **Next sprint** | Per-key metadata | JSONB `meta` field on credentials and tokens | Very Low |
| **Sprint +1** | Variable cost | Model-based cost tables for the remaining counter | Low |
| **Sprint +2** | Identities | Group credentials under a customer identity with shared limits | Medium |
| **Sprint +3** | Permissions | Per-token model/endpoint restrictions | Medium |
| **Sprint +4** | Standalone rate limiting | Rate limit by arbitrary identifier (end-user, model, IP) | Medium |
| **Later** | Key recovery | Opt-in decrypt-on-demand with audit logging | Low |
| **Enterprise** | IP whitelisting | Per-token IP/CIDR restrictions | Low |

---

## The Compound Effect

These features don't just add value individually — they **multiply** when combined:

**A single scoped token could have**:
- Bound to credential `cred_abc` (Anthropic key)
- TTL: 1 hour (expires automatically)
- Remaining: 200 requests (hard usage cap)
- Cost table: `{ "claude-haiku": 1, "claude-sonnet": 5, "claude-opus": 15 }` (variable consumption)
- Permissions: `["models:claude-haiku-*", "models:claude-sonnet-*"]` (no Opus access)
- IP whitelist: `["10.0.0.0/8"]` (sandbox network only)
- Meta: `{ "sandbox_id": "sbx_789", "agent": "code-reviewer" }` (audit context)
- Identity: `ident_customer42` (shared 50K monthly credits across all providers)

That token is **incredibly specific, maximally safe, and fully auditable**. No other product in the market can create a credential with this level of granular control for LLM access.

This is LLMVault's moat: **not just a proxy, but the most granular, secure, and controllable way to give anything access to an LLM.**
