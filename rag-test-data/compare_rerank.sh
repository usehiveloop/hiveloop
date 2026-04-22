#!/usr/bin/env bash
# Run the same search twice — once without rerank, once with Qwen3-Reranker-8B
# via SiliconFlow — and print a side-by-side comparison of the top-5 results.
#
# Usage:
#   ./rag-test-data/compare_rerank.sh "kubernetes zero-downtime rolling deployment"
#   ./rag-test-data/compare_rerank.sh "billing dunning retry policy" 5 0.7
#      (args: query, limit, hybrid_alpha)

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PROTO_ROOT="$REPO_ROOT/proto"
ADDR="${RAG_ENGINE_ADDR:-127.0.0.1:50651}"

SECRET="$(grep '^RAG_ENGINE_SHARED_SECRET=' "$REPO_ROOT/.env.rag" | cut -d= -f2-)"
[[ -n "$SECRET" ]] || { echo "missing RAG_ENGINE_SHARED_SECRET in .env.rag" >&2; exit 1; }

QUERY="${1:?usage: compare_rerank.sh \"<query>\" [limit] [hybrid_alpha]}"
LIMIT="${2:-5}"
ALPHA="${3:-0.7}"
# Use org-acme and include PUBLIC; pretend we're an engineering user who
# belongs to team_backend — gives a realistic ACL footprint.
ORG="${ORG:-org-acme}"

if [[ -z "${NO_COLOR:-}" ]] && [[ -t 1 ]]; then
    C_RESET=$'\033[0m'; C_BOLD=$'\033[1m'; C_DIM=$'\033[2m'
    C_CYAN=$'\033[36m'; C_GREEN=$'\033[32m'; C_YELLOW=$'\033[33m'; C_MAGENTA=$'\033[35m'; C_GRAY=$'\033[90m'
else
    C_RESET=''; C_BOLD=''; C_DIM=''; C_CYAN=''; C_GREEN=''; C_YELLOW=''; C_MAGENTA=''; C_GRAY=''
fi

run_search () {
    local rerank="$1"; local out="$2"
    # Broad ACL footprint so we don't artificially suppress results
    local acls='["user_email:user01@example.com","user_email:user05@example.com","user_email:user42@example.com","external_group:team_backend","external_group:team_platform","external_group:team_sre"]'
    local req
    req=$(python3 -c "
import json,sys
q=sys.argv[1]; limit=int(sys.argv[2]); alpha=float(sys.argv[3])
print(json.dumps({
  'dataset_name':'hiveloop_demo',
  'org_id':'$ORG',
  'query_text':q,
  'mode':'SEARCH_MODE_HYBRID',
  'acl_any_of':$acls,
  'include_public':True,
  'limit':limit,
  'candidate_pool':50,
  'hybrid_alpha':alpha,
  'rerank':bool(int('$rerank')),
}))
" "$QUERY" "$LIMIT" "$ALPHA")
    grpcurl -plaintext \
        -proto "$PROTO_ROOT/rag_engine.proto" \
        -import-path "$PROTO_ROOT" \
        -H "authorization: Bearer $SECRET" \
        -d "$req" \
        "$ADDR" hiveloop.rag.v1.RagEngine/Search > "$out" 2>&1
}

print_hits () {
    local file="$1"; local label="$2"
    echo "${C_BOLD}${label}${C_RESET}"
    if ! grep -q '"hits"' "$file"; then
        echo "${C_YELLOW}  (no hits or error)${C_RESET}"
        head -6 "$file" | sed 's/^/  /'
        return
    fi
    python3 - "$file" <<'PY'
import json, sys
with open(sys.argv[1]) as f: d = json.load(f)
for i,h in enumerate(d.get("hits", [])):
    score = h.get("score", 0.0) or 0.0
    topic = h.get("metadata",{}).get("topic","?")
    source = h.get("metadata",{}).get("source","?")
    docid = h.get("docId","?")
    blurb = (h.get("blurb","") or "").replace("\n"," ").strip()[:90]
    print(f"  {i+1}. [score={score:.4f}] [topic={topic:<14}] [source={source:<12}] {docid}")
    print(f"       {blurb}")
PY
}

TMP_BASE=$(mktemp -t rag-cmp-base.XXXXXX.json)
TMP_RR=$(mktemp -t rag-cmp-rr.XXXXXX.json)
trap 'rm -f "$TMP_BASE" "$TMP_RR"' EXIT

echo "${C_BOLD}══════════════════════════════════════════════════════════════════════════════${C_RESET}"
echo "${C_BOLD}Query:${C_RESET} ${C_CYAN}$QUERY${C_RESET}"
echo "${C_DIM}  org=$ORG  limit=$LIMIT  candidate_pool=50  hybrid_alpha=$ALPHA${C_RESET}"
echo "${C_BOLD}══════════════════════════════════════════════════════════════════════════════${C_RESET}"
echo ""

t0=$(python3 -c 'import time;print(time.time())')
run_search 0 "$TMP_BASE"
t1=$(python3 -c 'import time;print(time.time())')
run_search 1 "$TMP_RR"
t2=$(python3 -c 'import time;print(time.time())')

lat_base=$(python3 -c "print(f'{($t1-$t0)*1000:.0f}')")
lat_rr=$(python3 -c "print(f'{($t2-$t1)*1000:.0f}')")
diff=$(python3 -c "print(f'{($t2-$t1-($t1-$t0))*1000:+.0f}')")

print_hits "$TMP_BASE" "${C_GREEN}── VECTOR + BM25 HYBRID (no rerank) · ${lat_base}ms${C_RESET}"
echo ""
print_hits "$TMP_RR"   "${C_MAGENTA}── HYBRID + Qwen3-Reranker-8B · ${lat_rr}ms (delta ${diff}ms)${C_RESET}"
echo ""
echo "${C_DIM}Rerank overhead: ${diff}ms (=real SiliconFlow API latency for reranking the top-50 candidates)${C_RESET}"

# Topic-coherence summary
echo ""
echo "${C_BOLD}── Topic concentration in top-5 ─${C_RESET}"
python3 - "$TMP_BASE" "$TMP_RR" <<'PY'
import json, sys, collections
base = json.load(open(sys.argv[1])).get("hits", [])[:5]
rr = json.load(open(sys.argv[2])).get("hits", [])[:5]
def summ(hits, label):
    topics = collections.Counter(h.get("metadata",{}).get("topic","?") for h in hits)
    sources = collections.Counter(h.get("metadata",{}).get("source","?") for h in hits)
    top = ", ".join(f"{t}×{c}" for t,c in topics.most_common(3))
    print(f"  {label:<10}  topics: {top}")
summ(base, "no-rerank")
summ(rr,   "rerank")
PY
