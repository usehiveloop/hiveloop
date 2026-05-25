#!/usr/bin/env bash
set -euo pipefail

docker run --rm --network host -e PGPASSWORD=localdev pgvector/pgvector:pg17 \
  psql -h 127.0.0.1 -p 5433 -U hivy -d hivy_test \
  -tc "SELECT 1 FROM pg_database WHERE datname = 'nango'" | grep -q 1 || \
docker run --rm --network host -e PGPASSWORD=localdev pgvector/pgvector:pg17 \
  psql -h 127.0.0.1 -p 5433 -U hivy -d hivy_test \
  -c "CREATE DATABASE nango OWNER hivy"
