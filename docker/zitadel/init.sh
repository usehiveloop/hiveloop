#!/bin/sh
set -e

ZITADEL_URL="http://zitadel:8080"
HOST_HEADER="localhost"
CREDS_FILE="/zitadel/bootstrap/api-credentials.json"

# If already initialized, skip
if [ -f "$CREDS_FILE" ]; then
    echo "ZITADEL already initialized, skipping."
    exit 0
fi

apk add --no-cache curl jq > /dev/null 2>&1

# Wait for the admin PAT to be written by ZITADEL
echo "Waiting for ZITADEL admin PAT..."
RETRIES=0
while [ ! -f /zitadel/bootstrap/admin.pat ]; do
    RETRIES=$((RETRIES + 1))
    if [ "$RETRIES" -gt 60 ]; then
        echo "Timed out waiting for admin PAT"
        exit 1
    fi
    sleep 2
done

PAT=$(cat /zitadel/bootstrap/admin.pat)
echo "Admin PAT loaded."

# Wait for ZITADEL API to be responsive
echo "Waiting for ZITADEL API..."
RETRIES=0
until curl -sf -H "Host: $HOST_HEADER" "$ZITADEL_URL/auth/v1/users/me" \
    -H "Authorization: Bearer $PAT" > /dev/null 2>&1; do
    RETRIES=$((RETRIES + 1))
    if [ "$RETRIES" -gt 30 ]; then
        echo "Timed out waiting for ZITADEL API"
        exit 1
    fi
    sleep 2
done
echo "ZITADEL API is ready."

# Create project "llmvault"
echo "Creating project..."
PROJECT_RESPONSE=$(curl -sf -X POST "$ZITADEL_URL/management/v1/projects" \
    -H "Host: $HOST_HEADER" \
    -H "Authorization: Bearer $PAT" \
    -H "Content-Type: application/json" \
    -d '{"name": "llmvault", "projectRoleAssertion": true}')

PROJECT_ID=$(echo "$PROJECT_RESPONSE" | jq -r '.id')
if [ -z "$PROJECT_ID" ] || [ "$PROJECT_ID" = "null" ]; then
    echo "Failed to create project: $PROJECT_RESPONSE"
    exit 1
fi
echo "Project created: $PROJECT_ID"

# Add roles to the project
echo "Adding project roles..."
curl -sf -X POST "$ZITADEL_URL/management/v1/projects/$PROJECT_ID/roles/_bulk" \
    -H "Host: $HOST_HEADER" \
    -H "Authorization: Bearer $PAT" \
    -H "Content-Type: application/json" \
    -d '{"roles": [{"key": "admin", "displayName": "Admin"}, {"key": "viewer", "displayName": "Viewer"}]}' > /dev/null

echo "Roles added."

# Create API application (for introspection authentication)
echo "Creating API application..."
APP_RESPONSE=$(curl -sf -X POST "$ZITADEL_URL/management/v1/projects/$PROJECT_ID/apps/api" \
    -H "Host: $HOST_HEADER" \
    -H "Authorization: Bearer $PAT" \
    -H "Content-Type: application/json" \
    -d '{"name": "llmvault-api", "authMethodType": "API_AUTH_METHOD_TYPE_BASIC"}')

CLIENT_ID=$(echo "$APP_RESPONSE" | jq -r '.clientId')
CLIENT_SECRET=$(echo "$APP_RESPONSE" | jq -r '.clientSecret')

if [ -z "$CLIENT_ID" ] || [ "$CLIENT_ID" = "null" ]; then
    echo "Failed to create API app: $APP_RESPONSE"
    exit 1
fi

# Write credentials file
cat > "$CREDS_FILE" << EOF
{
    "projectId": "$PROJECT_ID",
    "clientId": "$CLIENT_ID",
    "clientSecret": "$CLIENT_SECRET"
}
EOF

echo "ZITADEL initialized successfully."
echo "  Project ID:    $PROJECT_ID"
echo "  Client ID:     $CLIENT_ID"
echo "  Credentials:   $CREDS_FILE"
