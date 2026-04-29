#!/usr/bin/env bash
# Show counts of tasks per asynq queue + state. Useful for asserting that a
# webhook dispatch enqueued the expected task without grepping log strings.
#
# Asynq key layout (using cluster hash tags so all queue keys hash together):
#   asynq:{<queue>}:pending     LIST    waiting to be processed
#   asynq:{<queue>}:active      LIST    currently being worked on
#   asynq:{<queue>}:scheduled   ZSET    scheduled for a future time
#   asynq:{<queue>}:retry       ZSET    failed, retrying
#   asynq:{<queue>}:archived    ZSET    failed past max retries
#
# Set VERBOSE=1 to also list the task type names in pending lists.
set -e

REDIS_PORT="${REDIS_PORT:-6379}"
QUEUES=(critical default bulk periodic)

cli() { redis-cli -h 127.0.0.1 -p "$REDIS_PORT" "$@"; }

if ! cli ping 2>/dev/null | grep -q PONG; then
  echo "ERROR: redis not reachable on :$REDIS_PORT" >&2; exit 1
fi

printf "%-10s %8s %8s %10s %8s %10s\n" "queue" "pending" "active" "scheduled" "retry" "archived"
printf "%-10s %8s %8s %10s %8s %10s\n" "----------" "-------" "------" "---------" "-----" "--------"

for q in "${QUEUES[@]}"; do
  P="$(cli LLEN  "asynq:{$q}:pending"   2>/dev/null || echo 0)"
  A="$(cli LLEN  "asynq:{$q}:active"    2>/dev/null || echo 0)"
  S="$(cli ZCARD "asynq:{$q}:scheduled" 2>/dev/null || echo 0)"
  R="$(cli ZCARD "asynq:{$q}:retry"     2>/dev/null || echo 0)"
  X="$(cli ZCARD "asynq:{$q}:archived"  2>/dev/null || echo 0)"
  printf "%-10s %8s %8s %10s %8s %10s\n" "$q" "$P" "$A" "$S" "$R" "$X"
done

if [ "${VERBOSE:-}" = "1" ]; then
  echo ""
  echo "Pending task types (sampled):"
  for q in "${QUEUES[@]}"; do
    cli LRANGE "asynq:{$q}:pending" 0 50 2>/dev/null | python3 -c "
import sys, json
counts = {}
for line in sys.stdin:
    line = line.strip()
    if not line:
        continue
    try:
        msg = json.loads(line)
    except Exception:
        continue
    t = msg.get('Type') or msg.get('type') or '?'
    counts[t] = counts.get(t, 0) + 1
for t, c in sorted(counts.items()):
    print(f'  $q: {t}  x{c}')
" 2>/dev/null
  done
fi
