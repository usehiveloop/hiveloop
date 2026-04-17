#!/usr/bin/env bash
# LSP smoke test: install an LSP, start bridge in a cloned repo, make an
# OpenRouter-backed agent call the lsp tool and verify the result.
#
# Required env: OPENROUTER_API_KEY
# Knobs:
#   LSP_ID            (default: typescript)
#   REPO_URL          (default: sindresorhus/is)
#   REPO_SUBDIR       (default: .)
#   LANG_HINT_FILE    (default: source/index.ts — passed to agent as file_path)
#   BRIDGE_PORT       (default: 9100)
#   MODEL             (default: openrouter/elephant-alpha)
#   AGENT_ID          (default: lsp-smoke)
#   WORK_DIR          (default: /tmp/lsp-test/workspaces/$LSP_ID)
#   SKIP_NPM_INSTALL  (default: 0; set 1 to skip)
#   EXTRA_INSTRUCTION (optional; appended to the instruction sent to the agent)
#   TURN_TIMEOUT      (default: 180s; overall wait for turn_completed)
set -uo pipefail

BRIDGE_REPO="${BRIDGE_REPO:-/Users/bahdcoder/code/useportal.bridge}"
BRIDGE_BIN="${BRIDGE_BIN:-${BRIDGE_REPO}/target/release/bridge}"

LSP_ID="${LSP_ID:-typescript}"
REPO_URL="${REPO_URL:-https://github.com/sindresorhus/is.git}"
REPO_SUBDIR="${REPO_SUBDIR:-.}"
LANG_HINT_FILE="${LANG_HINT_FILE:-source/index.ts}"
BRIDGE_PORT="${BRIDGE_PORT:-9100}"
MODEL="${MODEL:-openrouter/elephant-alpha}"
AGENT_ID="${AGENT_ID:-lsp-smoke}"
WORK_DIR="${WORK_DIR:-/tmp/lsp-test/workspaces/${LSP_ID}}"
SKIP_NPM_INSTALL="${SKIP_NPM_INSTALL:-0}"
EXTRA_INSTRUCTION="${EXTRA_INSTRUCTION:-}"
TURN_TIMEOUT="${TURN_TIMEOUT:-180}"

if [[ -z "${OPENROUTER_API_KEY:-}" ]]; then
    echo "ERR: OPENROUTER_API_KEY not set" >&2
    exit 2
fi

step() { printf "[%s][%s] ▶ %s\n" "$(date +%H:%M:%S)" "${LSP_ID}" "$*"; }
fail() { printf "[%s][%s] ✗ FAIL: %s\n" "$(date +%H:%M:%S)" "${LSP_ID}" "$*" >&2; cleanup; exit 1; }
pass() { printf "[%s][%s] ✓ %s\n" "$(date +%H:%M:%S)" "${LSP_ID}" "$*"; }

cleanup() {
    if [[ -n "${SSE_PID:-}" ]] && kill -0 "$SSE_PID" 2>/dev/null; then
        kill "$SSE_PID" 2>/dev/null || true
    fi
    if [[ -n "${BRIDGE_PID:-}" ]] && kill -0 "$BRIDGE_PID" 2>/dev/null; then
        kill "$BRIDGE_PID" 2>/dev/null || true
        sleep 1
        kill -9 "$BRIDGE_PID" 2>/dev/null || true
    fi
}
trap cleanup EXIT

REPO_DIR="${WORK_DIR}/repo"
SSE_LOG="${WORK_DIR}/sse.log"
BRIDGE_LOG="${WORK_DIR}/bridge.log"
PUSH_RESPONSE="${WORK_DIR}/push.json"
CONV_RESPONSE="${WORK_DIR}/conv.json"

step "workspace: ${WORK_DIR}"
rm -rf "${WORK_DIR}"
mkdir -p "${WORK_DIR}"

INSTALL_IDS="${LSP_ID}${EXTRA_LSPS:+,${EXTRA_LSPS}}"
step "bridge install-lsp ${INSTALL_IDS}"
INSTALL_LOG="${WORK_DIR}/install.log"
if ! "${BRIDGE_BIN}" install-lsp "${INSTALL_IDS}" >"${INSTALL_LOG}" 2>&1; then
    tail -20 "${INSTALL_LOG}" >&2
    fail "bridge install-lsp ${INSTALL_IDS} failed"
fi
pass "install OK"

step "clone ${REPO_URL}"
if ! git clone --depth 1 "${REPO_URL}" "${REPO_DIR}" >"${WORK_DIR}/clone.log" 2>&1; then
    tail -20 "${WORK_DIR}/clone.log" >&2
    fail "git clone failed"
