# llmvault — Implementation Plan

## Overview

A multi-tenant secret-custodian + streaming reverse proxy for LLM API calls.
Stores customer LLM API keys securely, mints scoped sandbox tokens, and proxies
requests to any LLM provider with sub-5ms hot-path latency.

**Architecture: 3-tier cache for sub-millisecond hot path**
```
L1: In-memory (memguard + LRU)    ~0.01ms   per-instance, sealed memory
L2: Redis                          ~0.5ms    shared across all proxy instances
L3: Postgres + Vault Transit       ~3-8ms    source of truth, cold path only
```

**HA model: stateless proxy, scale horizontally**
```
         ┌─── Load Balancer ───┐
         │                     │
         ▼          ▼          ▼
     ┌────────┐ ┌────────┐ ┌────────┐
     │Proxy 1 │ │Proxy 2 │ │Proxy 3 │   ← stateless, N instances
     └───┬────┘ └───┬────┘ └───┬────┘
         │          │          │
         ▼          ▼          ▼
     ┌──────────────────────────────┐
     │          Redis (Sentinel)    │   ← L2 cache + pub/sub invalidation
     └──────────────┬───────────────┘
                    │
     ┌──────────────▼───────────────┐
     │  PostgreSQL (Patroni HA)     │   ← source of truth
     └──────────────┬───────────────┘
                    │
     ┌──────────────▼───────────────┐
     │  Vault (Raft HA cluster)     │   ← Transit KMS
     └──────────────────────────────┘
```

---

## Project Structure

```
llmvault/
├── cmd/
│   └── server/
│       └── main.go                    # Entrypoint: config loading, dependency wiring, server start
│
├── internal/
│   ├── config/
│   │   └── config.go                  # Env-based config struct (caarlos0/env)
│   │
│   ├── crypto/
│   │   ├── envelope.go                # Envelope encryption: generate DEK, encrypt/decrypt credentials
│   │   ├── envelope_test.go
│   │   ├── vault.go                   # Vault Transit client: wrap/unwrap DEKs
│   │   └── vault_test.go
│   │
│   ├── middleware/
│   │   ├── orgauth.go                 # Org API key authentication middleware
│   │   ├── orgauth_test.go
│   │   ├── tokenauth.go              # Sandbox token (JWT) authentication middleware
│   │   ├── tokenauth_test.go
│   │   ├── ratelimit.go              # Per-org rate limiting middleware
│   │   ├── ratelimit_test.go
│   │   ├── audit.go                   # Audit log middleware (writes to Postgres)
│   │   └── audit_test.go
│   │
│   ├── model/
│   │   ├── org.go                     # Org struct + DB methods
│   │   ├── credential.go             # Credential struct + DB methods
│   │   ├── token.go                   # Token struct + DB methods
│   │   └── audit.go                   # AuditEntry struct + DB methods
│   │
│   ├── handler/
│   │   ├── credentials.go            # POST/DELETE /v1/credentials
│   │   ├── credentials_test.go
│   │   ├── tokens.go                  # POST /v1/tokens
│   │   ├── tokens_test.go
│   │   ├── proxy.go                   # POST /v1/proxy/* — the streaming reverse proxy
│   │   ├── proxy_test.go
│   │   ├── orgs.go                    # POST /v1/orgs (admin: create orgs, rotate org API keys)
│   │   └── orgs_test.go
│   │
│   ├── cache/
│   │   ├── cache.go                   # CacheManager: orchestrates L1 + L2 + L3 lookup chain
│   │   ├── cache_test.go
│   │   ├── memory.go                  # L1: in-memory LRU + memguard (per-instance)
│   │   ├── memory_test.go
│   │   ├── redis.go                   # L2: Redis get/set with encrypted values + pub/sub invalidation
│   │   ├── redis_test.go
│   │   ├── invalidation.go           # Cross-instance invalidation via Redis pub/sub
│   │   └── invalidation_test.go
│   │
│   ├── proxy/
│   │   ├── director.go                # httputil.ReverseProxy Director: rewrites URL, attaches auth
│   │   ├── director_test.go
│   │   ├── transport.go               # Custom transport: timeouts, connection pooling
│   │   └── auth.go                    # attachAuth() — the 4-case auth scheme switch
│   │
│   ├── token/
│   │   ├── jwt.go                     # Mint + validate sandbox JWTs (golang-jwt)
│   │   └── jwt_test.go
│   │
│   └── db/
│       ├── db.go                      # pgx connection pool setup
│       ├── migrations/
│       │   ├── 001_create_orgs.sql
│       │   ├── 002_create_credentials.sql
│       │   ├── 003_create_tokens.sql
│       │   ├── 004_create_audit_log.sql
│       │   └── 005_create_usage.sql
│       └── migrate.go                 # Simple migration runner
│
├── e2e/
│   ├── e2e_test.go                    # Test harness: spins up full stack via docker-compose
│   ├── anthropic_test.go             # Real Anthropic streaming call
│   ├── openai_test.go                # Real OpenAI streaming call
│   ├── openrouter_test.go            # Real OpenRouter streaming call
│   ├── fireworks_test.go             # Real Fireworks streaming call
│   ├── gemini_test.go                # Real Gemini streaming call
│   ├── credential_lifecycle_test.go  # Create, use, revoke, verify denied
│   ├── token_expiry_test.go          # Mint token with short TTL, verify expiry
│   ├── tenant_isolation_test.go      # Org A cannot access Org B's credentials
│   ├── auth_schemes_test.go          # Verify all 4 auth scheme patterns work
│   ├── streaming_fidelity_test.go    # SSE chunk integrity
│   ├── cache_invalidation_test.go    # Revoke credential → all instances reject immediately
│   └── cache_tiers_test.go           # Verify L1 miss → L2 hit, L2 miss → L3 hit, promotion
│
├── docker/
│   ├── Dockerfile                     # Multi-stage: build Go binary, copy to distroless
│   ├── vault/
│   │   └── config.hcl                # Vault dev config with Transit engine enabled
│   └── postgres/
│       └── init.sql                   # Bootstrap DB and roles
│
├── docker-compose.yml                 # Postgres + Vault + Redis + Proxy for local dev/testing
├── .env.example                       # Documented env vars
├── .env.test                          # Test env vars (points to docker-compose services)
├── go.mod
├── go.sum
├── Makefile                           # build, test, test-e2e, lint, migrate, up, down
└── IMPLEMENTATION_PLAN.md
```

