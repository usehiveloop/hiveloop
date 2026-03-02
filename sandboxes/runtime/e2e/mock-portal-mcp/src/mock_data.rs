use serde_json::{json, Value};

/// Team "ENG" with members and statuses.
pub fn team_eng() -> Value {
    json!({
        "id": "team_eng_001",
        "name": "ENG",
        "key": "ENG",
        "description": "Engineering team",
        "icon": "code",
        "color": "#3B82F6"
    })
}

pub fn team_members() -> Value {
    json!([
        {"id": "member_001", "name": "Alice Chen", "email": "alice@portal.dev", "role": "admin", "displayName": "Alice Chen"},
        {"id": "member_002", "name": "Bob Martinez", "email": "bob@portal.dev", "role": "member", "displayName": "Bob Martinez"},
        {"id": "member_003", "name": "Charlie Kim", "email": "charlie@portal.dev", "role": "member", "displayName": "Charlie Kim"},
        {"id": "member_004", "name": "Dana Patel", "email": "dana@portal.dev", "role": "member", "displayName": "Dana Patel"},
        {"id": "member_005", "name": "Eve Johnson", "email": "eve@portal.dev", "role": "lead", "displayName": "Eve Johnson"}
    ])
}

pub fn statuses() -> Value {
    json!([
        {"id": "status_backlog", "name": "Backlog", "color": "#6B7280", "position": 0, "type": "backlog"},
        {"id": "status_todo", "name": "Todo", "color": "#EAB308", "position": 1, "type": "unstarted"},
        {"id": "status_in_progress", "name": "In Progress", "color": "#3B82F6", "position": 2, "type": "started"},
        {"id": "status_in_review", "name": "In Review", "color": "#8B5CF6", "position": 3, "type": "started"},
        {"id": "status_qa", "name": "QA", "color": "#F97316", "position": 4, "type": "started"},
        {"id": "status_done", "name": "Done", "color": "#22C55E", "position": 5, "type": "completed"}
    ])
}

pub fn labels() -> Value {
    json!([
        {"id": "label_bug", "name": "bug", "color": "#EF4444"},
        {"id": "label_feature", "name": "feature", "color": "#3B82F6"},
        {"id": "label_security", "name": "security", "color": "#F59E0B"},
        {"id": "label_docs", "name": "documentation", "color": "#10B981"},
        {"id": "label_perf", "name": "performance", "color": "#8B5CF6"}
    ])
}

/// 3 issues in the ENG team.
pub fn issues() -> Vec<Value> {
    vec![
        json!({
            "id": "issue_042",
            "identifier": "ENG-42",
            "title": "Implement JWT authentication middleware",
            "description": "Add JWT-based authentication middleware to all protected API endpoints. Should validate tokens, extract user claims, and attach user context to requests.",
            "status": {"id": "status_in_progress", "name": "In Progress"},
            "priority": 1,
            "priorityLabel": "Urgent",
            "assignee": {"id": "member_001", "name": "Alice Chen"},
            "labels": [{"id": "label_feature", "name": "feature"}, {"id": "label_security", "name": "security"}],
            "team": {"id": "team_eng_001", "name": "ENG", "key": "ENG"},
            "project": {"id": "project_q1", "name": "Q1 Launch"},
            "createdAt": "2025-01-15T10:00:00Z",
            "updatedAt": "2025-02-20T14:30:00Z"
        }),
        json!({
            "id": "issue_043",
            "identifier": "ENG-43",
            "title": "Add Redis caching layer for API responses",
            "description": "Implement a caching layer using Redis to cache frequently accessed API responses. Should support TTL-based expiration and cache invalidation on writes.",
            "status": {"id": "status_todo", "name": "Todo"},
            "priority": 2,
            "priorityLabel": "High",
            "assignee": {"id": "member_003", "name": "Charlie Kim"},
            "labels": [{"id": "label_feature", "name": "feature"}, {"id": "label_perf", "name": "performance"}],
            "team": {"id": "team_eng_001", "name": "ENG", "key": "ENG"},
            "project": {"id": "project_q1", "name": "Q1 Launch"},
            "createdAt": "2025-01-20T09:00:00Z",
            "updatedAt": "2025-02-18T11:00:00Z"
        }),
        json!({
            "id": "issue_044",
            "identifier": "ENG-44",
            "title": "Write comprehensive API documentation",
            "description": "Document all HTTP API endpoints including request/response formats, authentication requirements, error codes, and example usage. Create an API reference document.",
            "status": {"id": "status_backlog", "name": "Backlog"},
            "priority": 3,
            "priorityLabel": "Medium",
            "assignee": null,
            "labels": [{"id": "label_docs", "name": "documentation"}],
            "team": {"id": "team_eng_001", "name": "ENG", "key": "ENG"},
            "project": {"id": "project_q1", "name": "Q1 Launch"},
            "createdAt": "2025-01-25T08:00:00Z",
            "updatedAt": "2025-02-10T16:00:00Z"
        }),
    ]
}

