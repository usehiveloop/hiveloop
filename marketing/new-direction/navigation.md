# ZiraLoop — Navigation Plan (New Direction)

---

## Core Concept

ZiraLoop has two modes: **App** and **Platform**. They share the same dashboard shell (sidebar, org selector, user menu) but render completely different navigation, pages, and hit different API endpoints.

- **App mode** (default) — A developer using ZiraLoop directly. They connect their own accounts, store their own LLM keys, get MCP servers for their own agents.
- **Platform mode** — A startup building on ZiraLoop. They install their own OAuth apps, embed the Connect Widget, mint scoped tokens for their customers' sandboxed agents.

The switch lives in the **user profile menu** (bottom of sidebar or top-right avatar dropdown). No interruption, no onboarding gate. Toggle at any time, persisted to user preferences.

---

## The Switch

Located in the user profile/avatar menu alongside "Settings," "Sign out," etc.:

```
┌──────────────────────────────┐
│  Kalu Agu                    │
│  kalu@agentdesk.com          │
│  ────────────────────────────│
│  Switch to Platform     →    │
│  ────────────────────────────│
│  Settings                    │
│  Sign out                    │
└──────────────────────────────┘
```

When in Platform mode, the menu shows "Switch to App →" instead.

The switch:
- Persists to user preferences (database, not just localStorage — survives across devices)
- Can also be stored at the org level if the org should default to one mode
- Does NOT gate route access — all routes are always accessible by URL. The switch only controls which sidebar navigation renders.
- Auto-switches to Platform when certain platform actions are detected (create API key, create integration via API, create connect session)

---

## App Mode Navigation

**For developers using ZiraLoop directly.**

```
Home

MY CONNECTIONS
  Connections
  LLM Keys

LOGS
  Generations

Settings
```

5 nav items. Dead simple. A developer signs up, connects GitHub, gets an MCP URL, done.

### Code

```typescript
const appNav: NavSection[] = [
  {
    items: [
      { label: "Home", icon: LayoutDashboard, href: "/dashboard" },
    ],
  },
  {
    title: "My Connections",
    items: [
      { label: "Connections", icon: Unplug, href: "/dashboard/connections" },
      { label: "LLM Keys", icon: KeyRound, href: "/dashboard/llm-keys" },
    ],
  },
  {
    title: "Logs",
    items: [
      { label: "Generations", icon: Sparkles, href: "/dashboard/generations" },
    ],
  },
  {
    items: [
      { label: "Settings", icon: Settings, href: "/dashboard/settings" },
    ],
  },
];
```

### Routes

```
/dashboard                          Home (personal usage stats, recent activity)
/dashboard/connections              My connected apps (MCP URLs front and center)
/dashboard/connections/new          Browse ZiraLoop's apps, start OAuth flow
/dashboard/connections/[id]         Connection detail: MCP URL, scopes, resources, activity
/dashboard/llm-keys                 My stored LLM API keys
/dashboard/llm-keys/new             Add a new LLM key (provider picker, key input, validation)
/dashboard/llm-keys/[id]            LLM key detail: proxy token, usage, status
/dashboard/generations              My agent activity log (per-request LLM usage)
/dashboard/generations/[id]         Generation detail
/dashboard/settings                 Account settings
/dashboard/settings/team            Team
/dashboard/settings/billing         Billing
```

### API Endpoints (New — App Mode)

These are **new endpoints** separate from the existing platform endpoints. They are simpler, opinionated, and handle the full flow (OAuth + token minting + MCP server creation) in fewer calls.

