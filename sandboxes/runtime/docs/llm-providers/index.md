# LLM Providers

Bridge works with multiple AI providers. This section explains how to configure each.

---

## Provider Types

Bridge supports two types of providers:

### Native Providers

Use each vendor's native API directly:

- Anthropic (Claude)
- Google (Gemini)
- Cohere

Native providers use the vendor's own:
- Authentication method
- Request/response format
- Endpoint URLs

### OpenAI-Compatible Providers

Any provider with a `/chat/completions` endpoint:

- OpenAI
- Groq
- DeepSeek
- Mistral
- xAI
- Together AI
- Fireworks AI
- Ollama
- Any custom provider

These all use the same request format but with different `base_url` values.

---

## Quick Comparison

| Provider | Type | Best For |
|----------|------|----------|
| Anthropic | Native | High-quality reasoning, coding |
| OpenAI | OpenAI-compat | GPT models, broad capabilities |
| Groq | OpenAI-compat | Fast inference, low latency |
| DeepSeek | OpenAI-compat | Cost-effective |
| Cohere | Native | Command models, embeddings |

---

## Configuration

All providers are configured per-agent:

```json
{
  "provider": {
    "provider_type": "anthropic",
    "model": "claude-sonnet-4-20250514",
    "api_key": "<anthropic-api-key>",
    "base_url": "https://api.anthropic.com"
  }
}
```

### Common Fields

| Field | Required | Description |
|-------|----------|-------------|
| `provider_type` | Yes | Which provider to use |
| `model` | Yes | Model identifier |
| `api_key` | Yes | Authentication key |
| `base_url` | Sometimes | Custom endpoint URL |

### Base URL Requirements

| Provider Type | Base URL Required? |
|---------------|-------------------|
| Native | No (uses default) |
| OpenAI-compatible | Yes |

---

## Supported Providers

| Provider | `provider_type` | Notes |
|----------|-----------------|-------|
| Anthropic | `anthropic` | Native |
| Google Gemini | `gemini` | Native |
| Cohere | `cohere` | Native |
| OpenAI | `openai` | Requires `base_url` |
| Groq | `groq` | Requires `base_url` |
| DeepSeek | `deepseek` | Requires `base_url` |
| Mistral | `mistral` | Requires `base_url` |
| xAI | `xai` | Requires `base_url` |
| Together AI | `together` | Requires `base_url` |
| Fireworks AI | `fireworks` | Requires `base_url` |
| Ollama | `ollama` | Requires `base_url` |
| Custom | `custom` | Requires `base_url` |

---

## Read More

- [Anthropic](anthropic.md) — Claude configuration
- [OpenAI-Compatible](openai-compatible.md) — All OpenAI-compatible providers
- [Custom Providers](custom-providers.md) — Bring your own provider
