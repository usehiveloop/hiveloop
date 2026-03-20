---
title: Integration Connections
description: Learn about OAuth-based integration connections, supported providers, resource selection, scopes, and managing integration credentials in Connect.
---

# Integration Connections

Connect supports OAuth-based integrations powered by Nango, allowing your users to securely connect third-party services like Slack, GitHub, Notion, and more. This guide covers the OAuth flow, resource selection, and integration management.

## Overview

Unlike LLM providers that use API keys, integrations use OAuth 2.0 (or similar protocols) for authentication. Connect handles the entire OAuth flow:

```
┌─────────────────────────────────────────────────────────────┐
│                 OAuth Integration Flow                       │
│                                                              │
│  User                                                        │
│   │                                                          │
│   │ 1. Select integration                                    │
│   ▼                                                          │
│  Connect Widget                                              │
│   │                                                          │
│   │ 2. Create connect session                                │
│   ▼                                                          │
│  Nango                                                       │
│   │                                                          │
│   │ 3. Open OAuth popup                                      │
│   ▼                                                          │
│  Provider (Slack/GitHub/etc)                                 │
│   │                                                          │
│   │ 4. User authorizes                                       │
│   ▼                                                          │
│  Nango                                                       │
│   │                                                          │
│   │ 5. Store tokens                                          │
│   ▼                                                          │
│  LLMVault                                                    │
│   │                                                          │
│   │ 6. Optional: Resource selection                          │
│   ▼                                                          │
│  Your App ◄─── Connection ID + Resources                     │
└─────────────────────────────────────────────────────────────┘
```

## OAuth Flow

### Step 1: Integration Selection

Users see integrations configured for your organization:

```
┌──────────────────────────────┐
│  Connect an integration [X]  │
├──────────────────────────────┤
│                              │
│  ┌────┬───────────────────┐  │
│  │ 📱 │ Slack   Connected │  │
│  ├────┼───────────────────┤  │
│  │ 🐙 │ GitHub  Connect   │  │
│  ├────┼───────────────────┤  │
│  │ 📝 │ Notion  Connect   │  │
│  └────┴───────────────────┘  │
│                              │
└──────────────────────────────┘
```

### Step 2: OAuth Authentication

When user clicks "Connect":

```
┌──────────────────────────────┐
│  [←]  Connect           [X]  │
├──────────────────────────────┤
│                              │
│         ┌──────────┐         │
│         │   🐙     │         │
│         │  GitHub  │         │
│         └──────────┘         │
│                              │
│     ⟳ Connecting to          │
│       GitHub...              │
│                              │
│   [OAuth popup opens]        │
└──────────────────────────────┘
```

A popup window opens for the OAuth flow:

```
┌─────────────────────────────────────────────────────────────┐
│  Authorize MyApp                                  GitHub    │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  MyApp would like to access your GitHub account.            │
│                                                              │
│  This application will be able to:                          │
│    ✓ Read your repositories                                 │
│    ✓ Read organization data                                 │
│                                                              │
│  [Cancel]                                    [Authorize]    │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

### Step 3: Resource Selection

After OAuth completion, users select specific resources:

```
┌──────────────────────────────┐
│  Select repositories    [X]  │
├──────────────────────────────┤
│  🐙 MyApp GitHub Integration │
│     Choose what the AI       │
│     can access               │
├──────────────────────────────┤
│  [Repositories] [Issues]     │
├──────────────────────────────┤
│  ┌─────────────────────────┐ │
│  │ 🔍 Search...            │ │
│  ├─────────────────────────┤ │
│  │ ☐ myapp/frontend        │ │
│  │ ☑ myapp/backend         │ │
│  │ ☑ myapp/docs            │ │
│  │ ☐ myapp/infrastructure  │ │
│  └─────────────────────────┘ │
├──────────────────────────────┤
│  [Save Selection]            │
│  Skip                        │
└──────────────────────────────┘
```

### Step 4: Success

```
┌──────────────────────────────┐
│                              │
│           ✓                  │
│      Successfully            │
│      connected to GitHub     │
│                              │
│   ┌─────────────────────┐    │
│   │     Manage          │    │
│   └─────────────────────┘    │
│                              │
│   ┌─────────────────────┐    │
│   │       Done          │    │
│   └─────────────────────┘    │
└──────────────────────────────┘
```

## Supported Integrations

Connect supports any provider available in the Nango catalog (200+ integrations). Common integrations include:

### Communication

| Provider | Provider Key | Auth Mode |
|----------|--------------|-----------|
| Slack | `slack` | OAUTH2 |
| Discord | `discord` | OAUTH2 |
| Microsoft Teams | `microsoft-teams` | OAUTH2 |
| Zoom | `zoom` | OAUTH2 |

### Development

| Provider | Provider Key | Auth Mode |
|----------|--------------|-----------|
| GitHub | `github` | OAUTH2 |
| GitLab | `gitlab` | OAUTH2 |
| Bitbucket | `bitbucket` | OAUTH2 |
| Jira | `jira` | OAUTH2 |

### Productivity

| Provider | Provider Key | Auth Mode |
|----------|--------------|-----------|
| Notion | `notion` | OAUTH2 |
| Confluence | `confluence` | OAUTH2 |
| Google Drive | `google-drive` | OAUTH2 |
| Dropbox | `dropbox` | OAUTH2 |

### CRM & Support

| Provider | Provider Key | Auth Mode |
|----------|--------------|-----------|
| Salesforce | `salesforce` | OAUTH2 |
| HubSpot | `hubspot` | OAUTH2 |
| Zendesk | `zendesk` | OAUTH2 |
| Intercom | `intercom` | OAUTH2 |

### Auth Modes

| Mode | Description |
|------|-------------|
| `OAUTH1` | OAuth 1.0a (Twitter, etc.) |
| `OAUTH2` | OAuth 2.0 (most common) |
| `OAUTH2_CC` | OAuth 2.0 Client Credentials |
| `BASIC` | HTTP Basic Auth |
| `API_KEY` | API Key authentication |
| `NONE` | No authentication required |

## Configuring Integrations

Integrations are configured in the LLMVault Dashboard or via API:

### Create Integration

```http
POST /v1/integrations
Authorization: Bearer {api_key}
Content-Type: application/json

