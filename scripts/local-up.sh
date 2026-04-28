#!/usr/bin/env bash
# Bring up the local test stack as native processes.
#
# Postgres + Redis are expected as system packages (apt installs in the
# dev-box image). This script starts them if they aren't reachable yet.
# fake-nango, backend, and frontend run under bash supervisors that restart
# them on crash with a 2s delay.
#
# Idempotent — skips processes that are already healthy. Pid files in
# $RUN_DIR/{name}.pid (child) and $RUN_DIR/{name}.supervisor.pid (loop).
#
# Env overrides:
#   FAKE_NANGO_PORT   default 13004
#   BACKEND_PORT      default 18080
#   FRONTEND_PORT     default 31112
#   PG_PORT           default 5433
#   REDIS_PORT        default 6379
#   RUN_DIR           default /tmp/agent-test
set -euo pipefail

FAKE_NANGO_PORT="${FAKE_NANGO_PORT:-13004}"
BACKEND_PORT="${BACKEND_PORT:-18080}"
FRONTEND_PORT="${FRONTEND_PORT:-31112}"
PG_PORT="${PG_PORT:-5433}"
REDIS_PORT="${REDIS_PORT:-6379}"
RUN_DIR="${RUN_DIR:-/tmp/agent-test}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"

mkdir -p "$RUN_DIR"
cd "$ROOT"

# ─── Helpers ────────────────────────────────────────────────────────────────
healthy_http() { curl -sf -o /dev/null "$1" 2>/dev/null; }
wait_http() {
  local url="$1" name="$2" tries="${3:-30}"
  for i in $(seq 1 "$tries"); do
    healthy_http "$url" && { echo "  ✓ $name"; return 0; }
    sleep 1
  done
  echo "  ✗ $name (timeout after ${tries}s)"; return 1
}

# supervise NAME -- CMD ARGS...
# Backgrounds CMD under a restart loop. Writes:
#   $RUN_DIR/$NAME.pid             — current child pid
#   $RUN_DIR/$NAME.supervisor.pid  — the loop's pid
#   $RUN_DIR/$NAME.log             — combined stdout/stderr
# local-down.sh kills supervisor.pid first (loop dies), then child pid.
supervise() {
  local name="$1"; shift
  local logf="$RUN_DIR/$name.log"
  local pidf="$RUN_DIR/$name.pid"
  local supf="$RUN_DIR/$name.supervisor.pid"

  ( while true; do
      "$@" >> "$logf" 2>&1 &
      local child=$!
      echo "$child" > "$pidf"
      wait "$child"
      local ec=$?
      # 143 = SIGTERM (graceful shutdown initiated by local-down). Exit silently.
      if [ $ec -eq 143 ] || [ $ec -eq 130 ]; then
        rm -f "$pidf"
        exit 0
      fi
      echo "[$(date '+%H:%M:%S')] [supervise] $name exited (code=$ec), restarting in 2s..." >> "$logf"
      sleep 2
    done
  ) &
  echo $! > "$supf"
}

# ─── 1. Postgres ────────────────────────────────────────────────────────────
echo "==> postgres (:$PG_PORT)"
if pg_isready -h 127.0.0.1 -p "$PG_PORT" -U hiveloop -q 2>/dev/null; then
  echo "  ✓ already up"
else
  # Ubuntu's pg_ctlcluster wrapper — only works if the cluster has been
  # configured to listen on $PG_PORT. The dev-box image setup is responsible
  # for that (see cmd/buildtemplates).
  if command -v pg_ctlcluster >/dev/null 2>&1; then
    PG_VER="$(pg_lsclusters -h 2>/dev/null | awk '$3 == '"$PG_PORT"' { print $1; exit }')"
    if [ -n "${PG_VER:-}" ]; then
      pg_ctlcluster "$PG_VER" main start 2>/dev/null || true
    else
      echo "  ! no cluster configured for port $PG_PORT — see dev-box init" >&2
    fi
  else
    echo "  ! pg_ctlcluster not found; expecting postgres on :$PG_PORT" >&2
  fi
  for i in $(seq 1 15); do
    pg_isready -h 127.0.0.1 -p "$PG_PORT" -U hiveloop -q 2>/dev/null && { echo "  ✓ ready"; break; }
    sleep 1
  done
  pg_isready -h 127.0.0.1 -p "$PG_PORT" -U hiveloop -q 2>/dev/null \
    || { echo "  ✗ postgres not reachable on :$PG_PORT" >&2; exit 1; }
fi

# ─── 2. Redis ───────────────────────────────────────────────────────────────
echo ""
echo "==> redis (:$REDIS_PORT)"
if redis-cli -h 127.0.0.1 -p "$REDIS_PORT" ping 2>/dev/null | grep -q PONG; then
  echo "  ✓ already up"
else
  redis-server --daemonize yes --port "$REDIS_PORT" \
    --maxmemory 256mb --maxmemory-policy allkeys-lru \
    --logfile "$RUN_DIR/redis.log" \
    --pidfile "$RUN_DIR/redis.pid" 2>/dev/null || true
  for i in $(seq 1 10); do
    redis-cli -h 127.0.0.1 -p "$REDIS_PORT" ping 2>/dev/null | grep -q PONG \
      && { echo "  ✓ ready"; break; }
    sleep 1
  done
  redis-cli -h 127.0.0.1 -p "$REDIS_PORT" ping 2>/dev/null | grep -q PONG \
    || { echo "  ✗ redis not reachable on :$REDIS_PORT" >&2; exit 1; }
