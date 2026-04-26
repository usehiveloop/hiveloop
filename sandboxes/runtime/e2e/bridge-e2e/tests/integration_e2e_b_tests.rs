#![allow(dead_code, unused_imports, unused_mut, unused_variables)]
use bridge_e2e::{SseStream, TestHarness};
use std::time::Duration;

/// Timeout for mock LLM conversations (fast, deterministic).
const TIMEOUT: Duration = Duration::from_secs(30);

// ============================================================================
// Agent loading with integrations
// ============================================================================

#[tokio::test]
async fn test_integration_tool_require_approval_deny() {
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

    let bridge_url = harness.bridge_url();
    let stream = SseStream::connect(bridge_url, &conv_id)
        .await
        .expect("SSE connect failed");

    let msg_resp = harness
        .send_message(&conv_id, "use_integration:slack:send_message")
        .await
        .expect("send message failed");
    assert!(msg_resp.status().is_success() || msg_resp.status().as_u16() == 202);

    // Wait for approval
    let approval_event = stream
        .wait_for_event("tool_approval_required", TIMEOUT)
        .await
        .expect("expected tool_approval_required");

    let request_id = approval_event.data["request_id"]
        .as_str()
        .expect("no request_id")
        .to_string();

    // Deny
    let deny_resp = harness
        .resolve_approval("agent_integrations", &conv_id, &request_id, "deny")
        .await
        .expect("deny approval failed");
    assert!(deny_resp.status().is_success());

    // Wait for done
    let events = stream.wait_for_done(TIMEOUT).await;
    assert!(
        events.iter().any(|e| e.event_type == "done"),
        "expected done event"
    );

    // Should have a denial error in tool_call_result
    let has_denial = events.iter().any(|e| {
        e.event_type == "tool_call_result"
            && e.data.get("is_error").and_then(|v| v.as_bool()) == Some(true)
    });
    assert!(has_denial, "expected tool_call_result with denial error");
}

#[tokio::test]
async fn test_integration_webhooks() {
    let harness = TestHarness::start()
        .await
        .expect("failed to start test harness");

    // Clear prior webhooks
    harness
        .clear_webhook_log()
        .await
        .expect("failed to clear webhook log");

    // Use an allowed integration tool
    let turn = harness
        .converse(
            "agent_integrations",
            None,
            "use_integration:github:list_issues",
            TIMEOUT,
        )
        .await
        .expect("conversation failed");

    // Verify conversation completed
    assert!(turn.sse_events.iter().any(|e| e.event_type == "done"));

    // Wait for webhooks
    let log = harness
        .wait_for_webhooks(4, Duration::from_secs(10))
        .await
        .expect("wait_for_webhooks failed");

    // Should have standard lifecycle events
    log.assert_has_type("conversation_created");
    log.assert_has_type("message_received");

    // Should have HMAC signature
    log.assert_has_signature_header();

    // All webhooks should have agent_id and conversation_id
    log.assert_all_have_agent_id();
    log.assert_all_have_conversation_id();

    // Verify agent_id matches
    let created = log.by_type("conversation_created");
    assert_eq!(created[0].agent_id(), Some("agent_integrations"));
}

#[tokio::test]
async fn test_integration_agent_also_has_builtin_tools() {
    let harness = TestHarness::start()
        .await
        .expect("failed to start test harness");

    // The integration agent has no explicit tools list, so it gets all builtins
    // plus its integration tools. Verify the agent loads and can converse.
    let turn = harness
        .converse(
            "agent_integrations",
            None,
            "Hello, just respond with text",
            TIMEOUT,
        )
        .await
        .expect("text conversation failed");

    assert!(
        turn.sse_events.iter().any(|e| e.event_type == "done"),
        "text-only conversation should complete"
    );
}
