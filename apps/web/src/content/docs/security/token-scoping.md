---
title: Token Scoping
description: How scoped tokens limit access to specific resources and actions.
---

# Token Scoping

Token scoping allows you to create fine-grained, least-privilege access tokens that can only perform specific actions on specific resources. This is essential for security when giving LLMs access to external integrations.

## Scope Structure

### TokenScope Definition

```go
// From: internal/mcp/scope.go
type TokenScope struct {
    ConnectionID string              `json:"connection_id"`
    Actions      []string            `json:"actions"`
    Resources    map[string][]string `json:"resources,omitempty"`
}
```

**Fields:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `connection_id` | string | Yes | UUID of the OAuth connection |
| `actions` | []string | Yes | List of allowed actions from the provider catalog |
| `resources` | map[string][]string | No | Resource type → Resource IDs mapping |

### Example Scope

```json
{
  "connection_id": "550e8400-e29b-41d4-a716-446655440000",
  "actions": ["slack.post_message", "slack.list_channels"],
  "resources": {
    "channel": ["C1234567890", "C0987654321"]
  }
}
```

This scope grants the token permission to:
- Post messages to two specific Slack channels
- List channels (resource-constrained to the specified channels)

## Actions

### Action Definition

Actions are defined in the provider catalog (`internal/mcp/catalog/providers/*.actions.json`):

```go
// From: internal/mcp/catalog/catalog.go
type ActionDef struct {
    DisplayName  string           `json:"display_name"`
    Description  string           `json:"description"`
    Access       string           `json:"access"`              // "read" or "write"
    ResourceType string           `json:"resource_type"`       // e.g. "channel", "repo"
    Parameters   json.RawMessage  `json:"parameters"`          // JSON Schema
    Execution    *ExecutionConfig `json:"execution,omitempty"` // Execution config
}

// Access type constants
const (
    AccessRead  = "read"
    AccessWrite = "write"
)
```

### Catalog Structure

```json
{
  "display_name": "Slack",
  "resources": {
    "channel": {
      "display_name": "Channel",
      "description": "A Slack channel",
      "id_field": "id",
      "name_field": "name",
      "icon": "hash",
      "list_action": "slack.list_channels"
    }
  },
  "actions": {
    "slack.post_message": {
      "display_name": "Post Message",
      "description": "Post a message to a channel",
      "access": "write",
      "resource_type": "channel",
      "parameters": {
        "type": "object",
        "required": ["channel", "text"],
        "properties": {
          "channel": { "type": "string", "description": "Channel ID" },
          "text": { "type": "string", "description": "Message text" }
        }
      }
    },
    "slack.list_channels": {
      "display_name": "List Channels",
      "description": "List accessible channels",
      "access": "read",
      "resource_type": "channel"
    }
  }
}
```

### Action Validation

All actions are validated against the catalog at token mint time:

```go
// From: internal/mcp/scope.go
func ValidateScopes(db *gorm.DB, orgID uuid.UUID, cat *catalog.Catalog, scopes []TokenScope) error {
    for i, scope := range scopes {
        // Validate connection exists and belongs to org
        var conn model.Connection
        if err := db.Preload("Integration").
            Where("id = ? AND org_id = ? AND revoked_at IS NULL", connUUID, orgID).
            First(&conn).Error; err != nil {
            return fmt.Errorf("scope[%d]: connection %q not found or revoked", i, scope.ConnectionID)
        }

        provider := conn.Integration.Provider

        // Validate actions against catalog
        if err := cat.ValidateActions(provider, scope.Actions); err != nil {
            return fmt.Errorf("scope[%d]: %w", i, err)
        }

        // Validate resources
        if err := cat.ValidateResources(provider, scope.Actions, scope.Resources, nil); err != nil {
            return fmt.Errorf("scope[%d]: %w", i, err)
        }
    }
    return nil
}
```

### Wildcard Rejection

For security, wildcard actions are explicitly rejected:

```go
// From: internal/mcp/catalog/catalog.go
func (c *Catalog) ValidateActions(provider string, actions []string) error {
    for _, action := range actions {
        if action == "*" {
            return fmt.Errorf("wildcard actions are not allowed; explicitly list each action")
        }
        if _, ok := p.Actions[action]; !ok {
            return fmt.Errorf("unknown action %q for provider %q", action, provider)
        }
    }
    return nil
}
```

## Resources

### Resource Types

Resources constrain actions to specific instances (e.g., specific Slack channels):

```go
// From: internal/mcp/catalog/catalog.go
type ResourceDef struct {
    DisplayName   string         `json:"display_name"`
    Description   string         `json:"description"`
    IDField       string         `json:"id_field"`       // Field containing the resource ID
    NameField     string         `json:"name_field"`     // Field containing display name
    Icon          string         `json:"icon,omitempty"`
    ListAction    string         `json:"list_action"`    // Action to list available resources
    RequestConfig *RequestConfig `json:"request_config,omitempty"`
}
```

### Resource Validation

Resources are validated against the connection's configured resources:

```go
// From: internal/mcp/catalog/catalog.go
func (c *Catalog) ValidateResources(provider string, actions []string, requestedResources, allowedResources map[string][]string) error {
    // Build set of valid resource types from the listed actions
    validResourceTypes := make(map[string]bool)
    for _, actionKey := range actions {
        if action, ok := p.Actions[actionKey]; ok && action.ResourceType != "" {
            validResourceTypes[action.ResourceType] = true
        }
    }

    for resourceType, requestedIDs := range requestedResources {
        // Check resource type is valid for these actions
        if !validResourceTypes[resourceType] {
            return fmt.Errorf("resource type %q does not match any listed action", resourceType)
        }

        // Check each requested ID is in the allowed set
        if allowedResources != nil {
            allowedIDs := allowedResources[resourceType]
            allowedSet := make(map[string]bool, len(allowedIDs))
            for _, id := range allowedIDs {
                allowedSet[id] = true
            }

            for _, reqID := range requestedIDs {
                if !allowedSet[reqID] {
                    return fmt.Errorf("resource %q of type %q not configured for this connection", reqID, resourceType)
                }
            }
        }
    }
    return nil
}
```

### Resource Example: GitHub

```json
{
  "connection_id": "550e8400-e29b-41d4-a716-446655440000",
  "actions": ["github.list_issues", "github.create_issue"],
  "resources": {
    "repo": ["llmvault/llmvault", "llmvault/docs"],
    "issue": ["*"]
  }
}
```

This scope:
- Limits repository access to two specific repos
- Allows issue operations on all issues within those repos

## MCP Scopes

### MCP Integration

Scopes are integrated with the Model Context Protocol (MCP) server:

```go
// From: internal/handler/mcphandler.go
type MCPHandler struct {
    db          *gorm.DB
    signingKey  []byte
    catalog     *catalog.Catalog
    nango       *nango.Client
    counter     *counter.Counter
    ServerCache *mcpserver.ServerCache
}

// serverFactory returns or builds an MCP server for the given request's token.
func (h *MCPHandler) serverFactory(r *http.Request) *mcp.Server {
    claims, ok := middleware.ClaimsFromContext(r.Context())
    
    srv, err := h.ServerCache.GetOrBuild(claims.JTI, func() (*mcp.Server, time.Time, error) {
        // Load token record with scopes
        var token model.Token
        if err := h.db.Where("jti = ?", claims.JTI).First(&token).Error; err != nil {
            return nil, time.Time{}, err
        }

        // Parse scopes from JSONB
        scopes, err := parseTokenScopes(token.Scopes)
        if err != nil {
            return nil, time.Time{}, err
        }

        // Build MCP server from scopes
        srv, err := mcpserver.BuildServer(&token, scopes, h.catalog, h.nango, h.db, h.counter)
        return srv, token.ExpiresAt, nil
    })
    
    return srv
}
```

### Scope Hash in JWT

Scopes are committed to in the JWT claims via a SHA-256 hash:

```go
// From: internal/mcp/scope.go
func ScopeHash(scopes []TokenScope) (string, error) {
    canonical, err := json.Marshal(scopes)
    if err != nil {
        return "", fmt.Errorf("marshaling scopes: %w", err)
    }
    hash := sha256.Sum256(canonical)
    return fmt.Sprintf("%x", hash), nil
}
```

**JWT Claims:**
```go
// From: internal/token/jwt.go
type Claims struct {
    OrgID        string `json:"org_id"`
    CredentialID string `json:"cred_id"`
    ScopeHash    string `json:"scope_hash,omitempty"`  // SHA-256 of scopes
    jwt.RegisteredClaims
}
```

This prevents scope tampering - the server validates the scopes match the hash before execution.

### MCP Tool Registration

Each scope action becomes an MCP tool:

```go
// From: internal/mcpserver/builder.go
func BuildServer(token *model.Token, scopes []mcp.TokenScope, cat *catalog.Catalog, nangoClient *nango.Client, db *gorm.DB, ctr *counter.Counter) (*mcp.Server, error) {
    server := mcp.NewServer(&mcp.Implementation{
        Name:    "llmvault",
        Version: "v1.0.0",
    }, nil)

    for _, scope := range scopes {
        for _, actionKey := range scope.Actions {
            action, ok := cat.GetAction(provider, actionKey)
            
            toolName := provider + "_" + actionKey
            inputSchema := buildInputSchema(action.Parameters)

            server.AddTool(
                &mcp.Tool{
                    Name:        toolName,
                    Description: action.Description,
                    InputSchema: inputSchema,
                },
                func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
                    // Execute with resource constraints
                    result, err := ExecuteAction(ctx, nangoClient, capturedProvider, 
                        capturedCfgKey, capturedConnID, capturedAction, params, capturedResources)
                    // ...
                },
            )
        }
    }
    return server, nil
}
```

### Scope Validation Middleware

Two middleware functions enforce scope constraints:

```go
// From: internal/handler/mcphandler.go

// ValidateJTIMatch ensures the URL {jti} matches the JWT's JTI claim.
func (h *MCPHandler) ValidateJTIMatch(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        urlJTI := chi.URLParam(r, "jti")
        claims, ok := middleware.ClaimsFromContext(r.Context())
        if !ok || urlJTI != claims.JTI {
            writeJSON(w, http.StatusForbidden, map[string]string{
                "error": "token JTI does not match URL"
            })
            return
        }
        next.ServeHTTP(w, r)
    })
}

// ValidateHasScopes ensures the token has scopes (returns 403 if no scopes).
func (h *MCPHandler) ValidateHasScopes(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        claims, ok := middleware.ClaimsFromContext(r.Context())
        if !ok {
            writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing claims"})
            return
        }

        var token model.Token
        if err := h.db.Where("jti = ?", claims.JTI).First(&token).Error; err != nil {
            writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "token not found"})
            return
        }

        scopes, err := parseTokenScopes(token.Scopes)
        if err != nil || len(scopes) == 0 {
            writeJSON(w, http.StatusForbidden, map[string]string{"error": "token has no MCP scopes"})
            return
        }

        next.ServeHTTP(w, r)
    })
}
```

## Token Minting with Scopes

### API Request

```bash
POST /v1/tokens
Authorization: Bearer llmv_sk_...
Content-Type: application/json

{
  "credential_id": "550e8400-e29b-41d4-a716-446655440000",
  "ttl": "1h",
  "scopes": [
    {
      "connection_id": "660e8400-e29b-41d4-a716-446655440001",
      "actions": ["slack.post_message", "slack.list_channels"],
      "resources": {
        "channel": ["C1234567890"]
      }
    }
  ]
}
```

### Token Handler Implementation

```go
// From: internal/handler/tokens.go
func (h *TokenHandler) Mint(w http.ResponseWriter, r *http.Request) {
    // ... validation ...

    // Validate scopes against catalog and database
    if len(req.Scopes) > 0 && h.catalog != nil {
        if err := mcp.ValidateScopes(h.db, org.ID, h.catalog, req.Scopes); err != nil {
            writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
            return
        }
    }

    // Compute scope hash for JWT claims
    var mintOpts []token.MintOptions
    if len(req.Scopes) > 0 {
        scopeHash, err := mcp.ScopeHash(req.Scopes)
        if err != nil {
            writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to compute scope hash"})
            return
        }
        mintOpts = append(mintOpts, token.MintOptions{ScopeHash: scopeHash})
    }

    // Mint the JWT
    tokenStr, jti, err := token.Mint(h.signingKey, org.ID.String(), cred.ID.String(), ttl, mintOpts...)
    
    // Store token with scopes
    tokenRecord := model.Token{
        ID:        uuid.New(),
        OrgID:     org.ID,
        JTI:       jti,
        Scopes:    scopesJSON,  // Stored as JSONB
        // ... other fields ...
    }
    h.db.Create(&tokenRecord)

    resp := mintTokenResponse{
        Token:     "ptok_" + tokenStr,
        ExpiresAt: expiresAt.Format(time.RFC3339),
        JTI:       jti,
        MCPEndpoint: h.mcpBaseURL + "/" + jti,  // MCP endpoint if scopes present
    }
}
```

## Best Practices

### Principle of Least Privilege

1. **Grant only needed actions** - Don't include `read` if only `write` is needed
2. **Constrain resources** - Always specify resource IDs when possible
3. **Short TTL** - Use the minimum viable token lifetime
4. **Single connection per scope** - Don't mix multiple integrations in one token

### Example: Minimal Slack Scope

```json
{
  "connection_id": "...",
  "actions": ["slack.post_message"],
  "resources": {
    "channel": ["C1234567890"]
  }
}
```

Instead of:

```json
{
  "connection_id": "...",
  "actions": ["slack.*"],  // ❌ Wildcards not allowed
  "resources": {}          // ❌ Unconstrained resources
}
```

### Scope Testing

Test scopes before production use:

```bash
# 1. Create a test token
export TOKEN=$(curl -s -X POST https://api.llmvault.dev/v1/tokens \
  -H "Authorization: Bearer $API_KEY" \
  -d '{"credential_id": "...", "ttl": "5m", "scopes": [...]}' \
  | jq -r '.token')

# 2. Test MCP endpoint
curl -X POST https://api.llmvault.dev/mcp/$JTI \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"action": "slack.post_message", ...}'
```

## Security Considerations

### Scope Injection Attacks

Scopes are validated at:
1. **Mint time**: Catalog validation rejects unknown actions
2. **Runtime**: JTI matching prevents token replay to different endpoints
3. **Hash verification**: Scope hash in JWT prevents tampering

### Resource Enumeration

Even with scopes, resource IDs may be enumerable. Ensure:
- Resource IDs are not sequential
- Resource access is logged
- Rate limiting is applied per token

### Token Leakage

If a scoped token is leaked:
1. **Revoke immediately**: `DELETE /v1/tokens/{jti}`
2. **Scope limits damage**: Attacker can only perform scoped actions
3. **TTL limits window**: Token expires automatically
4. **Audit trail**: All actions are logged with token JTI
