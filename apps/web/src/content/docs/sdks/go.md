---
title: Go SDK
description: Official Go SDK for the LLMVault API
---

The official Go SDK for LLMVault provides a type-safe, idiomatic way to interact with the LLMVault API from Go applications.

## Installation

```bash
go get github.com/llmvault/llmvault-go
```

Requires Go 1.21 or later.

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/llmvault/llmvault-go"
)

func main() {
    client := llmvault.New(llmvault.Config{
        APIKey:  "llmv_sk_...", // Your API key
        BaseURL: "https://api.llmvault.dev", // Optional, defaults to production
    })

    // Create an API key
    resp, err := client.APIKeys.Create(context.Background(), llmvault.CreateAPIKeyRequest{
        Name: "my-api-key",
    })
    if err != nil {
        log.Fatalf("Error: %v", err)
    }

    fmt.Printf("Created key: %s\n", resp.Key)
}
```

## Configuration

### `Config`

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `APIKey` | `string` | Yes | Your LLMVault API key (starts with `llmv_sk_`) |
| `BaseURL` | `string` | No | API base URL. Defaults to `https://api.llmvault.dev` |
| `HTTPClient` | `*http.Client` | No | Custom HTTP client |

```go
client := llmvault.New(llmvault.Config{
    APIKey:     "llmv_sk_...",
    BaseURL:    "https://api.llmvault.dev", // Optional
    HTTPClient: &http.Client{Timeout: 30 * time.Second}, // Optional
})
```

## Resources

The SDK is organized into resource namespaces that mirror the API structure:

```go
client := llmvault.New(config)

client.APIKeys       // APIKeysResource - API key management
client.Credentials   // CredentialsResource - LLM credential storage
client.Tokens        // TokensResource - Proxy token minting
client.Identities    // IdentitiesResource - Identity management
client.Connect       // ConnectResource - Connect widget (sessions & settings)
client.Integrations  // IntegrationsResource - OAuth integrations
client.Connections   // ConnectionsResource - Integration connections
client.Usage         // UsageResource - Usage statistics
client.Audit         // AuditResource - Audit log
client.Org           // OrgResource - Organization info
client.Providers     // ProvidersResource - LLM provider catalog
```

---

## API Keys

Manage API keys for your organization.

### `Create(ctx, req)`

Create a new API key.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `ctx` | `context.Context` | Yes | Request context |
| `req.Name` | `string` | Yes | Descriptive name for the key |

**Returns:** `(*CreateAPIKeyResponse, error)`

```go
resp, err := client.APIKeys.Create(ctx, llmvault.CreateAPIKeyRequest{
    Name: fmt.Sprintf("sdk-test-%d", time.Now().Unix()),
})
if err != nil {
    log.Fatal(err)
}

fmt.Printf("ID: %s\n", resp.ID)
fmt.Printf("Key: %s\n", resp.Key)        // Full key (shown once)
fmt.Printf("Prefix: %s\n", resp.KeyPrefix) // First 16 chars

// Response structure:
// type CreateAPIKeyResponse struct {
//     ID        string    `json:"id"`
//     Key       string    `json:"key"`        // Full key (shown once)
//     KeyPrefix string    `json:"key_prefix"` // First 16 chars
//     Name      string    `json:"name"`
//     CreatedAt time.Time `json:"created_at"`
// }
```

### `List(ctx, opts)`

List API keys with cursor pagination.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `ctx` | `context.Context` | Yes | Request context |
| `opts.Limit` | `int` | No | Max items per page (1-100, default 50) |
| `opts.Cursor` | `string` | No | Pagination cursor |

**Returns:** `(*PaginatedAPIKeys, error)`

```go
resp, err := client.APIKeys.List(ctx, llmvault.ListAPIKeysOptions{
    Limit: 5,
})
if err != nil {
    log.Fatal(err)
}

for _, key := range resp.Data {
    fmt.Printf("Key: %s\n", key.Name)
}
fmt.Printf("Has more: %v\n", resp.HasMore)

// Response structure:
// type PaginatedAPIKeys struct {
//     Data       []APIKeyResponse `json:"data"`
//     HasMore    bool             `json:"has_more"`
//     NextCursor *string          `json:"next_cursor"`
// }
```

