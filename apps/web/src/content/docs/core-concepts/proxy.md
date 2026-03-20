---
title: Proxy
description: How LLMVault's streaming reverse proxy works, including request routing, three-tier caching, and error handling.
---

# Proxy

The LLMVault proxy is a streaming reverse proxy that forwards requests to upstream LLM providers while transparently handling authentication, caching, and security.

## How the Proxy Works

### Request Flow

```
Client → Token Auth → Rate Limits → Remaining Check → Cache Resolve → Upstream
           ↓              ↓              ↓                ↓
        Validate      Identity      Token/Cred      Decrypt via
        JWT           Rate Limit    Counters        3-tier cache
```

### URL Rewriting

Requests to `/v1/proxy/*` are rewritten to the credential's upstream:

```
/v1/proxy/v1/chat/completions → https://api.openai.com/v1/chat/completions
/v1/proxy/v1/embeddings      → https://api.openai.com/v1/embeddings
```

The proxy strips the `/v1/proxy` prefix and appends the remainder to the credential's `base_url`.

### Authentication Swap

The incoming sandbox token is replaced with the real API key:

1. Extract claims from JWT
2. Resolve credential from 3-tier cache
3. Strip `Authorization` header
4. Attach real API key using credential's `auth_scheme`

```go
req.Header.Del("Authorization")
AttachAuth(req, cred.AuthScheme, cred.APIKey)
```

## Director Function

The `Director` function transforms incoming requests:

```go
func NewDirector(cacheManager *cache.Manager) func(req *http.Request) {
    return func(req *http.Request) {
        // 1. Extract claims
        claims, ok := middleware.ClaimsFromContext(req.Context())
        
        // 2. Resolve credential (L1 → L2 → L3)
        cred, err := cacheManager.GetDecryptedCredential(req.Context(), claims.CredentialID, orgID)
        
        // 3. SSRF validation
        if err := ValidateBaseURL(cred.BaseURL); err != nil {
            req.Header.Set("X-Proxy-Error", "disallowed upstream")
            return
        }
        
        // 4. Strip metadata headers (cloud SSRF protection)
        for _, h := range []string{
            "Metadata-Flavor",
            "X-Aws-Ec2-Metadata-Token",
            "X-Aws-Ec2-Metadata-Token-Ttl-Seconds",
            "Metadata",
        } {
            req.Header.Del(h)
        }
        
        // 5. URL rewriting
        upstreamPath := stripProxyPrefix(req.URL.Path)
        req.URL.Scheme = "https" // or http
        req.URL.Host = parsedHost
        req.URL.Path = basePath + upstreamPath
        
        // 6. Auth swap
        req.Header.Del("Authorization")
        AttachAuth(req, cred.AuthScheme, cred.APIKey)
        
        // 7. Zero out plaintext key
        for i := range cred.APIKey {
            cred.APIKey[i] = 0
        }
        
        // 8. Set tracing header
        req.Header.Set("X-Request-ID", uuid.New().String())
    }
}
```

## Streaming Support

### SSE (Server-Sent Events)

The proxy immediately flushes SSE chunks for streaming responses:

```go
rp := &httputil.ReverseProxy{
    Director:      director,
    Transport:     transport,
    FlushInterval: -1, // immediate SSE streaming
    ErrorHandler:  errorHandler,
}
```

Setting `FlushInterval: -1` ensures real-time streaming for LLM completions.

### Request Methods

All HTTP methods are supported:

- `POST` - Chat completions, embeddings
- `GET` - Model listings, status checks
- `DELETE` - Resource deletion
- `PUT`/`PATCH` - Updates

## Three-Tier Cache

The proxy resolves credentials through a sophisticated 3-tier cache:

### Performance Characteristics

| Tier | Latency | Data State |
|------|---------|------------|
| L1 (Memory) | ~0.01ms | Decrypted, memguard sealed |
| L2 (Redis) | ~0.5ms | Still encrypted (DEK + KMS) |
| L3 (DB + KMS) | ~3-8ms | Wrapped DEK, encrypted key |

### L1: In-Memory Cache

```go
type MemoryCache struct {
    lru *expirable.LRU[string, *CachedCredential]
}

type CachedCredential struct {
    Enclave    *memguard.Enclave  // Sealed API key
    BaseURL    string
    AuthScheme string
    OrgID      uuid.UUID
    CachedAt   time.Time
    HardExpiry time.Time
}
```

- Max size: 10,000 entries
- TTL: 5 minutes
- Hard expiry: 15 minutes

### L2: Redis Cache

```go
type RedisCredential struct {
    EncryptedKey []byte `json:"ek"` // Still DEK-encrypted
    WrappedDEK   []byte `json:"wd"` // Still KMS-wrapped
    BaseURL      string `json:"bu"`
    AuthScheme   string `json:"as"`
    OrgID        string `json:"oi"`
}
```

