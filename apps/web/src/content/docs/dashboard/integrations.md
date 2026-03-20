---
title: Integrations
description: Adding integrations, auth modes, credentials, connections, and webhooks
---

# Integrations

Integrations connect LLMVault to third-party services via OAuth, enabling your users to authenticate with external providers.

## Integrations List

Navigate to **Experience > Integrations** to view configured integrations.

### List View

The integrations table displays:

| Column | Description |
|--------|-------------|
| Name | Display name with provider logo |
| Provider | Provider identifier |
| Created | Creation date |
| Updated | Last modification |
| Actions | Delete button |

### Searching

Search by:
- Display name
- Provider name

## Adding an Integration

Click **"Add Integration"** to configure a new provider.

### Step 1: Select Provider

Browse available providers from the catalog:
- Search by name
- View provider logos
- Click to select

### Step 2: Configure Integration

**Required Fields:**

| Field | Description | Example |
|-------|-------------|---------|
| Display Name | Human-readable name | "Slack Production" |

**Credential Fields** (vary by auth mode):

### Auth Modes

| Mode | Description | Required Credentials |
|------|-------------|---------------------|
| OAUTH1 | OAuth 1.0a | `client_id`, `client_secret` |
| OAUTH2 | OAuth 2.0 | `client_id`, `client_secret` |
| TBA | Token-based auth | `client_id`, `client_secret` |
| APP | App-based | `app_id`, `app_link`, `private_key` |
| CUSTOM | Custom OAuth | `client_id`, `client_secret`, `app_id`, `app_link`, `private_key` |
| MCP_OAUTH2 | MCP OAuth 2.0 | `client_id`, `client_secret` (if static) |
| MCP_OAUTH2_GENERIC | Generic MCP | Optional |
| INSTALL_PLUGIN | Plugin install | `app_link` |
| NONE | No credentials | None |
| BASIC | Basic auth | None |
| API_KEY | API key auth | None |

### Webhook Secret

For providers supporting webhooks:
- Paste webhook secret from provider dashboard
- Used to verify webhook signatures
- Optional for most providers

Click **"Create Integration"** to save.

### SDK Equivalent

```typescript
import { LLMVault } from "@llmvault/sdk";

const vault = new LLMVault({ apiKey: "ak_live_..." });

const { data, error } = await vault.integrations.create({
  display_name: "Slack Production",
  provider: "slack",
  credentials: {
    client_id: "...",
    client_secret: "..."
  }
});
```

## Integration Detail Page

Click an integration to view details and manage connections.

### Stats Cards

| Card | Description |
|------|-------------|
| Total Connections | Number of user connections |
| Auth Mode | Authentication type used |

### Configuration Card

| Field | Description |
|-------|-------------|
| Callback URL | OAuth redirect URL |
| Webhook URL | Incoming webhook endpoint |
| Created | Timestamp |
| ID | Integration UUID |
| Unique Key | Internal identifier |

### Credentials Section

View and rotate credentials:

**View Mode:**
- Client ID (visible)
- Client Secret (masked, reveal toggle)
- Scopes
- App ID / App Link
- Private Key (masked)
- Webhook Secret

**Rotate Mode:**
1. Click **"Rotate"** button
2. Enter new credential values
3. Click **"Save Credentials"**
4. New credentials take effect immediately

**Edit Webhook Secret:**
- Click pencil icon (if user-defined)
- Enter new secret
- Save or cancel

### Connections Table

Lists authenticated users:

| Column | Description |
|--------|-------------|
| Connection ID | Connection identifier |
| Identity | Linked LLMVault identity |
| Created | Connection timestamp |
| Status | Always "Active" |

Connections are created when users complete OAuth flows.

### Empty State

When no connections exist:
- Guidance on how users connect
- Links to Connect UI documentation

## Managing Connections

Users create connections via:
- Connect UI embedded component
- Direct API calls
- SDK methods

Connections appear automatically in the dashboard.

### SDK Equivalent

```typescript
// List connections for an integration
const { data, error } = await vault.connections.list(integrationId, {
  limit: 20
});

// Retrieve an access token for a connection
const { data: token } = await vault.connections.retrieveToken(connectionId);
```

## Editing Display Name

1. Click the pencil icon next to the name
2. Enter new display name
3. Press Enter or click **Save**
4. Click **Cancel** or Escape to abort

## Deleting an Integration

**Warning:** Deleting an integration:
- Removes the integration permanently
- Invalidates all connections
- Cannot be undone

To delete:
1. Click **"Delete"** button
2. Confirm in dialog
3. Integration is permanently removed

## Provider List

View all available providers:

```typescript
import { LLMVault } from "@llmvault/sdk";

const vault = new LLMVault({ apiKey: "ak_live_..." });

const { data, error } = await vault.integrations.listProviders();
```

Returns provider name, display name, auth mode, and webhook support.

## Webhook Handling

LLMVault automatically:
- Receives webhooks at configured URLs
- Verifies signatures (if secret configured)
- Routes to appropriate integration
- Logs events to audit log

## Best Practices

1. **Use descriptive names** - Include environment (e.g., "Slack Production", "GitHub Staging")
2. **Rotate credentials regularly** - Security hygiene
3. **Configure webhooks** - Enable real-time updates
4. **Monitor connections** - Track active users
5. **Test in development** - Use sandbox providers first
