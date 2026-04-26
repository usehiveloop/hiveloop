#![allow(dead_code, unused_imports, unused_mut, unused_variables)]
//! Real E2E tests that use actual LLM calls via Fireworks.
//!
//! These tests are `#[ignore]` by default — run them with:
//!
//! ```sh
//! FIREWORKS_API_KEY=<key> cargo test -p bridge-e2e --test real_e2e_tests -- --ignored
//! ```
//!
//! Tests run serially to avoid Fireworks rate limits.

use bridge_e2e::{check, check_eq, step, ConversationTurn, TestHarness};
use std::time::Duration;

/// Default timeout for LLM responses (real model with tool loops).
/// With max_turns=5, each Fireworks round trip can be 15-40s (depending on
/// context size and tool count), so worst case ~240s for a full 5-turn loop.
const LLM_TIMEOUT: Duration = Duration::from_secs(300);

/// Maximum retries for a conversation turn that returns an empty/error response.
/// Real LLM APIs can have transient failures (rate limits, empty responses).
const MAX_RETRIES: usize = 2;

/// Skip test if FIREWORKS_API_KEY is not set.
fn require_fireworks_key() -> bool {
    if std::env::var("FIREWORKS_API_KEY").is_err() {
        eprintln!("FIREWORKS_API_KEY not set — skipping real E2E test");
        return false;
    }
    true
}

/// Send a conversation turn with retries on empty/error responses.
/// Real LLM APIs can intermittently return empty responses or transient errors.
async fn converse_with_retry(
    harness: &TestHarness,
    agent_id: &str,
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
            "[{}] Sending message to '{}': '{}'",
            label,
            agent_id,
            &message[..message.len().min(80)]
        );
        let turn = harness
            .converse(agent_id, None, message, LLM_TIMEOUT)
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
                .map(|e| format!("{}:{}", e.event_type, {
                    let s = e.data.to_string();
                    &s[..s.floor_char_boundary(120.min(s.len()))].to_string()
                }))
                .collect::<Vec<_>>()
        );
        last_turn = Some(turn);
    }

    // Return the last turn — the test assertion will fail with diagnostics
    last_turn.unwrap()
}

/// Assert that at least one of the given tools was called, checking SSE events.
/// Unlike `harness.assert_any_tool_called`, this catches built-in tools (Glob,
/// Grep, Read, etc.) that are handled by the bridge runtime and don't appear in
/// the MCP tool call log.
fn assert_any_tool_called_in_sse(turn: &ConversationTurn, tool_names: &[&str], label: &str) {
    let called_tools: Vec<String> = turn
        .sse_events
        .iter()
        .filter(|e| e.event_type == "tool_call_start")
        .filter_map(|e| {
            e.data
                .get("name")
                .and_then(|n| n.as_str())
                .map(String::from)
        })
        .collect();

    let found = called_tools
        .iter()
        .any(|t| tool_names.contains(&t.as_str()));
    check!(
        found,
        "[{}] at least one of {:?} should be called. Tools called (from SSE): {:?}",
        label,
        tool_names,
        called_tools
    );
}

/// Assert response is non-empty with diagnostic output on failure.
fn assert_response_not_empty(turn: &ConversationTurn, label: &str) {
    check!(
        !turn.response_text.is_empty(),
        "[{}] response should not be empty. SSE events received: {:?}",
        label,
        turn.sse_events
            .iter()
            .map(|e| format!("{}:{}", e.event_type, {
                let s = e.data.to_string();
                &s[..s.floor_char_boundary(200.min(s.len()))].to_string()
            }))
            .collect::<Vec<_>>()
    );
}

// ============================================================================
// Test 2: Nova — Portal Control
// Verifies: listTeamIssues/listTeams, createIssue/submitApprovalRequest, pingHuman
// ============================================================================
#[tokio::test]
#[ignore]
async fn test_nova_portal_control() {
    if !require_fireworks_key() {
        return;
    }

    step!("Starting harness with real LLM");
    let harness = TestHarness::start_real()
        .await
        .expect("failed to start real harness");

    // Turn 1: List issues
    step!("[nova] Turn 1: listing team issues");
    let turn1 = converse_with_retry(
        &harness,
        "portal-control",
        "What issues does the ENG team currently have? Give me a brief status summary.",
        "nova turn1",
    )
    .await;

    assert_response_not_empty(&turn1, "nova turn1");

    step!("[nova turn1] Verifying team/issue tools were called");
    harness
        .assert_any_tool_called(&["listTeamIssues", "listTeams", "getTeam"])
        .expect("team/issue tools should have been called");

    step!(
        "[nova turn1] completed in {:?}, response: {} chars",
        turn1.duration,
        turn1.response_text.len()
    );

    // Clear log for turn 2 assertions
    let log_dir = std::env::temp_dir().join("portal-mcp-logs");
    let _ = std::fs::remove_dir_all(&log_dir);
    let _ = std::fs::create_dir_all(&log_dir);

    // Turn 2: Create issue (separate conversation)
    step!("[nova] Turn 2: creating issue");
    let turn2 = converse_with_retry(
        &harness,
        "portal-control",
        "Create a new high-priority issue on the ENG team titled 'Implement API rate limiting'. You have my approval — go ahead and use createIssue directly.",
        "nova turn2",
    )
    .await;

    assert_response_not_empty(&turn2, "nova turn2");

    step!("[nova turn2] Verifying issue creation/approval tools were called");
    harness
        .assert_any_tool_called(&["createIssue", "submitApprovalRequest"])
        .expect("issue creation or approval tools should have been called");

    step!(
        "[nova turn2] completed in {:?}, response: {} chars",
        turn2.duration,
        turn2.response_text.len()
    );

    // Clear log for turn 3
    let _ = std::fs::remove_dir_all(&log_dir);
    let _ = std::fs::create_dir_all(&log_dir);

    // Turn 3: Ping human (separate conversation)
    step!("[nova] Turn 3: pinging human");
    let turn3 = converse_with_retry(
        &harness,
        "portal-control",
        "Use the pingHuman tool to alert the team lead that we need human review on the rate limiting approach. It's urgent.",
        "nova turn3",
    )
    .await;

    assert_response_not_empty(&turn3, "nova turn3");

    step!("[nova turn3] Verifying pingHuman was called");
    harness
        .assert_tool_called("pingHuman")
        .expect("pingHuman should have been called");

    step!(
        "PASS — nova portal-control all 3 turns completed (turn3: {:?}, {} chars)",
        turn3.duration,
        turn3.response_text.len()
    );
}
