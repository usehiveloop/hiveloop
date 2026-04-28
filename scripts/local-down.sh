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

# Then kill each child's whole process group. local-up.sh starts children
# under setsid so PGID == child PID — sending SIGKILL to -PID hits the
# entire tree (e.g. pnpm → node → next-server).
for f in "$RUN_DIR"/*.pid; do
  [ -f "$f" ] || continue
  NAME="$(basename "$f" .pid)"
  # redis.pid is owned by redis-server itself — we DO NOT kill (it's stable
  # and may be shared with other tools). Skip.
  [ "$NAME" = "redis" ] && continue
  PID="$(cat "$f")"
  # Negative pid → kill the whole process group rooted at PID.
  if kill -9 -- "-$PID" 2>/dev/null; then
    echo "  killed group:      $NAME (pgid $PID)"
  elif kill -9 "$PID" 2>/dev/null; then
    echo "  killed child:      $NAME (pid $PID)"
  fi
  rm -f "$f"
done

# Belt + braces: any next-server / pnpm / fake-nango / hiveloop survivors
# (from sessions before setsid was added, or from a forced kill earlier).
pkill -9 -f "next-server" 2>/dev/null || true
pkill -9 -f "/tmp/agent-test/(fake-nango|hiveloop)" 2>/dev/null || true

if [ "${HARD:-}" = "1" ]; then
  rm -f "$RUN_DIR"/*.log "$RUN_DIR"/*.env
  echo "  cleaned $RUN_DIR (logs + env files)"
fi

echo "stopped."
