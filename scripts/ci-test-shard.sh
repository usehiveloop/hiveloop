#!/usr/bin/env bash
set -euo pipefail

suite="${1:?suite is required: internal, e2e, or cmd}"
shard_index="${SHARD_INDEX:-${CI_NODE_INDEX:-0}}"
shard_total="${SHARD_TOTAL:-${CI_NODE_TOTAL:-1}}"
race="${RACE:-0}"

test_env=(
  "DATABASE_URL=postgres://hivy:localdev@localhost:5433/hivy_test?sslmode=disable"
  "HIVY_DATABASE_URL=postgres://hivy:localdev@localhost:5433/hivy_test?sslmode=disable"
  "TEST_DATABASE_URL=postgres://hivy:localdev@localhost:5433/hivy_test?sslmode=disable"
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

race_flag=()
if [[ "$race" == "1" ]]; then
  race_flag=(-race)
fi

case "$suite" in
  internal)
    packages="$(go list ./internal/... | awk -v s="$shard_index" -v n="$shard_total" 'NR % n == s')"
    if [[ -z "$packages" ]]; then
      echo "No internal packages assigned to shard $shard_index/$shard_total"
      exit 0
    fi
    echo "$packages"
    printf '%s\n' "$packages" | xargs env "${test_env[@]}" go test "${race_flag[@]}" -count=1
    ;;
  e2e)
    tests="$(go test ./e2e -list '^Test' | awk '/^Test/ {print}' | awk -v s="$shard_index" -v n="$shard_total" 'NR % n == s')"
    env "${test_env[@]}" go test ./e2e/fakebridge "${race_flag[@]}" -count=1 -timeout=5m
    if [[ -z "$tests" ]]; then
      echo "No e2e tests assigned to shard $shard_index/$shard_total"
      exit 0
    fi
    pattern="$(printf '%s\n' "$tests" | paste -sd'|' -)"
    echo "$tests"
    env "${test_env[@]}" go test ./e2e "${race_flag[@]}" -count=1 -timeout=5m -run "^(${pattern})$"
    ;;
  cmd)
    env "${test_env[@]}" go test ./cmd/... -count=1
    ;;
  *)
    echo "unknown suite: $suite" >&2
    exit 2
    ;;
esac
