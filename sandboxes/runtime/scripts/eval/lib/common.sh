#!/usr/bin/env bash

set -euo pipefail

eval_init() {
  ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
  EVAL_DIR="$ROOT/scripts/eval"
  export PATH="/usr/local/bin:/opt/homebrew/bin:$HOME/.cargo/bin:$PATH"

  EVAL_RUN_VERSION="${EVAL_RUN_VERSION:-}"
  EVAL_RUN_ID="${EVAL_RUN_ID:-${EVAL_RUN_VERSION:-$(date -u +%Y%m%dT%H%M%SZ)-$$}}"
  EVAL_RUN_DIR="${EVAL_RUN_DIR:-$EVAL_DIR/runs/$EVAL_RUN_ID}"
  IMAGE="${HIVY_EVAL_IMAGE:-hivy-runtime-rails-eval:eval-$EVAL_RUN_ID}"
  CONTAINER="${HIVY_EVAL_CONTAINER:-hivy-rails-eval-${EVAL_RUN_VERSION:-$$}}"
  SECRET="${HIVY_RUNTIME_SECRET:-eval-secret-$$}"
  PORT="${HIVY_EVAL_PORT:-$(pick_eval_port)}"
  BASE_URL="http://localhost:${PORT}"
  API_KEY="${OPENROUTER_API_KEY:-${HIVY_PROXY_API_KEY:-}}"
  RAILS_VERSION="${EVAL_RAILS_VERSION:-8.1.3}"
  APP_PATH="/workspace/app"
  GIT_USERNAME="${HIVY_GIT_USERNAME:-Hivy Eval}"
  GIT_EMAIL="${HIVY_GIT_EMAIL:-eval@usehivy.local}"
  EVAL_SKIP_SESSION="${EVAL_SKIP_SESSION:-0}"
  EVAL_KEEP_CONTAINER="${EVAL_KEEP_CONTAINER:-0}"
  EVAL_REBUILD_IMAGE="${EVAL_REBUILD_IMAGE:-1}"
  EVAL_REBUILD_BINARY="${EVAL_REBUILD_BINARY:-1}"
  EVAL_REPLACE_CONTAINER="${EVAL_REPLACE_CONTAINER:-0}"
  EVAL_SSE_LOG="${EVAL_SSE_LOG:-$EVAL_RUN_DIR/stream.sse}"
  EVAL_CONSOLE_LOG="${EVAL_CONSOLE_LOG:-$EVAL_RUN_DIR/console.log}"
  EVAL_DOCKER_LOG="${EVAL_DOCKER_LOG:-$EVAL_RUN_DIR/docker.log}"
  EVAL_RENDERED_CONFIG="${EVAL_RENDERED_CONFIG:-$EVAL_RUN_DIR/config.rendered.json}"
  EVAL_TRACE_SUMMARY="${EVAL_TRACE_SUMMARY:-$EVAL_RUN_DIR/trace-summary.json}"
  DOCKER_LOG_PID=""

  DEFAULT_EVAL_TASK="Build me a habit tracker where I can add habits, check them off each day, see my current streaks, edit a habit, and delete habits I no longer want."
  EVAL_TASK="${EVAL_TASK:-$DEFAULT_EVAL_TASK}"

  RED=$'\033[0;31m'
  GREEN=$'\033[0;32m'
  YELLOW=$'\033[1;33m'
  BLUE=$'\033[0;34m'
  GRAY=$'\033[0;90m'
  BOLD=$'\033[1m'
  NC=$'\033[0m'

  mkdir -p "$EVAL_RUN_DIR"
}

require_eval_dependencies() {
  if [[ -z "$API_KEY" ]]; then
    echo -e "${RED}ERROR: OPENROUTER_API_KEY or HIVY_PROXY_API_KEY must be set${NC}" >&2
    exit 1
  fi

  if ! command -v docker &>/dev/null; then
    echo -e "${RED}ERROR: docker is required${NC}" >&2
    exit 1
  fi

  if ! command -v jq &>/dev/null; then
    echo -e "${RED}ERROR: jq is required${NC}" >&2
    exit 1
  fi

  if ! port_available "$PORT"; then
    echo -e "${RED}ERROR: localhost port $PORT is already in use${NC}" >&2
    echo "Set HIVY_EVAL_PORT to a free port, or omit it and the eval will pick one." >&2
    exit 1
  fi
}

pick_eval_port() {
  python3 - <<'PY'
import socket
with socket.socket() as s:
    s.bind(("127.0.0.1", 0))
    print(s.getsockname()[1])
PY
}

port_available() {
  local port="$1"
  python3 - "$port" <<'PY'
import socket
import sys
port = int(sys.argv[1])
with socket.socket() as s:
    try:
        s.bind(("127.0.0.1", port))
    except OSError:
        sys.exit(1)
PY
}

skill_body() {
  awk '
    NR == 1 && $0 == "---" { in_frontmatter = 1; next }
    in_frontmatter && $0 == "---" { in_frontmatter = 0; next }
    !in_frontmatter { print }
  ' "$1"
}

cleanup_eval_container() {
  if [[ -n "${DOCKER_LOG_PID:-}" ]]; then
    kill "$DOCKER_LOG_PID" 2>/dev/null || true
    wait "$DOCKER_LOG_PID" 2>/dev/null || true
  fi

  if docker ps -a --format '{{.Names}}' | grep -Fxq "$CONTAINER"; then
    docker logs "$CONTAINER" >"$EVAL_DOCKER_LOG" 2>&1 || true
  fi

  if [[ "$EVAL_KEEP_CONTAINER" == "1" ]]; then
    echo -e "\n${YELLOW}Keeping container for inspection: $CONTAINER${NC}"
    echo -e "Docker log: ${BOLD}$EVAL_DOCKER_LOG${NC}"
    return 0
  fi

  echo -e "\n${YELLOW}Cleaning up...${NC}"
  docker stop "$CONTAINER" 2>/dev/null || true
  docker rm "$CONTAINER" 2>/dev/null || true
}

wait_for_runtime_path() {
  local path="$1"
  local label="$2"
  local auth="${3:-}"

  echo -n "  Waiting for ${label}"
  for i in $(seq 1 30); do
    if [[ "$auth" == "auth" ]]; then
      if curl -sf -H "Authorization: Bearer $SECRET" "$BASE_URL/$path" >/dev/null 2>&1; then
        echo -e " ${GREEN}OK${NC}"
        return 0
      fi
    else
      if curl -sf "$BASE_URL/$path" >/dev/null 2>&1; then
        echo -e " ${GREEN}OK${NC}"
        return 0
      fi
    fi
    if [[ $i -eq 30 ]]; then
      echo -e " ${RED}TIMEOUT${NC}"
      return 1
    fi
    echo -n "."
    sleep 1
  done
}
