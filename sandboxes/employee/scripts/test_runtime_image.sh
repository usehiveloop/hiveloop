#!/usr/bin/env bash
set -euo pipefail
export PATH="/usr/local/bin:/opt/homebrew/bin:$PATH"

IMAGE="${EMPLOYEE_BRIDGE_RUNTIME_IMAGE:-employee-bridge:runtime}"
NAME="${EMPLOYEE_BRIDGE_RUNTIME_CONTAINER:-employee-bridge-runtime-smoke}"
SECRET="${RUNTIME_SECRET:-runtime-test-token}"
DOCKER_BIN="${DOCKER_BIN:-$(command -v docker)}"
PLATFORM="${EMPLOYEE_BRIDGE_RUNTIME_PLATFORM:-$("$DOCKER_BIN" image inspect -f '{{.Os}}/{{.Architecture}}' "$IMAGE" 2>/dev/null || true)}"

"$DOCKER_BIN" rm -f "$NAME" >/dev/null 2>&1 || true

run_args=()
if [[ -n "$PLATFORM" ]]; then
  run_args+=(--platform "$PLATFORM")
fi

"$DOCKER_BIN" run -d \
  "${run_args[@]}" \
  --name "$NAME" \
  -p 17080:7080 \
  -e SLACK_BOT_TOKEN="${SLACK_BOT_TOKEN:-<slack-bot-token>}" \
  -e SLACK_APP_TOKEN="${SLACK_APP_TOKEN:-<slack-app-token>}" \
  -e RUNTIME_SECRET="$SECRET" \
  -e OPENROUTER_API_KEY="${OPENROUTER_API_KEY:-dummy}" \
  "$IMAGE" >/tmp/employee-bridge-runtime-container-id

cleanup() {
  "$DOCKER_BIN" logs "$NAME" >/tmp/employee-bridge-runtime-smoke.log 2>&1 || true
  "$DOCKER_BIN" rm -f "$NAME" >/dev/null 2>&1 || true
}
trap cleanup EXIT

for _ in $(seq 1 50); do
  if curl -fsS -H "Authorization: Bearer $SECRET" http://127.0.0.1:17080/config >/tmp/runtime-config.json 2>/dev/null; then
    break
  fi
  if ! "$DOCKER_BIN" ps --format '{{.Names}}' | grep -qx "$NAME"; then
    echo "container exited before control-plane API became ready" >&2
    "$DOCKER_BIN" logs "$NAME" >&2 || true
    exit 1
  fi
  sleep 0.1
done

test -s /tmp/runtime-config.json

python3 - <<'PY'
import json
cfg = json.load(open("/tmp/runtime-config.json"))
cfg["skills"] = [{
    "name": "runtime-smoke",
    "description": "Verify config skills materialize in packaged Debian runtime.",
    "trigger": {"type": "always"},
    "instructions": "Imported through /config in the runtime image.",
    "files": {"references/check.md": "# Runtime smoke ok"},
    "category": "testing",
    "tags": ["runtime", "debian"],
}]
json.dump(cfg, open("/tmp/runtime-config-updated.json", "w"))
PY

curl -fsS \
  -X PUT http://127.0.0.1:17080/config \
  -H "Authorization: Bearer $SECRET" \
  -H "Content-Type: application/json" \
  -d @/tmp/runtime-config-updated.json >/tmp/runtime-put-response.json

"$DOCKER_BIN" exec "$NAME" test -f /workspace/.skills/runtime-smoke/SKILL.md
"$DOCKER_BIN" exec "$NAME" test -f /workspace/.skills/runtime-smoke/references/check.md
"$DOCKER_BIN" exec "$NAME" grep -q "runtime-smoke" /workspace/.skills/runtime-smoke/SKILL.md
"$DOCKER_BIN" exec "$NAME" grep -q "Runtime smoke ok" /workspace/.skills/runtime-smoke/references/check.md

echo "runtime image smoke passed"
