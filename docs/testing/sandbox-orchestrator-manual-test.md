# Sandbox Orchestrator — Manual Integration Test Plan

Tests the real end-to-end flow: ZiraLoop → Daytona → Bridge running in sandbox → Turso.

## Prerequisites

### Environment Variables

All must be set in `.env`:

```bash
# Daytona
SANDBOX_PROVIDER_ID=daytona
SANDBOX_PROVIDER_URL=https://app.daytona.io/api
SANDBOX_PROVIDER_KEY=<your daytona api key>
SANDBOX_TARGET=us

# Turso
TURSO_API_TOKEN=<your turso platform api token>
TURSO_ORG_SLUG=<your turso org slug>
TURSO_GROUP=default

# Sandbox encryption
SANDBOX_ENCRYPTION_KEY=<output of: openssl rand -base64 32>

# Bridge
BRIDGE_BASE_IMAGE_PREFIX=ziraloop-bridge-0-10-0
BRIDGE_HOST=ziraloop.outray.app

# Timeouts
SHARED_SANDBOX_IDLE_TIMEOUT_MINS=30
DEDICATED_SANDBOX_GRACE_PERIOD_MINS=5
```

### Infrastructure

```bash
# Docker compose running
make dev

# Templates already built
make build-templates VERSION=0.10.0 SIZE=small
```

### Tunnel

`https://ziraloop.outray.app` must be pointing to `localhost:8080`.

---

## Test 1: Shared Sandbox Creation (EnsureSharedSandbox)

**What it tests:** Full flow — register, create identity, create credential, create agent, and verify a shared sandbox gets created in Daytona with Bridge running inside it.

### Steps

```bash
# 1. Register and get API key
REG=$(curl -s -X POST http://localhost:8080/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"sandbox-test-'$(date +%s)'@test.local","password":"password123","name":"Sandbox Tester"}')
JWT=$(echo "$REG" | python3 -c "import sys,json; print(json.load(sys.stdin.buffer)['access_token'])")

KEY_RESP=$(curl -s -X POST http://localhost:8080/v1/api-keys \
  -H "Authorization: Bearer $JWT" \
  -H "Content-Type: application/json" \
  -d '{"name":"sandbox-test","scopes":["agents","credentials","all"]}')
API_KEY=$(echo "$KEY_RESP" | python3 -c "import sys,json; print(json.load(sys.stdin.buffer)['key'])")
echo "API Key: $API_KEY"

# 2. Create identity
IDENT=$(curl -s -X POST http://localhost:8080/v1/identities \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"external_id":"sandbox-test-user"}')
IDENT_ID=$(echo "$IDENT" | python3 -c "import sys,json; print(json.load(sys.stdin.buffer)['id'])")
echo "Identity: $IDENT_ID"

# 3. Create credential
source .env
CRED=$(curl -s -X POST http://localhost:8080/v1/credentials \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d "{\"provider_id\":\"anthropic\",\"api_key\":\"$ANTHROPIC_API_KEY\",\"label\":\"Test\",\"base_url\":\"https://api.anthropic.com\"}")
CRED_ID=$(echo "$CRED" | python3 -c "import sys,json; print(json.load(sys.stdin.buffer)['id'])")
echo "Credential: $CRED_ID"

# 4. Create a shared agent (this should trigger sandbox creation in Phase 6+)
# For now, the orchestrator is not yet wired to agent creation.
# We test the orchestrator directly via a Go test script below.
```

### Direct Orchestrator Test (Go)

Since the orchestrator isn't yet exposed via HTTP endpoints (that's Phase 6), test it with a Go integration test:

```bash
go test ./internal/sandbox/ -v -count=1 -timeout=10m -run TestRealDaytona
```

This requires the test file below.

---

## Test 2: Verify Daytona Sandbox

After a sandbox is created, verify:

### 2a. Sandbox exists in Daytona

Check the Daytona dashboard or API:
```bash
source .env
curl -s "https://app.daytona.io/api/sandbox" \
  -H "Authorization: Bearer $SANDBOX_PROVIDER_KEY" | python3 -m json.tool | grep zira
```

### 2b. Bridge is running inside the sandbox

Use the pre-auth URL from the sandbox record to hit Bridge's health endpoint:
```bash
# Get the BridgeURL from the sandbox record in Postgres
PGPASSWORD=localdev psql -h localhost -p 5433 -U ziraloop -d ziraloop \
  -c "SELECT id, bridge_url, status FROM sandboxes ORDER BY created_at DESC LIMIT 1;"

# Hit Bridge health endpoint
curl -s <BRIDGE_URL>/health
# Expected: {"status":"ok","uptime_secs":...}
```

### 2c. Bridge can reach ZiraLoop via tunnel

Check server logs for any incoming webhook test or push from Bridge. Or manually trigger:
```bash
# From inside sandbox (via Daytona SSH or exec):
curl -s https://ziraloop.outray.app/healthz
# Expected: {"status":"ok"}
```

---

## Test 3: Turso Database Provisioned

```bash
PGPASSWORD=localdev psql -h localhost -p 5433 -U ziraloop -d ziraloop \
  -c "SELECT org_id, turso_database_name, storage_url FROM workspace_storages ORDER BY created_at DESC LIMIT 5;"
```

