#!/usr/bin/env bash
set -euo pipefail

echo "==> Tearing down all services and volumes..."
docker compose down -v --remove-orphans 2>/dev/null || true
rm -f docker/zitadel/bootstrap/admin.pat
rm -f docker/zitadel/bootstrap/api-credentials.json
rm -f docker/zitadel/bootstrap/dashboard-credentials.json

echo ""
echo "==> Starting infrastructure..."
docker compose up -d postgres redis zitadel zitadel-init

echo ""
echo "==> Waiting for services to be healthy..."

until docker compose exec -T postgres pg_isready -U llmvault -q 2>/dev/null; do sleep 1; done
echo "  ✓ Postgres"

until docker compose exec -T redis redis-cli ping 2>/dev/null | grep -q PONG; do sleep 1; done
echo "  ✓ Redis"

until curl -sf http://localhost:8085/debug/ready >/dev/null 2>&1; do sleep 2; done
echo "  ✓ ZITADEL"

echo ""
echo "==> Waiting for ZITADEL init to complete..."
timeout=120
while [ $timeout -gt 0 ]; do
    if docker compose ps -a zitadel-init 2>/dev/null | grep -q "Exited"; then
        break
    fi
    sleep 2
    timeout=$((timeout - 2))
done
if [ $timeout -le 0 ]; then
    echo "  ✗ Timed out waiting for zitadel-init"
    docker compose logs zitadel-init
    exit 1
fi
# Verify it exited successfully
if ! docker compose ps -a zitadel-init 2>/dev/null | grep -q "Exited (0)"; then
    echo "  ✗ zitadel-init failed"
    docker compose logs zitadel-init
    exit 1
fi
echo "  ✓ ZITADEL init done"

echo ""
echo "==> Extracting ZITADEL credentials from init logs..."

INIT_LOGS=$(docker compose logs zitadel-init 2>/dev/null)

extract() {
    echo "$INIT_LOGS" | grep "$1=" | tail -1 | sed "s/.*$1=//"
}

export ZITADEL_PROJECT_ID=$(extract ZITADEL_PROJECT_ID)
export ZITADEL_CLIENT_ID=$(extract ZITADEL_CLIENT_ID)
export ZITADEL_CLIENT_SECRET=$(extract ZITADEL_CLIENT_SECRET)
export ZITADEL_ADMIN_PAT=$(extract ZITADEL_ADMIN_PAT)

echo "  PROJECT_ID=$ZITADEL_PROJECT_ID"
echo "  CLIENT_ID=$ZITADEL_CLIENT_ID"
echo "  ADMIN_PAT=$ZITADEL_ADMIN_PAT"

if [ -z "$ZITADEL_PROJECT_ID" ] || [ -z "$ZITADEL_CLIENT_ID" ] || [ -z "$ZITADEL_CLIENT_SECRET" ] || [ -z "$ZITADEL_ADMIN_PAT" ]; then
    echo ""
    echo "  ✗ Failed to extract ZITADEL credentials. Full init logs:"
    echo "$INIT_LOGS"
    exit 1
fi

echo ""
echo "==> Running internal tests..."
go test ./internal/... -v -race -count=1

echo ""
echo "==> Running e2e tests..."
go test ./e2e/... -v -count=1 -timeout=5m

echo ""
echo "========================================"
echo "  All tests passed"
echo "========================================"
