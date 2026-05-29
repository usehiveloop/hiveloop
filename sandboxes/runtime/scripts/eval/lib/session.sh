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

stream_eval_output() {
  local event_type=""
  local current_section=""

  echo -e "${BLUE}[6/7] Streaming SSE output...${NC}"
  mkdir -p "$(dirname "$EVAL_SSE_LOG")"
  : >"$EVAL_SSE_LOG"
  : >"$EVAL_CONSOLE_LOG"
  echo -e "  SSE log: ${BOLD}$EVAL_SSE_LOG${NC}"
  echo -e "  Pretty log: ${BOLD}$EVAL_CONSOLE_LOG${NC}"
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
