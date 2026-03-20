---
title: Python SDK
description: Official Python SDK for the LLMVault API
---

The official Python SDK for LLMVault provides a simple, Pythonic way to interact with the LLMVault API.

## Installation

```bash
pip install llmvault
```

Requires Python 3.8 or later.

## Quick Start

```python
from llmvault import LLMVault

vault = LLMVault(
    api_key="llmv_sk_...",  # Your API key
    base_url="https://api.llmvault.dev"  # Optional, defaults to production
)

# Create an API key
result = vault.api_keys.create({
    "name": "my-api-key"
})

if result.error:
    print(f"Error: {result.error}")
else:
    print(f"Created key: {result.data['key']}")
```

## Configuration

### `LLMVault`

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `api_key` | `str` | Yes | Your LLMVault API key (starts with `llmv_sk_`) |
| `base_url` | `str` | No | API base URL. Defaults to `https://api.llmvault.dev` |

```python
from llmvault import LLMVault

vault = LLMVault(
    api_key="llmv_sk_...",
    base_url="https://api.llmvault.dev"  # Optional
)
```

## Resources

The SDK is organized into resource namespaces that mirror the API structure:

```python
vault = LLMVault(api_key="...")

vault.api_keys       # API key management
vault.credentials    # LLM credential storage
vault.tokens         # Proxy token minting
vault.identities     # Identity management
vault.connect        # Connect widget (sessions & settings)
vault.integrations   # OAuth integrations
vault.connections    # Integration connections
vault.usage          # Usage statistics
vault.audit          # Audit log
vault.org            # Organization info
vault.providers      # LLM provider catalog
```

---

## API Keys

Manage API keys for your organization.

### `api_keys.create(body)`

Create a new API key.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `body["name"]` | `str` | Yes | Descriptive name for the key |
| `body["scopes"]` | `list[str]` | No | List of permission scopes |
| `body["expires_in"]` | `str` | No | TTL like "24h", "7d". Omit for no expiration |

**Returns:** `ApiResult[CreateAPIKeyResponse]`

```python
result = vault.api_keys.create({
    "name": f"sdk-test-{int(time.time())}",
    "scopes": ["credentials"]
})

if result.data:
    print(f"ID: {result.data['id']}")
    print(f"Key: {result.data['key']}")  # Full key (shown once)
    print(f"Prefix: {result.data['key_prefix']}")

# Response data structure:
# {
#     "id": "key_abc123",
#     "key": "llmv_sk_...",       # Full key (shown once)
#     "key_prefix": "llmv_sk_...", # First 16 chars
#     "name": "my-key",
#     "scopes": ["credentials"],
#     "created_at": "2024-01-15T10:30:00Z",
#     "expires_at": "2024-01-22T10:30:00Z"
# }
```

### `api_keys.list(query=None)`

List API keys with cursor pagination.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `query["limit"]` | `int` | No | Max items per page (1-100, default 50) |
| `query["cursor"]` | `str` | No | Pagination cursor from previous response |

**Returns:** `ApiResult[PaginatedApiKeys]`

```python
result = vault.api_keys.list({"limit": 5})

if result.data:
    for key in result.data["data"]:
        print(f"Key: {key['name']}")
    print(f"Has more: {result.data['has_more']}")
    print(f"Next cursor: {result.data.get('next_cursor')}")
```

### `api_keys.delete(id)`

Revoke an API key by ID.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `id` | `str` | Yes | API key ID |

**Returns:** `ApiResult[dict]`

```python
result = vault.api_keys.delete("key_abc123")
```

---

## Credentials

Store and manage encrypted LLM API credentials.

### `credentials.create(body)`

