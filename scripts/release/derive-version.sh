#!/usr/bin/env bash
set -euo pipefail

tag="${1:-${GITHUB_REF_NAME:-}}"
commit="${2:-${GITHUB_SHA:-}}"

if [[ -z "${tag}" ]]; then
  echo "error: release tag is required" >&2
  exit 1
fi

if [[ -z "${commit}" ]]; then
  if command -v git >/dev/null 2>&1; then
    commit="$(git rev-parse HEAD)"
  else
    echo "error: release commit is required" >&2
    exit 1
  fi
fi

if [[ ! "${tag}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z][0-9A-Za-z.-]*)?$ ]]; then
  echo "error: release tag must be vX.Y.Z or vX.Y.Z-prerelease, got: ${tag}" >&2
  exit 1
fi

version="${tag#v}"
dashed="${version//./-}"
is_prerelease=false
if [[ "${version}" == *-* ]]; then
  is_prerelease=true
fi

short_commit="${commit:0:7}"

cat <<EOF
RELEASE_TAG=${tag}
RELEASE_VERSION=${version}
RELEASE_DASHED=${dashed}
RELEASE_IS_PRERELEASE=${is_prerelease}
RELEASE_COMMIT=${commit}
RELEASE_SHORT_COMMIT=${short_commit}
API_IMAGE=ghcr.io/usehivy/hivy:${tag}
SANDBOX_BRIDGE_IMAGE=ghcr.io/usehivy/sandbox-bridge:${tag}
EMPLOYEE_SANDBOX_IMAGE=ghcr.io/usehivy/employee-sandbox:${tag}
BRIDGE_SNAPSHOT_SMALL=hivy-bridge-${dashed}-small-v1
EMPLOYEE_SANDBOX_SNAPSHOT_SMALL=hivy-employee-sandbox-${dashed}-small-v1
EOF
