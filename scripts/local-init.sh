#!/usr/bin/env bash
# One-time bootstrap for a fresh sandbox: start the apt-installed Postgres,
# create the `hiveloop` user + database, allow trust-auth from 127.0.0.1.
# Idempotent — safe to re-run; skips work that's already done.
#
# Uses the apt cluster's default port (whatever postgresql.conf says — 5432
# on Ubuntu). Doesn't reconfigure anything.
#
# Output (stdout, last line): the port postgres is listening on.
# local-up.sh captures it and uses it for the rest of the run.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
RUN_DIR="${RUN_DIR:-/tmp/agent-test}"
mkdir -p "$RUN_DIR"

# ─── Locate the apt cluster ─────────────────────────────────────────────────
if ! command -v pg_ctlcluster >/dev/null 2>&1; then
  echo "ERROR: pg_ctlcluster not found — install with: apt-get install -y postgresql" >&2
  exit 1
fi

# pg_lsclusters output: "Ver Cluster Port Status Owner Data dir   Log file"
PG_LINE="$(pg_lsclusters -h 2>/dev/null | head -1 || true)"
if [ -z "$PG_LINE" ]; then
  echo "ERROR: no postgres cluster found in pg_lsclusters" >&2
  exit 1
fi
PG_VER="$(echo "$PG_LINE"  | awk '{print $1}')"
PG_NAME="$(echo "$PG_LINE" | awk '{print $2}')"
PG_PORT="$(echo "$PG_LINE" | awk '{print $3}')"
PG_HBA="/etc/postgresql/$PG_VER/$PG_NAME/pg_hba.conf"

echo "  cluster: $PG_VER/$PG_NAME on port $PG_PORT" >&2

# ─── Trust local connections (idempotent) ───────────────────────────────────
# Two lines are required; either may already be present from a prior run.
if [ -w "$PG_HBA" ] || sudo -n true 2>/dev/null; then
  if ! grep -qE "^host\s+all\s+all\s+127\.0\.0\.1/32\s+trust" "$PG_HBA" 2>/dev/null; then
    echo "host all all 127.0.0.1/32 trust" | sudo -n tee -a "$PG_HBA" >/dev/null
    echo "  + appended trust line to pg_hba.conf" >&2
  fi
  if ! grep -qE "^local\s+all\s+all\s+trust" "$PG_HBA" 2>/dev/null; then
    echo "local all all trust" | sudo -n tee -a "$PG_HBA" >/dev/null
    echo "  + appended local trust to pg_hba.conf" >&2
  fi
else
  echo "  ! cannot edit $PG_HBA (no sudo); assuming trust auth already set" >&2
fi

# ─── Start the cluster (idempotent) ─────────────────────────────────────────
if ! pg_isready -h 127.0.0.1 -p "$PG_PORT" -q 2>/dev/null; then
  sudo -n pg_ctlcluster "$PG_VER" "$PG_NAME" start 2>&1 >&2 || \
    pg_ctlcluster "$PG_VER" "$PG_NAME" start 2>&1 >&2 || true
  for i in $(seq 1 15); do
    pg_isready -h 127.0.0.1 -p "$PG_PORT" -q 2>/dev/null && break
    sleep 1
  done
fi
pg_isready -h 127.0.0.1 -p "$PG_PORT" -q 2>/dev/null || {
  echo "ERROR: postgres failed to start on :$PG_PORT" >&2; exit 1
}
echo "  ✓ postgres reachable on :$PG_PORT" >&2

# ─── Create hiveloop user (idempotent) ──────────────────────────────────────
USER_EXISTS="$(sudo -n -u postgres psql -p "$PG_PORT" -tAc \
  "SELECT 1 FROM pg_roles WHERE rolname='hiveloop'" 2>/dev/null || true)"
if [ "$USER_EXISTS" != "1" ]; then
  sudo -n -u postgres psql -p "$PG_PORT" -c \
    "CREATE USER hiveloop WITH SUPERUSER PASSWORD 'localdev';" >&2
  echo "  + created user hiveloop" >&2
else
  echo "  ✓ user hiveloop exists" >&2
fi

# ─── Create hiveloop database (idempotent) ──────────────────────────────────
DB_EXISTS="$(sudo -n -u postgres psql -p "$PG_PORT" -tAc \
  "SELECT 1 FROM pg_database WHERE datname='hiveloop'" 2>/dev/null || true)"
if [ "$DB_EXISTS" != "1" ]; then
  sudo -n -u postgres createdb -p "$PG_PORT" -O hiveloop hiveloop
  echo "  + created database hiveloop" >&2
else
  echo "  ✓ database hiveloop exists" >&2
fi

# ─── Output ─────────────────────────────────────────────────────────────────
# Last line of stdout = the port. Callers (local-up.sh) parse this.
echo "$PG_PORT" > "$RUN_DIR/pg.port"
echo "$PG_PORT"
