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

Authenticate via the `?token=` query parameter using the same API key configured in `BRIDGE_CONTROL_PLANE_API_KEY`. WebSocket clients cannot always set custom headers, so query parameter authentication is used.

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

Unlike webhooks (which use per-conversation sequence numbers), WebSocket sequence numbers are **global** across all conversations. They are strictly increasing with no gaps, making it easy to detect missed events on reconnection.

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
| `response_started` | Assistant started responding | `{}` |
| `response_chunk` | Streaming chunk generated | `delta` |
| `response_completed` | Assistant finished responding | `input_tokens`, `output_tokens`, `full_response` |
| `turn_completed` | Turn/stream completed | `{}` |

### Tool Events

| Event Type | When it fires | Data Fields |
|------------|---------------|-------------|
| `tool_call_started` | Tool was invoked | `tool_name`, `arguments` |
| `tool_call_completed` | Tool finished executing | `tool_name`, `result`, `is_error` |
| `tool_approval_required` | Tool needs user approval | `request_id`, `tool_name`, `tool_call_id`, `arguments` |
| `tool_approval_resolved` | User approved/denied tool | `request_id`, `decision` |

### Other Events

| Event Type | When it fires | Data Fields |
|------------|---------------|-------------|
| `todo_updated` | Todo list updated | `todos` |
| `agent_error` | Error occurred | `code`, `message` |
| `background_task_completed` | Background task finished | Task result data |

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

When Bridge shuts down, it sends a WebSocket close frame to all connected clients. Clients should handle the close event and reconnect if needed.

---

## See Also

- [SSE Events](sse-events.md) — Per-conversation streaming for frontends
- [Webhooks](../core-concepts/webhooks.md) — HTTP-based event delivery with retries
- [Configuration](../getting-started/configuration.md) — Full configuration guide
