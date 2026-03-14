#!/bin/sh
set -e

# ---------------------------------------------------------------------------
# Logto production bootstrap (Phase 2 only — Management API)
#
# Requires an existing M2M app with Management API access.
# See comments at the bottom for how to create one manually.
#
# Usage:
#   LOGTO_ENDPOINT=https://auth.llmvault.dev \
#   M2M_APP_ID=<your-m2m-app-id> \
#   M2M_APP_SECRET=<your-m2m-app-secret> \
#   DASHBOARD_REDIRECT_URI=https://app.llmvault.dev/callback \
#   DASHBOARD_LOGOUT_URI=https://app.llmvault.dev \
#   ./init-prod.sh
# ---------------------------------------------------------------------------

: "${LOGTO_ENDPOINT:?LOGTO_ENDPOINT is required}"
: "${M2M_APP_ID:?M2M_APP_ID is required}"
: "${M2M_APP_SECRET:?M2M_APP_SECRET is required}"
: "${DASHBOARD_REDIRECT_URI:?DASHBOARD_REDIRECT_URI is required}"
: "${DASHBOARD_LOGOUT_URI:?DASHBOARD_LOGOUT_URI is required}"

# ---------------------------------------------------------------------------
# Obtain Management API token
# ---------------------------------------------------------------------------

echo "Obtaining Management API token..."

TOKEN_RESPONSE=$(curl -sf -X POST "${LOGTO_ENDPOINT}/oidc/token" \
    -H "Content-Type: application/x-www-form-urlencoded" \
    -d "grant_type=client_credentials&client_id=${M2M_APP_ID}&client_secret=${M2M_APP_SECRET}&resource=https://default.logto.app/api&scope=all")

ACCESS_TOKEN=$(echo "$TOKEN_RESPONSE" | jq -r '.access_token // empty')
if [ -z "$ACCESS_TOKEN" ]; then
    echo "ERROR: Failed to obtain Management API token"
    echo "Response: $TOKEN_RESPONSE"
    exit 1
fi
echo "Token obtained."

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

# Create User-type roles
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

# Create MachineToMachine-type roles
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

EXISTING_BACKEND=$(logto_get "/api/applications" | jq -r '.[] | select(.name == "llmvault-backend") | .id // empty')

if [ -n "$EXISTING_BACKEND" ]; then
    BACKEND_APP_ID="$EXISTING_BACKEND"
    echo "  Backend M2M app already exists: $BACKEND_APP_ID"
    BACKEND_APP_SECRET=$(logto_get "/api/applications/$BACKEND_APP_ID" | jq -r '.secret // empty')
else
    BACKEND_RESPONSE=$(logto_post "/api/applications" \
        '{"name":"llmvault-backend","type":"MachineToMachine","description":"LLMVault backend for org management"}')
    BACKEND_APP_ID=$(echo "$BACKEND_RESPONSE" | jq -r '.id // empty')
    BACKEND_APP_SECRET=$(echo "$BACKEND_RESPONSE" | jq -r '.secret // empty')
    if [ -z "$BACKEND_APP_ID" ]; then
        echo "ERROR: Failed to create backend M2M app"
        exit 1
    fi
    echo "  Backend M2M app created: $BACKEND_APP_ID"
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
        exit 1
    fi
    echo "  Dashboard app created: $DASHBOARD_APP_ID"
fi

# ---------------------------------------------------------------------------
# Output
# ---------------------------------------------------------------------------

echo ""
echo "============================================"
echo "Logto production setup complete."
echo "============================================"
echo ""
echo "Backend env vars (for llmvault API service):"
echo ""
echo "  LOGTO_ENDPOINT=${LOGTO_ENDPOINT}"
echo "  LOGTO_AUDIENCE=https://api.llmvault.dev"
echo "  LOGTO_M2M_APP_ID=$BACKEND_APP_ID"
echo "  LOGTO_M2M_APP_SECRET=$BACKEND_APP_SECRET"
echo ""
echo "Dashboard env vars (for apps/web):"
echo ""
echo "  LOGTO_ENDPOINT=${LOGTO_ENDPOINT}"
echo "  LOGTO_APP_ID=$DASHBOARD_APP_ID"
echo "  LOGTO_APP_SECRET=$DASHBOARD_APP_SECRET"
echo "  LOGTO_BASE_URL=<your-dashboard-url>"
echo "  LOGTO_COOKIE_SECRET=<generate with: openssl rand -hex 32>"
echo "  NEXT_PUBLIC_API_RESOURCE=https://api.llmvault.dev"
echo ""
