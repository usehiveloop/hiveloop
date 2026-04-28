#!/usr/bin/env bash
# Stop the supervised app processes started by local-up.sh.
#
# Postgres + Redis are NOT stopped — they're system services that may be
# shared. To stop them: `pg_ctlcluster <ver> main stop` and `redis-cli shutdown`.
#
# To also remove the cached env files + logs: HARD=1 ./scripts/local-down.sh
set -e

RUN_DIR="${RUN_DIR:-/tmp/agent-test}"
[ -d "$RUN_DIR" ] || { echo "no $RUN_DIR — nothing to stop"; exit 0; }

# Kill supervisor loops first so they stop restarting children.
for f in "$RUN_DIR"/*.supervisor.pid; do
  [ -f "$f" ] || continue
  NAME="$(basename "$f" .supervisor.pid)"
  PID="$(cat "$f")"
  if kill -9 "$PID" 2>/dev/null; then
    echo "  killed supervisor: $NAME (pid $PID)"
  fi
  rm -f "$f"
done

# Then kill the actual children (orphaned by their dead supervisors).
for f in "$RUN_DIR"/*.pid; do
  [ -f "$f" ] || continue
  NAME="$(basename "$f" .pid)"
  # redis.pid is owned by redis-server itself — we DO NOT kill (it's stable
  # and may be shared with other tools). Skip.
  [ "$NAME" = "redis" ] && continue
  PID="$(cat "$f")"
  if kill -9 "$PID" 2>/dev/null; then
    echo "  killed child:      $NAME (pid $PID)"
  fi
  rm -f "$f"
done

if [ "${HARD:-}" = "1" ]; then
  rm -f "$RUN_DIR"/*.log "$RUN_DIR"/*.env
  echo "  cleaned $RUN_DIR (logs + env files)"
fi

echo "stopped."
