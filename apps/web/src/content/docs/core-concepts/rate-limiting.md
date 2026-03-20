---
title: Rate Limiting
description: Comprehensive rate limiting at organization, identity, token, and credential levels with automatic refill and layered enforcement.
---

# Rate Limiting

LLMVault enforces rate limits at multiple levels to protect your LLM quota, prevent abuse, and give you fine-grained control over who can make how many requests. Each layer serves a different purpose, and requests must pass through all of them.

## Rate Limit Layers

```
┌─────────────────────────────────────────────────────┐
│  Organization Rate Limit                             │
│  Global per-org request rate                         │
├─────────────────────────────────────────────────────┤
│  Identity Rate Limits                                │
│  Per-user or per-entity limits                       │
├─────────────────────────────────────────────────────┤
│  Token Request Caps                                  │
│  Per-token lifetime request budgets                  │
├─────────────────────────────────────────────────────┤
│  Credential Request Caps                             │
│  Per-credential lifetime request budgets             │
└─────────────────────────────────────────────────────┘
```

A request must pass **all** applicable layers. If any layer rejects the request, it returns `429 Too Many Requests` with a `Retry-After` header.

## Organization Rate Limits

Every organization has a global request rate limit that applies to all proxy traffic. This prevents any single org from overwhelming the system.

- **Default**: 1,000 requests per minute
- **Burst**: Up to 10% of the rate limit in a single burst (e.g., 100 requests at once for a 1,000/min limit)

When exceeded:

```json
{"error": "rate limit exceeded"}
```

The `Retry-After` header indicates when to retry (typically 1 second).

| Rate Limit | Effective RPS | Burst Capacity |
|------------|---------------|----------------|
| 1,000/min | ~17 req/sec | 100 |
| 6,000/min | 100 req/sec | 600 |
| 100/min | ~1.7 req/sec | 10 |

## Identity Rate Limits

Identity rate limits enforce per-user or per-entity request rates. They're configured on [identities](/docs/core-concepts/identities) and checked whenever a request comes through a credential linked to that identity.

### Setting Identity Rate Limits

```typescript
const vault = new LLMVault({ apiKey: "your-api-key" });

const { data, error } = await vault.identities.create({
  external_id: "user_12345",
  rate_limit: [
    { name: "burst", limit: 20, duration: 1000 },
    { name: "default", limit: 1000, duration: 60000 },
  ],
});
```

### Multiple Rate Limits

An identity can have multiple named rate limits that work together:

| Name | Limit | Duration | Purpose |
|------|-------|----------|---------|
| `burst` | 20 | 1000ms | Prevents spikes: max 20 req/sec |
| `default` | 1000 | 60000ms | Sustained cap: max 1,000 req/min |

All rate limits must pass for a request to proceed. This lets you prevent both short bursts and sustained overuse.

### Window Behavior

Rate limit windows work as follows:

1. The first request starts a new window with the configured duration
2. Each subsequent request within the window increments the counter
3. When the limit is reached, all requests are rejected until the window expires
4. After expiry, the next request starts a fresh window

When exceeded:

```json
{"error": "identity rate limit exceeded: burst"}
```

The `Retry-After` header tells you how many seconds until the current window expires.

## Token Request Caps

Token request caps limit the total number of requests a single token can make over its entire lifetime. Unlike rate limits (which reset on a window), request caps are a hard budget that counts down.

### Setting Token Caps

```typescript
const { data, error } = await vault.tokens.create({
  credential_id: "cred-uuid",
  expires_in: "4h",
  remaining: 100,
  refill_amount: 100,
  refill_interval: "1h",
});
```

| Field | Description |
|-------|-------------|
| `remaining` | Starting request budget |
| `refill_amount` | Requests to restore when the interval elapses |
| `refill_interval` | How often to refill (e.g., `1h`, `24h`) |

### Automatic Refill

When a token's cap hits zero, LLMVault checks if enough time has passed since the last refill. If so, the counter is automatically reset to `refill_amount` — no manual intervention needed.

If no refill is configured or no refill is due yet:

```json
{"error": "token request cap exhausted"}
```

Token caps are automatically cleaned up when the token expires.

