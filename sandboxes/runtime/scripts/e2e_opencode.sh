#!/usr/bin/env bash
# End-to-end test against a Dockerized bridge running the OpenCode harness.
# Mirrors scripts/e2e_claude.sh: same six phases, same assertions, just
# with `harness: "open_code"` on the agent definition. opencode reads its
# config from /work/.opencode/opencode.json which bridge writes per-agent
# at session start.
#
# Required env (no fallback — opencode needs a real provider):
#   OPENCODE_PROVIDER_TYPE      bridge ProviderType (e.g. open_ai|anthropic)
#   OPENCODE_MODEL              model id; either bare ("gpt-4o") or "provider/model"
#   OPENCODE_API_KEY            api key (passed through to opencode.json)
#   OPENCODE_BASE_URL           optional; gateway base URL
#
# Plus standard flags:
#   --no-build  skip docker build
#   --keep      keep container after run for inspection
#
# Usage: scripts/e2e_opencode.sh [--no-build] [--keep]

set -euo pipefail

NO_BUILD=0
KEEP=0
while [[ $# -gt 0 ]]; do
    case "$1" in
        --no-build) NO_BUILD=1 ;;
        --keep) KEEP=1 ;;
        *) echo "unknown arg $1" >&2; exit 2 ;;
    esac
    shift
done

if [[ -z "${OPENCODE_PROVIDER_TYPE:-}" || -z "${OPENCODE_MODEL:-}" || -z "${OPENCODE_API_KEY:-}" ]]; then
    echo "✗ OPENCODE_PROVIDER_TYPE, OPENCODE_MODEL, OPENCODE_API_KEY are required" >&2
    exit 2
fi
: "${OPENCODE_BASE_URL:=}"
: "${BRIDGE_BASE_URL:=http://127.0.0.1:8080}"

CTRL_KEY="test-control-plane-key"
IMAGE_TAG="bridge-e2e:latest"
CONTAINER_NAME="bridge-e2e"

WEBHOOK_PID=""
cleanup() {
    if [[ -n "${WEBHOOK_PID}" ]]; then
        kill "${WEBHOOK_PID}" >/dev/null 2>&1 || true
    fi
    if [[ $KEEP -eq 1 ]]; then
        echo "→ keeping container ${CONTAINER_NAME} for inspection"
        return
    fi
    echo "→ tearing down container ${CONTAINER_NAME}"
    docker rm -f "${CONTAINER_NAME}" >/dev/null 2>&1 || true
    rm -f /tmp/bridge_events.* 2>/dev/null
}
trap cleanup EXIT

build_image() {
    echo "→ building docker image ${IMAGE_TAG}"
    DOCKER_BUILDKIT=1 docker build -f docker/Dockerfile -t "${IMAGE_TAG}" .
}

start_container() {
    local webhook_url="${1:-}"
    echo "→ removing any stale container"
    docker rm -f "${CONTAINER_NAME}" >/dev/null 2>&1 || true

    local webhook_arg=""
    if [[ -n "${webhook_url}" ]]; then
        webhook_arg="-e BRIDGE_WEBHOOK_URL=${webhook_url}"
    fi

    local sentry_args=()
    if [[ -n "${SENTRY_DSN:-}" ]]; then
        sentry_args+=(-e "SENTRY_DSN=${SENTRY_DSN}")
        sentry_args+=(-e "SENTRY_ENVIRONMENT=${SENTRY_ENVIRONMENT:-e2e}")
        sentry_args+=(-e "BRIDGE_INSTANCE_ID=${BRIDGE_INSTANCE_ID:-bridge-e2e-opencode}")
    fi

    echo "→ starting container${webhook_url:+ webhook=${webhook_url}}"
    docker run -d --name "${CONTAINER_NAME}" \
        -p 8080:8080 \
        ${webhook_arg} \
        "${sentry_args[@]}" \
        "${IMAGE_TAG}" >/dev/null

    echo "→ waiting for /health"
    for i in {1..30}; do
        if curl -fsS "${BRIDGE_BASE_URL}/health" >/dev/null 2>&1; then
            echo "  bridge healthy after ${i}s"
            return
        fi
        sleep 1
    done
    echo "✗ bridge failed to start" >&2
    docker logs "${CONTAINER_NAME}" >&2 || true
    exit 1
}

