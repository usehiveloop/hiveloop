---
title: Dashboard Overview
description: Navigating the LLMVault dashboard, metrics, and quick actions
---

# Dashboard Overview

The LLMVault Dashboard provides a unified interface for managing your organization's LLM credentials, tokens, identities, integrations, and monitoring usage.

## Navigation Structure

The dashboard is organized into four main sections accessible from the left sidebar:

### Security
- **Credentials** - Store and manage encrypted LLM provider API keys
- **Tokens** - Mint short-lived proxy tokens scoped to credentials
- **API Keys** - Create and manage management API authentication keys
- **Identities** - Track end-users and enforce per-user rate limits

### Experience
- **Connect UI** - Embed authentication flows for integrations
- **Integrations** - Configure OAuth providers and third-party connections

### Manage
- **Audit Log** - View detailed request logs and activity history
- **Settings** - Manage workspace, team members, and billing

## Dashboard Home

The main dashboard page displays key metrics and quick actions:

### Metrics Overview

**Stat Cards** show real-time counts for:

| Metric | Description |
|--------|-------------|
| Active Credentials | Total credentials not revoked |
| Active Tokens | Tokens not expired or revoked |
| Identities | Total tracked end-users |
| Requests Today | Proxy requests in current day with vs-yesterday comparison |

### Request Visualization

**Requests Chart (Last 30 Days)**
- Bar chart showing daily proxy request volume
- Hover over bars to see exact counts per day
- Total count displayed in top-right corner

**Top Credentials**
- Lists top 5 credentials by request volume (last 30 days)
- Shows credential label, provider, and request count
- Click "View all" to navigate to credentials page

### Summary Row

Additional statistics displayed at the bottom:
- **Total Credentials** - All credentials including revoked
- **Total Tokens** - All tokens minted
- **Requests (7d)** - Rolling 7-day request count
- **Requests (All Time)** - Cumulative request count

## Quick Actions

From the dashboard header, click **"New Credential"** to immediately start creating a new LLM provider credential.

## Plan Usage Widget

The sidebar displays your current plan usage:

**Free Plan Limits:**
- Credentials: 15
- Proxy Requests: 10,000/month
- Identities: 500

Progress bars show current usage with color coding:
- Yellow (#EAB308) for credentials and requests
- Purple (#8B5CF6) for identities

Click **"Upgrade"** to view pricing plans.

## Workspace Switcher

The dashboard supports multiple organizations:

1. Click the workspace name in the sidebar to open the switcher
2. Select from available organizations
3. Click "Create organization" to add a new workspace

The active workspace context is persisted across sessions.

## Mobile Navigation

On mobile devices:
- Tap the hamburger menu (☰) to open the sidebar overlay
- Swipe or tap outside to close
- All navigation items remain accessible

## Data Refresh

Dashboard data automatically refreshes:
- Metrics update on page load
- Real-time stats are fetched from `/v1/usage` endpoint
- Manual refresh by navigating away and back

## Empty States

When no data exists, the dashboard shows:
- Friendly empty state illustrations
- Descriptive text explaining the feature
- Primary call-to-action button to create first resource
