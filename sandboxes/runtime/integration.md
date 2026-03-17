# Portal Bridge Integration Guide

This guide walks through integrating your control plane with Portal Bridge using two real-world examples: a **customer support agent** and a **Node.js engineer agent**.

## Architecture Overview

```
                    Push agents & history
  Control Plane  ──────────────────────────►  Portal Bridge
       │                                          │
       │◄────── Webhooks (HMAC-signed) ───────────┤
       │                                          │
  Your Backend                               Conversations
       │                                     SSE streaming
       │                                          │
  Your Frontend  ◄────────────────────────────────┘
```

Bridge is a **push-based** runtime. It starts with zero agents and receives everything from your control plane via HTTP. It never polls.

- **Control plane** owns agent definitions and conversation history
- **Bridge** runs agents, manages conversations, executes tools
- **Webhooks** flow back to your control plane for persistence and monitoring
- **SSE** streams directly to your frontend for real-time UI

## Authentication

All `/push/*` endpoints require a bearer token:

```
Authorization: Bearer {BRIDGE_CONTROL_PLANE_API_KEY}
```

Set this when starting bridge:

```bash
BRIDGE_CONTROL_PLANE_API_KEY=<bridge-control-plane-api-key> \
BRIDGE_LISTEN_ADDR=0.0.0.0:8080 \
BRIDGE_WEBHOOK_URL=https://your-api.com/webhooks/bridge \
./bridge
```

Public endpoints (`/agents/*`, `/conversations/*`, `/health`, `/metrics`) require no authentication.

---

## Step 1: Push Agent Definitions

On startup (or whenever agents change), your control plane pushes definitions to bridge.

### Example: Support Agent

```bash
curl -X POST https://bridge.yourapp.com/push/agents \
  -H "Authorization: Bearer <bridge-control-plane-api-key>" \
  -H "Content-Type: application/json" \
  -d '{
    "agents": [
      {
        "id": "support_agent",
        "name": "Customer Support",
        "system_prompt": "You are a friendly customer support agent for Acme Corp. Help users with billing questions, account issues, and product information. Always be polite and concise. If you cannot resolve an issue, escalate by telling the user you will connect them with a human agent.",
        "provider": {
          "provider_type": "anthropic",
          "model": "claude-haiku-4-5-20251001",
          "api_key": "<anthropic-api-key>"
        },
        "tools": [],
        "mcp_servers": [],
        "skills": [],
        "integrations": [],
        "config": {
          "max_tokens": 2048,
          "max_turns": 50,
          "temperature": 0.3
        },
        "subagents": [],
        "permissions": {},
        "webhook_url": "https://your-api.com/webhooks/bridge",
        "webhook_secret": "<webhook-secret>",
        "version": "1"
      }
    ]
  }'
```

**Response:**
```json
{"loaded": 1}
```

### Example: Node.js Engineer Agent

This agent has access to filesystem tools, an MCP server for code intelligence, and a GitHub integration:

```bash
curl -X POST https://bridge.yourapp.com/push/agents \
  -H "Authorization: Bearer <bridge-control-plane-api-key>" \
  -H "Content-Type: application/json" \
  -d '{
    "agents": [
      {
        "id": "nodejs_engineer",
        "name": "Node.js Engineer",
        "system_prompt": "You are a senior Node.js engineer. You have access to the project filesystem and can read, write, and edit files. Use tools to explore the codebase before making changes. Always explain what you are doing and why. Write clean, well-tested TypeScript code following the existing project conventions.",
        "provider": {
          "provider_type": "anthropic",
          "model": "claude-sonnet-4-20250514",
          "api_key": "<anthropic-api-key>",
          "base_url": "https://api.anthropic.com"
        },
        "tools": [],
        "mcp_servers": [
          {
            "name": "filesystem",
            "transport": {
              "type": "stdio",
              "command": "npx",
              "args": ["@modelcontextprotocol/server-filesystem", "/home/user/project"],
              "env": {}
            }
          }
        ],
        "skills": [],
        "integrations": [
          {
            "name": "github",
            "description": "GitHub repository management",
            "actions": [
              {
                "name": "create_pull_request",
                "description": "Create a pull request",
                "parameters_schema": {
                  "type": "object",
                  "properties": {
                    "title": {"type": "string"},
                    "body": {"type": "string"},
                    "head": {"type": "string"},
                    "base": {"type": "string"}
                  },
                  "required": ["title", "head", "base"]
                },
                "permission": "require_approval"
              }
            ]
          }
        ],
        "config": {
          "max_tokens": 8192,
          "max_turns": 100,
          "temperature": 0.2,
          "compaction": {
            "token_budget": 80000,
            "tail_messages": 10,
            "summary_provider": {
              "provider_type": "anthropic",
              "model": "claude-haiku-4-5-20251001",
              "api_key": "<anthropic-api-key>"
            }
          }
        },
        "subagents": [],
        "permissions": {
          "github__create_pull_request": "require_approval"
        },
        "webhook_url": "https://your-api.com/webhooks/bridge",
        "webhook_secret": "<webhook-secret>",
        "version": "3"
      }
    ]
  }'
```