### `Delete(ctx, id)`

Revoke an API key by ID.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `ctx` | `context.Context` | Yes | Request context |
| `id` | `string` | Yes | API key ID |

**Returns:** `(map[string]string, error)`

```go
resp, err := client.APIKeys.Delete(ctx, "key_abc123")
if err != nil {
    log.Fatal(err)
}
```

---

## Credentials

Store and manage encrypted LLM API credentials.

### `Create(ctx, req)`

Store a new credential.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `ctx` | `context.Context` | Yes | Request context |
| `req.Label` | `string` | Yes | Descriptive label |
| `req.APIKey` | `string` | Yes | The API key to encrypt |
| `req.ProviderID` | `*string` | No | Provider ID (e.g., "openai", "anthropic") |
| `req.BaseURL` | `*string` | No | API base URL |
| `req.AuthScheme` | `*string` | No | Auth scheme (e.g., "bearer") |
| `req.IdentityID` | `*string` | No | Link to an identity |
| `req.Meta` | `map[string]any` | No | Metadata object |

**Returns:** `(*CredentialResponse, error)`

```go
resp, err := client.Credentials.Create(ctx, llmvault.CreateCredentialRequest{
    Label:      "OpenAI Production",
    APIKey:     "sk-...",
    ProviderID: stringPtr("openai"),
    BaseURL:    stringPtr("https://api.openai.com/v1"),
    AuthScheme: stringPtr("bearer"),
})
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Created: %s\n", resp.ID)
fmt.Printf("Provider: %s\n", *resp.ProviderID)

// Response structure:
// type CredentialResponse struct {
//     ID             string         `json:"id"`
//     Label          string         `json:"label"`
//     ProviderID     *string        `json:"provider_id"`
//     AuthScheme     *string        `json:"auth_scheme"`
//     BaseURL        *string        `json:"base_url"`
//     CreatedAt      time.Time      `json:"created_at"`
//     RequestCount   int            `json:"request_count"`
//     IdentityID     *string        `json:"identity_id"`
//     Meta           map[string]any `json:"meta"`
//     LastUsedAt     *time.Time     `json:"last_used_at"`
//     RevokedAt      *time.Time     `json:"revoked_at"`
// }
```

### `List(ctx, opts)`

List credentials with filtering and pagination.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `ctx` | `context.Context` | Yes | Request context |
| `opts.Limit` | `int` | No | Page size |
| `opts.Cursor` | `string` | No | Pagination cursor |
| `opts.IdentityID` | `string` | No | Filter by identity ID |
| `opts.ExternalID` | `string` | No | Filter by external ID |
| `opts.Meta` | `string` | No | Filter by metadata |

**Returns:** `(*PaginatedCredentials, error)`

```go
resp, err := client.Credentials.List(ctx, llmvault.ListCredentialsOptions{
    Limit: 5,
})
if err != nil {
    log.Fatal(err)
}

for _, cred := range resp.Data {
    providerID := ""
    if cred.ProviderID != nil {
        providerID = *cred.ProviderID
    }
    fmt.Printf("Credential: %s (%s)\n", cred.Label, providerID)
}
```

### `Get(ctx, id)`

Get a single credential by ID.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `ctx` | `context.Context` | Yes | Request context |
| `id` | `string` | Yes | Credential ID |

**Returns:** `(*CredentialResponse, error)`

```go
resp, err := client.Credentials.Get(ctx, "cred_abc123")
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Label: %s\n", resp.Label)
fmt.Printf("Request count: %d\n", resp.RequestCount)
```

### `Delete(ctx, id)`

Revoke a credential.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `ctx` | `context.Context` | Yes | Request context |
| `id` | `string` | Yes | Credential ID |

**Returns:** `(*CredentialResponse, error)`

```go
resp, err := client.Credentials.Delete(ctx, "cred_abc123")
if err != nil {
    log.Fatal(err)
}
```

---

## Tokens

Mint short-lived proxy tokens for LLM API access.

### `Create(ctx, req)`

Mint a new proxy token.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `ctx` | `context.Context` | Yes | Request context |
| `req.CredentialID` | `string` | Yes | Credential to scope the token to |
| `req.Name` | `*string` | No | Descriptive name for the token |
| `req.Scopes` | `[]TokenScope` | No | Array of permission scopes |
| `req.ExpiresIn` | `*string` | No | Token lifetime (e.g., "1h", "24h") |

