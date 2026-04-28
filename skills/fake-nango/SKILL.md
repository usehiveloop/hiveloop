---
name: fake-nango
description: Local Nango replacement for agent test runs. Use this skill whenever you're writing or modifying code in `internal/nango`, `internal/handler/in_*`, `internal/handler/nango_webhooks*`, `internal/handler/git_credentials.go`, `internal/handler/railway_proxy.go`, `internal/resources/discovery.go`, `internal/rag/connectors/github`, `internal/mcpserver/executor.go`, or any code that constructs/uses `nango.Client`. Triggers include integration CRUD, OAuth/Connect Session flows, GitHub App install, provider proxy calls, webhook ingestion, git-credentials helper, and any test that would otherwise require hitting `integrations.dev.hiveloop.com` or `integrations.usehiveloop.com`. Prefer fake-nango over real Nango for every local and sandboxed test — never point a test at the real dev or prod tenant.
---

# fake-nango

A drop-in Nango replacement that runs as a single Go binary inside the agent
sandbox. The HTTP + WebSocket surfaces match what `internal/nango/client.go`
calls and what `@nangohq/frontend` opens. Inbound bearer tokens, HMACs, and
session TTLs are all unverified — this is a fake. Outbound webhooks ARE
properly signed because the real backend at
`internal/handler/nango_webhooks.go` verifies `X-Nango-Hmac-Sha256`.

Source: `cmd/fake-nango/`. Sample scenarios: `cmd/fake-nango/scenarios/`.

## When to use it

Use fake-nango whenever your change might:

- Add or change a Nango API call in `internal/nango/client.go`.
- Touch handlers under `internal/handler/in_*` (integrations / connections / sessions).
- Modify the webhook ingestion path (`internal/handler/nango_webhooks*.go`,
  `internal/handler/nango_webhooks_infer.go`, dispatch tasks).
- Add a new provider that goes through Nango's proxy.
- Change `internal/handler/git_credentials.go` or `railway_proxy.go`.
- Need to drive a Connect UI flow from `apps/web` against a local backend.

Do NOT use fake-nango for: pure unit tests with no Nango I/O, or tests that
explicitly verify cross-instance Redis pub/sub for the WS publisher (single
instance only).

## Quick start

```bash
# In one terminal: start the fake.
fake-nango -addr :3004 \
  -secret "$NANGO_WEBHOOK_SECRET" \
  -webhook-target http://localhost:8080/internal/webhooks/nango

# In your backend env:
export NANGO_ENDPOINT=http://localhost:3004
export NANGO_SECRET_KEY=any-string-fake-doesnt-verify
export NEXT_PUBLIC_CONNECTIONS_HOST=http://localhost:3004

# Seed a baseline scenario.
curl -sX POST http://localhost:3004/_admin/load \
  -H 'Content-Type: application/json' \
  -d '{"name":"github-basic"}'

# Now run your test or `make dev` — every Nango call hits the fake.
```

## What the fake serves

### Nango HTTP API (matched to `internal/nango/client.go`)

| Verb + path | Notes |
|---|---|
| `GET  /providers`, `/providers.json` | Static catalog from `cmd/fake-nango/providers.json` |
| `GET  /providers/{name}` | Single provider by name |
| `POST /integrations` | Stores integration; auto-mints `webhook_secret` for APP creds |
| `GET  /integrations/{key}` | Returns `{data:{webhook_url, credentials:{webhook_secret}, forward_webhooks:true, ...}}` |
| `PATCH /integrations/{key}` | Update; rotates `webhook_secret` on APP credential change |
| `DELETE /integrations/{key}` | Remove |
| `POST /connections`, `POST /connection` | Direct create (API_KEY path) |
| `GET  /connections/{id}`, `GET /connection/{id}` | Returns `{credentials, connection_config, provider, ...}` |
| `DELETE /connections/{id}`, `DELETE /connection/{id}` | Revoke |
| `POST /connect/sessions` | `{data:{token, connect_link, expires_at}}` |
| `POST /connect/sessions/reconnect` | Same shape |
| `GET  /connect/session`, `DELETE /connect/session` | Session lookup/cleanup |
| `ALL  /proxy/*` | Looks up YAML/admin-API fixtures; 404 with hint if no match |

### OAuth / Connect popup surface (matched to `@nangohq/frontend`)