```
GET    /v1/app/providers                  List available apps to connect (ZiraLoop's pre-installed OAuth apps)
GET    /v1/app/providers/{id}             Get app detail (actions, resources, auth mode)

POST   /v1/app/connections                Start a new connection (initiates OAuth or accepts API key)
GET    /v1/app/connections                List my connections (with MCP URLs)
GET    /v1/app/connections/{id}           Get connection detail (MCP URL, scopes, resources, token info)
PATCH  /v1/app/connections/{id}           Update scopes/resources (regenerates MCP token)
DELETE /v1/app/connections/{id}           Disconnect (revoke connection + token)

POST   /v1/app/connections/{id}/regenerate   Regenerate MCP token (new URL, old one invalidated)

POST   /v1/app/llm-keys                  Store an LLM API key (validates, encrypts, auto-mints proxy token)
GET    /v1/app/llm-keys                   List my LLM keys
GET    /v1/app/llm-keys/{id}              Get LLM key detail (proxy token, usage)
DELETE /v1/app/llm-keys/{id}              Revoke LLM key
POST   /v1/app/llm-keys/{id}/verify       Re-verify key against provider

GET    /v1/app/generations                 List my generations (filterable)
GET    /v1/app/generations/{id}            Get generation detail

GET    /v1/app/usage                       My usage summary (requests, tokens, cost)
```

**Key differences from platform endpoints:**

| Concern | App endpoints (`/v1/app/*`) | Platform endpoints (`/v1/*`) |
|---|---|---|
| Auth | JWT (user session) | API key or JWT |
| Identity | Implicit (the logged-in user) | Explicit (identity_id parameter) |
| OAuth apps | ZiraLoop's pre-installed apps | Builder's own installed apps |
| MCP token | Auto-minted on connect | Manually minted via `POST /v1/tokens` |
| Scopes | Configured via UI, stored on connection | Passed at token mint time |
| Complexity | 3 calls to go from zero to MCP URL | 5+ calls across integrations/connections/tokens |

---

## Platform Mode Navigation

**For startups building on ZiraLoop.**

```
Home

APPS
  Installed Apps
  App Catalog

CUSTOMERS
  Connections
  Identities

CONNECT WIDGET
  Appearance
  Sessions

ACCESS
  LLM Keys
  Tokens
  API Keys

LOGS
  Generations
  Audit Log

Settings
```

14 nav items. Full power for platform builders.

### Code

```typescript
const platformNav: NavSection[] = [
  {
    items: [
      { label: "Home", icon: LayoutDashboard, href: "/dashboard" },
    ],
  },
  {
    title: "Apps",
    items: [
      { label: "Installed Apps", icon: Cable, href: "/dashboard/apps" },
      { label: "App Catalog", icon: LayoutGrid, href: "/dashboard/apps/catalog" },
    ],
  },
  {
    title: "Customers",
    items: [
      { label: "Connections", icon: Unplug, href: "/dashboard/customer-connections" },
      { label: "Identities", icon: Users, href: "/dashboard/identities" },
    ],
  },
  {
    title: "Connect Widget",
    items: [
      { label: "Appearance", icon: Palette, href: "/dashboard/widget" },
      { label: "Sessions", icon: Timer, href: "/dashboard/widget/sessions" },
    ],
  },
  {
    title: "Access",
    items: [
      { label: "LLM Keys", icon: KeyRound, href: "/dashboard/llm-keys" },
      { label: "Tokens", icon: Coins, href: "/dashboard/tokens" },
      { label: "API Keys", icon: Key, href: "/dashboard/api-keys" },
    ],
  },
  {
    title: "Logs",
    items: [
      { label: "Generations", icon: Sparkles, href: "/dashboard/generations" },
      { label: "Audit Log", icon: Clock, href: "/dashboard/audit-log" },
    ],
  },
  {
    items: [
      { label: "Settings", icon: Settings, href: "/dashboard/settings" },
    ],
  },
];
```

### Routes

```
/dashboard                              Home (org-wide stats, top customers, cost trends)

/dashboard/apps                         Installed Apps (your OAuth apps)
/dashboard/apps/catalog                 App Catalog (browse 250+ providers)
/dashboard/apps/[id]                    App detail (config, customer connections, actions)

/dashboard/customer-connections         Customer Connections (across all apps)
/dashboard/customer-connections/[id]    Customer connection detail

/dashboard/identities                   Identities (your end-users)
/dashboard/identities/[id]             Identity detail (connections, rate limits, usage)

/dashboard/widget                       Connect Widget appearance (theming, preview)
/dashboard/widget/sessions              Widget sessions

/dashboard/llm-keys                     LLM Keys (customer credentials, BYOK)
/dashboard/llm-keys/[id]               LLM Key detail

/dashboard/tokens                       Tokens (proxy + MCP scoped tokens)
/dashboard/api-keys                     API Keys (management API keys)

/dashboard/generations                  Generations (all customer LLM usage)
/dashboard/generations/[id]             Generation detail
/dashboard/audit-log                    Audit Log

/dashboard/settings                     Settings
/dashboard/settings/team                Team
/dashboard/settings/billing             Billing
```

