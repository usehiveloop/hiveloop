# Google (Gemini)

Configure Bridge to use Google's Gemini models.

---

## Configuration

```json
{
  "provider": {
    "provider_type": "google",
    "model": "gemini-2.5-flash",
    "api_key": "<gemini-api-key>"
  }
}
```

### Fields

| Field | Required | Description |
|-------|----------|-------------|
| `provider_type` | Yes | Must be `google` |
| `model` | Yes | Gemini model ID |
| `api_key` | Yes | Your Google AI Studio API key |
| `base_url` | No | Custom endpoint (rarely needed) |

---

## Available Models

| Model | Best For |
|-------|----------|
| `gemini-2.5-flash` | Fast, efficient tasks |
| `gemini-2.5-pro` | Complex reasoning, coding |
| `gemini-2.0-flash` | Balanced performance |

Check [Google's documentation](https://ai.google.dev/models) for the latest models.

---

## Getting an API Key

1. Go to [Google AI Studio](https://aistudio.google.com/app/apikey)
2. Create a new API key
3. Copy the key

---

## Example Agent

```json
{
  "id": "gemini-assistant",
  "name": "Gemini Assistant",
  "system_prompt": "You are a helpful AI assistant powered by Gemini.",
  "provider": {
    "provider_type": "google",
    "model": "gemini-2.5-flash",
    "api_key": "${GEMINI_API_KEY}"
  },
  "tools": ["read", "write"],
  "config": {
    "max_tokens": 4096,
    "temperature": 0.7
  }
}
```

---

## Schema Handling

Gemini's API has strict schema requirements. Bridge automatically:

- Ensures every property has a valid `type` field
- Inlines `$ref` references
- Removes unsupported schema keywords
- Defaults missing types to `"string"`

This ensures tool schemas work correctly with Gemini models.

---

## Rate Limits

Google enforces rate limits based on your tier:

| Tier | Requests/min | Tokens/min |
|------|--------------|------------|
| Free | 60 | 1,000,000 |
| Pay-as-you-go | Higher | Higher |

Check [Google's rate limits](https://ai.google.dev/pricing) for current limits.

---

## Troubleshooting

### 400 Bad Request - Schema Error

Gemini is strict about JSON schemas:

```
ERROR: Provider returned 400 - Invalid schema
```

Fix: Bridge automatically flattens schemas for Gemini. If you see this error, check that your tool schemas have proper types for all properties.

### 401 Unauthorized

Invalid API key:

```
ERROR: Provider returned 401
```

Fix: Check that your API key is valid and has not expired.

### 429 Rate Limited

```
ERROR: Provider returned 429
```

Fix: Wait before retrying or upgrade your Google AI Studio plan.

---

## Long-running conversations with Gemini

Bridge's immortal mode keeps history bounded with **in-place forgecode-style compaction** — pure code, no extra LLM call. Set `config.immortal` and choose a budget appropriate for the model context.

```json
{
  "config": {
    "immortal": {
      "token_budget": 800000,
      "retention_window": 20,
      "eviction_window": 0.5,
      "expose_journal_tools": true
    }
  }
}
```

See [Immortal Mode](../core-concepts/agents.md#immortal-mode) for the full configuration.

---

## See Also

- [Google AI Studio Documentation](https://ai.google.dev/gemini-api/docs)
- [OpenAI-Compatible Providers](openai-compatible.md) — Other options