Store a new credential.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `body["label"]` | `str` | Yes | Descriptive label |
| `body["provider_id"]` | `str` | Yes | Provider ID (e.g., "openai", "anthropic") |
| `body["base_url"]` | `str` | Yes | API base URL |
| `body["auth_scheme"]` | `str` | Yes | Auth scheme (e.g., "bearer") |
| `body["api_key"]` | `str` | Yes | The API key to encrypt |
| `body["identity_id"]` | `str` | No | Link to an identity |
| `body["external_id"]` | `str` | No | External reference ID |
| `body["meta"]` | `dict` | No | Metadata object |
| `body["remaining"]` | `int` | No | Usage limit (credits) |
| `body["refill_amount"]` | `int` | No | Auto-refill amount |
| `body["refill_interval"]` | `str` | No | Refill interval (e.g., "1h") |

**Returns:** `ApiResult[CredentialResponse]`

```python
result = vault.credentials.create({
    "label": "OpenAI Production",
    "provider_id": "openai",
    "base_url": "https://api.openai.com/v1",
    "auth_scheme": "bearer",
    "api_key": "sk-..."
})

if result.data:
    print(f"Created: {result.data['id']}")
    print(f"Provider: {result.data['provider_id']}")

# Response data structure:
# {
#     "id": "cred_abc123",
#     "label": "OpenAI Production",
#     "provider_id": "openai",
#     "auth_scheme": "bearer",
#     "base_url": "https://api.openai.com/v1",
#     "created_at": "2024-01-15T10:30:00Z",
#     "request_count": 0,
#     "identity_id": None,
#     "meta": None,
#     "remaining": None,
#     "refill_amount": None,
#     "refill_interval": None,
#     "last_used_at": None,
#     "revoked_at": None
# }
```

### `credentials.list(query=None)`

List credentials with filtering and pagination.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `query["limit"]` | `int` | No | Page size (default 50) |
| `query["cursor"]` | `str` | No | Pagination cursor |
| `query["identity_id"]` | `str` | No | Filter by identity ID |
| `query["external_id"]` | `str` | No | Filter by external ID |
| `query["meta"]` | `str` | No | Filter by metadata (JSON string) |

**Returns:** `ApiResult[PaginatedCredentials]`

```python
result = vault.credentials.list({"limit": 5})

if result.data:
    for cred in result.data["data"]:
        print(f"Credential: {cred['label']} ({cred['provider_id']})")
```

### `credentials.get(id)`

Get a single credential by ID.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `id` | `str` | Yes | Credential ID |

**Returns:** `ApiResult[CredentialResponse]`

```python
result = vault.credentials.get("cred_abc123")

if result.data:
    print(f"Label: {result.data['label']}")
    print(f"Request count: {result.data['request_count']}")
```

### `credentials.delete(id)`

Revoke a credential.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `id` | `str` | Yes | Credential ID |

**Returns:** `ApiResult[CredentialResponse]`

```python
result = vault.credentials.delete("cred_abc123")
```

---

## Tokens

Mint short-lived proxy tokens for LLM API access.

### `tokens.create(body)`

Mint a new proxy token.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `body["credential_id"]` | `str` | Yes | Credential to scope the token to |
| `body["ttl"]` | `str` | No | Token lifetime (e.g., "1h", "24h") |
| `body["scopes"]` | `list[TokenScope]` | No | List of permission scopes |
| `body["remaining"]` | `int` | No | Usage limit |
| `body["refill_amount"]` | `int` | No | Auto-refill amount |
| `body["refill_interval"]` | `str` | No | Refill interval |
| `body["meta"]` | `dict` | No | Metadata |

**Returns:** `ApiResult[MintTokenResponse]`

```python
result = vault.tokens.create({
    "credential_id": "cred_abc123",
    "ttl": "1h"
})

if result.data:
    print(f"Token: {result.data['token']}")  # Starts with ptok_
    print(f"JTI: {result.data['jti']}")
    print(f"Expires: {result.data['expires_at']}")
    print(f"MCP Endpoint: {result.data['mcp_endpoint']}")

# Response data structure:
# {
#     "token": "ptok_...",
#     "jti": "jti_abc123",
#     "expires_at": "2024-01-15T11:30:00Z",
#     "mcp_endpoint": "https://api.llmvault.dev/v1/mcp"
# }
```

### `tokens.list(query=None)`

