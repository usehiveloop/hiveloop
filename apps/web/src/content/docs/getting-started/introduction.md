---
title: Introduction
description: Overview of LLMVault — secure credential proxy and controlled third-party integrations for AI agents.
---

# Introduction to LLMVault

LLMVault is the infrastructure layer for AI platforms that need to securely handle credentials and third-party integrations. Store LLM API keys with envelope encryption, proxy requests to any provider with sub-5ms overhead, and give your AI agents fully controlled access to services like Slack, GitHub, and Google Drive — without building months of security and integration infrastructure yourself.

## What is LLMVault?

AI platforms face two hard infrastructure problems:

1. **Credential security** — How do you store your customers' LLM API keys, proxy requests without exposing them, and give sandboxed agents scoped access?
2. **Controlled integrations** — How do you let AI agents interact with third-party services like Slack, Google Drive, or Notion on behalf of your users — with granular, user-approved permissions?

LLMVault solves both. It's a credential vault and an integration gateway, purpose-built for platforms where AI agents need secure, scoped access to external services.

## Key Features

### Secure Credential Vault
- **Envelope encryption** for all stored API keys — encrypted at rest, in transit, and in cache
- **Multi-tenant isolation** enforced at every layer
- **Scoped, short-lived tokens** — mint proxy tokens with configurable TTL for sandboxed agents
- **Instant revocation** — credential and token revocations propagate in sub-millisecond time
- Your customers' keys never touch your application code

### Controlled Integrations for AI Agents
- **200+ third-party integrations** — Slack, GitHub, Google Drive, Notion, Salesforce, and more via OAuth
- **Resource-level permissions** — users choose exactly which channels, repositories, or documents an agent can access
- **Pre-built Connect widget** — drop-in UI for your users to authorize integrations and select resources
- **Token management handled for you** — OAuth token storage, refresh, and revocation managed automatically
- Your agents get scoped access tokens — never raw OAuth credentials

### Sub-5ms Proxy Overhead
- **Three-tier caching** keeps hot credentials fast
- Hot path: ~0.01ms, cold path: 3-8ms
- Your users won't notice the extra hop

### Any LLM Provider, One Interface
- Abstracts auth scheme differences across providers
- Supports: `bearer`, `x-api-key`, `api-key`, `query_param`
- Works with OpenAI, Anthropic, Google, Fireworks, OpenRouter, and any custom provider

## Use Cases

### 1. Bring Your Own Key (BYOK)
Let customers connect their own LLM provider accounts to your platform. Store their credentials securely and proxy requests without ever exposing their API keys to your application code.

### 2. AI Agents with Third-Party Access
Your AI agents need to read Slack messages, create GitHub issues, or search Google Drive — but only the specific channels, repos, or folders the user has approved. LLMVault handles the OAuth flows, resource selection, and token lifecycle so your agents get exactly the access they need, nothing more.

### 3. Sandboxed AI Agents
Give AI agents running in sandboxes scoped, short-lived credentials that auto-expire. If a sandbox is compromised, only that token is affected — the blast radius is minimal.

### 4. Multi-Tenant Platforms
Isolate credentials and integrations by tenant with org-scoped access control. Tenant A can never access Tenant B's credentials or connections — enforced at the architecture level.

### 5. Audit and Compliance
Full audit trail of every credential created, revoked, and proxied. Pass enterprise security reviews with built-in encryption, audit trails, and tenant isolation.

## How It Works

### Credential Proxy

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
```

1. **Store**: Your customer's API key is encrypted and stored securely in LLMVault
2. **Mint**: Your backend mints a scoped proxy token tied to that credential
3. **Proxy**: Your app (or sandbox) uses the token to make requests — LLMVault resolves the real key and attaches the correct auth headers

### Integration Gateway

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│   Your User     │────▶│   LLMVault      │────▶│   Third-Party   │
│                 │     │   Connect       │     │   Service       │
│ Authorizes via  │     │                 │     │                 │
│ Connect widget  │     │ 1. OAuth flow   │     │ (Slack, GitHub, │
│                 │     │ 2. User selects │     │  Google Drive,  │
└─────────────────┘     │    resources    │     │  Notion, etc.)  │
                        │ 3. Store tokens │     │                 │
┌─────────────────┐     │ 4. Issue scoped │     └─────────────────┘
│   Your AI Agent │────▶│    access       │────▶│                 │
│                 │     │                 │     │                 │
│ Requests access │     │ 5. Retrieve     │     │                 │
│ token           │     │    token        │     │                 │
└─────────────────┘     └─────────────────┘     └─────────────────┘
```

1. **Authorize**: Your user opens the Connect widget and authorizes an integration (e.g., Slack)
2. **Select resources**: The user chooses exactly which channels, repos, or files the agent can access
3. **Agent requests access**: Your agent retrieves a scoped access token from LLMVault for that connection
4. **Controlled access**: The agent interacts with the third-party service using only the permissions the user granted

### Security Model

- **At rest**: Every credential is encrypted using envelope encryption with an external KMS (AWS KMS or HashiCorp Vault in production)
- **In transit**: All API communications use TLS
- **In cache**: Cached credentials remain encrypted — even if the cache is compromised, the data is unusable without KMS access
- **Tokens**: Proxy tokens are signed JWTs with configurable TTL and usage limits
- **OAuth tokens**: Stored and refreshed automatically — never exposed to your frontend or agent code

## API Base URL

```
https://api.llmvault.dev
```

All API requests should be made to this base URL. The API follows RESTful conventions and returns JSON responses.

## Next Steps

- **[Quickstart](./quickstart)**: Get your first proxied request running in under 15 minutes
- **[Installation](./installation)**: Install the SDK for your language (TypeScript, Python, Go)
- **[Authentication](./authentication)**: Learn about API keys, proxy tokens, and Connect sessions
