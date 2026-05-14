# LLM Providers

Bridge works with multiple AI providers. This section explains how to configure each.

---

## Provider Types

Bridge supports two types of providers:

### Native Providers

Use each vendor's native API directly:

- **Anthropic** (Claude) - Uses Anthropic's native API
- **Google** (Gemini) - Uses Google's Gemini API
- **Cohere** (Command) - Uses Cohere's native API

Native providers use the vendor's own:
- Authentication method
- Request/response format
- Endpoint URLs (base_url is optional, defaults to vendor's standard URL)

### OpenAI-Compatible Providers

Any provider with a `/chat/completions` endpoint. These providers all use the OpenAI request format but with different `base_url` values:

- **OpenAI** (GPT-4, GPT-3.5)
- **Groq** (Fast inference)
- **DeepSeek** (Cost-effective)
- **Mistral** (European models)
- **xAI** (Grok)
- **Together AI** (Open source models)
- **Fireworks AI** (Model variety)
- **Ollama** (Local models)
- **Custom** (Any OpenAI-compatible endpoint)

---

## Quick Comparison

| Provider | Type | `provider_type` | Best For |
|----------|------|-----------------|----------|
| Anthropic | Native | `anthropic` | High-quality reasoning, coding |
| Google | Native | `google` | Gemini models, multimodal |
| Cohere | Native | `cohere` | Command models, embeddings |
| OpenAI | OpenAI-compat | `open_ai` | GPT models, broad capabilities |
| Groq | OpenAI-compat | `groq` | Fast inference, low latency |
| DeepSeek | OpenAI-compat | `deep_seek` | Cost-effective |
| Mistral | OpenAI-compat | `mistral` | European data residency |
| xAI | OpenAI-compat | `x_ai` | Grok models |
| Together AI | OpenAI-compat | `together` | Open source models |
| Fireworks AI | OpenAI-compat | `fireworks` | Variety of models |
| Ollama | OpenAI-compat | `ollama` | Local/self-hosted models |
| Custom | OpenAI-compat | `custom` | Bring your own endpoint |

---

## Configuration

All providers are configured per-agent:

```json
{
  "provider": {
    "provider_type": "anthropic",
    "model": "claude-sonnet-4-20250514",
    "api_key": "<provider-api-key>",
    "base_url": "https://api.anthropic.com"
  }
}
```

### Common Fields

| Field | Required | Description |
|-------|----------|-------------|
| `provider_type` | Yes | Which provider to use (see table above) |
| `model` | Yes | Model identifier (provider-specific) |
| `api_key` | Yes | Authentication key |
| `base_url` | Sometimes | Custom endpoint URL |

### Base URL Requirements

| Provider Type | Base URL Required? | Default Behavior |
|---------------|-------------------|------------------|
| Native | No | Uses vendor's standard URL |
| OpenAI-compatible | Yes | Must specify provider's URL |

### Provider Type Aliases

The following aliases are accepted when using string parsing:

| Alias | Maps To |
|-------|---------|
| `openai` | `open_ai` |
| `deepseek` | `deep_seek` |
| `xai` | `x_ai` |

---

## Supported Providers Reference

| Provider | `provider_type` | Base URL Required | Notes |
|----------|-----------------|-------------------|-------|
| Anthropic | `anthropic` | No | Native API |
| Google Gemini | `google` | No | Native API |
| Cohere | `cohere` | No | Native API |
| OpenAI | `open_ai` | Yes | Use `https://api.openai.com/v1` |
| Groq | `groq` | Yes | Use `https://api.groq.com/openai/v1` |
| DeepSeek | `deep_seek` | Yes | Use `https://api.deepseek.com/v1` |
| Mistral | `mistral` | Yes | Use `https://api.mistral.ai/v1` |
| xAI | `x_ai` | Yes | Use `https://api.x.ai/v1` |
| Together AI | `together` | Yes | Use `https://api.together.xyz/v1` |
| Fireworks AI | `fireworks` | Yes | Use `https://api.fireworks.ai/inference/v1` |
| Ollama | `ollama` | Yes | Use `http://localhost:11434/v1` |
| Custom | `custom` | Yes | Your custom endpoint |

---

## Timeout Configuration

Bridge applies the following timeouts to provider requests:

| Operation | Timeout |
|-----------|---------|
| Standard agent chat | 180 seconds |
| Foreground subagent | 120 seconds |
| Background subagent | 300 seconds |

These timeouts are fixed and cannot be configured per-agent. If you need longer timeouts, consider using background subagents.

**Note:** When an agent has tools with `require_approval` permission, the timeout is disabled for that turn to allow for indefinite user approval waits.

---

## Rate Limiting

Bridge supports per-agent rate limiting through the `rate_limit_rpm` configuration:

```json
{
  "config": {
    "rate_limit_rpm": 60
  }
}
```

This limits the agent to 60 requests per minute. The rate limit is enforced by the control plane.

---

## Streaming Support

All providers support streaming responses. Bridge uses Server-Sent Events (SSE) to stream:

- `MessageStart` - Response generation started
- `ContentDelta` - Text chunks as they're generated
- `ToolCallStart` - Tool invocation started
- `ToolCallResult` - Tool execution completed
- `MessageEnd` - Response generation completed
- `Done` - Stream complete

Streaming is enabled automatically for all supported providers.

---

## Error Handling

Bridge maps provider errors to standard error codes:

| HTTP Code | Error Code | Description |
|-----------|------------|-------------|
| 401 | `unauthorized` | Invalid API key |
| 429 | `rate_limited` | Provider rate limit exceeded |
| 404 | `agent_not_found` | Model not found |
| 500 | `provider_error` | Provider internal error |
| 408/Timeout | `agent_timeout` | Request timed out |

---

## Provider-Specific Documentation

- [Anthropic](anthropic.md) — Claude configuration
- [Google (Gemini)](google.md) — Gemini models
- [Cohere](cohere.md) — Command models
- [OpenAI-Compatible](openai-compatible.md) — All OpenAI-compatible providers
- [Custom Providers](custom-providers.md) — Bring your own provider

---

## Implementation Details

### How Providers Work

Bridge uses the [rig-core](https://github.com/0xPlaygrounds/rig) library for provider communication:

- **Native providers** (Anthropic, Google, Cohere) use rig's native clients
- **OpenAI-compatible providers** all use rig's OpenAI client with custom `base_url`

### Adding a New Provider

To add support for a new OpenAI-compatible provider:

1. Add the provider type to `ProviderType` enum in `crates/core/src/provider.rs`
2. Add the variant to the OpenAI-compatible match arm in `crates/llm/src/providers.rs`
3. Update this documentation

No code changes are needed for custom providers — just use `provider_type: "custom"`.
