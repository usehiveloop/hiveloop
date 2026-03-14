#!/bin/sh
set -e

# ---------------------------------------------------------------------------
# Logto bootstrap script (idempotent)
#
# Phase 1: Insert a bootstrap M2M application into Logto's database so we
#           can authenticate to the Management API.
# Phase 2: Use the Management API to create:
#           - API resource (https://api.llmvault.dev) with scopes
#           - Organization template roles (admin, viewer)
#           - Traditional web application (for dashboard OIDC)
# ---------------------------------------------------------------------------

: "${LOGTO_DB_URL:?LOGTO_DB_URL is required}"
: "${LOGTO_ENDPOINT:?LOGTO_ENDPOINT is required}"
: "${DASHBOARD_REDIRECT_URI:?DASHBOARD_REDIRECT_URI is required}"
: "${DASHBOARD_LOGOUT_URI:?DASHBOARD_LOGOUT_URI is required}"

# Fixed bootstrap M2M credentials (deterministic for dev/CI)
# Note: Logto IDs are varchar(21) max
BOOTSTRAP_APP_ID="lv-bootstrap-m2m"
BOOTSTRAP_APP_SECRET="llmvault-bootstrap-secret-for-dev"

apk add --no-cache curl jq postgresql16-client > /dev/null 2>&1

# ---------------------------------------------------------------------------
# Wait for Logto
# ---------------------------------------------------------------------------

echo "Waiting for Logto to be ready..."
RETRIES=0
until curl -sf "${LOGTO_ENDPOINT}/oidc/.well-known/openid-configuration" > /dev/null 2>&1; do
    RETRIES=$((RETRIES + 1))
    if [ "$RETRIES" -gt 90 ]; then
        echo "Timed out waiting for Logto"
        exit 1
    fi
    sleep 2
done
echo "Logto is ready."

# ---------------------------------------------------------------------------
# Phase 1: Bootstrap M2M app via direct DB insertion
# ---------------------------------------------------------------------------

echo "Bootstrapping M2M application..."

# Check if the bootstrap app already exists
EXISTING=$(psql "$LOGTO_DB_URL" -tAc "SELECT id FROM applications WHERE id = '$BOOTSTRAP_APP_ID' AND tenant_id = 'default'" 2>/dev/null || true)

if [ -n "$EXISTING" ]; then
    echo "Bootstrap M2M app already exists."
else
    echo "Inserting bootstrap M2M app into database..."

    # Find the Management API resource ID
    MGMT_RESOURCE_ID=$(psql "$LOGTO_DB_URL" -tAc "SELECT id FROM resources WHERE indicator LIKE '%/api' AND tenant_id = 'default' LIMIT 1")
    if [ -z "$MGMT_RESOURCE_ID" ]; then
        echo "ERROR: Could not find Management API resource in database"
        exit 1
    fi
    echo "  Management API resource ID: $MGMT_RESOURCE_ID"

    # Find the Management API "all" scope
    MGMT_SCOPE_ID=$(psql "$LOGTO_DB_URL" -tAc "SELECT id FROM scopes WHERE resource_id = '$MGMT_RESOURCE_ID' AND name = 'all' AND tenant_id = 'default' LIMIT 1")
    if [ -z "$MGMT_SCOPE_ID" ]; then
        echo "ERROR: Could not find 'all' scope for Management API"
        exit 1
    fi
    echo "  Management API 'all' scope ID: $MGMT_SCOPE_ID"

    # Insert the M2M application
    psql "$LOGTO_DB_URL" <<SQL
INSERT INTO applications (tenant_id, id, name, secret, description, type, oidc_client_metadata, custom_client_metadata, is_third_party, created_at)
VALUES (
    'default',
    '$BOOTSTRAP_APP_ID',
    'LLMVault Bootstrap',
    '$BOOTSTRAP_APP_SECRET',
    'Bootstrap M2M app for automated setup',
    'MachineToMachine',
    '{"redirectUris":[],"postLogoutRedirectUris":[]}',
    '{}',
    false,
    NOW()
)
ON CONFLICT (id) DO NOTHING;
SQL

    # Create a role for this app with Management API access
    ROLE_ID="lv-mgmt-role"
    psql "$LOGTO_DB_URL" <<SQL
INSERT INTO roles (tenant_id, id, name, description, type)
VALUES ('default', '$ROLE_ID', 'LLMVault Management Access', 'Full management API access for LLMVault bootstrap', 'MachineToMachine')
ON CONFLICT (id) DO NOTHING;
SQL

    # Link the role to the Management API scope
    psql "$LOGTO_DB_URL" <<SQL
