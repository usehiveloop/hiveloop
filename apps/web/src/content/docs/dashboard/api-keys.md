---
title: API Keys
description: Creating API keys, managing scopes, expiration, and revocation
---

# API Keys

API Keys provide programmatic access to the LLMVault management API. They are separate from proxy tokens and are used for administrative operations.

## API Keys List

Navigate to **Security > API Keys** to view all keys.

### List View

The API keys table displays:

| Column | Description |
|--------|-------------|
| Name | Human-readable identifier |
| Key | Prefix with masked remainder (`ak_abc...****`) |
| Scopes | Permission badges |
| Status | Active, Expiring, or Revoked |
| Last Used | Relative timestamp |
| Created | Creation date |
| Actions | Revoke button |

### Status Filters

Filter keys by status:
- **All** - Show all keys
- **Active** - Valid, non-expired keys
- **Expired** - Past expiration date
- **Revoked** - Explicitly disabled

Status counts display next to each filter tab.

### Searching

Search by:
- Key name
- Key prefix

## Creating an API Key

Click **"Create API Key"** to generate a new key.

### Configuration

**Required Fields:**

| Field | Description | Example |
|-------|-------------|---------|
| Name | Identifier for this key | "Production API Key" |
| Scopes | Permissions granted | See below |

**Scope Options:**

| Scope | Access |
|-------|--------|
| `all` | Full access to all endpoints |
| `connect` | Connect UI and OAuth flows |
| `credentials` | Credential management |
| `tokens` | Token minting and management |

Select multiple scopes by clicking each button. Selecting `all` automatically deselects others.

**Optional Fields:**

| Field | Description | Options |
|-------|-------------|---------|
| Expiration | When key becomes invalid | Never (default), 30 days, 90 days, 1 year |

### Key Creation Success

After creation, a success dialog displays:

**Important:** The full API key is shown **only once**.

```
ak_live_abc123def456...
```

Copy the key immediately. It cannot be retrieved later.

## API Key Statuses

| Status | Description |
|--------|-------------|
| Active | Valid for API requests |
| Expiring | Expires within 24 hours |
| Revoked | Explicitly disabled |

## Revoking API Keys

To revoke a key:

1. Find the key in the list
2. Click **"Revoke"** in the Actions column
3. Confirm the revocation
4. Key is immediately invalidated

**Effects:**
- Key removed from cache
- Future requests return 401 Unauthorized
- Historical audit log entries remain

## Using API Keys

Include the API key in the `Authorization` header:

```bash
curl https://api.llmvault.dev/v1/credentials \
  -H "Authorization: Bearer ak_live_..."
```

## SDK Authentication

```typescript
import { LLMVault } from "@llmvault/sdk";

const sdk = new LLMVault({
  apiKey: "ak_live_...",
});
```

## Security Considerations

1. **Store securely** - Treat API keys like passwords
2. **Rotate regularly** - Create new keys, revoke old ones
3. **Use minimal scopes** - Grant only needed permissions
4. **Set expiration** - Force periodic rotation
5. **Monitor usage** - Check last_used_at regularly
6. **Revoke immediately** - If compromise suspected

## Rate Limits

API keys are subject to rate limiting based on your plan. See the [Billing](/dashboard/billing) page for details.
