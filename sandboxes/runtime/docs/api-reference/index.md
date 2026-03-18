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
| `/conversations/{id}` | DELETE | No | End conversation |
| `/push/agents` | POST | Yes | Bulk load agents |
| `/push/agents/{id}` | PUT | Yes | Update single agent |
| `/push/agents/{id}` | DELETE | Yes | Remove agent |
| `/push/agents/{id}/conversations` | POST | Yes | Hydrate conversation |

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