## Credential Request Caps

Credential request caps work like token caps, but apply to the credential itself — shared across **all** tokens minted against it.

### Setting Credential Caps

```typescript
const { data, error } = await vault.credentials.create({
  label: "Production OpenAI",
  api_key: "sk-...",
  provider_id: "openai",
  remaining: 10000,
  refill_amount: 10000,
  refill_interval: "24h",
});
```

| Field | Description |
|-------|-------------|
| `remaining` | Starting request budget for the credential |
| `refill_amount` | Requests to restore when the interval elapses |
| `refill_interval` | How often to refill (e.g., `1h`, `24h`, `168h`) |

### Two-Phase Checking

When both a token and its credential have request caps, LLMVault checks them in sequence:

1. Decrement the token counter
2. Decrement the credential counter
3. If the credential counter is exhausted, the token counter is rolled back

This ensures your token cap isn't consumed when the credential itself is out of budget.

When exceeded:

```json
{"error": "credential request cap exhausted"}
```

### Refill Intervals

| Interval | Description |
|----------|-------------|
| `15m` | Every 15 minutes |
| `1h` | Every hour |
| `24h` | Daily |
| `168h` | Weekly |

## How the Layers Interact

Here's the full sequence for every proxy request:

```
Request arrives
  │
  ├─ 1. Organization rate limit
  │     → 429 "rate limit exceeded" (Retry-After: 1)
  │
  ├─ 2. Identity rate limits (if credential has an identity)
  │     → 429 "identity rate limit exceeded: {name}" (Retry-After: N)
  │
  ├─ 3. Token request cap (if configured)
  │     → Attempt auto-refill if exhausted
  │     → 429 "token request cap exhausted"
  │
  ├─ 4. Credential request cap (if configured)
  │     → Rollback token cap if credential is exhausted
  │     → Attempt auto-refill if exhausted
  │     → 429 "credential request cap exhausted"
  │
  └─ ✓ Request forwarded to upstream provider
```

## Fail-Open Behavior

If Redis is temporarily unavailable, all Redis-backed rate limits (identity rate limits, token caps, credential caps) fail open — requests are allowed through. This prioritizes availability over strict enforcement during infrastructure issues.

Organization rate limits are in-memory and are not affected by Redis availability.

## Configuration Examples

### Interactive User

Moderate rate limits with burst protection:

```typescript
// Identity with layered rate limits
await vault.identities.create({
  external_id: "user_12345",
  rate_limit: [
    { name: "burst", limit: 10, duration: 1000 },
    { name: "default", limit: 60, duration: 60000 },
  ],
});

// Credential with daily cap and refill
await vault.credentials.create({
  label: "User Key",
  api_key: "sk-...",
  provider_id: "openai",
  identity_id: "user_12345",
  remaining: 1000,
  refill_amount: 1000,
  refill_interval: "24h",
});
```

### Batch Processing Service

High throughput with generous caps:

```typescript
await vault.identities.create({
  external_id: "batch-service",
  rate_limit: [
    { name: "default", limit: 10000, duration: 60000 },
  ],
});

await vault.credentials.create({
  label: "Batch Service Key",
  api_key: "sk-...",
  provider_id: "openai",
  identity_id: "batch-service",
  remaining: 1000000,
  refill_amount: 1000000,
  refill_interval: "24h",
});
```

### Disposable Token for Untrusted Client

Short-lived token with a tight cap, no refill:

```typescript
const { data: token } = await vault.tokens.create({
  credential_id: "cred-uuid",
  expires_in: "15m",
  remaining: 10,
});

// This token can only make 10 requests in 15 minutes, then it's done
```

## Best Practices

1. **Always set identity rate limits** for multi-tenant applications — this prevents any single user from consuming all your quota
2. **Use short-lived tokens with tight caps** when sharing access with untrusted clients
3. **Configure refill on credentials** for long-lived production keys so they don't permanently exhaust
4. **Layer burst + sustained rate limits** on identities for effective traffic shaping
5. **Set organization rate limits** appropriate to your infrastructure capacity and provider quotas
6. **Monitor 429 responses** in your audit log to identify users hitting limits and adjust as needed
