#![allow(dead_code, unused_imports, unused_mut, unused_variables)]
//! Real LLM E2E tests for the integration tool.
//!
//! These tests use a real LLM (Fireworks) but mock the control plane and
//! integration proxy. The LLM decides which integration tools to call based
//! on the user's natural language request — verifying that the full pipeline
//! works end-to-end with a real model.
//!
//! Tests are `#[ignore]` by default — run them with:
//!
//! ```sh
//! FIREWORKS_API_KEY=<key> cargo test -p bridge-e2e --test integration_real_e2e_tests -- --ignored
//! ```

use bridge_e2e::{check, check_eq, step, ConversationTurn, SseStream, TestHarness};
use std::time::Duration;

const AGENT_ID: &str = "integration-agent";

/// Timeout for waiting for a specific SSE event (e.g., tool_approval_required).
const EVENT_TIMEOUT: Duration = Duration::from_secs(120);

/// Timeout for a full conversation turn (LLM + tool execution + follow-up).
const FULL_TIMEOUT: Duration = Duration::from_secs(300);

/// Maximum retries for flaky LLM responses.
const MAX_RETRIES: usize = 2;

fn require_fireworks_key() -> bool {
    if std::env::var("FIREWORKS_API_KEY").is_err() {
        eprintln!("FIREWORKS_API_KEY not set — skipping real integration E2E test");
        return false;
    }
    true
}

/// Send a conversation turn with retries on empty/error responses.
async fn converse_with_retry(
    harness: &TestHarness,
    message: &str,
    label: &str,
) -> ConversationTurn {
    let mut last_turn = None;
    for attempt in 0..MAX_RETRIES {
        if attempt > 0 {
            step!(
                "[{}] Retrying (attempt {}/{})",
                label,
                attempt + 1,
                MAX_RETRIES
            );
            tokio::time::sleep(Duration::from_secs(2)).await;
        }

        step!(
            "[{}] Sending message: '{}'",
            label,
            &message[..message.len().min(100)]
        );
        let turn = harness
            .converse(AGENT_ID, None, message, FULL_TIMEOUT)
            .await
            .expect("conversation failed");

        let has_error = turn.sse_events.iter().any(|e| e.event_type == "error");

        if !turn.response_text.is_empty() && !has_error {
            step!(
                "[{}] Got response ({} chars)",
                label,
                turn.response_text.len()
            );
            eprintln!(
                "    Response: {:?}",
                &turn.response_text[..turn.response_text.len().min(200)]
            );

            step!(
                "[{}] SSE events received ({} total)",
                label,
                turn.sse_events.len()
            );
            for e in &turn.sse_events {
                eprintln!("    - {}", e.event_type);
            }
            return turn;
        }

        eprintln!(
            "[{}] attempt {} got empty/error response. Events: {:?}",
            label,
            attempt + 1,
            turn.sse_events
                .iter()
                .map(|e| format!(
                    "{}:{}",
                    e.event_type,
                    &e.data.to_string()[..e.data.to_string().len().min(120)]
                ))
                .collect::<Vec<_>>()
        );
        last_turn = Some(turn);
    }

    last_turn.unwrap()
}

/// Helper: create conversation, connect SSE, send message.
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
    let bridge_url = harness.bridge_url();
    let stream = SseStream::connect(bridge_url, &conv_id)
        .await
        .expect("SSE connect failed");

    step!("Sending message: '{}'", &message[..message.len().min(100)]);
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
// Test 3: RequireApproval — approve github__create_pull_request
//
// The LLM calls github__create_pull_request, the bridge blocks and emits
// tool_approval_required. We approve, and the tool executes against the
// mock control plane.
// ============================================================================
#[tokio::test]
#[ignore]
async fn test_real_llm_integration_require_approval_approve() {
    if !require_fireworks_key() {
        return;
    }

    step!("Starting harness with real LLM");
    let harness = TestHarness::start_real()
        .await
        .expect("failed to start real harness");

    let (conv_id, stream) = setup_conversation(
        &harness,
        "Use the github__create_pull_request tool to create a PR with title 'Add feature X', head branch 'feature-x', and base branch 'main'.",
    )
    .await;

    step!("Conversation created: {}", conv_id);

    step!(
        "Waiting for tool_approval_required SSE event (timeout: {:?})",
        EVENT_TIMEOUT
    );
    // Wait for tool_approval_required
    let approval_event = stream
        .wait_for_event("tool_approval_required", EVENT_TIMEOUT)
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

    step!("Verifying integration metadata in approval event");
    // Verify integration metadata in the approval event
    check_eq!(
        approval_event.data["tool_name"].as_str(),
        Some("github__create_pull_request"),
        "tool_name should be github__create_pull_request"
    );
    check_eq!(
        approval_event.data["integration_name"].as_str(),
        Some("github"),
        "integration_name should be present"
    );
    check_eq!(
        approval_event.data["integration_action"].as_str(),
        Some("create_pull_request"),
        "integration_action should be present"
    );

    step!("Listing pending approvals");
    // List pending approvals
    let pending = harness
        .list_approvals(AGENT_ID, &conv_id)
        .await
        .expect("list approvals failed");
    check!(
        !pending.is_empty(),
        "expected at least one pending approval"
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

    step!("Verifying tool_call_result with PR data after approval");
    // Verify tool executed — should have tool_call_result with PR data
    let has_tool_result = events.iter().any(|e| {
        e.event_type == "tool_call_result"
            && e.data
                .get("result")
                .and_then(|r| r.as_str())
                .map(|r| r.contains("pull") || r.contains("123") || r.contains("open"))
                .unwrap_or(false)
    });
    check!(
        has_tool_result,
        "expected tool_call_result with PR data after approval"
    );

    // Log tool results
    for e in events.iter().filter(|e| e.event_type == "tool_call_result") {
        eprintln!(
            "    tool_call_result data: {}",
            serde_json::to_string_pretty(&e.data).unwrap_or_default()
        );
    }

    step!("PASS — github__create_pull_request approved and executed, returned PR data");
}
