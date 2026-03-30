---
title: TypeScript SDK
description: Server-side TypeScript SDK for the LLMVault API
---

The official TypeScript SDK for LLMVault provides a type-safe way to interact with the LLMVault API from Node.js applications.

## Installation

```bash
npm install @llmvault/sdk
```

Requires Node.js 18 or later.

## Quick Start

```typescript
import { LLMVault } from "@llmvault/sdk";

const vault = new LLMVault({
  apiKey: "llmv_sk_...", // Your API key
  baseUrl: "https://api.llmvault.dev", // Optional, defaults to production
});

// Create an API key
const { data, error } = await vault.apiKeys.create({
  name: "my-api-key",
  scopes: ["credentials"],
});

if (error) {
  console.error("Error:", error);
} else {
  console.log("Created key:", data.key);
}
```

## Configuration

### `LLMVaultConfig`

| Property | Type | Required | Description |
|----------|------|----------|-------------|
| `apiKey` | `string` | Yes | Your LLMVault API key (starts with `llmv_sk_`) |
| `baseUrl` | `string` | No | API base URL. Defaults to `https://api.llmvault.dev` |

```typescript
interface LLMVaultConfig {
  apiKey: string;
  baseUrl?: string;
}
```

## Resources

The SDK is organized into resource namespaces that mirror the API structure:

```typescript
const vault = new LLMVault({ apiKey: "..." });

vault.apiKeys      // API key management
vault.credentials  // LLM credential storage
vault.tokens       // Proxy token minting
vault.identities   // Identity management
vault.connect      // Connect widget (sessions & settings)
vault.integrations // OAuth integrations
vault.connections  // Integration connections
vault.usage        // Usage statistics
vault.audit        // Audit log
vault.org          // Organization info
vault.providers    // LLM provider catalog
```

---

## API Keys

Manage API keys for your organization.

### `apiKeys.create(body)`

Create a new API key.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `body.name` | `string` | Yes | Descriptive name for the key |
| `body.scopes` | `string[]` | No | Array of permission scopes |
| `body.expires_in` | `string` | No | TTL like "24h", "7d". Omit for no expiration |

**Returns:** `{ data: CreateAPIKeyResponse, error: ErrorResponse }`

```typescript
const { data, error } = await vault.apiKeys.create({
  name: `sdk-test-${Date.now()}`,
  scopes: ["credentials"],
});

// Response data:
// {
//   id: "key_abc123",
//   key: "llmv_sk_...",      // Full key (shown once)
//   key_prefix: "llmv_sk_...", // First 16 chars
//   name: "my-key",
//   scopes: ["credentials"],
//   created_at: "2024-01-15T10:30:00Z",
//   expires_at: "2024-01-22T10:30:00Z"
// }
```

### `apiKeys.list(query?)`

List API keys with cursor pagination.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `query.limit` | `number` | No | Max items per page (1-100, default 50) |
| `query.cursor` | `string` | No | Pagination cursor from previous response |

**Returns:** `{ data: PaginatedApiKeys, error: ErrorResponse }`

```typescript
const { data, error } = await vault.apiKeys.list({ limit: 5 });

// Response data:
// {
//   data: [...],
//   has_more: true,
//   next_cursor: "abc123"
// }
```

### `apiKeys.delete(id)`

Revoke an API key by ID.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `id` | `string` | Yes | API key ID |

**Returns:** `{ data: Record<string, string>, error: ErrorResponse }`

```typescript
const { data, error } = await vault.apiKeys.delete("key_abc123");
```

---

## Credentials

Store and manage encrypted LLM API credentials.

### `credentials.create(body)`

Store a new credential.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `body.label` | `string` | Yes | Descriptive label |
| `body.api_key` | `string` | Yes | The API key to encrypt |
| `body.provider_id` | `string` | No | Provider ID (e.g., "openai", "anthropic"). Auto-detected from `base_url` if omitted. |
| `body.base_url` | `string` | No | API base URL |
| `body.auth_scheme` | `string` | No | Auth scheme (e.g., "bearer") |
| `body.identity_id` | `string` | No | Link to an identity |
| `body.external_id` | `string` | No | External reference ID |
| `body.meta` | `JSON` | No | Metadata object |
| `body.remaining` | `number` | No | Usage limit (credits) |
| `body.refill_amount` | `number` | No | Auto-refill amount |
| `body.refill_interval` | `string` | No | Refill interval (e.g., "1h") |

