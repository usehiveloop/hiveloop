---
title: Rate Limiting
description: Comprehensive rate limiting at organization, identity, token, and credential levels with Redis-backed counters and lazy refill.
---

# Rate Limiting

LLMVault implements a multi-layered rate limiting system that operates at organization, identity, token, and credential levels. Each layer provides different granularity and use cases.

## Rate Limit Layers

```
┌─────────────────────────────────────────────────────────────┐
│                    Organization Rate Limit                   │
│           (Global per-org request rate - in-memory)          │
├─────────────────────────────────────────────────────────────┤
│                    Identity Rate Limits                      │
│        (Per-user/entity limits - Redis backed)               │
├─────────────────────────────────────────────────────────────┤
│                      Token Quotas                            │
│         (Per-token request caps - Redis backed)              │
├─────────────────────────────────────────────────────────────┤
│                    Credential Caps                           │
│       (Per-credential request caps - Redis backed)           │
└─────────────────────────────────────────────────────────────┘
```

## Organization Rate Limits

Organization rate limits enforce a global request rate per organization using in-memory token buckets.

### Configuration

```go
type Org struct {
    ID        uuid.UUID
    Name      string
    RateLimit int  `gorm:"not null;default:1000"` // requests per minute
    // ...
}
```

Default: **1000 requests per minute**

### Implementation

Uses `golang.org/x/time/rate` for token bucket rate limiting:

```go
func RateLimit() func(http.Handler) http.Handler {
    var mu sync.Mutex
    limiters := make(map[string]*rate.Limiter)

    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            org := OrgFromContext(r.Context())
            orgID := org.ID.String()

            mu.Lock()
            limiter, exists := limiters[orgID]
            if !exists {
                // Convert per-minute to per-second rate
                rps := rate.Limit(float64(org.RateLimit) / 60.0)
                burst := max(org.RateLimit/10, 1)
                limiter = rate.NewLimiter(rps, burst)
                limiters[orgID] = limiter
            }
            mu.Unlock()

            if !limiter.Allow() {
                w.Header().Set("Retry-After", "1")
                writeJSON(w, 429, map[string]string{"error": "rate limit exceeded"})
                return
            }

            next.ServeHTTP(w, r)
        })
    }
}
```

### Burst Calculation

```go
burst := max(org.RateLimit/10, 1)
```

| Rate Limit | Burst | RPS |
|------------|-------|-----|
| 1000/min | 100 | 16.67 |
| 6000/min | 600 | 100 |
| 100/min | 10 | 1.67 |

### Response on Limit

```json
{"error": "rate limit exceeded"}
```

With `Retry-After: 1` header (seconds).

## Identity Rate Limits

Identity rate limits provide per-user or per-entity request limiting backed by Redis.

### Rate Limit Definition

```go
type IdentityRateLimit struct {
    ID         uuid.UUID
    IdentityID uuid.UUID
    Name       string  // e.g., "default", "burst"
    Limit      int64   // max requests
    Duration   int64   // window in milliseconds
}
```

### Multiple Rate Limits per Identity

An identity can have multiple named rate limits:

| Name | Limit | Duration | Purpose |
|------|-------|----------|---------|
| `default` | 100 | 60000 | 100 req/min sustained |
| `burst` | 10 | 1000 | 10 req/sec burst |

### Redis Implementation

Atomic Lua script for rate limit check:

```lua
local key = KEYS[1]
local limit = tonumber(ARGV[1])
local window_ms = tonumber(ARGV[2])

local current = redis.call("GET", key)
if current == false then
    -- First request in window
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

### Key Format

```
pbrl:ident:{identity_uuid}:{rate_limit_name}
```

Example: `pbrl:ident:550e8400-e29b-41d4-a716-446655440000:default`

### Window Behavior

- First request creates key with TTL = window duration
- Subsequent requests increment counter
- When limit reached, all requests rejected until TTL expires
- Window is sliding (resets after duration from first request)

### Retry-After Calculation

```go
ttl, _ := rdb.PTTL(ctx, key).Result()
retryAfter := int(ttl / time.Millisecond / 1000)
if retryAfter <= 0 {
    retryAfter = 1
}
w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfter))
```

## Token Quotas

Token quotas limit the total number of requests a single token can make.

### Configuration

```go
type Token struct {
    ID             uuid.UUID
    Remaining      *int64     // Current request budget
    RefillAmount   *int64     // Amount to refill
    RefillInterval *string    // Go duration string
    LastRefillAt   *time.Time
    // ...
}
```

### Redis Counter

Key format: `pbreq:tok:{jti}`

Counter is seeded at token creation:

```go
func (c *Counter) SeedToken(ctx context.Context, jti string, value int64, tokenTTL time.Duration) error {
    return c.Seed(ctx, tokKey(jti), value, tokenTTL+time.Minute)
}
```

The counter TTL is set to token expiry + 1 minute buffer.

### Decrement Algorithm

```lua
local v = redis.call("GET", KEYS[1])
if v == false then
    return -1  -- No cap configured
end
local n = tonumber(v)
if n <= 0 then
    return 0   -- Exhausted
end
redis.call("DECR", KEYS[1])
return 1       -- Success
```

Return values:
- `1` (DecrOK) - Decremented successfully
- `0` (DecrExhausted) - Counter at 0
- `-1` (DecrNoCap) - No cap configured

### Lazy Refill

When a token quota is exhausted, lazy refill is attempted:

```go
if tokResult == counter.DecrExhausted {
    refilled, err := ctr.CheckAndRefillToken(ctx, claims.JTI)
    if refilled {
        // Retry decrement after refill
        tokResult, _ = ctr.Decrement(ctx, counter.TokKey(claims.JTI))
    }
    if tokResult == counter.DecrExhausted {
        writeJSON(w, 429, map[string]string{"error": "token request cap exhausted"})
        return
    }
}
```

## Credential Caps

Credential caps limit total requests per credential, shared across all tokens.

### Configuration

```go
type Credential struct {
    Remaining      *int64
    RefillAmount   *int64
    RefillInterval *string
    LastRefillAt   *time.Time
    // ...
}
```

### Redis Counter

Key format: `pbreq:cred:{credential_id}`

Seeded at credential creation:

```go
if cred.Remaining != nil && h.counter != nil {
    _ = h.counter.SeedCredential(r.Context(), cred.ID.String(), *cred.Remaining)
}
```

### Counter Lifecycle

Credential counters have no TTL - they persist until manually removed or credential is revoked.

### Two-Phase Check

Token and credential counters are checked in sequence:

```go
// 1. Decrement token counter
tokResult, _ := ctr.Decrement(ctx, counter.TokKey(claims.JTI))

// 2. Decrement credential counter  
credResult, _ := ctr.Decrement(ctx, counter.CredKey(claims.CredentialID))

// 3. If credential rejected, rollback token decrement
if credResult == counter.DecrExhausted {
    if tokResult == counter.DecrOK {
        _ = ctr.Undo(ctx, counter.TokKey(claims.JTI))
    }
    writeJSON(w, 429, map[string]string{"error": "credential request cap exhausted"})
    return
}
```

## Refill Mechanics

### Refill Algorithm

Both tokens and credentials support automatic refill:

```go
func (c *Counter) CheckAndRefillCredential(ctx context.Context, credentialID string) (bool, error) {
    // 1. Load credential from DB
    var cred model.Credential
    db.Where("id = ?", credentialID).First(&cred)

    // 2. Check if refill configured
    if cred.RefillAmount == nil || cred.RefillInterval == nil {
        return false, nil
    }

    // 3. Parse interval
    interval, _ := time.ParseDuration(*cred.RefillInterval)

    // 4. Check if refill due
    now := time.Now()
    lastRefill := cred.CreatedAt
    if cred.LastRefillAt != nil {
        lastRefill = *cred.LastRefillAt
    }
    if now.Sub(lastRefill) < interval {
        return false, nil  // Not yet due
    }

    // 5. Optimistic locking update
    result := db.Model(&model.Credential{}).
        Where("id = ? AND (last_refill_at = ? OR (last_refill_at IS NULL AND ? = ?))",
            credentialID, lastRefill, lastRefill, cred.CreatedAt).
        Updates(map[string]any{
            "remaining":      *cred.RefillAmount,
            "last_refill_at": now,
        })

    if result.RowsAffected == 0 {
        return false, nil  // Another instance did the refill
    }

    // 6. Reset Redis counter
    c.SeedCredential(ctx, credentialID, *cred.RefillAmount)
    return true, nil
}
```

### Optimistic Locking

Prevents double-refill in concurrent scenarios:

```sql
UPDATE credentials
SET remaining = $1, last_refill_at = $2
WHERE id = $3 
  AND (last_refill_at = $4 OR (last_refill_at IS NULL AND $4 = created_at))
