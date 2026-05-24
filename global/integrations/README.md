# Global Integrations

Each `*.json` file in this directory defines one platform-managed integration.
Startup syncs enabled manifests into Nango and `integrations`.

Secrets must be referenced through environment variable names only. Do not
commit OAuth client secrets, app private keys, API keys, or webhook secrets.

`required: true` makes missing env vars or Nango sync failures fail startup.
Optional integrations are skipped when their env vars are absent.

Provider requirements checked against real Nango `/providers`:

| Manifest | Nango provider | Auth mode | Startup env refs | Scopes | End-user connection config |
| --- | --- | --- | --- | --- | --- |
| `github-app`, `github-app-code-reviews` | `github-app` | `APP` | app id, app public link, app private key, webhook secret | none | automated `appPublicLink`, automated `installation_id` |
| `linear` | `linear` | `OAUTH2` | client id, client secret, webhook secret | none configured in prod | none |
| `notion` | `notion` | `OAUTH2` | client id, client secret, webhook secret | none | none |
| `railway` | `railway` | `OAUTH2` | client id, client secret | `openid,offline_access,email,profile,workspace:admin` | none |
| `slack` | `slack` | `OAUTH2` | client id, client secret | configured in `slack.json` | none |
| `bugsink` | `bugsink` | `API_KEY` | none | none | `baseUrl` |
| `vercel` | `vercel` | `API_KEY` | none | none | none |
