#!/usr/bin/env bash
set -e

RUN_DIR="${RUN_DIR:-/tmp/agent-test}"
[ -d "$RUN_DIR" ] || { echo "no $RUN_DIR — nothing to stop"; exit 0; }

kill_supervisors() {
  for f in "$RUN_DIR"/*.supervisor.pid; do
    [ -f "$f" ] || continue
    local name pid
    name="$(basename "$f" .supervisor.pid)"
    pid="$(cat "$f")"
    if kill -9 "$pid" 2>/dev/null; then
      echo "  killed supervisor: $name (pid $pid)"
    fi
    rm -f "$f"
  done
}

kill_children() {
  for f in "$RUN_DIR"/*.pid; do
    [ -f "$f" ] || continue
    local name pid
    name="$(basename "$f" .pid)"
    [ "$name" = "redis" ] && continue
    pid="$(cat "$f")"
    if kill -9 -- "-$pid" 2>/dev/null; then
      echo "  killed group:      $name (pgid $pid)"
    elif kill -9 "$pid" 2>/dev/null; then
      echo "  killed child:      $name (pid $pid)"
    fi
    rm -f "$f"
  done
}

port_pids() {
  local port="$1"
  if command -v lsof >/dev/null 2>&1; then
    lsof -ti tcp:"$port" 2>/dev/null
  elif command -v fuser >/dev/null 2>&1; then
    fuser -n tcp "$port" 2>/dev/null | tr -s ' ' '\n' | grep -E '^[0-9]+$'
  else
    netstat -tlnp 2>/dev/null \
      | awk -v p=":$port " '$4 ~ p { sub(/\/.*/, "", $7); if ($7 ~ /^[0-9]+$/) print $7 }'
  fi
}

kill_port_holders() {
  for port in 13004 18080 31112; do
    for pid in $(port_pids "$port"); do
      kill -9 "$pid" 2>/dev/null && echo "  killed port:       :$port holder (pid $pid)"
    done
  done
}

clean_artifacts() {
  [ "${HARD:-}" = "1" ] || return 0
  rm -f "$RUN_DIR"/*.log "$RUN_DIR"/*.env
  echo "  cleaned $RUN_DIR (logs + env files)"
}

kill_supervisors
kill_children
kill_port_holders
clean_artifacts
echo "stopped."