**Returns:** `{ data: CredentialResponse, error: ErrorResponse }`

```typescript
const { data, error } = await vault.credentials.create({
  label: "OpenAI Production",
  provider_id: "openai",
  base_url: "https://api.openai.com/v1",
  auth_scheme: "bearer",
  api_key: "sk-...",
});

// Response data:
// {
//   id: "cred_abc123",
//   label: "OpenAI Production",
//   provider_id: "openai",
//   auth_scheme: "bearer",
//   base_url: "https://api.openai.com/v1",
//   created_at: "2024-01-15T10:30:00Z",
//   request_count: 0,
//   identity_id: null,
//   meta: null,
//   remaining: null,
//   refill_amount: null,
//   refill_interval: null,
//   last_used_at: null,
//   revoked_at: null
// }
```

### `credentials.list(query?)`

List credentials with filtering and pagination.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `query.limit` | `number` | No | Page size (default 50) |
| `query.cursor` | `string` | No | Pagination cursor |
| `query.identity_id` | `string` | No | Filter by identity ID |
| `query.external_id` | `string` | No | Filter by external ID |
| `query.meta` | `string` | No | Filter by metadata (JSON) |

**Returns:** `{ data: PaginatedCredentials, error: ErrorResponse }`

```typescript
const { data, error } = await vault.credentials.list({ limit: 5 });
```

### `credentials.get(id)`

Get a single credential by ID.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `id` | `string` | Yes | Credential ID |

**Returns:** `{ data: CredentialResponse, error: ErrorResponse }`

```typescript
const { data, error } = await vault.credentials.get("cred_abc123");
```

### `credentials.delete(id)`

Revoke a credential.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `id` | `string` | Yes | Credential ID |

**Returns:** `{ data: CredentialResponse, error: ErrorResponse }`

```typescript
const { data, error } = await vault.credentials.delete("cred_abc123");
```

---

## Tokens

Mint short-lived proxy tokens for LLM API access.

### `tokens.create(body)`

Mint a new proxy token.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `body.credential_id` | `string` | Yes | Credential to scope the token to |
| `body.ttl` | `string` | No | Token lifetime (e.g., "1h", "24h") |
| `body.scopes` | `TokenScope[]` | No | Array of permission scopes |
| `body.remaining` | `number` | No | Usage limit |
| `body.refill_amount` | `number` | No | Auto-refill amount |
| `body.refill_interval` | `string` | No | Refill interval |
| `body.meta` | `JSON` | No | Metadata |

**Returns:** `{ data: MintTokenResponse, error: ErrorResponse }`

```typescript
const { data, error } = await vault.tokens.create({
  credential_id: "cred_abc123",
  ttl: "1h",
});

// Response data:
// {
//   token: "ptok_...",
//   jti: "jti_abc123",
//   expires_at: "2024-01-15T11:30:00Z",
//   mcp_endpoint: "https://api.llmvault.dev/v1/mcp"
// }
```

### `tokens.list(query?)`

List tokens with pagination.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `query.limit` | `number` | No | Page size |
| `query.cursor` | `string` | No | Pagination cursor |
| `query.credential_id` | `string` | No | Filter by credential |

**Returns:** `{ data: PaginatedTokens, error: ErrorResponse }`

```typescript
const { data, error } = await vault.tokens.list({ credential_id: "cred_abc123" });
```

### `tokens.delete(jti)`

Revoke a token by its JTI.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `jti` | `string` | Yes | Token JTI (JWT ID) |

**Returns:** `{ data: Record<string, string>, error: ErrorResponse }`

```typescript
const { data, error } = await vault.tokens.delete("jti_abc123");
```

---

## Identities

Manage user identities for request tracking and rate limiting.

### `identities.create(body)`

Create a new identity.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `body.external_id` | `string` | No | External reference ID |
| `body.meta` | `JSON` | No | Metadata object |
| `body.ratelimits` | `IdentityRateLimitParams[]` | No | Rate limit configurations |

**Returns:** `{ data: IdentityResponse, error: ErrorResponse }`

