#!/usr/bin/env bash
# Programmatic OTP login. Drives the same flow as a human: request OTP via
# /api/proxy/auth/otp/request, read the code from the backend log (LogSender
# prints it), submit via /api/proxy/auth/otp/verify. The proxy intercepts the
# verify response and writes the __session cookie into the agent-browser
# Chrome session — durable until cleared.
#
# Usage: ./scripts/login-test-session.sh [email]
#
# Defaults email to agent-test@example.com.
# Requires: agent-browser running, fake-nango + backend + frontend up.
set -euo pipefail

EMAIL="${1:-agent-test@example.com}"
RUN_DIR="${RUN_DIR:-/tmp/agent-test}"
FRONTEND_URL="${FRONTEND_URL:-http://localhost:31112}"
BACKEND_LOG="${BACKEND_LOG:-$RUN_DIR/backend.log}"

[ -f "$BACKEND_LOG" ] || { echo "ERROR: $BACKEND_LOG not found — is the backend running?" >&2; exit 1; }

# Establish the browser session at /auth so the proxy origin is set up.
agent-browser open "$FRONTEND_URL/auth" >/dev/null

# Request OTP. The page-level fetch runs through /api/proxy → backend, so
# CORS + cookies behave like a real signup.
agent-browser eval "fetch('/api/proxy/auth/otp/request', {
  method:'POST',
  headers:{'Content-Type':'application/json'},
  body: JSON.stringify({ email: '$EMAIL' })
}).then(r => r.status)" >/dev/null

# Pull the freshest OTP code from the backend log. The LogSender writes
# `email (template) ... variables=map[code:NNNNNN ...]`.
sleep 0.5
OTP="$(grep -oE 'code:[0-9]{6}' "$BACKEND_LOG" | tail -1 | cut -d: -f2 || true)"
if [ -z "$OTP" ]; then
  echo "ERROR: no OTP found in $BACKEND_LOG. Is KIBAMAIL_API_KEY empty so LogSender is used?" >&2
  exit 1
fi

# Verify. The /api/proxy auth interceptor pulls access_token + refresh_token
# from the response and writes them into the encrypted __session cookie.
RESULT="$(agent-browser eval "fetch('/api/proxy/auth/otp/verify', {
  method:'POST',
  headers:{'Content-Type':'application/json'},
  body: JSON.stringify({ email: '$EMAIL', code: '$OTP' })
}).then(r => r.json())" 2>&1 | tail -1)"

echo "logged in: $EMAIL  (otp=$OTP)"
echo "$RESULT"