| Verb + path | Notes |
|---|---|
| `GET  /oauth/connect/{key}?ws_client_id=&connection_id=` | OAuth2 / APP popup target. Issues `state`, redirects straight to `/oauth/callback` |
| `GET  /oauth/callback?state=&code=&installation_id=&setup_action=` | Resolves session, applies outcome, fires WS `success`/`error`, fires auth webhook, renders self-closing HTML |
| `POST /oauth2/auth/{key}` | 2-legged client credentials, synchronous |
| `POST /auth/oauth-outbound/{key}` | GitHub App outbound flow |
| `POST /api-auth/api-key/{key}` | Synchronous, no popup, no WS |
| `POST /api-auth/basic/{key}` | Same |
| `POST /auth/jwt/{key}`, `/auth/tba/{key}`, `/auth/two-step/{key}`, `/auth/bill/{key}`, `/auth/signature/{key}`, `/app-store-auth/{key}`, `/auth/unauthenticated/{key}` | All synchronous |
| `WS   /` (configurable `-ws-path`) | Sends `connection_ack` immediately; fires `success`/`error` from callback handler |

### Admin API (controls the fake)

| Verb + path | Body | Effect |
|---|---|---|
| `POST /_admin/load` | `{"name":"github-basic"}` or `{"path":"/abs/path.yaml"}` | Replaces integrations + connections + proxy fixtures from a YAML scenario |
| `POST /_admin/fixtures` | `{"fixtures":[{...}]}` | Replaces just the proxy fixtures (no need for a YAML file) |
| `POST /_admin/outcome` | `{"result":"reject","error_type":"access_denied"}` | The next OAuth callback rejects instead of approves |
| `POST /_admin/github-webhook` | `{"connection_id","provider_config_key","event_type","action","payload"?}` | Builds a properly-shaped GitHub forward webhook with `X-GitHub-Event` header and signs it |
| `POST /_admin/webhook/forward` | `{"target","connection_id","provider_config_key","provider","payload","provider_headers"}` | Generic forward-webhook firing |
| `POST /_admin/ws/notify` | `{"ws_client_id","provider_config_key","connection_id","result"}` | Manually push a WS message (rare) |
| `POST /_admin/reset` | `{}` | Clears connections + sessions + outcome + fixtures + call log; keeps integrations |
| `GET  /_admin/log` | — | JSON dump of call log + delivered webhooks |

## Scenario YAML format

```yaml
name: github-basic
integrations:
  - unique_key: in_org-github   # must match what the backend will compute via inNangoKey()
    provider: github
    display_name: GitHub
    credentials:
      type: APP                  # auto-mints webhook_secret
      app_id: "12345"
connections:
  - id: conn-github-1
    provider_config_key: in_org-github
    provider: github
    credentials:
      type: APP
      access_token: ghs_fake_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
      installation_id: "42"
proxy:
  - method: GET                  # GET/POST/PATCH/DELETE/* — "" or "*" matches any verb
    path: /user/repos            # exact match
    status: 200
    body:
      - { id: 1, name: agent-test-repo, full_name: agent/agent-test-repo }

  - method: POST
    path_pattern: /repos/*/*/issues   # glob, single-segment wildcard
    status: 201
    body:
      id: 999
      number: 1
      state: open

  - path_pattern: /repos/*/*/contents/*
    status: 200
    body: { type: file, encoding: base64, content: ZmFrZQ==, sha: deadbeef }
```

Lookup: first matching fixture wins. Method matching is case-insensitive after
normalization. `path` takes priority over `path_pattern` if both are set.

Scenarios live in `cmd/fake-nango/scenarios/` by default. Override with
`FAKE_NANGO_SCENARIOS_DIR=/abs/dir`. `_admin/load` accepts either `name`
(resolves to `<dir>/<name>.yaml`) or absolute `path`.

## Common patterns

### 1. End-to-end OAuth in a Go test

```go
// 1. Stand up fake-nango against a process port.
cmd := exec.Command(fakeNangoPath, "-addr", "127.0.0.1:0", "-webhook-target", backend.URL+"/internal/webhooks/nango")
// ... start, capture port ...

// 2. Point the backend's Nango client at it.
nangoClient := nango.NewClient(fakeNangoURL, "ignored")
nangoClient.FetchProviders(ctx)

// 3. Drive the integration handler.
req := nango.CreateIntegrationRequest{...}
require.NoError(t, nangoClient.CreateIntegration(ctx, req))

// 4. Optionally seed scenarios via /_admin/load before exercising proxy paths.
```

For most existing Go tests, the in-process `e2e/nango_mock_test.go` pattern is
still simpler. Use the binary fake-nango when:
- Tests need to share state with `pnpm dev` or another process.
- A browser flow drives `/oauth/connect/{key}`.
- You need realistic webhook timing (binary stays alive across multiple commands).

### 2. Browser-driven OAuth via `agent-browser`

