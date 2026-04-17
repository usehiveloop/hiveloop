#!/usr/bin/env bash
# Runs the LSP smoke test inside the lsp-testbed container.
# Called from the host. Requires OPENROUTER_API_KEY in the host env.
#
# Usage:
#   ./docker_harness.sh build-bridge   # build the Linux bridge binary
#   ./docker_harness.sh run-all        # run all defined tests inside one container
#   ./docker_harness.sh run <LSP_ID> <REPO_URL> <FILE> <PORT>  # run a single test

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
IMAGE="lsp-testbed:latest"

if [[ -z "${OPENROUTER_API_KEY:-}" ]]; then
    if [[ -f "${REPO_ROOT}/.env" ]]; then
        set -a; source "${REPO_ROOT}/.env"; set +a
    fi
fi
if [[ -z "${OPENROUTER_API_KEY:-}" ]]; then
    echo "ERR: OPENROUTER_API_KEY not set (and not in .env)" >&2
    exit 2
fi

build_bridge() {
    echo "==> building bridge binary for linux/amd64 inside ${IMAGE}"
    docker run --rm --platform linux/amd64 \
        -v "${REPO_ROOT}:/work" \
        -v lsp-testbed-cargo-registry:/root/.cargo/registry \
        -v lsp-testbed-cargo-git:/root/.cargo/git \
        -v lsp-testbed-target:/work/target-linux \
        -e CARGO_TARGET_DIR=/work/target-linux \
        -w /work \
        "${IMAGE}" \
        cargo build --release -p bridge
    echo "==> bridge binary built at ${REPO_ROOT}/target-linux/release/bridge"
}

run_single() {
    local lsp_id="$1" repo_url="$2" hint_file="$3" port="$4" extra_lsps="${5:-}"
    docker run --rm --platform linux/amd64 \
        -v "${REPO_ROOT}:/work" \
        -v lsp-testbed-target:/target-linux \
        -e OPENROUTER_API_KEY="${OPENROUTER_API_KEY}" \
        -e BRIDGE_REPO=/work \
        -e BRIDGE_BIN=/target-linux/release/bridge \
        -e LSP_ID="${lsp_id}" \
        -e EXTRA_LSPS="${extra_lsps}" \
        -e REPO_URL="${repo_url}" \
        -e LANG_HINT_FILE="${hint_file}" \
        -e BRIDGE_PORT="${port}" \
        -e WORK_DIR="/tmp/lsp-test/workspaces/${lsp_id}" \
        "${IMAGE}" \
        /work/e2e/lsp-smoke/run_smoke.sh
}

# Matrix of tests: LSP_ID | REPO_URL | LANG_HINT_FILE | PORT
TESTS=(
    "typescript|https://github.com/sindresorhus/is.git|source/index.ts|9101"
    "go|https://github.com/spf13/cobra.git|command.go|9102"
    "python|https://github.com/pallets/click.git|src/click/core.py|9103"
    "bash|https://github.com/rupa/z.git|z.sh|9104"
    "yaml-ls|https://github.com/actions/checkout.git|action.yml|9105"
    "prisma|https://github.com/prisma/prisma-examples.git|databases/turso/prisma/schema.prisma|9106"
    "svelte|https://github.com/sveltejs/template.git|src/App.svelte|9107"
    "vue|https://github.com/antfu-collective/vitesse.git|src/App.vue|9108"
    "astro|https://github.com/ixartz/Astro-boilerplate.git|src/pages/index.astro|9109"
    "rust|https://github.com/sharkdp/fd.git|src/main.rs|9110"
    "clangd|https://github.com/antirez/smallchat.git|smallchat-server.c|9111"
    "php|https://github.com/laravel/laravel.git|routes/web.php|9112"
    "ruby-lsp|https://github.com/rack/rack.git|lib/rack.rb|9113"
    "terraform|https://github.com/terraform-aws-modules/terraform-aws-vpc.git|main.tf|9114"
    "tailwindcss|https://github.com/tailwindlabs/tailwindcss.git|packages/tailwindcss/src/index.ts|9115|typescript,eslint,biome,deno"
    "biome|https://github.com/biomejs/website.git|astro.config.ts|9116"
    "vimls|https://github.com/tpope/vim-fugitive.git|plugin/fugitive.vim|9117"
    "graphql|https://github.com/graphql/graphiql.git|packages/graphiql-toolkit/src/graphql-helpers/__tests__/__queries__/testQuery.graphql|9118"
    "cmake|https://github.com/Kitware/CMake.git|Modules/CMakeSystemSpecificInformation.cmake|9119"
)

