#![allow(dead_code, unused_imports, unused_mut, unused_variables)]
use bridge_e2e::{SseStream, TestHarness};
use std::time::Duration;

/// Timeout for mock LLM conversations (fast, deterministic).
const TIMEOUT: Duration = Duration::from_secs(30);

// ============================================================================
// Agent loading with integrations
// ============================================================================

#[tokio::test]
async fn test_agent_with_integrations_loads() {
    let harness = TestHarness::start()
        .await
        .expect("failed to start test harness");

    let agents = harness.get_agents().await.expect("get_agents failed");
    let agent_ids: Vec<&str> = agents
        .iter()
        .filter_map(|a| a.get("id").and_then(|v| v.as_str()))
        .collect();

    assert!(
        agent_ids.contains(&"agent_integrations"),
        "should contain agent_integrations; got {:?}",
        agent_ids
    );
}

#[tokio::test]
async fn test_agent_with_integrations_creates_conversation() {
    let harness = TestHarness::start()
        .await
        .expect("failed to start test harness");

    let resp = harness
        .create_conversation("agent_integrations")
        .await
        .expect("create_conversation request failed");

    assert_eq!(
        resp.status().as_u16(),
        201,
        "POST /agents/agent_integrations/conversations should return 201"
    );

    let body: serde_json::Value = resp
        .json()
        .await
        .expect("failed to parse conversation body");

    assert!(
        body.get("conversation_id").is_some(),
        "response should contain conversation_id"
    );
}

#[tokio::test]
async fn test_integration_tool_allow_executes_immediately() {
    let harness = TestHarness::start()
        .await
        .expect("failed to start test harness");

    let turn = harness
        .converse(
            "agent_integrations",
            None,
            "use_integration:github:list_issues",
            TIMEOUT,
        )
        .await
        .expect("conversation failed");

    // Should have tool_call_start for the integration tool
    let tool_starts: Vec<&str> = turn
        .sse_events
        .iter()
        .filter(|e| e.event_type == "tool_call_start")
        .filter_map(|e| e.data.get("name").and_then(|n| n.as_str()))
        .collect();

    assert!(
        tool_starts.contains(&"github__list_issues"),
        "expected github__list_issues tool call, got {:?}",
        tool_starts
    );

    // Should NOT have any approval events
    let has_approval = turn
        .sse_events
        .iter()
        .any(|e| e.event_type == "tool_approval_required");
    assert!(
        !has_approval,
        "allowed integration tool should NOT require approval"
    );

    // Should have tool_call_result with realistic issues data
    let tool_results: Vec<&str> = turn
        .sse_events
        .iter()
        .filter(|e| e.event_type == "tool_call_result")
        .filter_map(|e| e.data.get("result").and_then(|r| r.as_str()))
        .collect();

    assert!(
        !tool_results.is_empty(),
        "should have at least one tool_call_result"
    );

    // Verify the result contains realistic GitHub issues data
    let has_issues_data = tool_results.iter().any(|r| {
        r.contains("Fix login page crash") || r.contains("list_issues") || r.contains("number")
    });
    assert!(
        has_issues_data,
        "tool result should contain realistic issues data; got: {:?}",
        tool_results
    );

    // Should complete with done
    assert!(
        turn.sse_events.iter().any(|e| e.event_type == "done"),
        "expected done event"
    );
}

#[tokio::test]
async fn test_integration_tool_allow_mailchimp() {
    let harness = TestHarness::start()
        .await
        .expect("failed to start test harness");

    let turn = harness
        .converse(
            "agent_integrations",
            None,
            "use_integration:mailchimp:create_campaign",
            TIMEOUT,
        )
        .await
        .expect("conversation failed");

    let tool_starts: Vec<&str> = turn
        .sse_events
        .iter()
        .filter(|e| e.event_type == "tool_call_start")
        .filter_map(|e| e.data.get("name").and_then(|n| n.as_str()))
        .collect();

    assert!(
        tool_starts.contains(&"mailchimp__create_campaign"),
        "expected mailchimp__create_campaign tool call, got {:?}",
        tool_starts
    );

    // No approval required for allow-permission action
    let has_approval = turn
        .sse_events
        .iter()
        .any(|e| e.event_type == "tool_approval_required");
    assert!(!has_approval, "allowed tool should NOT require approval");

    // Verify result contains campaign data
    let tool_results: Vec<&str> = turn
        .sse_events
        .iter()
        .filter(|e| e.event_type == "tool_call_result")
        .filter_map(|e| e.data.get("result").and_then(|r| r.as_str()))
        .collect();

    let has_campaign_data = tool_results
        .iter()
        .any(|r| r.contains("mc_campaign") || r.contains("campaign") || r.contains("subject"));
    assert!(
        has_campaign_data,
        "tool result should contain campaign data; got: {:?}",
        tool_results
    );

    assert!(turn.sse_events.iter().any(|e| e.event_type == "done"));
}

#[tokio::test]
async fn test_integration_tool_require_approval_approve() {
    let harness = TestHarness::start()
        .await
        .expect("failed to start test harness");

    let resp = harness
        .create_conversation("agent_integrations")
        .await
        .expect("create conversation failed");
    let body: serde_json::Value = resp.json().await.expect("parse response");
    let conv_id = body["conversation_id"]
        .as_str()
        .expect("no conversation_id")
        .to_string();

    // Connect SSE before sending message
    let bridge_url = harness.bridge_url();
    let stream = SseStream::connect(bridge_url, &conv_id)
        .await
        .expect("SSE connect failed");

    // Send message that triggers require_approval integration tool
    let msg_resp = harness
        .send_message(&conv_id, "use_integration:github:create_pull_request")
        .await
        .expect("send message failed");
    assert!(msg_resp.status().is_success() || msg_resp.status().as_u16() == 202);

    // Wait for tool_approval_required
    let approval_event = stream
        .wait_for_event("tool_approval_required", TIMEOUT)
        .await
        .expect("expected tool_approval_required SSE event");

    let request_id = approval_event.data["request_id"]
        .as_str()
        .expect("no request_id")
        .to_string();

    // Verify integration metadata in approval event
    assert_eq!(
        approval_event.data["tool_name"].as_str(),
        Some("github__create_pull_request"),
        "tool_name should be github__create_pull_request"
    );
    assert_eq!(
        approval_event.data["integration_name"].as_str(),
        Some("github"),
        "integration_name should be present"
    );
    assert_eq!(
        approval_event.data["integration_action"].as_str(),
        Some("create_pull_request"),
        "integration_action should be present"
    );

    // List pending approvals
    let pending = harness
        .list_approvals("agent_integrations", &conv_id)
        .await
        .expect("list approvals failed");
    assert!(
        !pending.is_empty(),
        "should have at least one pending approval"
    );

    // Approve
    let approve_resp = harness
        .resolve_approval("agent_integrations", &conv_id, &request_id, "approve")
        .await
        .expect("resolve approval failed");
    assert!(approve_resp.status().is_success());

    // Wait for done
    let events = stream.wait_for_done(TIMEOUT).await;
    assert!(
        events.iter().any(|e| e.event_type == "done"),
        "expected done event"
    );

    // Verify tool executed — should have tool_call_result with PR data
    let has_tool_result = events.iter().any(|e| {
        e.event_type == "tool_call_result"
            && e.data
                .get("result")
                .and_then(|r| r.as_str())
                .map(|r| r.contains("pull") || r.contains("123") || r.contains("open"))
                .unwrap_or(false)
    });
    assert!(
        has_tool_result,
        "expected tool_call_result with PR data after approval"
    );
}