Verify the Turso database exists:
```bash
source .env
curl -s "https://api.turso.tech/v1/organizations/$TURSO_ORG_SLUG/databases" \
  -H "Authorization: Bearer $TURSO_API_TOKEN" | python3 -m json.tool | grep zira
```

---

## Test 4: Sandbox Wake (Stop + Re-ensure)

```bash
# Stop the sandbox via Daytona
source .env
EXTERNAL_ID=$(PGPASSWORD=localdev psql -h localhost -p 5433 -U ziraloop -d ziraloop -t \
  -c "SELECT external_id FROM sandboxes WHERE status='running' ORDER BY created_at DESC LIMIT 1;" | xargs)

curl -s -X POST "https://app.daytona.io/api/sandbox/$EXTERNAL_ID/stop" \
  -H "Authorization: Bearer $SANDBOX_PROVIDER_KEY"

# Verify DB shows stopped (health checker should pick it up within 30s)
sleep 35
PGPASSWORD=localdev psql -h localhost -p 5433 -U ziraloop -d ziraloop \
  -c "SELECT id, status FROM sandboxes ORDER BY created_at DESC LIMIT 1;"

# Re-call EnsureSharedSandbox (via test or future API) — should wake it
# Bridge should auto-hydrate from libsql
```

---

## Test 5: Pre-Auth URL Refresh

```bash
# Expire the URL manually in DB
PGPASSWORD=localdev psql -h localhost -p 5433 -U ziraloop -d ziraloop \
  -c "UPDATE sandboxes SET bridge_url_expires_at = NOW() - INTERVAL '1 hour' WHERE status='running';"

# Next GetBridgeClient call should refresh it
# Trigger via the Go test or wait for Phase 6 endpoint
```

---

## Test 6: Multiple Identities Get Separate Sandboxes

Create two identities and verify each gets its own shared sandbox:

```bash
# Create identity 2
IDENT2=$(curl -s -X POST http://localhost:8080/v1/identities \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"external_id":"sandbox-test-user-2"}')
IDENT2_ID=$(echo "$IDENT2" | python3 -c "import sys,json; print(json.load(sys.stdin.buffer)['id'])")

# After Phase 6, creating agents for both identities should result in 2 sandboxes
PGPASSWORD=localdev psql -h localhost -p 5433 -U ziraloop -d ziraloop \
  -c "SELECT id, identity_id, sandbox_type, status FROM sandboxes ORDER BY created_at DESC;"
```

---

## Test 7: Health Checker Auto-Stop

```bash
# Set a very short idle timeout for testing
# In .env: SHARED_SANDBOX_IDLE_TIMEOUT_MINS=1

# Create a sandbox, then wait 2 minutes without activity
# Health checker should auto-stop it

PGPASSWORD=localdev psql -h localhost -p 5433 -U ziraloop -d ziraloop \
  -c "SELECT id, status, last_active_at, NOW() - last_active_at AS idle_duration FROM sandboxes;"
```

---

## Real Daytona Integration Test

Add this test to run against real Daytona (skip if env not set):

**File:** `internal/sandbox/orchestrator_real_test.go`

```go
//go:build integration

package sandbox

// Run with: go test ./internal/sandbox/ -v -tags=integration -run TestRealDaytona -timeout=10m
```

This test file should:
1. Skip if `SANDBOX_PROVIDER_KEY` is not set
2. Create a real Daytona sandbox using the orchestrator
3. Verify Bridge health endpoint responds
4. Stop the sandbox
5. Wake it
6. Delete it
7. Verify Turso database was created

---

## Cleanup

After testing, clean up:

```bash
# Delete sandboxes from DB
PGPASSWORD=localdev psql -h localhost -p 5433 -U ziraloop -d ziraloop \
  -c "DELETE FROM sandboxes;"

# Delete test Turso databases
source .env
curl -s "https://api.turso.tech/v1/organizations/$TURSO_ORG_SLUG/databases" \
  -H "Authorization: Bearer $TURSO_API_TOKEN" | python3 -c "
import sys, json
dbs = json.load(sys.stdin.buffer).get('databases', [])
for db in dbs:
    if db['Name'].startswith('zira-'):
        print(f'  Delete: {db[\"Name\"]}')
"

# Delete from Daytona
curl -s "https://app.daytona.io/api/sandbox" \
  -H "Authorization: Bearer $SANDBOX_PROVIDER_KEY" | python3 -c "
import sys, json
sbs = json.load(sys.stdin.buffer).get('items', [])
for sb in sbs:
    name = sb.get('name', '')
    if name.startswith('zira-'):
        print(f'  Delete: {name} (id={sb[\"id\"]})')
"
```

---

## Expected Results Summary

| Test | Expected |
|------|----------|
| Shared sandbox creation | Daytona sandbox running, Bridge healthy on port 25434, Turso DB created |
| Return existing | Same sandbox ID returned, no new Daytona sandbox |
| Wake stopped | Sandbox starts, Bridge auto-hydrates from libsql |
| Pre-auth URL refresh | New URL generated, old one expired |
| Multiple identities | Separate sandboxes per identity |
| Health checker auto-stop | Idle sandbox stopped after timeout |
| Dedicated sandbox | New sandbox per agent, agent_id set |
