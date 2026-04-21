# Phase 5+6 Real Integration Test Plan

Tests the full end-to-end flow: API → Daytona sandbox → Bridge startup → Agent push/update/delete.

## Prerequisites

### Environment Variables

All must be set in `.env`:

```bash
# Daytona
SANDBOX_PROVIDER_ID=daytona
SANDBOX_PROVIDER_URL=https://app.daytona.io/api
SANDBOX_PROVIDER_KEY=<your daytona api key>
SANDBOX_TARGET=us

# Turso (optional — sandboxes work without it, no libsql persistence)
# TURSO_API_TOKEN=<your turso platform api token>
# TURSO_ORG_SLUG=<your turso org slug>
# TURSO_GROUP=default

# Sandbox encryption (generate with: openssl rand -base64 32)
SANDBOX_ENCRYPTION_KEY=<base64 encoded 32 bytes>

# Bridge
BRIDGE_BASE_IMAGE_PREFIX=hiveloop-bridge-0-10-0
BRIDGE_HOST=hiveloop.outray.app

# Anthropic (for creating real credentials)
ANTHROPIC_API_KEY=<your anthropic api key>
```

### Infrastructure

```bash
# Docker compose running (Postgres + Redis)
docker compose up -d postgres redis mailpit

# Templates already built (at least "small")
make build-templates VERSION=0.10.0 SIZE=small

# Tunnel active (sandbox → your local machine)
# https://hiveloop.outray.app must point to localhost:8080

# Server running
make dev
```

### Verify server started with orchestrator

Check the `make dev` terminal output for:
```
msg="sandbox config check" provider_key_set=true encryption_key_set=true
msg="sandbox orchestrator ready"
```

If you see `provider_key_set=false`, the env vars aren't loaded — restart the server.

### Clean slate

Before testing, ensure no leftover sandboxes in Daytona or your DB:

```bash
# Check Daytona for leftover sandboxes
SANDBOX_KEY=$(grep "^SANDBOX_PROVIDER_KEY=" .env | cut -d= -f2- | tr -d '[:space:]')
SANDBOX_URL=$(grep "^SANDBOX_PROVIDER_URL=" .env | cut -d= -f2- | tr -d '[:space:]')
curl -s "${SANDBOX_URL}/sandbox/paginated" -H "Authorization: Bearer ${SANDBOX_KEY}" | python3 -c "
import sys,json
items = json.load(sys.stdin.buffer).get('items', [])
for sb in items:
    if sb.get('name','').startswith('hiveloop-'):
        print(f'  {sb[\"name\"]}  id={sb[\"id\"]}  state={sb[\"state\"]}')
        print(f'    DELETE: curl -X DELETE \"{sb.get(\"id\")}\"')
print(f'Total hiveloop sandboxes: {sum(1 for s in items if s.get(\"name\",\"\").startswith(\"hiveloop-\"))}')
"

# Delete any leftover sandboxes
# curl -s -X DELETE "${SANDBOX_URL}/sandbox/<ID>" -H "Authorization: Bearer ${SANDBOX_KEY}"

# Clean local DB
PGPASSWORD=localdev psql -h localhost -p 5433 -U hiveloop -d hiveloop -c \
  "DELETE FROM conversation_events; DELETE FROM agent_conversations; DELETE FROM sandboxes; DELETE FROM agents;"
```

---

## Test Script

> **Important:** Do NOT use `source .env` in scripts — the base64 `SANDBOX_ENCRYPTION_KEY`
> contains `=` and `/` which corrupt the shell. Use `grep` to extract individual values.

### Step 1: Setup — Register, API Key, Identity, Credential

