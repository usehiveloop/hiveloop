---
name: local-testing-slim
description: Slim entry for local end-to-end testing of the hiveloop stack. Use when you've changed code in `internal/`, `cmd/server`, or `apps/web` and want to verify against running processes. Triggers include "test in the browser", "verify the flow", "smoke test", and anything touching auth, OAuth, integrations, webhooks, dispatch, or routing. Skip for unit-test-only or doc-only changes.
---

# local-testing (slim)

Four-process native stack. `make local-up` builds, supervises, and seeds. Drive with `agent-browser`, assert with `psql` + log greps. Third-parties faked via `cmd/fake-nango`.

```bash
make local-up      # idempotent; auto-runs seed-test
make local-status  # health-checks the ports + lists pids
make login-test    # writes a __session cookie for agent-test@example.com
make local-down    # kills supervisors + child groups + port holders
make local-reset   # local-down + local-up
```

Ports / logs (under `/tmp/agent-test/`): postgres `cat pg.port` (5432), redis 6379, fake-nango 13004, backend 18080, frontend 31112. Ephemeral `AUTH_RSA_PRIVATE_KEY` + `KMS_KEY` generated when `.env` is absent. Don't reproduce by hand — read `scripts/local-up.sh`.

## Seeded identity

| Thing | Value |
|---|---|
| User | `agent-test@example.com` (auto-confirm, OTP) |
| Org | `Agent Test Workspace` (owner + platform admin) |
| API key | `hvl_sk_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa` (full access) |
| Integrations | github, slack, notion, linear, asana, jira, salesforce, railway (OAUTH2) |
| Agent | `Test Agent` |

## Two auth modes

- **Bearer API key** for org-scoped routes (`/v1/agents`, `/v1/credentials`, admin POSTs).
- **Session cookie** (`__session`, AES-GCM) for user-scoped routes (`/v1/in/connections`, `/v1/me`). `make login-test` writes one to the agent-browser profile. Bearer returns `{"error":"invalid token"}` for these.

## DB + queue

```bash
PG_PORT=$(cat /tmp/agent-test/pg.port 2>/dev/null || echo 5432)
PSQL="PGPASSWORD=localdev psql -h localhost -p $PG_PORT -U hiveloop -d hiveloop"
redis-cli LRANGE asynq:default:scheduled 0 5   # or `make asynq-peek`
```

Direct SQL fine for assertions and fixture cleanup. Use the API for encrypted columns (`agents.encrypted_env_vars`, `credentials.encrypted_key`, `sandboxes.encrypted_bridge_api_key`) and anything with audit/asynq side effects. `make seed-test` is idempotent.

## OAuth approve recipe

```bash
$PSQL -c "UPDATE in_connections SET revoked_at=NOW()
  WHERE in_integration_id IN (SELECT id FROM in_integrations WHERE provider='github')
  AND revoked_at IS NULL;"
curl -sX POST http://localhost:13004/_admin/reset
agent-browser open http://localhost:31112/w/connections
agent-browser find role button click --name "Add connection"
agent-browser find role button click --name "github GitHub"
sleep 3   # popup → callback → ws notify → frontend POST → row persisted
$PSQL -tAc "SELECT count(*) FROM in_connections c JOIN in_integrations i ON i.id=c.in_integration_id
  WHERE i.provider='github' AND c.revoked_at IS NULL;"   # expect 1
```

Reject path: `POST /_admin/outcome` with `{"provider_config_key":"in_github-test","result":"reject"}` before the click. Full admin surface: `fake-nango` skill.

## Top gotchas

| Symptom | Fix |
|---|---|
| `invalid signature` on webhook | `NANGO_SECRET_KEY` ≠ fake-nango `-secret` (local-up aligns them) |
| Popup hangs | fake-nango unreachable or `NEXT_PUBLIC_CONNECTIONS_HOST` wrong |
| Toast "success" but `/v1/in/connections` empty | Bearer can't read user-scoped data — drive via UI |
| `make local-up` exits before seed | A supervisor reported `✗`. Check `/tmp/agent-test/*.log`, run `make seed-test` |
| `pnpm dev` lock conflict | `make local-down` clears it |
| `Element not found` | Stale agent-browser ref. Re-snapshot or `find role button click --name "..."` |

Out of scope: prod ops, real Daytona, LLM-proxy, Paystack. Browser CLI: `agent-browser` skill.
