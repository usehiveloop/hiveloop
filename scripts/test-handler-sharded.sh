#!/usr/bin/env bash
set -euo pipefail

pkg="${HANDLER_TEST_PACKAGE:-./internal/handler}"
shards="${HANDLER_TEST_SHARDS:-8}"
timeout="${HANDLER_TEST_TIMEOUT:-5m}"
go_bin="${GO_BIN:-go}"

if ! [[ "$shards" =~ ^[1-9][0-9]*$ ]]; then
  echo "HANDLER_TEST_SHARDS must be a positive integer" >&2
  exit 1
fi

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

tests_file="$tmp_dir/tests"
"$go_bin" test "$pkg" -list '^Test' 2>/dev/null |
  sed -n 's/^\(Test[A-Za-z0-9_]*\)$/\1/p' |
  sort > "$tests_file"

test_count="$(wc -l < "$tests_file" | tr -d ' ')"
if [ "$test_count" = "0" ]; then
  echo "No handler tests found for $pkg" >&2
  exit 1
fi

echo "Running $test_count handler tests across $shards shards"

pids=()
logs=()
for shard in $(seq 0 $((shards - 1))); do
  shard_file="$tmp_dir/shard-$shard.tests"
  awk -v shard="$shard" -v shards="$shards" '(NR - 1) % shards == shard { print }' "$tests_file" > "$shard_file"
  if [ ! -s "$shard_file" ]; then
    continue
  fi

  regex="^($(paste -sd '|' "$shard_file"))$"
  log_file="$tmp_dir/shard-$shard.log"
  logs+=("$log_file")
  (
    echo "==> shard $((shard + 1))/$shards ($(wc -l < "$shard_file" | tr -d ' ') tests)"
    "$go_bin" test "$pkg" -count=1 -timeout="$timeout" -run "$regex"
  ) > "$log_file" 2>&1 &
  pids+=("$!")
done

failed=0
for i in "${!pids[@]}"; do
  if ! wait "${pids[$i]}"; then
    failed=1
  fi
done

for log in "${logs[@]}"; do
  cat "$log"
done

exit "$failed"
