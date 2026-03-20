---
title: Tokens
description: Short-lived proxy tokens with JWT-based authentication, TTL constraints, scope-based access control, and MCP endpoint support.
---

# Tokens

Tokens are short-lived JWTs that grant scoped access to LLM credentials. They enable secure, temporary access for applications, users, or integrations without exposing the underlying API key.

## Token Anatomy

LLMVault tokens are signed JWTs with the following structure:

### Header

```json
{
  "alg": "HS256",
  "typ": "JWT"
}
```

### Claims

```go
type Claims struct {
    OrgID        string `json:"org_id"`      // Organization UUID
    CredentialID string `json:"cred_id"`     // Target credential UUID
    ScopeHash    string `json:"scope_hash"`  // SHA-256 of scopes (if present)
    jwt.RegisteredClaims
}
```

The registered claims include:

| Claim | Description |
|-------|-------------|
| `jti` | Unique token ID (UUID) |
| `iat` | Issued at timestamp |
| `exp` | Expiration timestamp |

### Token Format

Tokens are prefixed with `ptok_` for identification:

```
ptok_eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
```

The prefix is stripped during validation.

## Token Data Model

```go
type Token struct {
    ID             uuid.UUID  `gorm:"type:uuid;primaryKey"`
    OrgID          uuid.UUID  `gorm:"type:uuid;not null"`
    CredentialID   uuid.UUID  `gorm:"type:uuid;not null;index"`
    JTI            string     `gorm:"column:jti;not null;uniqueIndex"`
    ExpiresAt      time.Time  `gorm:"not null"`
    Remaining      *int64     // Optional request cap
    RefillAmount   *int64     // Auto-refill amount
    RefillInterval *string    // Go duration format
    LastRefillAt   *time.Time
    Scopes         JSON       `gorm:"type:jsonb"` // MCP scopes
    Meta           JSON       `gorm:"type:jsonb;default:'{}'"`
    RevokedAt      *time.Time
    CreatedAt      time.Time
}
```

## TTL and Expiry

### Maximum TTL

Tokens have a maximum lifetime of **24 hours**:

```go
const maxTokenTTL = 24 * time.Hour
```

Attempting to mint a token with longer TTL returns:

```json
{"error": "ttl exceeds maximum of 24h"}
```

### Default TTL

If not specified, tokens default to **1 hour**:

```go
ttl := time.Hour // default
if req.TTL != "" {
    ttl, err = time.ParseDuration(req.TTL)
}
```

### Valid TTL Formats

Any valid Go duration string:

- `15m` - 15 minutes
- `1h` - 1 hour  
- `4h30m` - 4 hours 30 minutes
- `24h` - Maximum allowed

## Minting a Token

```bash
curl -X POST https://api.llmvault.dev/v1/tokens \
  -H "Authorization: Bearer {org_token}" \
  -H "Content-Type: application/json" \
  -d '{
    "credential_id": "550e8400-e29b-41d4-a716-446655440000",
    "ttl": "4h",
    "remaining": 100,
    "refill_amount": 100,
    "refill_interval": "1h",
    "scopes": [
      {
        "connection_id": "conn-uuid",
        "actions": ["chat.completions", "embeddings"],
        "resources": {
          "models": ["gpt-4", "gpt-3.5-turbo"]
        }
      }
    ],
    "meta": {"purpose": "batch-processing"}
  }'
```

### Response

```json
{
  "token": "ptok_eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "expires_at": "2024-01-15T18:30:00Z",
  "jti": "550e8400-e29b-41d4-a716-446655440001",
  "mcp_endpoint": "https://mcp.llmvault.dev/550e8400-e29b-41d4-a716-446655440001"
}
```

## Token Scoping Rules

### MCP Token Scopes

Scopes define fine-grained access control for MCP (Model Context Protocol) integrations:

```go
type TokenScope struct {
    ConnectionID string              `json:"connection_id"`
    Actions      []string            `json:"actions"`
    Resources    map[string][]string `json:"resources,omitempty"`
}
```

| Field | Description |
|-------|-------------|
| `connection_id` | UUID of the integration connection |
| `actions` | Explicit list of allowed actions (no wildcards) |
| `resources` | Optional resource type → ID mapping |