push_agent() {
    local permission_mode="$1"
    local mcp_servers_json="${2:-[]}"
    local skills_json="${3:-[]}"
    AGENT_ID="agent_test"

    local base_url_field=""
    if [[ -n "${OPENCODE_BASE_URL}" ]]; then
        base_url_field=", \"base_url\": \"${OPENCODE_BASE_URL}\""
    fi

    echo "→ pushing agent (harness=open_code, permission_mode=${permission_mode})"
    PUSH_BODY=$(cat <<JSON
{
  "agents": [
    {
      "id": "${AGENT_ID}",
      "name": "Test OpenCode",
      "harness": "open_code",
      "system_prompt": "You are a helpful, terse assistant. Always answer in under 50 words. When the user asks you to remember or recall something, use the available memory tools (memory_retain, memory_recall, memory_retrieve) instead of relying on your own context.",
      "provider": {
        "provider_type": "${OPENCODE_PROVIDER_TYPE}",
        "model": "${OPENCODE_MODEL}",
        "api_key": "${OPENCODE_API_KEY}"${base_url_field}
      },
      "mcp_servers": ${mcp_servers_json},
      "skills": ${skills_json},
      "config": {
        "permission_mode": "${permission_mode}"
      }
    }
  ]
}
JSON
)
    PUSH_RESP=$(curl -sS -w "\n%{http_code}" \
        -X POST "${BRIDGE_BASE_URL}/push/agents" \
        -H "content-type: application/json" \
        -H "authorization: Bearer ${CTRL_KEY}" \
        -d "${PUSH_BODY}")
    local code=$(echo "${PUSH_RESP}" | tail -n1)
    local body=$(echo "${PUSH_RESP}" | sed '$d')
    if [[ "${code}" != "200" ]]; then
        echo "✗ push/agents returned ${code}: ${body}" >&2
        docker logs "${CONTAINER_NAME}" >&2
        exit 1
    fi
    echo "  pushed: ${body}"
}

create_conversation() {
    CONV_RESP=$(curl -sS -X POST "${BRIDGE_BASE_URL}/agents/${AGENT_ID}/conversations" \
        -H "content-type: application/json" \
        -H "authorization: Bearer ${CTRL_KEY}" \
        -d '{}')
    CONV_ID=$(echo "${CONV_RESP}" | python3 -c "import sys,json;print(json.load(sys.stdin)['conversation_id'])")
    echo "  conversation_id=${CONV_ID}"
}

start_sse_subscriber() {
    EVENTS_FILE=$(mktemp /tmp/bridge_events.XXXXXX)
    echo "  events → ${EVENTS_FILE}"
    curl -sN "${BRIDGE_BASE_URL}/conversations/${CONV_ID}/stream" > "${EVENTS_FILE}" &
    SSE_PID=$!
    sleep 1
}

send_message() {
    local prompt="$1"
    local body=$(printf '{"content": %s}' "$(printf '%s' "${prompt}" | python3 -c 'import sys,json;print(json.dumps(sys.stdin.read()))')")
    curl -fsS -X POST "${BRIDGE_BASE_URL}/conversations/${CONV_ID}/messages" \
        -H "content-type: application/json" \
        -d "${body}" >/dev/null
}

wait_for_terminal_event() {
    local timeout="${1:-90}"
    local deadline=$((SECONDS + timeout))
    while (( SECONDS < deadline )); do
        if grep -q "event: turn_completed\|event: agent_error" "${EVENTS_FILE}" 2>/dev/null; then
            return 0
        fi
        sleep 1
    done
    echo "✗ timed out waiting for turn_completed" >&2
    docker logs "${CONTAINER_NAME}" 2>&1 | tail -100 >&2
    exit 1
}

stop_subscriber() {
    kill "${SSE_PID}" >/dev/null 2>&1 || true
    wait "${SSE_PID}" >/dev/null 2>&1 || true
}

dump_events() {
    echo "──── EVENTS (${1}) ────"
    cat "${EVENTS_FILE}"
    echo "──── /END ────"
}

assert_event() {
    local pattern="$1"
    local description="$2"
    if grep -q "${pattern}" "${EVENTS_FILE}"; then
        echo "  ✓ ${description}"
    else
        echo "  ✗ MISSING: ${description}" >&2
        dump_events "fail"
        docker logs "${CONTAINER_NAME}" 2>&1 | tail -100 >&2
        exit 1
    fi
}