fi

# ─── 3. Build ───────────────────────────────────────────────────────────────
echo ""
echo "==> build"
go build -o "$RUN_DIR/fake-nango" ./cmd/fake-nango
go build -o "$RUN_DIR/hiveloop"   ./cmd/server
echo "  ✓ fake-nango + hiveloop"

# ─── 4. fake-nango (supervised) ─────────────────────────────────────────────
echo ""
echo "==> fake-nango (:$FAKE_NANGO_PORT)"
if healthy_http "http://localhost:$FAKE_NANGO_PORT/providers.json"; then
  echo "  ✓ already up"
else
  FAKE_NANGO_SCENARIOS_DIR="$ROOT/cmd/fake-nango/scenarios" \
    supervise "fake-nango" \
      "$RUN_DIR/fake-nango" \
        -addr ":$FAKE_NANGO_PORT" \
        -secret fake-nango-secret \
        -webhook-target "http://localhost:$BACKEND_PORT/internal/webhooks/nango"
  wait_http "http://localhost:$FAKE_NANGO_PORT/providers.json" "fake-nango"
fi

# ─── 5. Backend env ─────────────────────────────────────────────────────────
cat > "$RUN_DIR/backend.env" <<EOF
ENVIRONMENT=development
PORT=$BACKEND_PORT
LOG_LEVEL=info
LOG_FORMAT=text
DB_HOST=localhost
DB_PORT=$PG_PORT
DB_USER=hiveloop
DB_PASSWORD=localdev
DB_NAME=hiveloop
DB_SSLMODE=disable
KMS_TYPE=aead
KMS_KEY=zvEnqF+4dO8J+h7pRbGgXstMEWcjbNt78yEOk1ywQ7I=
REDIS_ADDR=localhost:$REDIS_PORT
REDIS_CACHE_TTL=30m
MEM_CACHE_TTL=5m
MEM_CACHE_MAX_SIZE=10000
JWT_SIGNING_KEY=local-dev-signing-key
CORS_ORIGINS=http://localhost:$FRONTEND_PORT
AUTO_CONFIRM_EMAIL=true
PLATFORM_ADMIN_EMAILS=agent-test@example.com
ADMIN_API_ENABLED=true
AUTH_ISSUER=hiveloop
AUTH_AUDIENCE=http://localhost:$BACKEND_PORT
FRONTEND_URL=http://localhost:$FRONTEND_PORT
NANGO_ENDPOINT=http://localhost:$FAKE_NANGO_PORT
NANGO_SECRET_KEY=fake-nango-secret
SANDBOX_PROVIDER_ID=daytona
EOF
grep "^AUTH_RSA_PRIVATE_KEY=" .env >> "$RUN_DIR/backend.env" 2>/dev/null \
  || echo "  ! warning: AUTH_RSA_PRIVATE_KEY missing from .env" >&2

# Build the env-arg string once (avoids re-reading inside the supervisor loop)
BACKEND_ENV_ARGS=$(grep -v '^\s*#' "$RUN_DIR/backend.env" | grep -v '^\s*$')

# ─── 6. Backend (supervised) ────────────────────────────────────────────────
echo ""
echo "==> backend (:$BACKEND_PORT)"
if healthy_http "http://localhost:$BACKEND_PORT/healthz"; then
  echo "  ✓ already up"
else
  supervise "backend" env $BACKEND_ENV_ARGS "$RUN_DIR/hiveloop" serve
  wait_http "http://localhost:$BACKEND_PORT/healthz" "backend" 15
fi

# ─── 7. Frontend (supervised) ───────────────────────────────────────────────
cat > "$RUN_DIR/web.env" <<EOF
NEXT_PUBLIC_API_URL=http://localhost:$BACKEND_PORT
API_URL=http://localhost:$BACKEND_PORT
NEXT_PUBLIC_CONNECTIONS_HOST=http://localhost:$FAKE_NANGO_PORT
SESSION_SECRET=dev-session-secret-32chars-padding
EOF

echo ""
echo "==> frontend (:$FRONTEND_PORT)"
if healthy_http "http://localhost:$FRONTEND_PORT/"; then
  echo "  ✓ already up"
else
  # Free apps/web/.next/dev/lock if a stale next dev still holds it
  LOCK_PID="$(lsof -t apps/web/.next/dev/lock 2>/dev/null | head -1 || true)"
  if [ -n "$LOCK_PID" ]; then
    echo "  ! killing stale next dev (pid $LOCK_PID)"
    kill -9 "$LOCK_PID" 2>/dev/null || true
    sleep 1
    rm -f apps/web/.next/dev/lock
  fi

  WEB_ENV_ARGS="$(cat "$RUN_DIR/web.env" | xargs)"
  supervise "web" bash -c "cd '$ROOT/apps/web' && exec env $WEB_ENV_ARGS pnpm dev --port $FRONTEND_PORT"
  wait_http "http://localhost:$FRONTEND_PORT/" "frontend" 60
fi

echo ""
echo "Stack is up:"
echo "  fake-nango   http://localhost:$FAKE_NANGO_PORT"
echo "  backend      http://localhost:$BACKEND_PORT"
echo "  frontend     http://localhost:$FRONTEND_PORT"
echo ""
echo "Logs:        $RUN_DIR/{fake-nango,backend,web}.log"
echo "Tail:        tail -f $RUN_DIR/*.log"
echo "Stop:        make local-down"
echo "Next:        make seed-test  &&  make login-test"
