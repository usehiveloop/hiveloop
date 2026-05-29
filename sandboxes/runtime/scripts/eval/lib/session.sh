#!/usr/bin/env bash

set -euo pipefail

send_eval_task() {
  local msg_response

  echo -e "${BLUE}[5/7] Sending task...${NC}"
  echo -e "  ${GRAY}$EVAL_TASK${NC}"

	  msg_response=$(curl -s -X POST "$BASE_URL/gateway/http/messages" \
	    -H "Authorization: Bearer $SECRET" \
	    -H "Content-Type: application/json" \
	    -d "$(jq -n \
	      --arg text "$EVAL_TASK" \
	      --arg run_id "$EVAL_RUN_ID" \
	      --arg run_version "$EVAL_RUN_VERSION" \
	      '{text: $text, user: "eval-user", user_display_name: "Evaluator", raw: {eval_run_id: $run_id, eval_run_version: $run_version}}')")

  STREAM_ID=$(echo "$msg_response" | jq -r '.stream_id')
  SESSION_ID=$(echo "$msg_response" | jq -r '.session_id')
  TRACE_ID=$(echo "$msg_response" | jq -r '.trace_id')

  if [[ -z "$STREAM_ID" || "$STREAM_ID" == "null" ]]; then
    echo -e "  ${RED}Failed to get stream_id${NC}"
    echo "$msg_response" | jq .
    exit 1
  fi

  echo -e "  Session: ${BOLD}$SESSION_ID${NC}"
  echo -e "  Stream:  ${BOLD}$STREAM_ID${NC}"
  echo -e "  Trace:   ${BOLD}$TRACE_ID${NC}"
}

declare -a SUBAGENT_PIDS=()

stream_eval_output() {
  local event_type=""
  local current_section=""

  echo -e "${BLUE}[6/7] Streaming SSE output...${NC}"
  mkdir -p "$(dirname "$EVAL_SSE_LOG")"
  mkdir -p "$EVAL_RUN_DIR/subagents"
  : >"$EVAL_SSE_LOG"
  : >"$EVAL_CONSOLE_LOG"
  echo -e "  SSE log: ${BOLD}$EVAL_SSE_LOG${NC}"
  echo -e "  Pretty log: ${BOLD}$EVAL_CONSOLE_LOG${NC}"
  echo -e "  Subagent streams: ${BOLD}$EVAL_RUN_DIR/subagents/${NC}"
  echo -e "${GRAY}────────────────────────────────────────────────────${NC}"

  curl -sN "$BASE_URL/gateway/http/streams/$STREAM_ID" \
    -H "Authorization: Bearer $SECRET" \
    -H "Accept: text/event-stream" | while IFS= read -r line; do
    printf '%s\n' "$line" >>"$EVAL_SSE_LOG"

    if [[ "$line" == event:* ]]; then
      event_type="${line#event: }"
      event_type="${event_type%%[[:space:]]*}"
    elif [[ "$line" == data:* ]]; then
      if ! stream_event "$event_type" "${line#data: }" current_section; then
        break
      fi
    fi
  done

  # Wait for all subagent streams to finish
  if [[ ${#SUBAGENT_PIDS[@]} -gt 0 ]]; then
    echo -e "\n${GRAY}Waiting for ${#SUBAGENT_PIDS[@]} subagent stream(s)...${NC}"
    for pid in "${SUBAGENT_PIDS[@]}"; do
      wait "$pid" 2>/dev/null || true
    done
    echo -e "${GREEN}All subagent streams complete.${NC}"
  fi
}

subscribe_subagent_stream() {
  local agent_name="$1"
  local stream_url="$2"
  local job_id="$3"
  local subagent_dir="$EVAL_RUN_DIR/subagents"
  local sse_file="$subagent_dir/${job_id}-${agent_name}.sse"
  local console_file="$subagent_dir/${job_id}-${agent_name}.log"

  mkdir -p "$subagent_dir"
  echo -e "  ${BLUE}[subagent]${NC} Subscribing to ${BOLD}$agent_name${NC} stream: $stream_url"

  (
    local sa_event_type=""
    curl -sN "$BASE_URL$stream_url" \
      -H "Authorization: Bearer $SECRET" \
      -H "Accept: text/event-stream" | while IFS= read -r line; do
      printf '%s\n' "$line" >>"$sse_file"

      if [[ "$line" == event:* ]]; then
        sa_event_type="${line#event: }"
        sa_event_type="${sa_event_type%%[[:space:]]*}"
      elif [[ "$line" == data:* ]]; then
        local sa_data="${line#data: }"
        case "$sa_event_type" in
          token)
            echo "$sa_data" | jq -r '.text // empty' 2>/dev/null >>"$console_file"
            ;;
          thinking)
            printf "[thinking] %s\n" "$(echo "$sa_data" | jq -r '.text // empty' 2>/dev/null | head -c 200)" >>"$console_file"
            ;;
          tool_call)
            printf "[tool_call] %s %s\n" \
              "$(echo "$sa_data" | jq -r '.tool // "?"' 2>/dev/null)" \
              "$(echo "$sa_data" | jq -c '.args // {}' 2>/dev/null | head -c 200)" >>"$console_file"
            ;;
          tool_result)
            printf "[tool_result] %s\n" "$(echo "$sa_data" | jq -c '.result // .' 2>/dev/null | head -c 300)" >>"$console_file"
            ;;
          final)
            printf "\n[final]\n%s\n" "$(echo "$sa_data" | jq -r '.text // empty' 2>/dev/null)" >>"$console_file"
            ;;
          done)
            printf "[done]\n" >>"$console_file"
            return 0
            ;;
          error|model_request_failed|model_stream_failed)
            printf "[error] %s\n" "$(echo "$sa_data" | jq -r '.message // .error // empty' 2>/dev/null)" >>"$console_file"
            ;;
          model_usage)
            printf "[usage] %s\n" "$(echo "$sa_data" | jq -c '.usage // {}' 2>/dev/null)" >>"$console_file"
            ;;
        esac
      fi
    done
  ) &
  SUBAGENT_PIDS+=($!)
}

