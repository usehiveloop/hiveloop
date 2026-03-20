---
title: Managing Tokens
description: Minting proxy tokens, configuration, integration access, and revocation
---

# Managing Tokens

Tokens are short-lived credentials scoped to a single parent credential. They authenticate requests to the LLM proxy and can be configured with request caps, expiration times, and integration access.

## Tokens List

Navigate to **Security > Tokens** to view all tokens.

### List View

The tokens table displays:

| Column | Description |
|--------|-------------|
| JTI | Token identifier (truncated) |
| Credential | Parent credential ID |
| Status | Active, Revoked, or Expiring |
| Remaining | Request cap progress bar |
| Expires | Relative expiration time |
| Created | Creation timestamp |

### Filtering

Filter tokens by credential:
- Select **"All Credentials"** to show all tokens
- Choose a specific credential to filter

### Searching

Search by:
- JTI (token identifier)
- Credential ID

## Minting a Token

Click **"Mint Token"** to create a new token.

### Required Configuration

| Field | Description | Default |
|-------|-------------|---------|
| Credential | Parent credential to scope to | First active credential |
| TTL | Token lifetime | `1h` |

**TTL Options:**
- Go duration format (e.g., `30m`, `1h`, `24h`)
- Maximum: 24 hours

### Optional Configuration

| Field | Description | Example |
|-------|-------------|---------|
| Remaining | Request cap | `1000` |
| Refill Amount | Auto-refill quantity | `1000` |
| Refill Interval | Refill period | `1h` |
| Metadata | JSON object | `{"user_id": "123"}` |

### Integration Access (Scopes)

Expand **"Integration Access"** to grant MCP tool access:

1. Select connections from available integrations
2. Choose permitted actions per connection
3. Select specific resources (if applicable)

### Mint Success Dialog

After minting, a success dialog displays:

**Critical Warning:**
> This token is shown only once. Copy it now — you won't be able to see it again.

**Displayed Information:**
- Full token string (with `ptok_` prefix)
- JTI identifier
- Expiration time
- MCP endpoint URL (if scopes configured)

**Quick Start Examples:**

**cURL:**
```bash
curl https://api.llmvault.dev/v1/proxy/v1/chat/completions \
  -H "Authorization: Bearer ptok_..." \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-4o","messages":[...]}'
```

**MCP Configuration (Claude Desktop):**
```json
{
  "mcpServers": {
    "llmvault": {
      "url": "https://mcp.llmvault.dev/{jti}",
      "headers": {
        "Authorization": "Bearer ptok_..."
      }
    }
  }
}
```

Click **"Copy"** buttons to copy values to clipboard.

### SDK Equivalent

```typescript
import { LLMVault } from "@llmvault/sdk";

const vault = new LLMVault({ apiKey: "ak_live_..." });

const { data, error } = await vault.tokens.create({
  credential_id: "cred-uuid",
  name: "My Token",
  scopes: ["slack:send_message"],
  expires_in: "1h"
});
```

## Token Statuses

| Status | Description |
|--------|-------------|
| Active | Valid and can authenticate requests |
| Revoked | Explicitly disabled or expired |
| Expiring | Remaining requests at or below zero |

## Revoking Tokens

Tokens can be revoked via:
- **API:** `DELETE /v1/tokens/{jti}`
- **SDK:** `vault.tokens.delete(jti)`

**Effects:**
- Token is immediately invalidated
- Any in-flight requests may still complete
- MCP connections using this token are disconnected

## Token Detail on Credential Page

From a credential's detail page, view minted tokens:
- Shows tokens scoped to that credential
- Paginated list (20 per page)
- Same columns as main tokens list

## Token Lifecycle

```
Mint → Active → [Revoked | Expired]
        ↓
    Requests
        ↓
   Depleted → Expiring
```

## Best Practices

1. **Short TTLs** - Use 1 hour or less for production
2. **Request caps** - Set `remaining` to limit exposure
3. **Metadata** - Attach user/context info for tracking
4. **Revoke promptly** - Invalidate tokens when no longer needed
5. **Scopes** - Grant only the minimal integration access needed