```bash
ANTHROPIC_KEY=$(grep "^ANTHROPIC_API_KEY=" .env | cut -d= -f2- | tr -d '[:space:]')

# Register
REG=$(curl -s -X POST http://localhost:8080/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"sandbox-test-'$(date +%s)'@test.local","password":"password123","name":"Sandbox Tester"}')
JWT=$(echo "$REG" | python3 -c "import sys,json; print(json.load(sys.stdin.buffer)['access_token'])")

# Create API key with agents + credentials + all scopes
KEY_RESP=$(curl -s -X POST http://localhost:8080/v1/api-keys \
  -H "Authorization: Bearer $JWT" \
  -H "Content-Type: application/json" \
  -d '{"name":"sandbox-test","scopes":["agents","credentials","all"]}')
API_KEY=$(echo "$KEY_RESP" | python3 -c "import sys,json; print(json.load(sys.stdin.buffer)['key'])")
echo "API Key: ${API_KEY:0:24}..."

# Create identity
IDENT=$(curl -s -X POST http://localhost:8080/v1/identities \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"external_id":"test-user-1"}')
IDENT_ID=$(echo "$IDENT" | python3 -c "import sys,json; print(json.load(sys.stdin.buffer)['id'])")
echo "Identity: $IDENT_ID"

# Create credential (Anthropic)
CRED=$(curl -s -X POST http://localhost:8080/v1/credentials \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d "{\"provider_id\":\"anthropic\",\"api_key\":\"$ANTHROPIC_KEY\",\"label\":\"Anthropic\",\"base_url\":\"https://api.anthropic.com\"}")
CRED_ID=$(echo "$CRED" | python3 -c "import sys,json; print(json.load(sys.stdin.buffer)['id'])")
echo "Credential: $CRED_ID"
```

### Step 2: Create shared agent (TRIGGERS SANDBOX CREATION)

This is the big one. It will:
1. Create a Daytona sandbox from the `hiveloop-bridge-0-10-0-small` snapshot
2. Start Bridge inside the sandbox (`nohup /usr/local/bin/bridge ...`)
3. Wait for Bridge `/health` to respond 200
4. Mint a proxy token (`ptok_...`) for the agent's credential
5. Push the agent definition to Bridge

**Expect ~5-10 seconds** (Daytona snapshot creation + Bridge startup).

```bash
A1=$(curl -s --max-time 300 -X POST http://localhost:8080/v1/agents \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d "{
    \"name\": \"support-bot\",
    \"description\": \"Customer support agent\",
    \"identity_id\": \"$IDENT_ID\",
    \"credential_id\": \"$CRED_ID\",
    \"sandbox_type\": \"shared\",
    \"system_prompt\": \"You are a helpful customer support agent.\",
    \"model\": \"claude-sonnet-4-20250514\",
    \"agent_config\": {\"max_tokens\": 4096, \"temperature\": 0.3},
    \"permissions\": {\"bash\": \"require_approval\"}
  }")
echo "$A1" | python3 -m json.tool
A1_ID=$(echo "$A1" | python3 -c "import sys,json; print(json.load(sys.stdin.buffer)['id'])")
```

**Check server logs** — you should see:
```
msg="starting bridge in sandbox" sandbox_id=... external_id=...
msg="bridge healthy" sandbox_id=... attempts=1 elapsed=...
msg="sandbox created" sandbox_id=... external_id=... type=shared
msg="agent pushed to bridge" agent_id=... agent_name=support-bot sandbox_id=...
```

### Step 3: Verify sandbox in DB

```bash
PGPASSWORD=localdev psql -h localhost -p 5433 -U hiveloop -d hiveloop -c \
  "SELECT id, status, sandbox_type, external_id, substring(bridge_url, 1, 70) as url FROM sandboxes;"
```

Expected: 1 row, status=`running`, sandbox_type=`shared`.

### Step 4: Verify Bridge is alive and has the agent

```bash
BRIDGE_URL=$(PGPASSWORD=localdev psql -h localhost -p 5433 -U hiveloop -d hiveloop -t -A -c \
  "SELECT bridge_url FROM sandboxes WHERE status='running' LIMIT 1;")

# Health check
curl -s "$BRIDGE_URL/health" | python3 -m json.tool
# Expected: {"status":"ok","uptime_secs":...}

# List agents in Bridge
curl -s "$BRIDGE_URL/agents" | python3 -m json.tool
# Expected: 1 agent with name "support-bot", provider pointing to our proxy
```