INSERT INTO roles_scopes (tenant_id, id, role_id, scope_id)
VALUES ('default', 'lv-role-scope', '$ROLE_ID', '$MGMT_SCOPE_ID')
ON CONFLICT (id) DO NOTHING;
SQL

    # Assign the role to the application
    psql "$LOGTO_DB_URL" <<SQL
INSERT INTO applications_roles (tenant_id, id, application_id, role_id)
VALUES ('default', 'lv-app-role', '$BOOTSTRAP_APP_ID', '$ROLE_ID')
ON CONFLICT (id) DO NOTHING;
SQL

    echo "Bootstrap M2M app inserted."
fi

# ---------------------------------------------------------------------------
# Phase 2: Management API setup
# ---------------------------------------------------------------------------

echo ""
echo "Obtaining Management API token..."

# Get the Management API resource indicator
MGMT_INDICATOR=$(psql "$LOGTO_DB_URL" -tAc "SELECT indicator FROM resources WHERE indicator LIKE '%/api' AND tenant_id = 'default' LIMIT 1")
echo "  Management API indicator: $MGMT_INDICATOR"

TOKEN_RESPONSE=$(curl -sf -X POST "${LOGTO_ENDPOINT}/oidc/token" \
    -H "Content-Type: application/x-www-form-urlencoded" \
    -d "grant_type=client_credentials&client_id=${BOOTSTRAP_APP_ID}&client_secret=${BOOTSTRAP_APP_SECRET}&resource=${MGMT_INDICATOR}&scope=all")

ACCESS_TOKEN=$(echo "$TOKEN_RESPONSE" | jq -r '.access_token // empty')
if [ -z "$ACCESS_TOKEN" ]; then
    echo "ERROR: Failed to obtain Management API token"
    echo "Response: $TOKEN_RESPONSE"
    exit 1
fi
echo "Management API token obtained."

# Helper functions
logto_get() {
    curl -sf "${LOGTO_ENDPOINT}$1" \
        -H "Authorization: Bearer $ACCESS_TOKEN" \
        -H "Content-Type: application/json"
}

logto_post() {
    curl -sf -X POST "${LOGTO_ENDPOINT}$1" \
        -H "Authorization: Bearer $ACCESS_TOKEN" \
        -H "Content-Type: application/json" \
        -d "$2"
}

# ---------------------------------------------------------------------------
# API Resource: https://api.llmvault.dev
# ---------------------------------------------------------------------------

echo ""
echo "Setting up API resource..."
EXISTING_RESOURCE=$(logto_get "/api/resources" | jq -r '.[] | select(.indicator == "https://api.llmvault.dev") | .id // empty')

if [ -n "$EXISTING_RESOURCE" ]; then
    API_RESOURCE_ID="$EXISTING_RESOURCE"
    echo "  API resource already exists: $API_RESOURCE_ID"
else
    RESOURCE_RESPONSE=$(logto_post "/api/resources" \
        '{"name":"LLMVault API","indicator":"https://api.llmvault.dev","accessTokenTtl":3600}')
    API_RESOURCE_ID=$(echo "$RESOURCE_RESPONSE" | jq -r '.id // empty')
    if [ -z "$API_RESOURCE_ID" ]; then
        echo "ERROR: Failed to create API resource"
        echo "Response: $RESOURCE_RESPONSE"
        exit 1
    fi
    echo "  API resource created: $API_RESOURCE_ID"
fi

# Ensure scopes exist on the API resource
echo "  Ensuring API scopes..."
EXISTING_SCOPES=$(logto_get "/api/resources/$API_RESOURCE_ID/scopes" | jq -r '.[].name' 2>/dev/null || true)

for SCOPE in "manage:org" "read:credentials" "write:credentials" "read:tokens" "write:tokens" "admin" "viewer"; do
    if echo "$EXISTING_SCOPES" | grep -qx "$SCOPE"; then
        continue
    fi
    logto_post "/api/resources/$API_RESOURCE_ID/scopes" \
        "{\"name\":\"$SCOPE\",\"description\":\"$SCOPE permission\"}" > /dev/null 2>&1 || true
done
echo "  API scopes ready."

# ---------------------------------------------------------------------------
# Organization template: roles + permissions
# ---------------------------------------------------------------------------

echo ""
echo "Setting up organization template..."

# Create organization permissions
for PERM in "manage:org" "read:credentials" "write:credentials" "read:tokens" "write:tokens"; do
    logto_post "/api/organization-scopes" \
        "{\"name\":\"$PERM\",\"description\":\"$PERM\"}" > /dev/null 2>&1 || true
done
echo "  Organization permissions ready."

# Fetch org scope IDs
ALL_ORG_SCOPES=$(logto_get "/api/organization-scopes")

