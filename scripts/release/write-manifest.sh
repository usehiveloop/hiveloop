#!/usr/bin/env bash
set -euo pipefail

tag="${1:?usage: write-manifest.sh <tag> <output-path> [commit]}"
out="${2:?usage: write-manifest.sh <tag> <output-path> [commit]}"
commit="${3:-${GITHUB_SHA:-}}"
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

eval "$("${script_dir}/derive-version.sh" "${tag}" "${commit}")"

latest=false
if [[ "${RELEASE_IS_PRERELEASE}" != "true" ]]; then
  latest=true
fi

cat >"${out}" <<EOF
{
  "version": "${RELEASE_TAG}",
  "semver": "${RELEASE_VERSION}",
  "commit": "${RELEASE_COMMIT}",
  "prerelease": ${RELEASE_IS_PRERELEASE},
  "latest": ${latest},
  "images": {
    "api": "ghcr.io/usehivy/hivy:${RELEASE_TAG}",
    "apiSemver": "ghcr.io/usehivy/hivy:${RELEASE_VERSION}",
    "sandboxesRuntime": "ghcr.io/usehivy/hivy-sandboxes-runtime:${RELEASE_TAG}",
    "sandboxesRuntimeSemver": "ghcr.io/usehivy/hivy-sandboxes-runtime:${RELEASE_VERSION}",
    "sandboxesRuntimeSpecialist": "ghcr.io/usehivy/hivy-sandboxes-runtime-specialist:${RELEASE_TAG}",
    "sandboxesRuntimeSpecialistSemver": "ghcr.io/usehivy/hivy-sandboxes-runtime-specialist:${RELEASE_VERSION}"
  },
  "runtimeConfig": {
    "HIVY_SPECIALIST_SANDBOX_RUNTIME_VERSION": "${RELEASE_TAG}",
    "HIVY_RAILWAY_RUNTIME_IMAGE": "ghcr.io/usehivy/hivy-sandboxes-runtime:${RELEASE_TAG}",
    "HIVY_RAILWAY_SPECIALIST_RUNTIME_IMAGE": "ghcr.io/usehivy/hivy-sandboxes-runtime-specialist:${RELEASE_TAG}",
    "HIVY_SANDBOXES_RUNTIME_BASE_IMAGE_PREFIX": "hivy-sandboxes-runtime-${RELEASE_DASHED}-small-v1",
    "HIVY_SANDBOXES_RUNTIME_SPECIALIST_IMAGE_PREFIX": "hivy-sandboxes-runtime-specialist-${RELEASE_DASHED}-small-v1"
  },
  "snapshots": {
    "sandboxesRuntime": {
      "small": "hivy-sandboxes-runtime-${RELEASE_DASHED}-small-v1",
      "medium": "hivy-sandboxes-runtime-${RELEASE_DASHED}-medium-v1",
      "large": "hivy-sandboxes-runtime-${RELEASE_DASHED}-large-v1",
      "xlarge": "hivy-sandboxes-runtime-${RELEASE_DASHED}-xlarge-v1"
    },
    "sandboxesRuntimeSpecialist": {
      "small": "hivy-sandboxes-runtime-specialist-${RELEASE_DASHED}-small-v1",
      "medium": "hivy-sandboxes-runtime-specialist-${RELEASE_DASHED}-medium-v1",
      "large": "hivy-sandboxes-runtime-specialist-${RELEASE_DASHED}-large-v1",
      "xlarge": "hivy-sandboxes-runtime-specialist-${RELEASE_DASHED}-xlarge-v1"
    }
  }
}
EOF
