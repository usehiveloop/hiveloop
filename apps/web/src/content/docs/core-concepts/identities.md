---
title: Identities
description: Identity management for user-level rate limiting, external ID mapping, and request attribution.
---

# Identities

Identities let you map your application's users (or any entity) into LLMVault. By linking credentials to identities, you can enforce per-user rate limits, track usage by user, and organize credentials by owner.

## Why Use Identities?

- **Per-user rate limiting** — prevent any single user from monopolizing your LLM quota
- **Usage attribution** — see exactly which user made each request in your audit log
- **Credential organization** — group and filter credentials by the user they belong to
- **Flexible mapping** — use any string as an external ID (UUIDs, emails, usernames)

## Creating an Identity

```typescript
const vault = new LLMVault({ apiKey: "your-api-key" });

const { data, error } = await vault.identities.create({
  external_id: "user_12345",
  name: "Alice",
  meta: { plan: "premium", team: "engineering" },
});
```

Or with curl:

```bash
curl -X POST https://api.llmvault.dev/v1/identities \
  -H "Authorization: Bearer {org_token}" \
  -H "Content-Type: application/json" \
  -d '{
    "external_id": "user_12345",
    "meta": {"plan": "premium", "team": "engineering"}
  }'
```

### External IDs

The `external_id` is the bridge between your user model and LLMVault. It can be any string that uniquely identifies a user within your organization:

- `user_12345`
- `550e8400-e29b-41d4-a716-446655440000`
- `alice@example.com`
- `service_account_prod`

External IDs are unique per organization — the same external ID can exist in different orgs without conflict.

### Auto-Creation via Credentials

You don't always need to create identities explicitly. When you pass an `external_id` while creating a credential, LLMVault automatically creates the identity if it doesn't exist:

```typescript
const { data, error } = await vault.credentials.create({
  label: "User Key",
  api_key: "sk-...",
  provider_id: "openai",
  identity_id: "user_12345",
});
```

If `user_12345` doesn't exist yet, it's created automatically and linked to the credential.

## Rate Limits

Identities support multiple named rate limits, letting you enforce both burst and sustained request limits for each user.

### Setting Rate Limits

```typescript
const { data, error } = await vault.identities.create({
  external_id: "user_12345",
  rate_limit: [
    { name: "burst", limit: 10, duration: 1000 },
    { name: "default", limit: 100, duration: 60000 },
  ],
});
```

Or update rate limits on an existing identity:

```typescript
const { data, error } = await vault.identities.update("identity-uuid", {
  rate_limit: [
    { name: "burst", limit: 20, duration: 1000 },
    { name: "default", limit: 500, duration: 60000 },
  ],
});
```

### Rate Limit Fields

| Field | Description |
|-------|-------------|
| `name` | A label for this rate limit (e.g., `default`, `burst`, `hourly`) |
| `limit` | Maximum number of requests allowed in the window |
| `duration` | Time window in milliseconds |

### Common Configurations

| Use Case | Name | Limit | Duration | Effective Rate |
|----------|------|-------|----------|----------------|
| Interactive user | `default` | 60 | 60000 | 1 req/sec average |
| Burst protection | `burst` | 10 | 1000 | 10 req/sec max |
| Hourly cap | `hourly` | 1000 | 3600000 | 1,000 req/hour |
| Batch processing | `default` | 1000 | 60000 | 1,000 req/min |
| High-throughput service | `default` | 10000 | 60000 | 10,000 req/min |

### How Rate Limits Work

When a proxied request comes in through a credential linked to an identity:

1. All rate limits for the identity are checked
2. If any limit is exceeded, the request is rejected with `429`
3. The response includes a `Retry-After` header with seconds until the window resets

```json
{"error": "identity rate limit exceeded: burst"}
```

You can layer multiple rate limits on a single identity. For example, a `burst` limit of 10 req/sec prevents spikes, while a `default` limit of 100 req/min caps sustained throughput. All limits must pass for a request to proceed.

### Fail-Open Behavior

If the rate limiting backend (Redis) is temporarily unavailable, rate limits fail open — requests are allowed through. This ensures your application remains available even during infrastructure issues.

## Usage Attribution

When a credential is linked to an identity, every proxy request is attributed to that identity in the audit log. This lets you answer questions like:

- How many requests did user X make today?
- Which users are approaching their rate limits?
- What's the usage breakdown by team or plan?

```typescript
const { data, error } = await vault.audit.list({
  action: "proxy.request",
});
```

## Listing and Filtering

```typescript
// List all identities
const { data, error } = await vault.identities.list();

// Filter by external ID
const { data, error } = await vault.identities.list({
  external_id: "user_12345",
});

// Filter by metadata
const { data, error } = await vault.identities.list({
  meta: { plan: "premium" },
});

// Paginate
const { data, error } = await vault.identities.list({
  limit: 50,
  cursor: "eyJpZCI6...",
});
```

### Filtering Credentials by Identity

Once you've linked credentials to identities, you can filter credentials by identity:

```typescript
// By identity ID
const { data, error } = await vault.credentials.list({
  identity_id: "identity-uuid",
});

// By external ID
const { data, error } = await vault.credentials.list({
  external_id: "user_12345",
});
```

## Metadata

Use metadata to store custom attributes on identities for filtering and reporting:

```typescript
const { data, error } = await vault.identities.create({
  external_id: "user_12345",
  meta: {
    plan: "premium",
    team: "ml-platform",
    cost_center: "engineering",
  },
});
```

Metadata supports JSONB containment queries, so you can filter identities by any combination of metadata fields.

## Deleting an Identity

```typescript
const { data, error } = await vault.identities.delete("identity-uuid");
```

When an identity is deleted:

- Credentials linked to it remain functional, but their identity link is removed
- All rate limits for the identity are deleted
- Audit log entries retain the identity reference for historical tracking

## Best Practices

1. **Use consistent external IDs** — pick one format (e.g., `user_{id}` or `svc_{name}`) and use it everywhere
2. **Layer rate limits** — combine burst (e.g., 10 req/sec) and sustained (e.g., 100 req/min) limits for effective traffic shaping
3. **Tag with metadata** — use metadata fields like `plan`, `team`, and `environment` for filtering and cost attribution
4. **Let identities auto-create** — pass `external_id` when creating credentials to avoid managing identity lifecycle separately
