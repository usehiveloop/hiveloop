#!/usr/bin/env bash
set -euo pipefail

wait_for() {
  local name="$1"
  local command="$2"
  local attempts="${3:-90}"
  local delay="${4:-2}"

  for _ in $(seq 1 "$attempts"); do
    if eval "$command" >/dev/null 2>&1; then
      echo "ok: $name"
      return 0
    fi
    sleep "$delay"
  done

  echo "timeout waiting for $name" >&2
  return 1
}

wait_for "postgres" "docker run --rm --network host -e PGPASSWORD=localdev pgvector/pgvector:pg17 pg_isready -h 127.0.0.1 -p 5433 -U hivy -d hivy_test -q"
wait_for "redis" "docker run --rm --network host redis:7-alpine redis-cli -h 127.0.0.1 -p 16279 ping | grep -q PONG"
