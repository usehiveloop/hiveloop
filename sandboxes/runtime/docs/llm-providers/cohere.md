# Cohere

Configure Bridge to use Cohere's Command models.

---

## Configuration

```json
{
  "provider": {
    "provider_type": "cohere",
    "model": "command-a-03-2025",
    "api_key": "<cohere-api-key>"
  }
}
```

### Fields

| Field | Required | Description |
|-------|----------|-------------|
| `provider_type` | Yes | Must be `cohere` |
| `model` | Yes | Cohere model ID |
| `api_key` | Yes | Your Cohere API key |
| `base_url` | No | Custom endpoint (rarely needed) |

---

## Available Models

| Model | Best For |
|-------|----------|
| `command-a-03-2025` | General purpose, long context |
| `command-r-plus` | Complex tasks, reasoning |
| `command-r` | Balanced performance |
| `command` | Simple tasks |
| `command-light` | Fast, cost-effective |

Check [Cohere's docs](https://docs.cohere.com) for the latest models.

---

## Getting an API Key

1. Sign up at [cohere.com](https://cohere.com)
2. Go to Dashboard → API Keys
3. Create a new key
4. Copy the key

---

## Example Agent

```json
{
  "id": "cohere-assistant",
  "name": "Cohere Assistant",
  "system_prompt": "You are a helpful AI assistant powered by Cohere.",
  "provider": {
    "provider_type": "cohere",
    "model": "command-a-03-2025",
    "api_key": "${COHERE_API_KEY}"
  },
  "tools": ["read", "write"],
  "config": {
    "max_tokens": 4096,
    "temperature": 0.7
  }
}
```

---

## Rate Limits

Cohere enforces rate limits based on your plan:

| Plan | Requests/min | Tokens/min |
|------|--------------|------------|
| Trial | 20 | 100,000 |
| Production | 1,000+ | 100,000+ |

Check your limits in the Cohere dashboard.

---

## Troubleshooting

### 401 Unauthorized

Invalid API key:

```
ERROR: Provider returned 401
```

Fix: Verify your API key in the Cohere dashboard.

### 429 Rate Limited

```
ERROR: Provider returned 429
```

Fix: Wait before retrying or upgrade your plan.

### Model Not Found

```
ERROR: Model 'xyz' not found
```

Fix: Check that you're using a valid Cohere model ID from their documentation.

---

## Compaction with Cohere

For long conversations, use a cheaper model for compaction:

```json
{
  "config": {
    "compaction": {
      "token_budget": 80000,
      "tail_messages": 10,
      "summary_provider": {
        "provider_type": "cohere",
        "model": "command-light",
        "api_key": "<cohere-api-key>"
      }
    }
  }
}
```

---

## See Also

- [Cohere Documentation](https://docs.cohere.com)
- [OpenAI-Compatible Providers](openai-compatible.md) — Other options
