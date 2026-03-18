# Quickstart

Let's run your first agent. This tutorial takes about 5 minutes once Bridge is built.

**Prerequisites:**
- Bridge built from source (see [Installation](installation.md)) — first build takes 5-10 minutes
- An API key from an AI provider (Anthropic, OpenAI, etc.)

---

## Step 1: Start Bridge

You'll need an API key from an AI provider. This example uses Anthropic, but you can use any supported provider.

```bash
export BRIDGE_CONTROL_PLANE_API_KEY="<bridge-control-plane-api-key>"
export ANTHROPIC_API_KEY="<anthropic-api-key>"

./target/release/bridge
```

You should see:

```
INFO  bridge > Starting server on 0.0.0.0:8080
```

Bridge is now running and waiting for agents.

---

## Step 2: Push Your First Agent

In another terminal, create an agent:

```bash
curl -X POST http://localhost:8080/push/agents \
  -H "Authorization: Bearer <bridge-control-plane-api-key>" \
  -H "Content-Type: application/json" \
  -d '{
    "agents": [{
      "id": "greeter",
      "name": "Friendly Greeter",
      "system_prompt": "You are a friendly assistant. Greet users warmly and answer their questions helpfully.",
      "provider": {
        "provider_type": "anthropic",
        "model": "claude-sonnet-4-20250514",
        "api_key": "'"$ANTHROPIC_API_KEY"'"
      },
      "tools": [],
      "skills": [],
      "integrations": [],
      "config": {
        "max_tokens": 1024,
        "max_turns": 10,
        "temperature": 0.7
      },
      "version": "1"
    }]
  }'
```

You should get:

```json
{"loaded": 1}
```

---

## Step 3: Start a Conversation

Create a conversation:

```bash
curl -X POST http://localhost:8080/agents/greeter/conversations \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "user-123",
    "metadata": {"source": "quickstart"}
  }'
```

Save the `conversation_id` from the response:

```json
{
  "conversation_id": "conv-abc123...",
  "agent_id": "greeter"
}
```

---

## Step 4: Send a Message

Use your conversation ID:

```bash
curl -X POST http://localhost:8080/conversations/conv-abc123.../messages \
  -H "Content-Type: application/json" \
  -d '{
    "role": "user",
    "content": "Hello! Who are you?"
  }'
```

---

## Step 5: Stream the Response

Connect to the SSE stream to see the AI respond in real time:

```bash
curl -N http://localhost:8080/conversations/conv-abc123.../stream
```

You'll see events like:

```
event: message_start
data: {"message_id": "msg-..."}

event: content_delta
data: {"delta": "Hello"}

event: content_delta
data: {"delta": " there"}

event: message_end
data: {}
```

---

## What Just Happened?

1. **Started Bridge** — The runtime is now listening for requests
2. **Pushed an agent** — You defined an AI worker and sent it to Bridge
3. **Created a conversation** — A container for back-and-forth messages
4. **Sent a message** — User input for the agent to respond to
5. **Streamed the response** — Real-time events as the AI generates text

---

## Next Steps

Now that you've seen Bridge work:

- Learn about [configuration options](configuration.md)
- Understand the [core concepts](../core-concepts/index.md)
- Add [tools](../tools-reference/index.md) to your agent
- Set up your [control plane](../control-plane/index.md) to push agents programmatically
