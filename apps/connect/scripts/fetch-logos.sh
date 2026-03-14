#!/usr/bin/env bash
# Downloads provider logo SVGs from models.dev into public/logos/.
# Usage: ./scripts/fetch-logos.sh
# Reads provider IDs from ../../internal/registry/models.json

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT="$SCRIPT_DIR/.."
MODELS_JSON="$ROOT/../../internal/registry/models.json"
OUT_DIR="$ROOT/public/logos"

if [ ! -f "$MODELS_JSON" ]; then
  echo "Error: models.json not found at $MODELS_JSON" >&2
  exit 1
fi

mkdir -p "$OUT_DIR"

# Extract all provider IDs from the JSON array
IDS=$(python3 -c "
import json, sys
with open('$MODELS_JSON') as f:
    data = json.load(f)
for p in data:
    print(p['id'])
")

TOTAL=$(echo "$IDS" | wc -l | tr -d ' ')
COUNT=0
FAILED=0

echo "Fetching $TOTAL provider logos into $OUT_DIR ..."

for id in $IDS; do
  COUNT=$((COUNT + 1))
  URL="https://models.dev/logos/${id}.svg"
  DEST="$OUT_DIR/${id}.svg"

  if curl -sfL --max-time 10 -o "$DEST" "$URL"; then
    printf "  [%3d/%d] ✓ %s\n" "$COUNT" "$TOTAL" "$id"
  else
    printf "  [%3d/%d] ✗ %s (failed)\n" "$COUNT" "$TOTAL" "$id"
    FAILED=$((FAILED + 1))
    rm -f "$DEST"
  fi
done

echo ""
echo "Done: $((TOTAL - FAILED))/$TOTAL logos downloaded. $FAILED failed."