List tokens with pagination.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `query["limit"]` | `int` | No | Page size |
| `query["cursor"]` | `str` | No | Pagination cursor |
| `query["credential_id"]` | `str` | No | Filter by credential |

**Returns:** `ApiResult[PaginatedTokens]`

```python
result = vault.tokens.list({"credential_id": "cred_abc123"})

if result.data:
    for token in result.data["data"]:
        print(f"Token: {token['jti']} - Expires: {token['expires_at']}")
```

### `tokens.delete(jti)`

Revoke a token by its JTI.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `jti` | `str` | Yes | Token JTI (JWT ID) |

**Returns:** `ApiResult[dict]`

```python
result = vault.tokens.delete("jti_abc123")
```

---

## Identities

Manage user identities for request tracking and rate limiting.

### `identities.create(body)`

Create a new identity.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `body["external_id"]` | `str` | No | External reference ID |
| `body["meta"]` | `dict` | No | Metadata object |
| `body["ratelimits"]` | `list[IdentityRateLimitParams]` | No | Rate limit configurations |

**Returns:** `ApiResult[IdentityResponse]`

```python
result = vault.identities.create({
    "external_id": "user_123",
    "meta": {"source": "web-app"},
    "ratelimits": [
        {"name": "default", "limit": 100, "duration": 60000}
    ]
})

if result.data:
    print(f"ID: {result.data['id']}")
    print(f"External ID: {result.data['external_id']}")

# Response data structure:
# {
#     "id": "id_abc123",
#     "external_id": "user_123",
#     "meta": {"source": "web-app"},
#     "ratelimits": [{"name": "default", "limit": 100, "duration": 60000}],
#     "created_at": "2024-01-15T10:30:00Z",
#     "updated_at": "2024-01-15T10:30:00Z",
#     "request_count": 0,
#     "last_used_at": None
# }
```

### `identities.list(query=None)`

List identities with pagination.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `query["limit"]` | `int` | No | Page size |
| `query["cursor"]` | `str` | No | Pagination cursor |
| `query["external_id"]` | `str` | No | Filter by external ID |
| `query["meta"]` | `str` | No | Filter by metadata |

**Returns:** `ApiResult[PaginatedIdentities]`

```python
result = vault.identities.list({"limit": 5})
```

### `identities.get(id)`

Get a single identity.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `id` | `str` | Yes | Identity ID |

**Returns:** `ApiResult[IdentityResponse]`

```python
result = vault.identities.get("id_abc123")

if result.data:
    print(f"Identity: {result.data['external_id']}")
    print(f"Request count: {result.data['request_count']}")
```

### `identities.update(id, body)`

Update an identity.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `id` | `str` | Yes | Identity ID |
| `body["external_id"]` | `str` | No | New external ID |
| `body["meta"]` | `dict` | No | New metadata |
| `body["ratelimits"]` | `list[IdentityRateLimitParams]` | No | New rate limits |

**Returns:** `ApiResult[IdentityResponse]`

```python
result = vault.identities.update("id_abc123", {
    "meta": {"source": "web-app", "updated": True}
})
```

### `identities.delete(id)`

Delete an identity.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `id` | `str` | Yes | Identity ID |

**Returns:** `ApiResult[dict]`

```python
result = vault.identities.delete("id_abc123")
```

---

## Connect

Manage the Connect widget for end-user credential collection.

### `connect.sessions.create(body)`

Create a short-lived session for the Connect widget.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `body["identity_id"]` | `str` | No* | Identity ID (required if no external_id) |
| `body["external_id"]` | `str` | No* | External user ID (required if no identity_id) |
| `body["permissions"]` | `list[str]` | No | Permissions like ["create", "list"] |
| `body["ttl"]` | `str` | No | Session lifetime (e.g., "5m") |
| `body["allowed_integrations"]` | `list[str]` | No | Restrict allowed integrations |
| `body["allowed_origins"]` | `list[str]` | No | Allowed widget origins |
| `body["metadata"]` | `dict` | No | Session metadata |

**Returns:** `ApiResult[ConnectSessionResponse]`