### Scope Validation

Scopes are validated against the embedded actions catalog:

1. Connection must exist, belong to org, and not be revoked
2. Integration must not be soft-deleted
3. Each action must exist in the provider's catalog
4. Wildcard actions (`*`) are **explicitly rejected**
5. Resource types must match the actions' resource types

```go
// Wildcard rejection
if action == "*" {
    return fmt.Errorf("wildcard actions are not allowed; explicitly list each action")
}
```

### Scope Hash

Scopes are hashed for inclusion in JWT claims:

```go
func ScopeHash(scopes []TokenScope) (string, error) {
    canonical, _ := json.Marshal(scopes)
    hash := sha256.Sum256(canonical)
    return fmt.Sprintf("%x", hash), nil
}
```

This enables validation that the token's scopes haven't been tampered with.

## Token Authentication

### Authorization Header

```
Authorization: Bearer ptok_{jwt_token}
```

### Validation Flow

1. Extract token from `Authorization` header
2. Strip `ptok_` prefix if present
3. Validate JWT signature (HS256)
4. Verify claims (`org_id`, `cred_id`, `exp`)
5. Check database for revocation (`revoked_at IS NULL`)
6. Place claims on request context

```go
claims, err := token.Validate(signingKey, jwtString)
if err != nil {
    return 401, "invalid or expired token"
}

// Check revocation
var tokenRecord model.Token
result := db.Where("jti = ?", claims.ID).First(&tokenRecord)
if result.Error != nil || tokenRecord.RevokedAt != nil {
    return 401, "token revoked"
}
```

## Token Request Caps

Like credentials, tokens support request caps with refill:

```json
{
  "remaining": 100,
  "refill_amount": 100,
  "refill_interval": "1h"
}
```

### Counter Key Format

Redis key: `pbreq:tok:{jti}`

### Counter TTL

Token counters expire with the token plus a 1-minute buffer:

```go
func (c *Counter) SeedToken(ctx context.Context, jti string, value int64, tokenTTL time.Duration) error {
    return c.Seed(ctx, tokKey(jti), value, tokenTTL+time.Minute)
}
```

## MCP Endpoint

When scopes are provided, the token response includes an MCP endpoint URL:

```json
{
  "mcp_endpoint": "https://mcp.llmvault.dev/{jti}"
}
```

This endpoint enables Model Context Protocol connections scoped to the token's permissions.

## Token Revocation

Tokens can be revoked before expiry:

```bash
curl -X DELETE https://api.llmvault.dev/v1/tokens/{jti} \
  -H "Authorization: Bearer {org_token}"
```

Revocation triggers:
1. `revoked_at` timestamp set in database
2. Redis revocation entry with token's original TTL
3. Cross-instance pub/sub notification
4. Cached MCP server eviction

### Revocation Check Flow

```
L1 (Memory set) → L2 (Redis: pbrev:{jti}) → L3 (Postgres revoked_at)
```

## Listing Tokens

```bash
# List all tokens
curl "https://api.llmvault.dev/v1/tokens?limit=50"

# Filter by credential
curl "https://api.llmvault.dev/v1/tokens?credential_id={uuid}"

# Cursor pagination
curl "https://api.llmvault.dev/v1/tokens?cursor=eyJpZCI6..."
```

Response:

```json
{
  "data": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "jti": "token-jti-uuid",
      "credential_id": "cred-uuid",
      "remaining": 95,
      "refill_amount": 100,
      "refill_interval": "1h",
      "scopes": {"scopes": [...]},
      "meta": {"purpose": "batch-processing"},
      "expires_at": "2024-01-15T18:30:00Z",
      "created_at": "2024-01-15T14:30:00Z"
    }
  ],
  "has_more": true,
  "next_cursor": "eyJpZCI6..."
}
```

## Security Properties

1. **Short lifetime**: Maximum 24 hours, default 1 hour
2. **Cryptographic signing**: HMAC-SHA256 with 256-bit key
3. **Unique ID**: UUID-based JTI prevents replay
4. **Revocation support**: Immediate invalidation possible
5. **Scope integrity**: SHA-256 hash in JWT claims
6. **Automatic expiry**: Redis TTLs align with token lifetime