- Key prefix: `pbcred:{credential_id}`
- TTL: 30 minutes
- Values remain encrypted in Redis

### L3: Database + KMS

Cold path when L1 and L2 miss:

1. Query Postgres: `SELECT * FROM credentials WHERE id = ? AND revoked_at IS NULL`
2. Unwrap DEK via KMS
3. Decrypt API key with DEK
4. Promote to L2 and L1

### Singleflight Deduplication

Concurrent requests for the same credential are deduplicated:

```go
v, err, _ := m.flight.Do(credentialID, func() (any, error) {
    return m.resolveFromLowerTiers(ctx, credentialID, orgID)
})
```

## Error Handling

### Proxy Error Detection

The director sets `X-Proxy-Error` headers for error conditions:

```go
if !ok {
    req.Header.Set("X-Proxy-Error", "missing claims")
    return
}
```

### Error Handler

The error handler checks for director-set errors:

```go
ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
    if proxyErr := r.Header.Get("X-Proxy-Error"); proxyErr != "" {
        http.Error(w, `{"error":"`+proxyErr+`"}`, http.StatusBadGateway)
        return
    }
    // Log and return generic error
    slog.Error("proxy upstream error", "error", err, ...)
    http.Error(w, `{"error":"upstream unreachable"}`, http.StatusBadGateway)
}
```

### Error Types

| Error | Cause | Status Code |
|-------|-------|-------------|
| `missing claims` | Token auth failed | 502 |
| `invalid org_id` | Claims parsing error | 502 |
| `credential error` | Cache resolution failed | 502 |
| `disallowed upstream` | SSRF validation failed | 502 |
| `upstream unreachable` | Network/connectivity error | 502 |

## SSRF Protection

The proxy validates upstream URLs against SSRF attacks:

### Blocked Networks

**IPv4:**
- Loopback: `127.0.0.0/8`
- Link-local: `169.254.0.0/16`
- Private: `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`
- CGNAT: `100.64.0.0/10`
- Multicast: `224.0.0.0/4`
- Reserved: `240.0.0.0/4`

**IPv6:**
- Loopback: `::1/128`
- Link-local: `fe80::/10`
- Unique local: `fc00::/7`
- Multicast: `ff00::/8`

### Blocked Hostnames

```go
blockedHostnames = map[string]struct{}{
    "localhost":                {},
    "localhost.localdomain":    {},
    "metadata.google.internal": {},
    "metadata":                 {},
}
```

### Validation Process

1. Parse URL scheme (must be `http` or `https`)
2. Check for blocked hostnames
3. If IP literal: validate against disallowed networks
4. If hostname: DNS resolve and validate all returned IPs

## Authentication Attachment

The proxy supports multiple auth schemes:

```go
func AttachAuth(req *http.Request, scheme string, apiKey []byte) {
    switch scheme {
    case "bearer":
        req.Header.Set("Authorization", "Bearer "+string(apiKey))
    case "x-api-key":
        req.Header.Set("x-api-key", string(apiKey))
    case "api-key":
        req.Header.Set("api-key", string(apiKey))
    case "query_param":
        q := req.URL.Query()
        q.Set("key", string(apiKey))
        req.URL.RawQuery = q.Encode()
    }
}
```

## Cross-Instance Invalidation

Cache invalidation propagates across all proxy instances via Redis pub/sub:

```go
const (
    CredentialChannel = "llmvault:invalidate:credential"
    TokenChannel      = "llmvault:invalidate:token"
)
```

When a credential is revoked, all instances purge their L1 cache.

## Request Logging

Every proxy request is logged to the audit log:

```go
type AuditEntry struct {
    ID           uuid.UUID
    OrgID        uuid.UUID
    CredentialID uuid.UUID
    IdentityID   *uuid.UUID
    Action       string  // "proxy.request"
    Method       string  // HTTP method
    Path         string  // Request path
    StatusCode   int
    Duration     int64   // milliseconds
    CreatedAt    time.Time
}
```

## Usage Example

```bash
# Mint a token
curl -X POST https://api.llmvault.dev/v1/tokens \
  -H "Authorization: Bearer {org_token}" \
  -d '{"credential_id": "...", "ttl": "1h"}'

# Use token to call OpenAI through proxy
curl https://api.llmvault.dev/v1/proxy/v1/chat/completions \
  -H "Authorization: Bearer ptok_..." \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

## Security Properties

1. **No key exposure**: Real API keys never leave the proxy
2. **Memory protection**: Keys sealed in `memguard` enclaves
3. **Zeroization**: Keys wiped immediately after use
4. **SSRF hardening**: Multi-layer IP and hostname validation
5. **Request isolation**: Each request gets fresh credential resolution
6. **Audit trail**: Every request logged with credential and identity