---

## Phase 1: Foundation (Infrastructure + Skeleton)

### Step 1.1 — Initialize Go module + dependencies

```
go mod init github.com/useportal/llmvault

go get github.com/jackc/pgx/v5
go get github.com/hashicorp/vault/api/v2
go get github.com/awnumar/memguard
go get github.com/hashicorp/golang-lru/v2
go get github.com/golang-jwt/jwt/v5
go get github.com/go-chi/chi/v5
go get github.com/caarlos0/env/v11
go get github.com/redis/go-redis/v9
```

### Step 1.2 — Docker Compose (local stack)

File: `docker-compose.yml`

Four services:
- **postgres**: PostgreSQL 16, port 5432, volume for data persistence, health check
- **vault**: Vault 1.15+ in dev mode with Transit engine auto-enabled via init script, port 8200
- **redis**: Redis 7 with AOF persistence, port 6379, health check, no auth for local dev
- **proxy**: Our Go binary, depends_on all three healthy, env vars for all config

Vault init script (`docker/vault/config.hcl` + entrypoint):
- Enable Transit secrets engine
- Create an encryption key named `llmvault-master`
- Create a policy that only allows encrypt/decrypt on that key
- Generate a token scoped to that policy (used by the proxy)

Postgres init script (`docker/postgres/init.sql`):
- Create database `llmvault`
- Create application user with limited privileges (no CREATE/DROP)
- A separate migration user for schema changes

### Step 1.3 — Config

File: `internal/config/config.go`

```go
type Config struct {
    // Server
    Port            int           `env:"PORT" envDefault:"8080"`

    // Postgres
    DatabaseURL     string        `env:"DATABASE_URL,required"`

    // Vault
    VaultAddr       string        `env:"VAULT_ADDR,required"`
    VaultToken      string        `env:"VAULT_TOKEN,required"`
    VaultKeyName    string        `env:"VAULT_KEY_NAME" envDefault:"llmvault-master"`

    // Redis
    RedisAddr       string        `env:"REDIS_ADDR,required"`
    RedisPassword   string        `env:"REDIS_PASSWORD" envDefault:""`
    RedisDB         int           `env:"REDIS_DB" envDefault:"0"`
    RedisCacheTTL   time.Duration `env:"REDIS_CACHE_TTL" envDefault:"30m"`

    // L1 Cache (in-memory)
    MemCacheTTL     time.Duration `env:"MEM_CACHE_TTL" envDefault:"5m"`
    MemCacheMaxSize int           `env:"MEM_CACHE_MAX_SIZE" envDefault:"10000"`

    // JWT
    JWTSigningKey   string        `env:"JWT_SIGNING_KEY,required"`

    // Security
    AdminAPIKey     string        `env:"ADMIN_API_KEY,required"`
}
```

### Step 1.4 — Database migrations

Run in order on startup (or via `make migrate`):

**001_create_orgs.sql**
```sql
CREATE TABLE orgs (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    api_key_hash TEXT NOT NULL,          -- bcrypt hash of org's API key
    rate_limit  INT NOT NULL DEFAULT 1000, -- requests per minute
    active      BOOLEAN NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_orgs_api_key_hash ON orgs(api_key_hash);
```

**002_create_credentials.sql**
```sql
CREATE TABLE credentials (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    label           TEXT NOT NULL DEFAULT '',
    base_url        TEXT NOT NULL,
    auth_scheme     TEXT NOT NULL CHECK (auth_scheme IN ('bearer', 'x-api-key', 'api-key', 'query_param')),
    encrypted_key   BYTEA NOT NULL,     -- AES-GCM encrypted API key
    wrapped_dek     BYTEA NOT NULL,     -- DEK wrapped by Vault Transit
    revoked_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_credentials_org_id ON credentials(org_id);
```

**003_create_tokens.sql**
```sql
CREATE TABLE tokens (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    credential_id   UUID NOT NULL REFERENCES credentials(id) ON DELETE CASCADE,
    jti             TEXT NOT NULL UNIQUE,   -- JWT ID for revocation lookups
    expires_at      TIMESTAMPTZ NOT NULL,
    revoked_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_tokens_jti ON tokens(jti);
CREATE INDEX idx_tokens_credential_id ON tokens(credential_id);
```

**004_create_audit_log.sql**
```sql
CREATE TABLE audit_log (
    id              BIGSERIAL PRIMARY KEY,
    org_id          UUID NOT NULL,
    credential_id   UUID,
    action          TEXT NOT NULL,          -- 'credential.created', 'credential.revoked', 'proxy.request', 'token.minted'
    metadata        JSONB DEFAULT '{}',     -- provider, status_code, latency_ms, etc.
    ip_address      INET,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_audit_log_org_id_created ON audit_log(org_id, created_at DESC);
```

**005_create_usage.sql**
```sql
CREATE TABLE usage (
    id              BIGSERIAL PRIMARY KEY,
    org_id          UUID NOT NULL REFERENCES orgs(id),
    credential_id   UUID NOT NULL REFERENCES credentials(id),
    request_count   BIGINT NOT NULL DEFAULT 0,
    period_start    TIMESTAMPTZ NOT NULL,
    period_end      TIMESTAMPTZ NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),

    UNIQUE(org_id, credential_id, period_start)
);
```

