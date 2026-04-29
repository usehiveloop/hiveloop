#!/usr/bin/env bash
set -e

REDIS_PORT="${REDIS_PORT:-6379}"
QUEUES=(critical default bulk periodic)

cli() { redis-cli -h 127.0.0.1 -p "$REDIS_PORT" "$@"; }

require_redis() {
  cli ping 2>/dev/null | grep -q PONG \
    || { echo "ERROR: redis not reachable on :$REDIS_PORT" >&2; exit 1; }
}

queue_count() { cli "$1" "$2" 2>/dev/null || echo 0; }

print_header() {
  printf "%-10s %8s %8s %10s %8s %10s\n" "queue" "pending" "active" "scheduled" "retry" "archived"
  printf "%-10s %8s %8s %10s %8s %10s\n" "----------" "-------" "------" "---------" "-----" "--------"
}

print_queue_row() {
  local q="$1"
  printf "%-10s %8s %8s %10s %8s %10s\n" "$q" \
    "$(queue_count LLEN  "asynq:{$q}:pending")"   \
    "$(queue_count LLEN  "asynq:{$q}:active")"    \
    "$(queue_count ZCARD "asynq:{$q}:scheduled")" \
    "$(queue_count ZCARD "asynq:{$q}:retry")"     \
    "$(queue_count ZCARD "asynq:{$q}:archived")"
}

sample_pending_types() {
  [ "${VERBOSE:-}" = "1" ] || return 0
  echo
  echo "Pending task types (sampled):"
  for q in "${QUEUES[@]}"; do
    cli LRANGE "asynq:{$q}:pending" 0 50 2>/dev/null | python3 -c "
import sys, json
counts = {}
for line in sys.stdin:
    line = line.strip()
    if not line: continue
    try: msg = json.loads(line)
    except: continue
    t = msg.get('Type') or msg.get('type') or '?'
    counts[t] = counts.get(t, 0) + 1
for t, c in sorted(counts.items()):
    print(f'  $q: {t}  x{c}')
" 2>/dev/null
  done
}

require_redis
print_header
for q in "${QUEUES[@]}"; do
  print_queue_row "$q"
done
sample_pending_types
