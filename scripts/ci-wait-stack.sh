#!/usr/bin/env bash
set -euo pipefail

wait_for() {
  local name="$1"
  local command="$2"
  local attempts="${3:-120}"
  local delay="${4:-2}"

  for _ in $(seq 1 "$attempts"); do
    if eval "$command" >/dev/null 2>&1; then
      echo "✓ $name"
      return 0
    fi
    sleep "$delay"
  done

  echo "timeout waiting for $name" >&2
  docker compose ps >&2 || true
  docker compose logs --tail=200 >&2 || true
  return 1
}

wait_for "Postgres" "docker compose exec -T postgres pg_isready -U hivy -q"
wait_for "Redis" "docker compose exec -T redis redis-cli ping | grep -q PONG"
wait_for "Nango" "curl -fsS http://localhost:23003/health"
wait_for "MinIO" "curl -fsS http://localhost:9000/minio/health/ready"
wait_for "Qdrant" "curl -fsS http://localhost:6333/readyz"
wait_for "Hindsight" "curl -fsS http://localhost:8888/health || curl -fsS http://localhost:8888"
wait_for "API" "curl -fsS http://localhost:8080/healthz"
wait_for "Worker" "curl -fsS http://localhost:8090/healthz"
wait_for "Web" "curl -fsS http://localhost:30112/api/health || curl -fsS http://localhost:30112"

docker compose ps