**Returns:** `(*MintTokenResponse, error)`

```go
resp, err := client.Tokens.Create(ctx, llmvault.MintTokenRequest{
    CredentialID: "cred_abc123",
    ExpiresIn:    stringPtr("1h"),
})
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Token: %s\n", resp.Token) // Starts with ptok_
fmt.Printf("JTI: %s\n", resp.JTI)
fmt.Printf("Expires: %s\n", resp.ExpiresAt)
fmt.Printf("MCP Endpoint: %s\n", resp.MCPEndpoint)

// Response structure:
// type MintTokenResponse struct {
//     Token       string     `json:"token"`        // Starts with ptok_
//     JTI         string     `json:"jti"`
//     ExpiresAt   time.Time  `json:"expires_at"`
//     MCPEndpoint string     `json:"mcp_endpoint"`
// }
```

### `List(ctx, opts)`

List tokens with pagination.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `ctx` | `context.Context` | Yes | Request context |
| `opts.Limit` | `int` | No | Page size |
| `opts.Cursor` | `string` | No | Pagination cursor |
| `opts.CredentialID` | `string` | No | Filter by credential |

**Returns:** `(*PaginatedTokens, error)`

```go
resp, err := client.Tokens.List(ctx, llmvault.ListTokensOptions{
    CredentialID: stringPtr("cred_abc123"),
})
if err != nil {
    log.Fatal(err)
}

for _, token := range resp.Data {
    fmt.Printf("Token: %s - Expires: %s\n", token.JTI, token.ExpiresAt)
}
```

### `Delete(ctx, jti)`

Revoke a token by its JTI.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `ctx` | `context.Context` | Yes | Request context |
| `jti` | `string` | Yes | Token JTI (JWT ID) |

**Returns:** `(map[string]string, error)`

```go
resp, err := client.Tokens.Delete(ctx, "jti_abc123")
if err != nil {
    log.Fatal(err)
}
```

---

## Identities

Manage user identities for request tracking and rate limiting.

### `Create(ctx, req)`

Create a new identity.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `ctx` | `context.Context` | Yes | Request context |
| `req.ExternalID` | `string` | Yes | External reference ID |
| `req.Name` | `*string` | No | Display name |
| `req.Meta` | `map[string]any` | No | Metadata object |
| `req.RateLimit` | `*RateLimitConfig` | No | Rate limit configuration |

**Returns:** `(*IdentityResponse, error)`

```go
resp, err := client.Identities.Create(ctx, llmvault.CreateIdentityRequest{
    ExternalID: "user_123",
    Name:       stringPtr("User 123"),
    Meta: map[string]any{
        "source": "web-app",
    },
})
if err != nil {
    log.Fatal(err)
}

fmt.Printf("ID: %s\n", resp.ID)
fmt.Printf("External ID: %s\n", resp.ExternalID)

// Response structure:
// type IdentityResponse struct {
//     ID           string          `json:"id"`
//     ExternalID   string          `json:"external_id"`
//     Name         *string         `json:"name"`
//     Meta         map[string]any  `json:"meta"`
//     RateLimit    *RateLimitConfig `json:"rate_limit"`
//     CreatedAt    time.Time       `json:"created_at"`
//     UpdatedAt    time.Time       `json:"updated_at"`
//     RequestCount int             `json:"request_count"`
//     LastUsedAt   *time.Time      `json:"last_used_at"`
// }
```

### `List(ctx, opts)`

List identities with pagination.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `ctx` | `context.Context` | Yes | Request context |
| `opts.Limit` | `int` | No | Page size |
| `opts.Cursor` | `string` | No | Pagination cursor |
| `opts.ExternalID` | `string` | No | Filter by external ID |
| `opts.Meta` | `string` | No | Filter by metadata |

**Returns:** `(*PaginatedIdentities, error)`

```go
resp, err := client.Identities.List(ctx, llmvault.ListIdentitiesOptions{
    Limit: 5,
})
if err != nil {
    log.Fatal(err)
}
```

### `Get(ctx, id)`

Get a single identity.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `ctx` | `context.Context` | Yes | Request context |
| `id` | `string` | Yes | Identity ID |

