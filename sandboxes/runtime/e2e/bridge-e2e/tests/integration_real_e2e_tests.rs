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

use bridge_e2e::{ConversationTurn, SseStream, TestHarness};
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
            eprintln!(
                "[{}] retrying (attempt {}/{})",
                label,
                attempt + 1,
                MAX_RETRIES
            );
            tokio::time::sleep(Duration::from_secs(2)).await;
        }

        let turn = harness
            .converse(AGENT_ID, None, message, FULL_TIMEOUT)
            .await
            .expect("conversation failed");

        let has_error = turn.sse_events.iter().any(|e| e.event_type == "error");

        if !turn.response_text.is_empty() && !has_error {
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
    let bridge_url = harness.bridge_url();
    let stream = SseStream::connect(bridge_url, &conv_id)
        .await
        .expect("SSE connect failed");

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
// Test 1: Allow tool executes immediately — github__list_issues
//
// The LLM should call github__list_issues without approval and return
// realistic issues data from the mock control plane.
// ============================================================================
#[tokio::test]
#[ignore]
async fn test_real_llm_integration_allow_executes() {
    if !require_fireworks_key() {
        return;
    }

    let harness = TestHarness::start_real()
        .await
        .expect("failed to start real harness");

    let turn = converse_with_retry(
        &harness,
        "Use the github__list_issues tool to list all open issues. Just call the tool and show me the results.",
        "integration_allow",
    )
    .await;

    // Should have called github__list_issues
    let tool_starts: Vec<&str> = turn
        .sse_events
        .iter()
        .filter(|e| e.event_type == "tool_call_start")
        .filter_map(|e| e.data.get("name").and_then(|n| n.as_str()))
        .collect();

    eprintln!("[test] tools called: {:?}", tool_starts);

    assert!(
        tool_starts.contains(&"github__list_issues"),
        "expected github__list_issues tool call, got {:?}",
        tool_starts
    );

    // No approval events for allow-permission tools
    let has_approval = turn
        .sse_events
        .iter()
        .any(|e| e.event_type == "tool_approval_required");
    assert!(
        !has_approval,
        "allowed integration tool should NOT require approval"
    );

    // Should have a tool result with issues data
    let tool_results: Vec<&str> = turn
        .sse_events
        .iter()
        .filter(|e| e.event_type == "tool_call_result")
        .filter_map(|e| e.data.get("result").and_then(|r| r.as_str()))
        .collect();

    let has_issues_data = tool_results.iter().any(|r| {
        r.contains("Fix login page crash") || r.contains("issues") || r.contains("number")
    });
    assert!(
        has_issues_data,
        "tool result should contain GitHub issues data; got: {:?}",
        tool_results
    );

    assert!(
        turn.sse_events.iter().any(|e| e.event_type == "done"),
        "expected done event"
    );

    eprintln!("[test] test_real_llm_integration_allow_executes PASSED");
}

// ============================================================================
// Test 2: Allow tool — mailchimp__create_campaign
// ============================================================================
#[tokio::test]
#[ignore]
async fn test_real_llm_integration_allow_mailchimp() {
    if !require_fireworks_key() {
        return;
    }

    let harness = TestHarness::start_real()
        .await
        .expect("failed to start real harness");

    let turn = converse_with_retry(
        &harness,
        "Use the mailchimp__create_campaign tool to create a new email campaign with list_id 'list_main' and subject 'Weekly Update'. Call the tool directly.",
        "integration_mailchimp",
    )
    .await;

    let tool_starts: Vec<&str> = turn
        .sse_events
        .iter()
        .filter(|e| e.event_type == "tool_call_start")
        .filter_map(|e| e.data.get("name").and_then(|n| n.as_str()))
        .collect();

    eprintln!("[test] tools called: {:?}", tool_starts);

    assert!(
        tool_starts.contains(&"mailchimp__create_campaign"),
        "expected mailchimp__create_campaign tool call, got {:?}",
        tool_starts
    );

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

    eprintln!("[test] test_real_llm_integration_allow_mailchimp PASSED");
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

    let harness = TestHarness::start_real()
        .await
        .expect("failed to start real harness");

    let (conv_id, stream) = setup_conversation(
        &harness,
        "Use the github__create_pull_request tool to create a PR with title 'Add feature X', head branch 'feature-x', and base branch 'main'.",
    )
    .await;

    eprintln!("[test] conversation created: {}", conv_id);

    // Wait for tool_approval_required
    let approval_event = stream
        .wait_for_event("tool_approval_required", EVENT_TIMEOUT)
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

    // Verify integration metadata in the approval event
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
        .list_approvals(AGENT_ID, &conv_id)
        .await
        .expect("list approvals failed");
    assert!(
        !pending.is_empty(),
        "expected at least one pending approval"
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

    eprintln!("[test] test_real_llm_integration_require_approval_approve PASSED");
}

// ============================================================================
// Test 4: RequireApproval — deny slack__send_message
//
// The LLM calls slack__send_message, we deny it. The LLM should receive
// a denial error and respond gracefully.
// ============================================================================
#[tokio::test]
#[ignore]
async fn test_real_llm_integration_require_approval_deny() {
    if !require_fireworks_key() {
        return;
    }

    let harness = TestHarness::start_real()
        .await
        .expect("failed to start real harness");

    let (conv_id, stream) = setup_conversation(
        &harness,
        "Use the slack__send_message tool to send the message 'Hello team' to channel C01234567.",
    )
    .await;

    eprintln!("[test] conversation created: {}", conv_id);

    // Wait for approval
    let approval_event = stream
        .wait_for_event("tool_approval_required", EVENT_TIMEOUT)
        .await
        .expect("expected tool_approval_required");

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

    // Wait for done — LLM should respond after receiving the denial
    let events = stream.wait_for_done(FULL_TIMEOUT).await;
    assert!(
        events.iter().any(|e| e.event_type == "done"),
        "expected done event"
    );

    // Should have a tool_call_result with is_error=true
    let has_denial = events.iter().any(|e| {
        e.event_type == "tool_call_result"
            && e.data.get("is_error").and_then(|v| v.as_bool()) == Some(true)
    });
    assert!(has_denial, "expected tool_call_result with denial error");

    eprintln!("[test] test_real_llm_integration_require_approval_deny PASSED");
}

// ============================================================================
// Test 5: Deny-permission tool is never exposed to LLM
//
// github__delete_repository has deny permission. The LLM should not see it
// as an available tool. When asked to delete a repo, it should refuse or
// use a different approach — but never call github__delete_repository.
// ============================================================================
#[tokio::test]
#[ignore]
async fn test_real_llm_integration_deny_tool_not_exposed() {
    if !require_fireworks_key() {
        return;
    }

    let harness = TestHarness::start_real()
        .await
        .expect("failed to start real harness");

    let turn = converse_with_retry(
        &harness,
        "Delete the repository named 'my-old-repo' using the github delete_repository integration. If you can't find the tool, just tell me you don't have that capability.",
        "integration_deny",
    )
    .await;

    // The LLM should NEVER call github__delete_repository
    let tool_starts: Vec<&str> = turn
        .sse_events
        .iter()
        .filter(|e| e.event_type == "tool_call_start")
        .filter_map(|e| e.data.get("name").and_then(|n| n.as_str()))
        .collect();

    eprintln!("[test] tools called: {:?}", tool_starts);

    assert!(
        !tool_starts.contains(&"github__delete_repository"),
        "github__delete_repository should NEVER be called (deny permission), but got {:?}",
        tool_starts
    );

    assert!(
        turn.sse_events.iter().any(|e| e.event_type == "done"),
        "expected done event"
    );

    // The LLM should explain it can't do this
    assert!(
        !turn.response_text.is_empty(),
        "expected a text response explaining the tool is unavailable"
    );

    eprintln!("[test] test_real_llm_integration_deny_tool_not_exposed PASSED");
}

// ============================================================================
// Test 6: LLM chooses the right integration tool from natural language
//
// Instead of naming the exact tool, we describe the task in natural language
// and verify the LLM picks the correct integration tool.
// ============================================================================
#[tokio::test]
#[ignore]
async fn test_real_llm_integration_natural_language_routing() {
    if !require_fireworks_key() {
        return;
    }

    let harness = TestHarness::start_real()
        .await
        .expect("failed to start real harness");

    let turn = converse_with_retry(
        &harness,
        "I need to see what open issues we have on GitHub. Can you check?",
        "integration_routing",
    )
    .await;

    let tool_starts: Vec<&str> = turn
        .sse_events
        .iter()
        .filter(|e| e.event_type == "tool_call_start")
        .filter_map(|e| e.data.get("name").and_then(|n| n.as_str()))
        .collect();

    eprintln!("[test] tools called: {:?}", tool_starts);

    // The LLM should route this to github__list_issues
    assert!(
        tool_starts.contains(&"github__list_issues"),
        "expected LLM to call github__list_issues from natural language, got {:?}",
        tool_starts
    );

    assert!(
        turn.sse_events.iter().any(|e| e.event_type == "done"),
        "expected done event"
    );

    eprintln!("[test] test_real_llm_integration_natural_language_routing PASSED");
}
