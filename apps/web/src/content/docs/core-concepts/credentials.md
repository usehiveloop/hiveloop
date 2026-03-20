---
title: Credentials
description: Secure storage and lifecycle management of LLM API credentials with envelope encryption, auto-detection, and request caps.
---

# Credentials

Credentials are the foundation of LLMVault's security model. Each credential stores an encrypted LLM provider API key alongside access controls, request caps, and lifecycle management — so your real keys never touch your application code.

## What is a Credential?

A credential represents a stored LLM provider API key. When you create a credential, LLMVault encrypts the API key using envelope encryption and stores it securely. From that point on, you interact with the credential through short-lived tokens — your actual API key is never exposed again.

A credential contains:

- **Encrypted API key** — protected using AES-256-GCM envelope encryption
- **Provider** — which LLM provider this key belongs to (e.g., `openai`, `anthropic`)
- **Base URL** — the upstream provider endpoint
- **Auth scheme** — how to authenticate with the provider
- **Label** — a human-readable name for identification
- **Identity** — optional link to an identity for per-user rate limiting
- **Request caps** — optional limits on API call volume with automatic refill
- **Metadata** — custom key-value pairs for tagging and filtering

## Creating a Credential

```typescript
const vault = new LLMVault({ apiKey: "your-api-key" });

const { data, error } = await vault.credentials.create({
  label: "Production OpenAI",
  api_key: "sk-...",
  provider_id: "openai",
  base_url: "https://api.openai.com/v1",
  auth_scheme: "bearer",
  meta: { environment: "production", team: "ai" },
});
```

Or with curl:

```bash
curl -X POST https://api.llmvault.dev/v1/credentials \
  -H "Authorization: Bearer {org_token}" \
  -H "Content-Type: application/json" \
  -d '{
    "label": "Production OpenAI",
    "api_key": "sk-...",
    "provider_id": "openai",
    "base_url": "https://api.openai.com/v1",
    "auth_scheme": "bearer",
    "meta": {"environment": "production", "team": "ai"}
  }'
```

### Required and Optional Fields

| Field | Required | Description |
|-------|----------|-------------|
| `label` | Yes | A human-readable name for the credential |
| `api_key` | Yes | The LLM provider API key to encrypt and store |
| `provider_id` | No | Provider identifier (e.g., `openai`, `anthropic`). Validated against the built-in catalog. |
| `base_url` | No | The upstream provider endpoint URL |
| `auth_scheme` | No | How to attach the API key to requests (see Auth Schemes below) |
| `identity_id` | No | Link to an existing identity for rate limiting |
| `meta` | No | Custom JSON metadata for tagging and filtering |

### Auto-Creating Identities

If you pass an `external_id` when creating a credential, LLMVault will automatically look up (or create) an identity with that external ID and link it to the credential:

```typescript
const { data, error } = await vault.credentials.create({
  label: "User API Key",
  api_key: "sk-...",
  provider_id: "openai",
  identity_id: "user_12345",
});
```

This is a convenient shorthand — you don't need to create the identity separately.

## Envelope Encryption

LLMVault uses a two-layer envelope encryption scheme to protect your API keys:

```
┌──────────────────────────────────────────────────┐
│  Your API Key                                     │
│  Encrypted with a unique Data Encryption Key      │
│  (AES-256-GCM)                                    │
├──────────────────────────────────────────────────┤
│  Data Encryption Key (DEK)                        │
│  Encrypted by your KMS                            │
│  (AWS KMS, AEAD, or Vault Transit)                │
└──────────────────────────────────────────────────┘
```

**How it works:**

1. Each credential gets its own unique 256-bit encryption key (DEK)
2. Your API key is encrypted locally using AES-256-GCM with the DEK
3. The DEK itself is encrypted by your configured Key Management System (KMS)
4. Both ciphertexts are stored — your plaintext API key is never persisted

