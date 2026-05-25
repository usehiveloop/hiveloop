#!/usr/bin/env bash
set -euo pipefail

docker rm -f hivy-ci-hindsight >/dev/null 2>&1 || true
docker run -d --name hivy-ci-hindsight --network host \
  -e HINDSIGHT_API_DATABASE_URL=postgresql://hivy:localdev@127.0.0.1:5433/hivy_test \
  -e HINDSIGHT_API_DATABASE_SCHEMA=hindsight \
  -e HINDSIGHT_API_RUN_MIGRATIONS_ON_STARTUP=true \
  -e HINDSIGHT_API_HOST=0.0.0.0 \
  -e HINDSIGHT_API_PORT=8888 \
  -e HINDSIGHT_API_WORKERS=1 \
  -e HINDSIGHT_API_WORKER_ENABLED=false \
  -e HINDSIGHT_API_LOG_LEVEL=info \
  -e HINDSIGHT_API_LOG_FORMAT=json \
  -e HINDSIGHT_API_LLM_PROVIDER=none \
  -e HINDSIGHT_API_LLM_BASE_URL=http://localhost \
  -e HINDSIGHT_API_LLM_API_KEY=dummy \
  -e HINDSIGHT_API_LLM_MODEL=openai/gpt-oss-20b \
  -e HINDSIGHT_API_CONSOLIDATION_LLM_PROVIDER=none \
  -e HINDSIGHT_API_CONSOLIDATION_LLM_API_KEY=dummy \
  -e HINDSIGHT_API_CONSOLIDATION_LLM_MODEL=openai/gpt-oss-20b \
  -e HINDSIGHT_API_REFLECT_LLM_PROVIDER=none \
  -e HINDSIGHT_API_REFLECT_LLM_API_KEY=dummy \
  -e HINDSIGHT_API_REFLECT_LLM_MODEL=openai/gpt-oss-20b \
  -e HINDSIGHT_API_RETAIN_LLM_PROVIDER=none \
  -e HINDSIGHT_API_RETAIN_LLM_API_KEY=dummy \
  -e HINDSIGHT_API_RETAIN_LLM_MODEL=openai/gpt-oss-20b \
  -e HINDSIGHT_API_EMBEDDINGS_PROVIDER=local \
  -e HINDSIGHT_API_EMBEDDINGS_LOCAL_FORCE_CPU=true \
  -e HINDSIGHT_API_RERANKER_PROVIDER=rrf \
  -e HINDSIGHT_API_RERANKER_ZEROENTROPY_API_KEY=dummy \
  -e HINDSIGHT_API_RERANKER_ZEROENTROPY_MODEL=zerank-2 \
  ghcr.io/vectorize-io/hindsight:latest >/dev/null

for _ in $(seq 1 90); do
  if curl -fsS http://localhost:8888/health >/dev/null 2>&1 || curl -fsS http://localhost:8888 >/dev/null 2>&1; then
    echo "ok: hindsight"
    exit 0
  fi
  sleep 2
done

docker logs --tail=200 hivy-ci-hindsight >&2 || true
echo "timeout waiting for hindsight" >&2
exit 1