```typescript
const { data, error } = await vault.identities.create({
  external_id: "user_123",
  meta: { source: "web-app" },
  ratelimits: [{ name: "default", limit: 100, duration: 60000 }],
});

// Response data:
// {
//   id: "id_abc123",
//   external_id: "user_123",
//   meta: { source: "web-app" },
//   ratelimits: [{ name: "default", limit: 100, duration: 60000 }],
//   created_at: "2024-01-15T10:30:00Z",
//   updated_at: "2024-01-15T10:30:00Z",
//   request_count: 0,
//   last_used_at: null
// }
```

### `identities.list(query?)`

List identities with pagination.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `query.limit` | `number` | No | Page size |
| `query.cursor` | `string` | No | Pagination cursor |
| `query.external_id` | `string` | No | Filter by external ID |
| `query.meta` | `string` | No | Filter by metadata |

**Returns:** `{ data: PaginatedIdentities, error: ErrorResponse }`

```typescript
const { data, error } = await vault.identities.list({ limit: 5 });
```

### `identities.get(id)`

Get a single identity.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `id` | `string` | Yes | Identity ID |

**Returns:** `{ data: IdentityResponse, error: ErrorResponse }`

```typescript
const { data, error } = await vault.identities.get("id_abc123");
```

### `identities.update(id, body)`

Update an identity.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `id` | `string` | Yes | Identity ID |
| `body.external_id` | `string` | No | New external ID |
| `body.meta` | `JSON` | No | New metadata |
| `body.ratelimits` | `IdentityRateLimitParams[]` | No | New rate limits |

**Returns:** `{ data: IdentityResponse, error: ErrorResponse }`

```typescript
const { data, error } = await vault.identities.update("id_abc123", {
  meta: { source: "web-app", updated: true },
});
```

### `identities.delete(id)`

Delete an identity.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `id` | `string` | Yes | Identity ID |

**Returns:** `{ data: Record<string, string>, error: ErrorResponse }`

```typescript
const { data, error } = await vault.identities.delete("id_abc123");
```

---

## Connect

Manage the Connect widget for end-user credential collection.

### `connect.sessions.create(body)`

Create a short-lived session for the Connect widget.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `body.identity_id` | `string` | No* | Identity ID (required if no external_id) |
| `body.external_id` | `string` | No* | External user ID (required if no identity_id) |
| `body.permissions` | `string[]` | No | Permissions like ["create", "list"] |
| `body.ttl` | `string` | No | Session lifetime (e.g., "5m") |
| `body.allowed_integrations` | `string[]` | No | Restrict allowed integrations |
| `body.allowed_origins` | `string[]` | No | Allowed widget origins |
| `body.metadata` | `JSON` | No | Session metadata |

**Returns:** `{ data: ConnectSessionResponse, error: ErrorResponse }`

```typescript
const { data, error } = await vault.connect.sessions.create({
  external_id: `user-${Date.now()}`,
  permissions: ["create", "list"],
  ttl: "5m",
});

// Response data:
// {
//   id: "sess_abc123",
//   session_token: "csess_...",
//   identity_id: "id_xyz789",
//   external_id: "user-123",
//   expires_at: "2024-01-15T10:35:00Z",
//   created_at: "2024-01-15T10:30:00Z",
//   allowed_integrations: [],
//   allowed_origins: []
// }
```

### `connect.settings.get()`

Get Connect widget settings.

**Returns:** `{ data: ConnectSettingsResponse, error: ErrorResponse }`

```typescript
const { data, error } = await vault.connect.settings.get();

// Response data:
// {
//   allowed_origins: ["https://app.example.com"]
// }
```

### `connect.settings.update(body)`

Update Connect widget settings.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `body.allowed_origins` | `string[]` | No | Allowed origins for the widget |

**Returns:** `{ data: ConnectSettingsResponse, error: ErrorResponse }`

```typescript
const { data, error } = await vault.connect.settings.update({
  allowed_origins: ["https://app.example.com", "https://app2.example.com"],
});
```

---

## Integrations

Manage OAuth integrations with third-party providers.

### `integrations.create(body)`

Create a new integration.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `body.provider` | `string` | Yes | Provider name (e.g., "slack", "github") |
| `body.display_name` | `string` | No | Custom display name |
| `body.credentials` | `NangoCredentials` | No | OAuth credentials |
| `body.meta` | `JSON` | No | Metadata |

**Returns:** `{ data: IntegrationResponse, error: ErrorResponse }`

