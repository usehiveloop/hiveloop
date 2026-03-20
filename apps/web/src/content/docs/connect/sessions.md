---
title: Connect Sessions
description: Learn how to create and manage Connect sessions, configure permissions, restrict allowed integrations, and understand session lifecycle.
---

# Connect Sessions

Connect sessions are short-lived, cryptographically secure tokens that authorize the Connect widget to act on behalf of your users. This guide covers session creation, configuration, and lifecycle management.

## Session Overview

A Connect session:
- Links to a specific user (via `identity_id` or `external_id`)
- Has a limited time-to-live (TTL)
- Can be scoped to specific permissions
- Can restrict available integrations
- Validates origin for embedded usage

```
┌─────────────────────────────────────────────────────────────┐
│                     Session Creation                         │
│                                                              │
│   Your Backend ──POST /v1/connect/sessions──► LLMVault API  │
│                                    │                         │
│                                    ▼                         │
│                           ┌─────────────────┐               │
│                           │  Session Token  │               │
│                           │  (JWT-style)    │               │
│                           │  Expires: 15m   │               │
│                           └────────┬────────┘               │
│                                    │                         │
│   Your Frontend ◄────session_token─┘                         │
│         │                                                    │
│         ▼                                                    │
│   Connect.open({ sessionToken })                             │
│         │                                                    │
│         ▼                                                    │
│   Widget validates session on load                           │
└─────────────────────────────────────────────────────────────┘
```

## Creating Sessions

### Basic Session

Create a session linked to your user:

```http
POST /v1/connect/sessions
Authorization: Bearer {your_api_key}
Content-Type: application/json

{
  "external_id": "user-123"
}
```

**Response:**

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "session_token": "connect_session_xxx",
  "external_id": "user-123",
  "expires_at": "2026-03-20T09:00:00Z",
  "created_at": "2026-03-20T08:45:00Z"
}
```

### Session with Identity Resolution

You can link sessions to existing LLMVault identities or auto-create them:

```http
POST /v1/connect/sessions
Content-Type: application/json

{
  "identity_id": "550e8400-e29b-41d4-a716-446655440000"
}
```

Or use `external_id` to auto-create an identity if it doesn't exist:

```http
POST /v1/connect/sessions
Content-Type: application/json

{
  "external_id": "user-123"
}
```

**Resolution logic:**
1. If `identity_id` provided → Validate identity exists in your org
2. If `external_id` provided → Find existing identity or create new one
3. One of `identity_id` or `external_id` is **required**

## Permissions

Control what actions users can perform within the Connect widget:

```http
POST /v1/connect/sessions
Content-Type: application/json

{
  "external_id": "user-123",
  "permissions": ["create", "list", "verify"]
}
```

### Available Permissions

| Permission | Description | API Endpoints Affected |
|------------|-------------|------------------------|
| `create` | Create new connections | `POST /v1/widget/connections` |
| `list` | View existing connections | `GET /v1/widget/connections` |
| `delete` | Revoke connections | `DELETE /v1/widget/connections/{id}` |
| `verify` | Verify connection validity | `POST /v1/widget/connections/{id}/verify` |

### Permission Behavior

```typescript
// Read-only session (view connections only)
{ "permissions": ["list"] }

// Full access (default if not specified)
{ "permissions": ["create", "list", "delete", "verify"] }

// Create-only (add connections but not view existing)
{ "permissions": ["create"] }
```

**Permission denied response:**

```json
{
  "error": "permission denied"
}
```

## Allowed Integrations

Restrict which integrations users can connect:

```http
POST /v1/connect/sessions
Content-Type: application/json

{
  "external_id": "user-123",
  "allowed_integrations": ["slack-prod", "github-team"]
}
```

### Use Cases

1. **Feature-gating**: Only show integrations based on user's plan
2. **Organization restrictions**: Limit to company-approved tools
3. **Progressive rollout**: Gradually enable new integrations

### Integration Keys

Integration keys are the `unique_key` values from your integration configuration:

```http
GET /v1/integrations
Authorization: Bearer {api_key}
```

**Response:**

```json
{
  "data": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "unique_key": "slack-prod",
      "display_name": "Slack Production"
    }
  ]
}
```

### Invalid Integration Error

If you specify an integration that doesn't exist in your organization:

```json
{
  "error": "unknown integration: invalid-key"
}
```

## Allowed Origins

For additional security, restrict which domains can use the session:

```http
POST /v1/connect/sessions
Content-Type: application/json

{
  "external_id": "user-123",
  "allowed_origins": ["https://app.example.com", "https://dashboard.example.com"]
}
```

### Origin Validation

1. Session `allowed_origins` must be a subset of your organization's `allowed_origins`
2. The widget validates the parent window origin on load
3. Invalid origins result in a session error

### Organization Origins

Configure organization-wide allowed origins in the Dashboard or via API:

```http
PATCH /v1/org
Content-Type: application/json

