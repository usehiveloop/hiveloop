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

    step!("Waiting for tool_approval_required SSE event (timeout: {:?})", TOOL_CALL_TIMEOUT);
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
    eprintln!("    Approval event data: {}", serde_json::to_string_pretty(&approval_event.data).unwrap_or_default());

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
    check!(approve_resp.status().is_success(), "approve response is success");

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
        eprintln!("    tool_call_result data: {}", serde_json::to_string_pretty(&e.data).unwrap_or_default());
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

    step!("Waiting for tool_approval_required SSE event (timeout: {:?})", TOOL_CALL_TIMEOUT);
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
    eprintln!("    Approval event data: {}", serde_json::to_string_pretty(&approval_event.data).unwrap_or_default());

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
        eprintln!("    Denial result data: {}", serde_json::to_string_pretty(&e.data).unwrap_or_default());
    }

    step!("PASS — tool call denied, LLM received denial error");
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

    step!("Starting harness with real LLM");
    let harness = TestHarness::start_real()
        .await
        .expect("failed to start real harness");

    let (conv_id, stream) = setup_conversation(
        &harness,
        "Use the Glob tool to list all .json files in the current directory. The pattern should be '*.json'.",
    )
    .await;

    step!("Conversation created: {}", conv_id);

    step!("Waiting for tool_call_start SSE event (timeout: {:?})", TOOL_CALL_TIMEOUT);
    // Wait for the Glob tool_call_start, then its denied result
    let glob_start = stream
        .wait_for_event("tool_call_start", TOOL_CALL_TIMEOUT)
        .await
        .expect("expected at least one tool_call_start");

    step!(
        "First tool call: {}",
        glob_start
            .data
            .get("name")
            .and_then(|n| n.as_str())
            .unwrap_or("?")
    );
    eprintln!("    tool_call_start data: {}", serde_json::to_string_pretty(&glob_start.data).unwrap_or_default());

    step!("Waiting briefly for denied result to arrive");
    // Wait a moment for the denied result to arrive
    tokio::time::sleep(Duration::from_secs(1)).await;

    let events_so_far = stream.events();

    step!("Checking events so far ({} total)", events_so_far.len());
    for e in &events_so_far {
        eprintln!("    - {} {}", e.event_type, serde_json::to_string(&e.data).unwrap_or_default().chars().take(120).collect::<String>());
    }

    step!("Verifying Glob was denied");
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
    check!(
        glob_denied,
        "expected Glob tool call to be denied with error. Tool events: {:?}",
        events_so_far
            .iter()
            .filter(|e| e.event_type.contains("tool"))
            .map(|e| format!("{}: {}", e.event_type, e.data))
            .collect::<Vec<_>>()
    );

    step!("Handling potential fallback tool calls (approving bash if needed)");
    // The LLM may fall back to other tools (including bash which requires approval).
    // If bash is called, approve it so the conversation can complete.
    // Wait for either done or a bash approval request.
    loop {
        let events = stream.events();
        if events.iter().any(|e| e.event_type == "done") {
            step!("Done event received");
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
                step!("Approving fallback tool call {}", req_id);
                let _ = harness
                    .resolve_approval(AGENT_ID, &conv_id, &req_id, "approve")
                    .await;
            }
        }

        tokio::time::sleep(Duration::from_millis(500)).await;

        // Safety timeout
        if events.len() > 100 {
            step!("Too many events ({}), breaking loop", events.len());
            break;
        }
    }

    step!("Waiting for final done event (timeout: {:?})", FULL_TIMEOUT);
    let final_events = stream.wait_for_done(FULL_TIMEOUT).await;

    step!("Final SSE events ({} total)", final_events.len());
    for e in &final_events {
        eprintln!("    - {}", e.event_type);
    }

    check!(
        final_events.iter().any(|e| e.event_type == "done"),
        "expected done event"
    );

    step!("PASS — Glob tool denied, conversation completed after fallback");
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

    step!("Starting harness with real LLM");
    let harness = TestHarness::start_real()
        .await
        .expect("failed to start real harness");

    step!("Sending message: 'Use the Read tool to read the file at /etc/hostname...'");
    let turn = harness
        .converse(
            AGENT_ID,
            None,
            "Use the Read tool to read the file at /etc/hostname. If it doesn't exist, that's fine.",
            FULL_TIMEOUT,
        )
        .await
        .expect("conversation failed");

    step!("Verifying response text is non-empty");
    check!(!turn.response_text.is_empty(), "response should not be empty");
    eprintln!("    Response: {:?}", &turn.response_text[..turn.response_text.len().min(200)]);

    step!("Listing SSE events received ({} total)", turn.sse_events.len());
    for e in &turn.sse_events {
        eprintln!("    - {}", e.event_type);
    }

    let tool_starts: Vec<&str> = turn
        .sse_events
        .iter()
        .filter(|e| e.event_type == "tool_call_start")
        .filter_map(|e| e.data.get("name").and_then(|n| n.as_str()))
        .collect();

    step!("Tools called: {:?}", tool_starts);

    check!(
        tool_starts.iter().any(|t| t.eq_ignore_ascii_case("read")),
        "expected Read tool to be called, got {:?}",
        tool_starts
    );

    step!("Verifying no approval events");
    // No approval events
    let has_approval = turn
        .sse_events
        .iter()
        .any(|e| e.event_type == "tool_approval_required");
    check!(!has_approval, "allowed tool should NOT require approval");

    check!(
        turn.sse_events.iter().any(|e| e.event_type == "done"),
        "expected done event"
    );

    step!("PASS — allowed tool (Read) executed without approval");
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

    step!("Waiting for tool_approval_required SSE event (timeout: {:?})", TOOL_CALL_TIMEOUT);
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
    eprintln!("    Approval event data: {}", serde_json::to_string_pretty(&approval_event.data).unwrap_or_default());

    step!("Waiting for tool_approval_required webhook (timeout: 10s)");
    // Check webhook for tool_approval_required
    let webhook_log = harness
        .wait_for_webhook_type("tool_approval_required", Duration::from_secs(10))
        .await
        .expect("wait for webhook failed");
    webhook_log.assert_has_type("tool_approval_required");

    let approval_webhooks = webhook_log.by_type("tool_approval_required");
    check!(!approval_webhooks.is_empty(), "should have tool_approval_required webhook");

    if let Some(data) = approval_webhooks[0].data() {
        step!("Checking tool_approval_required webhook data");
        eprintln!("    Webhook data: {}", serde_json::to_string_pretty(data).unwrap_or_default());

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
    check!(!resolved_webhooks.is_empty(), "should have tool_approval_resolved webhook");

    if let Some(data) = resolved_webhooks[0].data() {
        step!("Checking tool_approval_resolved webhook data");
        eprintln!("    Webhook data: {}", serde_json::to_string_pretty(data).unwrap_or_default());

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

    check!(events.iter().any(|e| e.event_type == "done"), "expected done event");

    step!("PASS — approval webhook flow verified (required -> resolved -> done)");
}
