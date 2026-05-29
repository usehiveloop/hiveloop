#!/usr/bin/env bash

set -euo pipefail

load_eval_assets() {
  CONFIG_TEMPLATE="$ROOT/config.json"
}

resolve_ref_path() {
  local ref="$1"
  local path="$ROOT/$ref"

  if [[ "$ref" = /* || "$ref" == *".."* || ! -f "$path" ]]; then
    echo -e "${RED}ERROR: invalid config reference: $ref${NC}" >&2
    exit 1
  fi

  printf '%s' "$path"
}

read_text_ref() {
  local ref="$1"
  local path

  path=$(resolve_ref_path "$ref")
  if [[ "$path" == */SKILL.md ]]; then
    skill_body "$path"
  else
    cat "$path"
  fi
}

collect_text_refs() {
  jq -r '
    [
      .. | objects |
      (
        to_entries[]
        | select(
            (.key | endswith("_ref"))
            and (.key | endswith("_json_ref") | not)
            and .key != "files_ref"
          )
        | .value
      ),
      (.. | objects | select(has("files_ref")) | .files_ref[]?)
    ]
    | unique[]
  ' "$CONFIG_TEMPLATE"
}

collect_json_refs() {
  jq -r '
    [
      .. | objects |
      to_entries[]
      | select(.key | endswith("_json_ref"))
      | .value
    ]
    | unique[]
  ' "$CONFIG_TEMPLATE"
}

build_text_refs_json() {
  local refs="{}"
  local ref
  local content

  while IFS= read -r ref; do
    [[ -n "$ref" ]] || continue
    content=$(read_text_ref "$ref")
    refs=$(jq -n \
      --argjson refs "$refs" \
      --arg ref "$ref" \
      --arg content "$content" \
      '$refs + {($ref): $content}')
  done < <(collect_text_refs)

  printf '%s' "$refs"
}

build_json_refs_json() {
  local refs="{}"
  local ref
  local path

  while IFS= read -r ref; do
    [[ -n "$ref" ]] || continue
    path=$(resolve_ref_path "$ref")
    refs=$(jq -n \
      --argjson refs "$refs" \
      --arg ref "$ref" \
      --slurpfile content "$path" \
      '$refs + {($ref): $content[0]}')
  done < <(collect_json_refs)

  printf '%s' "$refs"
}

build_agent_config() {
  local definition

  definition=$(jq -n \
    --slurpfile template "$CONFIG_TEMPLATE" \
    --argjson text_refs "$(build_text_refs_json)" \
    --argjson json_refs "$(build_json_refs_json)" \
    -f "$EVAL_DIR/config/resolve-config.jq")

  jq -n \
    --arg api_key "$API_KEY" \
    --argjson definition "$definition" \
    '{"runtime_env":{"HIVY_PROXY_API_KEY":$api_key},"definition":$definition}'
}

push_agent_config() {
  local config_json
  local http_code

  echo -e "${BLUE}[4/7] Pushing Rails app-builder agent config...${NC}"
  config_json=$(build_agent_config)
  printf '%s\n' "$config_json" | jq . >"$EVAL_RENDERED_CONFIG"
  echo -e "  Rendered config: ${BOLD}$EVAL_RENDERED_CONFIG${NC}"

  http_code=$(curl -s -o /dev/null -w "%{http_code}" \
    -X PUT "$BASE_URL/config" \
    -H "Authorization: Bearer $SECRET" \
    -H "Content-Type: application/json" \
    -d "$config_json")

  if [[ "$http_code" != "200" ]]; then
    echo -e "  ${RED}Config push failed (HTTP $http_code)${NC}"
    curl -s -X PUT "$BASE_URL/config" \
      -H "Authorization: Bearer $SECRET" \
      -H "Content-Type: application/json" \
      -d "$config_json" | jq .
    exit 1
  fi

  echo -e "  ${GREEN}Config pushed successfully${NC}"
  wait_for_runtime_path "readyz" "readyz" "auth"
}
