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
// Test: Streaming — text interleaved with tool calls
// Verifies: content_delta events arrive BEFORE and AFTER tool_call_start/result
// ============================================================================
#[tokio::test]
#[ignore]
async fn test_streaming_text_between_tool_calls() {
    if !require_fireworks_key() {
        return;
    }

    step!("Starting harness with real LLM");
    let harness = TestHarness::start_real()
        .await
        .expect("failed to start real harness");

    let turn = converse_with_retry(
        &harness,
        "streaming-agent",
        "Look up issue ENG-42 and tell me about it.",
        "streaming",
    )
    .await;

    assert_response_not_empty(&turn, "streaming");

    step!("[streaming] Verifying getIssue was called");
    // The agent should have called getIssue
    assert_any_tool_called_in_sse(&turn, &["getIssue"], "streaming");

    // ---- Core streaming assertion ----
    // Verify that content_delta events appear both BEFORE and AFTER tool calls.
    // This is the key behavior: the LLM streams text, then makes a tool call,
    // then streams more text — and the client sees all of it in real time.

    // Find the index of the first tool_call_start event
    let first_tool_start_idx = turn
        .sse_events
        .iter()
        .position(|e| e.event_type == "tool_call_start");

    // Find the index of the last tool_call_result event
    let last_tool_result_idx = turn
        .sse_events
        .iter()
        .rposition(|e| e.event_type == "tool_call_result");

    check!(
        first_tool_start_idx.is_some(),
        "[streaming] expected at least one tool_call_start event"
    );
    check!(
        last_tool_result_idx.is_some(),
        "[streaming] expected at least one tool_call_result event"
    );

    let tool_start = first_tool_start_idx.unwrap();
    let tool_end = last_tool_result_idx.unwrap();

    // Check for content_delta events BEFORE the first tool call
    let deltas_before_tool = turn.sse_events[..tool_start]
        .iter()
        .any(|e| e.event_type == "content_delta");

    // Check for content_delta events AFTER the last tool result
    let deltas_after_tool = turn.sse_events[tool_end + 1..]
        .iter()
        .any(|e| e.event_type == "content_delta");

    // Log the full event sequence for debugging
    let event_sequence: Vec<String> = turn
        .sse_events
        .iter()
        .map(|e| {
            if e.event_type == "content_delta" {
                let text = e.data.get("delta").and_then(|d| d.as_str()).unwrap_or("");
                let preview: String = text.chars().take(40).collect();
                format!("content_delta(\"{}\")", preview)
            } else if e.event_type == "tool_call_start" {
                let name = e.data.get("name").and_then(|n| n.as_str()).unwrap_or("?");
                format!("tool_call_start({})", name)
            } else {
                e.event_type.clone()
            }
        })
        .collect();

    step!(
        "[streaming] Event sequence ({} events)",
        event_sequence.len()
    );
    for (i, e) in event_sequence.iter().enumerate() {
        eprintln!("    [{}] {}", i, e);
    }

    step!("[streaming] Verifying content_delta events BEFORE tool call");
    check!(
        deltas_before_tool,
        "[streaming] expected content_delta events BEFORE tool_call_start. \
         The LLM should stream explanatory text before calling a tool. \
         Events: {:?}",
        event_sequence
    );

    step!("[streaming] Verifying content_delta events AFTER tool call");
    check!(
        deltas_after_tool,
        "[streaming] expected content_delta events AFTER tool_call_result. \
         The LLM should stream a summary after receiving the tool result. \
         Events: {:?}",
        event_sequence
    );

    // Also verify multiple content_delta events (proving incremental streaming,
    // not a single bulk event)
    let delta_count = turn
        .sse_events
        .iter()
        .filter(|e| e.event_type == "content_delta")
        .count();

    step!(
        "[streaming] Verifying incremental streaming ({} content_deltas)",
        delta_count
    );
    check!(
        delta_count >= 3,
        "[streaming] expected at least 3 content_delta events (incremental streaming), got {}. \
         Events: {:?}",
        delta_count,
        event_sequence
    );

    step!(
        "PASS — streaming verified: {} content_deltas, text interleaved with tool calls, completed in {:?}",
        delta_count,
        turn.duration
    );
}
