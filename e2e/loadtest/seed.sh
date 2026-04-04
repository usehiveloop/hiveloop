#!/usr/bin/env bash
# Seed 5 agents for load testing, reusing existing identity and credential.
# Outputs agent IDs to stdout (one per line).
set -euo pipefail

API="https://api.ziraloop.com"
AUTH="Authorization: Bearer $ZIRALOOP_API_KEY"
MODEL="google/gemini-2.5-flash-lite"

# --- Get or create identity ---
IDENTITY_ID=$(curl -s "$API/v1/identities?limit=1" -H "$AUTH" | \
  python3 -c "import sys,json; items=json.load(sys.stdin).get('data',[]); print(items[0]['id'] if items else '')")

if [ -z "$IDENTITY_ID" ]; then
  echo "Creating identity..." >&2
  IDENTITY_ID=$(curl -s -X POST "$API/v1/identities" -H "$AUTH" -H "Content-Type: application/json" \
    -d '{"external_id":"loadtest-user"}' | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])")
fi
echo "Identity: $IDENTITY_ID" >&2

# --- Get credential ---
CREDENTIAL_ID=$(curl -s "$API/v1/credentials?limit=1" -H "$AUTH" | \
  python3 -c "import sys,json; items=json.load(sys.stdin).get('data',[]); print(items[0]['id'] if items else '')")

if [ -z "$CREDENTIAL_ID" ]; then
  echo "ERROR: No credentials found. Create one with an OpenRouter API key first." >&2
  exit 1
fi
echo "Credential: $CREDENTIAL_ID" >&2

# --- Create 5 agents ---
PROMPTS=(
  "You are a math tutor. Give short, precise answers to math questions."
  "You are a geography expert. Answer questions about countries and capitals concisely."
  "You are a cooking assistant. Give brief recipe suggestions."
  "You are a fitness coach. Give short exercise recommendations."
  "You are a history teacher. Answer history questions in 1-2 sentences."
)

RUN_ID=$(date +%s)
NAMES=("math-lt-$RUN_ID" "geo-lt-$RUN_ID" "cook-lt-$RUN_ID" "fitness-lt-$RUN_ID" "history-lt-$RUN_ID")

AGENT_IDS=()
for i in 0 1 2 3 4; do
  NAME="${NAMES[$i]}"
  PROMPT="${PROMPTS[$i]}"

  AGENT_RESP=$(curl -s -X POST "$API/v1/agents" -H "$AUTH" -H "Content-Type: application/json" \
    -d "{
      \"name\": \"$NAME\",
      \"identity_id\": \"$IDENTITY_ID\",
      \"credential_id\": \"$CREDENTIAL_ID\",
      \"model\": \"$MODEL\",
      \"system_prompt\": \"$PROMPT\",
      \"sandbox_type\": \"shared\",
      \"agent_config\": {\"max_turns\": 100, \"max_tokens\": 1500}
    }")

  AID=$(echo "$AGENT_RESP" | python3 -c "import sys,json; print(json.load(sys.stdin).get('id',''))" 2>/dev/null)
  if [ -z "$AID" ]; then
    echo "ERROR creating agent $NAME: $AGENT_RESP" >&2
    exit 1
  fi
  echo "Created agent $NAME: $AID" >&2
  AGENT_IDS+=("$AID")
done

# Output agent IDs (one per line)
for AID in "${AGENT_IDS[@]}"; do
  echo "$AID"
done
