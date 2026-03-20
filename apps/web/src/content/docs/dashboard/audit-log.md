---
title: Audit Log
description: Viewing logs, filtering, log details, and retention policies
---

# Audit Log

The Audit Log provides a comprehensive record of all API requests and proxy activity for your organization.

## Audit Log List

Navigate to **Manage > Audit Log** to view activity.

### List View

The audit log table displays:

| Column | Description |
|--------|-------------|
| Timestamp | Request time (MMM DD, HH:MM:SS) |
| Credential | Credential ID (truncated) |
| Method | HTTP method badge |
| Path | Request path |
| Status | HTTP status code (color-coded) |
| Latency | Response time |
| IP Address | Client IP |

### Method Badges

| Method | Color |
|--------|-------|
| GET | Blue (#3B82F6) |
| POST | Green (#22C55E) |
| PUT | Amber (#F59E0B) |
| PATCH | Amber (#F59E0B) |
| DELETE | Red (#EF4444) |

### Status Colors

| Range | Color | Meaning |
|-------|-------|---------|
| 2xx | Green | Success |
| 4xx | Amber | Client error |
| 5xx | Red | Server error |

## Filtering

### Action Type Filter

Filter by request type:
- **All** - Show all entries
- **Proxy** - LLM proxy requests only (`proxy.request`)
- **Management** - API management requests (`api.request`)

Filter tabs show counts for each category.

### Searching

Search entries by:
- Path (e.g., `/v1/chat/completions`)
- Credential ID
- IP address

Search is case-insensitive and matches partial strings.

## Pagination

Entries are paginated (50 per page):
- Click **Next** / **Previous** to navigate
- Page number displayed below table
- Cursor-based pagination for performance

## Log Entry Details

Each entry contains:

```json
{
  "id": 12345,
  "action": "proxy.request",
  "method": "POST",
  "path": "/v1/chat/completions",
  "status": 200,
  "latency_ms": 1250,
  "credential_id": "cred-uuid",
  "identity_id": "ident-uuid",
  "ip_address": "192.168.1.1",
  "created_at": "2026-03-20T10:30:00Z"
}
```

**Metadata Fields** (stored in JSONB):
- `method` - HTTP method
- `path` - Request path
- `status` - HTTP status code
- `latency_ms` - Response time in milliseconds

## Retention Policies

Log retention varies by plan:

| Plan | Retention |
|------|-----------|
| Free | 7 days |
| Pro | 90 days |
| Enterprise | 1 year |

Logs are automatically purged after the retention period.

## Exporting Logs

Currently, logs can be accessed via:
- Dashboard UI (paginated viewing)
- API: `GET /v1/audit`
- SDK: `vault.audit.list()`

Direct export functionality is planned for future releases.

## API Access

Fetch logs programmatically:

```typescript
import { LLMVault } from "@llmvault/sdk";

const vault = new LLMVault({ apiKey: "ak_live_..." });

const { data, error } = await vault.audit.list({
  limit: 50,
  action: "proxy.request"
});
```

Query parameters:
- `limit` - Items per page (1-100, default 50)
- `cursor` - Pagination cursor
- `action` - Filter by action type

## Use Cases

### Debugging
- Trace specific credential activity
- Check error rates by endpoint
- Monitor latency trends

### Security
- Identify unusual IP addresses
- Track credential usage patterns
- Detect potential abuse

### Billing
- Count requests for usage reports
- Attribute usage by credential
- Calculate costs per identity

### Compliance
- Maintain audit trails
- Review access history
- Support investigations

## Performance Notes

- Log writes are asynchronous
- Queries use cursor-based pagination
- Large result sets are streamed
- IP geolocation not performed

## Best Practices

1. **Regular review** - Check logs weekly for anomalies
2. **Filter first** - Use action filters before searching
3. **Monitor latency** - Watch for degrading response times
4. **Track errors** - Follow up on 4xx and 5xx status codes
5. **Export periodically** - Backup logs before retention expiration
