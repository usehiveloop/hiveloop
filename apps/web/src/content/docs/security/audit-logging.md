---
title: Audit Logging
description: Complete audit trail of all API and proxy requests.
---

# Audit Logging

LLMVault maintains a comprehensive audit trail of all security-relevant events. This provides visibility into who accessed what, when, and from where - essential for security monitoring, compliance, and incident investigation.

## Logged Events

### Event Types

| Action | Description | Source |
|--------|-------------|--------|
| `api.request` | Management API requests | All API endpoints |
| `proxy.request` | Proxy requests to LLM providers | Proxy handler |

### Audit Entry Schema

```go
// From: internal/model/audit.go
type AuditEntry struct {
    ID           int64      `gorm:"primaryKey;autoIncrement"`
    OrgID        uuid.UUID  `gorm:"type:uuid;not null;index:idx_audit_org_created"`
    CredentialID *uuid.UUID `gorm:"type:uuid;index:idx_audit_credential"`
    IdentityID   *uuid.UUID `gorm:"type:uuid;index:idx_audit_identity"`
    Action       string     `gorm:"not null"`
    Metadata     JSON       `gorm:"type:jsonb;default:'{}'"`
    IPAddress    *string    `gorm:"type:inet"`
    CreatedAt    time.Time  `gorm:"index:idx_audit_org_created"`
}
```

### Metadata Structure

The `metadata` JSONB column contains event-specific details:

```go
// From: internal/middleware/audit.go
entry := model.AuditEntry{
    Action: a,
    Metadata: model.JSON{
        "method":     r.Method,
        "path":       r.URL.Path,
        "status":     sw.status,
        "latency_ms": time.Since(start).Milliseconds(),
    },
}
```

**Common Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `method` | string | HTTP method (GET, POST, etc.) |
| `path` | string | Request path |
| `status` | int | HTTP response status code |
| `latency_ms` | int64 | Request processing time in milliseconds |

### Database Schema

```sql
CREATE TABLE audit_log (
    id BIGSERIAL PRIMARY KEY,
    org_id UUID NOT NULL,
    credential_id UUID,
    identity_id UUID,
    action VARCHAR(255) NOT NULL,
    metadata JSONB DEFAULT '{}',
    ip_address INET,
    created_at TIMESTAMP NOT NULL
);

-- Indexes for efficient querying
CREATE INDEX idx_audit_org_created ON audit_log(org_id, created_at);
CREATE INDEX idx_audit_credential ON audit_log(credential_id);
CREATE INDEX idx_audit_identity ON audit_log(identity_id);
```

## Audit Middleware

### Buffered Async Writing

Audit logs are written asynchronously to avoid blocking the request hot path:

```go
// From: internal/middleware/audit.go
type AuditWriter struct {
    db      *gorm.DB
    entries chan model.AuditEntry
    wg      sync.WaitGroup
}

// NewAuditWriter creates an AuditWriter with the given buffer size.
func NewAuditWriter(db *gorm.DB, bufferSize int) *AuditWriter {
    aw := &AuditWriter{
        db:      db,
        entries: make(chan model.AuditEntry, bufferSize),
    }
    aw.wg.Add(1)
    go aw.drain()
    return aw
}

// drain processes entries in a background goroutine.
func (aw *AuditWriter) drain() {
    defer aw.wg.Done()
    for entry := range aw.entries {
        if err := aw.db.Create(&entry).Error; err != nil {
            slog.Error("audit write failed", "error", err, "action", entry.Action)
        }
    }
}
```

### Non-Blocking Write

Entries are queued without blocking; full buffer drops entries with warning:

```go
// From: internal/middleware/audit.go
func (aw *AuditWriter) Write(entry model.AuditEntry) {
    select {
    case aw.entries <- entry:
    default:
        slog.Warn("audit buffer full, dropping entry", "action", entry.Action)
    }
}
```

### Request Logging

The middleware logs every request after completion:

```go
// From: internal/middleware/audit.go
func Audit(aw *AuditWriter, action ...string) func(http.Handler) http.Handler {
    a := "api.request"
    if len(action) > 0 {
        a = action[0]
    }
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            start := time.Now()

            // Wrap response writer to capture status code
            sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
            next.ServeHTTP(sw, r)

            // Build audit entry after handler completes
            entry := model.AuditEntry{
                Action:   a,
                Metadata: model.JSON{
                    "method":     r.Method,
                    "path":       r.URL.Path,
                    "status":     sw.status,
                    "latency_ms": time.Since(start).Milliseconds(),
                },
            }

            // Extract IP address
            if ip, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
                entry.IPAddress = &ip
            } else {
                addr := r.RemoteAddr
                entry.IPAddress = &addr
            }

            // Extract organization from context
            if org, ok := OrgFromContext(r.Context()); ok {
                entry.OrgID = org.ID
            }

            // Extract credential from token claims
            if claims, ok := ClaimsFromContext(r.Context()); ok {
                if credID, err := uuid.Parse(claims.CredentialID); err == nil {
                    entry.CredentialID = &credID
                }
                // For proxy routes, org may not be in context - extract from claims
                if entry.OrgID == uuid.Nil {
                    if orgID, err := uuid.Parse(claims.OrgID); err == nil {
                        entry.OrgID = orgID
                    }
                }
            }

            // Extract identity ID if present
            if identID, ok := CredentialIdentityIDFromContext(r.Context()); ok && identID != nil {
                entry.IdentityID = identID
            }

            aw.Write(entry)
        })
    }
}
```

### Status Code Capture

The `statusWriter` captures the HTTP status code:

```go
// From: internal/middleware/audit.go
type statusWriter struct {
    http.ResponseWriter
    status      int
    wroteHeader bool
}

func (sw *statusWriter) WriteHeader(code int) {
    if !sw.wroteHeader {
        sw.status = code
        sw.wroteHeader = true
    }
    sw.ResponseWriter.WriteHeader(code)
}
```

## Log Retention

### Default Retention Policy

Audit logs are retained based on your organization's plan:

| Plan | Retention Period |
|------|-----------------|
| Free | 7 days |
| Pro | 90 days |
| Enterprise | Custom (up to 7 years) |

### Self-Hosted Retention

For self-hosted deployments, configure retention via database partitioning:

```sql
-- Example: Partition by month for efficient retention
CREATE TABLE audit_log_2025_01 PARTITION OF audit_log
    FOR VALUES FROM ('2025-01-01') TO ('2025-02-01');

-- Drop old partitions
DROP TABLE audit_log_2024_01;
```

### Archiving

For compliance, export logs before deletion:

```bash
# Export to S3
psql -c "COPY (SELECT * FROM audit_log WHERE created_at < NOW() - INTERVAL '1 year') TO STDOUT CSV" \
  | aws s3 cp - s3://my-audit-bucket/audit-2024.csv

# Vacuum to reclaim space
psql -c "VACUUM FULL audit_log;"
```

## Querying Logs

### API Endpoint

Query audit logs via the REST API:

```bash
GET /v1/audit?action=proxy.request&limit=50
Authorization: Bearer llmv_sk_...
```

### Response Format

```go
// From: internal/handler/audit.go
type auditEntryResponse struct {
    ID           int64   `json:"id"`
    Action       string  `json:"action"`
    Method       string  `json:"method,omitempty"`
    Path         string  `json:"path,omitempty"`
    Status       int     `json:"status,omitempty"`
    LatencyMs    int64   `json:"latency_ms,omitempty"`
    CredentialID *string `json:"credential_id,omitempty"`
    IdentityID   *string `json:"identity_id,omitempty"`
    IPAddress    *string `json:"ip_address,omitempty"`
    CreatedAt    string  `json:"created_at"`
}
```

### Query Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `limit` | int | Max items per page (1-100, default 50) |
| `cursor` | string | Pagination cursor (entry ID) |
| `action` | string | Filter by action type |

### Cursor Pagination

The API uses cursor-based pagination for efficient deep paging:

```go
// From: internal/handler/audit.go
func (h *AuditHandler) List(w http.ResponseWriter, r *http.Request) {
    limit := 50
    if l := r.URL.Query().Get("limit"); l != "" {
        n, _ := strconv.Atoi(l)
        if n > 100 { n = 100 }
        limit = n
    }

    q := h.db.Table("audit_log").Where("org_id = ?", org.ID)

    if action := r.URL.Query().Get("action"); action != "" {
        q = q.Where("action = ?", action)
    }

    if c := r.URL.Query().Get("cursor"); c != "" {
        cursorID, _ := strconv.ParseInt(c, 10, 64)
        q = q.Where("id < ?", cursorID)
    }

    q = q.Order("id DESC").Limit(limit + 1)
    // ... execute query ...
}
```

### Example Queries

**Recent Proxy Requests:**
```bash
curl -s "https://api.llmvault.dev/v1/audit?action=proxy.request&limit=10" \
  -H "Authorization: Bearer $API_KEY" | jq
```

**Credential Usage:**
```bash
curl -s "https://api.llmvault.dev/v1/audit?credential_id=xxx&limit=100" \
  -H "Authorization: Bearer $API_KEY" | jq
```

