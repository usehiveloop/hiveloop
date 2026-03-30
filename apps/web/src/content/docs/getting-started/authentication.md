---
title: Authentication
description: Understanding LLMVault's authentication layers — API keys, proxy tokens, and Connect sessions.
---

# Authentication

LLMVault uses three distinct authentication mechanisms for different use cases:

1. **API Keys** — For management operations (storing credentials, minting tokens)
2. **Proxy Tokens** — For making proxied LLM requests from sandboxes
3. **Connect Sessions** — For frontend widget authentication

## API Keys

API keys are used to manage your organization's credentials, tokens, and settings. They are long-lived credentials that should be kept secure on your backend.

### Format

```
llmv_sk_<64-character-hex>
```

Example: `llmv_sk_a1b2c3d4e5f6789012345678901234567890abcdef...`

### Creating API Keys

```bash
curl -X POST https://api.llmvault.dev/v1/api-keys \
  -H "Authorization: Bearer YOUR_AUTH_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Production Backend",
    "scopes": ["credentials", "tokens", "connect"],
    "expires_in": "720h"
  }'
```

Response:
```json
{
  "id": "key_abc123",
  "key": "llmv_sk_a1b2c3d4...",
  "key_prefix": "llmv_sk_a1b2c3d4",
  "name": "Production Backend",
  "scopes": ["credentials", "tokens", "connect"],
  "expires_at": "2026-04-20T10:00:00Z",
  "created_at": "2026-03-20T10:00:00Z"
}
```

**Important**: The `key` is only shown once. Store it securely.

### Scopes

API keys can be scoped to limit their permissions:

| Scope | Permissions |
|-------|-------------|
| `credentials` | Create, list, get, revoke credentials |
| `tokens` | Mint, list, revoke proxy tokens |
| `connect` | Create Connect sessions |
| `integrations` | Manage integrations and connections |
| `all` | Full access (identities, settings, etc.) |

### Using API Keys

Include the API key in the `Authorization` header:

```bash
curl https://api.llmvault.dev/v1/credentials \
  -H "Authorization: Bearer llmv_sk_a1b2c3d4..."
```

### Revoking API Keys

```bash
curl -X DELETE https://api.llmvault.dev/v1/api-keys/key_abc123 \
  -H "Authorization: Bearer llmv_sk_a1b2c3d4..."
```

Revocation is immediate and propagates to all instances via Redis pub/sub.

## Proxy Tokens

Proxy tokens are short-lived JWTs used to make LLM requests through the proxy. They are scoped to a single credential and can have usage limits.

### Format

```
ptok_<jwt-token>
```

Example: `ptok_eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...`

### Minting Proxy Tokens

Use your API key to mint a proxy token:

```bash
curl -X POST https://api.llmvault.dev/v1/tokens \
  -H "Authorization: Bearer llmv_sk_a1b2c3d4..." \
  -H "Content-Type: application/json" \
  -d '{
    "credential_id": "cred_550e8400-e29b-41d4-a716-446655440000",
    "ttl": "1h",
    "remaining": 1000,
    "refill_amount": 100,
    "refill_interval": "1h"
  }'
```

Response:
```json
{
  "token": "ptok_eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "expires_at": "2026-03-20T11:05:00Z",
  "jti": "token-jti-identifier"
}
```

### Token Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `credential_id` | string (required) | UUID of the credential to scope to |
| `ttl` | string | Token lifetime (default: `1h`, max: `24h`). Examples: `15m`, `1h`, `24h` |
| `remaining` | integer | Optional request cap |
| `refill_amount` | integer | Amount to refill at each interval |
| `refill_interval` | string | Refill interval (e.g., `1h`, `24h`) |
| `meta` | object | Optional metadata attached to the token |

### Using Proxy Tokens

Use the proxy token to make requests to the LLM provider through LLMVault:

```bash
curl -X POST https://api.llmvault.dev/v1/proxy/v1/chat/completions \
  -H "Authorization: Bearer ptok_eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..." \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

The proxy token is validated, the real credential is resolved from encrypted storage, and the request is forwarded to the upstream provider with the correct authentication headers.

### Token Validation

When a proxy token is presented:

1. The JWT signature is verified
2. The token is checked against the database (not revoked)
3. Usage limits are checked (if configured)
4. Identity rate limits are applied (if linked)
5. The credential is resolved and decrypted
6. The request is forwarded to the upstream provider

### Revoking Tokens

```bash
curl -X DELETE https://api.llmvault.dev/v1/tokens/{jti} \
  -H "Authorization: Bearer llmv_sk_a1b2c3d4..."
```

Replace `{jti}` with the token's JTI (JSON Token Identifier). Revocation is immediate across all instances.

### Token Lifecycle

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   Create    │────▶│    Use      │────▶│   Expire    │────▶│   Cleanup   │
│  (1 API call)│     │ (0-N requests)│    │ (TTL reached)│    │ (async)    │
└─────────────┘     └─────────────┘     └─────────────┘     └─────────────┘
       │
       ▼
┌─────────────┐
│   Revoke    │ (optional, immediate)
└─────────────┘
```

## Connect Sessions

Connect sessions are short-lived tokens used by the LLMVault Connect widget to authenticate frontend users. They are designed for browser-based authentication flows.

### Format

```
csess_<session-token>
```

Example: `csess_abc123def456...`

### Creating Connect Sessions

Create a Connect session from your backend:

```bash
curl -X POST https://api.llmvault.dev/v1/connect/sessions \
  -H "Authorization: Bearer llmv_sk_a1b2c3d4..." \
  -H "Content-Type: application/json" \
  -d '{
    "external_id": "user_12345",
    "allowed_integrations": ["slack", "github"],
    "allowed_origins": ["https://app.example.com"],
    "permissions": ["create", "list"],
    "ttl": "15m"
  }'
```

Response:
```json
{
  "id": "sess_abc123",
  "session_token": "csess_abc123def456...",
  "external_id": "user_12345",
  "allowed_integrations": ["slack", "github"],
  "allowed_origins": ["https://app.example.com"],
  "expires_at": "2026-03-20T10:20:00Z",
  "created_at": "2026-03-20T10:05:00Z"
}
```

### Session Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `identity_id` | string | Existing identity UUID (optional) |
| `external_id` | string | Your system's user identifier (optional if `identity_id` provided) |
| `allowed_integrations` | string[] | Limit which integrations can be connected |
| `allowed_origins` | string[] | Allowed CORS origins for this session |
| `permissions` | string[] | Permissions: `create`, `list`, `delete`, `verify` |
| `metadata` | object | Optional metadata |
| `ttl` | string | Session lifetime (default: `15m`, max: `30m`) |

### Using Connect Sessions

Pass the session token to the frontend SDK:

```typescript
import { LLMVaultConnect } from '@llmvault/frontend';

const connect = new LLMVaultConnect({
  baseURL: 'https://connect.llmvault.dev',
  theme: 'system'
});

connect.open({
  sessionToken: 'csess_abc123def456...',
  onSuccess: (payload) => {
    console.log('Connected:', payload);
  },
  onError: (error) => {
    console.error('Error:', error);
  }
});
```

### Connect Widget API

The Connect widget uses the session token to call the widget API at `https://api.llmvault.dev/v1/widget/*`:

| Endpoint | Description |
|----------|-------------|
| `GET /v1/widget/session` | Get current session info |
| `GET /v1/widget/providers` | List available providers |
| `GET /v1/widget/integrations` | List integrations |
| `POST /v1/widget/connections` | Create a direct connection |
| `POST /v1/widget/integrations/{id}/connections` | Create an integration connection |
| `DELETE /v1/widget/connections/{id}` | Delete a connection |

### Session Security

Connect sessions include several security features:

- **Origin validation**: Requests must come from `allowed_origins`
- **Short lifetime**: Max 30 minutes, default 15 minutes
- **Activation tracking**: First use marks the session as activated
- **One-time link to identity**: Sessions are linked to an identity for audit trails