```typescript
const { data, error } = await vault.integrations.create({
  provider: "slack",
  display_name: "Slack Workspace",
});

// Response data:
// {
//   id: "int_abc123",
//   provider: "slack",
//   display_name: "Slack Workspace",
//   unique_key: "slack",
//   created_at: "2024-01-15T10:30:00Z",
//   updated_at: "2024-01-15T10:30:00Z",
//   meta: null,
//   nango_config: null
// }
```

### `integrations.list(query?)`

List integrations.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `query.limit` | `number` | No | Page size |
| `query.cursor` | `string` | No | Pagination cursor |
| `query.provider` | `string` | No | Filter by provider |
| `query.meta` | `string` | No | Filter by metadata |

**Returns:** `{ data: PaginatedIntegrations, error: ErrorResponse }`

```typescript
const { data, error } = await vault.integrations.list({ limit: 10 });
```

### `integrations.get(id)`

Get a single integration.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `id` | `string` | Yes | Integration ID |

**Returns:** `{ data: IntegrationResponse, error: ErrorResponse }`

```typescript
const { data, error } = await vault.integrations.get("int_abc123");
```

### `integrations.update(id, body)`

Update an integration.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `id` | `string` | Yes | Integration ID |
| `body.display_name` | `string` | No | New display name |
| `body.credentials` | `NangoCredentials` | No | New credentials |
| `body.meta` | `JSON` | No | New metadata |

**Returns:** `{ data: IntegrationResponse, error: ErrorResponse }`

```typescript
const { data, error } = await vault.integrations.update("int_abc123", {
  display_name: "Updated Name",
});
```

### `integrations.delete(id)`

Delete an integration.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `id` | `string` | Yes | Integration ID |

**Returns:** `{ data: Record<string, string>, error: ErrorResponse }`

```typescript
const { data, error } = await vault.integrations.delete("int_abc123");
```

### `integrations.listProviders()`

List available integration providers.

**Returns:** `{ data: IntegrationProviderInfo[], error: ErrorResponse }`

```typescript
const { data, error } = await vault.integrations.listProviders();

// Response data:
// [
//   {
//     name: "slack",
//     display_name: "Slack",
//     auth_mode: "OAUTH2",
//     webhook_user_defined_secret: false
//   }
// ]
```

---

## Connections

Manage connections to OAuth integrations.

### `connections.availableScopes()`

List available scopes for all active connections.

**Returns:** `Promise<AvailableScopeConnection[]>`

```typescript
const scopes = await vault.connections.availableScopes();

// Returns:
// [
//   {
//     connection_id: "conn_abc",
//     integration_id: "int_xyz",
//     provider: "slack",
//     display_name: "Slack",
//     actions: [...],
//     resources: {...}
//   }
// ]
```

### `connections.create(integrationId, body)`

Create a connection for an integration.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `integrationId` | `string` | Yes | Integration ID |
| `body.identity_id` | `string` | No | Identity ID |
| `body.meta` | `JSON` | No | Metadata |
| `body.nango_connection_id` | `string` | Yes | Nango connection ID |

**Returns:** `{ data: IntegConnResponse, error: ErrorResponse }`

```typescript
const { data, error } = await vault.connections.create("int_abc123", {
  nango_connection_id: "nango_conn_xyz",
});
```

### `connections.list(integrationId, query?)`

List connections for an integration.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `integrationId` | `string` | Yes | Integration ID |
| `query.limit` | `number` | No | Page size |
| `query.cursor` | `string` | No | Pagination cursor |

**Returns:** `{ data: PaginatedIntegConns, error: ErrorResponse }`

```typescript
const { data, error } = await vault.connections.list("int_abc123");
```

### `connections.get(id)`

Get a connection.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `id` | `string` | Yes | Connection ID |

**Returns:** `{ data: IntegConnResponse, error: ErrorResponse }`

```typescript
const { data, error } = await vault.connections.get("conn_abc123");
```

### `connections.retrieveToken(id)`

Retrieve the OAuth access token for a connection.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `id` | `string` | Yes | Connection ID |

**Returns:** `{ data: TokenResponse, error: ErrorResponse }`