**Paginated Results:**
```bash
# First page
curl -s "https://api.llmvault.dev/v1/audit?limit=50" \
  -H "Authorization: Bearer $API_KEY" | jq

# Next page using cursor from response
curl -s "https://api.llmvault.dev/v1/audit?limit=50&cursor=12345" \
  -H "Authorization: Bearer $API_KEY" | jq
```

## Usage Analytics

### Credential Statistics

Audit logs power usage analytics:

```go
// From: internal/handler/credentials.go
h.db.Raw(`SELECT credential_id, COUNT(*) AS request_count, MAX(created_at) AS last_used_at
    FROM audit_log
    WHERE org_id = ? AND action = 'proxy.request' AND credential_id IN (?)
    GROUP BY credential_id`, org.ID, credIDs).Scan(&stats)
```

### Dashboard Integration

The dashboard displays credential usage from audit data:
- Request count per credential
- Last used timestamp
- Usage trends over time

## Security Monitoring

### Suspicious Activity Detection

Monitor audit logs for:

**Failed Authentication Attempts:**
```sql
SELECT ip_address, COUNT(*) as failures
FROM audit_log
WHERE status = 401 AND created_at > NOW() - INTERVAL '1 hour'
GROUP BY ip_address
HAVING COUNT(*) > 10;
```

**Unusual Proxy Activity:**
```sql
SELECT credential_id, COUNT(*) as requests
FROM audit_log
WHERE action = 'proxy.request' 
  AND created_at > NOW() - INTERVAL '5 minutes'
GROUP BY credential_id
HAVING COUNT(*) > 1000;
```

**After-Hours Access:**
```sql
SELECT * FROM audit_log
WHERE action = 'api.request'
  AND EXTRACT(HOUR FROM created_at) NOT BETWEEN 9 AND 18
  AND created_at > NOW() - INTERVAL '1 day';
```

### Alerting

Configure alerts via your SIEM:

```yaml
# Example Datadog monitor
name: "High Rate of Failed Auth"
type: metric alert
query: "avg(last_5m):sum:llmvault.audit.status{status:401} > 100"
message: "Possible brute force attack from {{ip_address}}"
```

## Compliance

### SOC 2 Requirements

Audit logging satisfies SOC 2 control requirements:

| Control | Implementation |
|---------|---------------|
| CC6.1 | All access logged with timestamp, actor, and action |
| CC7.2 | Audit trail of system events |
| CC7.3 | Log retention per policy |

### GDPR Article 30

Audit logs support GDPR record-keeping:
- Processing activities recorded
- Access to personal data logged
- Retention policies enforced

### Export for Auditors

```bash
# Export all logs for a date range
psql -c "\copy (SELECT * FROM audit_log 
  WHERE org_id = 'xxx' 
  AND created_at BETWEEN '2025-01-01' AND '2025-01-31'
  TO '/tmp/audit-jan-2025.csv' CSV HEADER;"
```

## Best Practices

### 1. Enable Comprehensive Logging

Ensure all handlers are wrapped with audit middleware:

```go
// Router setup
r.Use(middleware.Audit(auditWriter, "api.request"))
r.Route("/proxy", func(r chi.Router) {
    r.Use(middleware.Audit(auditWriter, "proxy.request"))
})
```

### 2. Set Appropriate Buffer Size

Size the audit buffer for your traffic:

```go
// Buffer for 10k entries (~100MB at peak)
auditWriter := middleware.NewAuditWriter(db, 10000)
defer auditWriter.Shutdown(ctx)
```

### 3. Monitor Audit Health

Alert on:
- Dropped entries (buffer full)
- Write failures (DB connectivity)
- Unusual latency

### 4. Protect Log Integrity

For tamper-evident logs:
- Enable Postgres WAL archiving
- Ship logs to WORM storage (S3 Glacier)
- Sign log batches cryptographically

### 5. Regular Review

Schedule periodic audit reviews:
- Daily: Failed auth patterns
- Weekly: Access anomalies
- Monthly: Full access review

## Troubleshooting

### Missing Logs

If logs are not appearing:

1. **Check buffer size** - May be dropping entries
2. **Verify DB connectivity** - Connection pool exhaustion
3. **Check disk space** - Audit table may be full

### Performance Impact

If audit logging is slow:

1. **Add index** on `(org_id, created_at)`
2. **Partition table** by month
3. **Increase buffer size**
4. **Use dedicated audit DB**

### Storage Growth

If audit table grows too large:

1. **Reduce retention** period
2. **Archive old logs** to cold storage
3. **Compress metadata** JSON
4. **Exclude health checks** from logging
