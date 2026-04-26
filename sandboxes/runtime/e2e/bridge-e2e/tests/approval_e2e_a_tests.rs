#![allow(dead_code, unused_imports, unused_mut, unused_variables)]
//! E2E tests for the tool approval flow with a real LLM (Fireworks).
//!
//! These tests are `#[ignore]` by default — run them with:
//!
//! ```sh
//! FIREWORKS_API_KEY=<key> cargo test -p bridge-e2e --test approval_e2e_tests -- --ignored
//! ```

use bridge_e2e::{check, check_eq, step, SseStream, TestHarness};
use std::time::Duration;

/// Timeout for waiting for the LLM to request a tool call.
const TOOL_CALL_TIMEOUT: Duration = Duration::from_secs(120);

/// Timeout for the full conversation after approval is resolved.
const FULL_TIMEOUT: Duration = Duration::from_secs(300);

const AGENT_ID: &str = "approval-agent";

fn require_fireworks_key() -> bool {
    if std::env::var("FIREWORKS_API_KEY").is_err() {
        eprintln!("FIREWORKS_API_KEY not set — skipping approval E2E test");
        return false;
    }
    true
}

/// Helper: create conversation, send message, connect SSE stream.
/// Returns (conv_id, sse_stream).
async fn setup_conversation(harness: &TestHarness, message: &str) -> (String, SseStream) {
    step!("Creating conversation for agent '{}'", AGENT_ID);
    let resp = harness
        .create_conversation(AGENT_ID)
        .await
        .expect("create conversation failed");
    let body: serde_json::Value = resp.json().await.expect("parse create conv response");
    let conv_id = body["conversation_id"]
        .as_str()
        .expect("no conversation_id")
        .to_string();

    step!("Connecting SSE stream for conversation {}", conv_id);
    // Connect to SSE stream BEFORE sending message so we don't miss events
    let bridge_url = format!("http://127.0.0.1:{}", harness.bridge_port);
    let stream = SseStream::connect(&bridge_url, &conv_id)
        .await
        .expect("SSE connect failed");

    step!("Sending message: '{}'", message);
    // Send message
    let msg_resp = harness
        .send_message(&conv_id, message)
        .await
        .expect("send message failed");
    check!(
        msg_resp.status().is_success() || msg_resp.status().as_u16() == 202,
        "send message returned success/202 (got {})",
        msg_resp.status()
    );

    (conv_id, stream)
}

// ============================================================================
// Test 1: Approve a tool call — bash with require_approval
// ============================================================================
#[tokio::test]
#[ignore]
async fn test_approve_tool_call() {
    if !require_fireworks_key() {
        return;
    }

    step!("Starting harness with real LLM");
    let harness = TestHarness::start_real()
        .await
        .expect("failed to start real harness");

    let (conv_id, stream) = setup_conversation(
        &harness,
        "Use the bash tool to run: echo hello_approval_test",
    )
    .await;

    step!("Conversation created: {}", conv_id);

    step!(
        "Waiting for tool_approval_required SSE event (timeout: {:?})",
        TOOL_CALL_TIMEOUT
    );
    // Wait for tool_approval_required
    let approval_event = stream
        .wait_for_event("tool_approval_required", TOOL_CALL_TIMEOUT)
        .await
        .expect("expected tool_approval_required SSE event");

    let request_id = approval_event.data["request_id"]
        .as_str()
        .expect("no request_id")
        .to_string();
    step!(
        "Got tool_approval_required: request_id={}, tool={}",
        request_id,
        approval_event.data["tool_name"].as_str().unwrap_or("?")
    );
    eprintln!(
        "    Approval event data: {}",
        serde_json::to_string_pretty(&approval_event.data).unwrap_or_default()
    );

    step!("Listing pending approvals via API");
    // List pending approvals via API
    let pending = harness
        .list_approvals(AGENT_ID, &conv_id)
        .await
        .expect("list approvals failed");
    check!(
        !pending.is_empty(),
        "expected at least one pending approval"
    );
    check!(
        pending
            .iter()
            .any(|a| a["id"].as_str() == Some(&request_id)),
        "expected pending approval with id={}",
        request_id
    );

    step!("Approving request {}", request_id);
    // Approve
    let approve_resp = harness
        .resolve_approval(AGENT_ID, &conv_id, &request_id, "approve")
        .await
        .expect("resolve approval failed");
    check!(
        approve_resp.status().is_success(),
        "approve response is success"
    );

    step!("Waiting for done event (timeout: {:?})", FULL_TIMEOUT);
    // Wait for done
    let events = stream.wait_for_done(FULL_TIMEOUT).await;

    step!("Listing SSE events received ({} total)", events.len());
    for e in &events {
        eprintln!("    - {}", e.event_type);
    }

    check!(
        events.iter().any(|e| e.event_type == "done"),
        "expected done event"
    );

    step!("Verifying tool_call_result after approval");
    // Verify tool executed — should have tool_call_result after approval
    let has_tool_result = events.iter().any(|e| e.event_type == "tool_call_result");
    check!(has_tool_result, "expected tool_call_result after approval");

    // Log tool results
    for e in events.iter().filter(|e| e.event_type == "tool_call_result") {
        eprintln!(
            "    tool_call_result data: {}",
            serde_json::to_string_pretty(&e.data).unwrap_or_default()
        );
    }

    step!("PASS — tool call approved and executed successfully");
}

