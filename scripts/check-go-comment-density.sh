#!/usr/bin/env bash
# check-go-comment-density — cap comments at 10% of added Go lines in a PR.
#
# Goal: keep comments rare and high-signal. This check exists primarily to
# push AI coding agents to stop narrating obvious code ("// increment counter",
# "// return the result", "// added for X flow"). Those comments rot, distract
# reviewers, and inflate diffs with no payoff. The ceiling forces agents to
# trim their own output rather than leaving the cleanup to humans.
#
# Scope: only lines added by this PR (origin/main..HEAD), only *.go files.
# Excludes generated/vendored code, matching the file-length checker.
#
# Tunables:
#   MAX_GO_COMMENT_PCT   integer percent ceiling (default 10)
#   BASE_REF             diff base (default origin/main)
#
# Portable bash 3.2+ (macOS default).

set -euo pipefail

MAX_PCT="${MAX_GO_COMMENT_PCT:-10}"
BASE_REF="${BASE_REF:-origin/main}"

EXCLUDE_REGEX='(^vendor/|^\.ignored/|\.pb\.go$|_gen\.go$|^docs/docs\.go$)'

files=$(git diff --name-only --diff-filter=AM "$BASE_REF"...HEAD -- '*.go' \
  | { grep -Ev "$EXCLUDE_REGEX" || true; })

if [[ -z "$files" ]]; then
  echo "✓ No Go files changed in this PR — comment density check skipped."
  exit 0
fi

comment=0
code=0

while IFS= read -r file; do
  [[ -z "$file" ]] && continue
  # -U0 drops context lines so we only see actual adds.
  added=$(git diff -U0 "$BASE_REF"...HEAD -- "$file" \
    | { grep -E '^\+' || true; } \
    | { grep -Ev '^\+\+\+' || true; })
  [[ -z "$added" ]] && continue

  while IFS= read -r line; do
    body="${line#+}"
    trimmed="${body#"${body%%[![:space:]]*}"}"
    [[ -z "$trimmed" ]] && continue
    # Classify by leading characters of the trimmed line. Inline trailing
    # comments (`x := 1 // note`) count as code — keeping the rule simple,
    # and the user-visible noise we care about is full comment lines.
    case "$trimmed" in
      //*|/\**|\**)
        comment=$((comment + 1))
        ;;
      *)
        code=$((code + 1))
        ;;
    esac
  done <<< "$added"
done <<< "$files"

total=$((comment + code))
if (( total == 0 )); then
  echo "✓ No added Go lines in this PR — comment density check skipped."
  exit 0
fi

pct=$(( comment * 100 / total ))

echo "Added Go lines in PR: $total (code: $code, comments: $comment)"
echo "Comment density: ${pct}% (ceiling ${MAX_PCT}%)"

if (( pct > MAX_PCT )); then
  {
    echo "::error::Comment density ${pct}% exceeds the ${MAX_PCT}% ceiling for this PR."
    echo
    echo "Why this check exists:"
    echo "  The goal is to remove useless comments as much as possible. This"
    echo "  rule is aimed primarily at AI coding agents, so they can correct"
    echo "  their own mistakes and trim down commenting before review."
    echo
    echo "What to delete:"
    echo "  * comments that restate what a well-named identifier already says"
    echo "  * step-by-step narration of obvious control flow"
    echo "  * \"added for X\" / \"used by Y\" notes (those belong in the PR description)"
    echo "  * placeholder TODOs, \"// removed ...\" markers, decorative banners"
    echo
    echo "What to keep:"
    echo "  Only comments that explain a non-obvious WHY — hidden constraints,"
    echo "  subtle invariants, workarounds for specific bugs, or behavior that"
    echo "  would surprise a future reader. Real godoc on exported declarations"
    echo "  is fine; the ceiling normally leaves plenty of room for it."
    echo
    echo "Re-run locally: scripts/check-go-comment-density.sh"
  } >&2
  exit 1
fi

echo "✓ Comment density within ${MAX_PCT}% ceiling."
