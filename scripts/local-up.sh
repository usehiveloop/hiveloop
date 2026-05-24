#!/usr/bin/env bash
set -euo pipefail

FAKE_NANGO_PORT="${FAKE_NANGO_PORT:-13004}"
BACKEND_PORT="${BACKEND_PORT:-18080}"
FRONTEND_PORT="${FRONTEND_PORT:-31112}"
PG_PORT="${PG_PORT:-5432}"
REDIS_PORT="${REDIS_PORT:-6379}"
RUN_DIR="${RUN_DIR:-/tmp/agent-test}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"

mkdir -p "$RUN_DIR"
cd "$ROOT"

healthy_http() { curl -sf -o /dev/null "$1" 2>/dev/null; }

wait_http() {
  local url="$1" name="$2" tries="${3:-30}"
  for _ in $(seq 1 "$tries"); do
    healthy_http "$url" && { echo "  ✓ $name"; return 0; }
    sleep 1
  done
  echo "  ✗ $name (timeout after ${tries}s)"
  return 1
}

supervise() {
  local name="$1"; shift
  local logf="$RUN_DIR/$name.log"
  local pidf="$RUN_DIR/$name.pid"
  local supf="$RUN_DIR/$name.supervisor.pid"
  ( set +e
    while true; do
      setsid "$@" >> "$logf" 2>&1 &
      local child=$!
      echo "$child" > "$pidf"
      wait "$child"
      local ec=$?
      if [ "$ec" = 143 ] || [ "$ec" = 130 ]; then
        rm -f "$pidf"
        exit 0
      fi
      echo "[$(date '+%H:%M:%S')] [supervise] $name exited (code=$ec), restarting in 2s..." >> "$logf"
      sleep 2
    done
  ) &
  echo $! > "$supf"
}

pg_can_connect() {
  PGPASSWORD=localdev psql -h 127.0.0.1 -p "$1" -U hivy -d hivy \
    -tAc 'SELECT 1' >/dev/null 2>&1
}

ensure_postgres() {
  echo "==> postgres"
  for try in "$PG_PORT" 5432 5433; do
    if pg_can_connect "$try"; then
      PG_PORT="$try"
      echo "$PG_PORT" > "$RUN_DIR/pg.port"
      echo "  ✓ ready on :$PG_PORT"
      return 0
    fi
  done
  echo "  initializing native cluster..."
  PG_PORT="$("$ROOT/scripts/local-init.sh" 2>&1 | tee /dev/stderr | tail -1)"
  pg_can_connect "$PG_PORT" \
    || { echo "  ✗ postgres still not reachable as hivy@hivy" >&2; exit 1; }
  echo "$PG_PORT" > "$RUN_DIR/pg.port"
  echo "  ✓ ready on :$PG_PORT"
}

ensure_redis() {
  echo "==> redis (:$REDIS_PORT)"
  if redis-cli -h 127.0.0.1 -p "$REDIS_PORT" ping 2>/dev/null | grep -q PONG; then
    echo "  ✓ already up"
    return 0
  fi
  redis-server --daemonize yes --port "$REDIS_PORT" \
    --maxmemory 256mb --maxmemory-policy allkeys-lru \
    --logfile "$RUN_DIR/redis.log" \
    --pidfile "$RUN_DIR/redis.pid" 2>/dev/null || true
  for _ in $(seq 1 10); do
    redis-cli -h 127.0.0.1 -p "$REDIS_PORT" ping 2>/dev/null | grep -q PONG \
      && { echo "  ✓ ready"; return 0; }
    sleep 1
  done
  echo "  ✗ redis not reachable on :$REDIS_PORT" >&2
  exit 1
}

build_binaries() {
  echo "==> build"
  go build -o "$RUN_DIR/fake-nango" ./cmd/fake-nango
  go build -o "$RUN_DIR/hivy"   ./cmd/server
  echo "  ✓ fake-nango + hivy"
}

start_fake_nango() {
  echo "==> fake-nango (:$FAKE_NANGO_PORT)"
  if healthy_http "http://localhost:$FAKE_NANGO_PORT/providers.json"; then
    echo "  ✓ already up"
    return 0
  fi
  FAKE_NANGO_SCENARIOS_DIR="$ROOT/cmd/fake-nango/scenarios" \
    supervise "fake-nango" \
      "$RUN_DIR/fake-nango" \
        -addr ":$FAKE_NANGO_PORT" \
        -secret fake-nango-secret \
        -webhook-target "http://localhost:$BACKEND_PORT/internal/webhooks/nango"
  wait_http "http://localhost:$FAKE_NANGO_PORT/providers.json" "fake-nango"
}

gen_rsa() { openssl genrsa 2048 2>/dev/null | base64 | tr -d '\n'; }
gen_aes() { openssl rand -base64 32 | tr -d '\n'; }

ensure_secret() {
  local var="$1" file="$2" gen="$3"
  if grep -q "^${var}=" .env 2>/dev/null; then
    grep "^${var}=" .env >> "$RUN_DIR/backend.env"
  elif [ -f "$RUN_DIR/$file" ]; then
    echo "${var}=$(cat "$RUN_DIR/$file")" >> "$RUN_DIR/backend.env"
  else
    local val
    val="$($gen)"
    echo "$val" > "$RUN_DIR/$file"
    echo "${var}=${val}" >> "$RUN_DIR/backend.env"
    echo "  ! generated ephemeral $var at $RUN_DIR/$file"
  fi
}