### Upsert a single agent

When an agent definition changes, push the update. Bridge compares versions — same version is a no-op, different version triggers a drain-and-replace:

```bash
curl -X PUT https://bridge.yourapp.com/push/agents/support_agent \
  -H "Authorization: Bearer <bridge-control-plane-api-key>" \
  -H "Content-Type: application/json" \
  -d '{
    "id": "support_agent",
    "name": "Customer Support",
    "system_prompt": "Updated prompt...",
    "provider": { ... },
    "version": "2"
  }'
```

**Responses:**
```json
{"status": "created"}    // 201 — new agent
{"status": "updated"}    // 200 — version changed, agent replaced
{"status": "unchanged"}  // 200 — same version, no-op
```

### Rotate an API key without downtime

No drain, no conversation interruption. Existing conversations pick up the new key on their next LLM turn:

```bash
curl -X PATCH https://bridge.yourapp.com/push/agents/support_agent/api-key \
  -H "Authorization: Bearer <bridge-control-plane-api-key>" \
  -H "Content-Type: application/json" \
  -d '{"api_key": "<anthropic-api-key>"}'
```

---

## Step 2: Hydrate Existing Conversations (Optional)

If bridge restarts and users have active conversations, push their history so they can continue seamlessly:

```bash
curl -X POST https://bridge.yourapp.com/push/agents/support_agent/conversations \
  -H "Authorization: Bearer <bridge-control-plane-api-key>" \
  -H "Content-Type: application/json" \
  -d '{
    "conversations": [
      {
        "id": "conv_abc123",
        "agent_id": "support_agent",
        "title": "Billing inquiry",
        "created_at": "2025-03-17T10:00:00Z",
        "updated_at": "2025-03-17T10:15:00Z",
        "messages": [
          {
            "role": "user",
            "content": [{"type": "text", "text": "I was charged twice for my subscription"}],
            "timestamp": "2025-03-17T10:00:00Z"
          },
          {
            "role": "assistant",
            "content": [{"type": "text", "text": "I am sorry to hear about the double charge. Let me look into your account. Could you provide your account email?"}],
            "timestamp": "2025-03-17T10:00:05Z"
          },
          {
            "role": "user",
            "content": [{"type": "text", "text": "It is jane@example.com"}],
            "timestamp": "2025-03-17T10:01:00Z"
          }
        ]
      }
    ]
  }'
```

The conversation is now live. When Jane reconnects, she can continue from where she left off.

**Constraints:**
- Agent must have **zero active conversations** (409 Conflict otherwise)
- Hydrate before users reconnect

### Message format

Messages support multiple content block types:

```json
{
  "role": "assistant",
  "content": [
    {"type": "tool_call", "id": "tc_1", "name": "Read", "arguments": {"file_path": "/src/index.ts"}},
    {"type": "text", "text": "I read the file and here is what I found..."}
  ],
  "timestamp": "2025-03-17T10:05:00Z"
}
```

```json
{
  "role": "tool",
  "content": [
    {"type": "tool_result", "tool_call_id": "tc_1", "content": "file contents here...", "is_error": false}
  ],
  "timestamp": "2025-03-17T10:05:01Z"
}
```

---

## Step 3: Create Conversations

Your frontend (or backend on behalf of a user) creates a conversation:

```bash
curl -X POST https://bridge.yourapp.com/agents/support_agent/conversations
```

**Response (201):**
```json
{
  "conversation_id": "conv_new_456",
  "stream_url": "/conversations/conv_new_456/stream"
}
```

Store the `conversation_id` in your database, associated with the user session.

---

## Step 4: Connect the SSE Stream

Open a persistent connection to receive real-time events:

```bash
curl -N https://bridge.yourapp.com/conversations/conv_new_456/stream
```

### JavaScript (frontend):

```javascript
const eventSource = new EventSource('/conversations/conv_new_456/stream');

eventSource.onmessage = (event) => {
  const data = JSON.parse(event.data);

  switch (data.type) {
    case 'message_start':
      // New assistant message — create a message bubble in the UI
      console.log('Message started:', data.message_id);
      break;

    case 'content_delta':
      // Append text chunk to the current message bubble
      appendToMessage(data.message_id, data.delta);
      break;

    case 'tool_call_start':
      // Show tool activity indicator
      showToolActivity(data.name, data.arguments);
      break;

    case 'tool_call_result':
      // Show tool result (or error)
      showToolResult(data.id, data.result, data.is_error);
      break;

    case 'tool_approval_required':
      // Show approval dialog to the user
      showApprovalDialog({
        requestId: data.request_id,
        toolName: data.tool_name,
        arguments: data.arguments,
        integrationName: data.integration_name,
        integrationAction: data.integration_action,
      });
      break;

    case 'tool_approval_resolved':
      // Dismiss approval dialog
      dismissApprovalDialog(data.request_id, data.decision);
      break;

    case 'message_end':
      // Finalize the message, show token usage
      finalizeMessage(data.message_id, data.usage);
      break;

    case 'todo_updated':
      // Update task list sidebar
      updateTodoList(data.todos);
      break;

    case 'error':
      // Show error in the UI
      showError(data.code, data.message);
      break;

    case 'done':
      // Turn complete — re-enable the input box
      enableInput();
      break;
  }
};
```

### SSE event reference

Every event is JSON with a `type` field:

| Event | Fields | When |
|-------|--------|------|
| `message_start` | `conversation_id`, `message_id` | Assistant begins responding |
| `content_delta` | `delta`, `message_id` | Text chunk streamed |
| `tool_call_start` | `id`, `name`, `arguments` | Agent invokes a tool |
| `tool_call_result` | `id`, `result`, `is_error` | Tool execution completed |
| `tool_approval_required` | `request_id`, `tool_name`, `tool_call_id`, `arguments`, `integration_name?`, `integration_action?` | Tool needs user approval |
| `tool_approval_resolved` | `request_id`, `decision` | Approval decided |
| `todo_updated` | `todos[]` (each: `content`, `status`, `priority`) | Agent updated its task list |
| `message_end` | `message_id`, `usage` (`input_tokens`, `output_tokens`) | Response complete |
| `error` | `code`, `message` | Error occurred |
| `done` | — | Turn finished, safe to send next message |

**Keep-alive:** Bridge sends `ping` every 15 seconds to keep the connection open.

---

## Step 5: Send Messages

```bash
curl -X POST https://bridge.yourapp.com/conversations/conv_new_456/messages \
  -H "Content-Type: application/json" \
  -d '{"content": "I was charged twice for my March subscription"}'
```

**Response (202):**
```json
{"status": "accepted"}
```

The response streams back over the SSE connection. Do not poll — just listen.

### Support agent conversation flow

```
User:  "I was charged twice for my March subscription"

SSE events:
  → message_start    {message_id: "msg_1"}
  → content_delta    {delta: "I'm sorry "}
  → content_delta    {delta: "to hear about "}
  → content_delta    {delta: "the double charge. "}
  → content_delta    {delta: "Could you provide your account email?"}
  → message_end      {message_id: "msg_1", usage: {input_tokens: 85, output_tokens: 22}}
  → done

User:  "jane@example.com"

SSE events:
  → message_start    {message_id: "msg_2"}
  → content_delta    {delta: "Thank you, Jane. "}
  → content_delta    {delta: "I can see the duplicate charge. "}
  → content_delta    {delta: "A refund of $29.99 has been initiated..."}
  → message_end      {message_id: "msg_2", usage: {input_tokens: 142, output_tokens: 45}}
  → done
```

