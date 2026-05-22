#!/usr/bin/env bash
set -euo pipefail
export PATH="/usr/local/bin:/opt/homebrew/bin:$PATH"

IMAGE="${EMPLOYEE_BRIDGE_RUNTIME_IMAGE:-employee-bridge:runtime}"
NAME="${EMPLOYEE_BRIDGE_RUNTIME_CONTAINER:-employee-bridge-runtime-smoke}"
AUTH_NAME="${EMPLOYEE_BRIDGE_RUNTIME_AUTH_CONTAINER:-$NAME-auth}"
SECRET="${RUNTIME_SECRET:-runtime-test-token}"
DOCKER_BIN="${DOCKER_BIN:-$(command -v docker)}"
PLATFORM="${EMPLOYEE_BRIDGE_RUNTIME_PLATFORM:-$("$DOCKER_BIN" image inspect -f '{{.Os}}/{{.Architecture}}' "$IMAGE" 2>/dev/null || true)}"
TMP_DIR="$(mktemp -d)"

cat >"$TMP_DIR/git_credentials_mock.py" <<'PY'
import json
import sys
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

log_path = sys.argv[1]


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path != "/health":
            self.send_response(404)
            self.end_headers()
            return
        self.send_response(204)
        self.end_headers()

    def do_POST(self):
        length = int(self.headers.get("Content-Length", "0") or "0")
        raw = self.rfile.read(length)
        try:
            body = json.loads(raw.decode("utf-8") or "{}")
        except json.JSONDecodeError:
            body = {"_raw": raw.decode("utf-8", errors="replace")}

        with open(log_path, "a", encoding="utf-8") as handle:
            json.dump(
                {
                    "path": self.path,
                    "authorization": self.headers.get("Authorization"),
                    "content_type": self.headers.get("Content-Type", ""),
                    "body": body,
                },
                handle,
            )
            handle.write("\n")

        argv = body.get("operation", {}).get("argv", [])
        if argv[:1] == ["blocked-test"]:
            self.send_response(403)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(b'{"error":"blocked by policy"}')
            return

        self.send_response(200)
        self.send_header("Content-Type", "text/plain")
        self.end_headers()
        self.wfile.write(b"username=x-access-token\npassword=ghs_runtime_mock_token\n")

    def log_message(self, _format, *_args):
        return


ThreadingHTTPServer(("127.0.0.1", 18081), Handler).serve_forever()
PY

