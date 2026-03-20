---
title: Providers
description: The embedded LLM provider catalog with auto-detection, model metadata, and pricing information sourced from models.dev.
---

# Providers

LLMVault includes a built-in catalog of LLM providers and their models, sourced from [models.dev](https://models.dev). The catalog gives you instant access to provider metadata, model capabilities, pricing, and token limits — without any external API calls.

## Why a Built-In Catalog?

The provider catalog helps you:

- **Browse available providers and models** before creating credentials
- **Validate provider IDs** when storing credentials
- **Look up model capabilities** like context window size, pricing, and supported modalities
- **Build provider selection UIs** using the discovery API endpoints

The catalog is embedded in LLMVault and served with zero latency — no external network calls required.

## Supported Providers

The catalog includes all major LLM providers:

| Provider ID | Name |
|-------------|------|
| `openai` | OpenAI |
| `anthropic` | Anthropic |
| `google` | Google AI |
| `cohere` | Cohere |
| `mistral` | Mistral AI |
| `groq` | Groq |
| `together` | Together AI |
| `fireworks` | Fireworks AI |
| `perplexity` | Perplexity |
| `azure_openai` | Azure OpenAI |
| `aws_bedrock` | AWS Bedrock |

And many more — the catalog covers 100+ providers and 3,000+ models.

## Browsing Providers

### List All Providers

```typescript
const vault = new LLMVault({ apiKey: "your-api-key" });

const { data, error } = await vault.providers.list();
```

Response:

```json
[
  {
    "id": "openai",
    "name": "OpenAI",
    "api": "https://platform.openai.com/docs/api-reference",
    "doc": "https://platform.openai.com/docs",
    "model_count": 12
  },
  {
    "id": "anthropic",
    "name": "Anthropic",
    "api": "https://docs.anthropic.com/claude/reference",
    "doc": "https://docs.anthropic.com",
    "model_count": 5
  }
]
```

### Get Provider Details

```typescript
const { data, error } = await vault.providers.get("openai");
```

Returns the provider along with its full model list.

### List Models for a Provider

```typescript
const { data, error } = await vault.providers.listModels("openai");
```

Response:

```json
[
  {
    "id": "gpt-4o",
    "name": "GPT-4o",
    "family": "gpt-4o",
    "reasoning": false,
    "tool_call": true,
    "structured_output": true,
    "open_weights": false,
    "knowledge": "2023-10",
    "release_date": "2024-05-13",
    "modalities": {
      "input": ["text", "image"],
      "output": ["text"]
    },
    "cost": {
      "input": 5.0,
      "output": 15.0
    },
    "limit": {
      "context": 128000,
      "output": 4096
    },
    "status": "active"
  }
]
```

These endpoints are public and do not require authentication — they're designed for building provider selection UIs.

## Model Metadata

Each model in the catalog includes rich metadata:

### Capabilities

| Field | Description |
|-------|-------------|
| `reasoning` | Supports chain-of-thought reasoning |
| `tool_call` | Can call external tools and functions |
| `structured_output` | Supports JSON mode and structured outputs |
| `open_weights` | Model weights are openly available |

### Pricing

Costs are expressed as price per 1 million tokens:

| Field | Description |
|-------|-------------|
| `cost.input` | Price per 1M input tokens (USD) |
| `cost.output` | Price per 1M output tokens (USD) |

### Token Limits

| Field | Description |
|-------|-------------|
| `limit.context` | Maximum context window size (tokens) |
| `limit.output` | Maximum output length (tokens) |

### Modalities

Describes what types of input and output the model supports:

- **Input**: `text`, `image`, `audio`, `video`
- **Output**: `text`, `image`, `audio`

### Status

| Status | Description |
|--------|-------------|
| `active` | Currently available and recommended |
| `deprecated` | Scheduled for removal — migrate to a newer model |
| `preview` | Beta or early access — may change without notice |

### Knowledge Cutoff and Release Date

- `knowledge` — training data cutoff (e.g., `2023-10` means October 2023)
- `release_date` — when the model was released (ISO 8601 format, e.g., `2024-05-13`)

## Using Providers with Credentials

When creating a credential, specify the `provider_id` to link it to a catalog entry:

```typescript
const { data, error } = await vault.credentials.create({
  label: "OpenAI Production",
  api_key: "sk-...",
  provider_id: "openai",
});
```

The provider ID is validated against the catalog. If you pass an unrecognized ID, the request is rejected:

```json
{"error": "unknown provider_id \"invalid-provider\""}
```

## Custom and Compatible Providers

You can use any OpenAI-compatible endpoint by specifying a `base_url` alongside a recognized `provider_id`:

```typescript
const { data, error } = await vault.credentials.create({
  label: "Local LLM",
  api_key: "my-key",
  provider_id: "openai",
  base_url: "https://api.mycustomllm.com/v1",
  auth_scheme: "bearer",
});
```

This works well for self-hosted models, custom gateways, or any provider that implements the OpenAI API format.

## Model Selection at Request Time

When making proxy requests, pass the model ID directly in your request body — it's forwarded transparently to the upstream provider:

```bash
curl https://api.llmvault.dev/v1/proxy/v1/chat/completions \
  -H "Authorization: Bearer ptok_..." \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

LLMVault does not modify or validate the model ID in proxy requests — it's passed through exactly as you send it.