fi
pass "cloned"
WORKDIR="${REPO_DIR}/${REPO_SUBDIR}"

cd "${WORKDIR}" || fail "cd ${WORKDIR}"

if [[ "${SKIP_NPM_INSTALL}" != "1" ]] && [[ -f package.json ]] && [[ "${LSP_ID}" =~ ^(typescript|eslint|biome|vue|svelte|astro|python|prisma|graphql|tailwindcss|dockerfile|yaml-ls|bash)$ ]]; then
    step "npm install (LSP context)"
    if ! npm install --no-audit --no-fund --silent >"${WORK_DIR}/npm.log" 2>&1; then
        echo "WARN: npm install had issues" >&2
        tail -10 "${WORK_DIR}/npm.log" >&2 || true
    fi
fi

step "start bridge on :${BRIDGE_PORT} cwd=${WORKDIR}"
# Ensure ~/.local/bin (where custom-installed LSPs land) is on PATH
mkdir -p "${HOME}/.local/bin"
EXTRA_PATH="${HOME}/.local/bin"
env -i \
    HOME="${HOME}" \
    PATH="${EXTRA_PATH}:${PATH}" \
    BRIDGE_LISTEN_ADDR="127.0.0.1:${BRIDGE_PORT}" \
    BRIDGE_CONTROL_PLANE_URL="http://127.0.0.1:65530" \
    BRIDGE_CONTROL_PLANE_API_KEY="<bridge-control-plane-api-key>" \
    BRIDGE_LOG_LEVEL="info" \
    "${BRIDGE_BIN}" > "${BRIDGE_LOG}" 2>&1 &
BRIDGE_PID=$!

for i in $(seq 1 45); do
    if curl -sf "http://127.0.0.1:${BRIDGE_PORT}/health" >/dev/null 2>&1; then
        pass "bridge healthy after ${i}s"
        break
    fi
    if ! kill -0 "$BRIDGE_PID" 2>/dev/null; then
        tail -50 "${BRIDGE_LOG}" >&2
        fail "bridge died during startup"
    fi
    sleep 1
done
if ! curl -sf "http://127.0.0.1:${BRIDGE_PORT}/health" >/dev/null 2>&1; then
    tail -50 "${BRIDGE_LOG}" >&2
    fail "bridge did not become healthy within 45s"
fi

step "push agent ${AGENT_ID}"
AGENT_JSON=$(cat <<EOF
{
  "id": "${AGENT_ID}",
  "name": "LSP Smoke Test Agent",
  "system_prompt": "You are a code navigator. Follow the user's instructions exactly. Call the 'lsp' tool as instructed without exploring other tools or the filesystem. Always produce a short final text response after tool calls.",
  "provider": {
    "provider_type": "open_ai",
    "model": "${MODEL}",
    "api_key": "${OPENROUTER_API_KEY}",
    "base_url": "https://openrouter.ai/api/v1"
  },
  "tools": [],
  "mcp_servers": [],
  "skills": [],
  "config": {
    "max_tokens": 2048,
    "max_turns": 10,
    "temperature": 0.0
  }
}
EOF
)
PUSH_HTTP=$(curl -s -o "${PUSH_RESPONSE}" -w "%{http_code}" \
    -X PUT "http://127.0.0.1:${BRIDGE_PORT}/push/agents/${AGENT_ID}" \
    -H "Authorization: Bearer <bridge-control-plane-api-key>" \
    -H "Content-Type: application/json" \
    -d "${AGENT_JSON}")
if [[ "${PUSH_HTTP}" != "200" && "${PUSH_HTTP}" != "201" ]]; then
    cat "${PUSH_RESPONSE}" >&2
    fail "push agent returned HTTP ${PUSH_HTTP}"
fi
pass "pushed: $(cat ${PUSH_RESPONSE})"

step "create conversation"
CONV_HTTP=$(curl -s -o "${CONV_RESPONSE}" -w "%{http_code}" \
    -X POST "http://127.0.0.1:${BRIDGE_PORT}/agents/${AGENT_ID}/conversations" \
    -H "Content-Type: application/json" \
    -d '{}')
if [[ "${CONV_HTTP}" != "201" && "${CONV_HTTP}" != "200" ]]; then
    cat "${CONV_RESPONSE}" >&2
    fail "create conversation returned HTTP ${CONV_HTTP}"
fi
CONV_ID=$(jq -r '.conversation_id' < "${CONV_RESPONSE}")
if [[ -z "${CONV_ID}" || "${CONV_ID}" == "null" ]]; then
    cat "${CONV_RESPONSE}" >&2
    fail "no conversation_id in response"
fi
pass "conversation: ${CONV_ID}"

