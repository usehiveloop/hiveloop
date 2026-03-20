---
title: Proxy
description: How LLMVault's streaming reverse proxy works, including request routing, three-tier caching, and error handling.
---

# Proxy

The LLMVault proxy is a streaming reverse proxy that sits between your application and upstream LLM providers. It transparently swaps your short-lived token for the real API key, enforces rate limits and request caps, and logs every request — all without your application ever seeing the actual credentials.

## How It Works

```
Your App                LLMVault Proxy               LLM Provider
   │                        │                            │
   ├─ ptok_... ────────────>│                            │
   │                        ├─ Validate token            │
   │                        ├─ Check rate limits         │
   │                        ├─ Resolve credential        │
   │                        ├─ Swap auth ───────────────>│
   │                        │                 sk-... ───>│
   │                        │<──────────── Response ─────┤
   │<─────────── Response ──┤                            │
   │                        ├─ Log to audit trail        │
```

1. Your application sends a request with a `ptok_` token
2. LLMVault validates the token and checks all rate limits
3. The credential is resolved from the cache and the real API key is attached
4. The request is forwarded to the upstream provider
5. The response streams back to your application in real time

## Making Proxy Requests

Use the `/v1/proxy/` endpoint prefix, followed by the path you'd normally send to the provider:

```bash
# Chat completions through the proxy
curl https://api.llmvault.dev/v1/proxy/v1/chat/completions \
  -H "Authorization: Bearer ptok_..." \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'

# Embeddings through the proxy
curl https://api.llmvault.dev/v1/proxy/v1/embeddings \
  -H "Authorization: Bearer ptok_..." \
  -H "Content-Type: application/json" \
  -d '{
    "model": "text-embedding-ada-002",
    "input": "The quick brown fox"
  }'
```

The proxy strips the `/v1/proxy` prefix and appends the rest to the credential's base URL:

```
/v1/proxy/v1/chat/completions → https://api.openai.com/v1/chat/completions
/v1/proxy/v1/embeddings       → https://api.openai.com/v1/embeddings
```

All HTTP methods are supported: `POST`, `GET`, `PUT`, `PATCH`, and `DELETE`.

## Streaming Support

The proxy fully supports Server-Sent Events (SSE) streaming. Chunks are flushed to your application immediately — there's no buffering delay. This means streaming LLM completions work exactly as they would when calling the provider directly.

```bash
curl https://api.llmvault.dev/v1/proxy/v1/chat/completions \
  -H "Authorization: Bearer ptok_..." \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4",
    "messages": [{"role": "user", "content": "Write a poem"}],
    "stream": true
  }'
```

## End-to-End Example

A typical workflow: create a credential, mint a token, and use the proxy.

```typescript
const vault = new LLMVault({ apiKey: "your-api-key" });

// 1. Store your API key as a credential
const { data: credential } = await vault.credentials.create({
  label: "OpenAI Production",
  api_key: "sk-...",
  provider_id: "openai",
});

// 2. Mint a short-lived token
const { data: token } = await vault.tokens.create({
  credential_id: credential.id,
  expires_in: "1h",
});

// 3. Use the token to call OpenAI through the proxy
const response = await fetch(
  "https://api.llmvault.dev/v1/proxy/v1/chat/completions",
  {
    method: "POST",
    headers: {
      Authorization: `Bearer ${token.token}`,
      "Content-Type": "application/json",
    },
    body: JSON.stringify({
      model: "gpt-4",
      messages: [{ role: "user", content: "Hello!" }],
    }),
  }
);
```

## Request Pipeline

Every proxy request passes through these checks in order:

1. **Token authentication** — the `ptok_` JWT is validated for signature and expiry
2. **Organization rate limit** — the org-wide request rate is checked
3. **Identity rate limits** — if the credential is linked to an identity, per-user limits apply
4. **Token request cap** — if the token has a `remaining` cap, it is decremented
5. **Credential request cap** — if the credential has a `remaining` cap, it is decremented
6. **Credential resolution** — the API key is decrypted from the cache
7. **Auth swap** — your token is replaced with the real API key
8. **Upstream forward** — the request is sent to the LLM provider

If any check fails, the request is rejected before reaching the upstream provider.

## Error Handling

The proxy returns structured JSON errors:

| Error | Status Code | Meaning |
|-------|-------------|---------|
| `invalid or expired token` | 401 | Token is expired, revoked, or has an invalid signature |
| `rate limit exceeded` | 429 | Organization rate limit was hit |
| `identity rate limit exceeded: {name}` | 429 | Per-user rate limit was hit |
| `token request cap exhausted` | 429 | Token has used all its allowed requests |
| `credential request cap exhausted` | 429 | Credential has used all its allowed requests |
| `upstream unreachable` | 502 | The LLM provider did not respond |

Rate limit errors include a `Retry-After` header indicating how many seconds to wait before retrying.

## Auth Schemes

The proxy supports multiple ways to authenticate with upstream providers. The auth scheme is configured on the credential and determines how the real API key is attached:

| Scheme | How the Key is Sent |
|--------|---------------------|
| `bearer` | `Authorization: Bearer {api_key}` |
| `x-api-key` | `x-api-key: {api_key}` |
| `api-key` | `api-key: {api_key}` |
| `query_param` | `?key={api_key}` |

## Cross-Instance Cache Invalidation

If you run multiple LLMVault proxy instances, cache invalidation is automatically coordinated. When a credential is revoked or a token is invalidated, all instances are notified immediately via pub/sub and purge their local caches.

## Security Guarantees

- **No key exposure** — your real API keys never leave the proxy; your application only sees tokens
- **Memory protection** — decrypted keys are held in protected memory and wiped immediately after each request
- **SSRF hardening** — upstream URLs are validated against private IP ranges, loopback addresses, link-local addresses, and cloud metadata endpoints
- **Cloud metadata protection** — headers used for cloud instance metadata (AWS, GCP) are automatically stripped from proxied requests
- **Request isolation** — each request resolves its own fresh copy of the credential
- **Full audit trail** — every proxy request is logged with the credential ID, identity, HTTP method, path, status code, and duration