**Returns:** `(*IdentityResponse, error)`

```go
resp, err := client.Identities.Get(ctx, "id_abc123")
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Identity: %s\n", resp.ExternalID)
fmt.Printf("Request count: %d\n", resp.RequestCount)
```

### `Update(ctx, id, req)`

Update an identity.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `ctx` | `context.Context` | Yes | Request context |
| `id` | `string` | Yes | Identity ID |
| `req.Name` | `*string` | No | New display name |
| `req.Meta` | `map[string]any` | No | New metadata |
| `req.RateLimit` | `*RateLimitConfig` | No | New rate limit configuration |

**Returns:** `(*IdentityResponse, error)`

```go
resp, err := client.Identities.Update(ctx, "id_abc123", llmvault.UpdateIdentityRequest{
    Meta: map[string]any{
        "source": "web-app",
        "updated": true,
    },
})
if err != nil {
    log.Fatal(err)
}
```

### `Delete(ctx, id)`

Delete an identity.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `ctx` | `context.Context` | Yes | Request context |
| `id` | `string` | Yes | Identity ID |

**Returns:** `(map[string]string, error)`

```go
resp, err := client.Identities.Delete(ctx, "id_abc123")
if err != nil {
    log.Fatal(err)
}
```

---

## Connect

Manage the Connect widget for end-user credential collection.

### `Sessions.Create(ctx, req)`

Create a short-lived session for the Connect widget.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `ctx` | `context.Context` | Yes | Request context |
| `req.IdentityID` | `*string` | No | Identity ID |
| `req.IntegrationIDs` | `[]string` | No | Restrict to specific integrations |
| `req.RedirectURL` | `*string` | No | Redirect URL after completion |
| `req.Meta` | `map[string]any` | No | Session metadata |

**Returns:** `(*ConnectSessionResponse, error)`

```go
resp, err := client.Connect.Sessions.Create(ctx, llmvault.CreateConnectSessionRequest{
    IdentityID:  stringPtr("id_xyz789"),
    RedirectURL: stringPtr("https://app.example.com/callback"),
})
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Session Token: %s\n", resp.SessionToken) // Starts with csess_
fmt.Printf("Expires: %s\n", resp.ExpiresAt)

// Response structure:
// type ConnectSessionResponse struct {
//     ID           string    `json:"id"`
//     SessionToken string    `json:"session_token"` // Starts with csess_
//     IdentityID   *string   `json:"identity_id"`
//     ExpiresAt    time.Time `json:"expires_at"`
//     CreatedAt    time.Time `json:"created_at"`
// }
```

### `Settings.Get(ctx)`

Get Connect widget settings.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `ctx` | `context.Context` | Yes | Request context |

**Returns:** `(*ConnectSettingsResponse, error)`

```go
resp, err := client.Connect.Settings.Get(ctx)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Allowed origins: %v\n", resp.AllowedOrigins)

// Response structure:
// type ConnectSettingsResponse struct {
//     AllowedOrigins []string `json:"allowed_origins"`
// }
```

### `Settings.Update(ctx, req)`

Update Connect widget settings.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `ctx` | `context.Context` | Yes | Request context |
| `req.AllowedOrigins` | `[]string` | No | Allowed origins for the widget |

**Returns:** `(*ConnectSettingsResponse, error)`

```go
resp, err := client.Connect.Settings.Update(ctx, llmvault.ConnectSettingsRequest{
    AllowedOrigins: []string{"https://app.example.com"},
})
if err != nil {
    log.Fatal(err)
}
```

---

## Integrations

Manage OAuth integrations with third-party providers.

### `Create(ctx, req)`

Create a new integration.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `ctx` | `context.Context` | Yes | Request context |
| `req.Provider` | `string` | Yes | Provider name (e.g., "slack", "github") |
| `req.DisplayName` | `*string` | No | Custom display name |
| `req.Credentials` | `*NangoCredentials` | No | OAuth credentials |
| `req.Meta` | `map[string]any` | No | Metadata |

**Returns:** `(*IntegrationResponse, error)`