{
  "provider": "github",
  "display_name": "GitHub Team Access",
  "credentials": {
    "type": "OAUTH2",
    "client_id": "your-client-id",
    "client_secret": "your-client-secret"
  },
  "meta": {
    "scopes": ["repo", "read:org"],
    "team": "engineering"
  }
}
```

**Response:**

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "unique_key": "github-550e8400",
  "provider": "github",
  "display_name": "GitHub Team Access",
  "meta": {
    "scopes": ["repo", "read:org"],
    "team": "engineering"
  },
  "created_at": "2026-03-20T08:45:00Z",
  "updated_at": "2026-03-20T08:45:00Z"
}
```

### Credentials by Auth Mode

**OAuth 2.0:**

```json
{
  "type": "OAUTH2",
  "client_id": "your-client-id",
  "client_secret": "your-client-secret"
}
```

**OAuth 1.0a:**

```json
{
  "type": "OAUTH1",
  "client_id": "your-consumer-key",
  "client_secret": "your-consumer-secret"
}
```

**App-based (GitHub Apps):**

```json
{
  "type": "APP",
  "app_id": "123456",
  "app_link": "https://github.com/apps/your-app",
  "private_key": "-----BEGIN RSA PRIVATE KEY-----\n..."
}
```

### List Available Providers

```http
GET /v1/integrations/providers
Authorization: Bearer {api_key}
```

**Response:**

```json
[
  {
    "name": "github",
    "display_name": "GitHub",
    "auth_mode": "OAUTH2",
    "webhook_user_defined_secret": false
  },
  {
    "name": "slack",
    "display_name": "Slack",
    "auth_mode": "OAUTH2",
    "webhook_user_defined_secret": true
  }
]
```

## Resource Selection

After OAuth completion, users can select specific resources to grant access to:

### Resource Types

Each integration can expose multiple resource types:

| Integration | Resource Types |
|-------------|----------------|
| Slack | `channels`, `users`, `files` |
| GitHub | `repositories`, `issues`, `pull_requests` |
| Notion | `pages`, `databases` |
| Google Drive | `files`, `folders` |

### Resource Discovery

The widget fetches available resources from the provider:

```http
GET /v1/widget/integrations/{id}/resources/{type}/available?nango_connection_id=xxx
Authorization: Bearer {session_token}
```

**Response:**

```json
{
  "resources": [
    {
      "id": "C123456",
      "name": "general",
      "type": "channel"
    },
    {
      "id": "C789012",
      "name": "engineering",
      "type": "channel"
    }
  ]
}
```

### Saving Resource Selection

Users' selections are saved with the connection:

```http
PATCH /v1/widget/integrations/{id}/connections/{connection_id}
Authorization: Bearer {session_token}
Content-Type: application/json

{
  "resources": {
    "channels": ["C123456", "C789012"],
    "users": ["U123456"]
  }
}
```

**Resource selection payload in events:**