```python
result = vault.connect.sessions.create({
    "external_id": f"user-{int(time.time())}",
    "permissions": ["create", "list"],
    "ttl": "5m"
})

if result.data:
    print(f"Session Token: {result.data['session_token']}")  # Starts with csess_
    print(f"Expires: {result.data['expires_at']}")

# Response data structure:
# {
#     "id": "sess_abc123",
#     "session_token": "csess_...",
#     "identity_id": "id_xyz789",
#     "external_id": "user-123",
#     "expires_at": "2024-01-15T10:35:00Z",
#     "created_at": "2024-01-15T10:30:00Z",
#     "allowed_integrations": [],
#     "allowed_origins": []
# }
```

### `connect.settings.get()`

Get Connect widget settings.

**Returns:** `ApiResult[ConnectSettingsResponse]`

```python
result = vault.connect.settings.get()

if result.data:
    print(f"Allowed origins: {result.data['allowed_origins']}")

# Response data structure:
# {
#     "allowed_origins": ["https://app.example.com"]
# }
```

### `connect.settings.update(body)`

Update Connect widget settings.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `body["allowed_origins"]` | `list[str]` | No | Allowed origins for the widget |

**Returns:** `ApiResult[ConnectSettingsResponse]`

```python
result = vault.connect.settings.update({
    "allowed_origins": ["https://app.example.com", "https://app2.example.com"]
})
```

---

## Integrations

Manage OAuth integrations with third-party providers.

### `integrations.create(body)`

Create a new integration.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `body["provider"]` | `str` | Yes | Provider name (e.g., "slack", "github") |
| `body["display_name"]` | `str` | No | Custom display name |
| `body["credentials"]` | `NangoCredentials` | No | OAuth credentials |
| `body["meta"]` | `dict` | No | Metadata |

**Returns:** `ApiResult[IntegrationResponse]`

```python
result = vault.integrations.create({
    "provider": "slack",
    "display_name": "Slack Workspace"
})

if result.data:
    print(f"Integration: {result.data['id']}")

# Response data structure:
# {
#     "id": "int_abc123",
#     "provider": "slack",
#     "display_name": "Slack Workspace",
#     "unique_key": "slack",
#     "created_at": "2024-01-15T10:30:00Z",
#     "updated_at": "2024-01-15T10:30:00Z",
#     "meta": None,
#     "nango_config": None
# }
```

### `integrations.list(query=None)`

List integrations.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `query["limit"]` | `int` | No | Page size |
| `query["cursor"]` | `str` | No | Pagination cursor |
| `query["provider"]` | `str` | No | Filter by provider |
| `query["meta"]` | `str` | No | Filter by metadata |

**Returns:** `ApiResult[PaginatedIntegrations]`

```python
result = vault.integrations.list({"limit": 10})
```

### `integrations.get(id)`

Get a single integration.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `id` | `str` | Yes | Integration ID |

**Returns:** `ApiResult[IntegrationResponse]`

```python
result = vault.integrations.get("int_abc123")
```

### `integrations.update(id, body)`

Update an integration.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `id` | `str` | Yes | Integration ID |
| `body["display_name"]` | `str` | No | New display name |
| `body["credentials"]` | `NangoCredentials` | No | New credentials |
| `body["meta"]` | `dict` | No | New metadata |

**Returns:** `ApiResult[IntegrationResponse]`

```python
result = vault.integrations.update("int_abc123", {
    "display_name": "Updated Name"
})
```

### `integrations.delete(id)`

Delete an integration.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `id` | `str` | Yes | Integration ID |

**Returns:** `ApiResult[dict]`

```python
result = vault.integrations.delete("int_abc123")
```

### `integrations.list_providers()`

List available integration providers.

**Returns:** `ApiResult[list[IntegrationProviderInfo]]`

```python
result = vault.integrations.list_providers()

if result.data:
    for provider in result.data:
        print(f"{provider['name']}: {provider['display_name']}")

# Response data structure:
# [
#     {
#         "name": "slack",
#         "display_name": "Slack",
#         "auth_mode": "OAUTH2",
#         "webhook_user_defined_secret": False
#     }
# ]
```

