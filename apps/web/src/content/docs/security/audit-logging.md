---
title: Audit Logging
description: Complete audit trail of all API and proxy requests.
---

# Audit Logging

LLMVault maintains a comprehensive audit trail of all security-relevant events. This gives you visibility into who accessed what, when, and from where -- essential for security monitoring, compliance, and incident investigation.

## What Gets Logged

### Event Types

| Action | Description |
|--------|-------------|
| `api.request` | Management API requests (creating credentials, minting tokens, etc.) |
| `proxy.request` | Proxy requests forwarded to LLM providers |

### Logged Fields

Every audit entry captures:

| Field | Type | Description |
|-------|------|-------------|
| `id` | integer | Unique, monotonically increasing entry ID |
| `action` | string | Event type (`api.request` or `proxy.request`) |
| `method` | string | HTTP method (GET, POST, DELETE, etc.) |
| `path` | string | Request path |
| `status` | integer | HTTP response status code |
| `latency_ms` | integer | Request processing time in milliseconds |
| `credential_id` | string | Credential used (for proxy requests) |
| `identity_id` | string | Identity associated with the request (when applicable) |
| `ip_address` | string | Client IP address |
| `created_at` | string | ISO 8601 timestamp |

## Querying Audit Logs

### Using the SDK

```typescript
import { LLMVault } from "@llmvault/sdk";
const vault = new LLMVault({ apiKey: "your-api-key" });

// List recent audit entries
const { data, error } = await vault.audit.list({
  limit: 50,
  action: "proxy.request"
});

// Paginate through results
const { data: nextPage } = await vault.audit.list({
  limit: 50,
  cursor: data.next_cursor
});
```

### Using the API

```bash
# Recent proxy requests
curl -s "https://api.llmvault.dev/v1/audit?action=proxy.request&limit=10" \
  -H "Authorization: Bearer llmv_sk_..."
```

### Query Parameters

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `limit` | integer | 50 | Results per page (1-100) |
| `cursor` | string | -- | Pagination cursor (entry ID from previous response) |
| `action` | string | -- | Filter by event type (`api.request` or `proxy.request`) |

### Response Format

```json
{
  "data": [
    {
      "id": 12345,
      "action": "proxy.request",
      "method": "POST",
      "path": "/v1/chat/completions",
      "status": 200,
      "latency_ms": 342,
      "credential_id": "550e8400-e29b-41d4-a716-446655440000",
      "identity_id": "user-abc-123",
      "ip_address": "203.0.113.42",
      "created_at": "2026-03-20T10:15:30Z"
    }
  ],
  "next_cursor": "12344"
}
```

### Pagination

The API uses cursor-based pagination for efficient deep paging. Use the `next_cursor` value from the response as the `cursor` parameter in your next request.

```typescript
// Walk through all entries
let cursor: string | undefined;
do {
  const { data } = await vault.audit.list({ limit: 100, cursor });
  processEntries(data.data);
  cursor = data.next_cursor;
} while (cursor);
```

## Log Retention

### Retention by Plan

| Plan | Retention Period |
|------|-----------------|
| Free | 7 days |
| Pro | 90 days |
| Enterprise | Custom (up to 7 years) |

### Self-Hosted Retention

For self-hosted deployments, you control retention entirely. We recommend partitioning the audit table by month for efficient management:

```sql
-- Partition by month
CREATE TABLE audit_log_2026_03 PARTITION OF audit_log
    FOR VALUES FROM ('2026-03-01') TO ('2026-04-01');

-- Drop old partitions when no longer needed
DROP TABLE audit_log_2025_03;
```

### Archiving for Compliance

Before deleting old logs, export them for long-term storage:

```bash
# Export to S3
psql -c "COPY (SELECT * FROM audit_log WHERE created_at < NOW() - INTERVAL '1 year') TO STDOUT CSV" \
  | aws s3 cp - s3://my-audit-bucket/audit-2025.csv
```

## Security Monitoring

### Suspicious Activity Patterns

Use audit logs to detect potential security issues:

**Failed authentication attempts** -- A high rate of 401 responses from a single IP may indicate a brute force attack.

**Unusual proxy volume** -- A sudden spike in proxy requests for a single credential may indicate token abuse.

**After-hours access** -- API or proxy requests outside normal business hours may warrant investigation.

### Alerting

Integrate audit data with your SIEM or monitoring platform for real-time alerts:

```yaml
# Example Datadog monitor
name: "High Rate of Failed Auth"
type: metric alert
query: "avg(last_5m):sum:llmvault.audit.status{status:401} > 100"
message: "Possible brute force attack detected"
```

### Dashboard Integration

Audit logs power usage analytics in the LLMVault dashboard:

- Request count per credential
- Last used timestamp for each credential
- Usage trends over time

## Compliance

### SOC 2

Audit logging satisfies key SOC 2 control requirements:

| Control | How LLMVault Addresses It |
|---------|--------------------------|
| CC6.1 | All access logged with timestamp, actor, and action |
| CC7.2 | Complete audit trail of system events |
| CC7.3 | Log retention enforced per policy |

### GDPR Article 30

Audit logs support GDPR record-keeping requirements:

- Processing activities are recorded
- Access to personal data is logged
- Retention policies are configurable and enforced

### Exporting Logs for Auditors

```bash
# Export all logs for a date range
psql -c "\copy (SELECT * FROM audit_log
  WHERE org_id = 'your-org-id'
  AND created_at BETWEEN '2026-01-01' AND '2026-01-31')
  TO '/tmp/audit-jan-2026.csv' CSV HEADER;"
```

## Best Practices

1. **Monitor for anomalies** -- Set up alerts for failed authentication spikes and unusual access patterns
2. **Set appropriate retention** -- Match your retention period to your compliance requirements (HIPAA: 6 years, SOC 2: 1 year minimum)
3. **Export to your SIEM** -- Ship logs to your central security monitoring platform for correlation with other signals
4. **Protect log integrity** -- For tamper-evident logs, enable database WAL archiving and ship to WORM storage (e.g., S3 Glacier)
5. **Review regularly** -- Schedule periodic audit reviews: daily for auth failures, weekly for access anomalies, monthly for full access review
6. **Partition for performance** -- For high-traffic deployments, partition the audit table by month to keep queries fast

## Troubleshooting

### Missing Logs

If audit entries are not appearing:

1. **Check connectivity** -- Ensure the application can write to the database
2. **Check disk space** -- The audit table may have exhausted available storage
3. **Check application logs** -- Look for buffer overflow warnings indicating dropped entries

### Storage Growth

If the audit table grows too large:

1. **Reduce retention** -- Shorten the retention period if compliance allows
2. **Archive old logs** -- Export to cold storage before deletion
3. **Partition the table** -- Monthly partitions make it easy to drop old data
4. **Exclude health checks** -- Configure your load balancer health checks to bypass audit logging
