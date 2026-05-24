use axum::extract::Path;
use axum::http::StatusCode;
use axum::response::IntoResponse;
use axum::Json;
use serde_json::{json, Value};

/// POST /integrations/{integration_name}/actions/{action_name}
///
/// Returns realistic mock data for each integration + action pair.
/// The response format mirrors the actual external service's API response.
pub async fn execute_integration_action(
    Path((integration_name, action_name)): Path<(String, String)>,
    Json(body): Json<Value>,
) -> impl IntoResponse {
    let params = body.get("params").cloned().unwrap_or(json!({}));
    let (status, response) = integration_response(&integration_name, &action_name, &params);
    (status, Json(response))
}

fn integration_response(integration: &str, action: &str, params: &Value) -> (StatusCode, Value) {
    match (integration, action) {
        // ── GitHub ──────────────────────────────────────────────
        ("github", "create_pull_request") => (
            StatusCode::OK,
            json!({
                "id": 42,
                "number": 123,
                "title": params.get("title").and_then(|v| v.as_str()).unwrap_or("Mock PR"),
                "body": params.get("body").and_then(|v| v.as_str()).unwrap_or(""),
                "state": "open",
                "html_url": "https://github.com/acme/repo/pull/123",
                "head": {
                    "ref": params.get("head").and_then(|v| v.as_str()).unwrap_or("feature-branch"),
                    "sha": "abc123def456"
                },
                "base": {
                    "ref": params.get("base").and_then(|v| v.as_str()).unwrap_or("main"),
                    "sha": "789ghi012jkl"
                },
                "user": { "login": "bot-user", "id": 99 },
                "created_at": "2026-03-06T10:00:00Z",
                "updated_at": "2026-03-06T10:00:00Z",
                "mergeable": true,
                "draft": false,
                "additions": 47,
                "deletions": 12,
                "changed_files": 3
            }),
        ),

        ("github", "list_issues") => (
            StatusCode::OK,
            json!([
                {
                    "id": 1001,
                    "number": 45,
                    "title": "Fix login page crash on mobile",
                    "state": "open",
                    "labels": [
                        { "name": "bug", "color": "d73a4a" },
                        { "name": "priority:high", "color": "e11d48" }
                    ],
                    "assignee": { "login": "dev-alice", "id": 201 },
                    "created_at": "2026-03-01T08:30:00Z",
                    "updated_at": "2026-03-05T14:20:00Z",
                    "html_url": "https://github.com/acme/repo/issues/45",
                    "comments": 3
                },
                {
                    "id": 1002,
                    "number": 46,
                    "title": "Add dark mode support",
                    "state": "open",
                    "labels": [
                        { "name": "enhancement", "color": "a2eeef" }
                    ],
                    "assignee": null,
                    "created_at": "2026-03-02T14:15:00Z",
                    "updated_at": "2026-03-03T09:00:00Z",
                    "html_url": "https://github.com/acme/repo/issues/46",
                    "comments": 7
                },
                {
                    "id": 1003,
                    "number": 47,
                    "title": "Update dependencies to latest versions",
                    "state": "open",
                    "labels": [
                        { "name": "maintenance", "color": "0075ca" }
                    ],
                    "assignee": { "login": "dev-bob", "id": 202 },
                    "created_at": "2026-03-04T11:00:00Z",
                    "updated_at": "2026-03-04T11:00:00Z",
                    "html_url": "https://github.com/acme/repo/issues/47",
                    "comments": 0
                }
            ]),
        ),

        ("github", "get_repository") => (
            StatusCode::OK,
            json!({
                "id": 500,
                "name": "repo",
                "full_name": "acme/repo",
                "private": false,
                "description": "A sample repository for testing integrations",
                "default_branch": "main",
                "stargazers_count": 142,
                "forks_count": 23,
                "open_issues_count": 12,
                "language": "Rust",
                "html_url": "https://github.com/acme/repo",
                "created_at": "2024-01-15T10:00:00Z",
                "updated_at": "2026-03-06T08:30:00Z"
            }),
        ),

        // ── Mailchimp ──────────────────────────────────────────
        ("mailchimp", "create_campaign") => (
            StatusCode::OK,
            json!({
                "id": "mc_campaign_abc123",
                "web_id": 7890,
                "type": "regular",
                "status": "save",
                "emails_sent": 0,
                "send_time": null,
                "content_type": "template",
                "recipients": {
                    "list_id": params.get("list_id").and_then(|v| v.as_str()).unwrap_or("list_default"),
                    "list_is_active": true,
                    "list_name": "Main Newsletter",
                    "segment_text": "All subscribers",
                    "recipient_count": 1542
                },
                "settings": {
                    "subject_line": params.get("subject").and_then(|v| v.as_str()).unwrap_or("Newsletter"),
                    "from_name": "Acme Inc",
                    "reply_to": "hello@acme.com",
                    "auto_footer": true
                },
                "tracking": {
                    "opens": true,
                    "html_clicks": true,
                    "text_clicks": false,
                    "goal_tracking": false
                },
                "create_time": "2026-03-06T10:00:00+00:00"
            }),
        ),

        ("mailchimp", "list_subscribers") => (
            StatusCode::OK,
            json!({
                "members": [
                    {
                        "id": "sub_001",
                        "email_address": "alice@example.com",
                        "status": "subscribed",
                        "merge_fields": { "FNAME": "Alice", "LNAME": "Smith" },
                        "stats": { "avg_open_rate": 0.45, "avg_click_rate": 0.12 },
                        "last_changed": "2026-02-15T10:00:00+00:00"
                    },
                    {
                        "id": "sub_002",
                        "email_address": "bob@example.com",
                        "status": "subscribed",
                        "merge_fields": { "FNAME": "Bob", "LNAME": "Jones" },
                        "stats": { "avg_open_rate": 0.38, "avg_click_rate": 0.08 },
                        "last_changed": "2026-01-20T14:30:00+00:00"
                    }
                ],
                "total_items": 2,
                "list_id": "list_default"
            }),
        ),

        // ── Slack ──────────────────────────────────────────────
        ("slack", "send_message") => (
            StatusCode::OK,
            json!({
                "ok": true,
                "channel": params.get("channel").and_then(|v| v.as_str()).unwrap_or("C01234567"),
                "ts": "1709726400.000100",
                "message": {
                    "text": params.get("text").and_then(|v| v.as_str()).unwrap_or("Hello from integration"),
                    "user": "U_BOT_001",
                    "type": "message",
                    "ts": "1709726400.000100",
                    "bot_id": "B_INTEGRATION_001"
                }
            }),
        ),

        ("slack", "list_channels") => (
            StatusCode::OK,
            json!({
                "ok": true,
                "channels": [
                    {
                        "id": "C01234567",
                        "name": "general",
                        "is_private": false,
                        "num_members": 42,
                        "topic": { "value": "Company-wide announcements" }
                    },
                    {
                        "id": "C01234568",
                        "name": "engineering",
                        "is_private": false,
                        "num_members": 15,
                        "topic": { "value": "Engineering discussions" }
                    },
                    {
                        "id": "C01234569",
                        "name": "alerts",
                        "is_private": true,
                        "num_members": 8,
                        "topic": { "value": "System alerts and monitoring" }
                    }
                ]
            }),
        ),

        // ── Unknown integration or action ──────────────────────
        (integration, action) => (
            StatusCode::NOT_FOUND,
            json!({
                "error": format!("Unknown action '{}' for integration '{}'", action, integration),
                "integration": integration,
                "available_actions": get_available_actions(integration)
            }),
        ),
    }
}

fn get_available_actions(integration: &str) -> Vec<&'static str> {
    match integration {
        "github" => vec!["create_pull_request", "list_issues", "get_repository"],
        "mailchimp" => vec!["create_campaign", "list_subscribers"],
        "slack" => vec!["send_message", "list_channels"],
        _ => vec![],
    }
}