step "start SSE stream"
curl -sN "http://127.0.0.1:${BRIDGE_PORT}/conversations/${CONV_ID}/stream" > "${SSE_LOG}" 2>&1 &
SSE_PID=$!
sleep 1

step "send message"
BASE_INSTR="You must call the lsp tool EXACTLY ONCE with these arguments: operation=\"documentSymbol\", file_path=\"${LANG_HINT_FILE}\". Do NOT explore the filesystem. Do NOT call bash, ls, read, or any other tool. Call ONLY the lsp tool. After the tool returns, write one sentence summarizing the top-level symbol names you found."
FULL_INSTR="${BASE_INSTR}${EXTRA_INSTRUCTION:+ ${EXTRA_INSTRUCTION}}"
MSG_JSON=$(jq -n --arg c "${FULL_INSTR}" '{content: $c}')
MSG_HTTP=$(curl -s -o /dev/null -w "%{http_code}" \
    -X POST "http://127.0.0.1:${BRIDGE_PORT}/conversations/${CONV_ID}/messages" \
    -H "Content-Type: application/json" \
    -d "${MSG_JSON}")
if [[ "${MSG_HTTP}" != "202" && "${MSG_HTTP}" != "200" ]]; then
    fail "send message returned HTTP ${MSG_HTTP}"
fi
pass "message sent"

step "wait up to ${TURN_TIMEOUT}s for turn_completed"
completed=0
for i in $(seq 1 "${TURN_TIMEOUT}"); do
    if grep -q "turn_completed\|event: error" "${SSE_LOG}" 2>/dev/null; then
        completed=1
        break
    fi
    if ! kill -0 "$BRIDGE_PID" 2>/dev/null; then
        tail -50 "${BRIDGE_LOG}" >&2
        fail "bridge died during turn"
    fi
    sleep 1
done
[[ $completed -eq 1 ]] || fail "turn did not complete within ${TURN_TIMEOUT}s"

step "inspect SSE log"
echo "--- SSE event types ---"
grep "^event:" "${SSE_LOG}" | sort | uniq -c | sort -rn
echo "--- tool call names ---"
grep -o '"name":"[^"]*"' "${SSE_LOG}" | sort | uniq -c
echo ""

if ! grep -q '"name":"lsp"' "${SSE_LOG}"; then
    echo "--- tail SSE log ---" >&2
    tail -40 "${SSE_LOG}" >&2
    fail "agent did not call the lsp tool"
fi

# Parse LSP result
LSP_RESULT=$(python3 - <<'PY' "${SSE_LOG}"
import json, re, sys
with open(sys.argv[1]) as f:
    text = f.read()
for block in text.split("\n\n"):
    m = re.search(r"^event:\s*(\S+)\s*\n\s*data:\s*(.+)$", block, re.DOTALL)
    if not m: continue
    if m.group(1) != "tool_call_result": continue
    try:
        data = json.loads(m.group(2))
    except Exception:
        continue
    d = data.get("data", {})
    # Find the one for lsp by looking at id; cross-ref with start events
# second pass: match id
lsp_call_ids = set()
for block in text.split("\n\n"):
    m = re.search(r"^event:\s*(\S+)\s*\n\s*data:\s*(.+)$", block, re.DOTALL)
    if not m: continue
    if m.group(1) == "tool_call_start":
        try:
            d = json.loads(m.group(2)).get("data", {})
        except Exception:
            continue
        if d.get("name") == "lsp":
            lsp_call_ids.add(d.get("id"))
for block in text.split("\n\n"):
    m = re.search(r"^event:\s*(\S+)\s*\n\s*data:\s*(.+)$", block, re.DOTALL)
    if not m: continue
    if m.group(1) != "tool_call_result": continue
    try:
        d = json.loads(m.group(2)).get("data", {})
    except Exception:
        continue
    if d.get("id") in lsp_call_ids:
        result = d.get("result", "")
        err = d.get("error")
        if err:
            print(f"LSP_ERROR: {err}")
        elif isinstance(result, str) and result.startswith("Toolset error"):
            print(f"LSP_ERROR: {result[:400]}")
        else:
            r = result if isinstance(result, str) else json.dumps(result)
            print(f"LSP_OK: {r[:400]}")
        break
PY
)
echo "--- lsp result ---"
echo "${LSP_RESULT}"

if [[ "${LSP_RESULT}" == LSP_ERROR:* ]]; then
    fail "lsp tool errored: ${LSP_RESULT}"
fi
if [[ "${LSP_RESULT}" != LSP_OK:* ]]; then
    fail "could not parse lsp tool result from SSE"
fi

step "SMOKE TEST PASS"
pass "LSP=${LSP_ID} MODEL=${MODEL}"
