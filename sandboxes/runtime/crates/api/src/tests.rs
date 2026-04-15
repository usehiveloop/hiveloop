use axum::body::Body;
use axum::middleware as axum_mw;
use axum::Router;
use http::Request;
use mcp::McpManager;
use runtime::AgentSupervisor;
use std::sync::Arc;
use tokio_util::sync::CancellationToken;
use tower::ServiceExt;
use webhooks::EventBus;

use crate::middleware::request_id;
use crate::router::build_router;
use crate::state::AppState;

/// Build a test `AppState` backed by a real (but empty) `AgentSupervisor`.
fn test_state() -> AppState {
    let mcp_manager = Arc::new(McpManager::new());
    let cancel = CancellationToken::new();
    let event_bus = Arc::new(EventBus::new(None, None, String::new(), String::new()));
    let supervisor = Arc::new(
        AgentSupervisor::new(mcp_manager, cancel.clone()).with_event_bus(Some(event_bus.clone())),
    );
    AppState::new(
        supervisor,
        "valid-control-plane-token".to_string(),
        None,
        cancel,
        event_bus,
    )
}

/// Build the application router with the request-id middleware applied,
/// using the given `AppState`.
fn app_with_request_id(state: AppState) -> Router {
    build_router(state).layer(axum_mw::from_fn(request_id))
}

/// Helper: read the full response body as bytes.
async fn body_bytes(response: axum::response::Response) -> Vec<u8> {
    axum::body::to_bytes(response.into_body(), usize::MAX)
        .await
        .expect("failed to read body")
        .to_vec()
}

/// Helper: read the full response body as a `serde_json::Value`.
async fn body_json(response: axum::response::Response) -> serde_json::Value {
    let bytes = body_bytes(response).await;
    serde_json::from_slice(&bytes).expect("body is not valid JSON")
}

// ── 1. GET /health → 200, body has "status": "ok" and "uptime_secs" ─────────

#[tokio::test]
async fn health_returns_200_with_status_ok_and_uptime() {
    let app = build_router(test_state());

    let response = app
        .oneshot(
            Request::builder()
                .uri("/health")
                .body(Body::empty())
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), 200);

    let json = body_json(response).await;
    assert_eq!(json["status"], "ok");
    assert!(
        json["uptime_secs"].is_number(),
        "uptime_secs should be a number"
    );
}

// ── 2. GET /agents → 200, returns JSON array (empty) ────────────────────────

#[tokio::test]
async fn list_agents_returns_empty_array() {
    let app = build_router(test_state());

    let response = app
        .oneshot(
            Request::builder()
                .uri("/agents")
                .body(Body::empty())
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), 200);

    let json = body_json(response).await;
    assert!(json.is_array(), "response should be an array");
    assert_eq!(json.as_array().unwrap().len(), 0, "array should be empty");
}

// ── 3. GET /agents/unknown → 404 ────────────────────────────────────────────

#[tokio::test]
async fn get_unknown_agent_returns_404() {
    let app = build_router(test_state());

    let response = app
        .oneshot(
            Request::builder()
                .uri("/agents/unknown")
                .body(Body::empty())
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), 404);

    let json = body_json(response).await;
    assert_eq!(json["error"]["code"], "agent_not_found");
}

// ── 4. POST /agents/unknown/conversations → error (agent not found) ─────────

#[tokio::test]
async fn create_conversation_for_unknown_agent_returns_error() {
    let app = build_router(test_state());

    let response = app
        .oneshot(
            Request::builder()
                .method("POST")
                .uri("/agents/unknown/conversations")
                .body(Body::empty())
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), 404);

    let json = body_json(response).await;
    assert_eq!(json["error"]["code"], "agent_not_found");
    assert!(
        json["error"]["message"]
            .as_str()
            .unwrap()
            .contains("unknown"),
        "error message should contain the agent id"
    );
}

// ── 5. POST /conversations/unknown/messages → error ─────────────────────────

#[tokio::test]
async fn send_message_to_unknown_conversation_returns_error() {
    let app = build_router(test_state());

    let response = app
        .oneshot(
            Request::builder()
                .method("POST")
                .uri("/conversations/unknown/messages")
                .header("content-type", "application/json")
                .body(Body::from(r#"{"content":"hello"}"#))
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), 404);

    let json = body_json(response).await;
    assert_eq!(json["error"]["code"], "conversation_not_found");
}

// ── 6. DELETE /conversations/unknown → error ────────────────────────────────

#[tokio::test]
async fn end_unknown_conversation_returns_error() {
    let app = build_router(test_state());

    let response = app
        .oneshot(
            Request::builder()
                .method("DELETE")
                .uri("/conversations/unknown")
                .body(Body::empty())
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), 404);

    let json = body_json(response).await;
    assert_eq!(json["error"]["code"], "conversation_not_found");
}

