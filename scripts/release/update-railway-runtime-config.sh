#!/usr/bin/env bash
set -euo pipefail

manifest="${1:?usage: update-railway-runtime-config.sh <release-manifest.json>}"
environment="${RAILWAY_ENVIRONMENT:-production}"
services="${RAILWAY_SERVICES:-api.usehivy.com asynq.usehivy.com admin.api.usehivy.com}"
wait_seconds="${RAILWAY_DEPLOY_WAIT_SECONDS:-900}"
poll_seconds="${RAILWAY_DEPLOY_POLL_SECONDS:-10}"

if [[ ! -f "${manifest}" ]]; then
  echo "manifest not found: ${manifest}" >&2
  exit 1
fi

command -v jq >/dev/null || {
  echo "jq is required" >&2
  exit 1
}
command -v railway >/dev/null || {
  echo "railway CLI is required" >&2
  exit 1
}

if [[ -n "${RAILWAY_PROJECT_ID:-}" ]]; then
  railway link --project "${RAILWAY_PROJECT_ID}" >/dev/null
fi

specialist_sandbox_runtime_version="$(jq -r '.runtimeConfig.HIVY_SPECIALIST_SANDBOX_RUNTIME_VERSION' "${manifest}")"
specialist_sandbox_base_image_prefix="$(jq -r '.runtimeConfig.HIVY_SPECIALIST_SANDBOX_BASE_IMAGE_PREFIX' "${manifest}")"
sandboxes_runtime_base_image_prefix="$(jq -r '.runtimeConfig.HIVY_SANDBOXES_RUNTIME_BASE_IMAGE_PREFIX' "${manifest}")"

for value in \
  "${specialist_sandbox_runtime_version}" \
  "${specialist_sandbox_base_image_prefix}" \
  "${sandboxes_runtime_base_image_prefix}"
do
  if [[ -z "${value}" || "${value}" == "null" ]]; then
    echo "release manifest is missing a runtimeConfig value" >&2
    exit 1
  fi
done

read -r -a service_list <<<"${services}"
if [[ "${#service_list[@]}" -eq 0 ]]; then
  echo "no Railway services configured" >&2
  exit 1
fi

for service in "${service_list[@]}"; do
  echo "Updating Railway runtime config on ${service}..."
  railway variable set \
    "HIVY_SPECIALIST_SANDBOX_RUNTIME_VERSION=${specialist_sandbox_runtime_version}" \
    "HIVY_SPECIALIST_SANDBOX_BASE_IMAGE_PREFIX=${specialist_sandbox_base_image_prefix}" \
    "HIVY_SANDBOXES_RUNTIME_BASE_IMAGE_PREFIX=${sandboxes_runtime_base_image_prefix}" \
    --environment "${environment}" \
    --service "${service}"
done

deadline=$((SECONDS + wait_seconds))
while true; do
  all_success=true
  for service in "${service_list[@]}"; do
    status="$(
      railway deployment list \
        --environment "${environment}" \
        --service "${service}" \
        --limit 1 \
        --json | jq -r '.[0].status'
    )"
    echo "${service}: ${status}"
    case "${status}" in
      SUCCESS)
        ;;
      FAILED | CRASHED | REMOVED)
        echo "Railway deployment failed for ${service}: ${status}" >&2
        exit 1
        ;;
      *)
        all_success=false
        ;;
    esac
  done

  if [[ "${all_success}" == "true" ]]; then
    echo "All Railway deployments are successful."
    exit 0
  fi

  if ((SECONDS >= deadline)); then
    echo "Timed out waiting for Railway deployments." >&2
    exit 1
  fi

  sleep "${poll_seconds}"
done