### Step 1.5 — Database connection + migration runner

File: `internal/db/db.go`
- Create pgx connection pool with sensible defaults (max conns, timeouts)
- Health check function

File: `internal/db/migrate.go`
- Read SQL files from embedded `migrations/` directory (Go embed)
- Execute in order, track applied migrations in a `schema_migrations` table
- Run automatically on startup

### Step 1.6 — Makefile

```makefile
build:          go build -o bin/llmvault ./cmd/server
test:           go test ./internal/... -v -race -count=1
test-e2e:       docker compose up -d && go test ./e2e/... -v -tags=e2e -count=1
lint:           golangci-lint run ./...
up:             docker compose up -d
down:           docker compose down -v
migrate:        go run ./cmd/server -migrate-only
```

**Unit tests to write in Phase 1:**
- `config_test.go`: Validates required env vars fail correctly, defaults work

---

## Phase 2: Crypto Layer

### Step 2.1 — Vault Transit client

File: `internal/crypto/vault.go`

```go
type VaultTransit struct {
    client  *vault.Client
    keyName string
}

func (v *VaultTransit) Wrap(plaintext []byte) (ciphertext []byte, err error)
func (v *VaultTransit) Unwrap(ciphertext []byte) (plaintext []byte, err error)
```

- `Wrap`: calls `transit/encrypt/{keyName}` with base64-encoded plaintext
- `Unwrap`: calls `transit/decrypt/{keyName}`, returns raw bytes
- Both return structured errors (Vault unavailable vs. permission denied vs. bad ciphertext)
- Connection pooling via Vault client's built-in HTTP client

**Tests (`vault_test.go`):**
- Wrap then unwrap returns original plaintext
- Unwrap with corrupted ciphertext returns error
- Unwrap with wrong key name returns error
- Integration test against real Vault in Docker

### Step 2.2 — Envelope encryption

File: `internal/crypto/envelope.go`

```go
func GenerateDEK() ([]byte, error)                                          // 256-bit random key
func EncryptCredential(plainKey []byte, dek []byte) (ciphertext []byte, err error)  // AES-256-GCM
func DecryptCredential(ciphertext []byte, dek []byte) (plainKey []byte, err error)  // AES-256-GCM
```

- `GenerateDEK`: `crypto/rand` → 32 bytes
- `EncryptCredential`: AES-256-GCM with random nonce prepended to ciphertext
- `DecryptCredential`: Extract nonce, decrypt, return plaintext
- All plaintext buffers zeroed after use

**Tests (`envelope_test.go`):**
- Encrypt then decrypt roundtrip
- Decrypt with wrong DEK fails
- Decrypt with truncated ciphertext fails
- Decrypt with flipped bits fails (GCM authentication)
- Nonce uniqueness: encrypt same plaintext twice, ciphertexts differ

---

## Phase 3: Data Layer (Models + CRUD)

### Step 3.1 — Org model

File: `internal/model/org.go`

```go
type Org struct {
    ID        uuid.UUID
    Name      string
    APIKeyHash string
    RateLimit int
    Active    bool
    CreatedAt time.Time
    UpdatedAt time.Time
}

func CreateOrg(ctx, pool, name) (org *Org, rawAPIKey string, err error)
func GetOrgByAPIKeyHash(ctx, pool, hash) (*Org, error)
func RotateOrgAPIKey(ctx, pool, orgID) (newRawKey string, err error)
```

- `CreateOrg` generates a random API key (`org_...`), bcrypt-hashes it, stores the hash
- Returns the raw key once (caller must save it — we never store it in plain)
- `GetOrgByAPIKeyHash` for auth middleware lookups

### Step 3.2 — Credential model

File: `internal/model/credential.go`

```go
type Credential struct {
    ID           uuid.UUID
    OrgID        uuid.UUID
    Label        string
    BaseURL      string
    AuthScheme   string
    EncryptedKey []byte
    WrappedDEK   []byte
    RevokedAt    *time.Time
    CreatedAt    time.Time
}

func CreateCredential(ctx, pool, orgID, label, baseURL, authScheme, encryptedKey, wrappedDEK) (*Credential, error)
func GetCredential(ctx, pool, credentialID, orgID) (*Credential, error)
func RevokeCredential(ctx, pool, credentialID, orgID) error
func ListCredentials(ctx, pool, orgID) ([]Credential, error)
```

- All queries scoped by `org_id` — you can never fetch another org's credential
- `GetCredential` checks `revoked_at IS NULL`
- No raw key ever touches this layer — only encrypted blobs

### Step 3.3 — Token model

File: `internal/model/token.go`

```go
func CreateTokenRecord(ctx, pool, orgID, credentialID, jti, expiresAt) error
func IsTokenRevoked(ctx, pool, jti) (bool, error)
func RevokeToken(ctx, pool, jti, orgID) error
```

### Step 3.4 — Audit model

File: `internal/model/audit.go`

```go
func WriteAuditEntry(ctx, pool, orgID, credentialID, action, metadata, ip) error
```

- Fire-and-forget via a buffered channel + background goroutine (audit writes must never block the proxy hot path)
- Flush on shutdown

---

## Phase 4: Auth Layer

### Step 4.1 — JWT minting + validation

File: `internal/token/jwt.go`

```go
type Claims struct {
    OrgID        string `json:"org_id"`
    CredentialID string `json:"cred_id"`
    jwt.RegisteredClaims
}

func Mint(signingKey []byte, orgID, credentialID string, ttl time.Duration) (string, string, error)
    // Returns (tokenString, jti, err)

func Validate(signingKey []byte, tokenString string) (*Claims, error)
    // Returns parsed claims or error
```