### Node.js engineer conversation flow with tools

```
User:  "Add input validation to the POST /users endpoint"

SSE events:
  → message_start    {message_id: "msg_1"}
  → content_delta    {delta: "I'll start by reading the current endpoint..."}
  → tool_call_start  {id: "tc_1", name: "Read", arguments: {"file_path": "/src/routes/users.ts"}}
  → tool_call_result {id: "tc_1", result: "import express from...", is_error: false}
  → content_delta    {delta: "I can see the endpoint accepts..."}
  → tool_call_start  {id: "tc_2", name: "Edit", arguments: {"file_path": "/src/routes/users.ts", ...}}
  → tool_call_result {id: "tc_2", result: "File edited successfully", is_error: false}
  → content_delta    {delta: "I've added zod validation for the request body..."}
  → message_end      {message_id: "msg_1", usage: {input_tokens: 2400, output_tokens: 850}}
  → done
```

### Node.js engineer with tool approval

When the agent tries to create a PR (configured with `require_approval`):

```
User:  "Create a PR with these changes"

SSE events:
  → message_start            {message_id: "msg_3"}
  → content_delta            {delta: "I'll create a pull request for the validation changes."}
  → tool_call_start          {id: "tc_5", name: "github__create_pull_request", arguments: {...}}
  → tool_approval_required   {
      request_id: "apr_789",
      tool_name: "github__create_pull_request",
      tool_call_id: "tc_5",
      arguments: {"title": "Add input validation to POST /users", "head": "feat/validation", "base": "main"},
      integration_name: "github",
      integration_action: "create_pull_request"
    }

  ... agent is paused, waiting for approval ...
```

Your UI shows the approval dialog. User clicks "Approve":

```bash
curl -X POST https://bridge.yourapp.com/agents/nodejs_engineer/conversations/conv_789/approvals/apr_789 \
  -H "Content-Type: application/json" \
  -d '{"decision": "approve"}'
```

The agent resumes:

```
SSE events (continued):
  → tool_approval_resolved   {request_id: "apr_789", decision: "approve"}
  → tool_call_result         {id: "tc_5", result: "PR #42 created", is_error: false}
  → content_delta            {delta: "Done! PR #42 has been created..."}
  → message_end              {message_id: "msg_3", usage: {...}}
  → done
```

---

## Step 6: Receive Webhooks

Bridge POSTs events to your `BRIDGE_WEBHOOK_URL` (or per-agent `webhook_url`). Use these to persist conversation history, track usage, and trigger workflows.

### Webhook format

Every webhook is an HTTP POST with:

```
POST https://your-api.com/webhooks/bridge
Content-Type: application/json
X-Webhook-Signature: {base64_hmac_sha256}
X-Webhook-Timestamp: {unix_seconds}

{
  "event_type": "message_received",
  "agent_id": "support_agent",
  "conversation_id": "conv_new_456",
  "timestamp": "2025-03-17T10:30:00Z",
  "data": {
    "content": "I was charged twice for my March subscription"
  }
}
```

### Verifying signatures

The signature covers `{timestamp}.{body}` using HMAC-SHA256 with the agent's `webhook_secret`:

```javascript
const crypto = require('crypto');

function verifyWebhook(body, signature, timestamp, secret) {
  const message = `${timestamp}.${body}`;
  const expected = crypto
    .createHmac('sha256', secret)
    .update(message)
    .digest('base64');
  return crypto.timingSafeEqual(
    Buffer.from(signature),
    Buffer.from(expected)
  );
}

// In your webhook handler:
app.post('/webhooks/bridge', (req, res) => {
  const signature = req.headers['x-webhook-signature'];
  const timestamp = req.headers['x-webhook-timestamp'];
  const body = JSON.stringify(req.body);

  if (!verifyWebhook(body, signature, timestamp, req.body.webhook_secret)) {
    return res.status(401).send('Invalid signature');
  }

  // Process the event...
  res.status(200).send('OK');
});
```

### Webhook event reference

