---
title: Provider Connections
description: Learn about LLM provider connections, supported providers, API key validation, and managing provider credentials in Connect.
---

# Provider Connections

Connect supports 20+ LLM providers, handling API key input, validation, and secure storage. This guide covers the provider connection flow, supported providers, and credential management.

## Connection Flow

```
┌─────────────────────────────────────────────────────────────┐
│                    Provider Connection Flow                  │
│                                                              │
│  1. Provider Selection                                       │
│     ├── Popular providers (OpenAI, Anthropic, Google)       │
│     └── Searchable list of all providers                     │
│                        │                                     │
│                        ▼                                     │
│  2. API Key Input                                            │
│     ├── Secure password field                               │
│     ├── Optional label (e.g., "Production")                 │
│     └── Provider documentation link                         │
│                        │                                     │
│                        ▼                                     │
│  3. Validation                                               │
│     ├── API key verified with provider                      │
│     ├── Encryption at rest                                  │
│     └── Credential stored                                   │
│                        │                                     │
│                        ▼                                     │
│  4. Success                                                  │
│     └── Connection ID returned via event                    │
└─────────────────────────────────────────────────────────────┘
```

### Step 1: Provider Selection

Users see a searchable list of available providers:

```
┌──────────────────────────────┐
│  Connect a provider     [X]  │
├──────────────────────────────┤
│  [Search providers...]       │
├──────────────────────────────┤
│  Popular                     │
│  [OpenAI] [Anthropic] [Google]│
├──────────────────────────────┤
│  All Providers               │
│  ┌────┬───────────────────┐  │
│  │ ◆  │ OpenAI     74 models│  │
│  ├────┼───────────────────┤  │
│  │ ◇  │ Anthropic  12 models│  │
│  ├────┼───────────────────┤  │
│  │ ◎  │ Google Gemini ... │  │
│  └────┴───────────────────┘  │
└──────────────────────────────┘
```

### Step 2: API Key Input

After selecting a provider, users enter their API key:

```
┌──────────────────────────────┐
│  [←]  [OpenAI Logo]  OpenAI [X]│
├──────────────────────────────┤
│                              │
│  API Key                     │
│  ┌─────────────────────────┐ │
│  │ sk-••••••••••••••••  👁 │ │
│  └─────────────────────────┘ │
│  Find your API key at        │
│  platform.openai.com         │
│                              │
│  Label — optional            │
│  ┌─────────────────────────┐ │
│  │ Production key          │ │
│  └─────────────────────────┘ │
│                              │
│  🔒 End-to-end encrypted     │
│                              │
│  ┌─────────────────────────┐ │
│  │        Connect          │ │
│  └─────────────────────────┘ │
└──────────────────────────────┘
```

### Step 3: Validation

While the API key is being validated:

```
┌──────────────────────────────┐
│                              │
│         ⟳ Validating         │
│         your credentials...  │
│                              │
│         OpenAI               │
│                              │
└──────────────────────────────┘
```

### Step 4: Success

After successful connection:

```
┌──────────────────────────────┐
│                              │
│           ✓                  │
│      Successfully            │
│      connected to OpenAI     │
│                              │
│   Connection ID: conn_xxx    │
│                              │
│  ┌─────────────────────────┐ │
│  │         Done            │ │
│  └─────────────────────────┘ │
└──────────────────────────────┘
```

## Supported Providers

Connect supports the following LLM providers with automatic base URL and auth scheme detection:

### Major Providers

| Provider | ID | Auth Scheme | Base URL |
|----------|-----|-------------|----------|
| OpenAI | `openai` | Bearer | `https://api.openai.com` |
| Anthropic | `anthropic` | X-API-Key | `https://api.anthropic.com` |
| Google Gemini | `google` | Query Param | `https://generativelanguage.googleapis.com` |
| Mistral | `mistral` | Bearer | `https://api.mistral.ai` |
| Cohere | `cohere` | Bearer | `https://api.cohere.com` |