- HMAC-SHA256 signing (symmetric — fast, no key distribution needed)
- Claims: `org_id`, `cred_id`, `exp`, `iat`, `jti`
- Validation checks: signature, expiry, required claims present

**Tests (`jwt_test.go`):**
- Mint then validate roundtrip
- Expired token rejected
- Tampered signature rejected
- Missing claims rejected
- Wrong signing key rejected

### Step 4.2 — Org auth middleware

File: `internal/middleware/orgauth.go`

- Reads `Authorization: Bearer org_...` header
- SHA-256 hash the key, look up org in DB
- Reject if org not found or `active = false`
- Set `org` on request context

**Tests (`orgauth_test.go`):**
- Valid org key → 200 + org on context
- Missing header → 401
- Invalid key → 401
- Inactive org → 403

### Step 4.3 — Sandbox token auth middleware

File: `internal/middleware/tokenauth.go`

- Reads `Authorization: Bearer ptok_...` header
- Validate JWT signature + expiry
- Check `jti` not in revoked tokens (check Redis first, then DB — cache revocation status in Redis with TTL)
- Set `org_id`, `credential_id` on request context

**Tests (`tokenauth_test.go`):**
- Valid token → claims on context
- Expired token → 401
- Revoked token → 401
- Token for wrong signing key → 401

### Step 4.4 — Rate limiting middleware

File: `internal/middleware/ratelimit.go`

- In-memory `golang.org/x/time/rate` limiter per org_id
- Limits from org's `rate_limit` field
- Returns 429 with `Retry-After` header when exceeded

---

## Phase 5: Cache Layer (3-Tier)

This is the core performance + HA layer. Three tiers with distinct roles:

```
Request arrives
  │
  ▼
L1: In-memory (memguard LRU)          ~0.01ms
  │ miss
  ▼
L2: Redis (encrypted values)           ~0.5ms
  │ miss
  ▼
L3: Postgres + Vault Transit           ~3-8ms  (cold path)
  │
  ▼
Promote: write back to L2, then L1
```

### Step 5.1 — L1: In-memory cache

File: `internal/cache/memory.go`

```go
type CachedCredential struct {
    Enclave    *memguard.Enclave   // Sealed, mlocked memory
    BaseURL    string
    AuthScheme string
    OrgID      uuid.UUID
    CachedAt   time.Time
    HardExpiry time.Time           // Absolute max — never serve past this
}

type MemoryCache struct {
    lru *expirable.LRU[string, *CachedCredential]
}

func NewMemoryCache(maxSize int, ttl time.Duration) *MemoryCache
func (c *MemoryCache) Get(credentialID string) (*CachedCredential, bool)
func (c *MemoryCache) Set(credentialID string, cred *CachedCredential)
func (c *MemoryCache) Invalidate(credentialID string)
func (c *MemoryCache) Purge()
```

- On eviction callback: `enclave.Destroy()` (zeros + munlocks memory)
- `Get` returns sealed enclave — caller opens, uses, destroys the buffer
- Short TTL (5m) — this is the hot tier, refreshes frequently

**Tests (`memory_test.go`):**
- Set then Get returns same data
- TTL expiry: Set, wait, Get returns miss
- Invalidate: Set, Invalidate, Get returns miss
- Eviction callback fires (mock to verify Destroy called)
- Concurrent access safety (parallel goroutines hammering Get/Set)

### Step 5.2 — L2: Redis cache

File: `internal/cache/redis.go`

```go
type RedisCredential struct {
    EncryptedKey []byte `json:"ek"`   // Still encrypted — DEK-encrypted API key
    WrappedDEK   []byte `json:"wd"`   // Still wrapped — Vault-wrapped DEK
    BaseURL      string `json:"bu"`
    AuthScheme   string `json:"as"`
    OrgID        string `json:"oi"`
}

type RedisCache struct {
    client *redis.Client
    ttl    time.Duration
}

func NewRedisCache(client *redis.Client, ttl time.Duration) *RedisCache
func (r *RedisCache) Get(ctx context.Context, credentialID string) (*RedisCredential, error)
func (r *RedisCache) Set(ctx context.Context, credentialID string, cred *RedisCredential) error
func (r *RedisCache) Invalidate(ctx context.Context, credentialID string) error
```

**Critical security property: Redis stores encrypted values only.**

The API key stored in Redis is still encrypted with the DEK. The DEK stored
in Redis is still wrapped by Vault. Even if Redis is compromised, the attacker
gets nothing usable without Vault access.

The flow on L2 hit:
1. Get encrypted blob + wrapped DEK from Redis (~0.5ms)
2. Call Vault Transit to unwrap DEK (~2ms) — BUT this is cached in L1 after first call
3. Decrypt API key in-memory
4. Promote to L1 (memguard enclave)

Wait — if we need Vault on L2 hit, that's not faster than L3. The trick:

**Optimization: Cache the unwrapped DEK in L1 memory (memguard) keyed by org_id.**

Each org has ONE DEK (or a small number). Once unwrapped, the DEK stays in
L1 memory. So L2 hit = Redis fetch (0.5ms) + in-memory DEK decrypt (0.01ms)
= ~0.5ms total. Vault is only called on cold start.

```go
type DEKCache struct {
    lru *expirable.LRU[string, *memguard.Enclave]  // org_id → unwrapped DEK
}
```