# Create organization roles
EXISTING_ORG_ROLES=$(logto_get "/api/organization-roles" | jq -r '.[].name' 2>/dev/null || true)

# Get scope IDs for role creation
API_SCOPES=$(logto_get "/api/resources/$API_RESOURCE_ID/scopes")
ADMIN_ORG_SCOPE_IDS=$(echo "$ALL_ORG_SCOPES" | jq -r '[.[].id]')
ADMIN_API_SCOPE_IDS=$(echo "$API_SCOPES" | jq -r '[.[] | select(.name == "admin" or .name == "manage:org" or .name == "read:credentials" or .name == "write:credentials" or .name == "read:tokens" or .name == "write:tokens") | .id]')
VIEWER_ORG_SCOPE_IDS=$(echo "$ALL_ORG_SCOPES" | jq -r '[.[] | select(.name | startswith("read:")) | .id]')
VIEWER_API_SCOPE_IDS=$(echo "$API_SCOPES" | jq -r '[.[] | select(.name == "viewer" or .name == "read:credentials" or .name == "read:tokens") | .id]')

# Create User-type roles (for human users)
if ! echo "$EXISTING_ORG_ROLES" | grep -qx "admin"; then
    logto_post "/api/organization-roles" \
        "{\"name\":\"admin\",\"description\":\"Organization administrator\",\"type\":\"User\",\"organizationScopeIds\":$ADMIN_ORG_SCOPE_IDS,\"resourceScopeIds\":$ADMIN_API_SCOPE_IDS}" > /dev/null 2>&1 || true
    echo "  Created 'admin' organization role (User)."
fi

if ! echo "$EXISTING_ORG_ROLES" | grep -qx "viewer"; then
    logto_post "/api/organization-roles" \
        "{\"name\":\"viewer\",\"description\":\"Organization viewer\",\"type\":\"User\",\"organizationScopeIds\":$VIEWER_ORG_SCOPE_IDS,\"resourceScopeIds\":$VIEWER_API_SCOPE_IDS}" > /dev/null 2>&1 || true
    echo "  Created 'viewer' organization role (User)."
fi

# Create MachineToMachine-type roles (for M2M apps / tests)
if ! echo "$EXISTING_ORG_ROLES" | grep -qx "m2m:admin"; then
    logto_post "/api/organization-roles" \
        "{\"name\":\"m2m:admin\",\"description\":\"M2M admin role\",\"type\":\"MachineToMachine\",\"organizationScopeIds\":$ADMIN_ORG_SCOPE_IDS,\"resourceScopeIds\":$ADMIN_API_SCOPE_IDS}" > /dev/null 2>&1 || true
    echo "  Created 'm2m:admin' organization role (MachineToMachine)."
fi

if ! echo "$EXISTING_ORG_ROLES" | grep -qx "m2m:viewer"; then
    logto_post "/api/organization-roles" \
        "{\"name\":\"m2m:viewer\",\"description\":\"M2M viewer role\",\"type\":\"MachineToMachine\",\"organizationScopeIds\":$VIEWER_ORG_SCOPE_IDS,\"resourceScopeIds\":$VIEWER_API_SCOPE_IDS}" > /dev/null 2>&1 || true
    echo "  Created 'm2m:viewer' organization role (MachineToMachine)."
fi

echo "  Organization template ready."

# ---------------------------------------------------------------------------
# M2M Application for LLMVault backend (org management)
# ---------------------------------------------------------------------------

echo ""
echo "Setting up backend M2M application..."

EXISTING_APPS=$(logto_get "/api/applications" | jq -r '.[] | select(.name == "llmvault-backend") | .id // empty')

if [ -n "$EXISTING_APPS" ]; then
    BACKEND_APP_ID="$EXISTING_APPS"
    echo "  Backend M2M app already exists: $BACKEND_APP_ID"
    # Fetch the secret
    BACKEND_APP_SECRET=$(logto_get "/api/applications/$BACKEND_APP_ID" | jq -r '.secret // empty')
else
    BACKEND_RESPONSE=$(logto_post "/api/applications" \
        '{"name":"llmvault-backend","type":"MachineToMachine","description":"LLMVault backend for org management"}')
    BACKEND_APP_ID=$(echo "$BACKEND_RESPONSE" | jq -r '.id // empty')
    BACKEND_APP_SECRET=$(echo "$BACKEND_RESPONSE" | jq -r '.secret // empty')
    if [ -z "$BACKEND_APP_ID" ]; then
        echo "ERROR: Failed to create backend M2M app"
        echo "Response: $BACKEND_RESPONSE"
        exit 1
    fi
    echo "  Backend M2M app created: $BACKEND_APP_ID"

    # Assign Management API access role to backend app
    # (it needs to create orgs, manage users, etc.)
    logto_post "/api/applications/$BACKEND_APP_ID/roles" \
        "{\"roleIds\":[\"lv-mgmt-role\"]}" > /dev/null 2>&1 || true
