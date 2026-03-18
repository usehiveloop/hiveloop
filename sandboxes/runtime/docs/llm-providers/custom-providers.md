# Custom Providers

Use any OpenAI-compatible API as a provider.

---

## Requirements

Your custom provider must:

1. Accept POST requests to `/chat/completions`
2. Accept OpenAI-format request bodies
3. Return OpenAI-format responses
4. Support Server-Sent Events for streaming (optional but recommended)

---

## Request Format

Bridge sends:

```json
{
  "model": "your-model",
  "messages": [
    {"role": "system", "content": "You are a helpful assistant."},
    {"role": "user", "content": "Hello"}
  ],
  "temperature": 0.7,
  "max_tokens": 1024,
  "stream": true
}
```

### Expected Response (Non-Streaming)

```json
{
  "choices": [{
    "message": {
      "role": "assistant",
      "content": "Hello! How can I help you?"
    },
    "finish_reason": "stop"
  }],
  "usage": {
    "prompt_tokens": 20,
    "completion_tokens": 10
  }
}
```

### Expected Response (Streaming)

Server-Sent Events:

```
data: {"choices": [{"delta": {"content": "Hello"}}]}

data: {"choices": [{"delta": {"content": " there"}}]}

data: {"choices": [{"delta": {}, "finish_reason": "stop"}]}

data: [DONE]
```

---

## Configuration

```json
{
  "provider": {
    "provider_type": "custom",
    "model": "my-model",
    "api_key": "your-key",
    "base_url": "https://api.yourprovider.com/v1"
  }
}
```

---

## Example: Proxy to Another Bridge

You can chain Bridge instances:

```json
{
  "id": "edge-bridge",
  "provider": {
    "provider_type": "custom",
    "model": "claude-sonnet",
    "api_key": "internal-key",
    "base_url": "https://internal-bridge.company.com/v1"
  }
}
```

---

## Example: Self-Hosted Model

Use vLLM or text-generation-inference:

```json
{
  "id": "local-llm",
  "provider": {
    "provider_type": "custom",
    "model": "meta-llama/Llama-2-70b-chat-hf",
    "api_key": "not-needed",
    "base_url": "http://localhost:8000/v1"
  }
}
```

---

## Building a Provider

Simple Node.js example:

```javascript
const express = require('express');
const app = express();

app.post('/v1/chat/completions', express.json(), async (req, res) => {
  const { model, messages, stream } = req.body;
  
  if (stream) {
    // Streaming response
    res.setHeader('Content-Type', 'text/event-stream');
    
    const response = await callYourLLM(messages);
    
    for (const chunk of response.chunks) {
      res.write(`data: ${JSON.stringify({
        choices: [{ delta: { content: chunk } }]
      })}\n\n`);
    }
    
    res.write('data: [DONE]\n\n');
    res.end();
  } else {
    // Non-streaming response
    const response = await callYourLLM(messages);
    
    res.json({
      choices: [{
        message: {
          role: 'assistant',
          content: response.text
        },
        finish_reason: 'stop'
      }],
      usage: {
        prompt_tokens: response.inputTokens,
        completion_tokens: response.outputTokens
      }
    });
  }
});

app.listen(8000);
```

---

## Testing Your Provider

Test with curl before using with Bridge:

```bash
# Test non-streaming
curl http://localhost:8000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "test",
    "messages": [{"role": "user", "content": "Hello"}]
  }'

# Test streaming
curl http://localhost:8000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "test",
    "messages": [{"role": "user", "content": "Hello"}],
    "stream": true
  }'
```

---

## Authentication

Bridge sends your `api_key` in the Authorization header:

```
Authorization: Bearer {api_key}
```

Your provider should validate this header.

---

## Error Handling

Return standard HTTP error codes:

| Code | When |
|------|------|
| 400 | Bad request (invalid JSON) |
| 401 | Invalid API key |
| 404 | Model not found |
| 429 | Rate limited |
| 500 | Internal error |

Error response format:

```json
{
  "error": {
    "message": "Invalid API key",
    "type": "authentication_error"
  }
}
```

---

## See Also

- [OpenAI-Compatible Providers](openai-compatible.md) — Existing providers
- [OpenAI API Reference](https://platform.openai.com/docs/api-reference) — Format specification
