#!/usr/bin/env bash
# check-log-budget — track total log call sites, fail CI if they grow past budget.
#
# Rationale: after the April 2026 logging cleanup we want to keep log noise
# from drifting back. Every PR that adds net-new log call sites must either
# stay under the budget, or bump the budget with explicit justification in
# the PR description.
#
# Counts emit-site call expressions across non-test Go code:
#   - slog.{Info,Warn,Error,Debug}
#   - logger.{Info,Warn,Error,Debug}
#   - log.{Print,Printf,Println,Fatal,Fatalf,Fatalln}
#   - fmt.{Print,Printf,Println}
#
# logging.Capture / sentry.CaptureException are NOT counted — those are the
# preferred home for non-critical errors and we want to encourage their use.
#
# Excluded: vendor, .ignored, *_test.go files, build-tagged spike binaries,
# the cmd/* CLI tools whose stdout is user-facing.

set -euo pipefail

# Adjust this when you intentionally add log volume. Keep PR descriptions
# honest about why. The cleanup landed at ~424 emit sites; 475 leaves
# headroom for legitimate growth without inviting drift.
BUDGET="${LOG_BUDGET:-475}"

EXCLUDE_DIRS=(
  -path './vendor' -o
  -path './.ignored' -o
  -path './.claude' -o
  -path './apps/web/node_modules' -o
  -path './internal/rag/vectorstore/spike' -o
  -path './cmd/sandbox-exec' -o
  -path './cmd/verify-devbox' -o
  -path './cmd/buildtemplates' -o
  -path './cmd/fake-nango' -o
  -path './cmd/fetchactions-graphql' -o
  -path './cmd/fetchactions-oas2' -o
  -path './cmd/fetchactions-oas3'
)

PATTERN='(\bslog\.(Info|Warn|Error|Debug)\(|\blogger\.(Info|Warn|Error|Debug)\(|\blog\.(Print|Printf|Println|Fatal|Fatalf|Fatalln)\(|\bfmt\.(Print|Printf|Println)\()'

count=$(
  find . \( "${EXCLUDE_DIRS[@]}" \) -prune -o \
    -name '*.go' -not -name '*_test.go' -type f -print0 |
  xargs -0 grep -E -h "$PATTERN" 2>/dev/null |
  wc -l |
  tr -d ' '
)

echo "Log emit sites: $count (budget: $BUDGET)"

if (( count > BUDGET )); then
  echo
  echo "::error::Log emit count $count exceeds budget $BUDGET."
  echo "Either:"
  echo "  1. Reduce log volume — most new logs should be slog.Debug or"
  echo "     should go to Sentry via logging.Capture(ctx, err)."
  echo "  2. Bump LOG_BUDGET in this script with a PR-description note"
  echo "     explaining the new floor."
  exit 1
fi

echo "✓ Log emit count under budget."