// ── 7. GET /metrics → 200, returns valid MetricsResponse JSON ───────────────

#[tokio::test]
async fn metrics_returns_valid_json() {
    let app = build_router(test_state());

    let response = app
        .oneshot(
            Request::builder()
                .uri("/metrics")
                .body(Body::empty())
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), 200);

    let json = body_json(response).await;

    // Verify top-level structure
    assert!(
        json["timestamp"].is_string(),
        "timestamp should be a string"
    );
    assert!(json["agents"].is_array(), "agents should be an array");
    assert!(json["global"].is_object(), "global should be an object");

    // Verify global metrics
    let global = &json["global"];
    assert_eq!(global["total_agents"], 0);
    assert_eq!(global["total_active_conversations"], 0);
    assert!(
        global["uptime_secs"].is_number(),
        "uptime_secs should be a number"
    );

    // With no agents loaded, agents array should be empty
    assert_eq!(json["agents"].as_array().unwrap().len(), 0);
}

// ── 8. Request ID middleware adds X-Request-ID header ────────────────────────

#[tokio::test]
async fn request_id_middleware_generates_id_when_absent() {
    let app = app_with_request_id(test_state());

    let response = app
        .oneshot(
            Request::builder()
                .uri("/health")
                .body(Body::empty())
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), 200);

    let header = response
        .headers()
        .get("x-request-id")
        .expect("x-request-id header should be present");

    let value = header.to_str().unwrap();
    assert!(!value.is_empty(), "x-request-id should not be empty");

    // Verify the generated value looks like a UUID (36 chars with hyphens)
    assert_eq!(value.len(), 36, "x-request-id should be a UUID");
}

#[tokio::test]
async fn request_id_middleware_preserves_existing_id() {
    let app = app_with_request_id(test_state());

    let custom_id = "my-custom-request-id-12345";

    let response = app
        .oneshot(
            Request::builder()
                .uri("/health")
                .header("x-request-id", custom_id)
                .body(Body::empty())
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), 200);

    let header = response
        .headers()
        .get("x-request-id")
        .expect("x-request-id header should be present");

    assert_eq!(header.to_str().unwrap(), custom_id);
}

// ── 9. Error responses have correct JSON structure ──────────────────────────

#[tokio::test]
async fn error_response_has_correct_json_structure() {
    let app = build_router(test_state());

    // Use GET /agents/nonexistent to trigger a 404 error response
    let response = app
        .oneshot(
            Request::builder()
                .uri("/agents/nonexistent")
                .body(Body::empty())
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), 404);

    let json = body_json(response).await;

    // Top-level must have "error" object
    assert!(
        json["error"].is_object(),
        "response must have an 'error' object"
    );

    // "error" object must have "code" and "message" fields
    let error = &json["error"];
    assert!(error["code"].is_string(), "error.code should be a string");
    assert!(
        error["message"].is_string(),
        "error.message should be a string"
    );

    // Verify specific values for this case
    assert_eq!(error["code"], "agent_not_found");
    assert!(
        error["message"].as_str().unwrap().contains("nonexistent"),
        "error message should reference the missing agent ID"
    );
}

#[tokio::test]
async fn conversation_not_found_error_has_correct_structure() {
    let app = build_router(test_state());

    let response = app
        .oneshot(
            Request::builder()
                .method("DELETE")
                .uri("/conversations/does-not-exist")
                .body(Body::empty())
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), 404);

    let json = body_json(response).await;

    let error = &json["error"];
    assert_eq!(error["code"], "conversation_not_found");
    assert!(
        error["message"]
            .as_str()
            .unwrap()
            .contains("does-not-exist"),
        "error message should reference the missing conversation ID"
    );
}

// ── 10. Push endpoint auth tests ─────────────────────────────────────────────

