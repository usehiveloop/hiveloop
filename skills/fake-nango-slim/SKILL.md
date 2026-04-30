---
name: fake-nango-slim
description: Local Nango replacement for agent test runs. Use when modifying `internal/nango`, `internal/handler/in_*`, `internal/handler/nango_webhooks*`, `internal/handler/git_credentials.go`, `internal/handler/railway_proxy.go`, or any code that constructs `nango.Client` — integration CRUD, OAuth flows, GitHub App install, webhook ingestion, provider proxy. Always use fake-nango locally; never point a test at real dev/prod Nango.
---

# fake-nango (slim)

Drop-in Nango replacement, single Go binary, started by `make local-up` on :13004. Outbound webhooks ARE properly signed (backend verifies `X-Nango-Hmac-Sha256` in `internal/handler/nango_webhooks.go`). Inbound auth is faked — any bearer/HMAC/session passes.

Source: `cmd/fake-nango/`. Scenarios: `cmd/fake-nango/scenarios/*.yaml`. `local-up` wires `NANGO_ENDPOINT=http://localhost:13004`, `NANGO_SECRET_KEY=fake-nango-secret`, `NEXT_PUBLIC_CONNECTIONS_HOST=http://localhost:13004`. The fake's `-secret` flag must equal `NANGO_SECRET_KEY` or webhook verification fails.

## Admin surface

| Path | Body | Effect |
|---|---|---|
| `POST /_admin/load` | `{"name":"all-enabled"}` | Replace integrations + connections + proxy fixtures from a YAML scenario |
| `POST /_admin/fixtures` | `{"fixtures":[...]}` | Replace just proxy fixtures |
| `POST /_admin/outcome` | `{"provider_config_key":"in_github-test","result":"reject","error_type":"access_denied"}` | Per-provider one-shot override; omit `provider_config_key` to set the sticky default |
| `POST /_admin/github-webhook` | `{"connection_id","provider_config_key","event_type","action","payload"?}` | Build + sign + deliver a realistic GitHub forward webhook |
| `POST /_admin/webhook/forward` | generic | Generic forward webhook |
| `POST /_admin/reset` | `{}` | Clears connections + sessions + outcomes + fixtures + log; integrations remain |
| `GET  /_admin/log` | — | JSON of received calls + delivered webhooks |

## Outcome semantics

Per-provider override (with `provider_config_key`) consumes once then falls back to default. Default is sticky until reset; default-default is `approve`.

## Common patterns

```bash
H='Content-Type: application/json'
curl -sX POST http://localhost:13004/_admin/outcome -H "$H" \
  -d '{"provider_config_key":"in_github-test","result":"reject","error_type":"access_denied"}'
curl -sX POST http://localhost:13004/_admin/github-webhook -H "$H" \
  -d '{"connection_id":"<from DB>","provider_config_key":"in_github-test","event_type":"pull_request","action":"opened"}'
curl -s  http://localhost:13004/_admin/log | jq
curl -sX POST http://localhost:13004/_admin/reset
```

## Scenario YAML

```yaml
name: github-basic
integrations:
  - unique_key: in_github-test         # must match `inNangoKey()`
    provider: github
    credentials: { type: APP, app_id: "12345" }   # APP auto-mints webhook_secret
connections:
  - id: conn-github-1
    provider_config_key: in_github-test
    credentials: { type: APP, access_token: ghs_fake_xxx, installation_id: "42" }
proxy:
  - { method: GET,  path: /user/repos, status: 200, body: [{ id: 1 }] }
  - { method: POST, path_pattern: /repos/*/*/issues, status: 201, body: { id: 999 } }
```

First match wins; `path` beats `path_pattern`. `_admin/load` REPLACES — bundle everything into one scenario.

## Quirks

- No inbound auth verification.
- Single-instance WS, no Redis pub/sub.
- `providers.json` is pinned — new providers fail with "unknown provider" until added.
- `/oauth/connect/{key}` redirects straight to `/oauth/callback`; no real provider hop.
- Loading a scenario does NOT clear connections — `/_admin/reset` between tests.

## Troubleshooting

| Symptom | Fix |
|---|---|
| `invalid signature` | `-secret` ≠ `NANGO_SECRET_KEY` |
| `in-connection not found` | DB `nango_connection_id` ≠ posted `connection_id` |
| Popup hangs | WS unreachable — check `NEXT_PUBLIC_CONNECTIONS_HOST` |
| `unknown provider` | Add to `providers.json` |
| `no fixture` 404 | Add to scenario or POST `_admin/fixtures` |
