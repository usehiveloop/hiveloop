#!/usr/bin/env bash
# Full load test: seed agents → run k6 → stream results
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

# --- Check requirements ---
if [ -z "${LLMVAULT_API_KEY:-}" ]; then
  echo "ERROR: Set LLMVAULT_API_KEY environment variable"
  echo "  export LLMVAULT_API_KEY=llmv_sk_..."
  exit 1
fi

if ! command -v k6 &>/dev/null; then
  echo "ERROR: k6 not installed. Run: brew install k6"
  exit 1
fi

echo "============================================"
echo " LLMVault Load Test"
echo " 200 conversations × 5 agents × 3 turns"
echo " 5 stream subscribers per conversation"
echo " Model: google/gemini-2.5-flash-lite"
echo "============================================"
echo ""

# --- Phase 1: Seed agents ---
echo "Phase 1: Seeding 5 agents..."
AGENT_IDS=$(bash "$SCRIPT_DIR/seed.sh")
AGENT_CSV=$(echo "$AGENT_IDS" | tr '\n' ',' | sed 's/,$//')
echo ""
echo "Agents: $AGENT_CSV"
echo ""

# --- Phase 2: Warm up (create one conversation to ensure sandbox is ready) ---
echo "Phase 2: Warming up sandbox..."
FIRST_AGENT=$(echo "$AGENT_IDS" | head -1)
WARMUP=$(curl -s --max-time 180 -X POST "https://api.llmvault.dev/v1/agents/$FIRST_AGENT/conversations" \
  -H "Authorization: Bearer $LLMVAULT_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{}')
WARMUP_ID=$(echo "$WARMUP" | python3 -c "import sys,json; print(json.load(sys.stdin).get('id','FAILED'))" 2>/dev/null)
echo "Warmup conversation: $WARMUP_ID"
if [ "$WARMUP_ID" = "FAILED" ]; then
  echo "ERROR: Warmup failed: $WARMUP"
  exit 1
fi

# Send a test message and wait for response
curl -s --max-time 15 -X POST "https://api.llmvault.dev/v1/conversations/$WARMUP_ID/messages" \
  -H "Authorization: Bearer $LLMVAULT_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"content": "Say ok."}' >/dev/null
echo "Waiting for warmup response..."
for i in $(seq 1 30); do
  EVENTS=$(curl -s --max-time 5 "https://api.llmvault.dev/v1/conversations/$WARMUP_ID/events" \
    -H "Authorization: Bearer $LLMVAULT_API_KEY" | python3 -c "
import sys,json
d=json.loads(sys.stdin.read(),strict=False)
turns=[e for e in d.get('data',[]) if e.get('event_type')=='turn_completed']
print(len(turns))
" 2>/dev/null)
  if [ "$EVENTS" -ge 1 ] 2>/dev/null; then
    echo "Warmup complete."
    break
  fi
  sleep 2
done
echo ""

# --- Phase 3: Run k6 load test ---
echo "Phase 3: Starting k6 load test..."
echo "============================================"
echo ""

k6 run \
  --env API_KEY="$LLMVAULT_API_KEY" \
  --env AGENT_IDS="$AGENT_CSV" \
  "$SCRIPT_DIR/loadtest.js"

echo ""
echo "============================================"
echo " Load test complete!"
echo "============================================"