#[tokio::test]
async fn push_without_auth_returns_401() {
    let app = build_router(test_state());

    let response = app
        .oneshot(
            Request::builder()
                .method("POST")
                .uri("/push/agents")
                .header("content-type", "application/json")
                .body(Body::from(r#"{"agents":[]}"#))
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), 401);

    let json = body_json(response).await;
    assert_eq!(json["error"]["code"], "unauthorized");
}

#[tokio::test]
async fn push_with_wrong_token_returns_401() {
    let app = build_router(test_state());

    let response = app
        .oneshot(
            Request::builder()
                .method("POST")
                .uri("/push/agents")
                .header("content-type", "application/json")
                .header("authorization", "Bearer invalid-control-plane-token")
                .body(Body::from(r#"{"agents":[]}"#))
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), 401);

    let json = body_json(response).await;
    assert_eq!(json["error"]["code"], "unauthorized");
}

#[tokio::test]
async fn push_with_correct_token_succeeds() {
    let app = build_router(test_state());

    let response = app
        .oneshot(
            Request::builder()
                .method("POST")
                .uri("/push/agents")
                .header("content-type", "application/json")
                .header("authorization", "Bearer valid-control-plane-token")
                .body(Body::from(r#"{"agents":[]}"#))
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), 200);

    let json = body_json(response).await;
    assert_eq!(json["loaded"], 0);
}

// ── 11. Push endpoint validation tests ───────────────────────────────────────

#[tokio::test]
async fn upsert_agent_path_body_mismatch_returns_400() {
    let app = build_router(test_state());

    let body = serde_json::json!({
        "id": "bar",
        "name": "Test",
        "system_prompt": "test",
        "provider": {
            "provider_type": "anthropic",
            "model": "claude-sonnet-4-20250514",
            "api_key": "<provider-api-key>"
        }
    });

    let response = app
        .oneshot(
            Request::builder()
                .method("PUT")
                .uri("/push/agents/foo")
                .header("content-type", "application/json")
                .header("authorization", "Bearer valid-control-plane-token")
                .body(Body::from(serde_json::to_string(&body).unwrap()))
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), 400);

    let json = body_json(response).await;
    assert_eq!(json["error"]["code"], "invalid_request");
}

#[tokio::test]
async fn remove_nonexistent_agent_returns_404() {
    let app = build_router(test_state());

    let response = app
        .oneshot(
            Request::builder()
                .method("DELETE")
                .uri("/push/agents/unknown")
                .header("authorization", "Bearer valid-control-plane-token")
                .body(Body::empty())
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), 404);

    let json = body_json(response).await;
    assert_eq!(json["error"]["code"], "agent_not_found");
}

#[tokio::test]
async fn hydrate_unknown_agent_returns_404() {
    let app = build_router(test_state());

    let response = app
        .oneshot(
            Request::builder()
                .method("POST")
                .uri("/push/agents/unknown/conversations")
                .header("content-type", "application/json")
                .header("authorization", "Bearer valid-control-plane-token")
                .body(Body::from(r#"{"conversations":[]}"#))
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), 404);

    let json = body_json(response).await;
    assert_eq!(json["error"]["code"], "agent_not_found");
}

#[tokio::test]
async fn push_diff_empty_succeeds() {
    let app = build_router(test_state());

    let response = app
        .oneshot(
            Request::builder()
                .method("POST")
                .uri("/push/diff")
                .header("content-type", "application/json")
                .header("authorization", "Bearer valid-control-plane-token")
                .body(Body::from(r#"{"added":[],"updated":[],"removed":[]}"#))
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), 200);

    let json = body_json(response).await;
    assert_eq!(json["added"], 0);
    assert_eq!(json["updated"], 0);
    assert_eq!(json["removed"], 0);
}

// ── Conversation tool/MCP scoping tests ───────────────────────────────────

/// Helper: push a minimal test agent and return its ID.
async fn push_test_agent(app: &Router, agent_id: &str) {
    let agent_def = serde_json::json!({
        "agents": [{
            "id": agent_id,
            "name": "Test Agent",
            "system_prompt": "You are a test agent.",
            "provider": {
                "provider_type": "open_ai",
                "model": "gpt-4o",
                "api_key": "test-key",
                "base_url": "https://api.openai.com/v1"
            }
        }]
    });

    let response = app
        .clone()
        .oneshot(
            Request::builder()
                .method("POST")
                .uri("/push/agents")
                .header("content-type", "application/json")
                .header("authorization", "Bearer valid-control-plane-token")
                .body(Body::from(serde_json::to_string(&agent_def).unwrap()))
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), 200, "push agent should succeed");
}

#[tokio::test]
async fn create_conversation_no_body_succeeds() {
    let app = build_router(test_state());
    push_test_agent(&app, "scoping-agent").await;

    let response = app
        .oneshot(
            Request::builder()
                .method("POST")
                .uri("/agents/scoping-agent/conversations")
                .body(Body::empty())
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), 201);
    let json = body_json(response).await;
    assert!(
        json["conversation_id"].is_string(),
        "should return conversation_id"
    );
    assert!(json["stream_url"].is_string(), "should return stream_url");
}

#[tokio::test]
async fn create_conversation_with_valid_tool_names_succeeds() {
    let app = build_router(test_state());
    push_test_agent(&app, "scoping-agent-2").await;

    // Agent has builtin tools because tools: [] in definition means all builtins.
    // "bash" and "Read" are always registered as builtins.
    let body = serde_json::json!({
        "tool_names": ["bash", "Read"]
    });

    let response = app
        .oneshot(
            Request::builder()
                .method("POST")
                .uri("/agents/scoping-agent-2/conversations")
                .header("content-type", "application/json")
                .body(Body::from(serde_json::to_string(&body).unwrap()))
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), 201);
    let json = body_json(response).await;
    assert!(json["conversation_id"].is_string());
}