Key things to verify in the agent listing:
- `provider.base_url` = `https://hiveloop.outray.app/v1/proxy` (our proxy)
- `provider.provider_type` = `anthropic`
- `provider.api_key` starts with `ptok_` (minted proxy token)
- MCP server `hiveloop` is configured

### Step 5: Create second shared agent (REUSES SANDBOX)

```bash
A2=$(curl -s --max-time 60 -X POST http://localhost:8080/v1/agents \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d "{
    \"name\": \"coding-bot\",
    \"identity_id\": \"$IDENT_ID\",
    \"credential_id\": \"$CRED_ID\",
    \"sandbox_type\": \"shared\",
    \"system_prompt\": \"You write code.\",
    \"model\": \"claude-sonnet-4-20250514\"
  }")
A2_ID=$(echo "$A2" | python3 -c "import sys,json; print(json.load(sys.stdin.buffer)['id'])")
echo "Agent 2: $A2_ID"

# Verify Bridge now has 2 agents
curl -s "$BRIDGE_URL/agents" | python3 -c "
import sys,json
agents = json.load(sys.stdin.buffer)
print(f'Agents in Bridge: {len(agents)}')
for a in agents:
    print(f'  - {a[\"name\"]} ({a[\"id\"]})')
"
```

Expected: 2 agents, NO new sandbox created (same sandbox reused). Should return in <1 second.

### Step 6: Update first agent

```bash
curl -s --max-time 30 -X PUT "http://localhost:8080/v1/agents/$A1_ID" \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"system_prompt":"You are an UPDATED support agent."}' | python3 -m json.tool
```

Verify the update was pushed to Bridge:
```bash
curl -s "$BRIDGE_URL/agents/$A1_ID" | python3 -c "
import sys,json
a = json.load(sys.stdin.buffer)
print(f'Name: {a[\"name\"]}')
print(f'Prompt: {a[\"system_prompt\"]}')
"
# Expected: "You are an UPDATED support agent."
```

### Step 7: Delete first agent

```bash
curl -s -X DELETE "http://localhost:8080/v1/agents/$A1_ID" \
  -H "Authorization: Bearer $API_KEY" | python3 -m json.tool
# Expected: {"status":"deleted"}

# Verify removed from Bridge
curl -s "$BRIDGE_URL/agents" | python3 -c "
import sys,json
agents = json.load(sys.stdin.buffer)
print(f'Agents in Bridge: {len(agents)}')
for a in agents:
    print(f'  - {a[\"name\"]}')
"
# Expected: 1 agent (coding-bot only)
```

### Step 8: Create dedicated agent (NO sandbox)

```bash
A3=$(curl -s --max-time 10 -X POST http://localhost:8080/v1/agents \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d "{
    \"name\": \"dedicated-coder\",
    \"identity_id\": \"$IDENT_ID\",
    \"credential_id\": \"$CRED_ID\",
    \"sandbox_type\": \"dedicated\",
    \"system_prompt\": \"You code in isolated sandboxes.\",
    \"model\": \"claude-sonnet-4-20250514\"
  }")
echo "$A3" | python3 -m json.tool

# Verify NO new sandbox was created
PGPASSWORD=localdev psql -h localhost -p 5433 -U hiveloop -d hiveloop -t -A -c \
  "SELECT COUNT(*) FROM sandboxes;"
# Expected: 1 (only the shared sandbox)
```

Dedicated agents don't create sandboxes on agent creation — sandbox is created lazily
when a conversation is started (Phase 7).

### Step 9: List all agents

```bash
curl -s "http://localhost:8080/v1/agents" \
  -H "Authorization: Bearer $API_KEY" | python3 -c "
import sys,json
data = json.load(sys.stdin.buffer)
for a in data['data']:
    print(f'  - {a[\"name\"]} (type={a[\"sandbox_type\"]}, status={a[\"status\"]})')
"
# Expected:
#   - dedicated-coder (type=dedicated, status=active)
#   - coding-bot (type=shared, status=active)
```

