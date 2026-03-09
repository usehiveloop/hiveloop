#!/bin/sh
set -e

# ---------------------------------------------------------------------------
# ZITADEL bootstrap script (idempotent)
#
# Creates the llmvault project, roles, API app, and OIDC dashboard app.
# Safe to run multiple times — checks ZITADEL state before creating anything.
#
# In docker-compose: writes credential files to the shared bootstrap volume
# so the proxy and frontend containers can consume them.
#
# In production: prints env vars to stdout. Set them for backend/frontend
# and don't run this script again.
# ---------------------------------------------------------------------------

# Required
: "${ZITADEL_URL:?ZITADEL_URL is required (e.g. http://zitadel:8080)}"
: "${ZITADEL_EXTERNAL_URL:?ZITADEL_EXTERNAL_URL is required (e.g. https://auth.llmvault.dev)}"
: "${DASHBOARD_REDIRECT_URI:?DASHBOARD_REDIRECT_URI is required (e.g. https://app.llmvault.dev/api/auth/callback/zitadel)}"
: "${DASHBOARD_LOGOUT_URI:?DASHBOARD_LOGOUT_URI is required (e.g. https://app.llmvault.dev)}"

# Optional
HOST_HEADER="${ZITADEL_HOST_HEADER:-${ZITADEL_EXTERNALDOMAIN:-localhost}}"
PAT_FILE="${ZITADEL_PAT_FILE:-/zitadel/bootstrap/admin.pat}"
DEV_MODE="${ZITADEL_DEV_MODE:-false}"

apk add --no-cache curl jq > /dev/null 2>&1

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

zitadel_get() {
    curl -sf "$ZITADEL_URL$1" \
        -H "Host: $HOST_HEADER" \
        -H "Authorization: Bearer $PAT" \
        -H "Content-Type: application/json"
}

zitadel_post() {
    curl -sf -X POST "$ZITADEL_URL$1" \
        -H "Host: $HOST_HEADER" \
        -H "Authorization: Bearer $PAT" \
        -H "Content-Type: application/json" \
        -d "$2"
}

# ---------------------------------------------------------------------------
# Wait for ZITADEL
# ---------------------------------------------------------------------------

echo "Waiting for ZITADEL admin PAT..."
RETRIES=0
while [ ! -f "$PAT_FILE" ]; do
    RETRIES=$((RETRIES + 1))
    if [ "$RETRIES" -gt 60 ]; then
        echo "Timed out waiting for admin PAT"
        exit 1
    fi
    sleep 2
done
PAT=$(cat "$PAT_FILE")
echo "Admin PAT loaded."

echo "Waiting for ZITADEL API..."
RETRIES=0
until zitadel_get "/auth/v1/users/me" > /dev/null 2>&1; do
    RETRIES=$((RETRIES + 1))
    if [ "$RETRIES" -gt 30 ]; then
        echo "Timed out waiting for ZITADEL API"
        exit 1
    fi
    sleep 2
done
echo "ZITADEL API is ready."

# ---------------------------------------------------------------------------
# Project (idempotent: search first, create only if missing)
# ---------------------------------------------------------------------------

echo "Checking for existing project..."
SEARCH_RESPONSE=$(zitadel_post "/management/v1/projects/_search" \
    '{"queries":[{"nameQuery":{"name":"llmvault","method":"TEXT_QUERY_METHOD_EQUALS"}}]}')

PROJECT_ID=$(echo "$SEARCH_RESPONSE" | jq -r '.result[0].id // empty')

if [ -n "$PROJECT_ID" ]; then
    echo "Project already exists: $PROJECT_ID"
else
    echo "Creating project..."
    PROJECT_RESPONSE=$(zitadel_post "/management/v1/projects" \
        '{"name": "llmvault", "projectRoleAssertion": true}')
    PROJECT_ID=$(echo "$PROJECT_RESPONSE" | jq -r '.id')
    if [ -z "$PROJECT_ID" ] || [ "$PROJECT_ID" = "null" ]; then
        echo "Failed to create project: $PROJECT_RESPONSE"
        exit 1
    fi
    echo "Project created: $PROJECT_ID"
fi

# ---------------------------------------------------------------------------
# Roles (idempotent: bulk add ignores existing roles)
# ---------------------------------------------------------------------------

echo "Ensuring project roles..."
zitadel_post "/management/v1/projects/$PROJECT_ID/roles/_bulk" \
    '{"roles": [{"key": "admin", "displayName": "Admin"}, {"key": "viewer", "displayName": "Viewer"}]}' > /dev/null 2>&1 || true
echo "Roles ready."

# ---------------------------------------------------------------------------
# API Application (idempotent: search by name in project apps)
# ---------------------------------------------------------------------------

echo "Checking for existing API application..."
APPS_RESPONSE=$(zitadel_post "/management/v1/projects/$PROJECT_ID/apps/_search" '{}')
API_APP_ID=$(echo "$APPS_RESPONSE" | jq -r '[(.result // [])[] | select(.name == "llmvault-api")] | .[0].id // empty')