| Event | `data` payload | Use case |
|-------|---------------|----------|
| `conversation_created` | `{}` | Log new conversation |
| `message_received` | `{"content": "user text"}` | Persist user message |
| `response_started` | `{}` | Show typing indicator |
| `response_chunk` | `{"chunk": "partial text"}` | Optional: persist streaming chunks |
| `response_completed` | `{"full_response": "complete text"}` | Persist assistant message |
| `tool_call_started` | `{"tool_name": "Read", "arguments": {...}}` | Log tool usage |
| `tool_call_completed` | `{"tool_name": "Read", "result": "...", "is_error": false}` | Log tool result |
| `tool_approval_required` | `{"request_id": "...", "tool_name": "...", "arguments": {...}}` | Notify user/admin |
| `tool_approval_resolved` | `{"request_id": "...", "decision": "approve"}` | Log decision |
| `todo_updated` | `{"todos": [...]}` | Update task tracking |
| `turn_completed` | `{}` | Mark turn done, update billing |
| `agent_error` | `{"code": "...", "message": "..."}` | Alert on failures |
| `conversation_ended` | `{}` | Mark conversation closed |
| `conversation_compacted` | `{"summary": "...", "messages_compacted": 35, "pre_compaction_tokens": 120000, "post_compaction_tokens": 15000}` | Log compaction events |

### Example: Persisting history from webhooks

```javascript
app.post('/webhooks/bridge', async (req, res) => {
  const { event_type, agent_id, conversation_id, data, timestamp } = req.body;

  switch (event_type) {
    case 'conversation_created':
      await db.conversations.create({
        id: conversation_id,
        agent_id,
        created_at: timestamp,
      });
      break;

    case 'message_received':
      await db.messages.create({
        conversation_id,
        role: 'user',
        content: data.content,
        timestamp,
      });
      break;

    case 'response_completed':
      await db.messages.create({
        conversation_id,
        role: 'assistant',
        content: data.full_response,
        timestamp,
      });
      break;

    case 'tool_call_started':
      await db.toolCalls.create({
        conversation_id,
        tool_name: data.tool_name,
        arguments: data.arguments,
        timestamp,
      });
      break;

    case 'turn_completed':
      await db.conversations.update(conversation_id, {
        updated_at: timestamp,
        turn_count: db.raw('turn_count + 1'),
      });
      break;

    case 'conversation_ended':
      await db.conversations.update(conversation_id, {
        status: 'ended',
        ended_at: timestamp,
      });
      break;

    case 'agent_error':
      await alerting.notify(`Agent error in ${conversation_id}: ${data.message}`);
      break;
  }

  res.status(200).send('OK');
});
```

---

## Step 7: Manage Agent Lifecycle

### Apply incremental diffs

When multiple agents change at once:

```bash
curl -X POST https://bridge.yourapp.com/push/diff \
  -H "Authorization: Bearer <bridge-control-plane-api-key>" \
  -H "Content-Type: application/json" \
  -d '{
    "added": [
      {"id": "new_agent", "name": "New Agent", ...}
    ],
    "updated": [
      {"id": "support_agent", "name": "Customer Support", "version": "3", ...}
    ],
    "removed": ["deprecated_agent"]
  }'
```

**Response:**
```json
{"added": 1, "updated": 1, "removed": 1}
```

Updated agents go through a drain cycle — active conversations are cancelled, the new agent is loaded, and new conversations use the updated definition.

### Remove an agent

```bash
curl -X DELETE https://bridge.yourapp.com/push/agents/deprecated_agent \
  -H "Authorization: Bearer <bridge-control-plane-api-key>"
```

### List active agents

```bash
curl https://bridge.yourapp.com/agents
```

```json
[
  {"id": "support_agent", "name": "Customer Support", "version": "2"},
  {"id": "nodejs_engineer", "name": "Node.js Engineer", "version": "3"}
]
```

### Get agent details

```bash
curl https://bridge.yourapp.com/agents/support_agent
```

```json
{
  "id": "support_agent",
  "name": "Customer Support",
  "system_prompt": "You are a friendly...",
  "version": "2",
  "active_conversations": 12
}
```

---

## Step 8: End Conversations

```bash
curl -X DELETE https://bridge.yourapp.com/conversations/conv_new_456
```

