#!/usr/bin/env bash
# Create the dataset that all 40 batches target. Idempotent; safe to re-run.
set -euo pipefail
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
ADDR="${RAG_ENGINE_ADDR:-127.0.0.1:50651}"
SECRET="$(grep '^RAG_ENGINE_SHARED_SECRET=' "$REPO_ROOT/.env.rag" | cut -d= -f2-)"
DIM="${LLM_EMBEDDING_DIM:-$(grep '^LLM_EMBEDDING_DIM=' "$REPO_ROOT/.env.rag" | cut -d= -f2-)}"
DIM="${DIM:-3072}"

echo "CreateDataset hiveloop_demo dim=$DIM on $ADDR"
grpcurl -plaintext \
    -proto "$REPO_ROOT/proto/rag_engine.proto" \
    -import-path "$REPO_ROOT/proto" \
    -H "authorization: Bearer $SECRET" \
    -d "{\"dataset_name\":\"hiveloop_demo\",\"vector_dim\":$DIM,\"embedding_precision\":\"float32\",\"idempotency_key\":\"create-hiveloop-demo-v1\"}" \
    "$ADDR" hiveloop.rag.v1.RagEngine/CreateDataset
