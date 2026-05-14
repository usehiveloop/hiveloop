# API Reference

Complete reference for the Bridge HTTP API.

---

## Base URL

All API paths are relative to your Bridge instance:

```
http://localhost:8080  (default local)
https://bridge.yourdomain.com  (production)
```

---

## API Sections

### Public Endpoints
No authentication required:

- **Health & Metrics** — Check status, get metrics
- **Agents** — List agents, get agent details
- **Conversations** — Create conversations, send messages, stream events

### Authenticated Endpoints
Bearer token required:

- **Push API** — Load agents, hydrate conversations

---

## Quick Reference

| Endpoint | Method | Auth | Purpose |
|----------|--------|------|---------|
| `/health` | GET | No | Health check |
| `/metrics` | GET | No | Prometheus metrics |
| `/agents` | GET | No | List all agents |
| `/agents/{id}` | GET | No | Get agent details |
| `/agents/{id}/conversations` | POST | No | Create conversation |
| `/conversations/{id}/messages` | POST | No | Send message |
| `/conversations/{id}/stream` | GET | No | SSE stream |
| `/ws/events?token={key}` | GET | Token | WebSocket event stream (all events) |
| `/events?token={key}` | GET | Token | Poll for events by sequence number — fallback when WS/SSE fails |
| `/conversations/{id}` | DELETE | No | End conversation |
| `/push/agents` | POST | Yes | Bulk load agents |
| `/push/agents/{id}` | PUT | Yes | Update single agent |
| `/push/agents/{id}` | DELETE | Yes | Remove agent |
| `/push/agents/{id}/conversations` | POST | Yes | Hydrate conversation |

---

## GET /events

Poll for events by sequence number. This is a fallback for environments where WebSocket and SSE connections are unreliable (e.g., behind aggressive proxies or load balancers).

### Request

```
GET /events?token={key}&after={sequence_number}&limit={count}
```

### Query Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `token` | string | Yes | — | Control plane API key (same as `BRIDGE_CONTROL_PLANE_API_KEY`) |
| `after` | u64 | No | `0` | Return events with `sequence_number` greater than this value |
| `limit` | u32 | No | `100` | Maximum number of events to return (max 1000) |

### Response

JSON array of `BridgeEvent` objects, ordered by `sequence_number` ascending:

```json
[
  {
    "event_id": "evt-abc123",
    "event_type": "response_started",
    "agent_id": "my-agent",
    "conversation_id": "conv-def456",
    "timestamp": "2026-01-15T10:30:00Z",
    "sequence_number": 42,
    "data": {}
  },
  {
    "event_id": "evt-abc124",
    "event_type": "response_chunk",
    "agent_id": "my-agent",
    "conversation_id": "conv-def456",
    "timestamp": "2026-01-15T10:30:01Z",
    "sequence_number": 43,
    "data": {
      "delta": "Hello"
    }
  }
]
```

| Field | Type | Description |
|-------|------|-------------|
| `event_id` | string | Unique event identifier |
| `event_type` | string | Event type (same types as WebSocket/webhooks) |
| `agent_id` | string | Agent that triggered the event |
| `conversation_id` | string | Conversation associated with the event |
| `timestamp` | string | ISO 8601 timestamp (UTC) |
| `sequence_number` | integer | Global monotonically increasing counter |
| `data` | object | Event-specific data (varies by event type) |

### Errors

| Status | Code | Meaning |
|--------|------|---------|
| 401 | `unauthorized` | Missing or invalid `token` query parameter |
| 400 | `storage_not_enabled` | Event storage is not enabled — enable it with `BRIDGE_EVENT_STORAGE_ENABLED=true` |

### Example

```bash
# Fetch the first batch of events
curl "http://localhost:8080/events?token=<bridge-control-plane-api-key>&after=0&limit=50"

# Fetch next batch using the last sequence_number from the previous response
curl "http://localhost:8080/events?token=<bridge-control-plane-api-key>&after=43&limit=50"
```

**Tip:** To implement a polling loop, track the highest `sequence_number` from each response and pass it as the `after` parameter in the next request.

---

## Content Types

Bridge accepts and returns JSON:

```
Content-Type: application/json
```

For SSE streams:

```
Accept: text/event-stream
```

---

## Response Format

Success responses:

```json
{
  "field": "value"
}
```

Error responses:

```json
{
  "error": {
    "code": "AGENT_NOT_FOUND",
    "message": "Agent 'xyz' not found"
  }
}
```

---

## Rate Limiting

Bridge doesn't implement rate limiting. Add it at your reverse proxy (nginx, etc.) if needed.

---

## OpenAPI Spec

Full OpenAPI specification is available in the repository:

```
openapi.json
```

Or at runtime:

```bash
curl http://localhost:8080/openapi.json
```

---

## Sections

- [Authentication](authentication.md) — How to authenticate
- [Agents API](agents-api.md) — Agent listing and details
- [Conversations API](conversations-api.md) — Conversations and messaging
- [Push API](push-api.md) — Control plane integration
- [SSE Events](sse-events.md) — Streaming event reference
- [WebSocket Events](websocket-events.md) — WebSocket event stream reference
