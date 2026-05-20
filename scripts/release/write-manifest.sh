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
    "sandboxBridge": "ghcr.io/usehivy/sandbox-bridge:${RELEASE_TAG}",
    "sandboxBridgeSemver": "ghcr.io/usehivy/sandbox-bridge:${RELEASE_VERSION}",
    "employeeSandbox": "ghcr.io/usehivy/employee-sandbox:${RELEASE_TAG}",
    "employeeSandboxSemver": "ghcr.io/usehivy/employee-sandbox:${RELEASE_VERSION}"
  },
  "bridgeAssets": {
    "linuxAmd64": "bridge-${RELEASE_TAG}-x86_64-unknown-linux-gnu.tar.gz",
    "linuxArm64": "bridge-${RELEASE_TAG}-aarch64-unknown-linux-gnu.tar.gz",
    "darwinAmd64": "bridge-${RELEASE_TAG}-x86_64-apple-darwin.tar.gz",
    "darwinArm64": "bridge-${RELEASE_TAG}-aarch64-apple-darwin.tar.gz"
  },
  "runtimeConfig": {
    "BRIDGE_BINARY_VERSION": "${RELEASE_TAG}",
    "BRIDGE_BASE_IMAGE_PREFIX": "hivy-bridge-${RELEASE_DASHED}-small-v1",
    "BRIDGE_BASE_DEDICATED_IMAGE_PREFIX": "hivy-bridge-${RELEASE_DASHED}-small-v1",
    "EMPLOYEE_SANDBOX_BASE_IMAGE_PREFIX": "hivy-employee-sandbox-${RELEASE_DASHED}-small-v1"
  },
  "snapshots": {
    "runtime": {
      "small": "hivy-bridge-${RELEASE_DASHED}-small-v1",
      "medium": "hivy-bridge-${RELEASE_DASHED}-medium-v1",
      "large": "hivy-bridge-${RELEASE_DASHED}-large-v1",
      "xlarge": "hivy-bridge-${RELEASE_DASHED}-xlarge-v1"
    },
    "employee": {
      "small": "hivy-employee-sandbox-${RELEASE_DASHED}-small-v1",
      "medium": "hivy-employee-sandbox-${RELEASE_DASHED}-medium-v1",
      "large": "hivy-employee-sandbox-${RELEASE_DASHED}-large-v1",
      "xlarge": "hivy-employee-sandbox-${RELEASE_DASHED}-xlarge-v1"
    }
  }
}
EOF
