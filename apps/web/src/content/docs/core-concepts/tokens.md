---
title: Tokens
description: Short-lived proxy tokens with JWT-based authentication, TTL constraints, scope-based access control, and MCP endpoint support.
---

# Tokens

Tokens are short-lived keys that grant scoped access to your stored credentials. Instead of sharing your LLM API key directly, you mint a token that provides temporary, constrained access — with built-in expiry, request caps, and scope restrictions.

## Why Tokens?

Tokens solve the problem of sharing LLM access safely:

- **Time-limited** — tokens expire automatically (default: 1 hour, max: 24 hours)
- **Scoped** — restrict which actions and models a token can access
- **Capped** — limit total requests a token can make
- **Revocable** — instantly invalidate a token before it expires
- **Auditable** — every request made with a token is logged

## Creating a Token

```typescript
const vault = new LLMVault({ apiKey: "your-api-key" });

const { data, error } = await vault.tokens.create({
  credential_id: "550e8400-e29b-41d4-a716-446655440000",
  name: "batch-processing",
  expires_in: "4h",
  scopes: [
    {
      connection_id: "conn-uuid",
      actions: ["chat.completions", "embeddings"],
      resources: {
        models: ["gpt-4", "gpt-3.5-turbo"],
      },
    },
  ],
});
```

Or with curl:

```bash
curl -X POST https://api.llmvault.dev/v1/tokens \
  -H "Authorization: Bearer {org_token}" \
  -H "Content-Type: application/json" \
  -d '{
    "credential_id": "550e8400-e29b-41d4-a716-446655440000",
    "ttl": "4h",
    "scopes": [
      {
        "connection_id": "conn-uuid",
        "actions": ["chat.completions", "embeddings"],
        "resources": {
          "models": ["gpt-4", "gpt-3.5-turbo"]
        }
      }
    ]
  }'
```

### Response

```json
{
  "token": "ptok_eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "expires_at": "2024-01-15T18:30:00Z",
  "jti": "550e8400-e29b-41d4-a716-446655440001",
  "mcp_endpoint": "https://mcp.llmvault.dev/550e8400-e29b-41d4-a716-446655440001"
}
```

The `token` value is the only time the full token string is returned. Store it securely — it cannot be retrieved again.

## Token Format

LLMVault tokens are prefixed with `ptok_` for easy identification:

```
ptok_eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
```

Under the hood, tokens are signed JWTs containing the organization ID, credential ID, and expiry. The `ptok_` prefix is stripped automatically during authentication.

## TTL (Time to Live)

| Setting | Value |
|---------|-------|
| Default TTL | 1 hour |
| Maximum TTL | 24 hours |

Valid TTL formats:

- `15m` — 15 minutes
- `1h` — 1 hour
- `4h30m` — 4 hours 30 minutes
- `24h` — maximum allowed

If you request a TTL longer than 24 hours, the API returns an error:

```json
{"error": "ttl exceeds maximum of 24h"}
```

## Using a Token

Pass the token in the `Authorization` header when making proxy requests:

```bash
curl https://api.llmvault.dev/v1/proxy/v1/chat/completions \
  -H "Authorization: Bearer ptok_eyJhbGciOi..." \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

LLMVault validates the token, resolves the linked credential, and forwards the request to the upstream provider with the real API key — all transparently.

## Scopes

Scopes let you restrict what a token can do. This is especially useful for MCP (Model Context Protocol) integrations where you want to limit access to specific actions and models.

```typescript
const { data, error } = await vault.tokens.create({
  credential_id: "cred-uuid",
  scopes: [
    {
      connection_id: "conn-uuid",
      actions: ["chat.completions", "embeddings"],
      resources: {
        models: ["gpt-4", "gpt-3.5-turbo"],
      },
    },
  ],
});
```

| Field | Description |
|-------|-------------|
| `connection_id` | The integration connection this scope applies to |
| `actions` | Explicit list of allowed actions (wildcards are not permitted) |
| `resources` | Optional mapping of resource types to allowed IDs |

**Scope validation rules:**

- The connection must exist and belong to your organization
- Each action must be a recognized action for the provider
- Wildcard actions (`*`) are rejected — you must list each action explicitly
- Resource types must match the actions' expected resource types

When scopes are provided, the token response includes an MCP endpoint URL for Model Context Protocol connections.

## Request Caps

Like credentials, tokens can have request caps with optional automatic refill:

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
| `remaining` | Total requests this token can make |
| `refill_amount` | Requests to restore when the interval elapses |
| `refill_interval` | How often to refill (e.g., `1h`) |

When a token's cap is exhausted, LLMVault automatically checks if a refill is due. If not, the request returns `429`:

```json
{"error": "token request cap exhausted"}
```

Token counters are automatically cleaned up when the token expires.

## Revoking a Token

Revoke a token before it expires:

```typescript
const { data, error } = await vault.tokens.delete("token-jti");
```

Or with curl:

```bash
curl -X DELETE https://api.llmvault.dev/v1/tokens/{jti} \
  -H "Authorization: Bearer {org_token}"
```

Revocation takes effect immediately across all proxy instances. Any subsequent request using the revoked token returns `401`.

## Listing Tokens

```typescript
// List all tokens
const { data, error } = await vault.tokens.list();

// Filter by credential
const { data, error } = await vault.tokens.list({
  credential_id: "cred-uuid",
});

// Paginate
const { data, error } = await vault.tokens.list({
  limit: 50,
  cursor: "eyJpZCI6...",
});
```

### Response

```json
{
  "data": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "jti": "token-jti-uuid",
      "credential_id": "cred-uuid",
      "remaining": 95,
      "refill_amount": 100,
      "refill_interval": "1h",
      "scopes": [...],
      "meta": {"purpose": "batch-processing"},
      "expires_at": "2024-01-15T18:30:00Z",
      "created_at": "2024-01-15T14:30:00Z"
    }
  ],
  "has_more": true,
  "next_cursor": "eyJpZCI6..."
}
```

## Security Guarantees

- **Short lifetime** — tokens last at most 24 hours, defaulting to 1 hour
- **Cryptographic signing** — tokens are HMAC-SHA256 signed JWTs that cannot be forged or tampered with
- **Unique identification** — every token has a UUID-based JTI that prevents replay attacks
- **Instant revocation** — revoked tokens are rejected immediately, even if they haven't expired
- **Scope integrity** — scope restrictions are cryptographically bound to the token and cannot be modified after creation