### API Endpoints (Existing — Platform Mode)

Platform mode uses the **existing** ZiraLoop API endpoints:

```
# Integrations (your OAuth apps)
POST   /v1/integrations                   Install an app (your OAuth credentials)
GET    /v1/integrations                   List installed apps
GET    /v1/integrations/{id}              Get app detail
PUT    /v1/integrations/{id}              Update app credentials
DELETE /v1/integrations/{id}              Uninstall app
GET    /v1/integrations/providers         List available providers (Nango catalog)

# Connections (your customers' connections)
POST   /v1/integrations/{id}/connections  Create connection
GET    /v1/integrations/{id}/connections  List connections for an app
GET    /v1/connections/{id}               Get connection detail
DELETE /v1/connections/{id}               Revoke connection
GET    /v1/connections/available-scopes   List all connections with catalog actions
/v1/connections/{id}/proxy/*              Proxy to upstream via Nango

# Credentials (customer LLM keys)
POST   /v1/credentials                   Store encrypted LLM key
GET    /v1/credentials                   List credentials
GET    /v1/credentials/{id}              Get credential
DELETE /v1/credentials/{id}              Revoke credential

# Tokens
POST   /v1/tokens                        Mint scoped proxy/MCP token
GET    /v1/tokens                        List tokens
DELETE /v1/tokens/{jti}                  Revoke token

# Identities
POST   /v1/identities                    Create identity
GET    /v1/identities                    List identities
GET    /v1/identities/{id}              Get identity
PUT    /v1/identities/{id}              Update identity
DELETE /v1/identities/{id}              Delete identity

# API Keys
POST   /v1/api-keys                      Create API key
GET    /v1/api-keys                      List API keys
DELETE /v1/api-keys/{id}                 Revoke API key

# Connect Sessions
POST   /v1/connect/sessions              Create widget session

# Widget API
GET    /v1/widget/session                Session info
POST   /v1/widget/connections            Create credential via widget
GET    /v1/widget/connections            List widget connections
...                                      (all existing widget endpoints)

# Observability
GET    /v1/usage                         Org usage dashboard
GET    /v1/generations                   List generations
GET    /v1/generations/{id}              Get generation
GET    /v1/reporting                     Aggregated analytics
GET    /v1/audit                         Audit log

# Proxy
* /v1/proxy/*                            LLM streaming proxy

# MCP
/{jti}/*                                 Scoped MCP server (separate port)

# Discovery (public)
GET    /v1/providers                     LLM provider catalog
GET    /v1/providers/{id}                Provider detail
GET    /v1/providers/{id}/models         Provider models
GET    /v1/catalog/integrations          Integration catalog
GET    /v1/catalog/integrations/{id}     Integration actions
```

---

## Shared vs. Separate

### Shared Across Both Modes

These routes and pages are the same regardless of mode:

| Route | Page | Reason |
|---|---|---|
| `/dashboard` | Home | Both modes need an overview (content differs based on mode) |
| `/dashboard/llm-keys` | LLM Keys | Same page, but App mode scopes to the user's own keys, Platform mode shows all org keys |
| `/dashboard/generations` | Generations | Same page, App mode filters to user's own, Platform mode shows all |
| `/dashboard/settings` | Settings | Always the same |

### App Mode Only

| Route | Page | API |
|---|---|---|
| `/dashboard/connections` | My Connections | `GET /v1/app/connections` |
| `/dashboard/connections/new` | New Connection | `GET /v1/app/providers`, `POST /v1/app/connections` |
| `/dashboard/connections/[id]` | Connection Detail | `GET /v1/app/connections/{id}` |