```typescript
const { data, error } = await vault.connections.retrieveToken("conn_abc123");

// Response data:
// {
//   access_token: "xoxb-...",
//   token_type: "Bearer",
//   expires_at: "2024-01-15T11:30:00Z",
//   provider: "slack",
//   connection_id: "conn_abc123"
// }
```

### `connections.delete(id)`

Revoke a connection.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `id` | `string` | Yes | Connection ID |

**Returns:** `{ data: Record<string, string>, error: ErrorResponse }`

```typescript
const { data, error } = await vault.connections.delete("conn_abc123");
```

---

## Usage

Get organization usage statistics.

### `usage.get()`

Get aggregated usage stats.

**Returns:** `{ data: UsageResponse, error: ErrorResponse }`

```typescript
const { data, error } = await vault.usage.get();

// Response data:
// {
//   credentials: { total: 10, active: 8, revoked: 2 },
//   tokens: { total: 100, active: 80, expired: 15, revoked: 5 },
//   api_keys: { total: 5, active: 5, revoked: 0 },
//   identities: { total: 50 },
//   requests: {
//     today: 1000,
//     yesterday: 850,
//     last_7d: 6000,
//     last_30d: 25000,
//     total: 100000
//   },
//   daily_requests: [...],
//   top_credentials: [...]
// }
```

---

## Audit

Access the audit log.

### `audit.list(query?)`

List audit log entries.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `query.limit` | `number` | No | Page size |
| `query.cursor` | `string` | No | Pagination cursor |
| `query.action` | `string` | No | Filter by action (e.g., "proxy.request") |

**Returns:** `{ data: PaginatedAuditEntries, error: ErrorResponse }`

```typescript
const { data, error } = await vault.audit.list({ limit: 5 });

// Response data:
// {
//   data: [
//     {
//       id: 123,
//       action: "proxy.request",
//       method: "POST",
//       path: "/v1/chat/completions",
//       status: 200,
//       credential_id: "cred_abc",
//       identity_id: "id_xyz",
//       latency_ms: 150,
//       ip_address: "192.168.1.1",
//       created_at: "2024-01-15T10:30:00Z"
//     }
//   ],
//   has_more: true,
//   next_cursor: "456"
// }
```

---

## Organization

### `org.getCurrent()`

Get current organization details.

**Returns:** `{ data: OrgResponse, error: ErrorResponse }`

```typescript
const { data, error } = await vault.org.getCurrent();

// Response data:
// {
//   id: "org_abc123",
//   name: "My Organization",
//   active: true,
//   rate_limit: 1000,
//   created_at: "2024-01-01T00:00:00Z"
// }
```

---

## Providers

Browse the LLM provider catalog.

### `providers.list()`

List all available LLM providers.

**Returns:** `{ data: ProviderSummary[], error: ErrorResponse }`

```typescript
const { data, error } = await vault.providers.list();

// Response data:
// [
//   {
//     id: "openai",
//     name: "OpenAI",
//     api: "https://api.openai.com",
//     doc: "https://platform.openai.com/docs",
//     model_count: 20
//   }
// ]
```

### `providers.get(id)`

Get provider details with all models.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `id` | `string` | Yes | Provider ID |

**Returns:** `{ data: ProviderDetail, error: ErrorResponse }`

```typescript
const { data, error } = await vault.providers.get("openai");

// Response data:
// {
//   id: "openai",
//   name: "OpenAI",
//   api: "https://api.openai.com",
//   doc: "https://platform.openai.com/docs",
//   models: [...]
// }
```

### `providers.listModels(id)`

List models for a provider.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `id` | `string` | Yes | Provider ID |

**Returns:** `{ data: ModelSummary[], error: ErrorResponse }`

```typescript
const { data, error } = await vault.providers.listModels("openai");

// Response data:
// [
//   {
//     id: "gpt-4",
//     name: "GPT-4",
//     family: "GPT-4",
//     cost: { input: 0.03, output: 0.06 },
//     tool_call: true,
//     structured_output: true
//   }
// ]
```

---

## Type Definitions

### Core Types