cleanup() {
  "$DOCKER_BIN" logs "$NAME" >/tmp/employee-bridge-runtime-smoke.log 2>&1 || true
  "$DOCKER_BIN" logs "$AUTH_NAME" >/tmp/employee-bridge-runtime-auth-smoke.log 2>&1 || true
  "$DOCKER_BIN" rm -f "$NAME" >/dev/null 2>&1 || true
  "$DOCKER_BIN" rm -f "$AUTH_NAME" >/dev/null 2>&1 || true
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

"$DOCKER_BIN" rm -f "$NAME" >/dev/null 2>&1 || true
"$DOCKER_BIN" rm -f "$AUTH_NAME" >/dev/null 2>&1 || true

run_args=()
if [[ -n "$PLATFORM" ]]; then
  run_args+=(--platform "$PLATFORM")
fi

"$DOCKER_BIN" run -d \
  "${run_args[@]}" \
  --name "$NAME" \
  -p 17080:7080 \
  -e RUNTIME_SECRET="$SECRET" \
  -e BRIDGE_API_KEY="$SECRET" \
  -e HIVY_GIT_USERNAME="Runtime Smoke" \
  -e HIVY_GIT_EMAIL="runtime-smoke@usehivy.com" \
  -e HIVY_GIT_CREDENTIALS_URL="http://127.0.0.1:18081/git-credentials" \
  -e OPENROUTER_API_KEY="${OPENROUTER_API_KEY:-dummy}" \
  "$IMAGE" >/tmp/employee-bridge-runtime-container-id

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
"$DOCKER_BIN" exec "$NAME" test -x /usr/local/bin/git-credential-hivy
"$DOCKER_BIN" exec "$NAME" test -x /usr/local/bin/gh-wrapper
"$DOCKER_BIN" exec "$NAME" test -L /usr/local/bin/gh
"$DOCKER_BIN" exec "$NAME" sh -lc 'test "$(git config --system credential.helper)" = "/usr/local/bin/git-credential-hivy"'
"$DOCKER_BIN" exec "$NAME" sh -lc 'test "$(git config --system user.name)" = "Runtime Smoke"'
"$DOCKER_BIN" exec "$NAME" sh -lc 'test "$(git config --system user.email)" = "runtime-smoke@usehivy.com"'

"$DOCKER_BIN" run -d \
  "${run_args[@]}" \
  --name "$AUTH_NAME" \
  --entrypoint /bin/sh \
  -e RUNTIME_SECRET="$SECRET" \
  -e BRIDGE_API_KEY="$SECRET" \
  -e HIVY_GIT_USERNAME="Runtime Smoke" \
  -e HIVY_GIT_EMAIL="runtime-smoke@usehivy.com" \
  -e HIVY_GIT_CREDENTIALS_URL="http://127.0.0.1:18081/git-credentials" \
  "$IMAGE" \
  -c 'git config --system user.name "$HIVY_GIT_USERNAME"; git config --system user.email "$HIVY_GIT_EMAIL"; sleep 300' >/tmp/employee-bridge-runtime-auth-container-id

"$DOCKER_BIN" exec "$AUTH_NAME" test -x /usr/local/bin/git-credential-hivy
"$DOCKER_BIN" exec "$AUTH_NAME" test -x /usr/local/bin/gh-wrapper
"$DOCKER_BIN" exec "$AUTH_NAME" test -L /usr/local/bin/gh
"$DOCKER_BIN" exec "$AUTH_NAME" sh -lc 'test "$(git config --system credential.helper)" = "/usr/local/bin/git-credential-hivy"'
"$DOCKER_BIN" exec "$AUTH_NAME" sh -lc 'test "$(git config --system user.name)" = "Runtime Smoke"'
"$DOCKER_BIN" exec "$AUTH_NAME" sh -lc 'test "$(git config --system user.email)" = "runtime-smoke@usehivy.com"'
"$DOCKER_BIN" cp "$TMP_DIR/git_credentials_mock.py" "$AUTH_NAME:/tmp/git_credentials_mock.py"
"$DOCKER_BIN" exec -d "$AUTH_NAME" python3 /tmp/git_credentials_mock.py /tmp/gh-wrapper-requests.jsonl
for _ in $(seq 1 50); do
  if "$DOCKER_BIN" exec "$AUTH_NAME" curl -fsS http://127.0.0.1:18081/health >/dev/null 2>&1; then
    break
  fi
  sleep 0.1
done
"$DOCKER_BIN" exec "$AUTH_NAME" curl -fsS http://127.0.0.1:18081/health >/dev/null
"$DOCKER_BIN" exec "$AUTH_NAME" gh --version >/tmp/runtime-gh-version.txt
"$DOCKER_BIN" exec -i "$AUTH_NAME" python3 - <<'PY'
import json

requests = [json.loads(line) for line in open("/tmp/gh-wrapper-requests.jsonl", encoding="utf-8")]
assert len(requests) == 1, requests
request = requests[0]
body = request["body"]
assert request["path"] == "/git-credentials", request
assert request["authorization"] == "Bearer runtime-test-token", request
assert request["content_type"].startswith("application/json"), request
assert body["schema_version"] == 1, body
assert body["client"] == "gh-wrapper", body
assert body["operation"]["program"] == "gh", body
assert body["operation"]["argv"] == ["--version"], body
assert body["operation"]["cwd"].startswith("/"), body
PY
if "$DOCKER_BIN" exec "$AUTH_NAME" gh blocked-test >/tmp/runtime-gh-denied.out 2>/tmp/runtime-gh-denied.err; then
  echo "gh-wrapper allowed a mocked policy rejection" >&2
  exit 1
fi
grep -q "gh-wrapper: control plane rejected gh command (403)" /tmp/runtime-gh-denied.err
"$DOCKER_BIN" exec -i "$AUTH_NAME" python3 - <<'PY'
import json

requests = [json.loads(line) for line in open("/tmp/gh-wrapper-requests.jsonl", encoding="utf-8")]
assert len(requests) == 2, requests
body = requests[1]["body"]
assert body["operation"]["argv"] == ["blocked-test"], body
PY

echo "runtime image smoke passed"
