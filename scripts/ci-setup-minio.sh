#!/usr/bin/env bash
set -euo pipefail

if ! curl -fsS http://localhost:9000/minio/health/ready >/dev/null 2>&1; then
  docker rm -f hivy-ci-minio >/dev/null 2>&1 || true
  docker run -d --name hivy-ci-minio \
    -p 9000:9000 -p 9001:9001 \
    -e MINIO_ROOT_USER=minioadmin \
    -e MINIO_ROOT_PASSWORD=minioadmin \
    minio/minio:latest server /data --console-address ":9001" >/dev/null
fi

wait_for_minio() {
  for _ in $(seq 1 90); do
    if curl -fsS http://localhost:9000/minio/health/ready >/dev/null 2>&1; then
      echo "ok: minio"
      return 0
    fi
    sleep 2
  done
  echo "timeout waiting for minio" >&2
  return 1
}

wait_for_minio
docker run --rm --network host minio/mc:latest sh -lc '
  mc alias set local http://127.0.0.1:9000 minioadmin minioadmin &&
  mc mb --ignore-existing local/hivy-rag-test &&
  mc mb --ignore-existing local/public-files-test &&
  mc anonymous set download local/public-files-test
'
