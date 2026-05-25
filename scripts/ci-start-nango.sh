#!/usr/bin/env bash
set -euo pipefail

./scripts/ci-create-nango-db.sh
docker rm -f hivy-ci-nango >/dev/null 2>&1 || true
docker run -d --name hivy-ci-nango --network host --platform linux/amd64 \
  -e PORT=23003 \
  -e SERVER_PORT=23003 \
  -e NANGO_DB_HOST=127.0.0.1 \
  -e NANGO_DB_PORT=5433 \
  -e NANGO_DB_USER=hivy \
  -e NANGO_DB_PASSWORD=localdev \
  -e NANGO_DB_NAME=nango \
  -e NANGO_DB_SSL=false \
  -e NANGO_ENCRYPTION_KEY=AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA= \
  -e NANGO_SERVER_URL=http://localhost:23003 \
  -e NANGO_PUBLIC_SERVER_URL=http://localhost:23003 \
  -e NANGO_SECRET_KEY=00000000-0000-4000-8000-000000000001 \
  -e NANGO_DASHBOARD_USERNAME=local \
  -e NANGO_DASHBOARD_PASSWORD=local \
  -e FLAG_AUTH_ENABLED=true \
  -e FLAG_SERVE_CONNECT_UI=false \
  -e NANGO_LOGS_ENABLED=false \
  -e TELEMETRY=false \
  ghcr.io/usehivy/integrations:latest >/dev/null

for _ in $(seq 1 90); do
  if curl -fsS http://localhost:23003/health >/dev/null 2>&1; then
    echo "ok: nango"
    exit 0
  fi
  sleep 2
done

docker logs --tail=200 hivy-ci-nango >&2 || true
echo "timeout waiting for nango" >&2
exit 1
