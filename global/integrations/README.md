# Global Integrations

Each `*.json` file in this directory defines one platform-managed integration.
Startup syncs enabled manifests into Nango and `in_integrations`.

Secrets must be referenced through environment variable names only. Do not
commit OAuth client secrets, app private keys, API keys, or webhook secrets.

`required: true` makes missing env vars or Nango sync failures fail startup.
Optional integrations are skipped when their env vars are absent.
