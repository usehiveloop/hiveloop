---
title: Token Scoping
description: How scoped tokens limit access to specific resources and actions.
---

# Token Scoping

Token scoping allows you to create fine-grained, least-privilege access tokens that can only perform specific actions on specific resources. This is essential for security when giving LLMs access to external integrations.

## How Scopes Work

A scope defines what a token is allowed to do. Each scope targets a specific **connection** (an OAuth integration), lists the **actions** the token can perform, and optionally constrains those actions to specific **resources**.

### Scope Structure

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `connection_id` | string | Yes | UUID of the OAuth connection |
| `actions` | string[] | Yes | List of allowed actions from the provider catalog |
| `resources` | object | No | Resource type to resource IDs mapping |

### Example: Slack Scope

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
- List channels (constrained to the specified channels)

The token cannot perform any other Slack actions, and it cannot interact with any other channels.

## Creating Scoped Tokens

### Using the SDK

```typescript
import { LLMVault } from "@llmvault/sdk";
const vault = new LLMVault({ apiKey: "your-api-key" });

const { data, error } = await vault.tokens.create({
  credential_id: "550e8400-e29b-41d4-a716-446655440000",
  expires_in: "1h",
  scopes: [
    {
      connection_id: "660e8400-e29b-41d4-a716-446655440001",
      actions: ["slack.post_message", "slack.list_channels"],
      resources: {
        channel: ["C1234567890"]
      }
    }
  ]
});

// data.token     - the proxy token (ptok_...)
// data.jti       - unique token ID for revocation
// data.mcp_endpoint - MCP endpoint URL if scopes are present
```

### Using the API

```bash
curl -X POST https://api.llmvault.dev/v1/tokens \
  -H "Authorization: Bearer llmv_sk_..." \
  -H "Content-Type: application/json" \
  -d '{
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
  }'
```

**Response:**

```json
{
  "token": "ptok_eyJhbGciOi...",
  "expires_at": "2026-03-20T14:00:00Z",
  "jti": "abc123",
  "mcp_endpoint": "https://api.llmvault.dev/mcp/abc123"
}
```

## Actions

Actions represent specific operations a token can perform. They are defined per provider in the LLMVault action catalog.

### Action Properties

Each action has:

| Property | Description |
|----------|-------------|
| **Display name** | Human-readable name (e.g., "Post Message") |
| **Description** | What the action does |
| **Access type** | `read` or `write` |
| **Resource type** | The type of resource this action operates on (e.g., "channel", "repo") |
| **Parameters** | JSON Schema defining the action's input parameters |

### Example: Slack Actions

| Action | Access | Resource Type | Description |
|--------|--------|---------------|-------------|
| `slack.post_message` | write | channel | Post a message to a channel |
| `slack.list_channels` | read | channel | List accessible channels |
| `slack.add_reaction` | write | message | Add a reaction to a message |

### Wildcard Rejection

For security, wildcard actions (`"*"`) are explicitly rejected. You must list each action individually. This prevents accidental over-permissioning:

```json
// This will be rejected:
{ "actions": ["slack.*"] }

// This is correct:
{ "actions": ["slack.post_message", "slack.list_channels"] }
```

## Resources

Resources constrain actions to specific instances. For example, instead of allowing a token to post to any Slack channel, you can limit it to specific channels.

### How Resources Work

Resources are organized by type (e.g., `channel`, `repo`, `issue`). Each resource type maps to a list of allowed resource IDs:

```json
{
  "resources": {
    "channel": ["C1234567890", "C0987654321"],
    "repo": ["llmvault/llmvault", "llmvault/docs"]
  }
}
```

Resource types must match the actions in the scope. If your actions operate on channels, you can only constrain by `channel` resources.

### Example: GitHub Scope

```json
{
  "connection_id": "550e8400-e29b-41d4-a716-446655440000",
  "actions": ["github.list_issues", "github.create_issue"],
  "resources": {
    "repo": ["llmvault/llmvault", "llmvault/docs"]
  }
}
```

This scope:
- Limits repository access to two specific repos
- Allows listing and creating issues only within those repos

## MCP Integration

When a token has scopes, LLMVault generates an MCP (Model Context Protocol) endpoint for it. Each scoped action becomes an MCP tool that LLMs can invoke.

### How It Works

1. You create a token with scopes
2. The response includes an `mcp_endpoint` URL
3. Connect your LLM to this endpoint
4. The LLM sees only the tools (actions) defined in the token's scopes
5. Every tool invocation is validated against the scope constraints and resources

### Scope Integrity

Scopes are cryptographically bound to the token. When a token is minted, a SHA-256 hash of the scopes is embedded in the JWT claims. At runtime, the server validates that the stored scopes match this hash, preventing any scope tampering.

## Revoking Scoped Tokens

Revoke a token immediately when it is no longer needed:

```typescript
// Using the SDK
await vault.tokens.delete("abc123"); // Pass the JTI
```

```bash
# Using the API
curl -X DELETE https://api.llmvault.dev/v1/tokens/abc123 \
  -H "Authorization: Bearer llmv_sk_..."
```

Revocation takes effect within seconds. The token's MCP endpoint becomes immediately inaccessible.

## Best Practices

### Principle of Least Privilege

1. **Grant only needed actions** -- Do not include `read` actions if only `write` is needed, and vice versa
2. **Constrain resources** -- Always specify resource IDs when possible
3. **Short TTL** -- Use the minimum viable token lifetime (e.g., 1 hour for a single task)
4. **Single connection per scope** -- Avoid mixing multiple integrations in one token

### Minimal Scope Example

**Good** -- specific action, specific resource, short TTL:

```typescript
const { data } = await vault.tokens.create({
  credential_id: "...",
  expires_in: "30m",
  scopes: [{
    connection_id: "...",
    actions: ["slack.post_message"],
    resources: { channel: ["C1234567890"] }
  }]
});
```

**Avoid** -- broad actions, no resource constraints:

```json
{
  "actions": ["slack.post_message", "slack.list_channels", "slack.add_reaction", "slack.list_users"],
  "resources": {}
}
```

### Testing Scopes

Test scoped tokens in a development environment before production use:

```typescript
// Create a short-lived test token
const { data } = await vault.tokens.create({
  credential_id: "...",
  expires_in: "5m",
  scopes: [{
    connection_id: "...",
    actions: ["slack.post_message"],
    resources: { channel: ["C-test-channel"] }
  }]
});

// The MCP endpoint is ready to test
console.log(data.mcp_endpoint);
```

## Security Considerations

### Validation at Every Stage

Scopes are validated at multiple points:

1. **Mint time**: All actions are validated against the provider catalog. Unknown actions are rejected. Connections are verified to exist and belong to your organization.
2. **Runtime**: The token's JTI is matched against the URL to prevent replay to different endpoints.
3. **Hash verification**: The scope hash in the JWT prevents tampering with scope definitions after minting.

### If a Token Is Leaked

Scoped tokens limit blast radius by design:

1. **Revoke immediately**: Use `vault.tokens.delete(jti)` or `DELETE /v1/tokens/{jti}`
2. **Scope limits damage**: The attacker can only perform the specific actions on the specific resources defined in the scope
3. **TTL limits window**: The token expires automatically at the defined TTL
4. **Full audit trail**: All actions performed with the token are logged with its JTI for investigation

### Resource Enumeration

Even with scopes, be aware that resource IDs may be discoverable. To limit this risk:

- Use non-sequential resource identifiers where possible
- Monitor audit logs for unusual resource access patterns
- Apply rate limiting per token
