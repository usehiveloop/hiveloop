//! E2E tests for the tool approval flow with a real LLM (Fireworks).
//!
//! These tests are `#[ignore]` by default — run them with:
//!
//! ```sh
//! FIREWORKS_API_KEY=<key> cargo test -p bridge-e2e --test approval_e2e_tests -- --ignored
//! ```

use bridge_e2e::{SseStream, TestHarness};
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
async fn setup_conversation(
    harness: &TestHarness,
    message: &str,
) -> (String, SseStream) {
    let resp = harness
        .create_conversation(AGENT_ID)
        .await
        .expect("create conversation failed");
    let body: serde_json::Value = resp.json().await.expect("parse create conv response");
    let conv_id = body["conversation_id"]
        .as_str()
        .expect("no conversation_id")
        .to_string();

    // Connect to SSE stream BEFORE sending message so we don't miss events
    let bridge_url = format!("http://127.0.0.1:{}", harness.bridge_port);
    let stream = SseStream::connect(&bridge_url, &conv_id)
        .await
        .expect("SSE connect failed");

    // Send message
    let msg_resp = harness
        .send_message(&conv_id, message)
        .await
        .expect("send message failed");
    assert!(
        msg_resp.status().is_success() || msg_resp.status().as_u16() == 202,
        "send message returned {}",
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

    let harness = TestHarness::start_real()
        .await
        .expect("failed to start real harness");

    let (conv_id, stream) =
        setup_conversation(&harness, "Use the bash tool to run: echo hello_approval_test").await;

    eprintln!("[test] conversation created: {}", conv_id);

    // Wait for tool_approval_required
    let approval_event = stream
        .wait_for_event("tool_approval_required", TOOL_CALL_TIMEOUT)
        .await
        .expect("expected tool_approval_required SSE event");

    let request_id = approval_event.data["request_id"]
        .as_str()
        .expect("no request_id")
        .to_string();
    eprintln!(
        "[test] got tool_approval_required: request_id={}, tool={}",
        request_id,
        approval_event.data["tool_name"].as_str().unwrap_or("?")
    );

    // List pending approvals via API
    let pending = harness
        .list_approvals(AGENT_ID, &conv_id)
        .await
        .expect("list approvals failed");
    assert!(
        !pending.is_empty(),
        "expected at least one pending approval"
    );
    assert!(
        pending.iter().any(|a| a["id"].as_str() == Some(&request_id)),
        "expected pending approval with id={}",
        request_id
    );

    // Approve
    let approve_resp = harness
        .resolve_approval(AGENT_ID, &conv_id, &request_id, "approve")
        .await
        .expect("resolve approval failed");
    assert!(approve_resp.status().is_success());
    eprintln!("[test] approved request {}", request_id);

    // Wait for done
    let events = stream.wait_for_done(FULL_TIMEOUT).await;
    assert!(
        events.iter().any(|e| e.event_type == "done"),
        "expected done event"
    );

    // Verify tool executed — should have tool_call_result after approval
    let has_tool_result = events.iter().any(|e| e.event_type == "tool_call_result");
    assert!(has_tool_result, "expected tool_call_result after approval");

    eprintln!("[test] test_approve_tool_call PASSED");
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

    let harness = TestHarness::start_real()
        .await
        .expect("failed to start real harness");

    let (conv_id, stream) =
        setup_conversation(&harness, "Use the bash tool to run: echo denied_test").await;

    eprintln!("[test] conversation created: {}", conv_id);

    // Wait for tool_approval_required
    let approval_event = stream
        .wait_for_event("tool_approval_required", TOOL_CALL_TIMEOUT)
        .await
        .expect("expected tool_approval_required SSE event");

    let request_id = approval_event.data["request_id"]
        .as_str()
        .expect("no request_id")
        .to_string();
    eprintln!("[test] denying request {}", request_id);

    // Deny
    let deny_resp = harness
        .resolve_approval(AGENT_ID, &conv_id, &request_id, "deny")
        .await
        .expect("deny approval failed");
    assert!(deny_resp.status().is_success());

    // Wait for done — the LLM should respond after receiving the denial
    let events = stream.wait_for_done(FULL_TIMEOUT).await;
    assert!(
        events.iter().any(|e| e.event_type == "done"),
        "expected done event"
    );

    // Should have a tool_call_result with is_error=true (the denial)
    let has_denial_result = events.iter().any(|e| {
        e.event_type == "tool_call_result"
            && e.data.get("is_error").and_then(|v| v.as_bool()) == Some(true)
    });
    assert!(has_denial_result, "expected tool_call_result with denial error");

    eprintln!("[test] test_deny_tool_call PASSED");
}

// ============================================================================
// Test 3: Denied tool — Glob is set to "deny"
//
// Glob calls are immediately rejected (no approval prompt).
// The LLM may then try alternative tools — we just verify Glob was denied.
// ============================================================================
#[tokio::test]
#[ignore]
async fn test_denied_tool() {
    if !require_fireworks_key() {
        return;
    }

    let harness = TestHarness::start_real()
        .await
        .expect("failed to start real harness");

    let (conv_id, stream) = setup_conversation(
        &harness,
        "Use the Glob tool to list all .json files in the current directory. The pattern should be '*.json'.",
    )
    .await;

    eprintln!("[test] conversation created: {}", conv_id);

    // Wait for the Glob tool_call_start, then its denied result
    let glob_start = stream
        .wait_for_event("tool_call_start", TOOL_CALL_TIMEOUT)
        .await
        .expect("expected at least one tool_call_start");

    eprintln!(
        "[test] first tool call: {}",
        glob_start.data.get("name").and_then(|n| n.as_str()).unwrap_or("?")
    );

    // Wait a moment for the denied result to arrive
    tokio::time::sleep(Duration::from_secs(1)).await;

    let events_so_far = stream.events();

    // Verify Glob was called and denied
    let glob_denied = events_so_far.iter().any(|e| {
        e.event_type == "tool_call_result"
            && e.data.get("is_error").and_then(|v| v.as_bool()) == Some(true)
            && e.data
                .get("result")
                .and_then(|r| r.as_str())
                .map(|r| r.contains("denied"))
                .unwrap_or(false)
    });
    assert!(
        glob_denied,
        "expected Glob tool call to be denied with error. Events: {:?}",
        events_so_far
            .iter()
            .filter(|e| e.event_type.contains("tool"))
            .map(|e| format!("{}: {}", e.event_type, e.data))
            .collect::<Vec<_>>()
    );

    // The LLM may fall back to other tools (including bash which requires approval).
    // If bash is called, approve it so the conversation can complete.
    // Wait for either done or a bash approval request.
    loop {
        let events = stream.events();
        if events.iter().any(|e| e.event_type == "done") {
            break;
        }

        if let Some(approval) = events
            .iter()
            .find(|e| e.event_type == "tool_approval_required")
        {
            let req_id = approval.data["request_id"]
                .as_str()
                .expect("no request_id")
                .to_string();

            // Check if we already resolved this one
            let pending = harness
                .list_approvals(AGENT_ID, &conv_id)
                .await
                .unwrap_or_default();
            if pending.iter().any(|a| a["id"].as_str() == Some(&req_id)) {
                eprintln!("[test] approving fallback tool call {}", req_id);
                let _ = harness
                    .resolve_approval(AGENT_ID, &conv_id, &req_id, "approve")
                    .await;
            }
        }

        tokio::time::sleep(Duration::from_millis(500)).await;

        // Safety timeout
        if events.len() > 100 {
            eprintln!("[test] too many events, breaking");
            break;
        }
    }

    let final_events = stream.wait_for_done(FULL_TIMEOUT).await;
    assert!(
        final_events.iter().any(|e| e.event_type == "done"),
        "expected done event"
    );

    eprintln!("[test] test_denied_tool PASSED");
}

// ============================================================================
// Test 4: Allowed tool — Read is set to "allow"
//
// No approval required — tool executes immediately.
// ============================================================================
#[tokio::test]
#[ignore]
async fn test_allowed_tool_no_approval() {
    if !require_fireworks_key() {
        return;
    }

    let harness = TestHarness::start_real()
        .await
        .expect("failed to start real harness");

    let turn = harness
        .converse(
            AGENT_ID,
            None,
            "Use the Read tool to read the file at /etc/hostname. If it doesn't exist, that's fine.",
            FULL_TIMEOUT,
        )
        .await
        .expect("conversation failed");

    let tool_starts: Vec<&str> = turn
        .sse_events
        .iter()
        .filter(|e| e.event_type == "tool_call_start")
        .filter_map(|e| e.data.get("name").and_then(|n| n.as_str()))
        .collect();

    eprintln!("[test] tools called: {:?}", tool_starts);

    assert!(
        tool_starts.iter().any(|t| t.eq_ignore_ascii_case("read")),
        "expected Read tool to be called, got {:?}",
        tool_starts
    );

    // No approval events
    let has_approval = turn
        .sse_events
        .iter()
        .any(|e| e.event_type == "tool_approval_required");
    assert!(!has_approval, "allowed tool should NOT require approval");

    assert!(
        turn.sse_events.iter().any(|e| e.event_type == "done"),
        "expected done event"
    );

    eprintln!("[test] test_allowed_tool_no_approval PASSED");
}

// ============================================================================
// Test 5: Webhook events for approval flow
// ============================================================================
#[tokio::test]
#[ignore]
async fn test_approval_webhook_events() {
    if !require_fireworks_key() {
        return;
    }

    let harness = TestHarness::start_real()
        .await
        .expect("failed to start real harness");

    harness
        .clear_webhook_log()
        .await
        .expect("clear webhook log failed");

    let (conv_id, stream) =
        setup_conversation(&harness, "Use the bash tool to run: echo webhook_test").await;

    // Wait for approval event
    let approval_event = stream
        .wait_for_event("tool_approval_required", TOOL_CALL_TIMEOUT)
        .await
        .expect("expected tool_approval_required");

    let request_id = approval_event.data["request_id"]
        .as_str()
        .expect("no request_id")
        .to_string();

    // Check webhook for tool_approval_required
    let webhook_log = harness
        .wait_for_webhook_type("tool_approval_required", Duration::from_secs(10))
        .await
        .expect("wait for webhook failed");
    webhook_log.assert_has_type("tool_approval_required");

    let approval_webhooks = webhook_log.by_type("tool_approval_required");
    assert!(!approval_webhooks.is_empty());

    if let Some(data) = approval_webhooks[0].data() {
        assert_eq!(
            data.get("request_id").and_then(|v| v.as_str()),
            Some(request_id.as_str()),
        );
        assert_eq!(
            data.get("tool_name").and_then(|v| v.as_str()),
            Some("bash"),
        );
    }

    // Approve
    harness
        .resolve_approval(AGENT_ID, &conv_id, &request_id, "approve")
        .await
        .expect("resolve approval failed");

    // Wait for tool_approval_resolved webhook
    let resolved_log = harness
        .wait_for_webhook_type("tool_approval_resolved", Duration::from_secs(10))
        .await
        .expect("wait for resolved webhook failed");
    resolved_log.assert_has_type("tool_approval_resolved");

    let resolved_webhooks = resolved_log.by_type("tool_approval_resolved");
    assert!(!resolved_webhooks.is_empty());

    if let Some(data) = resolved_webhooks[0].data() {
        assert_eq!(
            data.get("decision").and_then(|v| v.as_str()),
            Some("approve"),
        );
    }

    // Let conversation finish
    let events = stream.wait_for_done(FULL_TIMEOUT).await;
    assert!(events.iter().any(|e| e.event_type == "done"));

    eprintln!("[test] test_approval_webhook_events PASSED");
}