// ============================================================================
// Test 2: Deny a tool call — bash with require_approval
// ============================================================================
#[tokio::test]
#[ignore]
async fn test_deny_tool_call() {
    if !require_fireworks_key() {
        return;
    }

    step!("Starting harness with real LLM");
    let harness = TestHarness::start_real()
        .await
        .expect("failed to start real harness");

    let (conv_id, stream) =
        setup_conversation(&harness, "Use the bash tool to run: echo denied_test").await;

    step!("Conversation created: {}", conv_id);

    step!(
        "Waiting for tool_approval_required SSE event (timeout: {:?})",
        TOOL_CALL_TIMEOUT
    );
    // Wait for tool_approval_required
    let approval_event = stream
        .wait_for_event("tool_approval_required", TOOL_CALL_TIMEOUT)
        .await
        .expect("expected tool_approval_required SSE event");

    let request_id = approval_event.data["request_id"]
        .as_str()
        .expect("no request_id")
        .to_string();
    step!("Got tool_approval_required: request_id={}", request_id);
    eprintln!(
        "    Approval event data: {}",
        serde_json::to_string_pretty(&approval_event.data).unwrap_or_default()
    );

    step!("Denying request {}", request_id);
    // Deny
    let deny_resp = harness
        .resolve_approval(AGENT_ID, &conv_id, &request_id, "deny")
        .await
        .expect("deny approval failed");
    check!(deny_resp.status().is_success(), "deny response is success");

    step!("Waiting for done event (timeout: {:?})", FULL_TIMEOUT);
    // Wait for done — the LLM should respond after receiving the denial
    let events = stream.wait_for_done(FULL_TIMEOUT).await;

    step!("Listing SSE events received ({} total)", events.len());
    for e in &events {
        eprintln!("    - {}", e.event_type);
    }

    check!(
        events.iter().any(|e| e.event_type == "done"),
        "expected done event"
    );

    step!("Verifying tool_call_result with denial error");
    // Should have a tool_call_result with is_error=true (the denial)
    let has_denial_result = events.iter().any(|e| {
        e.event_type == "tool_call_result"
            && e.data.get("is_error").and_then(|v| v.as_bool()) == Some(true)
    });
    check!(
        has_denial_result,
        "expected tool_call_result with denial error"
    );

    // Log the denial result
    for e in events.iter().filter(|e| {
        e.event_type == "tool_call_result"
            && e.data.get("is_error").and_then(|v| v.as_bool()) == Some(true)
    }) {
        eprintln!(
            "    Denial result data: {}",
            serde_json::to_string_pretty(&e.data).unwrap_or_default()
        );
    }

    step!("PASS — tool call denied, LLM received denial error");
}