stream_event() {
  local event_type="$1"
  local data="$2"
  local section_var="$3"
  local text tool args result msg usage command exit_code output shown_lines total_lines truncated
  local section

  section="${!section_var}"

  case "$event_type" in
    token)
      switch_stream_section "$section_var" "$section" "assistant"
      text=$(echo "$data" | jq -r '.text // empty' 2>/dev/null)
      printf "%s" "$text" | tee -a "$EVAL_CONSOLE_LOG"
      ;;
    thinking)
      switch_stream_section "$section_var" "$section" "thinking"
      text=$(echo "$data" | jq -r '.text // empty' 2>/dev/null)
      printf "%s" "$text" | tee -a "$EVAL_CONSOLE_LOG"
      ;;
    tool_call)
      end_stream_section "$section_var" "$section"
      tool=$(echo "$data" | jq -r '.tool // "?"' 2>/dev/null)
      args=$(echo "$data" | jq -c '.args // {}' 2>/dev/null | head -c 300)
      printf "${YELLOW}[tool_call]${NC} %s\n" "$tool" | tee -a "$EVAL_CONSOLE_LOG"
      printf "${GRAY}%s${NC}\n" "$args" | tee -a "$EVAL_CONSOLE_LOG"
      ;;
    tool_result)
      end_stream_section "$section_var" "$section"
      print_tool_result "$data"
      ;;
    final)
      end_stream_section "$section_var" "$section"
      text=$(echo "$data" | jq -r '.text // empty' 2>/dev/null)
      printf "\n${BOLD}[final]${NC}\n%s\n" "$text" | tee -a "$EVAL_CONSOLE_LOG"
      ;;
    done)
      end_stream_section "$section_var" "$section"
      printf "${GRAY}────────────────────────────────────────────────────${NC}\n" | tee -a "$EVAL_CONSOLE_LOG"
      printf "${GREEN}${BOLD}DONE${NC}\n" | tee -a "$EVAL_CONSOLE_LOG"
      return 1
      ;;
    error|model_request_failed|model_stream_failed)
      end_stream_section "$section_var" "$section"
      msg=$(echo "$data" | jq -r '.message // .error // empty' 2>/dev/null)
      printf "${RED}[error]${NC} %s\n" "$msg" | tee -a "$EVAL_CONSOLE_LOG"
      ;;
    turn_started)
      end_stream_section "$section_var" "$section"
      printf "${BLUE}[turn_started]${NC}\n" | tee -a "$EVAL_CONSOLE_LOG"
      ;;
    subagent_started)
      end_stream_section "$section_var" "$section"
      local sa_agent sa_stream_url sa_job_id
      sa_agent=$(echo "$data" | jq -r '.agent_name // "unknown"' 2>/dev/null)
      sa_stream_url=$(echo "$data" | jq -r '.stream_url // empty' 2>/dev/null)
      sa_job_id=$(echo "$data" | jq -r '.job_id // empty' 2>/dev/null)
      printf "${BLUE}[subagent_started]${NC} %s → %s\n" "$sa_agent" "$sa_stream_url" | tee -a "$EVAL_CONSOLE_LOG"
      if [[ -n "$sa_stream_url" ]]; then
        subscribe_subagent_stream "$sa_agent" "$sa_stream_url" "$sa_job_id"
      fi
      ;;
    subagent_completed)
      end_stream_section "$section_var" "$section"
      local sa_agent_done sa_job_done
      sa_agent_done=$(echo "$data" | jq -r '.agent_name // "unknown"' 2>/dev/null)
      sa_job_done=$(echo "$data" | jq -r '.job_id // empty' 2>/dev/null)
      printf "${GREEN}[subagent_completed]${NC} %s (job: %s)\n" "$sa_agent_done" "$sa_job_done" | tee -a "$EVAL_CONSOLE_LOG"
      ;;
    subagent_errored)
      end_stream_section "$section_var" "$section"
      local sa_agent_err sa_job_err sa_error
      sa_agent_err=$(echo "$data" | jq -r '.agent_name // "unknown"' 2>/dev/null)
      sa_job_err=$(echo "$data" | jq -r '.job_id // empty' 2>/dev/null)
      sa_error=$(echo "$data" | jq -r '.error // "unknown"' 2>/dev/null)
      printf "${RED}[subagent_errored]${NC} %s (job: %s): %s\n" "$sa_agent_err" "$sa_job_err" "$sa_error" | tee -a "$EVAL_CONSOLE_LOG"
      ;;
    model_usage)
      end_stream_section "$section_var" "$section"
      usage=$(echo "$data" | jq -c '.usage // {}' 2>/dev/null)
      printf "${GRAY}[usage]${NC} %s\n" "$usage" | tee -a "$EVAL_CONSOLE_LOG"
      ;;
    *)
      end_stream_section "$section_var" "$section"
      printf "${GRAY}[%s]${NC} %s\n" "$event_type" "$(echo "$data" | head -c 300)" | tee -a "$EVAL_CONSOLE_LOG"
      ;;
  esac
}

