---
title: Managing Identities
description: Viewing identities, details, linked credentials, and rate limits
---

# Managing Identities

Identities represent your end-users in the LLMVault system. They enable per-user rate limiting, usage tracking, and credential association.

## Identities List

Navigate to **Security > Identities** to view all identities.

### List View

The identities table displays:

| Column | Description |
|--------|-------------|
| External ID | Your application's user identifier |
| Rate Limits | Applied rate limit badges |
| Requests | Total request count |
| Meta | Metadata key-value badges |
| Created | Creation date |

### Rate Limit Badges

Rate limits display as formatted badges:
```
requests: 100/1h
```

Format: `{name}: {limit}/{duration}`

### Searching

Search identities by:
- External ID

## Identity Detail Page

Click an identity to view full details.

### Configuration Card

| Field | Description |
|-------|-------------|
| External ID | User identifier from your application |
| Credentials | Count of linked credentials |
| Requests | Total proxy requests made |
| Created | Timestamp |
| ID | Internal UUID |

### Rate Limits Card

Lists all rate limits applied to this identity:

| Field | Description |
|-------|-------------|
| Name | Rate limit identifier |
| Limit / Duration | Request quota and window |
| Description | Human-readable explanation |

Example:
```
requests: 100 / 1h
100 requests per 1h window
```

### Linked Credentials

Table showing credentials linked to this identity:

| Column | Description |
|--------|-------------|
| Label | Credential name |
| Provider | LLM provider |
| Status | Active, Revoked, Expiring |
| Remaining | Request cap progress |
| Created | Timestamp |

Click a credential to navigate to its detail page.

### Metadata

Displays JSON metadata attached to the identity:

```json
{
  "tier": "premium",
  "region": "us-east"
}
```

## Creating Identities (API)

Identities are typically created programmatically:

```typescript
import { LLMVault } from "@llmvault/sdk";

const vault = new LLMVault({ apiKey: "ak_live_..." });

const { data, error } = await vault.identities.create({
  external_id: "user_12345",
  meta: { tier: "premium" },
  rate_limit: [
    { name: "requests", limit: 100, duration: 3600000 } // 1 hour in ms
  ]
});
```

Or auto-created when minting a token with `external_id`.

## Linking Credentials to Identities

When creating a credential:
- Set `identity_id` to link to existing identity
- Set `external_id` to auto-create and link

The identity detail page shows all linked credentials.

## Rate Limit Format

Rate limits are specified as:

| Field | Type | Description |
|-------|------|-------------|
| name | string | Identifier (e.g., "requests") |
| limit | number | Maximum requests allowed |
| duration | number | Window in milliseconds |

Common durations:
- 1 minute: `60000`
- 1 hour: `3600000`
- 1 day: `86400000`

## Updating Identities

From the detail page, you can:
- Edit identity metadata
- Modify rate limits
- Delete the identity

**Note:** UI editing is currently in development. Use the API for now:

```typescript
import { LLMVault } from "@llmvault/sdk";

const vault = new LLMVault({ apiKey: "ak_live_..." });

const { data, error } = await vault.identities.update(identityId, {
  meta: { updated: true },
  rate_limit: [
    { name: "requests", limit: 200, duration: 3600000 }
  ]
});
```

## Deleting Identities

**Warning:** Deleting an identity:
- Permanently removes the record
- Unlinks associated credentials
- Does not delete credentials
- Preserves audit log entries

Use the **"Delete"** button on the identity detail page.

## Identity-Based Usage Tracking

Identities enable:
- Per-user request counting
- Individual rate limit enforcement
- Usage analytics by user
- Cost allocation

## Best Practices

1. **Use consistent external IDs** - Match your application's user IDs
2. **Set appropriate rate limits** - Prevent abuse per user
3. **Attach metadata** - Store plan tier, region, etc.
4. **Link credentials** - Track which keys belong to which users
5. **Monitor usage** - Regularly review request counts
