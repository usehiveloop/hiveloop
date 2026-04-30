---
name: local-testing
description: How to test backend or frontend changes end-to-end on a local stack. Use this skill whenever you've changed code in `internal/` (handlers, middleware, dispatch, sandbox, billing, etc.), `cmd/server`, `apps/web`, or anything that affects HTTP/UI behavior, and you need to verify the change actually works against running processes — not just unit tests. Triggers include: "test this in the browser", "verify the flow", "drive the UI", "smoke test", "make sure connections still work", "see this work end-to-end", or any change touching auth, OAuth, integrations, webhooks, dispatch, or routing. Also trigger after writing a new handler or after merging upstream changes that may have broken local behavior. Skip this skill for pure unit-test additions, doc-only changes, or refactors with no observable behavior change.
---

# local-testing

Spin up the four-process stack, drive it with the `agent-browser` CLI, and assert via DB queries + log greps. Avoids hitting any real third-party (Nango, Paystack, OpenRouter) by routing every external call through `cmd/fake-nango` and treating email as logs.

## What runs and where

| Service | Port | Source | Notes |
|---|---|---|---|
| Postgres | 5432 | native (apt cluster) | Bootstrapped by `scripts/local-init.sh` on first run; live port written to `/tmp/agent-test/pg.port`. |
| Redis | 6379 | native (`redis-server`) | Daemonized; pidfile at `/tmp/agent-test/redis.pid`. |
| **fake-nango** | 13004 | `cmd/fake-nango` | Replaces every Nango HTTP + WS call. See `skills/fake-nango`. |
| **backend** | 18080 | `cmd/server` | `serve` mode. Use 18080 to coexist with anything on 8080. |
| **frontend** | 31112 | `apps/web` (Next.js) | Use 31112 to coexist with anything on 30112. |

The agent's browser is also process #5 — `agent-browser` launches a real Chrome window via CDP. Watch your dock; it's not headless.

The dev-box sandbox image already ships postgres, redis, Go, Node, and corepack. No docker-compose, no `.env` required — `make local-up` generates ephemeral RSA + KMS keys when `.env` is absent.

## Quick start

```bash
make local-up        # idempotent: brings up the full stack and auto-runs seed-test
make local-status    # health-checks the four ports and lists supervised pids
make login-test      # writes a __session cookie for agent-test@example.com to the agent-browser profile
make local-down      # tears down supervisors, child process groups, and port holders
make local-reset     # local-down + local-up
```

`make local-up` does, in order: postgres bring-up (`scripts/local-init.sh` initializes the apt cluster on first run), redis, build `cmd/fake-nango` + `cmd/server`, supervise fake-nango on :13004, write `/tmp/agent-test/backend.env` (sources `.env` if present, else generates ephemeral `AUTH_RSA_PRIVATE_KEY` + `KMS_KEY` under `/tmp/agent-test/`), supervise backend on :18080, install web deps via corepack/pnpm, supervise Next.js on :31112, then chain `make seed-test`.

Logs land in `/tmp/agent-test/{fake-nango,backend,web}.log` — `tail -f /tmp/agent-test/*.log` to follow. Each service is wrapped by a supervisor that restarts on crash; supervisor PIDs are in `*.supervisor.pid`, child PIDs in `*.pid`.

Don't reproduce the bring-up by hand — read `scripts/local-up.sh` if you need to deviate. Doing it manually drifts from what `make local-down` knows how to clean up.

## Test identity (after `make seed-test`)

| Thing | Value |
|---|---|
| User email | `agent-test@example.com` (auto-confirmed, OTP-only login) |
| Org name | `Agent Test Workspace` |
| User role | `owner` + platform admin (set via `PLATFORM_ADMIN_EMAILS`) |
| API key (full access) | `hvl_sk_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa` |
| Pre-seeded integrations | github, slack, notion, linear, asana, jira, salesforce, railway (all OAUTH2) |
| Pre-seeded agent | `Test Agent` |

## Authentication paths

The backend has two auth modes — pick the right one for the endpoint you're testing:

### Bearer API key — for org-scoped endpoints

