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
// Test 8: Tool Call SSE Events
// Verifies: tool_call_start and tool_call_result SSE events are emitted
// ============================================================================
#[tokio::test]
#[ignore]
async fn test_tool_call_sse_events() {
    if !require_fireworks_key() {
        return;
    }

    step!("Starting harness with real LLM");
    let harness = TestHarness::start_real()
        .await
        .expect("failed to start real harness");

    // Use the researcher agent — web_search is guaranteed to trigger tool calls
    let turn = converse_with_retry(
        &harness,
        "researcher",
        "Use the web_search tool to search for 'Rust async await'. Report your findings.",
        "tool-call-sse",
    )
    .await;

    assert_response_not_empty(&turn, "tool-call-sse");

    // Collect tool_call_start and tool_call_result events from the SSE stream
    let start_events: Vec<_> = turn
        .sse_events
        .iter()
        .filter(|e| e.event_type == "tool_call_start")
        .collect();

    let result_events: Vec<_> = turn
        .sse_events
        .iter()
        .filter(|e| e.event_type == "tool_call_result")
        .collect();

    step!(
        "tool_call_start events: {}, tool_call_result events: {}",
        start_events.len(),
        result_events.len()
    );

    // Log each tool call start/result pair
    for event in &start_events {
        eprintln!(
            "    tool_call_start: name={}, id={}",
            event
                .data
                .get("name")
                .and_then(|n| n.as_str())
                .unwrap_or("?"),
            event.data.get("id").and_then(|n| n.as_str()).unwrap_or("?")
        );
    }
    for event in &result_events {
        let result_str = event
            .data
            .get("result")
            .and_then(|r| r.as_str())
            .unwrap_or("");
        eprintln!(
            "    tool_call_result: id={}, result={:?}",
            event.data.get("id").and_then(|n| n.as_str()).unwrap_or("?"),
            &result_str[..result_str.len().min(200)]
        );
    }

    step!("Verifying tool_call_start events exist");
    // Assert tool_call_start events exist
    check!(
        !start_events.is_empty(),
        "expected at least one tool_call_start SSE event. All events: {:?}",
        turn.sse_events
            .iter()
            .map(|e| &e.event_type)
            .collect::<Vec<_>>()
    );

    step!("Verifying tool_call_result events exist");
    // Assert tool_call_result events exist
    check!(
        !result_events.is_empty(),
        "expected at least one tool_call_result SSE event. All events: {:?}",
        turn.sse_events
            .iter()
            .map(|e| &e.event_type)
            .collect::<Vec<_>>()
    );

    step!("Verifying start/result event counts match");
    // Each start should have a matching result (same count)
    check_eq!(
        start_events.len(),
        result_events.len(),
        "tool_call_start count should match tool_call_result count"
    );

    step!("Validating tool_call_start data fields");
    // Validate tool_call_start data contains required fields
    for event in &start_events {
        check!(
            event.data.get("name").is_some(),
            "tool_call_start should have 'name' field: {:?}",
            event.data
        );
        check!(
            event.data.get("arguments").is_some(),
            "tool_call_start should have 'arguments' field: {:?}",
            event.data
        );
        check!(
            event.data.get("id").is_some(),
            "tool_call_start should have 'id' field: {:?}",
            event.data
        );
    }

    step!("Validating tool_call_result data fields");
    // Validate tool_call_result data contains required fields
    for event in &result_events {
        check!(
            event.data.get("result").is_some(),
            "tool_call_result should have 'result' field: {:?}",
            event.data
        );
        check!(
            event.data.get("is_error").is_some(),
            "tool_call_result should have 'is_error' field: {:?}",
            event.data
        );
        check!(
            event.data.get("id").is_some(),
            "tool_call_result should have 'id' field: {:?}",
            event.data
        );
    }

    step!(
        "PASS — tool call SSE events verified: {} tool calls, completed in {:?}",
        start_events.len(),
        turn.duration
    );
}