```go
resp, err := client.Integrations.Create(ctx, llmvault.CreateIntegrationRequest{
    Provider:    "slack",
    DisplayName: stringPtr("Slack Workspace"),
})
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Integration: %s\n", resp.ID)

// Response structure:
// type IntegrationResponse struct {
//     ID         string         `json:"id"`
//     Provider   string         `json:"provider"`
//     DisplayName *string       `json:"display_name"`
//     UniqueKey  string         `json:"unique_key"`
//     CreatedAt  time.Time      `json:"created_at"`
//     UpdatedAt  time.Time      `json:"updated_at"`
//     Meta       map[string]any `json:"meta"`
//     NangoConfig map[string]any `json:"nango_config"`
// }
```

### `List(ctx, opts)`

List integrations.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `ctx` | `context.Context` | Yes | Request context |
| `opts.Limit` | `int` | No | Page size |
| `opts.Cursor` | `string` | No | Pagination cursor |
| `opts.Provider` | `string` | No | Filter by provider |
| `opts.Meta` | `string` | No | Filter by metadata |

**Returns:** `(*PaginatedIntegrations, error)`

```go
resp, err := client.Integrations.List(ctx, llmvault.ListIntegrationsOptions{
    Limit: 10,
})
if err != nil {
    log.Fatal(err)
}
```

### `Get(ctx, id)`

Get a single integration.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `ctx` | `context.Context` | Yes | Request context |
| `id` | `string` | Yes | Integration ID |

**Returns:** `(*IntegrationResponse, error)`

```go
resp, err := client.Integrations.Get(ctx, "int_abc123")
if err != nil {
    log.Fatal(err)
}
```

### `Update(ctx, id, req)`

Update an integration.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `ctx` | `context.Context` | Yes | Request context |
| `id` | `string` | Yes | Integration ID |
| `req.DisplayName` | `*string` | No | New display name |
| `req.Credentials` | `*NangoCredentials` | No | New credentials |
| `req.Meta` | `map[string]any` | No | New metadata |

**Returns:** `(*IntegrationResponse, error)`

```go
resp, err := client.Integrations.Update(ctx, "int_abc123", llmvault.UpdateIntegrationRequest{
    DisplayName: stringPtr("Updated Name"),
})
if err != nil {
    log.Fatal(err)
}
```

### `Delete(ctx, id)`

Delete an integration.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `ctx` | `context.Context` | Yes | Request context |
| `id` | `string` | Yes | Integration ID |

**Returns:** `(map[string]string, error)`

```go
resp, err := client.Integrations.Delete(ctx, "int_abc123")
if err != nil {
    log.Fatal(err)
}
```

### `ListProviders(ctx)`

List available integration providers.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `ctx` | `context.Context` | Yes | Request context |

**Returns:** `([]IntegrationProviderInfo, error)`

```go
providers, err := client.Integrations.ListProviders(ctx)
if err != nil {
    log.Fatal(err)
}

for _, p := range providers {
    fmt.Printf("%s: %s\n", p.Name, p.DisplayName)
}

// Response structure:
// type IntegrationProviderInfo struct {
//     Name                      string `json:"name"`
//     DisplayName               string `json:"display_name"`
//     AuthMode                  string `json:"auth_mode"`
//     WebhookUserDefinedSecret  bool   `json:"webhook_user_defined_secret"`
// }
```

---

## Connections

Manage connections to OAuth integrations.

### `AvailableScopes(ctx)`

List available scopes for all active connections.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `ctx` | `context.Context` | Yes | Request context |

**Returns:** `([]AvailableScopeConnection, error)`

```go
scopes, err := client.Connections.AvailableScopes(ctx)
if err != nil {
    log.Fatal(err)
}

for _, conn := range scopes {
    fmt.Printf("%s (%s)\n", conn.DisplayName, conn.Provider)
    for _, action := range conn.Actions {
        fmt.Printf("  - %s\n", action.DisplayName)
    }
}
```

### `Create(ctx, integrationID, req)`

Create a connection for an integration.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `ctx` | `context.Context` | Yes | Request context |
| `integrationID` | `string` | Yes | Integration ID |
| `req.IdentityID` | `*string` | No | Identity ID |
| `req.Meta` | `map[string]any` | No | Metadata |
| `req.NangoConnectionID` | `*string` | Yes | Nango connection ID |

**Returns:** `(*IntegConnResponse, error)`

