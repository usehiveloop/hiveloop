#!/usr/bin/env bash
# Ingest all 40 JSON batches into the rag-engine via grpcurl.
# Prints detailed per-batch progress + running totals + any doc-level failures.
#
# Usage:
#   ./rag-test-data/ingest_all.sh
#   RAG_ENGINE_ADDR=127.0.0.1:50651 ./rag-test-data/ingest_all.sh

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DOCS_DIR="$REPO_ROOT/rag-test-data/docs"
PROTO_ROOT="$REPO_ROOT/proto"
ADDR="${RAG_ENGINE_ADDR:-127.0.0.1:50651}"
OUT_LOG="$REPO_ROOT/rag-test-data/ingest_results.log"

SECRET="$(grep '^RAG_ENGINE_SHARED_SECRET=' "$REPO_ROOT/.env.rag" | cut -d= -f2-)"
[[ -n "$SECRET" ]] || { echo "missing RAG_ENGINE_SHARED_SECRET in .env.rag" >&2; exit 1; }
command -v grpcurl >/dev/null || { echo "grpcurl not installed — 'brew install grpcurl'" >&2; exit 1; }

files=("$DOCS_DIR"/docs_*.json)
total_files=${#files[@]}
[[ $total_files -gt 0 ]] || { echo "no files in $DOCS_DIR" >&2; exit 1; }

if [[ -z "${NO_COLOR:-}" ]] && [[ -t 1 ]]; then
    C_RESET=$'\033[0m'; C_DIM=$'\033[2m'; C_BOLD=$'\033[1m'
    C_CYAN=$'\033[36m'; C_GREEN=$'\033[32m'; C_YELLOW=$'\033[33m'; C_RED=$'\033[31m'; C_GRAY=$'\033[90m'
else
    C_RESET=''; C_DIM=''; C_BOLD=''; C_CYAN=''; C_GREEN=''; C_YELLOW=''; C_RED=''; C_GRAY=''
fi

echo "${C_BOLD}rag-engine ingest all${C_RESET}"
echo "  server:   $ADDR"
echo "  batches:  $total_files files"
echo "  source:   $DOCS_DIR"
echo "  log:      $OUT_LOG"
echo ""
: > "$OUT_LOG"

TMP_RESP=$(mktemp -t rag-ingest-resp.XXXXXX)
trap 'rm -f "$TMP_RESP"' EXIT

run_total_rows=0
run_total_succ=0
run_total_fail=0
run_total_skip=0
run_total_ms=0
run_total_embed_ms=0
run_failed_batches=0
start_total=$(date +%s)

i=0
for f in "${files[@]}"; do
    i=$((i + 1))
    name="$(basename "$f")"
    size=$(wc -c < "$f" | tr -d ' ')
    size_kb=$((size / 1024))
    printf "${C_BOLD}[%2d/%d]${C_RESET} ${C_CYAN}%s${C_RESET} ${C_DIM}(%d KB)${C_RESET}\n" \
        "$i" "$total_files" "$name" "$size_kb"

    start=$(date +%s)
    set +e
    grpcurl -plaintext \
        -proto "$PROTO_ROOT/rag_engine.proto" \
        -import-path "$PROTO_ROOT" \
        -H "authorization: Bearer $SECRET" \
        -max-msg-sz 33554432 \
        -d @ \
        "$ADDR" hiveloop.rag.v1.RagEngine/IngestBatch < "$f" > "$TMP_RESP" 2>&1
    status=$?
    set -e
    wall=$(( $(date +%s) - start ))

    {
        echo "================================================================================"
        echo "file: $name  wall: ${wall}s  exit: $status  at: $(date -u +%FT%TZ)"
        echo "================================================================================"
        cat "$TMP_RESP"
    } >> "$OUT_LOG"

    if [[ $status -ne 0 ]] || ! grep -q '"totals"' "$TMP_RESP"; then
        run_failed_batches=$((run_failed_batches + 1))
        echo "  ${C_RED}✗ FAILED${C_RESET} (wall=${wall}s)"
        head -6 "$TMP_RESP" | sed 's/^/      /'
        echo ""
        continue
    fi

    # Parse full response via a standalone helper (bash 3.2 chokes on
    # heredocs inside $(...), so keep the parser in its own file).
    eval "$(python3 "$REPO_ROOT/rag-test-data/_parse_ingest_response.py" "$TMP_RESP")"

    if [[ "$BFAIL" -gt 0 ]]; then icon="${C_YELLOW}⚠${C_RESET}"; else icon="${C_GREEN}✓${C_RESET}"; fi

    run_total_rows=$((run_total_rows + BROWS))
    run_total_succ=$((run_total_succ + BSUCC))
    run_total_fail=$((run_total_fail + BFAIL))
    run_total_skip=$((run_total_skip + BSKIP))
    run_total_ms=$((run_total_ms + BBATCH_MS))
    run_total_embed_ms=$((run_total_embed_ms + BEMBED_MS))

    printf "  %s ${C_GREEN}succ=%d${C_RESET} fail=%d skip=%d rows=%d   ${C_DIM}phase: chunk=%dms embed=%dms write=%dms total=%dms${C_RESET}\n" \
        "$icon" "$BSUCC" "$BFAIL" "$BSKIP" "$BROWS" "$BCHUNK_MS" "$BEMBED_MS" "$BWRITE_MS" "$BBATCH_MS"

    if [[ "${FAIL_SAMPLE_COUNT:-0}" -gt 0 ]]; then
        for j in 0 1 2; do
            var="FAIL_SAMPLE_$j"
            [[ -n "${!var:-}" ]] || continue
            IFS='|' read -r docid code reason <<< "${!var}"
            printf "      ${C_RED}✗${C_RESET} %s  [%s]  %s\n" "$docid" "$code" "$reason"
        done
    fi

    if [[ $((i % 10)) -eq 0 ]]; then
        total_elapsed=$(( $(date +%s) - start_total ))
        printf "${C_GRAY}  ── running: %d chunks, %d succeeded, %d failed, %d skipped, %ds elapsed${C_RESET}\n\n" \
            "$run_total_rows" "$run_total_succ" "$run_total_fail" "$run_total_skip" "$total_elapsed"
    fi
done

total_elapsed=$(( $(date +%s) - start_total ))

echo ""
echo "${C_BOLD}════════════════════════════════════════════════════════════════════════════${C_RESET}"
echo "${C_BOLD}  FINAL${C_RESET}"
echo "${C_BOLD}════════════════════════════════════════════════════════════════════════════${C_RESET}"
printf "  files processed:       %d / %d\n" "$i" "$total_files"
printf "  files failed (batch):  %d\n" "$run_failed_batches"
printf "  chunks written:        %d\n" "$run_total_rows"
printf "  docs succeeded:        ${C_GREEN}%d${C_RESET}\n" "$run_total_succ"
printf "  docs failed:           ${C_RED}%d${C_RESET}\n" "$run_total_fail"
printf "  docs skipped:          ${C_YELLOW}%d${C_RESET}\n" "$run_total_skip"
printf "  total wall time:       %ds\n" "$total_elapsed"
printf "  total server-time:     %dms\n" "$run_total_ms"
printf "  total embedding-time:  %dms  ${C_DIM}← real API latency${C_RESET}\n" "$run_total_embed_ms"
echo ""
echo "  ${C_DIM}full per-batch responses in: $OUT_LOG${C_RESET}"

[[ "$run_total_fail" -eq 0 && "$run_failed_batches" -eq 0 ]]
