# Authentication

Bridge uses bearer token authentication for push endpoints.

---

## Which Endpoints Need Auth

### No Authentication

These endpoints are public:

- `GET /health`
- `GET /metrics`
- `GET /agents`
- `GET /agents/{id}`
- `POST /agents/{id}/conversations`
- `POST /conversations/{id}/messages`
- `GET /conversations/{id}/stream`
- `DELETE /conversations/{id}`

### Bearer Token Required

These endpoints require authentication:

- `POST /push/agents`
- `PUT /push/agents/{id}`
- `DELETE /push/agents/{id}`
- `POST /push/agents/{id}/conversations`
- `POST /push/diff`
- `PATCH /push/agents/{id}/api-key`

---

## Setting the Token

Set `BRIDGE_CONTROL_PLANE_API_KEY` when starting Bridge:

```bash
export BRIDGE_CONTROL_PLANE_API_KEY="<bridge-control-plane-api-key>"
./bridge
```

---

## Using the Token

Include the token in the `Authorization` header:

```bash
curl -X POST http://localhost:8080/push/agents \
  -H "Authorization: Bearer <bridge-control-plane-api-key>" \
  -H "Content-Type: application/json" \
  -d '{...}'
```

The format is:

```
Authorization: Bearer {your-token}
```

---

## Authentication Errors

### Missing Token

```json
{
  "error": {
    "code": "UNAUTHORIZED",
    "message": "Missing authorization header"
  }
}
```

Status: `401 Unauthorized`

### Invalid Token

```json
{
  "error": {
    "code": "UNAUTHORIZED",
    "message": "Invalid API key"
  }
}
```

Status: `401 Unauthorized`

---

## Security Best Practices

### Generate Strong Tokens

Use a cryptographically secure random string:

```bash
# Generate a 32-byte token
openssl rand -hex 32
# Result: a1b2c3d4e5f6...
```

### Store Tokens Securely

- Use environment variables or secrets managers
- Never commit tokens to git
- Rotate tokens periodically

### Use HTTPS in Production

Always use HTTPS for production deployments. Without it, tokens are sent in plaintext.

```
# Bad (development only)
http://bridge.example.com/push/agents

# Good
https://bridge.example.com/push/agents
```

### Token Scope

Bridge uses a single token for all push endpoints. If you need more granular access control, implement it in your control plane.

---

## Rotating Tokens

To rotate the token without downtime:

1. Generate a new token
2. Update your control plane to use the new token
3. Update Bridge's `BRIDGE_CONTROL_PLANE_API_KEY`
4. Restart Bridge

There will be a brief window where the old and new tokens are both valid (during the rolling restart).

---

## Multiple Control Planes

Bridge only supports one control plane key. If you need multiple control planes to push agents, run separate Bridge instances.

---

## See Also

- [Push API](push-api.md) — Authenticated endpoints
- [Configuration](../getting-started/configuration.md) — Setting the API key