#[tokio::test]
async fn create_conversation_with_invalid_tool_name_returns_400() {
    let app = build_router(test_state());
    push_test_agent(&app, "scoping-agent-3").await;

    let body = serde_json::json!({
        "tool_names": ["bash", "totally_fake_tool"]
    });

    let response = app
        .oneshot(
            Request::builder()
                .method("POST")
                .uri("/agents/scoping-agent-3/conversations")
                .header("content-type", "application/json")
                .body(Body::from(serde_json::to_string(&body).unwrap()))
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), 400);
    let json = body_json(response).await;
    assert_eq!(json["error"]["code"], "invalid_request");
    assert!(
        json["error"]["message"]
            .as_str()
            .unwrap()
            .contains("totally_fake_tool"),
        "error should name the invalid tool"
    );
}

#[tokio::test]
async fn create_conversation_with_invalid_mcp_server_returns_400() {
    let app = build_router(test_state());
    push_test_agent(&app, "scoping-agent-4").await;

    let body = serde_json::json!({
        "mcp_server_names": ["nonexistent-server"]
    });

    let response = app
        .oneshot(
            Request::builder()
                .method("POST")
                .uri("/agents/scoping-agent-4/conversations")
                .header("content-type", "application/json")
                .body(Body::from(serde_json::to_string(&body).unwrap()))
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), 400);
    let json = body_json(response).await;
    assert_eq!(json["error"]["code"], "invalid_request");
    assert!(
        json["error"]["message"]
            .as_str()
            .unwrap()
            .contains("nonexistent-server"),
        "error should name the invalid MCP server"
    );
}

#[tokio::test]
async fn create_conversation_with_empty_json_body_succeeds() {
    let app = build_router(test_state());
    push_test_agent(&app, "scoping-agent-5").await;

    // Empty JSON object = both filters are None = all tools
    let response = app
        .oneshot(
            Request::builder()
                .method("POST")
                .uri("/agents/scoping-agent-5/conversations")
                .header("content-type", "application/json")
                .body(Body::from("{}"))
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), 201);
}

#[tokio::test]
async fn create_conversation_with_empty_tool_names_array_succeeds() {
    let app = build_router(test_state());
    push_test_agent(&app, "scoping-agent-6").await;

    // Empty array means zero tools — should succeed (agent has no tools)
    let body = serde_json::json!({
        "tool_names": []
    });

    let response = app
        .oneshot(
            Request::builder()
                .method("POST")
                .uri("/agents/scoping-agent-6/conversations")
                .header("content-type", "application/json")
                .body(Body::from(serde_json::to_string(&body).unwrap()))
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), 201);
}

// ── Agent and subagent tool reporting tests ─────────────────────────────────

