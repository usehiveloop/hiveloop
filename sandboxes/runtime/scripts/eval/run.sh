#!/usr/bin/env bash
set -euo pipefail

# E2E Evaluation: Rails 8.1 no-build app-builder subagents.
#
# Requirements:
#   - Docker running
#   - jq installed
#   - OPENROUTER_API_KEY env var, or HIVY_PROXY_API_KEY
#
# Usage:
#   OPENROUTER_API_KEY=sk-or-... ./scripts/eval/run.sh
#   EVAL_TASK="Build a waitlist app..." ./scripts/eval/run.sh
#   EVAL_SKIP_SESSION=1 ./scripts/eval/run.sh  # stop after config/readyz

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

source "$SCRIPT_DIR/lib/common.sh"
source "$SCRIPT_DIR/lib/runtime.sh"
source "$SCRIPT_DIR/lib/config.sh"
source "$SCRIPT_DIR/lib/session.sh"

main() {
  eval_init
  require_eval_dependencies
  trap cleanup_eval_container EXIT

  echo -e "${BLUE}Eval run:${NC} ${BOLD}$EVAL_RUN_ID${NC}"
  echo "  Run dir:   $EVAL_RUN_DIR"
  echo "  Image:     $IMAGE"
  echo "  Container: $CONTAINER"
  echo "  Port:      $PORT"

  load_eval_assets
  build_runtime_image
  start_eval_container
  create_rails_app
  push_agent_config

  if [[ "$EVAL_SKIP_SESSION" == "1" ]]; then
    echo -e "${GREEN}${BOLD}Dry run complete: runtime is ready and no agent session was created.${NC}"
    return 0
  fi

  send_eval_task
  stream_eval_output
  print_trace_summary
}

main "$@"
