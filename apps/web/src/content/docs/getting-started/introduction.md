---
title: Introduction
description: Overview of LLMVault, its key features, use cases, and architecture.
---

# Introduction to LLMVault

LLMVault is the secure proxy layer for platforms that handle LLM API keys. It enables you to store credentials with envelope encryption, mint scoped tokens for sandboxes, and proxy requests to any LLM provider — all with sub-5ms overhead.

## What is LLMVault?

Every AI-powered platform that supports "Bring Your Own Key" (BYOK) faces the same critical challenge: how do you securely store your customers' LLM API keys, proxy requests without exposing credentials, and give sandboxed agents scoped access — without building months of security infrastructure yourself?

LLMVault handles all of it. Store keys with envelope encryption, mint short-lived tokens for sandboxes, and proxy requests to any LLM provider with minimal latency. Your customers' keys never touch your application code.

## Key Features

### Security Without the Engineering Cost
- **Envelope encryption** using AES-256-GCM with Vault Transit KMS
- **Sealed memory** protection using memguard
- **Multi-tenant isolation** enforced at the database and cache levels
- Your customers' API keys are encrypted at rest, in transit, and in cache

### Sub-5ms Proxy Overhead
- **Three-tier cache architecture**: in-memory → Redis → Postgres/Vault
- Hot path (L1 cache hit): ~0.01ms
- Cold path: 3-8ms
- Your users won't notice the extra hop

### Scoped, Short-Lived Tokens
- Mint JWT tokens scoped to specific credentials
- Configurable TTL from seconds to 24 hours
- Perfect for giving AI agents in sandboxes just enough access, for just long enough
- Instant revocation propagates across all instances via Redis pub/sub

### Any Provider, One Interface
- Abstracts auth scheme differences across providers
- Supports: `bearer`, `x-api-key`, `api-key`, `query_param`
- Works with OpenAI, Anthropic, Google, Fireworks, OpenRouter, and any custom provider

### Instant Revocation
- Redis pub/sub propagates credential and token revocations in sub-millisecond time
- When a customer disconnects their key, it's dead everywhere immediately

## Use Cases

### 1. Bring Your Own Key (BYOK)
Let customers connect their own LLM provider accounts to your platform. Store their credentials securely and proxy requests without ever exposing their API keys to your application code.

### 2. Sandboxed AI Agents
Give AI agents running in sandboxes scoped, short-lived credentials that auto-expire. If a sandbox is compromised, only that token is affected — the blast radius is minimal.

### 3. Multi-Tenant Platforms
Isolate credentials by tenant with org-scoped queries and namespaced Redis keys. Tenant A can never access Tenant B's credentials — enforced at the architecture level.

### 4. Audit and Compliance
Full audit trail of every credential created, revoked, and proxied. Pass enterprise security reviews with built-in encryption, audit trails, and tenant isolation.

## Architecture Overview

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│   Your App      │────▶│   LLMVault      │────▶│   LLM Provider  │
│                 │     │                 │     │                 │
│ Uses scoped     │     │ 1. Validate     │     │ (OpenAI,        │
│ proxy token     │     │    proxy token  │     │  Anthropic,     │
│ (ptok_...)      │     │ 2. Decrypt      │     │  Google, etc.)  │
│                 │     │    credential   │     │                 │
└─────────────────┘     │ 3. Attach auth  │     └─────────────────┘
                        │    headers      │
                        │ 4. Stream       │
                        │    response     │
                        └─────────────────┘
                                 │
                    ┌────────────┼────────────┐
                    ▼            ▼            ▼
               ┌────────┐   ┌────────┐   ┌──────────┐
               │  L1    │   │  L2    │   │   L3     │
               │ Memory │──▶│ Redis  │──▶│ Postgres │
               │(sealed)│   │(enc)   │   │+ Vault   │
               └────────┘   └────────┘   └──────────┘
```

### Three-Tier Cache

| Tier | Storage | Purpose | Latency |
|------|---------|---------|---------|
| L1 | Sealed memory (memguard) | Hot credential cache | ~0.01ms |
| L2 | Redis (encrypted values) | Cross-instance shared cache | ~1ms |
| L3 | Postgres + Vault | Source of truth + KMS | ~3-8ms |

### Authentication Flow

1. **Store**: Customer API key is encrypted with a unique DEK, the DEK is wrapped by Vault Transit KMS, and encrypted blobs are stored
2. **Mint**: Your backend mints a JWT proxy token scoped to that credential
3. **Proxy**: Sandbox uses the token to make requests; LLMVault resolves the real key and attaches the correct auth headers

## Security Model

### Encryption at Rest
- Each credential gets a unique Data Encryption Key (DEK)
- DEKs are encrypted (wrapped) by Vault Transit KMS
- Only encrypted blobs are stored in Postgres

### Encryption in Transit
- All API communications use TLS
- Proxy tokens are signed JWTs

### Encryption in Cache
- Redis stores only encrypted credential values
- Even if Redis is compromised, the attacker gets nothing usable without Vault access

### Memory Protection
- Decrypted keys are stored in sealed memory (memguard)
- Keys are zeroed from memory immediately after use
- Core dumps are disabled in production

## API Base URL

```
https://api.llmvault.dev
```

All API requests should be made to this base URL. The API follows RESTful conventions and returns JSON responses.

## Next Steps

- **[Quickstart](./quickstart)**: Get your first proxied request running in under 15 minutes
- **[Installation](./installation)**: Install the SDK for your language (TypeScript, Python, Go)
- **[Authentication](./authentication)**: Learn about API keys, proxy tokens, and Connect sessions
