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
| GET | Blue |
| POST | Green |
| PUT | Amber |
| PATCH | Amber |
| DELETE | Red |

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
- **Proxy** - LLM proxy requests only
- **Management** - API management requests

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

## Log Entry Details

Each log entry includes:

| Field | Description |
|-------|-------------|
| Action | Type of request (proxy or management) |
| Method | HTTP method (GET, POST, etc.) |
| Path | Request path |
| Status | HTTP status code |
| Latency | Response time in milliseconds |
| Credential | Which credential was used |
| Identity | Which identity made the request |
| IP Address | Client IP address |
| Timestamp | When the request occurred |

## Retention Policies

Log retention varies by plan:

| Plan | Retention |
|------|-----------|
| Free | 7 days |
| Pro | 90 days |
| Enterprise | 1 year |

Logs are automatically purged after the retention period.

## Accessing Logs via SDK

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
- `action` - Filter by action type (`proxy.request` or `api.request`)

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

## Best Practices

1. **Regular review** - Check logs weekly for anomalies
2. **Filter first** - Use action filters before searching
3. **Monitor latency** - Watch for degrading response times
4. **Track errors** - Follow up on 4xx and 5xx status codes
5. **Export periodically** - Back up important logs before retention expiration