if [ -n "$API_APP_ID" ]; then
    echo "API application already exists: $API_APP_ID"
    # Read existing clientId from app details
    API_APP_DETAIL=$(zitadel_get "/management/v1/projects/$PROJECT_ID/apps/$API_APP_ID")
    API_CLIENT_ID=$(echo "$API_APP_DETAIL" | jq -r '.app.apiConfig.clientId // .app.oidcConfig.clientId // empty')

    if [ -z "$API_CLIENT_ID" ]; then
        echo "Warning: could not read clientId from existing API app"
    fi

    # Regenerate secret so we always have a valid one
    SECRET_RESPONSE=$(zitadel_post "/management/v1/projects/$PROJECT_ID/apps/$API_APP_ID/api_config/_generate_client_secret" '{}')
    API_CLIENT_SECRET=$(echo "$SECRET_RESPONSE" | jq -r '.clientSecret // empty')

    if [ -z "$API_CLIENT_SECRET" ]; then
        echo "Warning: could not regenerate API client secret"
    fi
else
    echo "Creating API application..."
    APP_RESPONSE=$(zitadel_post "/management/v1/projects/$PROJECT_ID/apps/api" \
        '{"name": "llmvault-api", "authMethodType": "API_AUTH_METHOD_TYPE_BASIC"}')
    API_CLIENT_ID=$(echo "$APP_RESPONSE" | jq -r '.clientId')
    API_CLIENT_SECRET=$(echo "$APP_RESPONSE" | jq -r '.clientSecret')

    if [ -z "$API_CLIENT_ID" ] || [ "$API_CLIENT_ID" = "null" ]; then
        echo "Failed to create API app: $APP_RESPONSE"
        exit 1
    fi
    echo "API application created: $API_CLIENT_ID"
fi

# ---------------------------------------------------------------------------
# OIDC Dashboard Application (idempotent: search by name in project apps)
# ---------------------------------------------------------------------------

DASHBOARD_APP_ID=$(echo "$APPS_RESPONSE" | jq -r '[(.result // [])[] | select(.name == "llmvault-dashboard")] | .[0].id // empty')

if [ -n "$DASHBOARD_APP_ID" ]; then
    echo "OIDC dashboard application already exists: $DASHBOARD_APP_ID"
    DASHBOARD_APP_DETAIL=$(zitadel_get "/management/v1/projects/$PROJECT_ID/apps/$DASHBOARD_APP_ID")
    DASHBOARD_CLIENT_ID=$(echo "$DASHBOARD_APP_DETAIL" | jq -r '.app.oidcConfig.clientId // empty')

    if [ -z "$DASHBOARD_CLIENT_ID" ]; then
        echo "Warning: could not read clientId from existing dashboard app"
    fi
else
    echo "Creating OIDC dashboard application..."
    OIDC_RESPONSE=$(zitadel_post "/management/v1/projects/$PROJECT_ID/apps/oidc" \
        "{
            \"name\": \"llmvault-dashboard\",
            \"redirectUris\": [\"$DASHBOARD_REDIRECT_URI\"],
            \"postLogoutRedirectUris\": [\"$DASHBOARD_LOGOUT_URI\"],
            \"responseTypes\": [\"OIDC_RESPONSE_TYPE_CODE\"],
            \"grantTypes\": [\"OIDC_GRANT_TYPE_AUTHORIZATION_CODE\"],
            \"appType\": \"OIDC_APP_TYPE_WEB\",
            \"authMethodType\": \"OIDC_AUTH_METHOD_TYPE_NONE\",
            \"accessTokenType\": \"OIDC_TOKEN_TYPE_BEARER\",
            \"devMode\": $DEV_MODE
        }")
    DASHBOARD_CLIENT_ID=$(echo "$OIDC_RESPONSE" | jq -r '.clientId')

    if [ -z "$DASHBOARD_CLIENT_ID" ] || [ "$DASHBOARD_CLIENT_ID" = "null" ]; then
        echo "Failed to create OIDC app: $OIDC_RESPONSE"
        exit 1
    fi
    echo "OIDC dashboard application created: $DASHBOARD_CLIENT_ID"
fi

# ---------------------------------------------------------------------------
# Output — copy these into your .env
# ---------------------------------------------------------------------------

echo ""
echo "============================================"
echo "ZITADEL initialized successfully."
echo "============================================"
echo ""
echo "Backend environment variables (add to .env):"
echo ""
echo "  ZITADEL_PROJECT_ID=$PROJECT_ID"
echo "  ZITADEL_CLIENT_ID=$API_CLIENT_ID"
echo "  ZITADEL_CLIENT_SECRET=$API_CLIENT_SECRET"
echo "  ZITADEL_ADMIN_PAT=$(cat "$PAT_FILE")"
echo ""
echo "Frontend (Next.js) environment variables:"
echo ""
echo "  AUTH_ZITADEL_ID=$DASHBOARD_CLIENT_ID"
echo "  AUTH_ZITADEL_ISSUER=$ZITADEL_EXTERNAL_URL"
echo ""