```bash
KEY="hvl_sk_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
curl -H "Authorization: Bearer $KEY" http://localhost:18080/v1/agents
```

Works for: `/v1/agents`, `/v1/credentials`, `/v1/in/integrations` (admin POST/PATCH/DELETE), `/v1/skills`, etc.
Does NOT work for: anything that needs `user_id` from session (the `/v1/in/connections` family, `/v1/me`, etc.) — those return `{"error":"invalid token"}` with bearer auth.

### Session cookie — for user-scoped endpoints

The cookie name is `__session`, HttpOnly, AES-256-GCM-encrypted (see `apps/web/lib/auth/session.ts`). Two ways to get one:

**Option A — programmatic OTP login through the page** (works because `/api/proxy` intercepts auth responses and writes the cookie automatically):

```bash
# 1. Trigger OTP via the proxy so the response can set the cookie later
agent-browser open "http://localhost:31112/auth"
agent-browser eval "fetch('/api/proxy/auth/otp/request', { method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify({email:'agent-test@example.com'}) }).then(r => r.status)"

# 2. Read OTP from server log
sleep 0.5
OTP=$(grep -oE 'code:[0-9]{6}' /tmp/agent-test/backend.log | tail -1 | cut -d: -f2)

# 3. Verify — proxy auto-sets __session cookie in chrome
agent-browser eval "fetch('/api/proxy/auth/otp/verify', { method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify({email:'agent-test@example.com',code:'$OTP'}) }).then(r => r.json())"
```

**Option B — drive the UI by hand**: open `/auth`, click email button, fill, submit, read OTP from log, fill OTP. ~3-5s. Use when verifying the auth UI itself.

The cookie persists in the chrome profile until cleared. Combine with `agent-browser --session-name agent-test ...` to reuse across test runs.

## Modifying DB state

Direct SQL is faster than going through the API for **assertions** and **fixture cleanup**. For **state changes that have side effects** (audit rows, asynq tasks, cache invalidation), use the API.

### psql cheat sheet

```bash
# Read the live port written by local-up (defaults to 5432).
PG_PORT=$(cat /tmp/agent-test/pg.port 2>/dev/null || echo 5432)
PSQL="PGPASSWORD=localdev psql -h localhost -p $PG_PORT -U hiveloop -d hiveloop"

# Inspect a connection by integration
$PSQL -c "SELECT id, nango_connection_id, revoked_at FROM in_connections
          WHERE in_integration_id IN (SELECT id FROM in_integrations WHERE provider='github')
          ORDER BY created_at DESC LIMIT 5;"

# Revoke all connections for a provider (force the connect dialog button to re-enable)
$PSQL -c "UPDATE in_connections SET revoked_at = NOW()
          WHERE in_integration_id IN (SELECT id FROM in_integrations WHERE provider='github')
          AND revoked_at IS NULL;"

# Confirm an event was dispatched (asynq queues). Redis is native — no docker exec.
redis-cli LRANGE asynq:default:scheduled 0 5
# For richer poking, see `make asynq-peek`.
```

### When to insert directly

- ✅ Setting up state the API doesn't expose (fake "this connection is 30 days old", `expires_at` overrides, etc.)
- ✅ Bulk seeding fixtures
- ✅ Resetting between test cases (`UPDATE ... SET revoked_at = NOW()` is faster than DELETE + re-insert)
- ❌ Anything with encrypted columns (`agents.encrypted_env_vars`, `credentials.encrypted_key`, `sandboxes.encrypted_bridge_api_key`) — use the API so the KMS wrapper runs
- ❌ Anything with audit/usage side effects you want exercised

### Re-running the seed

`make seed-test` is idempotent. Run it any time the DB feels dirty. It uses `ON CONFLICT (provider) DO UPDATE` for integrations, `ON CONFLICT (email) DO UPDATE` for the user, etc. Re-running won't duplicate.

## fake-nango control

`skills/fake-nango/SKILL.md` is the full reference. Cliff notes:

```bash
# Outcome of the next OAuth callback (per-provider, one-shot; or set default)
curl -sX POST http://localhost:13004/_admin/outcome -H 'Content-Type: application/json' -d \
  '{"provider_config_key":"in_github-test","result":"reject","error_type":"access_denied"}'

# Replace proxy fixtures
curl -sX POST http://localhost:13004/_admin/load -H 'Content-Type: application/json' -d \
  '{"name":"all-enabled"}'

# Fire a forward webhook (HMAC-signed; backend will verify)
curl -sX POST http://localhost:13004/_admin/github-webhook -H 'Content-Type: application/json' -d \
  '{"connection_id":"<from DB>","provider_config_key":"in_github-test","event_type":"pull_request","action":"opened"}'

# Inspect what fake-nango received + delivered
curl -s http://localhost:13004/_admin/log
```

Always reset between test cases that depend on state:

```bash
curl -sX POST http://localhost:13004/_admin/reset
curl -sX POST http://localhost:13004/_admin/load -d '{"name":"all-enabled"}' -H 'Content-Type: application/json'
```

## agent-browser patterns

`skills/agent-browser/SKILL.md` covers the CLI. Patterns specific to this stack:

### Navigate + snapshot pattern

Always `snapshot -i` before clicking. Refs reset on navigation.

```bash
agent-browser open http://localhost:31112/w/connections
agent-browser snapshot -i               # find the ref of the button you want
agent-browser click @e6                  # click by ref
```

### Click by name (prefer this when refs change)

```bash
agent-browser find role button click --name "Add connection"
```

This survives DOM reshuffles between snapshots. Use when chaining clicks.

### Reading toast / status messages

Sonner toasts auto-dismiss in ~5s. Capture immediately after the action:

```bash
agent-browser eval "
Array.from(document.querySelectorAll('li[role=\"listitem\"], [data-sonner-toast]'))
  .map(e => e.textContent).filter(Boolean).slice(-3)
"
```

### Screenshots as evidence

```bash
SHOTS=/tmp/agent-test/screenshots; mkdir -p $SHOTS; N=0
shot() { N=$((N+1)); agent-browser screenshot "$SHOTS/$(printf '%02d' $N)-$1.png"; }
shot "before-click"
agent-browser click @e6
shot "after-click"
```

`open /tmp/agent-test/screenshots/` to flip through. Capture before+after for every state change so PRs have evidence.

### Watching what's happening

The chrome window is a real window — it's the easiest live view. For DevTools-level inspection: `agent-browser get cdp-url` returns `ws://127.0.0.1:<port>/...` — open `chrome://inspect/#devices`, add `127.0.0.1:<port>` under Configure, then "inspect" the page target.

## Common end-to-end patterns

### OAuth approve flow

```bash
# 1. Ensure no existing connection blocks the button
PG_PORT=$(cat /tmp/agent-test/pg.port 2>/dev/null || echo 5432)
PGPASSWORD=localdev psql -h localhost -p $PG_PORT -U hiveloop -d hiveloop -c \
  "UPDATE in_connections SET revoked_at = NOW() WHERE in_integration_id IN
   (SELECT id FROM in_integrations WHERE provider='github') AND revoked_at IS NULL;"

# 2. fake-nango will approve next callback by default
curl -sX POST http://localhost:13004/_admin/reset
curl -sX POST http://localhost:13004/_admin/load -d '{"name":"all-enabled"}' -H 'Content-Type: application/json'

# 3. Drive UI
agent-browser open http://localhost:31112/w/connections
agent-browser find role button click --name "Add connection"
sleep 1
agent-browser find role button click --name "github GitHub"
sleep 3   # popup → callback → ws notify → frontend POST → connection persisted

# 4. Assert
PGPASSWORD=localdev psql -h localhost -p 5433 -U hiveloop -d hiveloop -tAc \
  "SELECT count(*) FROM in_connections c
   JOIN in_integrations i ON i.id = c.in_integration_id
   WHERE i.provider='github' AND c.revoked_at IS NULL;"
# Expect: 1
```

### OAuth reject flow

```bash
# Queue reject for a specific provider (per-provider override beats default)
curl -sX POST http://localhost:13004/_admin/outcome -H 'Content-Type: application/json' -d \
  '{"provider_config_key":"in_github-test","result":"reject","error_type":"access_denied"}'

# Drive same UI as approve flow. Expect "Connection failed" toast, no DB row.
agent-browser eval "Array.from(document.querySelectorAll('li[role=\"listitem\"]')).map(e=>e.textContent)" 
# Expect: includes "Connection failed."
```