/// Helper: push an agent with subagents and return the agent detail response.
async fn push_agent_with_subagents(app: &Router) -> serde_json::Value {
    let agent_def = serde_json::json!({
        "agents": [{
            "id": "parent-agent",
            "name": "Parent Agent",
            "system_prompt": "You are a parent agent.",
            "provider": {
                "provider_type": "open_ai",
                "model": "gpt-4o",
                "api_key": "test-key",
                "base_url": "https://api.openai.com/v1"
            },
            "subagents": [
                {
                    "id": "explorer-sub",
                    "name": "explorer",
                    "system_prompt": "You are an explorer subagent.",
                    "provider": {
                        "provider_type": "open_ai",
                        "model": "gpt-4o-mini",
                        "api_key": "test-key",
                        "base_url": "https://api.openai.com/v1"
                    }
                },
                {
                    "id": "coder-sub",
                    "name": "coder",
                    "system_prompt": "You are a coder subagent.",
                    "provider": {
                        "provider_type": "open_ai",
                        "model": "gpt-4o",
                        "api_key": "test-key-2",
                        "base_url": "https://api.openai.com/v1"
                    },
                    "tools": [
                        { "name": "bash", "description": "Run shell commands", "parameters_schema": {} },
                        { "name": "Read", "description": "Read files", "parameters_schema": {} }
                    ]
                }
            ]
        }]
    });

    let response = app
        .clone()
        .oneshot(
            Request::builder()
                .method("POST")
                .uri("/push/agents")
                .header("content-type", "application/json")
                .header("authorization", "Bearer valid-control-plane-token")
                .body(Body::from(serde_json::to_string(&agent_def).unwrap()))
                .unwrap(),
        )
        .await
        .unwrap();
    if response.status() != 200 {
        let status = response.status();
        let bytes = body_bytes(response).await;
        let body_text = String::from_utf8_lossy(&bytes);
        panic!("push agent failed with status {}: {}", status, body_text);
    }

    // Fetch agent detail
    let response = app
        .clone()
        .oneshot(
            Request::builder()
                .uri("/agents/parent-agent")
                .body(Body::empty())
                .unwrap(),
        )
        .await
        .unwrap();
    assert_eq!(response.status(), 200);
    body_json(response).await
}

#[tokio::test]
async fn parent_agent_has_registered_tools() {
    let app = build_router(test_state());
    let json = push_agent_with_subagents(&app).await;

    // Parent agent should have registered_tools populated from the runtime tool registry
    let registered_tools = json["registered_tools"]
        .as_array()
        .expect("registered_tools should be an array");
    assert!(
        !registered_tools.is_empty(),
        "parent agent should have registered tools, got empty array"
    );

    // Should contain core built-in tools
    let tool_names: Vec<&str> = registered_tools
        .iter()
        .filter_map(|t| t["name"].as_str())
        .collect();
    assert!(tool_names.contains(&"bash"), "should have bash tool");
    assert!(tool_names.contains(&"Read"), "should have Read tool");
    assert!(tool_names.contains(&"Grep"), "should have Grep tool");
    assert!(tool_names.contains(&"Glob"), "should have Glob tool");
    assert!(tool_names.contains(&"edit"), "should have edit tool");
    assert!(tool_names.contains(&"write"), "should have write tool");
}

#[tokio::test]
async fn parent_agent_has_subagents_listed() {
    let app = build_router(test_state());
    let json = push_agent_with_subagents(&app).await;

    let subagents = json["subagents"]
        .as_array()
        .expect("subagents should be an array");
    assert_eq!(subagents.len(), 2, "should have 2 subagents");

    let names: Vec<&str> = subagents
        .iter()
        .filter_map(|s| s["name"].as_str())
        .collect();
    assert!(names.contains(&"explorer"), "should have explorer subagent");
    assert!(names.contains(&"coder"), "should have coder subagent");
}