{
  "allowed_origins": ["https://*.example.com"]
}
```

### Origin Format

Origins must include protocol and host:

| Valid | Invalid |
|-------|---------|
| `https://app.example.com` | `app.example.com` (missing protocol) |
| `http://localhost:3000` | `*.example.com` (wildcards not supported) |
| `https://example.com:8080` | `/path` (path not allowed) |

## Session TTL

Sessions have a limited lifetime (default: 15 minutes, max: 30 minutes):

```http
POST /v1/connect/sessions
Content-Type: application/json

{
  "external_id": "user-123",
  "ttl": "30m"
}
```

### TTL Options

| TTL | Use Case |
|-----|----------|
| `5m` | Quick connection flows |
| `15m` | Standard (default) |
| `30m` | Complex multi-step flows |

### Invalid TTL Errors

```json
// Exceeds maximum
{
  "error": "ttl exceeds maximum of 30m"
}

// Invalid format
{
  "error": "invalid ttl: must be a valid Go duration (e.g. 15m, 30m)"
}

// Non-positive
{
  "error": "ttl must be positive"
}
```

## Session Metadata

Attach custom metadata to sessions for tracking:

```http
POST /v1/connect/sessions
Content-Type: application/json

{
  "external_id": "user-123",
  "metadata": {
    "source": "onboarding",
    "plan": "enterprise",
    "region": "us-east",
    "utm_campaign": "spring-launch"
  }
}
```

Metadata is stored with the session and available in webhook payloads.

## Complete Session Example

```typescript
import { LLMVault } from "@llmvault/sdk";

const vault = new LLMVault({ apiKey: process.env.LLMVAULT_API_KEY });

// Backend: Create a scoped session
async function createConnectSession(user: User) {
  const { data, error } = await vault.connect.sessions.create({
    // User identification
    external_id: user.id,

    // Scoped permissions
    permissions: user.plan === 'enterprise'
      ? ['create', 'list', 'delete', 'verify']
      : ['create', 'list'],

    // Integration restrictions
    allowed_integrations: user.allowedIntegrations,

    // Security
    allowed_origins: ['https://app.example.com'],
    ttl: '15m',

    // Tracking
    metadata: {
      user_plan: user.plan,
      source: 'settings_page',
    },
  });

  if (error) {
    throw new Error('Failed to create session');
  }

  return {
    token: data.session_token,
    expiresAt: data.expires_at,
  };
}
```

## Session Lifecycle

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   Created   │────►│   Active    │────►│  Expired    │
│   (POST)    │     │  (Widget)   │     │ (Timeout)   │
└─────────────┘     └─────────────┘     └─────────────┘
                           │
                           ▼
                    ┌─────────────┐
                    │   Closed    │
                    │  (User)     │
                    └─────────────┘
```

### State Descriptions

| State | Description |
|-------|-------------|
| **Created** | Session generated, token returned |
| **Active** | Widget loaded, user interacting |
| **Closed** | User closed widget or completed flow |
| **Expired** | TTL reached, token invalid |

### Widget Session Validation

When the widget loads, it validates the session:

```http
GET /v1/widget/session
Authorization: Bearer {session_token}
```

**Success response:**

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "external_id": "user-123",
  "allowed_integrations": ["slack-prod"],
  "permissions": ["create", "list"],
  "expires_at": "2026-03-20T09:00:00Z",
  "activated_at": "2026-03-20T08:46:00Z"
}
```

**Error response (401):**

```json
{
  "error": "missing session"
}
```

## Session Security Best Practices

1. **Short TTL**: Use the minimum TTL needed for your flow
2. **Origin Restriction**: Always set `allowed_origins` for production
3. **Scoped Permissions**: Only grant necessary permissions
4. **Backend Creation**: Never create sessions in client-side code
5. **One-time Use**: Treat session tokens as single-use credentials

### Security Checklist

- [ ] Sessions created server-side only
- [ ] TTL ≤ 15 minutes for standard flows
- [ ] `allowed_origins` configured
- [ ] Minimal permissions granted
- [ ] Integration restrictions applied (if needed)
- [ ] Metadata for audit logging

## Error Handling

### Common Session Errors

```json
// Missing identity
{
  "error": "identity_id or external_id is required"
}

// Identity not found
{
  "error": "identity not found"
}

// Invalid origin
{
  "error": "invalid origin: example.com (must be http(s)://host)"
}

// Origin not allowed by org
{
  "error": "origin not in org's allowed_origins: https://evil.com"
}

// Invalid permission
{
  "error": "invalid permission: admin"
}
```

## Session vs. Connection

| | Session | Connection |
|--|---------|------------|
| **Purpose** | Authorize widget access | Store user credentials |
| **Lifetime** | 15-30 minutes | Until revoked |
| **Scope** | Per widget open | Per provider/integration |
| **Storage** | Temporary cache | Encrypted database |
| **Multiple** | Create many | One per provider per user |

## Next Steps

- [Embedding](./embedding) — Use sessions to open the widget
- [Providers](./providers) — Configure LLM provider connections
- [Integrations](./integrations) — Set up OAuth integrations
- [Frontend SDK](./frontend-sdk) — SDK reference