### High-Performance Providers

| Provider | ID | Auth Scheme | Base URL |
|----------|-----|-------------|----------|
| Groq | `groq` | Bearer | `https://api.groq.com/openai` |
| DeepSeek | `deepseek` | Bearer | `https://api.deepseek.com` |
| Perplexity | `perplexity` | Bearer | `https://api.perplexity.ai` |
| Together AI | `togetherai` | Bearer | `https://api.together.xyz` |
| Cerebras | `cerebras` | Bearer | `https://api.cerebras.ai` |

### Enterprise/Cloud Providers

| Provider | ID | Auth Scheme | Base URL |
|----------|-----|-------------|----------|
| Azure OpenAI | `azure` | API-Key | `https://models.inference.ai.azure.com` |
| AWS Bedrock | `amazon-bedrock` | Bearer | `https://bedrock-runtime.us-east-1.amazonaws.com` |
| Google Vertex | `google-vertex` | Bearer | `https://us-central1-aiplatform.googleapis.com` |
| Vertex Anthropic | `google-vertex-anthropic` | Bearer | `https://us-central1-aiplatform.googleapis.com` |

### Specialized Providers

| Provider | ID | Auth Scheme | Base URL |
|----------|-----|-------------|----------|
| xAI | `xai` | Bearer | `https://api.x.ai` |
| Venice AI | `venice` | Bearer | `https://api.venice.ai/api` |
| Vercel AI | `vercel` | Bearer | `https://ai-gateway.vercel.sh` |
| v0 | `v0` | Bearer | `https://api.v0.dev` |
| Deep Infra | `deepinfra` | Bearer | `https://api.deepinfra.com/v1` |
| GitLab | `gitlab` | Bearer | `https://gitlab.com/api/v4/code_suggestions` |
| Cloudflare AI | `cloudflare-ai-gateway` | Bearer | `https://gateway.ai.cloudflare.com` |
| Azure Cognitive | `azure-cognitive-services` | API-Key | `https://eastus.api.cognitive.microsoft.com` |
| SAP AI Core | `sap-ai-core` | Bearer | `https://api.ai.prod.eu-central-1.aws.ml.hana.ondemand.com` |

### Auth Schemes Explained

| Scheme | Header Format | Used By |
|--------|---------------|---------|
| `bearer` | `Authorization: Bearer {key}` | Most providers |
| `x-api-key` | `X-API-Key: {key}` | Anthropic |
| `api-key` | `API-Key: {key}` | Azure |
| `query_param` | `?key={key}` | Google Gemini |

## API Key Validation

When a user submits their API key, Connect validates it with the provider:

### Validation Process

1. **Key format check**: Verify the key matches expected pattern
2. **Live validation**: Make test request to provider
3. **Error handling**: Return specific error messages
4. **Storage**: Encrypt and store only if valid

### Validation Endpoint

```http
POST /v1/widget/connections
Authorization: Bearer {session_token}
Content-Type: application/json

{
  "provider_id": "openai",
  "api_key": "sk-...",
  "label": "Production"
}
```

### Validation Errors

```json
// Invalid key format
{
  "error": "api key verification failed: invalid key format"
}

// Key rejected by provider
{
  "error": "api key verification failed: authentication failed"
}

// Unknown provider
{
  "error": "unknown provider: invalid-provider"
}

// No base URL configured
{
  "error": "no known base URL for provider: custom-provider"
}
```

### Validation Response

