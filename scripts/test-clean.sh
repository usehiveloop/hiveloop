#!/usr/bin/env bash
set -euo pipefail

TARGET="${1:-all}"

# Source .env if available (provides credentials)
if [ -f .env ]; then
    set -a
    source .env
    set +a
fi

echo "==> Tearing down all services and volumes..."
docker compose down -v --remove-orphans 2>/dev/null || true

echo ""
echo "==> Starting infrastructure..."
docker compose up -d postgres redis vault

echo ""
echo "==> Waiting for services to be healthy..."

until docker compose exec -T postgres pg_isready -U llmvault -q 2>/dev/null; do sleep 1; done
echo "  ✓ Postgres"

until docker compose exec -T redis redis-cli ping 2>/dev/null | grep -q PONG; do sleep 1; done
echo "  ✓ Redis"

until docker compose exec -T vault vault status 2>/dev/null | grep -q "Version"; do sleep 1; done
echo "  ✓ Vault running"

# Wait for Vault init script to complete (transit key must exist)
echo "  Waiting for Vault Transit key..."
until docker compose exec -T vault vault read transit/keys/llmvault-key 2>/dev/null | grep -q "type"; do sleep 2; done
echo "  ✓ Vault Transit key ready"

echo ""
echo "==> Verifying env vars..."

# Verify required env vars (from .env or environment)
: "${NANGO_ENDPOINT:?NANGO_ENDPOINT must be set}"
: "${NANGO_SECRET_KEY:?NANGO_SECRET_KEY must be set}"
: "${OPENROUTER_API_KEY:?OPENROUTER_API_KEY must be set}"
: "${FIREWORKS_API_KEY:?FIREWORKS_API_KEY must be set}"

echo "  ✓ All required env vars present"

run_tests() {
    local description="$1"
    shift
    echo ""
    echo "==> $description"
    "$@"
}

case "$TARGET" in
    auth)
        run_tests "Running auth middleware tests..." \
            go test ./internal/middleware/... -v -race -count=1 -run "Auth|MultiAuth_JWTPath"
        run_tests "Running auth e2e org tests..." \
            go test ./e2e/... -v -count=1 -timeout=5m -run "TestOrg"
        ;;
    nango)
        run_tests "Running Nango integration CRUD tests..." \
            go test ./e2e/... -v -count=1 -timeout=5m -run "TestE2E_Integration"
        ;;
    proxy)
        run_tests "Running proxy tests..." \
            go test ./e2e/... -v -count=1 -timeout=5m -run "TestE2E_Proxy|TestE2E_Fireworks"
        ;;
    connect)
        run_tests "Running Connect widget tests..." \
            go test ./e2e/... -v -count=1 -timeout=5m -run "TestE2E_Connect|TestE2E_Widget"
        ;;
    vault)
        run_tests "Running Vault KMS tests..." \
            go test ./e2e/... -v -count=1 -timeout=5m -run "TestVaultE2E"
        ;;
    integrations)
        run_tests "Running Nango integration CRUD tests..." \
            go test ./e2e/... -v -count=1 -timeout=5m -run "TestE2E_Integration"
        run_tests "Running Connect widget tests..." \
            go test ./e2e/... -v -count=1 -timeout=5m -run "TestE2E_Connect"
        run_tests "Running proxy tests..." \
            go test ./e2e/... -v -count=1 -timeout=5m -run "TestE2E_Proxy|TestE2E_Fireworks"
        run_tests "Running Vault KMS tests..." \
            go test ./e2e/... -v -count=1 -timeout=5m -run "TestVaultE2E"
        ;;
    all)
        run_tests "Running internal tests..." \
            go test ./internal/... -v -race -count=1
        run_tests "Running e2e tests..." \
            go test ./e2e/... -v -count=1 -timeout=5m
        ;;
    *)
        echo "Unknown target: $TARGET"
        echo "Usage: $0 [all|auth|nango|proxy|connect|vault|integrations]"
        exit 1
        ;;
esac

echo ""
echo "========================================"
echo "  All tests passed ($TARGET)"
echo "========================================"
