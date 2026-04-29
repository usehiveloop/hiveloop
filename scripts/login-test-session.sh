#!/usr/bin/env bash
set -euo pipefail

EMAIL="${1:-agent-test@example.com}"
RUN_DIR="${RUN_DIR:-/tmp/agent-test}"
FRONTEND_URL="${FRONTEND_URL:-http://localhost:31112}"
BACKEND_LOG="${BACKEND_LOG:-$RUN_DIR/backend.log}"

[ -f "$BACKEND_LOG" ] || {
  echo "ERROR: $BACKEND_LOG not found — is the backend running?" >&2
  exit 1
}

request_otp() {
  agent-browser open "$FRONTEND_URL/auth" >/dev/null
  agent-browser eval "fetch('/api/proxy/auth/otp/request', {
    method:'POST',
    headers:{'Content-Type':'application/json'},
    body: JSON.stringify({ email: '$EMAIL' })
  }).then(r => r.status)" >/dev/null
}

read_otp_from_log() {
  sleep 0.5
  grep -oE 'code:[0-9]{6}' "$BACKEND_LOG" | tail -1 | cut -d: -f2
}

verify_otp() {
  local code="$1"
  agent-browser eval "fetch('/api/proxy/auth/otp/verify', {
    method:'POST',
    headers:{'Content-Type':'application/json'},
    body: JSON.stringify({ email: '$EMAIL', code: '$code' })
  }).then(r => r.json())" 2>&1 | tail -1
}

request_otp
OTP="$(read_otp_from_log)"
[ -n "$OTP" ] || {
  echo "ERROR: no OTP found in $BACKEND_LOG. Is KIBAMAIL_API_KEY empty?" >&2
  exit 1
}
RESULT="$(verify_otp "$OTP")"
echo "logged in: $EMAIL  (otp=$OTP)"
echo "$RESULT"