write_backend_env() {
  cat > "$RUN_DIR/backend.env" <<EOF
HIVY_ENVIRONMENT=development
HIVY_PORT=$BACKEND_PORT
HIVY_LOG_LEVEL=info
HIVY_LOG_FORMAT=text
HIVY_DB_HOST=localhost
HIVY_DB_PORT=$PG_PORT
HIVY_DB_USER=hivy
HIVY_DB_PASSWORD=localdev
HIVY_DB_NAME=hivy
HIVY_DB_SSLMODE=disable
HIVY_KMS_TYPE=aead
HIVY_REDIS_ADDR=localhost:$REDIS_PORT
HIVY_REDIS_CACHE_TTL=30m
HIVY_MEM_CACHE_TTL=5m
HIVY_MEM_CACHE_MAX_SIZE=10000
HIVY_JWT_SIGNING_KEY=local-dev-signing-key
HIVY_CORS_ORIGINS=http://localhost:$FRONTEND_PORT
HIVY_AUTO_CONFIRM_EMAIL=true
HIVY_PLATFORM_ADMIN_EMAILS=agent-test@example.com
HIVY_AUTH_ISSUER=hivy
HIVY_AUTH_AUDIENCE=http://localhost:$BACKEND_PORT
HIVY_FRONTEND_URL=http://localhost:$FRONTEND_PORT
HIVY_NANGO_ENDPOINT=http://localhost:$FAKE_NANGO_PORT
HIVY_NANGO_SECRET_KEY=fake-nango-secret
HIVY_SANDBOX_PROVIDER_ID=daytona
HIVY_SPECIALIST_SANDBOX_RUNTIME_VERSION=v1.0.1
EOF
  ensure_secret HIVY_AUTH_RSA_PRIVATE_KEY auth-rsa.key gen_rsa
  ensure_secret HIVY_KMS_KEY              kms.key      gen_aes
}

start_backend() {
  echo "==> backend (:$BACKEND_PORT)"
  if healthy_http "http://localhost:$BACKEND_PORT/healthz"; then
    echo "  ✓ already up"
    return 0
  fi
  local args
  args=$(grep -v '^\s*#' "$RUN_DIR/backend.env" | grep -v '^\s*$')
  supervise "backend" env $args "$RUN_DIR/hivy" serve
  wait_http "http://localhost:$BACKEND_PORT/healthz" "backend" 15
}

run_migrations() {
  echo "==> migrations"
  local args
  args=$(grep -v '^\s*#' "$RUN_DIR/backend.env" | grep -v '^\s*$')
  env $args "$RUN_DIR/hivy" migrate up
  echo "  ✓ database schema current"
}

find_corepack() {
  command -v corepack >/dev/null 2>&1 && { command -v corepack; return; }
  local node_dir
  node_dir="$(dirname "$(command -v node 2>/dev/null)" 2>/dev/null)"
  [ -x "$node_dir/corepack" ] && { echo "$node_dir/corepack"; return; }
  return 1
}

ensure_pnpm() {
  command -v pnpm >/dev/null 2>&1 && return 0
  local corepack node_dir
  corepack="$(find_corepack)" || { echo "  ✗ corepack not found (need Node ≥16.10)" >&2; exit 1; }
  node_dir="$(dirname "$(command -v node)")"
  "$corepack" enable >/dev/null 2>&1 || true
  "$corepack" prepare pnpm@10.18.2 --activate >/dev/null 2>&1 || true
  if [ -x "$node_dir/pnpm" ] && [ "$node_dir/pnpm" != "/usr/local/bin/pnpm" ]; then
    ln -sf "$node_dir/pnpm" /usr/local/bin/pnpm 2>/dev/null || true
  fi
  command -v pnpm >/dev/null 2>&1 || { echo "  ✗ pnpm install failed" >&2; exit 1; }
}

ensure_web_deps() {
  [ -d apps/web/node_modules ] && return 0
  echo "  installing web deps..."
  ( cd apps/web && pnpm install --frozen-lockfile > "$RUN_DIR/pnpm-install.log" 2>&1 ) \
    || { echo "  ✗ pnpm install failed (see $RUN_DIR/pnpm-install.log)" >&2; exit 1; }
}

free_next_lock() {
  rm -f apps/web/.next/dev/lock 2>/dev/null
}

write_web_env() {
  cat > "$RUN_DIR/web.env" <<EOF
NEXT_PUBLIC_HIVY_API_URL=http://localhost:$BACKEND_PORT
HIVY_API_URL=http://localhost:$BACKEND_PORT
NEXT_PUBLIC_HIVY_CONNECTIONS_HOST=http://localhost:$FAKE_NANGO_PORT
HIVY_SESSION_SECRET=dev-session-secret-32chars-padding
EOF
}

start_frontend() {
  echo "==> frontend (:$FRONTEND_PORT)"
  if healthy_http "http://localhost:$FRONTEND_PORT/"; then
    echo "  ✓ already up"
    return 0
  fi
  ensure_pnpm
  ensure_web_deps
  free_next_lock
  local args
  args="$(cat "$RUN_DIR/web.env" | xargs)"
  supervise "web" bash -c "cd '$ROOT/apps/web' && exec env $args pnpm dev --port $FRONTEND_PORT"
  wait_http "http://localhost:$FRONTEND_PORT/" "frontend" 90
}

print_summary() {
  cat <<EOF

Stack is up:
  fake-nango   http://localhost:$FAKE_NANGO_PORT
  backend      http://localhost:$BACKEND_PORT
  frontend     http://localhost:$FRONTEND_PORT

Logs:        $RUN_DIR/{fake-nango,backend,web}.log
Tail:        tail -f $RUN_DIR/*.log
Stop:        make local-down
Next:        make seed-test  &&  make login-test
EOF
}

ensure_postgres
echo
ensure_redis
echo
build_binaries
echo
start_fake_nango
write_backend_env
echo
run_migrations
echo
start_backend
write_web_env
echo
start_frontend
print_summary
