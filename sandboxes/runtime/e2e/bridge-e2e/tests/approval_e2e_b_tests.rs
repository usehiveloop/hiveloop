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

    step!(
        "Waiting for tool_call_start SSE event (timeout: {:?})",
        TOOL_CALL_TIMEOUT
    );
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
    eprintln!(
        "    tool_call_start data: {}",
        serde_json::to_string_pretty(&glob_start.data).unwrap_or_default()
    );

    step!("Waiting briefly for denied result to arrive");
    // Wait a moment for the denied result to arrive
    tokio::time::sleep(Duration::from_secs(1)).await;

    let events_so_far = stream.events();

    step!("Checking events so far ({} total)", events_so_far.len());
    for e in &events_so_far {
        eprintln!(
            "    - {} {}",
            e.event_type,
            serde_json::to_string(&e.data)
                .unwrap_or_default()
                .chars()
                .take(120)
                .collect::<String>()
        );
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
    check!(
        !turn.response_text.is_empty(),
        "response should not be empty"
    );
    eprintln!(
        "    Response: {:?}",
        &turn.response_text[..turn.response_text.len().min(200)]
    );

    step!(
        "Listing SSE events received ({} total)",
        turn.sse_events.len()
    );
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
