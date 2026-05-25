#!/usr/bin/env bash
set -euo pipefail

if ! curl -fsS http://localhost:6333/readyz >/dev/null 2>&1; then
  docker rm -f hivy-ci-qdrant >/dev/null 2>&1 || true
  docker run -d --name hivy-ci-qdrant \
    -p 6333:6333 -p 6334:6334 \
    qdrant/qdrant:v1.17.1 >/dev/null
fi

for _ in $(seq 1 90); do
  if curl -fsS http://localhost:6333/readyz >/dev/null 2>&1; then
    echo "ok: qdrant"
    exit 0
  fi
  sleep 2
done

docker logs --tail=200 hivy-ci-qdrant >&2 || true
echo "timeout waiting for qdrant" >&2
exit 1