# ──────────────────────────────────────────
# Phase 1: simple Q&A
# ──────────────────────────────────────────
if [[ $NO_BUILD -eq 0 ]]; then
    build_image
fi
start_container
push_agent "bypassPermissions"

echo
echo "═══ Phase 1: simple Q&A ═══"
create_conversation
start_sse_subscriber
send_message "What is 2+2? Reply with just the number."
wait_for_terminal_event 30
stop_subscriber
echo
assert_event "event: content_delta" "Phase 1: got content_delta (response_chunk)"
assert_event "event: turn_completed" "Phase 1: got turn_completed"

# ──────────────────────────────────────────
# Phase 2: tool call
# ──────────────────────────────────────────
echo
echo "═══ Phase 2: tool call ═══"
create_conversation
start_sse_subscriber
send_message "Use the bash tool right now. Execute exactly this command: echo HELLO_FROM_BRIDGE. After running it, tell me the exact output."
wait_for_terminal_event 45
stop_subscriber
echo
assert_event "event: tool_call_start" "Phase 2: got tool_call_start"
assert_event "event: tool_call_result" "Phase 2: got tool_call_result"
assert_event "event: turn_completed" "Phase 2: got turn_completed"

# ──────────────────────────────────────────
# Phase 3: approval flow
# ──────────────────────────────────────────
echo
echo "═══ Phase 3: approval flow ═══"
docker rm -f "${CONTAINER_NAME}" >/dev/null 2>&1 || true
start_container
push_agent "default"

create_conversation
start_sse_subscriber
send_message "Use the write tool to create a new file at /workspace/approved.txt with the contents: APPROVED_AND_WRITTEN. Then confirm the path you wrote."

echo "→ waiting for tool_approval_required (up to 30s)"
APPROVAL_DEADLINE=$((SECONDS + 30))
APPROVAL_REQ_ID=""
while (( SECONDS < APPROVAL_DEADLINE )); do
    APPROVAL_LINE=$(grep "event: tool_approval_required" "${EVENTS_FILE}" -A1 2>/dev/null | tail -1 || true)
    if [[ -n "${APPROVAL_LINE}" ]]; then
        REQS=$(curl -sS "${BRIDGE_BASE_URL}/agents/${AGENT_ID}/conversations/${CONV_ID}/approvals")
        APPROVAL_REQ_ID=$(echo "${REQS}" | python3 -c "import sys,json;j=json.load(sys.stdin);print(j[0]['id'] if j else '')")
        if [[ -n "${APPROVAL_REQ_ID}" ]]; then
            break
        fi
    fi
    sleep 1
done

if [[ -z "${APPROVAL_REQ_ID}" ]]; then
    echo "✗ no approval request appeared" >&2
    dump_events "phase3-fail"
    docker logs "${CONTAINER_NAME}" 2>&1 | tail -100 >&2
    exit 1
fi
echo "  approval id: ${APPROVAL_REQ_ID}"

echo "→ approving via API"
curl -fsS -X POST "${BRIDGE_BASE_URL}/agents/${AGENT_ID}/conversations/${CONV_ID}/approvals/${APPROVAL_REQ_ID}" \
    -H "content-type: application/json" \
    -d '{"decision": "approve"}' >/dev/null

wait_for_terminal_event 45
stop_subscriber
echo
assert_event "event: tool_approval_required" "Phase 3: got tool_approval_required"
assert_event "event: tool_approval_resolved" "Phase 3: got tool_approval_resolved"
assert_event "event: tool_call_result" "Phase 3: got tool_call_result"
assert_event "event: turn_completed" "Phase 3: got turn_completed"

# ──────────────────────────────────────────
# Phase 4: custom MCP (hiveloop memory)
# ──────────────────────────────────────────
echo
echo "═══ Phase 4: custom MCP (hiveloop memory) ═══"
docker rm -f "${CONTAINER_NAME}" >/dev/null 2>&1 || true
start_container

if [[ -z "${HIVELOOP_MCP_URL:-}" || -z "${HIVELOOP_MCP_TOKEN:-}" ]]; then
    echo "✗ HIVELOOP_MCP_URL and HIVELOOP_MCP_TOKEN are required for Phase 4" >&2
    exit 2