```bash
# Start backend pointing at fake-nango.
NANGO_ENDPOINT=http://localhost:3004 NEXT_PUBLIC_CONNECTIONS_HOST=http://localhost:3004 \
  make dev &

# Seed.
curl -sX POST http://localhost:3004/_admin/load -d '{"name":"github-basic"}' \
  -H 'Content-Type: application/json'

# Drive the UI.
agent-browser open http://localhost:30112/w/connections
agent-browser snapshot -i
agent-browser click @e<button-ref>      # the "Connect GitHub" button

# Fake-nango popup auto-resolves; backend receives webhook; UI updates via TanStack invalidate.
```

### 3. Test the rejection path

```bash
curl -sX POST http://localhost:3004/_admin/outcome \
  -H 'Content-Type: application/json' \
  -d '{"result":"reject","error_type":"access_denied","error_desc":"user denied install"}'

# Now drive the same UI. The popup renders the error HTML and the SDK promise rejects.
# State resets to "approve" after one consumed call.
```

### 4. Fire a realistic GitHub PR webhook

```bash
curl -sX POST http://localhost:3004/_admin/github-webhook \
  -H 'Content-Type: application/json' \
  -d '{
    "connection_id": "conn-github-1",
    "provider_config_key": "in_org-github",
    "event_type": "pull_request",
    "action": "opened"
  }'
```

`payload` defaults to a minimal but plausibly-shaped object including
`repository`, `sender`, `pull_request` (or `issue`/`workflow_run` etc. by
event_type). Override with `"payload": {...}` for fixture-specific cases.

### 5. Test the dispatch path

The webhook lands at `POST /internal/webhooks/nango` with the dual signature
headers your handler verifies. After the call, inspect the asynq queues to
assert that `RouterDispatchTask` and `SubscriptionDispatchTask` were enqueued
(see `nango_webhooks_dispatch.go:107-145`).

## Backend env to wire fake-nango in

```
NANGO_ENDPOINT=http://localhost:3004
NANGO_SECRET_KEY=fake-nango-secret      # any string — fake doesn't verify
NANGO_WEBHOOK_SECRET=fake-nango-secret  # MUST match the -secret flag passed to fake-nango
                                        # the backend verifies X-Nango-Hmac-Sha256(this, body)
NEXT_PUBLIC_CONNECTIONS_HOST=http://localhost:3004
```

Tip: keep `NANGO_SECRET_KEY` and `NANGO_WEBHOOK_SECRET` set to the same string,
and pass that same string as `fake-nango -secret`. One env var to mismatch.

## Important quirks vs real Nango

- **No verification.** You can hit fake-nango with any bearer token, any HMAC,
  any session token. Do not write tests that rely on those being rejected.
- **Single-instance WS.** No Redis pub/sub. If your test spawns two fake-nango
  processes, WS messages don't cross.
- **Catalog is pinned.** `cmd/fake-nango/providers.json` is a snapshot. If
  your code adds support for a new provider that's not in the snapshot, add
  it there or the integration handler will reject the request as "unknown
  provider".
- **`/oauth/connect/{key}` skips the simulated provider redirect.** It
  returns `302 → /oauth/callback?state=…&code=…` directly. If you genuinely
  need to test "user lands on GitHub.com first," fake-nango is the wrong tool.
- **One outcome at a time.** `/_admin/outcome` overrides the next call only;
  state resets to `approve` after consumption. For repeated rejects, set it
  again before each flow.
- **`/_admin/fixtures` and `/_admin/load` REPLACE all fixtures**. There's no
  append. Bundle every fixture you need for a flow into one scenario YAML.
- **Loading a scenario does not clear connections.** Use `/_admin/reset`
  between independent test cases if you want a clean slate.

## Troubleshooting

| Symptom | Likely cause |
|---|---|
| Backend logs `nango webhook: invalid signature` | `NANGO_WEBHOOK_SECRET` (backend) ≠ `-secret` flag (fake) |
| Backend logs `nango webhook: in-connection not found` | The connection's `nango_connection_id` in your DB doesn't match the `connectionId` you set in `_admin/github-webhook` |
| `unknown provider` from integration create | Provider not in `providers.json` snapshot — add it |
| Frontend popup hangs | WebSocket couldn't connect — check `NEXT_PUBLIC_CONNECTIONS_HOST` is `http://` (SDK rewrites to `ws://`) and that fake-nango is reachable |
| `no fixture` 404 from proxy | Scenario didn't include that path — add it or post to `/_admin/fixtures` |

## What fake-nango does NOT cover

- Paystack (use Paystack test-mode keys, sk_test_xxx)
- OpenRouter / Fireworks / Anthropic / OpenAI (those are LLM providers, not Nango)
- Real OAuth provider redirects (fake-nango short-circuits the provider hop)
- Nango's sync/records/scripts deploy surface (we don't use it)

For those, see the four-tier test isolation model in `docs/testing/`.
