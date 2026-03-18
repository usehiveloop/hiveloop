# OpenAI-Compatible Providers

Bridge works with any provider that implements the OpenAI API format.

---

## How It Works

OpenAI-compatible providers use the same request/response format as OpenAI:

```
POST /chat/completions
{
  "model": "gpt-4",
  "messages": [...]
}
```

Bridge supports these providers by:
1. Sending requests to your `base_url`
2. Using the OpenAI message format
3. Parsing OpenAI-style responses

---

## Configuration

All OpenAI-compatible providers follow this pattern:

```json
{
  "provider": {
    "provider_type": "openai",
    "model": "model-name",
    "api_key": "<bridge-control-plane-api-key>",
    "base_url": "https://api.provider.com/v1"
  }
}
```

### Required Fields

| Field | Description |
|-------|-------------|
| `provider_type` | Use `openai` for all OpenAI-compatible providers |
| `model` | The model identifier (varies by provider) |
| `api_key` | Your API key for that provider |
| `base_url` | The provider's API endpoint |

---

## Supported Providers

### OpenAI

```json
{
  "provider_type": "openai",
  "model": "gpt-4",
  "api_key": "<provider-api-key>",
  "base_url": "https://api.openai.com/v1"
}
```

### Groq

Fast inference with open source models:

```json
{
  "provider_type": "openai",
  "model": "llama-3.1-70b-versatile",
  "api_key": "gsk_...",
  "base_url": "https://api.groq.com/openai/v1"
}
```

Get a key at [console.groq.com](https://console.groq.com)

### DeepSeek

Cost-effective Chinese and English models:

```json
{
  "provider_type": "openai",
  "model": "deepseek-chat",
  "api_key": "<provider-api-key>",
  "base_url": "https://api.deepseek.com/v1"
}
```

### Mistral

```json
{
  "provider_type": "openai",
  "model": "mistral-large-latest",
  "api_key": "...",
  "base_url": "https://api.mistral.ai/v1"
}
```

### xAI (Grok)

```json
{
  "provider_type": "openai",
  "model": "grok-beta",
  "api_key": "xai-...",
  "base_url": "https://api.x.ai/v1"
}
```

### Together AI

```json
{
  "provider_type": "openai",
  "model": "togethercomputer/llama-2-70b-chat",
  "api_key": "...",
  "base_url": "https://api.together.xyz/v1"
}
```

### Fireworks AI

```json
{
  "provider_type": "openai",
  "model": "accounts/fireworks/models/llama-v3p1-70b-instruct",
  "api_key": "...",
  "base_url": "https://api.fireworks.ai/inference/v1"
}
```

### Ollama (Local)

Run models locally:

```json
{
  "provider_type": "openai",
  "model": "llama3.1",
  "api_key": "ollama",
  "base_url": "http://localhost:11434/v1"
}
```

---

## Finding Your Base URL

Check your provider's documentation. Common patterns:

| Provider | Base URL Pattern |
|----------|------------------|
| OpenAI | `https://api.openai.com/v1` |
| Groq | `https://api.groq.com/openai/v1` |
| DeepSeek | `https://api.deepseek.com/v1` |
| Mistral | `https://api.mistral.ai/v1` |
| xAI | `https://api.x.ai/v1` |
| Fireworks | `https://api.fireworks.ai/inference/v1` |
| Ollama | `http://localhost:11434/v1` |

---

## Testing Your Configuration

Test with curl before pushing to Bridge:

```bash
curl https://api.groq.com/openai/v1/chat/completions \
  -H "Authorization: Bearer $GROQ_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama-3.1-70b-versatile",
    "messages": [{"role": "user", "content": "Hello"}]
  }'
```

If this works, your Bridge configuration should work too.

---

## Troubleshooting

### 404 Not Found

Wrong base URL:

```
ERROR: Provider returned 404
```

Fix: Check the provider's documentation for the correct endpoint.

### 401 Unauthorized

Invalid API key:

```
ERROR: Provider returned 401
```

Fix: Verify your key is correct and active.

### Model Not Found

```
ERROR: Model 'xyz' not found
```

Fix: Check the provider's model list. Names are case-sensitive.

---

## Provider Comparison

| Provider | Speed | Cost | Best For |
|----------|-------|------|----------|
| Groq | Very fast | Low | Low latency apps |
| DeepSeek | Fast | Very low | Budget-conscious |
| Mistral | Fast | Low | European data residency |
| Fireworks | Fast | Medium | Variety of models |
| Together | Medium | Medium | Open source models |

---

## See Also

- [Anthropic](anthropic.md) — Native Claude support
- [Custom Providers](custom-providers.md) — Bring your own