```typescript
interface LLMVaultConfig {
  apiKey: string;
  baseUrl?: string;
}

// API Key Types
type ApiKeyResponse = components["schemas"]["apiKeyResponse"];
type CreateAPIKeyRequest = components["schemas"]["createAPIKeyRequest"];
type CreateAPIKeyResponse = components["schemas"]["createAPIKeyResponse"];
type PaginatedApiKeys = components["schemas"]["paginatedResponse-apiKeyResponse"];

// Credential Types
type CredentialResponse = components["schemas"]["credentialResponse"];
type CreateCredentialRequest = components["schemas"]["createCredentialRequest"];
type PaginatedCredentials = components["schemas"]["paginatedResponse-credentialResponse"];

// Token Types
type MintTokenRequest = components["schemas"]["mintTokenRequest"];
type MintTokenResponse = components["schemas"]["mintTokenResponse"];
type TokenListItem = components["schemas"]["tokenListItem"];
type PaginatedTokens = components["schemas"]["paginatedResponse-tokenListItem"];
type TokenResponse = components["schemas"]["tokenResponse"];
type TokenScope = components["schemas"]["github_com_llmvault_llmvault_internal_mcp.TokenScope"];

// Identity Types
type IdentityResponse = components["schemas"]["identityResponse"];
type CreateIdentityRequest = components["schemas"]["createIdentityRequest"];
type UpdateIdentityRequest = components["schemas"]["updateIdentityRequest"];
type IdentityRateLimitParams = components["schemas"]["identityRateLimitParams"];
type PaginatedIdentities = components["schemas"]["paginatedResponse-identityResponse"];

// Connect Types
type ConnectSessionResponse = components["schemas"]["connectSessionResponse"];
type CreateConnectSessionRequest = components["schemas"]["createConnectSessionRequest"];
type ConnectSettingsRequest = components["schemas"]["connectSettingsRequest"];
type ConnectSettingsResponse = components["schemas"]["connectSettingsResponse"];

// Integration Types
type IntegrationResponse = components["schemas"]["integrationResponse"];
type CreateIntegrationRequest = components["schemas"]["createIntegrationRequest"];
type UpdateIntegrationRequest = components["schemas"]["updateIntegrationRequest"];
type NangoCredentials = components["schemas"]["github_com_llmvault_llmvault_internal_nango.Credentials"];
type IntegrationProviderInfo = components["schemas"]["integrationProviderInfo"];
type PaginatedIntegrations = components["schemas"]["paginatedResponse-integrationResponse"];

// Connection Types
type IntegConnResponse = components["schemas"]["integConnResponse"];
type IntegConnCreateRequest = components["schemas"]["integConnCreateRequest"];
type PaginatedIntegConns = components["schemas"]["paginatedResponse-integConnResponse"];

// Scope Types
interface AvailableScopeAction {
  key: string;
  display_name: string;
  description: string;
  resource_type?: string;
}

interface AvailableScopeResourceItem {
  id: string;
  name: string;
}

interface AvailableScopeResource {
  display_name: string;
  selected: AvailableScopeResourceItem[];
}

interface AvailableScopeConnection {
  connection_id: string;
  integration_id: string;
  provider: string;
  display_name: string;
  actions: AvailableScopeAction[];
  resources?: Record<string, AvailableScopeResource>;
}

// Usage & Audit Types
type UsageResponse = components["schemas"]["usageResponse"];
type AuditEntryResponse = components["schemas"]["auditEntryResponse"];
type PaginatedAuditEntries = components["schemas"]["paginatedResponse-auditEntryResponse"];

// Organization Types
type OrgResponse = components["schemas"]["orgResponse"];

// Provider Types
type ProviderSummary = components["schemas"]["providerSummary"];
type ProviderDetail = components["schemas"]["providerDetail"];
type ModelSummary = components["schemas"]["modelSummary"];

// Error Type
type ErrorResponse = components["schemas"]["errorResponse"];
type JSON = components["schemas"]["JSON"];
```

---

## Error Handling

All SDK methods return an object with `data` and `error` properties following the openapi-fetch pattern:

```typescript
const { data, error } = await vault.apiKeys.create({ name: "test" });

if (error) {
  // error.error contains the error message
  console.error("API Error:", error.error);
  return;
}

// data is guaranteed to be defined here
console.log("Success:", data);
```

Common error responses:

- `400` - Bad Request (invalid parameters)
- `401` - Unauthorized (invalid API key)
- `404` - Not Found (resource doesn't exist)
- `409` - Conflict (duplicate external_id, etc.)
- `500` - Internal Server Error
- `502` - Bad Gateway (upstream provider error)

## License

MIT License - see LICENSE file for details.
