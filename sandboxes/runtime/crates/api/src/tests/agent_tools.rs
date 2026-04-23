use axum::body::Body;
use axum::Router;
use http::Request;
use tower::ServiceExt;

use crate::router::build_router;
use crate::tests::{body_bytes, body_json, test_state};

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
    assert!(tool_names.contains(&"RipGrep"), "should have RipGrep tool");
    assert!(tool_names.contains(&"AstGrep"), "should have AstGrep tool");
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
    assert!(
        reg_names.contains(&"RipGrep"),
        "subagent should have RipGrep"
    );
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
