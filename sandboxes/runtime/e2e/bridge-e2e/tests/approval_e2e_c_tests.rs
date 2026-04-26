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
// Test 5: Webhook events for approval flow
// ============================================================================
#[tokio::test]
#[ignore]
async fn test_approval_webhook_events() {
    if !require_fireworks_key() {
        return;
    }

    step!("Starting harness with real LLM");
    let harness = TestHarness::start_real()
        .await
        .expect("failed to start real harness");

    step!("Clearing webhook log");
    harness
        .clear_webhook_log()
        .await
        .expect("clear webhook log failed");

    let (conv_id, stream) =
        setup_conversation(&harness, "Use the bash tool to run: echo webhook_test").await;

    step!(
        "Waiting for tool_approval_required SSE event (timeout: {:?})",
        TOOL_CALL_TIMEOUT
    );
    // Wait for approval event
    let approval_event = stream
        .wait_for_event("tool_approval_required", TOOL_CALL_TIMEOUT)
        .await
        .expect("expected tool_approval_required");

    let request_id = approval_event.data["request_id"]
        .as_str()
        .expect("no request_id")
        .to_string();
    step!("Got tool_approval_required: request_id={}", request_id);
    eprintln!(
        "    Approval event data: {}",
        serde_json::to_string_pretty(&approval_event.data).unwrap_or_default()
    );

    step!("Waiting for tool_approval_required webhook (timeout: 10s)");
    // Check webhook for tool_approval_required
    let webhook_log = harness
        .wait_for_webhook_type("tool_approval_required", Duration::from_secs(10))
        .await
        .expect("wait for webhook failed");
    webhook_log.assert_has_type("tool_approval_required");

    let approval_webhooks = webhook_log.by_type("tool_approval_required");
    check!(
        !approval_webhooks.is_empty(),
        "should have tool_approval_required webhook"
    );

    if let Some(data) = approval_webhooks[0].data() {
        step!("Checking tool_approval_required webhook data");
        eprintln!(
            "    Webhook data: {}",
            serde_json::to_string_pretty(data).unwrap_or_default()
        );

        check_eq!(
            data.get("request_id").and_then(|v| v.as_str()),
            Some(request_id.as_str()),
            "webhook request_id matches SSE request_id"
        );
        check_eq!(
            data.get("tool_name").and_then(|v| v.as_str()),
            Some("bash"),
            "webhook tool_name is bash"
        );
    }

    step!("Approving request {}", request_id);
    // Approve
    harness
        .resolve_approval(AGENT_ID, &conv_id, &request_id, "approve")
        .await
        .expect("resolve approval failed");

    step!("Waiting for tool_approval_resolved webhook (timeout: 10s)");
    // Wait for tool_approval_resolved webhook
    let resolved_log = harness
        .wait_for_webhook_type("tool_approval_resolved", Duration::from_secs(10))
        .await
        .expect("wait for resolved webhook failed");
    resolved_log.assert_has_type("tool_approval_resolved");

    let resolved_webhooks = resolved_log.by_type("tool_approval_resolved");
    check!(
        !resolved_webhooks.is_empty(),
        "should have tool_approval_resolved webhook"
    );

    if let Some(data) = resolved_webhooks[0].data() {
        step!("Checking tool_approval_resolved webhook data");
        eprintln!(
            "    Webhook data: {}",
            serde_json::to_string_pretty(data).unwrap_or_default()
        );

        check_eq!(
            data.get("decision").and_then(|v| v.as_str()),
            Some("approve"),
            "webhook decision is 'approve'"
        );
    }

    step!("Waiting for done event (timeout: {:?})", FULL_TIMEOUT);
    // Let conversation finish
    let events = stream.wait_for_done(FULL_TIMEOUT).await;

    step!("Final SSE events ({} total)", events.len());
    for e in &events {
        eprintln!("    - {}", e.event_type);
    }

    check!(
        events.iter().any(|e| e.event_type == "done"),
        "expected done event"
    );

    step!("PASS — approval webhook flow verified (required -> resolved -> done)");
}