```go
resp, err := client.Connections.Create(ctx, "int_abc123", llmvault.IntegConnCreateRequest{
    NangoConnectionID: stringPtr("nango_conn_xyz"),
})
if err != nil {
    log.Fatal(err)
}
```

### `List(ctx, integrationID, opts)`

List connections for an integration.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `ctx` | `context.Context` | Yes | Request context |
| `integrationID` | `string` | Yes | Integration ID |
| `opts.Limit` | `int` | No | Page size |
| `opts.Cursor` | `string` | No | Pagination cursor |

**Returns:** `(*PaginatedIntegConns, error)`

```go
resp, err := client.Connections.List(ctx, "int_abc123", llmvault.ListConnectionsOptions{})
if err != nil {
    log.Fatal(err)
}
```

### `Get(ctx, id)`

Get a connection.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `ctx` | `context.Context` | Yes | Request context |
| `id` | `string` | Yes | Connection ID |

**Returns:** `(*IntegConnResponse, error)`

```go
resp, err := client.Connections.Get(ctx, "conn_abc123")
if err != nil {
    log.Fatal(err)
}
```

### `RetrieveToken(ctx, id)`

Retrieve the OAuth access token for a connection.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `ctx` | `context.Context` | Yes | Request context |
| `id` | `string` | Yes | Connection ID |

**Returns:** `(*TokenResponse, error)`

```go
resp, err := client.Connections.RetrieveToken(ctx, "conn_abc123")
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Access token: %s\n", resp.AccessToken)
fmt.Printf("Expires: %s\n", resp.ExpiresAt)

// Response structure:
// type TokenResponse struct {
//     AccessToken string     `json:"access_token"`
//     TokenType   string     `json:"token_type"`
//     ExpiresAt   *time.Time `json:"expires_at"`
//     Provider    string     `json:"provider"`
//     ConnectionID string    `json:"connection_id"`
// }
```

### `Delete(ctx, id)`

Revoke a connection.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `ctx` | `context.Context` | Yes | Request context |
| `id` | `string` | Yes | Connection ID |

**Returns:** `(map[string]string, error)`

```go
resp, err := client.Connections.Delete(ctx, "conn_abc123")
if err != nil {
    log.Fatal(err)
}
```

---

## Usage

Get organization usage statistics.

### `Get(ctx)`

Get aggregated usage stats.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `ctx` | `context.Context` | Yes | Request context |

**Returns:** `(*UsageResponse, error)`

```go
resp, err := client.Usage.Get(ctx)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Credentials: %+v\n", resp.Credentials)
fmt.Printf("Tokens: %+v\n", resp.Tokens)
fmt.Printf("Requests today: %d\n", resp.Requests.Today)

// Response structure:
// type UsageResponse struct {
//     Credentials     CredentialStats     `json:"credentials"`
//     Tokens          TokenStats          `json:"tokens"`
//     APIKeys         APIKeyStats         `json:"api_keys"`
//     Identities      IdentityStats       `json:"identities"`
//     Requests        RequestStats        `json:"requests"`
//     DailyRequests   []DailyRequests     `json:"daily_requests"`
//     TopCredentials  []TopCredential     `json:"top_credentials"`
// }
```

---

## Audit

Access the audit log.

### `List(ctx, opts)`

List audit log entries.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `ctx` | `context.Context` | Yes | Request context |
| `opts.Limit` | `int` | No | Page size |
| `opts.Cursor` | `string` | No | Pagination cursor |
| `opts.Action` | `string` | No | Filter by action (e.g., "proxy.request") |

**Returns:** `(*PaginatedAuditEntries, error)`

```go
resp, err := client.Audit.List(ctx, llmvault.ListAuditOptions{
    Limit: 5,
})
if err != nil {
    log.Fatal(err)
}

for _, entry := range resp.Data {
    fmt.Printf("%s: %s %s (%d)\n", entry.Action, entry.Method, entry.Path, entry.Status)
}

// Response structure:
// type AuditEntryResponse struct {
//     ID           int        `json:"id"`
//     Action       string     `json:"action"`
//     Method       string     `json:"method"`
//     Path         string     `json:"path"`
//     Status       int        `json:"status"`
//     CredentialID *string    `json:"credential_id"`
//     IdentityID   *string    `json:"identity_id"`
//     LatencyMS    int        `json:"latency_ms"`
//     IPAddress    string     `json:"ip_address"`
//     CreatedAt    time.Time  `json:"created_at"`
// }
```