---

## Connections

Manage connections to OAuth integrations.

### `connections.available_scopes()`

List available scopes for all active connections.

**Returns:** `list[AvailableScopeConnection]`

```python
scopes = vault.connections.available_scopes()

for conn in scopes:
    print(f"{conn['display_name']} ({conn['provider']})")
    for action in conn['actions']:
        print(f"  - {action['display_name']}")

# Returns:
# [
#     {
#         "connection_id": "conn_abc",
#         "integration_id": "int_xyz",
#         "provider": "slack",
#         "display_name": "Slack",
#         "actions": [...],
#         "resources": {...}
#     }
# ]
```

### `connections.create(integration_id, body)`

Create a connection for an integration.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `integration_id` | `str` | Yes | Integration ID |
| `body["identity_id"]` | `str` | No | Identity ID |
| `body["meta"]` | `dict` | No | Metadata |
| `body["nango_connection_id"]` | `str` | Yes | Nango connection ID |

**Returns:** `ApiResult[IntegConnResponse]`

```python
result = vault.connections.create("int_abc123", {
    "nango_connection_id": "nango_conn_xyz"
})
```

### `connections.list(integration_id, query=None)`

List connections for an integration.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `integration_id` | `str` | Yes | Integration ID |
| `query["limit"]` | `int` | No | Page size |
| `query["cursor"]` | `str` | No | Pagination cursor |

**Returns:** `ApiResult[PaginatedIntegConns]`

```python
result = vault.connections.list("int_abc123")
```

### `connections.get(id)`

Get a connection.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `id` | `str` | Yes | Connection ID |

**Returns:** `ApiResult[IntegConnResponse]`

```python
result = vault.connections.get("conn_abc123")
```

### `connections.retrieve_token(id)`

Retrieve the OAuth access token for a connection.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `id` | `str` | Yes | Connection ID |

**Returns:** `ApiResult[TokenResponse]`

```python
result = vault.connections.retrieve_token("conn_abc123")

if result.data:
    print(f"Access token: {result.data['access_token']}")
    print(f"Expires: {result.data['expires_at']}")

# Response data structure:
# {
#     "access_token": "xoxb-...",
#     "token_type": "Bearer",
#     "expires_at": "2024-01-15T11:30:00Z",
#     "provider": "slack",
#     "connection_id": "conn_abc123"
# }
```

### `connections.delete(id)`

Revoke a connection.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `id` | `str` | Yes | Connection ID |

**Returns:** `ApiResult[dict]`

```python
result = vault.connections.delete("conn_abc123")
```

---

## Usage

Get organization usage statistics.

### `usage.get()`

Get aggregated usage stats.

**Returns:** `ApiResult[UsageResponse]`

```python
result = vault.usage.get()

if result.data:
    print(f"Credentials: {result.data['credentials']}")
    print(f"Tokens: {result.data['tokens']}")
    print(f"Requests today: {result.data['requests']['today']}")

# Response data structure:
# {
#     "credentials": {"total": 10, "active": 8, "revoked": 2},
#     "tokens": {"total": 100, "active": 80, "expired": 15, "revoked": 5},
#     "api_keys": {"total": 5, "active": 5, "revoked": 0},
#     "identities": {"total": 50},
#     "requests": {
#         "today": 1000,
#         "yesterday": 850,
#         "last_7d": 6000,
#         "last_30d": 25000,
#         "total": 100000
#     },
#     "daily_requests": [...],
#     "top_credentials": [...]
# }
```

---

## Audit

Access the audit log.

### `audit.list(query=None)`

List audit log entries.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `query["limit"]` | `int` | No | Page size |
| `query["cursor"]` | `str` | No | Pagination cursor |
| `query["action"]` | `str` | No | Filter by action (e.g., "proxy.request") |

**Returns:** `ApiResult[PaginatedAuditEntries]`

