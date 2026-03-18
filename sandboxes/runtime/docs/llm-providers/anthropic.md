# Anthropic (Claude)

Configure Bridge to use Anthropic's Claude models.

---

## Configuration

```json
{
  "provider": {
    "provider_type": "anthropic",
    "model": "claude-sonnet-4-20250514",
    "api_key": "<anthropic-api-key>"
  }
}
```

### Fields

| Field | Required | Description |
|-------|----------|-------------|
| `provider_type` | Yes | Must be `anthropic` |
| `model` | Yes | Claude model ID |
| `api_key` | Yes | Your Anthropic API key |
| `base_url` | No | Custom endpoint (rarely needed) |

---

## Available Models

| Model | Best For |
|-------|----------|
| `claude-opus-4-20250514` | Complex tasks, coding, analysis |
| `claude-sonnet-4-20250514` | General purpose, balanced |
| `claude-haiku-4-5-20251001` | Fast, cost-effective |

Check [Anthropic's docs](https://docs.anthropic.com) for the latest models.

---

## Getting an API Key

1. Sign up at [console.anthropic.com](https://console.anthropic.com)
2. Go to API Keys
3. Create a new key
4. Copy the key (starts with `sk-ant-`)

---

## Example Agent

```json
{
  "id": "claude-assistant",
  "name": "Claude Assistant",
  "system_prompt": "You are a helpful AI assistant.",
  "provider": {
    "provider_type": "anthropic",
    "model": "claude-sonnet-4-20250514",
    "api_key": "${ANTHROPIC_API_KEY}"
  },
  "tools": ["read", "write"],
  "config": {
    "max_tokens": 4096,
    "temperature": 0.7
  },
  "version": "1"
}
```

---

## Rate Limits

Anthropic enforces rate limits:

| Tier | Requests/min | Tokens/min |
|------|--------------|------------|
| Free | 30 | 50,000 |
| Paid 1 | 3,000 | 400,000 |
| Paid 2 | 4,000 | 800,000 |

Check your limits in the Anthropic console.

---

## Troubleshooting

### 401 Unauthorized

Your API key is invalid or expired:

```
ERROR: Anthropic API returned 401
```

Fix: Generate a new key in the Anthropic console.

### 429 Rate Limited

You've hit the rate limit:

```
ERROR: Anthropic API returned 429
```

Fix: Wait a moment and retry, or upgrade your Anthropic plan.

### 529 Overloaded

Anthropic's servers are busy:

```
ERROR: Anthropic API returned 529
```

Fix: Retry with exponential backoff.

---

## Compaction with Anthropic

For long conversations, use a cheaper model for compaction:

```json
{
  "config": {
    "compaction": {
      "token_budget": 80000,
      "summary_provider": {
        "provider_type": "anthropic",
        "model": "claude-haiku-4-5-20251001",
        "api_key": "<anthropic-api-key>"
      }
    }
  }
}
```

---

## See Also

- [Anthropic Documentation](https://docs.anthropic.com)
- [OpenAI-Compatible Providers](openai-compatible.md) — Other options
