# WebSocket Event Stream

The `/ws/events` endpoint provides a single persistent WebSocket connection that delivers **all events** from **all agents** and **all conversations**. It is an efficient alternative to webhooks for high-throughput control planes.

---

## When to Use WebSocket vs Webhooks vs SSE

| Mechanism | Best for | Connection model |
|-----------|----------|-----------------|
| **SSE** (`/conversations/{id}/stream`) | Frontend clients streaming a single conversation | One connection per conversation |
| **Webhooks** (`BRIDGE_WEBHOOK_URL`) | Server-to-server with persistence and retries | HTTP POST per event batch |
| **WebSocket** (`/ws/events`) | High-throughput control planes receiving all events | One persistent connection for everything |

You can enable any combination — SSE is always available, webhooks and WebSocket are opt-in.

---

## Enabling WebSocket

```bash
export BRIDGE_WEBSOCKET_ENABLED="true"
```

Or in `config.toml`:

```toml
websocket_enabled = true
```

---

## Connecting

```
GET /ws/events?token=<control_plane_api_key>
```

Authenticate via the `?token=` query parameter using the same API key configured in `BRIDGE_CONTROL_PLANE_API_KEY`.

**Why query parameter authentication?** WebSocket clients (particularly browsers) cannot set custom HTTP headers during the upgrade handshake. The `Authorization` header is not available when calling `new WebSocket(url)` in JavaScript. The `?token=` query parameter works around this browser limitation. If you are using a server-side WebSocket client that supports custom headers, you may still prefer the query parameter for consistency.

### JavaScript Example

```javascript
const ws = new WebSocket('ws://localhost:8080/ws/events?token=<bridge-control-plane-api-key>');

ws.onmessage = (event) => {
  const data = JSON.parse(event.data);

  if (data.type === 'lagged') {
    console.warn(`Missed ${data.missed_events} events — client fell behind`);
    return;
  }

  console.log(`[${data.agent_id}/${data.conversation_id}] ${data.event_type}`, data.data);
};

ws.onclose = () => console.log('WebSocket closed');
ws.onerror = (err) => console.error('WebSocket error', err);
```

### Python Example

```python
import asyncio
import json
import websockets

async def listen():
    uri = "ws://localhost:8080/ws/events?token=<bridge-control-plane-api-key>"
    async with websockets.connect(uri) as ws:
        async for message in ws:
            event = json.loads(message)
            if event.get("type") == "lagged":
                print(f"Warning: missed {event['missed_events']} events")
                continue
            print(f"[{event['agent_id']}] {event['event_type']}: {event.get('data', {})}")

asyncio.run(listen())
```

---

## Event Format

Each WebSocket message is a JSON object with the same fields as webhook payloads, minus the `webhook_url` and `webhook_secret` fields:

```json
{
  "event_id": "evt-abc123",
  "event_type": "response_started",
  "agent_id": "my-agent",
  "conversation_id": "conv-def456",
  "timestamp": "2026-01-15T10:30:00Z",
  "sequence_number": 42,
  "data": {}
}
```