Success returns the created connection:

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "label": "Production",
  "provider_id": "openai",
  "provider_name": "OpenAI",
  "base_url": "https://api.openai.com",
  "auth_scheme": "bearer",
  "created_at": "2026-03-20T08:45:00Z"
}
```

## Managing Connections

### List Connections

Users can view their existing provider connections:

```http
GET /v1/widget/connections?limit=50
Authorization: Bearer {session_token}
```

**Response:**

```json
{
  "data": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "label": "Production",
      "provider_id": "openai",
      "provider_name": "OpenAI",
      "base_url": "https://api.openai.com",
      "auth_scheme": "bearer",
      "created_at": "2026-03-20T08:45:00Z"
    }
  ],
  "has_more": false
}
```

### View Connection Details

Tap a connection to see details:

```
┌──────────────────────────────┐
│  [←]  OpenAI  Production [X] │
├──────────────────────────────┤
│                              │
│  Provider     OpenAI        │
│  Created      Mar 20, 2026  │
│  Endpoint     api.openai.com│
│                              │
│  ┌─────────────────────────┐ │
│  │    Verify Connection    │ │
│  └─────────────────────────┘ │
│                              │
│  ┌─────────────────────────┐ │
│  │    Revoke Access        │ │
│  └─────────────────────────┘ │
└──────────────────────────────┘
```

### Verify Connection

Test that a stored connection is still valid:

```http
POST /v1/widget/connections/{id}/verify
Authorization: Bearer {session_token}
```

**Response:**

```json
{
  "valid": true,
  "provider": "openai",
  "models": ["gpt-4", "gpt-3.5-turbo"]
}
```

Or if invalid:

```json
{
  "valid": false,
  "error": "invalid_api_key"
}
```

### Revoke Connection

Remove a connection (soft delete):

```http
DELETE /v1/widget/connections/{id}
Authorization: Bearer {session_token}
```

**Response:**

```json
{
  "status": "deleted"
}
```

After revocation, the API key is no longer accessible and cannot be used for proxy requests.

## Connection Security

### Encryption at Rest

All API keys are encrypted using industry-standard envelope encryption before being stored. Each credential is encrypted with a unique key, which is itself protected by your configured key management service (e.g., AWS KMS). Keys are never stored in plaintext.

### Permission Requirements

| Action | Required Permission |
|--------|---------------------|
| Create connection | `create` |
| List connections | `list` |
| Verify connection | `verify` |
| Delete connection | `delete` |

## Widget API Reference

### Get Session Info

```http
GET /v1/widget/session
Authorization: Bearer {session_token}
```

### List Providers

```http
GET /v1/widget/providers
Authorization: Bearer {session_token}
```

Returns all available LLM providers from the registry.

### List Connections

```http
GET /v1/widget/connections?limit=50&cursor=xxx
Authorization: Bearer {session_token}
```

Supports cursor pagination for users with many connections.

### Create Connection

```http
POST /v1/widget/connections
Authorization: Bearer {session_token}
Content-Type: application/json

{
  "provider_id": "openai",
  "api_key": "sk-...",
  "label": "Production API Key"
}
```

### Delete Connection

```http
DELETE /v1/widget/connections/{connection_id}
Authorization: Bearer {session_token}
```

### Verify Connection

```http
POST /v1/widget/connections/{connection_id}/verify
Authorization: Bearer {session_token}
```

## Custom Providers

If you need a provider not listed above, you can:

1. **Contact support**: Request addition to the provider registry
2. **Use direct integration**: Create credentials via the REST API
3. **Self-host**: Add providers to your own registry

## Provider Logos

Each provider is displayed with its official brand color in the Connect widget:

| Provider | Brand Color |
|----------|-------------|
| OpenAI | `#0D0D0D` |
| Anthropic | `#D4A373` |
| Google Gemini | `#4285F4` |
| Mistral | `#4A90D9` |
| Groq | `#E8734A` |
| DeepSeek | `#4A6CF7` |
| Cohere | `#39594D` |
| Perplexity | `#6366F1` |

These colors are used automatically and cannot be overridden.

## Next Steps

- [Sessions](./sessions) — Create sessions for provider connections
- [Embedding](./embedding) — Embed the provider selection flow
- [Integrations](./integrations) — Configure OAuth integrations
- [Frontend SDK](./frontend-sdk) — SDK reference