**Tests (`redis_test.go`):**
- Set then Get roundtrip (against real Redis in Docker)
- TTL expiry
- Invalidate removes key
- Stored values are encrypted (read raw Redis key, verify it's not plaintext)
- Redis down → graceful fallback (return miss, not error)

### Step 5.3 — Cross-instance cache invalidation

File: `internal/cache/invalidation.go`

```go
const InvalidationChannel = "llmvault:invalidate"

type Invalidator struct {
    pubsub   *redis.PubSub
    memCache *MemoryCache
}

func NewInvalidator(client *redis.Client, memCache *MemoryCache) *Invalidator
func (inv *Invalidator) Publish(ctx context.Context, credentialID string) error
func (inv *Invalidator) Subscribe(ctx context.Context) error  // blocking — run in goroutine
```

When a credential is revoked:
1. Remove from Postgres (soft delete: set `revoked_at`)
2. Remove from Redis (DEL key)
3. Remove from local L1 (memCache.Invalidate)
4. Publish to Redis pub/sub channel: `credential_id`

Every proxy instance subscribes to the channel on startup:
- On message received: `memCache.Invalidate(credential_id)`
- This gives sub-millisecond cross-instance invalidation

Also used for token revocation:
- Revoked JTIs published to a separate channel
- Each instance maintains a small in-memory set of recently revoked JTIs
- Prevents the DB lookup on hot path for revocation checks

**Tests (`invalidation_test.go`):**
- Publish on instance A → Subscribe on instance B receives it
- Invalidation message → L1 cache entry removed
- Redis pub/sub disconnect → reconnect automatically
- High-volume invalidation (1000 messages) → all delivered

### Step 5.4 — Cache Manager (orchestrator)

File: `internal/cache/cache.go`

```go
type CacheManager struct {
    memory      *MemoryCache
    redis       *RedisCache
    dekCache    *DEKCache
    invalidator *Invalidator
    vault       *crypto.VaultTransit
    db          *pgxpool.Pool
}

func (cm *CacheManager) GetDecryptedCredential(ctx context.Context, credentialID string, orgID uuid.UUID) (apiKey []byte, baseURL string, authScheme string, err error)
func (cm *CacheManager) InvalidateCredential(ctx context.Context, credentialID string) error
func (cm *CacheManager) InvalidateToken(ctx context.Context, jti string) error
func (cm *CacheManager) IsTokenRevoked(ctx context.Context, jti string) (bool, error)
```

`GetDecryptedCredential` implements the full lookup chain:

```
1. Check L1 (memory)
   → hit: open enclave, return plaintext API key                    ~0.01ms

2. Check L2 (Redis)
   → hit: get encrypted blob
          get DEK from DEK cache (L1 memory)
            → DEK cache hit: decrypt in-memory                      ~0.5ms
            → DEK cache miss: Vault unwrap, cache DEK               ~3ms (first time per org)
          promote to L1 (seal in memguard)
          return plaintext API key

3. Check L3 (Postgres + Vault)
   → hit: fetch row from Postgres                                   ~1ms
          Vault Transit unwrap DEK                                   ~2ms
          AES-GCM decrypt API key                                    ~0.01ms
          promote to L2 (Redis, encrypted values)
          promote to L1 (memguard enclave)
          cache DEK in DEK cache
          return plaintext API key

4. All miss: credential doesn't exist → return error
```

`IsTokenRevoked` implements fast revocation check:

```
1. Check in-memory revoked JTI set (populated by pub/sub)           ~0.01ms
   → found: token is revoked

2. Check Redis (revoked JTI stored with TTL matching token expiry)  ~0.5ms
   → found: token is revoked, add to in-memory set

3. Check Postgres                                                    ~1ms
   → found: token is revoked, add to Redis + in-memory set

4. All miss: token is not revoked
```

**Tests (`cache_test.go`):**
- L1 hit → no Redis or DB call (verify with mocks)
- L1 miss, L2 hit → no DB call, L1 populated after
- L1 miss, L2 miss, L3 hit → L2 and L1 populated after
- All miss → error returned
- Invalidate → removed from all three tiers
- Concurrent requests for same credential → only one L3 fetch (singleflight)
- Redis down → L1 + L3 still work (graceful degradation)
- Stale-while-revalidate: past L1 TTL but within hard expiry → serve stale, async refresh

---

## Phase 6: The Proxy (Core Hot Path)

### Step 6.1 — Auth attachment

File: `internal/proxy/auth.go`

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

**Tests:**
- Each scheme sets the correct header/param
- Original request headers preserved
- Auth attachment doesn't leak to response

### Step 6.2 — Custom transport

File: `internal/proxy/transport.go`

```go
func NewTransport() *http.Transport {
    return &http.Transport{
        MaxIdleConns:        200,
        MaxIdleConnsPerHost: 50,
        IdleConnTimeout:     90 * time.Second,
        TLSHandshakeTimeout: 5 * time.Second,
        ResponseHeaderTimeout: 30 * time.Second,
        DisableCompression: true,
    }
}
```

- Connection pooling per upstream host (reuse TLS connections to Anthropic, OpenAI, etc.)
- No compression — we pass through whatever the provider sends

### Step 6.3 — Director (URL rewriting)

File: `internal/proxy/director.go`

```go
func NewDirector(cacheManager *cache.CacheManager) func(req *http.Request)
```

The Director function (called by `httputil.ReverseProxy`):

1. Extract `credential_id` and `org_id` from request context (set by token auth middleware)
2. Call `cacheManager.GetDecryptedCredential(credentialID, orgID)` — handles all 3 tiers
3. Rewrite `req.URL` to `base_url + stripped_path`
4. Call `AttachAuth(req, scheme, apiKey)`
5. Zero the plaintext apiKey buffer
6. Strip incoming `Authorization` header (the sandbox token)
7. Set `X-Request-ID` for tracing

### Step 6.4 — Proxy handler

File: `internal/handler/proxy.go`

```go
func NewProxyHandler(director func(*http.Request), transport http.RoundTripper) http.Handler {
    proxy := &httputil.ReverseProxy{
        Director:       director,
        Transport:      transport,
        FlushInterval:  -1,  // Immediate chunk flushing for SSE streaming
        ErrorHandler:   customErrorHandler,
    }
    return proxy
}
```

- `FlushInterval: -1` is critical — makes `httputil.ReverseProxy` flush every chunk immediately
- `ErrorHandler` returns a JSON error response if upstream is unreachable
- No buffering, no body inspection, no modification — pure pass-through

**Tests (`proxy_test.go`):**
- Mock upstream returns 200 with body → proxy returns same body
- Mock upstream returns SSE stream → proxy streams each chunk immediately
- Mock upstream returns 500 → proxy passes through 500
- Mock upstream hangs → proxy returns 504 after timeout
- Correct auth header attached to upstream request
- Incoming Authorization header stripped (sandbox token not forwarded)

---

## Phase 7: API Handlers

### Step 7.1 — Credential handlers

File: `internal/handler/credentials.go`

**POST /v1/credentials**
```json
Request:
{
    "label": "customer_42_anthropic",
    "base_url": "https://api.anthropic.com",
    "auth_scheme": "x-api-key",
    "api_key": "sk-ant-..."
}

Response (201):
{
    "id": "uuid",
    "label": "customer_42_anthropic",
    "base_url": "https://api.anthropic.com",
    "auth_scheme": "x-api-key",
    "created_at": "2026-03-08T..."
}
```

Flow:
1. Validate input (base_url is valid URL, auth_scheme is one of 4, api_key non-empty)
2. Generate DEK → encrypt api_key with DEK → wrap DEK with Vault Transit
3. Store encrypted_key + wrapped_dek in Postgres
4. Audit log: `credential.created`
5. Return credential (without the key)

**DELETE /v1/credentials/:id**
- Soft revoke: sets `revoked_at = now()`
- `cacheManager.InvalidateCredential(id)` — removes from L1, L2, publishes to pub/sub
- Audit log: `credential.revoked`

**GET /v1/credentials**
- List all credentials for the org (no keys returned, ever)

**Tests (`credentials_test.go`):**
- Create credential → stored in DB with encrypted_key (not plaintext)
- Create with invalid auth_scheme → 400
- Create with invalid base_url → 400
- Delete → revoked_at set, removed from all cache tiers
- List returns only own org's credentials
- Cannot GET another org's credential (org_id mismatch → 404)

### Step 7.2 — Token handlers

File: `internal/handler/tokens.go`

**POST /v1/tokens**
```json
Request:
{
    "credential_id": "uuid",
    "ttl": "1h",
    "label": "sandbox_xyz"
}

Response (201):
{
    "token": "ptok_eyJhbGci...",
    "expires_at": "2026-03-08T12:00:00Z"
}
```

Flow:
1. Validate credential_id belongs to the authenticated org
2. Validate credential is not revoked
3. Validate ttl (> 0, <= 24h)
4. Mint JWT with claims: org_id, credential_id, jti, exp
5. Store token record in DB (for revocation support)
6. Audit log: `token.minted`
7. Return token (prefixed with `ptok_` for human identification)

**DELETE /v1/tokens/:jti**
- Revoke: sets `revoked_at = now()`
- `cacheManager.InvalidateToken(jti)` — adds to Redis revocation set + publishes to pub/sub
- Audit log: `token.revoked`

**Tests (`tokens_test.go`):**
- Mint token for valid credential → valid JWT returned
- Mint for revoked credential → 400
- Mint for another org's credential → 404
- Mint with TTL > 24h → 400
- Revoke token → subsequent proxy calls rejected

### Step 7.3 — Org handlers (admin)

File: `internal/handler/orgs.go`

**POST /v1/orgs** (admin API key auth)
```json
Request:  { "name": "Service A", "rate_limit": 5000 }
Response: { "id": "uuid", "name": "Service A", "api_key": "org_abc123..." }
```

- Returns raw API key once. We only store the hash.
- Protected by admin API key (separate from org keys)

---

## Phase 8: Server Wiring

File: `cmd/server/main.go`

```go
func main() {
    // 1. Parse config from env
    // 2. Init memguard (call memguard.CatchInterrupt(), rlimit core = 0)
    // 3. Connect to Postgres (pgx pool)
    // 4. Run migrations
    // 5. Init Vault Transit client
    // 6. Connect to Redis
    // 7. Init cache manager (L1 memory + L2 Redis + DEK cache + invalidator)
    // 8. Start invalidation subscriber (goroutine)
    // 9. Init JWT signer

    r := chi.NewRouter()

    // Global middleware
    r.Use(middleware.RequestID)
    r.Use(middleware.RealIP)
    r.Use(slogMiddleware)       // structured request logging
    r.Use(middleware.Recoverer)

    // Admin routes
    r.Group(func(r chi.Router) {
        r.Use(adminAuthMiddleware)
        r.Post("/v1/orgs", orgHandler.Create)
    })

    // Org-authenticated routes (Service A's backend)
    r.Group(func(r chi.Router) {
        r.Use(orgAuthMiddleware)
        r.Use(rateLimitMiddleware)
        r.Post("/v1/credentials", credHandler.Create)
        r.Get("/v1/credentials", credHandler.List)
        r.Delete("/v1/credentials/{id}", credHandler.Revoke)
        r.Post("/v1/tokens", tokenHandler.Create)
        r.Delete("/v1/tokens/{jti}", tokenHandler.Revoke)
    })

    // Sandbox-authenticated routes (proxy)
    r.Group(func(r chi.Router) {
        r.Use(tokenAuthMiddleware)
        r.Handle("/v1/proxy/*", proxyHandler)
    })

    // Health checks (no auth)
    r.Get("/healthz", healthHandler)    // liveness: process is alive
    r.Get("/readyz", readyHandler)      // readiness: Postgres + Redis + Vault reachable

    // 10. Start server with graceful shutdown
    // 11. On shutdown:
    //     - Set readyz to 503 (load balancer stops sending traffic)
    //     - Wait for drain period (in-flight requests complete)
    //     - Close invalidation subscriber
    //     - Flush audit buffer
    //     - Purge L1 cache (memguard.Purge)
    //     - Close Redis connection
    //     - Close DB pool
}
```

---

## Phase 9: Docker

### Step 9.1 — Dockerfile

```dockerfile
# Build stage
FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /llmvault ./cmd/server

# Runtime stage
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /llmvault /llmvault
USER nonroot:nonroot
ENTRYPOINT ["/llmvault"]
```

- Distroless: no shell, no package manager, no attack surface
- `nonroot` user: no root in the container
- Static binary: no libc dependency

### Step 9.2 — docker-compose.yml

```yaml
services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: llmvault
      POSTGRES_USER: llmvault
      POSTGRES_PASSWORD: localdev
    ports: ["5432:5432"]
    volumes:
      - pgdata:/var/lib/postgresql/data
      - ./docker/postgres/init.sql:/docker-entrypoint-initdb.d/init.sql
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U llmvault"]
      interval: 2s
      timeout: 5s
      retries: 5

  vault:
    image: hashicorp/vault:1.15
    cap_add: [IPC_LOCK]
    environment:
      VAULT_DEV_ROOT_TOKEN_ID: dev-token
    ports: ["8200:8200"]
    healthcheck:
      test: ["CMD", "vault", "status"]
      interval: 2s
      timeout: 5s
      retries: 5
    entrypoint: >
      sh -c "
        vault server -dev -dev-listen-address=0.0.0.0:8200 &
        sleep 2 &&
        vault secrets enable transit &&
        vault write -f transit/keys/llmvault-master &&
        wait
      "

  redis:
    image: redis:7-alpine
    ports: ["6379:6379"]
    command: redis-server --appendonly yes --maxmemory 256mb --maxmemory-policy allkeys-lru
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 2s
      timeout: 5s
      retries: 5

  proxy:
    build:
      context: .
      dockerfile: docker/Dockerfile
    ports: ["8080:8080"]
    depends_on:
      postgres: { condition: service_healthy }
      vault:    { condition: service_healthy }
      redis:    { condition: service_healthy }
    environment:
      PORT: 8080
      DATABASE_URL: postgres://llmvault:localdev@postgres:5432/llmvault?sslmode=disable
      VAULT_ADDR: http://vault:8200
      VAULT_TOKEN: dev-token
      VAULT_KEY_NAME: llmvault-master
      REDIS_ADDR: redis:6379
      REDIS_PASSWORD: ""
      REDIS_DB: 0
      REDIS_CACHE_TTL: 30m
      MEM_CACHE_TTL: 5m
      MEM_CACHE_MAX_SIZE: 10000
      JWT_SIGNING_KEY: local-dev-signing-key-change-in-prod
      ADMIN_API_KEY: admin-local-dev-key

volumes:
  pgdata:
```

---

## Phase 10: Testing

### Unit tests (Phases 2-7, alongside each component)

Every `_test.go` file listed above. Run with:
```
make test    # go test ./internal/... -v -race -count=1
```

Key unit test areas:
- Crypto: roundtrips, tamper detection, wrong key rejection
- JWT: signing, validation, expiry, tampering
- Auth middleware: valid/invalid/missing/expired credentials
- Rate limiter: allows within limit, blocks above limit
- Cache: all 3 tiers, promotion, invalidation, degradation
- Handlers: input validation, error responses, org isolation

### Integration tests (require Docker stack)

Included in unit test files with build tags or test helpers that skip if unavailable:
- Vault Transit wrap/unwrap against real Vault
- Redis get/set/pub-sub against real Redis
- Credential create/fetch/revoke against real Postgres
- Full cache chain: L1 → L2 → L3 with real dependencies
- Full middleware chain with real DB lookups

### E2E tests (75%+ of test suite — real LLM calls)

File: `e2e/e2e_test.go` — shared setup:

```go
//go:build e2e

func TestMain(m *testing.M) {
    // 1. Ensure docker-compose stack is up
    // 2. Create a test org via admin API
    // 3. Store real API keys as credentials (from env vars)
    // 4. Run tests
    // 5. Cleanup
}
```

Required env vars for e2e (set in CI secrets + `.env.test`):
```
E2E_ANTHROPIC_KEY=sk-ant-...
E2E_OPENAI_KEY=sk-...
E2E_OPENROUTER_KEY=sk-or-...
E2E_FIREWORKS_KEY=fw_...
E2E_GEMINI_KEY=AIza...
```

**E2E test files:**

**`e2e/anthropic_test.go`**
- Store Anthropic key as credential (base_url: https://api.anthropic.com, scheme: x-api-key)
- Mint sandbox token
- POST /v1/proxy/v1/messages with a small Claude request (streaming: true)
- Assert: response streams back, contains valid completion, status 200
- POST /v1/proxy/v1/messages with streaming: false
- Assert: response returns complete JSON body

**`e2e/openai_test.go`**
- Store OpenAI key (base_url: https://api.openai.com, scheme: bearer)
- POST /v1/proxy/v1/chat/completions (streaming + non-streaming)
- Assert: valid completion streamed back

**`e2e/openrouter_test.go`**
- Store OpenRouter key (base_url: https://openrouter.ai/api, scheme: bearer)
- POST /v1/proxy/v1/chat/completions
- Assert: valid response

**`e2e/fireworks_test.go`**
- Store Fireworks key (base_url: https://api.fireworks.ai/inference, scheme: bearer)
- POST /v1/proxy/v1/chat/completions
- Assert: valid response

**`e2e/gemini_test.go`**
- Store Gemini key (base_url: https://generativelanguage.googleapis.com, scheme: query_param)
- POST /v1/proxy/v1beta/models/gemini-pro:generateContent
- Assert: valid response (tests the query_param auth scheme specifically)

**`e2e/credential_lifecycle_test.go`**
- Create credential → mint token → proxy succeeds
- Revoke credential → proxy returns 401/404
- Verify cache invalidation (revoked credential fails immediately, no TTL wait)

**`e2e/token_expiry_test.go`**
- Mint token with 2s TTL
- Immediate proxy call → succeeds
- Wait 3s → proxy call fails with 401
- Mint token → revoke it → proxy call fails

**`e2e/tenant_isolation_test.go`**
- Create Org A + Org B
- Store credential in Org A
- Mint token scoped to Org A's credential
- Attempt to mint token from Org B for Org A's credential → 404
- Attempt to proxy with Org A's token against Org B's proxy → fails

**`e2e/auth_schemes_test.go`**
- One test per auth scheme (bearer, x-api-key, api-key, query_param)
- Verify the upstream request has the correct header/param set
- Uses a mock upstream server that echoes back received headers

**`e2e/streaming_fidelity_test.go`**
- Send streaming request to Anthropic
- Collect all SSE chunks
- Verify: first chunk arrives within reasonable time
- Verify: all chunks are valid SSE format
- Verify: final chunk contains stop reason
- Verify: concatenated chunks form valid completion

**`e2e/cache_invalidation_test.go`**
- Store credential → proxy succeeds (populates L1 + L2)
- Revoke credential → immediate retry fails (pub/sub invalidated L1 across instances)
- Verify Redis key deleted
- Store new credential → first proxy call populates L2 from L3 → second call hits L1

**`e2e/cache_tiers_test.go`**
- Store credential → proxy once (cold path: L3 → promotes to L2 + L1)
- Proxy again → verify L1 hit (mock/spy to confirm no Redis call)
- Invalidate L1 only → proxy → verify L2 hit (Redis call, no Postgres call)
- Invalidate L2 → proxy → verify L3 hit (Postgres + Vault call)
- Redis down → proxy still works via L1 (if cached) or L3 (if not)

---

## Phase 11: Security Hardening Checklist

Applied throughout, verified in a final review pass:

- [ ] **No plaintext keys in logs**: slog middleware redacts Authorization headers
- [ ] **No plaintext keys in DB**: only encrypted blobs stored
- [ ] **No plaintext keys in Redis**: only DEK-encrypted blobs stored (useless without Vault)
- [ ] **No plaintext keys in error responses**: error handler never leaks credential data
- [ ] **memguard.CatchInterrupt()**: cleanup on SIGINT/SIGTERM
- [ ] **RLIMIT_CORE = 0**: no core dumps
- [ ] **Distroless container**: no shell to exec into
- [ ] **nonroot user**: container runs unprivileged
- [ ] **Org isolation in every DB query**: all queries include `WHERE org_id = $org`
- [ ] **Redis key namespacing**: all keys prefixed with `pb:{org_id}:` to prevent cross-tenant access
- [ ] **Input validation**: base_url must be HTTPS in production, auth_scheme must be enum
- [ ] **Max TTL on tokens**: 24h cap, enforced server-side
- [ ] **Audit log**: every sensitive operation logged
- [ ] **Rate limiting**: per-org, in-memory, configurable
- [ ] **Graceful shutdown**: drain connections, flush audit buffer, purge cache
- [ ] **No request body logging**: proxy body is never logged or inspected
- [ ] **TLS in production**: proxy itself should sit behind TLS termination
- [ ] **Vault policy scoping**: proxy token can only encrypt/decrypt, never export keys
- [ ] **Redis AUTH**: password-protected in production
- [ ] **Redis TLS**: encrypted transport in production

---

## Implementation Order

```
Phase 1  → Foundation         → docker-compose boots (PG + Vault + Redis), migrations run, server starts
Phase 2  → Crypto             → envelope encryption works against Vault
Phase 3  → Models             → CRUD operations with encrypted storage
Phase 4  → Auth               → org keys + JWT tokens + middleware
Phase 5  → Cache              → 3-tier cache (L1 memory + L2 Redis + L3 PG/Vault) + pub/sub invalidation
Phase 6  → Proxy              → streaming reverse proxy with auth attachment
Phase 7  → Handlers           → REST API endpoints wired up
Phase 8  → Server wiring      → everything connected, chi router, graceful shutdown, readiness probes
Phase 9  → Docker             → full stack runs in docker-compose
Phase 10 → Tests              → unit tests throughout, e2e tests at the end
Phase 11 → Security review    → final hardening pass against checklist
```

Each phase is independently testable. Each builds on the last. No phase requires rework of a previous phase.

---

## HA Properties Summary

| Concern | Solution |
|---|---|
| Proxy horizontal scaling | Stateless — run N instances behind load balancer |
| Cross-instance cache invalidation | Redis pub/sub — sub-ms propagation to all instances |
| Vault temporary outage | L1 + L2 cache serve stale within hard expiry (1h) |
| Redis temporary outage | L1 (memory) + L3 (Postgres/Vault) still work — graceful degradation |
| Postgres temporary outage | L1 + L2 cache serve existing credentials — new credentials blocked |
| Rolling deploys | Readiness probe (`/readyz`) → LB drains before shutdown |
| Credential revocation consistency | Pub/sub invalidation + short L1 TTL (5m) + hard expiry (1h) |
| Token revocation consistency | Pub/sub for revoked JTIs + Redis revocation set + DB fallback |
