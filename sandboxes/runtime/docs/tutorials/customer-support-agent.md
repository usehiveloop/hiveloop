# Customer Support Agent

Build an agent that helps customers with order lookups and refunds.

---

## What You'll Build

A customer support agent that can:

- Look up order status
- Process refunds (with approval)
- Answer common questions
- Escalate to humans when needed

---

## Prerequisites

- Bridge running locally
- An API key (Anthropic or OpenAI-compatible)
- A mock order API (we'll use webhook.site for testing)

---

## Step 1: Create the Agent

Create `support-agent.json`:

```json
{
  "agents": [
    {
      "id": "support-agent",
      "name": "Customer Support",
      "system_prompt": "You are a helpful customer support agent for Acme Store.\n\nYou can help customers with:\n- Checking order status\n- Processing refunds\n- Answering questions about products\n\nAlways be polite and professional. If you cannot help with something, escalate to a human agent by saying 'I'll connect you with a human agent.'",
      "provider": {
        "provider_type": "anthropic",
        "model": "claude-sonnet-4-20250514",
        "api_key": "YOUR_API_KEY"
      },
      "tools": ["web_search"],
      "integrations": [
        {
          "name": "store_api",
          "description": "Access to store order data",
          "base_url": "https://webhook.site/YOUR-UNIQUE-ID",
          "actions": [
            {
              "name": "get_order",
              "description": "Get order details by order ID",
              "method": "GET",
              "path": "/orders/{order_id}",
              "parameters_schema": {
                "type": "object",
                "properties": {
                  "order_id": { "type": "string" }
                },
                "required": ["order_id"]
              }
            },
            {
              "name": "process_refund",
              "description": "Process a refund for an order",
              "method": "POST",
              "path": "/refunds",
              "parameters_schema": {
                "type": "object",
                "properties": {
                  "order_id": { "type": "string" },
                  "reason": { "type": "string" }
                },
                "required": ["order_id", "reason"]
              },
              "permission": "require_approval"
            }
          ]
        }
      ],
      "config": {
        "max_tokens": 1024,
        "max_turns": 20,
        "temperature": 0.3
      },
      "version": "1"
    }
  ]
}
```

Replace `YOUR_API_KEY` and get a webhook.site URL for testing.

---

## Step 2: Push the Agent

```bash
curl -X POST http://localhost:8080/push/agents \
  -H "Authorization: Bearer <bridge-control-plane-api-key>" \
  -H "Content-Type: application/json" \
  -d @support-agent.json
```

You should see `{"loaded": 1}`.

---

## Step 3: Test Order Lookup

Create a conversation:

```bash
curl -X POST http://localhost:8080/agents/support-agent/conversations \
  -H "Content-Type: application/json" \
  -d '{"user_id": "user-123"}'
```

Send a message:

```bash
curl -X POST http://localhost:8080/conversations/CONV_ID/messages \
  -H "Content-Type: application/json" \
  -d '{"role": "user", "content": "What's the status of order ORD-123?"}'
```

Watch the stream:

```bash
curl -N http://localhost:8080/conversations/CONV_ID/stream
```

Check webhook.site — you should see a request to `/orders/ORD-123`.

---

## Step 4: Test Refund Approval

Send a refund request:

```bash
curl -X POST http://localhost:8080/conversations/CONV_ID/messages \
  -H "Content-Type: application/json" \
  -d '{"role": "user", "content": "I want a refund for order ORD-123"}'
```

The agent will:
1. Ask for a reason
2. Call `process_refund` (requires approval)
3. Pause for your approval

Approve it:

```bash
curl -X POST http://localhost:8080/agents/support-agent/conversations/CONV_ID/approvals \
  -H "Content-Type: application/json" \
  -d '{"action": "approve_all"}'
```

---

## Complete Agent

See the full `support-agent.json` above.

---

## What You Learned

- Creating agents with integration tools
- Using `require_approval` for sensitive actions
- Setting up agent personality with system prompts
- Testing with webhook.site

---

## Next Steps

- Connect to your real order API
- Add more actions (update shipping, cancel orders)
- Set up webhook handling to save conversation history
- Deploy to production

---

## See Also

- [Integration Tools](../tools-reference/custom-tools.md)
- [Handling Webhooks](../control-plane/handling-webhooks.md)
