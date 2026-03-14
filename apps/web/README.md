# LLMVault Web App

Next.js application for LLMVault with authentication via ZITADEL.

## Environment Setup

### Option 1: Local Development (Docker ZITADEL)

```bash
# Uses local ZITADEL from docker-compose
cp .env.local.example .env.local
# Fill in ZITADEL_CLIENT_ID from zitadel-init logs
```

### Option 2: Staging/Development (AWS ZITADEL) ⭐

```bash
# Use staging ZITADEL (auth.dev.llmvault.dev)
cp .env.staging .env.local

# Get ZITADEL_CLIENT_ID from staging Zitadel:
# 1. Go to https://auth.dev.llmvault.dev/ui/console
# 2. Login with admin credentials
# 3. Find or create the web app OIDC client
# 4. Copy the Client ID to .env.local
```

**Staging Environment Values:**
| Variable | Value |
|----------|-------|
| `ZITADEL_DOMAIN` | `https://auth.dev.llmvault.dev` |
| `NEXT_PUBLIC_API_URL` | `https://api.dev.llmvault.dev` |

## Getting Started

```bash
# Install dependencies
npm install

# Run dev server
npm run dev
```

Open [http://localhost:30112](http://localhost:30112)

## Getting ZITADEL Client ID

### For Staging (auth.dev.llmvault.dev)

1. Visit: https://auth.dev.llmvault.dev/ui/console
2. Login with your admin user
3. Go to your organization → Projects
4. Find "LLMVault" project (or create one)
5. Go to "Apps" → Click "New" or find existing web app
6. Copy the **Client ID**
7. Add it to `.env.local`:
   ```
   ZITADEL_CLIENT_ID=your-client-id-here
   ```

### Required OIDC Settings

| Setting | Value |
|---------|-------|
| Redirect URIs | `http://localhost:30112/api/auth/callback/zitadel` |
| Post Logout URIs | `http://localhost:30112` |
| Response Type | `CODE` |
| Grant Types | `AUTHORIZATION_CODE`, `REFRESH_TOKEN` |
| Authentication Method | `NONE` (PKCE) |

## Switching Between Environments

```bash
# Switch to local ZITADEL
cp .env.local .env.staging.backup  # backup staging
cp .env.local.backup .env.local     # restore local

# Switch to staging ZITADEL
cp .env.local .env.local.backup     # backup local
cp .env.staging .env.local          # use staging
```

## Troubleshooting

### "Invalid client_id"
- Check `ZITADEL_CLIENT_ID` matches the app in Zitadel console
- Verify you're using the correct environment (local vs staging)

### "Invalid redirect_uri"
- Ensure redirect URI in Ziteld console matches `NEXTAUTH_URL/api/auth/callback/zitadel`
- For staging: `http://localhost:30112/api/auth/callback/zitadel`

### CORS errors
- Add `http://localhost:30112` to allowed origins in Zitadel app settings