```

### Refill Interval Formats

Valid Go duration strings:

| Interval | Description |
|----------|-------------|
| `15m` | 15 minutes |
| `1h` | 1 hour |
| `24h` | 24 hours |
| `168h` | 7 days |

## Fail-Open Behavior

If Redis is unavailable, rate limiting fails open:

```go
tokResult, err := ctr.Decrement(ctx, counter.TokKey(claims.JTI))
if err != nil {
    slog.Warn("remaining: redis error on token decrement, failing open", ...)
    next.ServeHTTP(w, r)  // Allow request
    return
}
```

This ensures high availability at the cost of temporary quota enforcement.

## Rate Limit Hierarchy

Requests must pass all rate limit layers:

```
1. Organization rate limit (in-memory token bucket)
   └─> If exceeded: 429 with Retry-After: 1

2. Identity rate limits (Redis atomic script)
   └─> If any exceeded: 429 with Retry-After: seconds_until_reset

3. Token quota (Redis DECR)
   └─> Try lazy refill first
   └─> If exhausted: 429 "token request cap exhausted"

4. Credential cap (Redis DECR)
   └─> Rollback token decrement if failed
   └─> Try lazy refill first
   └─> If exhausted: 429 "credential request cap exhausted"
```

## Configuration Examples

### Development Environment

```json
{
  "org_rate_limit": 1000,
  "identity_rate_limits": [
    {"name": "default", "limit": 60, "duration": 60000}
  ],
  "token": {
    "remaining": 1000,
    "refill_amount": 1000,
    "refill_interval": "24h"
  }
}
```

### Production User

```json
{
  "identity_rate_limits": [
    {"name": "burst", "limit": 20, "duration": 1000},
    {"name": "default", "limit": 1000, "duration": 60000}
  ],
  "credentials": [
    {
      "remaining": 10000,
      "refill_amount": 10000,
      "refill_interval": "1h"
    }
  ],
  "tokens": [
    {
      "remaining": 100,
      "refill_amount": null,
      "refill_interval": null
    }
  ]
}
```

### Service Account

```json
{
  "identity_rate_limits": [
    {"name": "default", "limit": 10000, "duration": 60000}
  ],
  "credential": {
    "remaining": 1000000,
    "refill_amount": 1000000,
    "refill_interval": "24h"
  }
}
```

## Monitoring Rate Limits

### Redis Key Patterns

```
pbrl:ident:*      # Identity rate limit counters
pbreq:tok:*       # Token request counters
pbreq:cred:*      # Credential request counters
```

### Audit Log Queries

```sql
-- Find rate-limited requests
SELECT * FROM audit_log 
WHERE org_id = ? 
  AND status_code = 429
  AND created_at > NOW() - INTERVAL '1 hour';

-- Usage by identity
SELECT 
    identity_id,
    COUNT(*) as requests,
    COUNT(CASE WHEN status_code = 429 THEN 1 END) as rate_limited
FROM audit_log
WHERE org_id = ?
GROUP BY identity_id;
```

## Best Practices

1. **Set identity rate limits** for multi-tenant applications
2. **Use short-lived tokens** with tight quotas for untrusted clients
3. **Configure refill** for long-lived credentials
4. **Layer rate limits** - burst + sustained protection
5. **Monitor Redis** for counter consistency
6. **Set org rate limits** appropriate for your infrastructure capacity
