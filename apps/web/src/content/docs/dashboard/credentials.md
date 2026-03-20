---
title: Managing Credentials
description: Creating, viewing, and managing LLM provider credentials in the dashboard
---

# Managing Credentials

Credentials store encrypted API keys for LLM providers. They serve as the foundation for minting proxy tokens and routing requests.

## Credentials List

Navigate to **Security > Credentials** to view all credentials.

### List View

The credentials table displays:

| Column | Description |
|--------|-------------|
| Credential | Label and truncated ID |
| Provider | Provider badge (OpenAI, Anthropic, etc.) |
| Status | Active, Revoked, or Expiring |
| Remaining | Request cap progress bar |
| Identity | Linked identity ID (if any) |
| Last Used | Relative timestamp |
| Requests | Total request count |

### Searching

Use the search box to filter by:
- Credential label
- Credential ID
- Provider name

Search is case-insensitive and matches partial strings.

### Pagination

Credentials are paginated (20 per page):
- Click **Next** / **Previous** to navigate
- Page number displayed below table

### Empty State

When no credentials exist:
- Icon and descriptive message displayed
- **"New Credential"** button to create first credential

## Creating a Credential

### Step 1: Select Provider

Click **"New Credential"** to open the creation dialog:

1. Browse available LLM providers
2. Search by provider name
3. Click a provider to continue

Supported providers include:
- OpenAI
- Anthropic
- Google AI (Gemini)
- Azure OpenAI
- Groq
- Together AI
- Cohere
- Mistral AI
- And more

### Step 2: Configure Credential

**Required Fields:**

| Field | Description | Example |
|-------|-------------|---------|
| Label | Human-readable name | "Production OpenAI" |
| Base URL | Provider API endpoint | `https://api.openai.com/v1` |
| Auth Scheme | Authentication method | Bearer, X-API-Key, API-Key, Query Param |
| API Key | Provider API key | `sk-...` |

**Optional Fields:**

| Field | Description | Example |
|-------|-------------|---------|
| Remaining | Request cap limit | `10000` |
| Refill Amount | Requests to add on refill | `10000` |
| Refill Interval | Duration between refills | `24h` |

### Auth Schemes

Available authentication methods:
- **Bearer** - `Authorization: Bearer <token>`
- **X-API-Key** - `X-API-Key: <key>`
- **API-Key** - `API-Key: <key>`
- **Query Param** - Appended to URL as query parameter

Click **"Create Credential"** to save. The API key is encrypted at rest using AES-256-GCM.

## Credential Detail Page

Click any credential row to view details.

### Configuration Card

Displays read-only configuration:
- Base URL
- Auth Scheme
- Provider
- Identity (linked user, if any)
- Created timestamp
- Revoked timestamp (if applicable)
- Credential ID

### Usage Card

Shows current consumption:
- Large counter showing remaining requests
- Progress bar visualizing usage
- Refill configuration (if set)
- Total request count
- Last used timestamp

### Minted Tokens Section

Lists all tokens minted from this credential:
- JTI (token identifier)
- Status (Active, Revoked, Expiring)
- Remaining requests
- Expiration time
- Creation time

Click **"View all tokens"** to see the full tokens list.

### Metadata Section

If the credential has metadata attached, it displays as formatted JSON:
```json
{
  "environment": "production",
  "team": "ai-platform"
}
```

## Revoking a Credential

To revoke a credential:

1. Open the credential detail page
2. Click the **"Revoke"** button (destructive action)
3. The credential is immediately revoked
4. All associated tokens become invalid

**Effects of revocation:**
- Credential status changes to "Revoked"
- No new tokens can be minted
- Existing tokens fail authentication
- Cache is invalidated across all tiers

## Credential Statuses

| Status | Description |
|--------|-------------|
| Active | Normal operation, requests allowed |
| Revoked | Disabled, no requests allowed |
| Expiring | Remaining requests at or below zero |

## Best Practices

1. **Use descriptive labels** - Include environment and purpose
2. **Set request caps** - Protect against runaway usage
3. **Configure refills** - Automatically replenish quotas
4. **Link identities** - Track per-user usage
5. **Revoke unused credentials** - Regular security hygiene