fi

# ---------------------------------------------------------------------------
# Traditional Web Application for dashboard (OIDC)
# ---------------------------------------------------------------------------

echo ""
echo "Setting up dashboard web application..."

EXISTING_DASHBOARD=$(logto_get "/api/applications" | jq -r '.[] | select(.name == "llmvault-dashboard") | .id // empty')

if [ -n "$EXISTING_DASHBOARD" ]; then
    DASHBOARD_APP_ID="$EXISTING_DASHBOARD"
    echo "  Dashboard app already exists: $DASHBOARD_APP_ID"
    DASHBOARD_APP_SECRET=$(logto_get "/api/applications/$DASHBOARD_APP_ID" | jq -r '.secret // empty')
else
    DASHBOARD_RESPONSE=$(logto_post "/api/applications" \
        "{\"name\":\"llmvault-dashboard\",\"type\":\"Traditional\",\"description\":\"LLMVault dashboard\",\"oidcClientMetadata\":{\"redirectUris\":[\"$DASHBOARD_REDIRECT_URI\"],\"postLogoutRedirectUris\":[\"$DASHBOARD_LOGOUT_URI\"]}}")
    DASHBOARD_APP_ID=$(echo "$DASHBOARD_RESPONSE" | jq -r '.id // empty')
    DASHBOARD_APP_SECRET=$(echo "$DASHBOARD_RESPONSE" | jq -r '.secret // empty')
    if [ -z "$DASHBOARD_APP_ID" ]; then
        echo "ERROR: Failed to create dashboard app"
        echo "Response: $DASHBOARD_RESPONSE"
        exit 1
    fi
    echo "  Dashboard app created: $DASHBOARD_APP_ID"
fi

# ---------------------------------------------------------------------------
# M2M Application for tests (acts as authenticated user in integration tests)
# ---------------------------------------------------------------------------

echo ""
echo "Setting up test M2M application..."

EXISTING_TEST_APP=$(logto_get "/api/applications" | jq -r '.[] | select(.name == "llmvault-test") | .id // empty')

if [ -n "$EXISTING_TEST_APP" ]; then
    TEST_APP_ID="$EXISTING_TEST_APP"
    echo "  Test M2M app already exists: $TEST_APP_ID"
    TEST_APP_SECRET=$(logto_get "/api/applications/$TEST_APP_ID" | jq -r '.secret // empty')
else
    TEST_RESPONSE=$(logto_post "/api/applications" \
        '{"name":"llmvault-test","type":"MachineToMachine","description":"Test M2M app for integration tests"}')
    TEST_APP_ID=$(echo "$TEST_RESPONSE" | jq -r '.id // empty')
    TEST_APP_SECRET=$(echo "$TEST_RESPONSE" | jq -r '.secret // empty')
    if [ -z "$TEST_APP_ID" ]; then
        echo "ERROR: Failed to create test M2M app"
        echo "Response: $TEST_RESPONSE"
        exit 1
    fi
    echo "  Test M2M app created: $TEST_APP_ID"

    # Assign Management API access to test app (for creating test orgs)
    logto_post "/api/applications/$TEST_APP_ID/roles" \
        "{\"roleIds\":[\"lv-mgmt-role\"]}" > /dev/null 2>&1 || true
fi

# ---------------------------------------------------------------------------
# Output
# ---------------------------------------------------------------------------

echo ""
echo "============================================"
echo "Logto initialized successfully."
echo "============================================"
echo ""
echo "Backend environment variables (add to .env):"
echo ""
echo "  LOGTO_ENDPOINT=${LOGTO_ENDPOINT}"
echo "  LOGTO_AUDIENCE=https://api.llmvault.dev"
echo "  LOGTO_M2M_APP_ID=$BACKEND_APP_ID"
echo "  LOGTO_M2M_APP_SECRET=$BACKEND_APP_SECRET"
echo ""
echo "Frontend (Next.js) environment variables:"
echo ""
echo "  AUTH_LOGTO_ID=$DASHBOARD_APP_ID"
echo "  AUTH_LOGTO_SECRET=$DASHBOARD_APP_SECRET"
echo "  AUTH_LOGTO_ISSUER=${LOGTO_ENDPOINT}/oidc"
echo ""
echo "Test environment variables:"
echo ""
echo "  LOGTO_TEST_APP_ID=$TEST_APP_ID"
echo "  LOGTO_TEST_APP_SECRET=$TEST_APP_SECRET"
echo ""