### Platform Mode Only

| Route | Page | API |
|---|---|---|
| `/dashboard/apps` | Installed Apps | `GET /v1/integrations` |
| `/dashboard/apps/catalog` | App Catalog | `GET /v1/integrations/providers` |
| `/dashboard/apps/[id]` | App Detail | `GET /v1/integrations/{id}` |
| `/dashboard/customer-connections` | Customer Connections | `GET /v1/integrations/{id}/connections` |
| `/dashboard/customer-connections/[id]` | Connection Detail | `GET /v1/connections/{id}` |
| `/dashboard/identities` | Identities | `GET /v1/identities` |
| `/dashboard/identities/[id]` | Identity Detail | `GET /v1/identities/{id}` |
| `/dashboard/widget` | Connect Widget | `GET /v1/settings/connect` |
| `/dashboard/widget/sessions` | Sessions | `POST /v1/connect/sessions` |
| `/dashboard/tokens` | Tokens | `GET /v1/tokens` |
| `/dashboard/api-keys` | API Keys | `GET /v1/api-keys` |
| `/dashboard/audit-log` | Audit Log | `GET /v1/audit` |

---

## New App Endpoints — Design

### `GET /v1/app/providers`

Returns ZiraLoop's pre-installed OAuth apps available for the user to connect. These are apps where ZiraLoop owns the OAuth credentials.

```json
[
  {
    "id": "github",
    "name": "GitHub",
    "auth_mode": "OAUTH2",
    "actions_count": 8,
    "resources": ["repo"],
    "description": "Access repositories, issues, pull requests"
  },
  {
    "id": "slack",
    "name": "Slack",
    "auth_mode": "OAUTH2",
    "actions_count": 12,
    "resources": ["channel"],
    "description": "Send messages, manage channels, read history"
  }
]
```

### `POST /v1/app/connections`

Starts a connection. For OAuth providers, returns a redirect URL. For API key providers, accepts the key directly.

**OAuth flow:**
```json
POST /v1/app/connections
{ "provider": "github" }

→ 200
{
  "connection_id": "conn_abc123",
  "status": "pending",
  "oauth_redirect_url": "https://github.com/login/oauth/authorize?client_id=..."
}
```

After OAuth callback, ZiraLoop:
1. Stores the connection
2. Auto-mints a scoped MCP token with all available actions
3. Sets status to "active"

**API key flow (for LLM keys created from App mode):**
```json
POST /v1/app/connections
{
  "provider": "openai",
  "api_key": "sk-..."
}

→ 201
{
  "connection_id": "conn_def456",
  "status": "active",
  "proxy_token": "ptok_eyJhbG...",
  "proxy_url": "https://api.ziraloop.com/v1/proxy"
}
```

### `GET /v1/app/connections`

Returns the user's connections with MCP/proxy URLs.

```json
[
  {
    "id": "conn_abc123",
    "provider": "github",
    "provider_name": "GitHub",
    "status": "active",
    "account": "@bahdcoder",
    "mcp_url": "https://mcp.ziraloop.com/tok_xyz789",
    "actions": ["create_issue", "list_issues", "create_pr", "list_prs"],
    "resources": {
      "repo": [
        { "id": "org/frontend", "name": "org/frontend" },
        { "id": "org/api", "name": "org/api" }
      ]
    },
    "token_expires_at": "2026-04-28T00:00:00Z",
    "created_at": "2026-03-29T10:00:00Z"
  },
  {
    "id": "conn_def456",
    "provider": "openai",
    "provider_name": "OpenAI",
    "status": "active",
    "proxy_token": "ptok_eyJhbG...",
    "proxy_url": "https://api.ziraloop.com/v1/proxy",
    "created_at": "2026-03-29T11:00:00Z"
  }
]
```

### `PATCH /v1/app/connections/{id}`

Update which actions and resources the MCP server exposes. Regenerates the token automatically.