This means even if the database is compromised, an attacker would need access to your KMS to decrypt any API key.

### Supported KMS Backends

| Backend | Use Case |
|---------|----------|
| **AEAD** | Local AES-256-GCM for development and single-node deployments |
| **AWS KMS** | Production AWS deployments |
| **Vault Transit** | HashiCorp Vault for enterprise environments |

## Auth Schemes

The auth scheme determines how LLMVault attaches your API key when proxying requests to the upstream provider:

| Scheme | How the Key is Sent |
|--------|---------------------|
| `bearer` | `Authorization: Bearer {api_key}` |
| `x-api-key` | `x-api-key: {api_key}` |
| `api-key` | `api-key: {api_key}` |
| `query_param` | `?key={api_key}` |

Most providers use `bearer`. Azure OpenAI uses `api-key`. Some Google services use `query_param`.

## Request Caps

Credentials support optional request caps that limit how many API calls can be made. Caps automatically refill on a schedule you define.

```typescript
const { data, error } = await vault.credentials.create({
  label: "Rate-Limited Key",
  api_key: "sk-...",
  provider_id: "openai",
  remaining: 10000,
  refill_amount: 10000,
  refill_interval: "24h",
});
```

| Field | Description |
|-------|-------------|
| `remaining` | Current request budget. Decremented with each proxied request. |
| `refill_amount` | How many requests to restore when the interval elapses |
| `refill_interval` | How often to refill (e.g., `1h`, `24h`, `168h` for weekly) |

When the cap is exhausted, LLMVault checks whether enough time has passed since the last refill. If so, the counter is automatically reset to `refill_amount`. This happens transparently — you don't need to trigger refills manually.

If the cap is exhausted and no refill is due, requests return a `429` status code:

```json
{"error": "credential request cap exhausted"}
```

## Credential Caching

LLMVault resolves credentials through a high-performance three-tier cache to minimize latency on every proxied request:

| Tier | Typical Latency | Description |
|------|-----------------|-------------|
| In-memory | ~0.01ms | Decrypted credentials held in protected memory |
| Redis | ~0.5ms | Encrypted credentials cached for fast retrieval |
| Database + KMS | ~3-8ms | Full decryption path from persistent storage |

Most requests resolve from the in-memory cache. You don't need to configure or manage the cache — it's fully automatic.

## Listing and Filtering

```typescript
// List all credentials
const { data, error } = await vault.credentials.list();

// Filter by identity
const { data, error } = await vault.credentials.list({
  identity_id: "identity-uuid",
});

// Filter by external ID
const { data, error } = await vault.credentials.list({
  external_id: "user_12345",
});

// Filter by metadata
const { data, error } = await vault.credentials.list({
  meta: { team: "ai" },
});

// Paginate
const { data, error } = await vault.credentials.list({
  limit: 50,
  cursor: "eyJpZCI6...",
});
```

## Retrieving a Credential

```typescript
const { data, error } = await vault.credentials.get("credential-uuid");
```

The response includes all credential metadata but never returns the decrypted API key. The only way to use the key is through the proxy with a valid token.

## Revoking a Credential

```typescript
const { data, error } = await vault.credentials.delete("credential-uuid");
```

Revocation is immediate and takes effect across all proxy instances:

- All active tokens tied to this credential become invalid
- Cached copies of the credential are purged
- Subsequent proxy requests using this credential's tokens return `401`

Revocation is a soft delete — audit log entries referencing this credential are preserved.

## Security Guarantees

- **Encryption at rest** — API keys are encrypted with AES-256-GCM and never stored in plaintext
- **Key isolation** — every credential gets its own unique encryption key
- **Memory protection** — decrypted keys are held in protected memory regions and wiped immediately after use
- **No key exposure** — your API key is returned once at creation time and never again through any API endpoint
- **SSRF protection** — base URLs are validated to prevent requests to internal networks
- **Audit trail** — every proxy request through a credential is logged for compliance