### Webhook dispatch verification

```bash
# Need a connection in place first (run the approve flow above), then:
PG_PORT=$(cat /tmp/agent-test/pg.port 2>/dev/null || echo 5432)
CONN_ID=$(PGPASSWORD=localdev psql -h localhost -p $PG_PORT -U hiveloop -d hiveloop -tAc \
  "SELECT nango_connection_id FROM in_connections c
   JOIN in_integrations i ON i.id = c.in_integration_id
   WHERE i.provider='github' AND c.revoked_at IS NULL LIMIT 1;")

curl -sX POST http://localhost:13004/_admin/github-webhook -H 'Content-Type: application/json' -d "{
  \"connection_id\": \"$CONN_ID\",
  \"provider_config_key\": \"in_github-test\",
  \"event_type\": \"pull_request\",
  \"action\": \"opened\"
}"

# Backend should: verify HMAC, identify connection, enqueue dispatch tasks
grep -E "webhook dispatch: enqueued|delivery_id=" /tmp/agent-test/backend.log | tail -5
```

If the webhook lands but isn't dispatched, check `nango_webhooks_identify.go:106-117` — `provider_config_key` must start with `in_` for the single-tenant lookup, otherwise it parses as `<orgUUID>_<unique_key>`.

## When something is wrong

| Symptom | First check |
|---|---|
| `nango webhook: invalid signature` in backend log | `NANGO_SECRET_KEY` (backend) and `-secret` (fake-nango) must match |
| `in-connection not found` on auth webhook | Frontend hasn't POSTed the connection row yet — race between fake-nango's auth webhook and the frontend's `/v1/in/integrations/{id}/connections`. Harmless for `type=auth`; only matters if a test depends on the auth event triggering something |
| Frontend popup hangs forever | WebSocket couldn't connect — check `NEXT_PUBLIC_CONNECTIONS_HOST` is `http://localhost:13004` (SDK rewrites to `ws://`) and that fake-nango is reachable |
| `"platform admin access required"` from `POST /v1/in/integrations` | `PLATFORM_ADMIN_EMAILS` env not set on backend (admin check is dynamic per-request, no need to re-login after setting it) |
| `Element not found` from agent-browser | Stale ref. Re-run `snapshot -i` and use the new ref, or switch to `find role button click --name "..."` |
| Connection toast says "successfully" but `/v1/in/connections` is empty | The frontend uses session cookie; bearer key won't see user-scoped data. Switch to driving via the UI (cookie auth) |
| `pnpm dev` won't start: `Unable to acquire lock` | A previous `next dev` is still holding the lock. `make local-down` clears it (port-holder kill + `local-up.sh:free_next_lock` removes `apps/web/.next/dev/lock`); only fall back to `lsof` if you're not using the Makefile flow |
| `make local-up` says "✗ frontend (timeout after 90s)" on first run | pnpm install over rosetta is slow. Look at `/tmp/agent-test/pnpm-install.log` — once it shows "Done in …" the supervisor will keep retrying and the next `make local-status` will show frontend up |
| `make local-up` exits before seed runs | `seed-test` only chains after `local-up.sh` exits cleanly. Check `/tmp/agent-test/*.log` for whichever supervisor reported `✗`. After fixing, `make seed-test` is safe to run by itself |
| `make seed-test` fails on `agents` insert | Partial unique index can't be ON CONFLICT target. The script uses `WHERE NOT EXISTS` instead — if you've copied it, make sure that block is intact |

## What this skill does NOT cover

- Production / staging operations
- Daytona sandbox provisioning (those are real cloud calls; fake them separately if a test needs them)
- LLM proxy testing (use OpenRouter/Fireworks test keys; that flow is orthogonal to Nango)
- Paystack billing (use `sk_test_xxx` keys + the `internal/billing/fake` provider — see `internal/billing/subscription/`)

For Nango fake details, see `fake-nango` skill. For agent-browser CLI, see `agent-browser` skill.