```json
{"status": "ended"}
```

### Abort an in-progress turn

If the agent is taking too long or the user wants to cancel:

```bash
curl -X POST https://bridge.yourapp.com/conversations/conv_new_456/abort
```

```json
{"status": "aborted"}
```

The conversation stays alive — the user can send another message. Only the current turn is cancelled.

---

## Step 9: Monitor

### Health check

```bash
curl https://bridge.yourapp.com/health
```

```json
{"status": "ok", "uptime_secs": 86400}
```

### Metrics

```bash
curl https://bridge.yourapp.com/metrics
```

```json
{
  "global": {
    "total_agents": 2,
    "uptime_secs": 86400
  },
  "timestamp": "2025-03-17T10:00:00Z",
  "agents": [
    {
      "agent_id": "support_agent",
      "agent_name": "Customer Support",
      "active_conversations": 12,
      "total_requests": 4520,
      "total_input_tokens": 1250000,
      "total_output_tokens": 380000,
      "total_errors": 3,
      "avg_latency_ms": 1200
    }
  ]
}
```

---

## Supported Providers

| Provider | `provider_type` | `base_url` | Notes |
|----------|----------------|------------|-------|
| Anthropic | `"anthropic"` | Optional (defaults to `https://api.anthropic.com`) | Native client, `x-api-key` auth |
| Google Gemini | `"google"` | Optional (defaults to Google endpoint) | Native client, API key as query param |
| Cohere | `"cohere"` | Optional (defaults to `https://api.cohere.ai`) | Native client, Bearer auth |
| OpenAI | `"open_ai"` | **Required** | OpenAI-compatible format |
| Groq | `"groq"` | **Required** | OpenAI-compatible |
| DeepSeek | `"deep_seek"` | **Required** | OpenAI-compatible |
| Mistral | `"mistral"` | **Required** | OpenAI-compatible |
| xAI | `"x_ai"` | **Required** | OpenAI-compatible |
| Together | `"together"` | **Required** | OpenAI-compatible |
| Fireworks | `"fireworks"` | **Required** | OpenAI-compatible |
| Ollama | `"ollama"` | **Required** | OpenAI-compatible, local |
| Custom | `"custom"` | **Required** | Any OpenAI-compatible endpoint |

---

## Error Handling

All errors return JSON:

```json
{
  "error": {
    "code": "agent_not_found",
    "message": "agent not found: nonexistent_agent"
  }
}
```

| Status | Code | Meaning |
|--------|------|---------|
| 400 | `invalid_request` | Bad request body or path mismatch |
| 401 | `unauthorized` | Missing or invalid bearer token |
| 404 | `agent_not_found` | Agent ID not loaded |
| 404 | `conversation_not_found` | Conversation ended or never existed |
| 409 | `conflict` | Agent has active conversations (hydration blocked) |
| 500 | `internal_error` | Server error |

---

## Full Lifecycle Sequence

```
1. Bridge boots (zero agents, listening on :8080)

2. Control plane pushes agents
   POST /push/agents → agents are live

3. (Optional) Control plane hydrates previous conversations
   POST /push/agents/{id}/conversations → conversations are live

4. User opens chat
   Frontend → POST /agents/{id}/conversations → gets conversation_id
   Frontend → GET /conversations/{id}/stream → SSE connection open

5. User sends message
   Frontend → POST /conversations/{id}/messages → 202
   Bridge processes asynchronously
   SSE: message_start → content_delta* → [tool events]* → message_end → done
   Webhooks: message_received → response_started → response_completed → turn_completed

6. (If tool approval needed)
   SSE: tool_approval_required
   Frontend shows dialog → user decides
   Frontend → POST /agents/{id}/conversations/{id}/approvals/{id} → {decision}
   Agent resumes

7. Agent definition changes
   Control plane → PUT /push/agents/{id} (new version) → drain + replace
   OR
   Control plane → PATCH /push/agents/{id}/api-key → instant key swap

8. User ends chat
   Frontend → DELETE /conversations/{id}
   Webhook: conversation_ended

9. Bridge restarts
   Control plane re-pushes agents (step 2)
   Control plane hydrates active conversations (step 3)
   Users reconnect SSE streams (step 4)
```