#[tokio::test]
async fn subagent_with_no_tools_field_reports_empty_tools_in_api() {
    // This test documents the current behavior: the API returns the definition's
    // tools field (allow-list), NOT the runtime-registered tools.
    // The explorer subagent has no tools field, so the API shows empty tools.
    let app = build_router(test_state());
    let json = push_agent_with_subagents(&app).await;

    let subagents = json["subagents"].as_array().unwrap();
    let explorer = subagents
        .iter()
        .find(|s| s["name"] == "explorer")
        .expect("explorer subagent");

    let tools = explorer["tools"]
        .as_array()
        .expect("tools should be an array");
    // The definition's tools field is empty (no custom tool definitions)
    assert!(
        tools.is_empty(),
        "explorer definition tools should be empty (no custom tool definitions)"
    );

    // But registered_tools should show the actual runtime tools
    let registered = explorer["registered_tools"]
        .as_array()
        .expect("registered_tools should be an array");
    assert!(
        !registered.is_empty(),
        "explorer should have registered runtime tools"
    );
    let reg_names: Vec<&str> = registered
        .iter()
        .filter_map(|t| t["name"].as_str())
        .collect();
    assert!(reg_names.contains(&"bash"), "subagent should have bash");
    assert!(reg_names.contains(&"Read"), "subagent should have Read");
    assert!(reg_names.contains(&"Grep"), "subagent should have Grep");
    assert!(reg_names.contains(&"edit"), "subagent should have edit");
    // Subagents should NOT have agent orchestration tools
    assert!(
        !reg_names.contains(&"agent"),
        "subagent should NOT have agent tool"
    );
    assert!(
        !reg_names.contains(&"sub_agent"),
        "subagent should NOT have sub_agent tool"
    );
    assert!(
        !reg_names.contains(&"parallel_agent"),
        "subagent should NOT have parallel_agent tool"
    );
    // Subagents should NOT have ping-me-back tools
    assert!(
        !reg_names.contains(&"ping_me_back_in"),
        "subagent should NOT have ping_me_back_in tool"
    );
    assert!(
        !reg_names.contains(&"cancel_ping_me_back"),
        "subagent should NOT have cancel_ping_me_back tool"
    );
}

#[tokio::test]
async fn subagent_with_tools_field_reports_only_those_tools_in_api() {
    // The coder subagent specifies tools: ["bash", "Read"]
    // The API should report exactly those tools from the definition.
    let app = build_router(test_state());
    let json = push_agent_with_subagents(&app).await;

    let subagents = json["subagents"].as_array().unwrap();
    let coder = subagents
        .iter()
        .find(|s| s["name"] == "coder")
        .expect("coder subagent");

    let tools = coder["tools"].as_array().expect("tools should be an array");
    let tool_names: Vec<&str> = tools.iter().filter_map(|t| t["name"].as_str()).collect();

    assert_eq!(
        tool_names.len(),
        2,
        "coder should have 2 tools in definition"
    );
    assert!(tool_names.contains(&"bash"), "should have bash");
    assert!(tool_names.contains(&"Read"), "should have Read");
}

#[tokio::test]
async fn subagent_api_response_has_no_registered_tools_field() {
    // BUG DETECTION: SubAgentSummary does not have a registered_tools field,
    // unlike AgentResponse which does. There's no way to see the actual runtime
    // tools a subagent has via the API.
    let app = build_router(test_state());
    let json = push_agent_with_subagents(&app).await;

    let subagents = json["subagents"].as_array().unwrap();
    let explorer = subagents
        .iter()
        .find(|s| s["name"] == "explorer")
        .expect("explorer subagent");

    // registered_tools should now be present on subagent summaries
    assert!(
        explorer.get("registered_tools").is_some(),
        "subagent summary should have registered_tools field"
    );
    let registered = explorer["registered_tools"].as_array().unwrap();
    assert!(
        !registered.is_empty(),
        "subagent registered_tools should not be empty"
    );
}

#[tokio::test]
async fn parent_registered_tools_includes_agent_orchestration_tools() {
    let app = build_router(test_state());
    let json = push_agent_with_subagents(&app).await;

    let registered_tools = json["registered_tools"].as_array().unwrap();
    let tool_names: Vec<&str> = registered_tools
        .iter()
        .filter_map(|t| t["name"].as_str())
        .collect();

    assert!(
        tool_names.contains(&"agent"),
        "parent should have agent tool"
    );
    assert!(
        tool_names.contains(&"sub_agent"),
        "parent should have sub_agent tool"
    );
    assert!(
        tool_names.contains(&"parallel_agent"),
        "parent should have parallel_agent tool"
    );
}

#[tokio::test]
async fn parent_registered_tools_includes_ping_me_back() {
    let app = build_router(test_state());
    let json = push_agent_with_subagents(&app).await;

    let registered_tools = json["registered_tools"].as_array().unwrap();
    let tool_names: Vec<&str> = registered_tools
        .iter()
        .filter_map(|t| t["name"].as_str())
        .collect();

    assert!(
        tool_names.contains(&"ping_me_back_in"),
        "should have ping_me_back_in tool"
    );
    assert!(
        tool_names.contains(&"cancel_ping_me_back"),
        "should have cancel_ping_me_back tool"
    );
}