```json
PATCH /v1/app/connections/conn_abc123
{
  "actions": ["create_issue", "list_issues"],
  "resources": {
    "repo": ["org/frontend"]
  }
}

→ 200
{
  "id": "conn_abc123",
  "mcp_url": "https://mcp.ziraloop.com/tok_newtoken",
  "actions": ["create_issue", "list_issues"],
  "resources": { "repo": [{ "id": "org/frontend", "name": "org/frontend" }] }
}
```

### `POST /v1/app/connections/{id}/regenerate`

Mint a fresh MCP token. Old URL stops working immediately.

```json
POST /v1/app/connections/conn_abc123/regenerate

→ 200
{
  "mcp_url": "https://mcp.ziraloop.com/tok_freshtoken",
  "token_expires_at": "2026-04-28T00:00:00Z"
}
```

---

## Home Page — Different Content Per Mode

### App Mode Home

Personal usage dashboard:

```
Welcome back, Kalu.

  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐
  │ 3            │ │ 2           │ │ 1,247       │
  │ Connections  │ │ LLM Keys    │ │ Requests    │
  └─────────────┘ └─────────────┘ └─────────────┘

  Recent Activity
  ┌─────────────────────────────────────────────┐
  │  2m ago  gpt-4o       1,247 tokens   $0.04  │
  │  5m ago  GitHub       create_issue   200    │
  │  8m ago  claude-4     3,891 tokens   $0.12  │
  │  12m ago Slack        send_message   200    │
  └─────────────────────────────────────────────┘
```

### Platform Mode Home

Org-wide dashboard:

```
AgentDesk Platform

  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐ ┌─────────────┐
  │ 3            │ │ 47          │ │ 23          │ │ 12,847      │
  │ Apps         │ │ Connections │ │ Identities  │ │ Requests    │
  └─────────────┘ └─────────────┘ └─────────────┘ └─────────────┘

  Requests (last 30 days)
  ┌─────────────────────────────────────────────┐
  │  ▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇        │
  └─────────────────────────────────────────────┘

  Top Models          Top Customers       Cost
  gpt-4o    4,231     user_42   1,892    $142.30
  claude-4  3,891     user_87     891    Today
  gemini-2  1,567     user_15     567
```

---

## Backend Requirements

### New Endpoints to Build

| Endpoint | Priority | Description |
|---|---|---|
| `GET /v1/app/providers` | **P0** | List ZiraLoop's pre-installed apps available for connection |
| `POST /v1/app/connections` | **P0** | Start OAuth flow or accept API key, auto-mint token |
| `GET /v1/app/connections` | **P0** | List user's connections with MCP/proxy URLs |
| `GET /v1/app/connections/{id}` | **P0** | Connection detail |
| `PATCH /v1/app/connections/{id}` | **P1** | Update scopes/resources, regenerate token |
| `DELETE /v1/app/connections/{id}` | **P0** | Disconnect and revoke |
| `POST /v1/app/connections/{id}/regenerate` | **P1** | Regenerate MCP token |
| `POST /v1/app/llm-keys` | **P0** | Store LLM key (validate, encrypt, auto-mint proxy token) |
| `GET /v1/app/llm-keys` | **P0** | List user's LLM keys |
| `GET /v1/app/llm-keys/{id}` | **P0** | Key detail |
| `DELETE /v1/app/llm-keys/{id}` | **P0** | Revoke |
| `POST /v1/app/llm-keys/{id}/verify` | **P1** | Re-verify against provider |
| `GET /v1/app/generations` | **P1** | List user's generations |
| `GET /v1/app/generations/{id}` | **P1** | Generation detail |
| `GET /v1/app/usage` | **P1** | User's personal usage summary |

### Infrastructure Requirements

| Requirement | Priority | Description |
|---|---|---|
| ZiraLoop's own OAuth apps | **P0** | Register ZiraLoop as an OAuth app on GitHub, Slack, Linear, Notion, etc. Store credentials as system-level integrations (not org-level). |
| OAuth callback handling | **P0** | `/v1/app/connections` needs to handle the OAuth redirect → callback → connection creation → auto-token-mint flow. |
| User preference storage | **P0** | Store "app vs platform" mode preference per user. Expose via `GET /v1/app/preferences` or include in session/user object. |
| Auto-switch heuristic | **P2** | Detect platform actions (create API key, create integration, create connect session) and suggest switching to Platform mode. |