```typescript
connect.open({
  onResourceSelection: (payload) => {
    console.log(payload.integrationId); // "slack-prod"
    console.log(payload.resources);
    // {
    //   "channels": ["C123456", "C789012"],
    //   "users": ["U123456"]
    // }
  },
});
```

## Scopes

OAuth scopes define what permissions your application requests:

### Common Scopes

**GitHub:**
- `repo` - Full repository access
- `public_repo` - Public repository access only
- `read:org` - Read organization data

**Slack:**
- `channels:read` - View channel information
- `chat:write` - Send messages
- `users:read` - View user profiles

**Notion:**
- `read_content` - Read pages and databases
- `write_content` - Create and edit content

### Configuring Scopes

Scopes are configured in your integration's `meta`:

```http
POST /v1/integrations
Content-Type: application/json

{
  "provider": "github",
  "display_name": "GitHub Read Only",
  "credentials": { /* ... */ },
  "meta": {
    "scopes": ["read:user", "read:org"]
  }
}
```

Or use default scopes defined by Nango for the provider.

## Managing Integration Connections

### List Integrations in Widget

```http
GET /v1/widget/integrations
Authorization: Bearer {session_token}
```

**Response:**

```json
[
  {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "unique_key": "github-550e8400",
    "provider": "github",
    "display_name": "GitHub Team Access",
    "auth_mode": "OAUTH2",
    "connection_id": "550e8400-e29b-41d4-a716-446655440001",
    "nango_connection_id": "user-123-github-550e8400",
    "resources": [
      {
        "type": "repositories",
        "display_name": "Repositories",
        "description": "Git repositories",
        "icon": "repo"
      }
    ],
    "selected_resources": {
      "repositories": ["repo-1", "repo-2"]
    }
  }
]
```

### Connection Status

| Field | Description |
|-------|-------------|
| `connection_id` | Set if user has connected |
| `nango_connection_id` | Nango's internal connection ID |
| `selected_resources` | Resources user granted access to |

### Disconnect Integration

Users can revoke access:

```http
DELETE /v1/widget/integrations/{id}/connections/{connection_id}
Authorization: Bearer {session_token}
```

This revokes the OAuth tokens in Nango and deletes the connection record.

## Widget API Reference

### List Integrations

```http
GET /v1/widget/integrations
Authorization: Bearer {session_token}
```

Returns integrations filtered by `allowed_integrations` from the session.

### Create Connect Session

```http
POST /v1/widget/integrations/{id}/connect-session
Authorization: Bearer {session_token}
```

Creates a Nango connect session for OAuth:

```json
{
  "token": "nango_connect_session_xxx",
  "provider_config_key": "org-id_github-550e8400"
}
```

### Create Connection

```http
POST /v1/widget/integrations/{id}/connections
Authorization: Bearer {session_token}
Content-Type: application/json

{
  "nango_connection_id": "user-123-github-550e8400",
  "resources": {
    "repositories": ["repo-1", "repo-2"]
  }
}
```

### Update Connection Resources

```http
PATCH /v1/widget/integrations/{id}/connections/{connection_id}
Authorization: Bearer {session_token}
Content-Type: application/json

{
  "resources": {
    "repositories": ["repo-1", "repo-2", "repo-3"]
  }
}
```

### Delete Connection

```http
DELETE /v1/widget/integrations/{id}/connections/{connection_id}
Authorization: Bearer {session_token}
```

### List Available Resources

```http
GET /v1/widget/integrations/{id}/resources/{type}/available?nango_connection_id=xxx
Authorization: Bearer {session_token}
```

## Error Handling

### OAuth Errors

```json
// User denied authorization
{
  "type": "error",
  "payload": {
    "code": "integration_failed",
    "message": "User denied authorization"
  }
}

// Connection already exists
{
  "error": "already connected to this integration"
}

// Invalid integration
{
  "error": "integration not found"
}
```

### Session Errors

```json
// No create permission
{
  "error": "permission denied"
}

// Integration not in allowed list
{
  "error": "integration not found"
}
```

## Best Practices

1. **Minimal Scopes**: Request only necessary permissions
2. **Resource Selection**: Always offer granular resource selection
3. **Clear Names**: Use descriptive `display_name` for integrations
4. **Webhook Handling**: Configure webhooks for connection status changes
5. **Token Refresh**: Nango handles OAuth token refresh automatically

## Next Steps

- [Sessions](./sessions) — Create sessions with integration restrictions
- [Embedding](./embedding) — Embed the integration selection flow
- [Providers](./providers) — LLM provider connections
- [Frontend SDK](./frontend-sdk) — SDK reference