run_all_parallel() {
    local run_id=$1; shift
    local filter="${1:-}"
    # Bridge binary lives in the docker volume `lsp-testbed-target`, exposed to each
    # test container at /target-linux/release/bridge. Verify via a throwaway container.
    if ! docker run --rm --platform linux/amd64 -v lsp-testbed-target:/t "${IMAGE}" \
            test -x /t/release/bridge 2>/dev/null; then
        echo "ERR: bridge binary missing in volume lsp-testbed-target — run: $0 build-bridge first" >&2
        exit 3
    fi
    local results_dir="/tmp/lsp-docker-results-${run_id}"
    rm -rf "${results_dir}"; mkdir -p "${results_dir}"
    local pids=()

    for entry in "${TESTS[@]}"; do
        IFS='|' read -r lsp repo file port extra <<<"${entry}"
        if [[ -n "${filter}" ]] && ! [[ ",${filter}," == *",${lsp},"* ]]; then
            continue
        fi
        log="${results_dir}/${lsp}.log"
        echo "▶ launching ${lsp} (port ${port})${extra:+ +extras=${extra}}"
        (
            run_single "${lsp}" "${repo}" "${file}" "${port}" "${extra:-}" >"${log}" 2>&1
            echo $? >"${results_dir}/${lsp}.exit"
        ) &
        pids+=($!)
    done

    echo "==> ${#pids[@]} tests running in parallel; waiting..."
    for pid in "${pids[@]}"; do
        wait "${pid}" || true
    done

    echo ""
    echo "=============== RESULTS (${run_id}) ==============="
    local pass=0 fail=0
    for entry in "${TESTS[@]}"; do
        IFS='|' read -r lsp _ _ _ <<<"${entry}"
        if [[ -n "${filter}" ]] && ! [[ ",${filter}," == *",${lsp},"* ]]; then
            continue
        fi
        exit_file="${results_dir}/${lsp}.exit"
        log="${results_dir}/${lsp}.log"
        if [[ -f "${exit_file}" ]] && [[ "$(cat "${exit_file}")" == "0" ]]; then
            printf "  ✅ %-15s\n" "${lsp}"
            pass=$((pass+1))
        else
            fail_line=$(grep -E '✗ FAIL|LSP_ERROR|npm install failed|installation failed' "${log}" 2>/dev/null | head -1)
            printf "  ❌ %-15s %s\n" "${lsp}" "${fail_line:-(check ${log})}"
            fail=$((fail+1))
        fi
    done
    echo "---"
    echo "PASS=${pass} FAIL=${fail}  (logs in ${results_dir})"
}

cmd="${1:-}"
case "${cmd}" in
    build-bridge) build_bridge ;;
    run-all) run_all_parallel "all" "" ;;
    run-filter)
        shift
        run_all_parallel "filter" "$1"
        ;;
    run)
        shift
        run_single "$@"
        ;;
    *)
        cat <<EOF
Usage:
  $0 build-bridge                        # build Linux bridge binary in container
  $0 run-all                             # run all 19 tests in parallel
  $0 run-filter <comma-sep-lsp-ids>      # run only listed LSPs (e.g. "vue,svelte,cmake")
  $0 run <LSP_ID> <REPO_URL> <FILE> <PORT>  # run a single test
EOF
        ;;
esac