---

## Organization

### `GetCurrent(ctx)`

Get current organization details.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `ctx` | `context.Context` | Yes | Request context |

**Returns:** `(*OrgResponse, error)`

```go
resp, err := client.Org.GetCurrent(ctx)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Org: %s\n", resp.Name)
fmt.Printf("Rate limit: %d\n", resp.RateLimit)

// Response structure:
// type OrgResponse struct {
//     ID         string    `json:"id"`
//     Name       string    `json:"name"`
//     Active     bool      `json:"active"`
//     RateLimit  int       `json:"rate_limit"`
//     CreatedAt  time.Time `json:"created_at"`
// }
```

---

## Providers

Browse the LLM provider catalog.

### `List(ctx)`

List all available LLM providers.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `ctx` | `context.Context` | Yes | Request context |

**Returns:** `([]ProviderSummary, error)`

```go
providers, err := client.Providers.List(ctx)
if err != nil {
    log.Fatal(err)
}

for _, p := range providers {
    fmt.Printf("%s: %d models\n", p.Name, p.ModelCount)
}

// Response structure:
// type ProviderSummary struct {
//     ID         string `json:"id"`
//     Name       string `json:"name"`
//     API        string `json:"api"`
//     Doc        string `json:"doc"`
//     ModelCount int    `json:"model_count"`
// }
```

### `Get(ctx, id)`

Get provider details with all models.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `ctx` | `context.Context` | Yes | Request context |
| `id` | `string` | Yes | Provider ID |

**Returns:** `(*ProviderDetail, error)`

```go
resp, err := client.Providers.Get(ctx, "openai")
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Provider: %s\n", resp.Name)
fmt.Printf("Models: %d\n", len(resp.Models))

// Response structure:
// type ProviderDetail struct {
//     ID     string         `json:"id"`
//     Name   string         `json:"name"`
//     API    string         `json:"api"`
//     Doc    string         `json:"doc"`
//     Models []ModelSummary `json:"models"`
// }
```

### `ListModels(ctx, id)`

List models for a provider.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `ctx` | `context.Context` | Yes | Request context |
| `id` | `string` | Yes | Provider ID |

**Returns:** `([]ModelSummary, error)`

```go
models, err := client.Providers.ListModels(ctx, "openai")
if err != nil {
    log.Fatal(err)
}

for _, m := range models {
    fmt.Printf("%s: tool_call=%v\n", m.ID, m.ToolCall)
}

// Response structure:
// type ModelSummary struct {
//     ID               string     `json:"id"`
//     Name             string     `json:"name"`
//     Family           string     `json:"family"`
//     Cost             *Cost      `json:"cost"`
//     ToolCall         bool       `json:"tool_call"`
//     StructuredOutput bool       `json:"structured_output"`
// }
```

---

## Error Handling

The SDK returns detailed errors that implement the standard `error` interface:

```go
resp, err := client.APIKeys.Create(ctx, req)
if err != nil {
    var apiErr *llmvault.APIError
    if errors.As(err, &apiErr) {
        // Structured API error
        fmt.Printf("API Error: %s\n", apiErr.Message)
        fmt.Printf("Status: %d\n", apiErr.StatusCode)
        fmt.Printf("Request ID: %s\n", apiErr.RequestID)
    } else {
        // Network or other error
        fmt.Printf("Error: %v\n", err)
    }
    return
}

// resp is guaranteed to be non-nil here
fmt.Printf("Success: %+v\n", resp)
```

### Error Types

```go
// APIError represents an error response from the API
type APIError struct {
    StatusCode int    // HTTP status code
    Message    string // Error message from API
    RequestID  string // Request ID for debugging
}

func (e *APIError) Error() string {
    return fmt.Sprintf("API error %d: %s", e.StatusCode, e.Message)
}
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

## Helper Functions

```go
// stringPtr returns a pointer to a string value
func stringPtr(s string) *string {
    return &s
}

// intPtr returns a pointer to an int value
func intPtr(i int) *int {
    return &i
}
```

## License

MIT License - see LICENSE file for details.