## Authentication Flow Summary

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         MANAGEMENT OPERATIONS                           │
│  Your Backend ─────────────────────────────────────▶ LLMVault API       │
│       │                                               │                  │
│       │  Authorization: Bearer llmv_sk_...            │                  │
│       │  (API Key - long-lived, backend-only)         │                  │
│       │                                               │                  │
│       │◀──────────────────────────────────────────────│                  │
│       │                                               │                  │
│       ▼                                               ▼                  │
│  ┌─────────────┐                              ┌─────────────┐            │
│  │ Store Cred  │                              │ Mint Token  │            │
│  │ Create Conn │                              │ Get Usage   │            │
│  │ Revoke Key  │                              │ List Audit  │            │
│  └─────────────┘                              └─────────────┘            │
└─────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────┐
│                         PROXY OPERATIONS                                │
│  Sandbox / Agent ──────────────────────────────────▶ LLMVault Proxy     │
│       │                                               │                  │
│       │  Authorization: Bearer ptok_...               │                  │
│       │  (Proxy Token - short-lived, scoped)          │                  │
│       │                                               │                  │
│       │◀──────────────────────────────────────────────│                  │
│       │         (Streamed LLM response)               │                  │
│       │                                               ▼                  │
│       │                                       ┌─────────────┐            │
│       │                                       │ Resolve Key │            │
│       │                                       │ Decrypt     │            │
│       │                                       │ Forward     │            │
│       │                                       │ Stream Back │            │
│       │                                       └─────────────┘            │
└─────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────┐
│                         CONNECT WIDGET                                  │
│  Browser ──────────────────────────────────────────▶ Widget API         │
│       │                                               │                  │
│       │  Authorization: Bearer csess_...              │                  │
│       │  (Session Token - very short-lived)           │                  │
│       │                                               │                  │
│       │◀──────────────────────────────────────────────│                  │
│       │                                               │                  │
│       │  Origin header validated against                │                  │
│       │  session's allowed_origins                      │                  │
│       │                                               ▼                  │
│       │                                       ┌─────────────┐            │
│       │                                       │ Connect     │            │
│       │                                       │ Provider    │            │
│       │                                       │ Select Res. │            │
│       │                                       │ Create Conn │            │
│       │                                       └─────────────┘            │
└─────────────────────────────────────────────────────────────────────────┘
```

## Security Best Practices

### API Keys

- **Store securely**: Use environment variables or secret managers
- **Rotate regularly**: Set expiration when creating keys
- **Use minimal scopes**: Only grant the permissions needed
- **Never expose to frontend**: API keys are backend-only

### Proxy Tokens

- **Use short TTLs**: Default to 1 hour or less
- **Set usage limits**: Use `remaining` to cap requests
- **Implement token refresh**: Your backend should rotate tokens
- **Monitor usage**: Check audit logs for unexpected patterns

### Connect Sessions

- **Validate origins**: Always set `allowed_origins` to your domain
- **Use short TTLs**: 15 minutes is usually sufficient
- **Link to identity**: Use `external_id` or `identity_id` for audit trails
- **Check onSuccess payload**: Verify the connection before using it

## Error Handling

### API Key Errors

```json
{ "error": "invalid api key" }
{ "error": "api key expired" }
{ "error": "api key lacks required scope: credentials" }
```

### Proxy Token Errors

```json
{ "error": "invalid or expired token" }
{ "error": "token has been revoked" }
{ "error": "token not found" }
{ "error": "request cap exceeded" }
```

### Connect Session Errors

```json
{ "error": "invalid session token" }
{ "error": "session expired" }
{ "error": "origin not allowed" }
{ "error": "session_token_missing" }
```

## Next Steps

- Review the [Quickstart](./quickstart) for a complete walkthrough
- Learn about [Identity Management](/docs/guides/identities) for multi-tenant use cases
- Explore [Audit Logging](/docs/guides/audit) for compliance requirements
