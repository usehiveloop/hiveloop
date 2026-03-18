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

## Configuration Pattern

All OpenAI-compatible providers follow this pattern:

```json
{
  "provider": {
    "provider_type": "<provider-specific-type>",
    "model": "model-name",
    "api_key": "<bridge-control-plane-api-key>",
    "base_url": "https://api.provider.com/v1"
  }
}
```

**Important:** Each provider has its own `provider_type` value (see table below). Do not use `"openai"` for all providers.

### Required Fields

| Field | Description |
|-------|-------------|
| `provider_type` | Provider-specific type (e.g., `groq`, `deep_seek`, `mistral`) |
| `model` | The model identifier (varies by provider) |
| `api_key` | Your API key for that provider |
| `base_url` | The provider's API endpoint (required) |

---

## Provider Type Values

| Provider | `provider_type` | Base URL |
|----------|-----------------|----------|
| OpenAI | `open_ai` | `https://api.openai.com/v1` |
| Groq | `groq` | `https://api.groq.com/openai/v1` |
| DeepSeek | `deep_seek` | `https://api.deepseek.com/v1` |
| Mistral | `mistral` | `https://api.mistral.ai/v1` |
| xAI | `x_ai` | `https://api.x.ai/v1` |
| Together AI | `together` | `https://api.together.xyz/v1` |
| Fireworks AI | `fireworks` | `https://api.fireworks.ai/inference/v1` |
| Ollama | `ollama` | `http://localhost:11434/v1` |
| Custom | `custom` | Your URL |

---

## Supported Providers

### OpenAI

```json
{
  "provider_type": "open_ai",
  "model": "gpt-4o",
  "api_key": "<provider-api-key>",
  "base_url": "https://api.openai.com/v1"
}
```

### Groq

Fast inference with open source models:

```json
{
  "provider_type": "groq",
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
  "provider_type": "deep_seek",
  "model": "deepseek-chat",
  "api_key": "<provider-api-key>",
  "base_url": "https://api.deepseek.com/v1"
}
```

**Alias:** You can also use `deepseek` (without underscore) as the provider type.

### Mistral

```json
{
  "provider_type": "mistral",
  "model": "mistral-large-latest",
  "api_key": "...",
  "base_url": "https://api.mistral.ai/v1"
}
```

### xAI (Grok)

```json
{
  "provider_type": "x_ai",
  "model": "grok-beta",
  "api_key": "xai-...",
  "base_url": "https://api.x.ai/v1"
}
```

**Alias:** You can also use `xai` (without underscore) as the provider type.

### Together AI

```json
{
  "provider_type": "together",
  "model": "togethercomputer/llama-2-70b-chat",
  "api_key": "...",
  "base_url": "https://api.together.xyz/v1"
}
```

### Fireworks AI

```json
{
  "provider_type": "fireworks",
  "model": "accounts/fireworks/models/llama-v3p1-70b-instruct",
  "api_key": "...",
  "base_url": "https://api.fireworks.ai/inference/v1"
}
```

### Ollama (Local)

Run models locally:

```json
{
  "provider_type": "ollama",
  "model": "llama3.1",
  "api_key": "ollama",
  "base_url": "http://localhost:11434/v1"
}
```

**Note:** The `api_key` can be any non-empty string for Ollama (e.g., `"ollama"` or `"not-needed"`).

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

Fix: Check the provider's documentation for the correct endpoint. Common issues:
- Missing `/v1` suffix
- Wrong path (e.g., `/chat/completions` is added automatically)

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

### Missing Base URL

```
ERROR: provider 'groq' requires base_url to be set in the agent definition
```

Fix: All OpenAI-compatible providers require a `base_url` in the configuration.

---

## Provider Comparison

| Provider | Speed | Cost | Best For |
|----------|-------|------|----------|
| Groq | Very fast | Low | Low latency apps |
| DeepSeek | Fast | Very low | Budget-conscious |
| Mistral | Fast | Low | European data residency |
| Fireworks | Fast | Medium | Variety of models |
| Together | Medium | Medium | Open source models |
| Ollama | Varies | Free (local) | Privacy, no API costs |

---

## See Also

- [Anthropic](anthropic.md) — Native Claude support
- [Custom Providers](custom-providers.md) — Bring your own
