---
title: Identities
description: Identity management for user-level rate limiting, external ID mapping, and request attribution.
---

# Identities

Identities in LLMVault provide a way to group credentials and enforce per-user or per-entity rate limits. They bridge your application's user model with LLMVault's access control system.

## What is an Identity?

An identity represents a user, service account, or any entity that consumes LLM API quota. It enables:

- **Per-entity rate limiting** - Enforce limits across all credentials for a user
- **Request attribution** - Track usage by external user ID
- **Credential grouping** - Organize credentials by owner
- **Metadata tagging** - Store custom JSONB metadata

```go
type Identity struct {
    ID         uuid.UUID `gorm:"type:uuid;primaryKey"`
    OrgID      uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_identity_org_external"`
    ExternalID string    `gorm:"not null;uniqueIndex:idx_identity_org_external"`
    Meta       JSON      `gorm:"type:jsonb;default:'{}'"`
    CreatedAt  time.Time
    UpdatedAt  time.Time

    RateLimits []IdentityRateLimit `gorm:"foreignKey:IdentityID;constraint:OnDelete:CASCADE"`
}
```

## External IDs

Identities are primarily identified by an **external ID** - a string that maps to your application's user identifier:

| Property | Description |
|----------|-------------|
| Scoped to org | Same external ID can exist in different orgs |
| Unique constraint | `(org_id, external_id)` is unique |
| String type | Supports any ID format (UUIDs, numbers, emails) |

Example external IDs:

- `user_12345`
- `550e8400-e29b-41d4-a716-446655440000`
- `alice@example.com`
- `service_account_prod`

## Creating Identities

### Explicit Creation

```bash
curl -X POST https://api.llmvault.dev/v1/identities \
  -H "Authorization: Bearer {org_token}" \
  -H "Content-Type: application/json" \
  -d '{
    "external_id": "user_12345",
    "meta": {"plan": "premium", "team": "engineering"}
  }'
```

### Auto-Creation via Credentials

Identities are automatically created when specifying `external_id` during credential creation:

```bash
curl -X POST https://api.llmvault.dev/v1/credentials \
  -H "Authorization: Bearer {org_token}" \
  -d '{
    "provider_id": "openai",
    "base_url": "https://api.openai.com/v1",
    "auth_scheme": "bearer",
    "api_key": "sk-...",
    "external_id": "user_12345"
  }'
```

Auto-creation logic:

```go
if req.ExternalID != nil && *req.ExternalID != "" {
    var ident model.Identity
    err := db.Where("external_id = ? AND org_id = ?", *req.ExternalID, org.ID).First(&ident).Error
    if err == gorm.ErrRecordNotFound {
        // Create new identity
        ident = model.Identity{
            ID:         uuid.New(),
            OrgID:      org.ID,
            ExternalID: *req.ExternalID,
        }
        db.Create(&ident)
    }
    identityID = &ident.ID
}
```

## Identity Rate Limits

Identities support multiple named rate limits with configurable windows:

```go
type IdentityRateLimit struct {
    ID         uuid.UUID `gorm:"type:uuid;primaryKey"`
    IdentityID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_identity_ratelimit_name"`
    Name       string    `gorm:"not null;uniqueIndex:idx_identity_ratelimit_name"`
    Limit      int64     `gorm:"not null"`
    Duration   int64     `gorm:"not null"` // milliseconds
    CreatedAt  time.Time
    UpdatedAt  time.Time
}
```

### Rate Limit Fields

| Field | Description |
|-------|-------------|
| `Name` | Identifier for this rate limit (e.g., "default", "burst") |
| `Limit` | Maximum requests allowed |
| `Duration` | Time window in **milliseconds** |

### Example Rate Limits

| Name | Limit | Duration | Effective Rate |
|------|-------|----------|----------------|
| `default` | 100 | 60000 | 100 req/min |
| `burst` | 10 | 1000 | 10 req/sec |
| `hourly` | 1000 | 3600000 | 1000 req/hour |

### Creating Rate Limits

```bash
curl -X POST https://api.llmvault.dev/v1/identities/{identity_id}/rate-limits \
  -H "Authorization: Bearer {org_token}" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "default",
    "limit": 100,
    "duration": 60000
  }'
```

## Rate Limit Enforcement

Identity rate limits are enforced by the `IdentityRateLimit` middleware:

### Check Flow

```
1. Extract credential ID from token claims
2. Look up credential's identity_id
3. Set identity ID on context (for audit logging)
4. If no identity → skip rate limiting
5. Load all rate limits for identity
6. For each rate limit:
   a. Build Redis key: pbrl:ident:{identity_id}:{name}
   b. Execute atomic Lua script
   c. If limit exceeded → return 429
