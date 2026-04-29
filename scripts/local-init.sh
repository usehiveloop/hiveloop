#!/usr/bin/env bash
set -euo pipefail

RUN_DIR="${RUN_DIR:-/tmp/agent-test}"
mkdir -p "$RUN_DIR"

as_root() {
  if [ "$(id -u)" = "0" ]; then "$@"; else sudo -n "$@"; fi
}

run_as_postgres() {
  if [ "$(id -u)" = "0" ]; then
    runuser -u postgres -- "$@"
  else
    sudo -n -u postgres "$@"
  fi
}

require_pg_ctlcluster() {
  command -v pg_ctlcluster >/dev/null 2>&1 || {
    echo "ERROR: pg_ctlcluster not found — install with: apt-get install -y postgresql" >&2
    exit 1
  }
}

discover_cluster() {
  local line
  line="$(pg_lsclusters -h 2>/dev/null | head -1 || true)"
  [ -n "$line" ] || { echo "ERROR: no postgres cluster found" >&2; exit 1; }
  PG_VER="$(echo  "$line" | awk '{print $1}')"
  PG_NAME="$(echo "$line" | awk '{print $2}')"
  PG_PORT="$(echo "$line" | awk '{print $3}')"
  PG_HBA="/etc/postgresql/$PG_VER/$PG_NAME/pg_hba.conf"
  echo "  cluster: $PG_VER/$PG_NAME on port $PG_PORT" >&2
}

trust_local_auth() {
  if [ ! -w "$PG_HBA" ] && [ "$(id -u)" != "0" ] && ! sudo -n true 2>/dev/null; then
    echo "  ! cannot edit $PG_HBA (no root, no sudo); assuming trust auth set" >&2
    return 0
  fi
  grep -qE "^host\s+all\s+all\s+127\.0\.0\.1/32\s+trust" "$PG_HBA" 2>/dev/null \
    || echo "host all all 127.0.0.1/32 trust" | as_root tee -a "$PG_HBA" >/dev/null
  grep -qE "^local\s+all\s+all\s+trust" "$PG_HBA" 2>/dev/null \
    || echo "local all all trust" | as_root tee -a "$PG_HBA" >/dev/null
}

start_cluster() {
  pg_isready -h 127.0.0.1 -p "$PG_PORT" -q 2>/dev/null && return 0
  as_root pg_ctlcluster "$PG_VER" "$PG_NAME" start 2>&1 >&2 || true
  for _ in $(seq 1 15); do
    pg_isready -h 127.0.0.1 -p "$PG_PORT" -q 2>/dev/null && return 0
    sleep 1
  done
  echo "ERROR: postgres failed to start on :$PG_PORT" >&2
  exit 1
}

ensure_user() {
  local exists
  exists="$(run_as_postgres psql -p "$PG_PORT" -tAc \
    "SELECT 1 FROM pg_roles WHERE rolname='hiveloop'" 2>/dev/null || true)"
  [ "$exists" = "1" ] && return 0
  run_as_postgres psql -p "$PG_PORT" -c \
    "CREATE USER hiveloop WITH SUPERUSER PASSWORD 'localdev';" >&2
}

ensure_database() {
  local exists
  exists="$(run_as_postgres psql -p "$PG_PORT" -tAc \
    "SELECT 1 FROM pg_database WHERE datname='hiveloop'" 2>/dev/null || true)"
  [ "$exists" = "1" ] && return 0
  run_as_postgres createdb -p "$PG_PORT" -O hiveloop hiveloop
}

require_pg_ctlcluster
discover_cluster
trust_local_auth
start_cluster
ensure_user
ensure_database

echo "$PG_PORT" > "$RUN_DIR/pg.port"
echo "$PG_PORT"
