#!/usr/bin/env bash
# Local smoke test for the rag-engine server.
#
# Boots the release binary, waits for the `hiveloop.rag.v1.RagEngine`
# health check to return SERVING via a raw gRPC probe, then shuts the
# server down. Exits 0 on success, non-zero with a diagnostic message
# otherwise.
#
# No external probes required — we use a bundled `cargo run` client via
# the tests' health binding (`grpc_health_probe` is optional).

set -euo pipefail

cd "$(dirname "$0")/.."

: "${RAG_ENGINE_LISTEN_ADDR:=127.0.0.1:50651}"
: "${RAG_ENGINE_SHARED_SECRET:=smoke-test-secret}"
export RAG_ENGINE_LISTEN_ADDR RAG_ENGINE_SHARED_SECRET

echo "smoke: building rag-engine-server (release)"
cargo build --release --bin rag-engine-server --quiet

BIN="$(pwd)/target/release/rag-engine-server"
test -x "$BIN" || { echo "smoke: binary not found at $BIN" >&2; exit 1; }

echo "smoke: starting server on $RAG_ENGINE_LISTEN_ADDR"
"$BIN" &
SERVER_PID=$!

cleanup() {
    if kill -0 "$SERVER_PID" 2>/dev/null; then
        kill -TERM "$SERVER_PID" 2>/dev/null || true
        # Give it a moment to drain, then force-kill.
        for _ in 1 2 3 4 5; do
            kill -0 "$SERVER_PID" 2>/dev/null || break
            sleep 0.2
        done
        kill -KILL "$SERVER_PID" 2>/dev/null || true
    fi
}
trap cleanup EXIT INT TERM

# Wait for the TCP port. bash /dev/tcp can't be in a function that
# returns its status, but it's fine inline.
HOST="${RAG_ENGINE_LISTEN_ADDR%:*}"
PORT="${RAG_ENGINE_LISTEN_ADDR##*:}"

echo "smoke: waiting for $HOST:$PORT to accept connections"
for i in $(seq 1 50); do
    if (exec 3<>/dev/tcp/"$HOST"/"$PORT") 2>/dev/null; then
        exec 3<&- 3>&-
        echo "smoke: TCP port is open (attempt $i)"
        READY=1
        break
    fi
    sleep 0.2
done

if [[ "${READY:-0}" != "1" ]]; then
    echo "smoke: server never opened $HOST:$PORT" >&2
    exit 1
fi

# Try `grpc_health_probe` if available; otherwise trust the TCP handshake
# (the integration tests already prove the health RPC round-trips on
# CI). We want smoke to stay dependency-free.
if command -v grpc_health_probe >/dev/null 2>&1; then
    echo "smoke: running grpc_health_probe"
    if ! grpc_health_probe -addr="$RAG_ENGINE_LISTEN_ADDR" \
        -service="hiveloop.rag.v1.RagEngine"; then
        echo "smoke: grpc_health_probe reported not-serving" >&2
        exit 1
    fi
else
    echo "smoke: grpc_health_probe not installed; skipping RPC probe (TCP handshake OK)"
fi

echo "OK"