fi
HIVELOOP_MCP=$(python3 -c "
import json, os
print(json.dumps([{
    'name': 'hiveloop',
    'transport': {
        'type': 'streamable_http',
        'url': os.environ['HIVELOOP_MCP_URL'],
        'headers': {'Authorization': f\"Bearer {os.environ['HIVELOOP_MCP_TOKEN']}\"},
    },
}]))
")
push_agent "bypassPermissions" "${HIVELOOP_MCP}"

# Phase 4a — retain
echo "── 4a: retain a fact ──"
create_conversation
start_sse_subscriber
send_message "Use the memory_retain tool to save this fact: my favorite color is purple."
wait_for_terminal_event 45
stop_subscriber
echo
assert_event "memory_retain" "Phase 4a: tool_call mentions memory_retain"
assert_event "event: tool_call_result" "Phase 4a: got tool_call_result"
assert_event "event: turn_completed" "Phase 4a: got turn_completed"

# Phase 4b — recall
echo "── 4b: recall the fact ──"
create_conversation
start_sse_subscriber
send_message "What is my favorite color? Use the memory_recall or memory_retrieve tool to look it up."
wait_for_terminal_event 45
stop_subscriber
echo
if grep -q "memory_recall\|memory_retrieve" "${EVENTS_FILE}"; then
    echo "  ✓ Phase 4b: tool_call mentions memory_recall or memory_retrieve"
else
    echo "  ✗ MISSING: Phase 4b: memory_recall/retrieve" >&2
    dump_events "phase4b-fail"
    docker logs "${CONTAINER_NAME}" 2>&1 | tail -50 >&2
    exit 1
fi
assert_event "event: tool_call_result" "Phase 4b: got tool_call_result"
assert_event "event: turn_completed" "Phase 4b: got turn_completed"

# ──────────────────────────────────────────
# Phase 5: stop/start container, restore conversation
# ──────────────────────────────────────────
echo
echo "═══ Phase 5: stop/start, restore conversation ═══"

docker rm -f "${CONTAINER_NAME}" >/dev/null 2>&1 || true
start_container
push_agent "bypassPermissions"
create_conversation
SAVED_CONV_ID="${CONV_ID}"
start_sse_subscriber
send_message "Remember this token: PURPLE_LLAMA_42. Reply with 'noted'."
wait_for_terminal_event 30
stop_subscriber
echo
assert_event "event: turn_completed" "Phase 5a: pre-restart turn_completed"

echo "→ stopping container (preserves writable layer)"
docker stop "${CONTAINER_NAME}" >/dev/null
echo "→ restarting container"
docker start "${CONTAINER_NAME}" >/dev/null
echo "→ waiting for /health"
for i in {1..30}; do
    if curl -fsS "${BRIDGE_BASE_URL}/health" >/dev/null 2>&1; then
        echo "  bridge healthy after ${i}s"
        break
    fi
    sleep 1
done

RESTORED_AGENTS=$(curl -fsS "${BRIDGE_BASE_URL}/agents" | python3 -c "import sys,json;print(len(json.load(sys.stdin)))")
if [[ "${RESTORED_AGENTS}" != "1" ]]; then
    echo "✗ expected 1 restored agent, got ${RESTORED_AGENTS}" >&2
    docker logs "${CONTAINER_NAME}" 2>&1 | tail -50 >&2
    exit 1
fi
echo "  ✓ Phase 5b: agent restored from storage"

CONV_ID="${SAVED_CONV_ID}"
EVENTS_FILE=$(mktemp /tmp/bridge_events.XXXXXX)
curl -sN "${BRIDGE_BASE_URL}/conversations/${CONV_ID}/stream" > "${EVENTS_FILE}" &
SSE_PID=$!
sleep 1
send_message "What was the token I asked you to remember? Reply with just the token."
wait_for_terminal_event 45
stop_subscriber
echo
assert_event "event: content_delta" "Phase 5b: post-restart content_delta"
assert_event "event: turn_completed" "Phase 5b: post-restart turn_completed"
RECALLED=$(python3 -c "
import json
buf = []
for line in open('${EVENTS_FILE}'):
    line = line.strip()
    if not line.startswith('data: '): continue
    try:
        ev = json.loads(line[6:])
    except Exception:
        continue
    if ev.get('event_type') in ('response_chunk', 'reasoning_delta'):
        c = ev.get('data', {}).get('content', {})
        if isinstance(c, dict) and c.get('type') == 'text':
            buf.append(c.get('text', ''))
print(''.join(buf))
")
if echo "${RECALLED}" | grep -q "PURPLE_LLAMA_42"; then
    echo "  ✓ Phase 5b: response references the pre-restart token (PURPLE_LLAMA_42)"
else
    echo "  ✗ MISSING: Phase 5b: response should mention PURPLE_LLAMA_42" >&2
    echo "  reconstructed: ${RECALLED}" >&2
    dump_events "phase5b-fail"
    docker logs "${CONTAINER_NAME}" 2>&1 | tail -50 >&2
    exit 1
fi

# ──────────────────────────────────────────
# Phase 6: webhook delivery
# ──────────────────────────────────────────
echo
echo "═══ Phase 6: webhook delivery — no events lost ═══"

WEBHOOK_PORT=9099
WEBHOOK_OUT=$(mktemp -t bridge_webhook.XXXXXX)
mv "${WEBHOOK_OUT}" "${WEBHOOK_OUT}.jsonl"
WEBHOOK_OUT="${WEBHOOK_OUT}.jsonl"
echo "→ starting webhook receiver on :${WEBHOOK_PORT} → ${WEBHOOK_OUT}"
python3 scripts/webhook_receiver.py "${WEBHOOK_PORT}" "${WEBHOOK_OUT}" >/dev/null 2>&1 &
WEBHOOK_PID=$!
sleep 1

docker rm -f "${CONTAINER_NAME}" >/dev/null 2>&1 || true
start_container "http://host.docker.internal:${WEBHOOK_PORT}/"
push_agent "bypassPermissions"

create_conversation
start_sse_subscriber
send_message "Reply with the single word: PING."
wait_for_terminal_event 60
echo "→ second turn (same SSE subscriber)"
send_message "Reply with the single word: PONG."
echo "→ waiting for second turn_completed (up to 60s)"
TURN_DEADLINE=$((SECONDS + 60))
while (( SECONDS < TURN_DEADLINE )); do
    n=$(grep -c "event: turn_completed" "${EVENTS_FILE}" 2>/dev/null || true)
    if [[ "${n}" -ge 2 ]]; then
        echo "  saw 2 turn_completed events"
        break
    fi
    sleep 1
done
stop_subscriber

sleep 3

WEBHOOK_COUNT=$(wc -l < "${WEBHOOK_OUT}" | tr -d ' ')
echo "  webhook events received: ${WEBHOOK_COUNT}"
if [[ "${WEBHOOK_COUNT}" == "0" ]]; then
    echo "  ✗ MISSING: no webhook deliveries received" >&2
    docker logs "${CONTAINER_NAME}" 2>&1 | tail -50 >&2
    exit 1
fi
echo "  ✓ Phase 6: webhook receiver got ${WEBHOOK_COUNT} events"

GAP_CHECK=$(python3 -c "
import json, sys
seqs = []
for line in open('${WEBHOOK_OUT}'):
    line = line.strip()
    if not line: continue
    try:
        ev = json.loads(line)
        seqs.append(int(ev['sequence_number']))
    except Exception as e:
        print('parse_error:', e, file=sys.stderr); sys.exit(1)
seqs.sort()
if seqs[0] != 1:
    print(f'gap_at_start: first seq is {seqs[0]} not 1'); sys.exit(1)
gaps = [(a, b) for a, b in zip(seqs, seqs[1:]) if b - a > 1]
if gaps:
    print(f'gaps: {gaps[:5]}'); sys.exit(1)
print(f'monotonic 1..{seqs[-1]}, count={len(seqs)}')
")
if [[ $? -ne 0 ]]; then
    echo "  ✗ MISSING: ${GAP_CHECK}" >&2
    head -5 "${WEBHOOK_OUT}" >&2
    exit 1
fi
echo "  ✓ Phase 6: sequence numbers are ${GAP_CHECK}"

TURN_COUNT=$(grep -c '"event_type": "turn_completed"' "${WEBHOOK_OUT}" || true)
if [[ "${TURN_COUNT}" -lt 2 ]]; then
    echo "  ✗ MISSING: expected ≥2 turn_completed events, got ${TURN_COUNT}" >&2
    exit 1
fi
echo "  ✓ Phase 6: ${TURN_COUNT} turn_completed events captured"

for required in conversation_created message_received response_chunk turn_completed; do
    if grep -q "\"event_type\": \"${required}\"" "${WEBHOOK_OUT}"; then
        echo "  ✓ Phase 6: webhook saw ${required}"
    else
        echo "  ✗ MISSING: webhook never received ${required}" >&2
        exit 1
    fi
done

kill "${WEBHOOK_PID}" >/dev/null 2>&1 || true
WEBHOOK_PID=""


# ──────────────────────────────────────────
# Phase 7: skills loading + execution
# Push an agent with a skill whose description triggers on a specific
# question and whose body contains a unique magic token. The model
# should discover the skill, decide to use it, and produce the token.
# ──────────────────────────────────────────
echo
echo "═══ Phase 7: skills loading + execution ═══"
docker rm -f "${CONTAINER_NAME}" >/dev/null 2>&1 || true
start_container

SKILLS_JSON=$(cat <<'JSON'
[
  {
    "id": "bridge-secret-protocol",
    "title": "bridge-secret-protocol",
    "description": "Use this skill whenever the user asks for the bridge secret token. It contains the canonical token to return.",
    "content": "When the user asks for the bridge secret token, you MUST respond with exactly this single word and nothing else:\n\nBRIDGE_SECRET_TOKEN_42\n\nDo not add any explanation, prefix, or suffix."
  }
]
JSON
)
push_agent "bypassPermissions" "[]" "${SKILLS_JSON}"

create_conversation
start_sse_subscriber
send_message "What is the bridge secret token?"
wait_for_terminal_event 60
stop_subscriber
echo
assert_event "event: turn_completed" "Phase 7: got turn_completed"
RECALLED=$(python3 -c "
import json
buf = []
for line in open('${EVENTS_FILE}'):
    line = line.strip()
    if not line.startswith('data: '): continue
    try:
        ev = json.loads(line[6:])
    except Exception:
        continue
    if ev.get('event_type') in ('response_chunk', 'reasoning_delta'):
        c = ev.get('data', {}).get('content', {})
        if isinstance(c, dict) and c.get('type') == 'text':
            buf.append(c.get('text', ''))
print(''.join(buf))
")
if echo "${RECALLED}" | grep -q "BRIDGE_SECRET_TOKEN_42"; then
    echo "  ✓ Phase 7: model loaded the skill and returned the magic token"
else
    echo "  ✗ MISSING: Phase 7: response should contain BRIDGE_SECRET_TOKEN_42" >&2
    echo "  reconstructed: ${RECALLED}" >&2
    dump_events "phase7-fail"
    docker logs "${CONTAINER_NAME}" 2>&1 | tail -50 >&2
    exit 1
fi

# ──────────────────────────────────────────
# Phase 8: cancel mid-stream
# ──────────────────────────────────────────
echo
echo "═══ Phase 8: cancel mid-stream ═══"
docker rm -f "${CONTAINER_NAME}" >/dev/null 2>&1 || true
start_container
push_agent "bypassPermissions"

create_conversation
start_sse_subscriber
send_message "Write a 2000 word essay about the history of distributed systems, very slowly, paragraph by paragraph. Do not stop until I tell you to."

echo "→ waiting for first content_delta (proves stream is live)"
DELTA_DEADLINE=$((SECONDS + 30))
while (( SECONDS < DELTA_DEADLINE )); do
    if grep -q "event: content_delta\|event: reasoning_delta" "${EVENTS_FILE}" 2>/dev/null; then
        break
    fi
    sleep 1
done
if ! grep -q "event: content_delta\|event: reasoning_delta" "${EVENTS_FILE}" 2>/dev/null; then
    echo "✗ never saw a delta before timeout — model produced no output to cancel" >&2
    dump_events "phase8-no-delta"
    docker logs "${CONTAINER_NAME}" 2>&1 | tail -100 >&2
    exit 1
fi

echo "→ aborting conversation"
ABORT_T0=$SECONDS
curl -fsS -X POST "${BRIDGE_BASE_URL}/conversations/${CONV_ID}/abort" \
    -H "content-type: application/json" >/dev/null

wait_for_terminal_event 30
ABORT_ELAPSED=$((SECONDS - ABORT_T0))
stop_subscriber
echo "  abort → terminal in ${ABORT_ELAPSED}s"

LAST_TURN=$(grep -A1 "event: turn_completed" "${EVENTS_FILE}" | tail -1 || true)
STOP=$(echo "${LAST_TURN}" | python3 -c "
import sys, json
line = sys.stdin.read().strip()
if line.startswith('data: '):
    line = line[6:]
try:
    j = json.loads(line)
    print(j.get('data', {}).get('stop_reason', ''))
except Exception:
    print('')
")
echo "  stop_reason: '${STOP}'"

# OpenCode's ACP layer reports stop_reason=EndTurn even after a cancel —
# it doesn't propagate the cancellation through PromptResponse. We can't
# rely on stop_reason here, so the load-bearing assertion is elapsed time:
# a 2000-word essay can't possibly complete in a handful of seconds, so a
# fast terminal proves the cancel actually shortened generation.
if (( ABORT_ELAPSED <= 10 )); then
    echo "  ✓ Phase 8: turn terminated quickly after abort (${ABORT_ELAPSED}s ≤ 10s)"
else
    echo "  ✗ Phase 8: terminal took ${ABORT_ELAPSED}s after abort — cancel did not shorten generation" >&2
    dump_events "phase8-slow-abort"
    docker logs "${CONTAINER_NAME}" 2>&1 | tail -50 >&2
    exit 1
fi
case "${STOP}" in
    cancelled|canceled|interrupted|aborted)
        echo "  ✓ Phase 8: stop_reason also signaled cancellation"
        ;;
    *)
        echo "  ⚠ Phase 8: stop_reason='${STOP}' (opencode known to report endturn on cancel)"
        ;;
esac


# ──────────────────────────────────────────
# Phase 9: deny tool approval
# ──────────────────────────────────────────
echo
echo "═══ Phase 9: deny tool approval ═══"
docker rm -f "${CONTAINER_NAME}" >/dev/null 2>&1 || true
start_container
push_agent "default"

docker exec "${CONTAINER_NAME}" sh -c 'rm -f /workspace/denied.txt' >/dev/null 2>&1 || true

create_conversation
start_sse_subscriber
send_message "Use the write tool to create the file /workspace/denied.txt with contents: SHOULD_NOT_EXIST. Then confirm you wrote it."

echo "→ waiting for tool_approval_required (up to 30s)"
APPROVAL_DEADLINE=$((SECONDS + 30))
APPROVAL_REQ_ID=""
while (( SECONDS < APPROVAL_DEADLINE )); do
    if grep -q "event: tool_approval_required" "${EVENTS_FILE}" 2>/dev/null; then
        REQS=$(curl -sS "${BRIDGE_BASE_URL}/agents/${AGENT_ID}/conversations/${CONV_ID}/approvals")
        APPROVAL_REQ_ID=$(echo "${REQS}" | python3 -c "import sys,json;j=json.load(sys.stdin);print(j[0]['id'] if j else '')")
        if [[ -n "${APPROVAL_REQ_ID}" ]]; then
            break
        fi
    fi
    sleep 1
done

if [[ -z "${APPROVAL_REQ_ID}" ]]; then
    echo "✗ no approval request appeared" >&2
    dump_events "phase9-no-approval"
    docker logs "${CONTAINER_NAME}" 2>&1 | tail -100 >&2
    exit 1
fi
echo "  approval id: ${APPROVAL_REQ_ID}"

echo "→ denying via API"
curl -fsS -X POST "${BRIDGE_BASE_URL}/agents/${AGENT_ID}/conversations/${CONV_ID}/approvals/${APPROVAL_REQ_ID}" \
    -H "content-type: application/json" \
    -d '{"decision": "deny"}' >/dev/null

wait_for_terminal_event 60
stop_subscriber
echo

assert_event "event: tool_approval_required" "Phase 9: got tool_approval_required"
assert_event "event: tool_approval_resolved" "Phase 9: got tool_approval_resolved"
assert_event "event: turn_completed" "Phase 9: got turn_completed"

if docker exec "${CONTAINER_NAME}" sh -c 'test -e /workspace/denied.txt' 2>/dev/null; then
    echo "  ✗ Phase 9: /workspace/denied.txt exists — deny did not block the write" >&2
    docker exec "${CONTAINER_NAME}" sh -c 'ls -la /workspace/denied.txt; cat /workspace/denied.txt' >&2
    dump_events "phase9-file-written"
    exit 1
fi
echo "  ✓ Phase 9: /workspace/denied.txt was not created"


echo
echo "✓✓✓ E2E PASSED (Phases 1, 2, 3, 4, 5, 6, 7, 8, 9) ✓✓✓"