### New Frontend Pages to Build

| Page | Route | Mode | Priority |
|---|---|---|---|
| My Connections | `/dashboard/connections` | App | **P0** |
| New Connection | `/dashboard/connections/new` | App | **P0** |
| Connection Detail | `/dashboard/connections/[id]` | App | **P0** |
| LLM Keys (shared) | `/dashboard/llm-keys` | Both | **P0** (rename from `/dashboard/credentials`) |
| Generations (shared) | `/dashboard/generations` | Both | **P1** |
| Generation Detail | `/dashboard/generations/[id]` | Both | **P1** |
| App Catalog | `/dashboard/apps/catalog` | Platform | **P1** |
| Customer Connections | `/dashboard/customer-connections` | Platform | **P1** |
| Widget Appearance | `/dashboard/widget` | Platform | **P1** (rename from `/dashboard/connect`) |
| Widget Sessions | `/dashboard/widget/sessions` | Platform | **P1** (rename from `/dashboard/connect/sessions`) |

### Renamed Pages

| Current Route | New Route | Current Nav Label | New Nav Label |
|---|---|---|---|
| `/dashboard/credentials` | `/dashboard/llm-keys` | Credentials | LLM Keys |
| `/dashboard/integrations` | `/dashboard/apps` | Integrations | Installed Apps |
| `/dashboard/connect` | `/dashboard/widget` | Connect UI | Appearance |
| `/dashboard/connect/sessions` | `/dashboard/widget/sessions` | Sessions | Sessions |

---

## Frontend Architecture

### Mode Detection

```typescript
// hooks/useMode.ts
import { useUser } from "@/hooks/useUser";

type Mode = "app" | "platform";

export function useMode(): [Mode, (mode: Mode) => void] {
  const { user, updatePreferences } = useUser();
  const mode = user?.preferences?.mode ?? "app";

  const setMode = (newMode: Mode) => {
    updatePreferences({ mode: newMode });
  };

  return [mode, setMode];
}
```

### Nav Rendering

```typescript
// dashboard-shell.tsx
function Sidebar() {
  const [mode] = useMode();
  const sections = mode === "app" ? appNav : platformNav;

  return (
    <nav>
      {sections.map((section) => (
        <NavSection key={section.title} {...section} />
      ))}
    </nav>
  );
}
```

### API Client Prefix

```typescript
// api/client.ts
// App mode endpoints use /v1/app/* prefix
// Platform mode endpoints use /v1/* prefix (existing)

export function useConnections(mode: Mode) {
  if (mode === "app") {
    return $api.useQuery("get", "/v1/app/connections");
  }
  // Platform mode: aggregate across integrations
  return $api.useQuery("get", "/v1/connections/available-scopes");
}
```

### Route Guards

Routes are never blocked. If a user navigates to a platform route while in app mode, the page renders normally. An optional banner at the top says:

```
┌──────────────────────────────────────────────────────────┐
│  ℹ This page is part of Platform mode.  [Switch now]     │
└──────────────────────────────────────────────────────────┘
```

---

## Summary

| Aspect | App Mode | Platform Mode |
|---|---|---|
| **Default** | Yes | No (opt-in) |
| **Target user** | Developer using ZiraLoop directly | Startup building on ZiraLoop |
| **Nav items** | 5 | 14 |
| **API prefix** | `/v1/app/*` (new) | `/v1/*` (existing) |
| **OAuth apps** | ZiraLoop's pre-installed | Builder's own installed |
| **Identity** | Implicit (logged-in user) | Explicit (identity_id parameter) |
| **MCP tokens** | Auto-minted on connect | Manually minted via API |
| **Customers concept** | No | Yes (Connections, Identities) |
| **Connect Widget** | No | Yes |
| **API Keys** | No | Yes |
| **Audit Log** | No | Yes |
| **Switch location** | User profile menu | User profile menu |