pub fn get_issue(identifier: &str) -> Option<Value> {
    issues().into_iter().find(|i| {
        i.get("identifier")
            .and_then(|v| v.as_str())
            .is_some_and(|id| id.eq_ignore_ascii_case(identifier))
            || i.get("id")
                .and_then(|v| v.as_str())
                .is_some_and(|id| id == identifier)
    })
}

/// Pull request #156 linked to ENG-42.
pub fn pull_request_156() -> Value {
    json!({
        "id": "pr_156",
        "number": 156,
        "title": "feat: add JWT authentication middleware",
        "body": "## Summary\n\nAdds JWT-based authentication middleware for all protected API endpoints.\n\n## Changes\n- New `auth/middleware.rs` with JWT validation\n- Token extraction from Authorization header\n- User context attachment to request extensions\n- Integration tests for auth flows\n\n## Test Plan\n- Unit tests for token validation\n- Integration tests for protected endpoints\n- Manual testing with Postman",
        "state": "open",
        "author": {"login": "alice-chen", "id": "member_001"},
        "headBranch": "feat/jwt-auth",
        "baseBranch": "main",
        "url": "https://github.com/portal-dev/bridge/pull/156",
        "additions": 342,
        "deletions": 12,
        "changedFiles": 8,
        "createdAt": "2025-02-18T10:00:00Z",
        "updatedAt": "2025-02-20T14:00:00Z",
        "diff": "diff --git a/src/auth/middleware.rs b/src/auth/middleware.rs\nnew file mode 100644\nindex 0000000..a1b2c3d\n--- /dev/null\n+++ b/src/auth/middleware.rs\n@@ -0,0 +1,85 @@\n+use axum::{\n+    extract::Request,\n+    http::StatusCode,\n+    middleware::Next,\n+    response::Response,\n+};\n+use jsonwebtoken::{decode, DecodingKey, Validation, Algorithm};\n+use serde::{Deserialize, Serialize};\n+\n+#[derive(Debug, Serialize, Deserialize, Clone)]\n+pub struct Claims {\n+    pub sub: String,\n+    pub email: String,\n+    pub role: String,\n+    pub exp: usize,\n+}\n+\n+pub async fn jwt_auth(\n+    mut req: Request,\n+    next: Next,\n+) -> Result<Response, StatusCode> {\n+    let token = req\n+        .headers()\n+        .get(\"Authorization\")\n+        .and_then(|v| v.to_str().ok())\n+        .and_then(|v| v.strip_prefix(\"Bearer \"))\n+        .ok_or(StatusCode::UNAUTHORIZED)?;\n+\n+    let secret = std::env::var(\"JWT_SECRET\")\n+        .unwrap_or_else(|_| \"default-secret\".to_string());\n+\n+    let token_data = decode::<Claims>(\n+        token,\n+        &DecodingKey::from_secret(secret.as_bytes()),\n+        &Validation::new(Algorithm::HS256),\n+    )\n+    .map_err(|_| StatusCode::UNAUTHORIZED)?;\n+\n+    req.extensions_mut().insert(token_data.claims);\n+    Ok(next.run(req).await)\n+}\n+\ndiff --git a/src/auth/mod.rs b/src/auth/mod.rs\nnew file mode 100644\nindex 0000000..b2c3d4e\n--- /dev/null\n+++ b/src/auth/mod.rs\n@@ -0,0 +1,2 @@\n+pub mod middleware;\n+pub use middleware::{jwt_auth, Claims};\n"
    })
}

pub fn issue_pull_requests() -> Value {
    json!([{
        "id": "pr_156",
        "number": 156,
        "title": "feat: add JWT authentication middleware",
        "state": "open",
        "url": "https://github.com/portal-dev/bridge/pull/156"
    }])
}

