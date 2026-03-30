# Auth — One Connect Flow. Every Credential.

> The Plaid Link experience for AI agent credentials. Drop in the widget, connect any provider.

---

## The Problem

Every platform building AI features needs a "Connect Your Provider" flow. Today, teams build this from scratch — custom OAuth integrations, API key input forms, token refresh logic, provider-specific auth handling — for every single provider. It takes weeks per integration, and the result is fragile, inconsistent, and insecure.

The auth landscape for AI agents is fragmented across fundamentally different protocols:
- LLM providers use API keys (OpenAI: Bearer token, Anthropic: x-api-key header, Google: query parameter)
- SaaS tools use OAuth 2.0, OAuth 1.0a, API keys, app installations, webhook secrets
- Each provider has different setup steps, different scopes, different token refresh behavior

Platforms end up with spaghetti auth code, keys stored in plaintext, and broken integrations when providers change their flows.

### Industry Context

- Plaid Link — the embeddable widget for financial account connections — proved that a drop-in connect flow can become the standard for an entire industry. OAuth support is now required in all Plaid integrations for US, EU, and UK institutions. ([Plaid Docs](https://plaid.com/docs/link/))
- **88% of organizations** reported AI agent security incidents in the past year, many traced to ad-hoc credential management ([Gravitee 2026](https://www.gravitee.io/blog/state-of-ai-agent-security-2026-report-when-adoption-outpaces-control))
- **Only 14.4%** of AI agents went live with full security/IT approval — the rest used custom, ungoverned auth ([Gravitee 2026](https://www.gravitee.io/blog/state-of-ai-agent-security-2026-report-when-adoption-outpaces-control))
- **27.2%** of organizations have reverted to custom, hardcoded authorization logic for agent connections ([Gravitee 2026](https://www.gravitee.io/blog/state-of-ai-agent-security-2026-report-when-adoption-outpaces-control))

---

## The Solution

LLMVault Auth is an embeddable connect widget and session API that handles the entire "Connect Your Provider" flow — for both LLM providers (API keys) and SaaS apps (OAuth) — in a single, drop-in component.

---

## How It Works

### 1. Create a Session

Your backend creates a scoped connect session for your end-user:

```bash
POST /v1/connect/sessions
{
  "external_id": "user_42",
  "allowed_integrations": ["slack-abc12345", "github-def67890"],
  "permissions": ["create", "list", "delete", "verify"],
  "allowed_origins": ["https://app.yourplatform.com"],
  "ttl": "15m"
}
```

The session controls exactly what the widget can do:
- **`allowed_integrations`** — restrict which integrations are shown (validated against your org's configured integrations)
- **`permissions`** — grant create, list, delete, verify capabilities individually
- **`allowed_origins`** — CORS enforcement, validated as a subset of org-level allowed origins
- **`ttl`** — session auto-expires (max 30 minutes, default 15 minutes)
- **`external_id`** — auto-upserts an identity for the end-user, linking all their connections

### 2. Embed the Widget

Drop the React component into your app:

```jsx
import { LLMVaultConnect } from '@llmvault/react'

<LLMVaultConnect
  sessionToken="sess_..."
  onConnect={(connection) => console.log('Connected:', connection.id)}
  onDisconnect={(connection) => console.log('Disconnected:', connection.id)}
  theme={{ primaryColor: '#6366f1', borderRadius: '8px' }}
/>
```

### 3. The Widget Handles Everything

**For LLM providers (API key flow):**
- Provider selection with logos and model counts (101 providers, 3,183 models)
- API key input with real-time validation against the provider
- Key encryption with envelope encryption (AES-256-GCM + KMS) before storage
- Connection status indicator
- Disconnect / reconnect flow

**For SaaS integrations (OAuth flow):**
- Integration provider selection
- OAuth redirect handling (via Nango)
- Automatic token refresh before expiration
- Resource selection — pick specific Slack channels, GitHub repos, Jira projects
- Connection management and revocation

---

## Full API Surface

### Connect Sessions
| Endpoint | Description |
|---|---|
| `POST /v1/connect/sessions` | Create a scoped widget session. Requires `identity_id` or `external_id`. Validates allowed_integrations against org integrations, origins against org-level allowed_origins. |

### Widget Endpoints (Session Token Auth)
| Endpoint | Description |
|---|---|
| `GET /v1/widget/session` | Get current session info (id, identity, permissions, expires_at) |
| `GET /v1/widget/providers` | List all 101 LLM providers with model counts |
| `POST /v1/widget/connections` | Create credential — validates API key against provider, encrypts, stores |
| `GET /v1/widget/connections` | List credential connections for session identity |
| `DELETE /v1/widget/connections/{id}` | Revoke credential connection |
| `POST /v1/widget/connections/{id}/verify` | Re-verify stored credential against provider |
| `GET /v1/widget/integrations/providers` | List available OAuth providers |
| `GET /v1/widget/integrations` | List configured integrations |
| `POST /v1/widget/integrations/{id}/connect-session` | Start OAuth flow via Nango |
| `GET /v1/widget/integrations/{id}/resources/{type}/available` | Discover available resources (channels, repos, projects) |
| `POST /v1/widget/integrations/{id}/connections` | Create integration connection after OAuth |
| `PATCH /v1/widget/integrations/{id}/connections/{connectionId}` | Update connection metadata/resources |
| `DELETE /v1/widget/integrations/{id}/connections/{connectionId}` | Revoke integration connection |

### Identity Management
| Endpoint | Description |
|---|---|
| `POST /v1/identities` | Create identity with external_id, metadata, rate limits |
| `GET /v1/identities` | List identities (filterable by external_id, meta) |
| `GET /v1/identities/{id}` | Get identity detail |
| `PUT /v1/identities/{id}` | Update identity rate limits and metadata |
| `DELETE /v1/identities/{id}` | Delete identity |

### Settings
| Endpoint | Description |
|---|---|
| `GET /v1/settings/connect` | Get org-level allowed origins for widget |
| `PUT /v1/settings/connect` | Update allowed origins |

---

## Supported Auth Modes

The widget handles every auth mode through the Nango integration layer:

| Auth Mode | Description | Example Providers |
|---|---|---|
| **OAuth 2.0** | Standard OAuth with client_id/client_secret | Slack, GitHub, HubSpot, Salesforce |
| **OAuth 1.0a** | Legacy OAuth | Twitter |
| **API Key** | Direct API key input with validation | OpenAI, Anthropic, all LLM providers |
| **Basic Auth** | Username/password | Jira Data Center |
| **APP** | App installation (app_id, app_link, private_key) | GitHub App |
| **MCP_OAUTH2** | MCP-specific OAuth flow | MCP-native providers |
| **MCP_OAUTH2_GENERIC** | Generic MCP OAuth (credentials optional) | Experimental MCP providers |
| **INSTALL_PLUGIN** | Plugin-based installation (app_link) | Shopify |
| **TBA** | Token-based authentication | NetSuite |
| **OAUTH2_CC** | Client credentials flow | Machine-to-machine |
| **CUSTOM** | All credential types required | Braintree |

---

## Widget Components

The Connect widget (`apps/connect/`) includes these views:

- **ProviderSelection** — Grid of LLM providers with search
- **ProviderDetail** — Provider info, setup instructions, API key input
- **ApiKeyInput** — Key input with real-time validation
- **Validating** — Loading state during key verification
- **IntegrationProviderSelection** — Grid of OAuth integration providers
- **IntegrationDetail** — Integration info and connect button
- **IntegrationAuth** — OAuth redirect handling
- **IntegrationResourceSelection** — Pick specific channels, repos, projects
- **ResourceSelectionSuccess** — Confirmation of selected resources
- **ConnectedList** — Manage all active connections
- **ConnectionCard** — Individual connection with status and actions
- **RevokeConfirm** / **IntegrationDisconnectConfirm** — Confirmation dialogs
- **Success** / **IntegrationSuccess** — Connection success states
- **SecurityCallout** — Trust indicator showing encryption details

---

## Sources

- [Plaid Link Documentation](https://plaid.com/docs/link/)
- [Gravitee — State of AI Agent Security 2026](https://www.gravitee.io/blog/state-of-ai-agent-security-2026-report-when-adoption-outpaces-control)
