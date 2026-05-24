#!/usr/bin/env bash
set -euo pipefail
set -a; source .env; set +a

[ -n "${LINEAR_API_KEY:-}" ] || { echo "LINEAR_API_KEY not set"; exit 1; }

TMPDIR=${TMPDIR:-/tmp}
FIFO_IN="$TMPDIR/mcp_in_$$"
FIFO_OUT="$TMPDIR/mcp_out_$$"

mkfifo "$FIFO_IN" "$FIFO_OUT"

npx -y mcp-remote https://mcp.linear.app/mcp < "$FIFO_IN" > "$FIFO_OUT" 2>/dev/null &
PID=$!
sleep 2

cat > "$FIFO_IN" <<'EOF'
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}
EOF

read -t 5 -r _ < "$FIFO_OUT" || true

cat > "$FIFO_IN" <<'EOF'
{"jsonrpc":"2.0","method":"notifications/initialized"}
EOF

cat > "$FIFO_IN" <<'EOF'
{"jsonrpc":"2.0","id":2,"method":"tools/list"}
EOF

read -t 5 -r tools_resp < "$FIFO_OUT" || true

if [ -n "$tools_resp" ]; then
    count=$(echo "$tools_resp" | python3 -c 'import json,sys; d=json.load(sys.stdin); print(len(d["result"]["tools"]))')
    echo "Total tools: $count"
    echo "---"
    echo "$tools_resp" | python3 -c '
import json,sys
d=json.load(sys.stdin)
for t in d["result"]["tools"]:
    print(t["name"])
'
fi

kill $PID 2>/dev/null || true
wait $PID 2>/dev/null || true
rm -f "$FIFO_IN" "$FIFO_OUT"
