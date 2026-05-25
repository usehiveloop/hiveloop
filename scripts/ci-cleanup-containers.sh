#!/usr/bin/env bash
set -euo pipefail

docker rm -f hivy-ci-hindsight hivy-ci-nango hivy-ci-minio hivy-ci-qdrant >/dev/null 2>&1 || true