switch_stream_section() {
  local section_var="$1"
  local section="$2"
  local next_section="$3"

  if [[ "$section" == "$next_section" ]]; then
    return 0
  fi

  if [[ -n "$section" ]]; then
    printf "\n" | tee -a "$EVAL_CONSOLE_LOG"
  fi

  case "$next_section" in
    thinking)
      printf "${BLUE}[thinking]${NC}\n" | tee -a "$EVAL_CONSOLE_LOG"
      ;;
    assistant)
      printf "${GRAY}[assistant]${NC}\n" | tee -a "$EVAL_CONSOLE_LOG"
      ;;
  esac

  printf -v "$section_var" '%s' "$next_section"
}

end_stream_section() {
  local section_var="$1"
  local section="$2"

  if [[ -n "$section" ]]; then
    printf "\n" | tee -a "$EVAL_CONSOLE_LOG"
    printf -v "$section_var" ''
  fi
}

print_tool_result() {
  local data="$1"
  local result command exit_code output shown_lines total_lines truncated

  result=$(echo "$data" | jq -c '.result // .' 2>/dev/null)
  command=$(echo "$result" | jq -r '.command // empty' 2>/dev/null)
  exit_code=$(echo "$result" | jq -r '.exit_code // empty' 2>/dev/null)
  output=$(echo "$result" | jq -r '.output // .stdout // .stderr // empty' 2>/dev/null)
  shown_lines=$(echo "$result" | jq -r '.shown_lines // empty' 2>/dev/null)
  total_lines=$(echo "$result" | jq -r '.total_lines // empty' 2>/dev/null)
  truncated=$(echo "$result" | jq -r '.truncated // false' 2>/dev/null)

  if [[ -n "$command" || -n "$output" || -n "$exit_code" ]]; then
    printf "${GREEN}[tool_result]${NC} exit=%s lines=%s/%s truncated=%s\n" \
      "${exit_code:-?}" "${shown_lines:-?}" "${total_lines:-?}" "$truncated" | tee -a "$EVAL_CONSOLE_LOG"
    if [[ -n "$command" ]]; then
      printf "${GRAY}$ %s${NC}\n" "$command" | tee -a "$EVAL_CONSOLE_LOG"
    fi
    if [[ -n "$output" ]]; then
      printf "%s\n" "$output" | sed '/^$/d' | tee -a "$EVAL_CONSOLE_LOG"
    fi
    return 0
  fi

  printf "${GREEN}[tool_result]${NC} %s\n" "$(echo "$result" | head -c 800)" | tee -a "$EVAL_CONSOLE_LOG"
}

print_trace_summary() {
  local trace_summary

  echo -e "\n${BLUE}[7/7] Trace summary...${NC}"

  trace_summary=$(curl -s "$BASE_URL/observability/traces/$TRACE_ID/summary" \
    -H "Authorization: Bearer $SECRET")

  echo "$trace_summary" | jq . | tee "$EVAL_TRACE_SUMMARY"
  echo -e "Trace summary: ${BOLD}$EVAL_TRACE_SUMMARY${NC}"
}
