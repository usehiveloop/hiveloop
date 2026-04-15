#!/bin/bash
# Test MCP server connectivity from inside a sandbox.
# Usage: ./scripts/test-sandbox-mcp.sh <sandbox-external-id>
#
# Requires: DAYTONA_API_URL and DAYTONA_API_KEY env vars, or pass via args.

set -euo pipefail

SANDBOX_ID="${1:?Usage: $0 <sandbox-external-id>}"
API_URL="${DAYTONA_API_URL:-}"
API_KEY="${DAYTONA_API_KEY:-}"

if [[ -z "$API_URL" || -z "$API_KEY" ]]; then
  echo "Set DAYTONA_API_URL and DAYTONA_API_KEY env vars"
  exit 1
fi

exec_cmd() {
  local cmd="$1"
  local result
  result=$(curl -sf -X POST \
    -H "Authorization: Bearer $API_KEY" \
    -H "Content-Type: application/json" \
    -d "{\"command\": \"$cmd\"}" \
    "$API_URL/toolbox/$SANDBOX_ID/toolbox/process/execute" 2>&1) || true
  echo "$result" | jq -r '.result // .error // .' 2>/dev/null || echo "$result"
}

echo "=== Environment ==="
exec_cmd "env | grep -E 'ZIRALOOP_|BRIDGE_|MCP' | sort"

echo ""
echo "=== Binaries ==="
exec_cmd "which codedb 2>/dev/null && codedb --version || echo 'codedb: not found'"
exec_cmd "which ziraloop-embeddings 2>/dev/null && ziraloop-embeddings version || echo 'ziraloop-embeddings: not found'"

echo ""
echo "=== Repos ==="
exec_cmd "ls -la /home/daytona/repos/ 2>/dev/null || echo 'no repos directory'"

echo ""
echo "=== Test codedb stdio ==="
# Send a JSON-RPC initialize request to codedb via stdin
exec_cmd "echo '{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"initialize\",\"params\":{\"protocolVersion\":\"2024-11-05\",\"capabilities\":{},\"clientInfo\":{\"name\":\"test\",\"version\":\"1.0\"}}}' | timeout 5 codedb /home/daytona/repos 2>/dev/null | head -1 || echo 'codedb mcp failed'"

echo ""
echo "=== Test ziraloop MCP endpoint ==="
exec_cmd "curl -sf -X POST -H 'Content-Type: application/json' -H \"Authorization: Bearer \$BRIDGE_CONTROL_PLANE_API_KEY\" \"\$ZIRALOOP_MCP_URL\" -d '{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"initialize\",\"params\":{\"protocolVersion\":\"2024-11-05\",\"capabilities\":{},\"clientInfo\":{\"name\":\"test\",\"version\":\"1.0\"}}}' 2>&1 | head -5 || echo 'ziraloop MCP failed'"

echo ""
echo "=== Test memory MCP endpoint ==="
exec_cmd "curl -sf -X POST -H 'Content-Type: application/json' \"\$(echo \$BRIDGE_WEBHOOK_URL | sed 's|/internal/webhooks/bridge/.*||')/mcp/memory/\$ZIRALOOP_AGENT_ID/\" -d '{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"initialize\",\"params\":{\"protocolVersion\":\"2024-11-05\",\"capabilities\":{},\"clientInfo\":{\"name\":\"test\",\"version\":\"1.0\"}}}' 2>&1 | head -5 || echo 'memory MCP failed'"

echo ""
echo "=== Bridge health ==="
exec_cmd "curl -sf http://localhost:25434/health || echo 'bridge not healthy'"

echo ""
echo "=== Bridge agents ==="
exec_cmd "curl -sf -H \"Authorization: Bearer \$BRIDGE_CONTROL_PLANE_API_KEY\" http://localhost:25434/agents | jq '.[].id' 2>/dev/null || echo 'no agents'"