| Field | Type | Description |
|-------|------|-------------|
| `event_id` | string | Unique event identifier |
| `event_type` | string | Event type (see [Event Types](#event-types)) |
| `agent_id` | string | Agent that triggered the event |
| `conversation_id` | string | Conversation associated with the event |
| `timestamp` | string | ISO 8601 timestamp (UTC) |
| `sequence_number` | integer | Global monotonically increasing counter |
| `data` | object | Event-specific data (varies by event type) |

### Sequence Numbers

Sequence numbers are **globally monotonic** across all agents and all conversations. They are assigned by the EventBus before fan-out to all delivery channels (WebSocket, SSE, webhooks), so the same event has the same `sequence_number` everywhere.

**Important:** Because sequence numbers are global (not per-conversation), you cannot assume that consecutive sequence numbers belong to the same conversation. If you are tracking a specific conversation, filter events by `conversation_id` after receiving them. The global ordering guarantees that if you store the last seen `sequence_number`, you can detect exactly how many events were missed during a disconnection.

---

## Event Types

The WebSocket delivers the same event types as webhooks:

### Conversation Events

| Event Type | When it fires | Data Fields |
|------------|---------------|-------------|
| `conversation_created` | New conversation started | `{}` |
| `message_received` | User message received | `content` |
| `conversation_ended` | Conversation ended | `{}` |
| `conversation_compacted` | History was summarized | `summary`, `messages_compacted`, `pre_compaction_tokens`, `post_compaction_tokens` |

### Response Events

| Event Type | When it fires | Data Fields |
|------------|---------------|-------------|
| `response_started` | Assistant started responding | `conversation_id`, `message_id` |
| `response_chunk` | Streaming chunk generated | `delta`, `message_id` |
| `response_completed` | Assistant finished responding | `message_id`, `input_tokens`, `output_tokens`, `model`, `timestamp`, `full_response` |
| `turn_completed` | Turn/stream completed | `input_tokens`, `output_tokens`, `model`, `timestamp`, `turn_number`, `cumulative_input_tokens`, `cumulative_output_tokens` |

### Tool Events

| Event Type | When it fires | Data Fields |
|------------|---------------|-------------|
| `tool_call_started` | Tool was invoked | `id`, `name`, `arguments` |
| `tool_call_completed` | Tool finished executing | `id`, `tool_name`, `result`, `is_error`, `duration_ms` |
| `tool_approval_required` | Tool needs user approval | `request_id`, `tool_name`, `tool_call_id`, `arguments`, `integration_name`, `integration_action` |
| `tool_approval_resolved` | User approved/denied tool | `request_id`, `decision` |

### Reasoning Events

| Event Type | When it fires | Data Fields |
|------------|---------------|-------------|
| `reasoning_delta` | Reasoning/thinking chunk from the model | `delta`, `message_id` |

### Subagent Events

| Event Type | When it fires | Data Fields |
|------------|---------------|-------------|
| `sub_agent_started` | A subagent was spawned | `subagent_name`, `mode`, `parent_conversation_id`, `depth` |
| `sub_agent_completed` | A subagent finished execution | `subagent_name`, `mode`, `task_id`, `duration_ms`, `is_error` |

### Chain Events (Immortal Conversations)

| Event Type | When it fires | Data Fields |
|------------|---------------|-------------|
| `chain_started` | Context chain handoff beginning | `chain_index`, `reason`, `token_count` |
| `chain_completed` | Context chain handoff finished | `chain_index`, `journal_entry_count`, `carry_forward_messages` |

### Other Events

| Event Type | When it fires | Data Fields |
|------------|---------------|-------------|
| `todo_updated` | Todo list updated | `todos` |
| `agent_error` | Error occurred | `code`, `message` |
| `background_task_completed` | Background task finished | `task_id`, `description`, `output`, `is_error` |
| `done` | Response stream complete (terminal signal) | `{}` |

---

## Lagged Client Warning

If a client falls behind the broadcast buffer (default: 10,000 events), it receives a special warning message:

```json
{
  "type": "lagged",
  "missed_events": 150
}
```

This means 150 events were dropped because the client wasn't reading fast enough. After receiving this warning, the client continues receiving new events normally.

---

## Multiple Clients

Multiple WebSocket clients can connect simultaneously. Each receives an independent copy of every event. This is useful for running multiple consumers (e.g., one for persistence, one for monitoring).

---

## Configuration Combinations

| `BRIDGE_WEBHOOK_URL` | `BRIDGE_WEBSOCKET_ENABLED` | Behavior |
|-----------------------|---------------------------|----------|
| Not set | `false` (default) | No external event delivery (SSE only) |
| Set | `false` | Webhooks only |
| Not set | `true` | WebSocket only |
| Set | `true` | Both webhooks and WebSocket |

---

## Graceful Shutdown

When Bridge shuts down, it sends a WebSocket Close frame (opcode `0x8`) to all connected clients before terminating the connection. Clients should handle the `onclose` event and implement reconnection logic with backoff.

**Note:** The Close frame uses the standard WebSocket close code `1001` (Going Away), indicating the server is shutting down. Clients can distinguish this from an unexpected disconnection (which typically produces code `1006` — Abnormal Closure) and adjust their reconnection strategy accordingly.

---

## See Also

- [SSE Events](sse-events.md) — Per-conversation streaming for frontends
- [Webhooks](../core-concepts/webhooks.md) — HTTP-based event delivery with retries
- [Configuration](../getting-started/configuration.md) — Full configuration guide
