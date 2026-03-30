---
title: Quickstart
description: Step-by-step guide to get started with LLMVault — get an API key, store a credential, mint a token, and proxy a request.
---

# Quickstart

Get your first proxied LLM request running in under 15 minutes. This guide walks you through the complete flow: creating an API key, storing a credential, minting a proxy token, and making a proxied request.

## Prerequisites

- An LLMVault account (sign up at [llmvault.dev](https://llmvault.dev))
- An API key from an LLM provider (OpenAI, Anthropic, or any supported provider)
- `curl` or any HTTP client

## Step 1: Get an API Key

First, create an API key for your organization. This key is used to manage credentials and mint proxy tokens.

```bash
curl -X POST https://api.llmvault.dev/v1/api-keys \
  -H "Authorization: Bearer YOUR_AUTH_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Production API Key",
    "scopes": ["credentials", "tokens"],
    "expires_in": "720h"
  }'
```

Response:
```json
{
  "id": "key_abc123",
  "key": "llmv_sk_a1b2c3d4e5f6789012345678...",
  "key_prefix": "llmv_sk_a1b2c3d4",
  "name": "Production API Key",
  "scopes": ["credentials", "tokens"],
  "expires_at": "2026-04-20T10:00:00Z",
  "created_at": "2026-03-20T10:00:00Z"
}
```

**Important**: Save the `key` value immediately. It is only shown once. Use this key for all subsequent management API calls.

## Step 2: Store a Credential

Store your customer's (or your own) LLM API key in LLMVault. The key is encrypted automatically with envelope encryption.

```bash
curl -X POST https://api.llmvault.dev/v1/credentials \
  -H "Authorization: Bearer llmv_sk_a1b2c3d4..." \
  -H "Content-Type: application/json" \
  -d '{
    "label": "acme_corp_openai",
    "provider_id": "openai",
    "base_url": "https://api.openai.com",
    "auth_scheme": "bearer",
    "api_key": "sk-openai-api-key-here"
  }'
```

Response:
```json
{
  "id": "cred_550e8400-e29b-41d4-a716-446655440000",
  "label": "acme_corp_openai",
  "provider_id": "openai",
  "base_url": "https://api.openai.com",
  "auth_scheme": "bearer",
  "created_at": "2026-03-20T10:05:00Z",
  "request_count": 0
}
```

Save the `id` field — this is your credential ID.

### Supported Auth Schemes

| Scheme | Header/Format | Provider Example |
|--------|---------------|------------------|
| `bearer` | `Authorization: Bearer <key>` | OpenAI, OpenRouter |
| `x-api-key` | `x-api-key: <key>` | Anthropic |
| `api-key` | `api-key: <key>` | Some Azure deployments |
| `query_param` | `?key=<key>` | Some Google APIs |

## Step 3: Mint a Proxy Token

Create a short-lived JWT token scoped to your credential. This token is what you'll use in sandboxed environments.

```bash
curl -X POST https://api.llmvault.dev/v1/tokens \
  -H "Authorization: Bearer llmv_sk_a1b2c3d4..." \
  -H "Content-Type: application/json" \
  -d '{
    "credential_id": "cred_550e8400-e29b-41d4-a716-446655440000",
    "ttl": "1h"
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

The `token` (prefixed with `ptok_`) is your proxy token. Save it for the next step.

### Token Options

You can configure additional options when minting tokens:

```bash
curl -X POST https://api.llmvault.dev/v1/tokens \
  -H "Authorization: Bearer llmv_sk_a1b2c3d4..." \
  -H "Content-Type: application/json" \
  -d '{
    "credential_id": "cred_550e8400-e29b-41d4-a716-446655440000",
    "ttl": "24h",
    "remaining": 1000,
    "refill_amount": 100,
    "refill_interval": "1h"
  }'
```

| Field | Description |
|-------|-------------|
| `ttl` | Token lifetime (max 24h). Examples: `15m`, `1h`, `24h` |
| `remaining` | Optional request cap for this token |
| `refill_amount` | Amount to refill at each interval |
| `refill_interval` | Refill interval (e.g., `1h`, `24h`) |

## Step 4: Proxy a Request

Now use your proxy token to make LLM API requests through LLMVault.

```bash
curl -X POST https://api.llmvault.dev/v1/proxy/v1/chat/completions \
  -H "Authorization: Bearer ptok_eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..." \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4",
    "messages": [{"role": "user", "content": "Hello, world!"}]
  }'
```

The response is streamed directly from the LLM provider through LLMVault. Your sandbox never sees the real API key.

### Proxy Path Format

```
POST /v1/proxy/{upstream-path}
```

The path after `/v1/proxy/` is forwarded to the upstream provider. Examples:

| Provider | LLMVault Path | Upstream Path |
|----------|---------------|---------------|
| OpenAI | `/v1/proxy/v1/chat/completions` | `/v1/chat/completions` |
| Anthropic | `/v1/proxy/v1/messages` | `/v1/messages` |

### Streaming Support

LLMVault supports SSE streaming with immediate flush (no buffering):

```bash
curl -X POST https://api.llmvault.dev/v1/proxy/v1/chat/completions \
  -H "Authorization: Bearer ptok_..." \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4",
    "messages": [{"role": "user", "content": "Count from 1 to 5"}],
    "stream": true
  }'
```

## Full Example in JavaScript

Here's the complete flow using the LLMVault SDK:

```typescript
import { LLMVault } from '@llmvault/sdk';

const vault = new LLMVault({
  apiKey: 'llmv_sk_a1b2c3d4...'
});

async function quickstart() {
  // 1. Store a credential
  const { data: credential } = await vault.credentials.create({
    label: 'my_openai_key',
    provider_id: 'openai',
    base_url: 'https://api.openai.com',
    auth_scheme: 'bearer',
    api_key: 'sk-openai-api-key-here'
  });

  // 2. Mint a proxy token
  const { data: token } = await vault.tokens.create({
    credential_id: credential.id,
    ttl: '1h'
  });

  // 3. Use the token to proxy requests
  const response = await fetch('https://api.llmvault.dev/v1/proxy/v1/chat/completions', {
    method: 'POST',
    headers: {
      'Authorization': `Bearer ${token.token}`,
      'Content-Type': 'application/json'
    },
    body: JSON.stringify({
      model: 'gpt-4',
      messages: [{ role: 'user', content: 'Hello!' }]
    })
  });

  const result = await response.json();
  console.log(result.choices[0].message.content);
}

quickstart();
```

## Next Steps

- Learn about [token revocation](./authentication#revoking-tokens) for instant access removal
- Set up the [SDK for your language](./installation)
- Explore [Connect sessions](./authentication#connect-sessions) for frontend integrations
- Review the [API reference](/docs/api) for all endpoints