---

## Expected Results Summary

| Test | Expected |
|------|----------|
| Create shared agent | Daytona sandbox created, Bridge healthy in ~5-10s, agent pushed |
| Verify sandbox in DB | 1 running shared sandbox |
| Verify Bridge alive | `/health` ok, agent listed with proxy config |
| Second shared agent | Same sandbox reused, Bridge shows 2 agents, <1s |
| Update agent | Prompt updated in Bridge |
| Delete agent | Removed from Bridge, 1 agent left |
| Dedicated agent | Created in DB only, sandbox count still 1 |
| List agents | 2 agents: 1 shared + 1 dedicated |

---

## Cleanup

```bash
# Delete agents from HiveLoop
curl -s -X DELETE "http://localhost:8080/v1/agents/$A2_ID" -H "Authorization: Bearer $API_KEY"
curl -s -X DELETE "http://localhost:8080/v1/agents/$A3_ID" -H "Authorization: Bearer $API_KEY"

# Delete sandbox from Daytona
SANDBOX_KEY=$(grep "^SANDBOX_PROVIDER_KEY=" .env | cut -d= -f2- | tr -d '[:space:]')
SANDBOX_URL=$(grep "^SANDBOX_PROVIDER_URL=" .env | cut -d= -f2- | tr -d '[:space:]')
EXTERNAL_ID=$(PGPASSWORD=localdev psql -h localhost -p 5433 -U hiveloop -d hiveloop -t -A -c \
  "SELECT external_id FROM sandboxes LIMIT 1;")
curl -s -X DELETE "${SANDBOX_URL}/sandbox/${EXTERNAL_ID}" -H "Authorization: Bearer ${SANDBOX_KEY}"

# Clean local DB
PGPASSWORD=localdev psql -h localhost -p 5433 -U hiveloop -d hiveloop -c \
  "DELETE FROM sandboxes; DELETE FROM agents; DELETE FROM workspace_storages;"
```

---

## Troubleshooting

### "Sandbox with name already exists" (409)
A previous sandbox wasn't cleaned up from Daytona. List and delete it:
```bash
curl -s "${SANDBOX_URL}/sandbox/paginated" -H "Authorization: Bearer ${SANDBOX_KEY}" | python3 -m json.tool
curl -s -X DELETE "${SANDBOX_URL}/sandbox/<ID>" -H "Authorization: Bearer ${SANDBOX_KEY}"
```

### "null value in column bridge_api_key"
Old schema has the deprecated `bridge_api_key` column. Drop it:
```bash
PGPASSWORD=localdev psql -h localhost -p 5433 -U hiveloop -d hiveloop -c \
  "ALTER TABLE sandboxes DROP COLUMN IF EXISTS bridge_api_key;"
```

### Push fails silently (agent created but no sandbox)
Check server logs for `"failed to push agent to bridge"`. Common causes:
- Orchestrator not initialized (check for `"sandbox orchestrator ready"` in startup logs)
- Turso 403 (plan limit) — comment out `TURSO_API_TOKEN` and `TURSO_ORG_SLUG` in `.env`
- Daytona API error — check the error message in the log

### Bridge health check fails (29+ attempts)
Bridge is running but the URL is not reachable. Previous issue was using unsigned
preview URLs for private sandboxes. Fixed: now uses Daytona's signed preview URL API
(`GET /sandbox/{id}/ports/{port}/signed-preview-url`).

### `source .env` corrupts shell
The `SANDBOX_ENCRYPTION_KEY` contains `=` and `/` (base64). Don't `source .env` directly.
Use `grep` to extract individual values:
```bash
ANTHROPIC_KEY=$(grep "^ANTHROPIC_API_KEY=" .env | cut -d= -f2- | tr -d '[:space:]')
```