pub fn pr_comments() -> Value {
    json!([
        {
            "id": "comment_pr_1",
            "body": "Nice approach with the middleware pattern. Consider adding rate limiting per-user as well.",
            "author": {"login": "bob-martinez"},
            "createdAt": "2025-02-19T09:00:00Z",
            "path": null,
            "line": null
        },
        {
            "id": "comment_pr_2",
            "body": "The `unwrap_or_else` for JWT_SECRET should probably log a warning in production.",
            "author": {"login": "eve-johnson"},
            "createdAt": "2025-02-19T14:00:00Z",
            "path": "src/auth/middleware.rs",
            "line": 32
        }
    ])
}

pub fn pr_reviews() -> Value {
    json!([{
        "id": "review_001",
        "author": {"login": "eve-johnson"},
        "state": "APPROVED",
        "body": "LGTM! Good test coverage. One minor suggestion about the secret handling.",
        "submittedAt": "2025-02-20T10:00:00Z"
    }])
}

pub fn pr_checks() -> Value {
    json!([
        {"id": "check_1", "name": "CI / Build", "status": "completed", "conclusion": "success"},
        {"id": "check_2", "name": "CI / Test", "status": "completed", "conclusion": "success"},
        {"id": "check_3", "name": "CI / Lint", "status": "completed", "conclusion": "success"},
        {"id": "check_4", "name": "Security / Audit", "status": "completed", "conclusion": "success"},
        {"id": "check_5", "name": "Coverage", "status": "completed", "conclusion": "success"}
    ])
}

/// Project "Q1 Launch".
pub fn project_q1() -> Value {
    json!({
        "id": "project_q1",
        "name": "Q1 Launch",
        "description": "Q1 2025 product launch milestone — authentication, caching, and API docs.",
        "status": "in_progress",
        "startDate": "2025-01-01",
        "targetDate": "2025-03-31",
        "lead": {"id": "member_005", "name": "Eve Johnson"},
        "issues": ["issue_042", "issue_043", "issue_044"]
    })
}

pub fn projects() -> Value {
    json!([project_q1()])
}

/// Documents.
pub fn documents() -> Vec<Value> {
    vec![
        json!({
            "id": "doc_prd",
            "title": "Q1 Launch PRD",
            "content": "# Q1 Launch Product Requirements\n\n## Overview\nThis document outlines the product requirements for the Q1 2025 launch.\n\n## Features\n1. JWT Authentication - Secure all API endpoints\n2. Redis Caching - Improve response times\n3. API Documentation - Comprehensive reference\n\n## Timeline\n- Jan: Auth implementation\n- Feb: Caching layer\n- Mar: Documentation and launch",
            "createdAt": "2025-01-05T10:00:00Z",
            "updatedAt": "2025-02-15T09:00:00Z",
            "createdBy": {"id": "member_005", "name": "Eve Johnson"},
            "project": {"id": "project_q1", "name": "Q1 Launch"}
        }),
        json!({
            "id": "doc_api_spec",
            "title": "API Specification v2",
            "content": "# Bridge API Specification\n\n## Endpoints\n\n### Health\n- GET /health - Health check\n\n### Agents\n- GET /agents - List agents\n- GET /agents/:id - Get agent\n\n### Conversations\n- POST /agents/:id/conversations - Create conversation\n- POST /conversations/:id/messages - Send message\n- GET /conversations/:id/stream - SSE stream\n- DELETE /conversations/:id - End conversation\n\n## Authentication\nAll endpoints require Bearer token in Authorization header.",
            "createdAt": "2025-01-10T08:00:00Z",
            "updatedAt": "2025-02-20T16:00:00Z",
            "createdBy": {"id": "member_002", "name": "Bob Martinez"},
            "project": {"id": "project_q1", "name": "Q1 Launch"}
        }),
    ]
}

pub fn comments() -> Value {
    json!([
        {
            "id": "comment_001",
            "body": "Started working on the JWT middleware. Using HS256 for now, will add RS256 support later.",
            "author": {"id": "member_001", "name": "Alice Chen"},
            "issueId": "issue_042",
            "createdAt": "2025-02-18T11:00:00Z"
        }
    ])
}

pub fn workspace_members() -> Value {
    team_members()
}

pub fn issue_dependencies() -> Value {
    json!([
        {
            "id": "dep_001",
            "issueId": "issue_043",
            "relatedIssueId": "issue_042",
            "type": "blocks"
        }
    ])
}
