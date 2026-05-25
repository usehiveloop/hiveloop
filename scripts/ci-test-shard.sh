#!/usr/bin/env bash
set -euo pipefail

suite="${1:?suite is required}"
shard_index="${SHARD_INDEX:-${CI_NODE_INDEX:-0}}"
shard_total="${SHARD_TOTAL:-${CI_NODE_TOTAL:-1}}"
timeout="${GO_TEST_TIMEOUT:-3m}"

test_env=(
  "DATABASE_URL=postgres://hivy:localdev@localhost:5433/hivy_test?sslmode=disable"
  "HIVY_DATABASE_URL=postgres://hivy:localdev@localhost:5433/hivy_test?sslmode=disable"
  "TEST_DATABASE_URL=postgres://hivy:localdev@localhost:5433/hivy_test?sslmode=disable"
  "HIVY_NANGO_DATABASE_URL=postgres://hivy:localdev@localhost:5433/nango?sslmode=disable"
  "HIVY_REDIS_ADDR=localhost:16279"
  "TEST_REDIS_ADDR=localhost:16279"
  "HIVY_NANGO_ENDPOINT=http://localhost:23003"
  "HIVY_QDRANT_HOST=localhost"
  "HIVY_QDRANT_PORT=6334"
  "HIVY_QDRANT_USE_TLS=false"
  "HIVY_HINDSIGHT_API_URL=http://localhost:8888"
  "HIVY_PUBLIC_ASSETS_S3_ENDPOINT=http://localhost:9000"
  "HIVY_AWS_ENDPOINT_URL=http://localhost:9000"
)

shard_lines() {
  awk -v s="$shard_index" -v n="$shard_total" 'NR % n == s'
}

run_packages() {
  local packages="$1"
  if [[ -z "$packages" ]]; then
    echo "No packages assigned to shard $shard_index/$shard_total"
    return 0
  fi
  echo "$packages"
  printf '%s\n' "$packages" | xargs env "${test_env[@]}" go test -count=1 -timeout="$timeout"
}

run_test_names() {
  local package="$1"
  local tests
  tests="$(env "${test_env[@]}" go test "$package" -list '^Test' | awk '/^Test/ {print}' | shard_lines)"
  if [[ -z "$tests" ]]; then
    echo "No tests assigned to shard $shard_index/$shard_total for $package"
    return 0
  fi
  local pattern
  pattern="$(printf '%s\n' "$tests" | paste -sd'|' -)"
  echo "$tests"
  env "${test_env[@]}" go test "$package" -count=1 -timeout="$timeout" -run "^(${pattern})$"
}

internal_packages() {
  go list ./internal/...
}

internal_core_packages() {
  internal_packages | grep -Ev 'internal/(handler|hindsight|integrations|nango|rag|storage|tasks)(/|$)'
}

case "$suite" in
  internal-core)
    run_packages "$(internal_core_packages | shard_lines)"
    ;;
  internal-handler)
    run_test_names ./internal/handler
    ;;
  internal-rag)
    run_packages "$(internal_packages | grep -E 'internal/rag(/|$)' | shard_lines)"
    ;;
  internal-tasks)
    run_test_names ./internal/tasks
    ;;
  internal-hindsight)
    run_packages "$(internal_packages | grep -E 'internal/hindsight(/|$)' | shard_lines)"
    ;;
  internal-integrations)
    run_packages "$(printf '%s\n' github.com/usehivy/hivy/internal/integrations github.com/usehivy/hivy/internal/nango | shard_lines)"
    ;;
  internal-storage)
    run_packages "$(printf '%s\n' github.com/usehivy/hivy/internal/storage | shard_lines)"
    ;;
  e2e)
    run_test_names ./e2e
    ;;
  e2e-fakebridge)
    run_packages "github.com/usehivy/hivy/e2e/fakebridge"
    ;;
  cmd)
    env "${test_env[@]}" go test ./cmd/... -count=1
    ;;
  *)
    echo "unknown suite: $suite" >&2
    exit 2
    ;;
esac