```python
result = vault.audit.list({"limit": 5})

if result.data:
    for entry in result.data["data"]:
        print(f"{entry['action']}: {entry['method']} {entry['path']} ({entry['status']})")

# Response data structure:
# {
#     "data": [
#         {
#             "id": 123,
#             "action": "proxy.request",
#             "method": "POST",
#             "path": "/v1/chat/completions",
#             "status": 200,
#             "credential_id": "cred_abc",
#             "identity_id": "id_xyz",
#             "latency_ms": 150,
#             "ip_address": "192.168.1.1",
#             "created_at": "2024-01-15T10:30:00Z"
#         }
#     ],
#     "has_more": True,
#     "next_cursor": "456"
# }
```

---

## Organization

### `org.get_current()`

Get current organization details.

**Returns:** `ApiResult[OrgResponse]`

```python
result = vault.org.get_current()

if result.data:
    print(f"Org: {result.data['name']}")
    print(f"Rate limit: {result.data['rate_limit']}")

# Response data structure:
# {
#     "id": "org_abc123",
#     "name": "My Organization",
#     "logto_org_id": "logto_xyz",
#     "active": True,
#     "rate_limit": 1000,
#     "created_at": "2024-01-01T00:00:00Z"
# }
```

---

## Providers

Browse the LLM provider catalog.

### `providers.list()`

List all available LLM providers.

**Returns:** `ApiResult[list[ProviderSummary]]`

```python
result = vault.providers.list()

if result.data:
    for provider in result.data:
        print(f"{provider['name']}: {provider['model_count']} models")

# Response data structure:
# [
#     {
#         "id": "openai",
#         "name": "OpenAI",
#         "api": "https://api.openai.com",
#         "doc": "https://platform.openai.com/docs",
#         "model_count": 20
#     }
# ]
```

### `providers.get(id)`

Get provider details with all models.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `id` | `str` | Yes | Provider ID |

**Returns:** `ApiResult[ProviderDetail]`

```python
result = vault.providers.get("openai")

if result.data:
    print(f"Provider: {result.data['name']}")
    print(f"Models: {len(result.data['models'])}")

# Response data structure:
# {
#     "id": "openai",
#     "name": "OpenAI",
#     "api": "https://api.openai.com",
#     "doc": "https://platform.openai.com/docs",
#     "models": [...]
# }
```

### `providers.list_models(id)`

List models for a provider.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `id` | `str` | Yes | Provider ID |

**Returns:** `ApiResult[list[ModelSummary]]`

```python
result = vault.providers.list_models("openai")

if result.data:
    for model in result.data:
        print(f"{model['id']}: tool_call={model['tool_call']}")

# Response data structure:
# [
#     {
#         "id": "gpt-4",
#         "name": "GPT-4",
#         "family": "GPT-4",
#         "cost": {"input": 0.03, "output": 0.06},
#         "tool_call": True,
#         "structured_output": True
#     }
# ]
```

---

## Response Types

All SDK methods return an `ApiResult` object:

```python
from dataclasses import dataclass
from typing import Generic, TypeVar, Optional

T = TypeVar('T')

@dataclass
class ApiResult(Generic[T]):
    data: Optional[T]      # Response data (None if error)
    error: Optional[dict]  # Error dict with "error" key (None if success)
    status_code: int       # HTTP status code
```

### Error Handling

```python
result = vault.api_keys.create({"name": "test"})

if result.error:
    # result.error is {"error": "error message"}
    print(f"Error: {result.error['error']}")
    print(f"Status: {result.status_code}")
else:
    # result.data is the response payload
    print(f"Success: {result.data}")
```

Common HTTP status codes:

- `200` - Success
- `201` - Created
- `400` - Bad Request (invalid parameters)
- `401` - Unauthorized (invalid API key)
- `404` - Not Found (resource doesn't exist)
- `409` - Conflict (duplicate external_id, etc.)
- `500` - Internal Server Error
- `502` - Bad Gateway (upstream provider error)

---

## Type Definitions

```python
from typing import TypedDict, Optional, Any

class CreateAPIKeyRequest(TypedDict, total=False):
    name: str
    scopes: list[str]
    expires_in: str

class CreateAPIKeyResponse(TypedDict):
    id: str
    key: str
    key_prefix: str
    name: str
    scopes: list[str]
    created_at: str
    expires_at: Optional[str]

class ApiKeyResponse(TypedDict):
    id: str
    key_prefix: str
    name: str
    scopes: list[str]
    created_at: str
    expires_at: Optional[str]
    last_used_at: Optional[str]
    revoked_at: Optional[str]

class PaginatedApiKeys(TypedDict):
    data: list[ApiKeyResponse]
    has_more: bool
    next_cursor: Optional[str]

class CreateCredentialRequest(TypedDict, total=False):
    label: str
    provider_id: str
    base_url: str
    auth_scheme: str
    api_key: str
    identity_id: Optional[str]
    external_id: Optional[str]
    meta: Optional[dict]
    remaining: Optional[int]
    refill_amount: Optional[int]
    refill_interval: Optional[str]

class CredentialResponse(TypedDict):
    id: str
    label: str
    provider_id: str
    auth_scheme: str
    base_url: str
    created_at: str
    request_count: int
    identity_id: Optional[str]
    meta: Optional[dict]
    remaining: Optional[int]
    refill_amount: Optional[int]
    refill_interval: Optional[str]
    last_used_at: Optional[str]
    revoked_at: Optional[str]

class MintTokenRequest(TypedDict, total=False):
    credential_id: str
    ttl: str
    scopes: list[dict]
    remaining: Optional[int]
    refill_amount: Optional[int]
    refill_interval: Optional[str]
    meta: Optional[dict]

class MintTokenResponse(TypedDict):
    token: str
    jti: str
    expires_at: str
    mcp_endpoint: str

class CreateIdentityRequest(TypedDict, total=False):
    external_id: Optional[str]
    meta: Optional[dict]
    ratelimits: list[dict]

class IdentityResponse(TypedDict):
    id: str
    external_id: Optional[str]
    meta: Optional[dict]
    ratelimits: list[dict]
    created_at: str
    updated_at: str
    request_count: int
    last_used_at: Optional[str]

class CreateConnectSessionRequest(TypedDict, total=False):
    identity_id: Optional[str]
    external_id: Optional[str]
    permissions: list[str]
    ttl: str
    allowed_integrations: list[str]
    allowed_origins: list[str]
    metadata: Optional[dict]

class ConnectSessionResponse(TypedDict):
    id: str
    session_token: str
    identity_id: Optional[str]
    external_id: Optional[str]
    expires_at: str
    created_at: str
    allowed_integrations: list[str]
    allowed_origins: list[str]

class CreateIntegrationRequest(TypedDict, total=False):
    provider: str
    display_name: Optional[str]
    credentials: Optional[dict]
    meta: Optional[dict]

class IntegrationResponse(TypedDict):
    id: str
    provider: str
    display_name: Optional[str]
    unique_key: str
    created_at: str
    updated_at: str
    meta: Optional[dict]
    nango_config: Optional[dict]

class IntegrationProviderInfo(TypedDict):
    name: str
    display_name: str
    auth_mode: str
    webhook_user_defined_secret: bool

class UsageResponse(TypedDict):
    credentials: dict
    tokens: dict
    api_keys: dict
    identities: dict
    requests: dict
    daily_requests: list[dict]
    top_credentials: list[dict]

class AuditEntryResponse(TypedDict):
    id: int
    action: str
    method: str
    path: str
    status: int
    credential_id: Optional[str]
    identity_id: Optional[str]
    latency_ms: int
    ip_address: str
    created_at: str

class OrgResponse(TypedDict):
    id: str
    name: str
    logto_org_id: str
    active: bool
    rate_limit: int
    created_at: str

class ProviderSummary(TypedDict):
    id: str
    name: str
    api: str
    doc: str
    model_count: int

class ProviderDetail(TypedDict):
    id: str
    name: str
    api: str
    doc: str
    models: list[dict]

class ModelSummary(TypedDict):
    id: str
    name: str
    family: str
    cost: Optional[dict]
    tool_call: bool
    structured_output: bool
```

## License

MIT License - see LICENSE file for details.