```

### Redis Lua Script

```lua
local key = KEYS[1]
local limit = tonumber(ARGV[1])
local window_ms = tonumber(ARGV[2])

local current = redis.call("GET", key)
if current == false then
    redis.call("SET", key, 1, "PX", window_ms)
    return 1  -- Allowed
end

local n = tonumber(current)
if n >= limit then
    return 0  -- Exceeded
end

redis.call("INCR", key)
return 1  -- Allowed
```

### Rate Limit Key Format

```
pbrl:ident:{identity_uuid}:{rate_limit_name}
```

Example: `pbrl:ident:550e8400-e29b-41d4-a716-446655440000:default`

### Response on Rate Limit

```json
{
  "error": "identity rate limit exceeded: default"
}
```

With `Retry-After` header indicating seconds until reset.

### Fail-Open Behavior

If Redis is unavailable, identity rate limiting fails open:

```go
result, err := identityRateLimitScript.Run(...)
if err != nil {
    slog.Warn("identity_ratelimit: redis error, failing open", ...)
    continue  // Skip this rate limit check
}
```

## Request Attribution

When a credential is linked to an identity, all proxy requests are attributed to that identity:

### Audit Logging

```go
type AuditEntry struct {
    ID           uuid.UUID
    OrgID        uuid.UUID
    CredentialID uuid.UUID
    IdentityID   *uuid.UUID  // Set when credential has identity
    Action       string
    Method       string
    Path         string
    StatusCode   int
    Duration     int64
    CreatedAt    time.Time
}
```

### Usage Tracking

Query usage by identity:

```sql
SELECT 
    identity_id,
    COUNT(*) as request_count,
    MAX(created_at) as last_used_at
FROM audit_log
WHERE org_id = ? 
  AND action = 'proxy.request'
  AND identity_id = ?
GROUP BY identity_id
```

## Filtering Credentials by Identity

```bash
# Filter by identity ID
curl "https://api.llmvault.dev/v1/credentials?identity_id={uuid}"

# Filter by external ID
curl "https://api.llmvault.dev/v1/credentials?external_id=user_12345"
```

## Metadata

Identities support JSONB metadata for custom tagging:

```json
{
  "plan": "premium",
  "team": "engineering",
  "cost_center": "ai-research"
}
```

Metadata can be filtered using PostgreSQL's JSONB containment operator:

```sql
SELECT * FROM identities 
WHERE meta @> '{"plan": "premium"}'
```

## Lifecycle

### Identity Deletion

When an identity is deleted:

1. All associated credentials have `identity_id` set to `NULL`
2. All rate limits are cascaded deleted
3. Audit log entries retain the identity ID for historical tracking

```go
type Credential struct {
    IdentityID *uuid.UUID
    Identity   *Identity `gorm:"constraint:OnDelete:SET NULL"`
}

type IdentityRateLimit struct {
    IdentityID uuid.UUID `gorm:"constraint:OnDelete:CASCADE"`
}
```

## Best Practices

### 1. Use Consistent External IDs

Use the same external ID format throughout your application:

```go
// Good: Consistent prefix
"user_" + user.ID
"svc_" + service.Name

// Avoid: Mixed formats
user.ID        // Sometimes just ID
"u:" + user.ID // Sometimes with prefix
```

### 2. Set Appropriate Rate Limits

Consider your use case when setting limits:

```go
// Interactive user
{limit: 60, duration: 60000}   // 1 req/sec average

// Batch processing
{limit: 1000, duration: 60000} // 1000 req/min

// Service account
{limit: 10000, duration: 60000} // High throughput
```

### 3. Use Multiple Rate Limits

Layer rate limits for burst and sustained protection:

```go
// Burst protection: 10 req/sec
{name: "burst", limit: 10, duration: 1000}

// Sustained: 100 req/min
{name: "default", limit: 100, duration: 60000}
```

### 4. Tag with Metadata

Use metadata for filtering and reporting:

```json
{
  "environment": "production",
  "team": "ml-platform",
  "cost_center": "engineering",
  "data_classification": "internal"
}
```

## Security Considerations

1. **Isolation** - Rate limits are per-identity, not shared
2. **No credential exposure** - Identities don't store or expose API keys
3. **Audit trail** - All requests attributed to identity for compliance
4. **Soft deletion** - Credentials remain functional if identity is deleted
